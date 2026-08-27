package k8sclient

import (
	"context"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

func TestInvestigationCorrelatesPodFailureOwnerAndEndpoint(t *testing.T) {
	now := time.Now().UTC()
	readyFalse := false
	controller := true
	podUID := types.UID("pod-uid")
	objects := []runtime.Object{
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "api-7d9", Namespace: "demo", UID: podUID,
				CreationTimestamp: metav1.NewTime(now.Add(-5 * time.Minute)),
				Labels:            map[string]string{"app": "api"},
				OwnerReferences:   []metav1.OwnerReference{{Kind: "ReplicaSet", Name: "api-7d9", UID: "rs-uid", Controller: &controller}},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodPending,
				Conditions: []corev1.PodCondition{{
					Type: corev1.PodReady, Status: corev1.ConditionFalse, Reason: "ContainersNotReady",
					LastTransitionTime: metav1.NewTime(now.Add(-4 * time.Minute)),
				}},
				ContainerStatuses: []corev1.ContainerStatus{{
					Name: "api", RestartCount: 3,
					State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff", Message: "manifest unknown"}},
				}},
			},
		},
		&appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{Name: "api-7d9", Namespace: "demo", UID: "rs-uid", OwnerReferences: []metav1.OwnerReference{{Kind: "Deployment", Name: "api", UID: "dep-uid", Controller: &controller}}},
		},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "demo"}, Spec: corev1.ServiceSpec{Selector: map[string]string{"app": "api"}}},
		&discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{Name: "api-1", Namespace: "demo", Labels: map[string]string{discoveryv1.LabelServiceName: "api"}},
			Endpoints:  []discoveryv1.Endpoint{{TargetRef: &corev1.ObjectReference{Kind: "Pod", Name: "api-7d9", UID: podUID}, Conditions: discoveryv1.EndpointConditions{Ready: &readyFalse}}},
		},
		&corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Name: "pull-failed", Namespace: "demo"},
			InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "api-7d9", Namespace: "demo", UID: podUID},
			Type:           corev1.EventTypeWarning, Reason: "Failed", Message: "Failed to pull image", Count: 4,
			LastTimestamp: metav1.NewTime(now.Add(-time.Minute)),
		},
	}
	client := fake.NewSimpleClientset(objects...)
	report, err := InvestigateResource(context.Background(), &Cluster{Clientset: client}, "Pod", "demo", "api-7d9")
	if err != nil {
		t.Fatal(err)
	}
	if report.State != "critical" {
		t.Fatalf("state = %q, want critical", report.State)
	}
	if !hasFinding(report, "ImagePullBackOff") {
		t.Fatalf("missing ImagePullBackOff finding: %#v", report.Findings)
	}
	if !hasFinding(report, "no ready endpoints") {
		t.Fatalf("missing EndpointSlice finding: %#v", report.Findings)
	}
	if !hasRelated(report, "Deployment", "api") || !hasRelated(report, "Service", "api") {
		t.Fatalf("missing related owner/service: %#v", report.Related)
	}
	if !hasAction(report, "rollout") || !hasAction(report, "topology") || !hasAction(report, "yaml") {
		t.Fatalf("missing safe next actions: %#v", report.Actions)
	}
	if len(report.Timeline) < 3 {
		t.Fatalf("timeline has only %d moments: %#v", len(report.Timeline), report.Timeline)
	}
}

func TestInvestigationMarksReadyPodHealthy(t *testing.T) {
	now := time.Now().UTC()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "healthy", Namespace: "demo", UID: "healthy-uid", CreationTimestamp: metav1.NewTime(now.Add(-time.Hour))},
		Status: corev1.PodStatus{
			Phase:      corev1.PodRunning,
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue, LastTransitionTime: metav1.NewTime(now.Add(-50 * time.Minute))}},
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "app", Ready: true, RestartCount: 2,
				State:                corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: metav1.NewTime(now.Add(-55 * time.Minute))}},
				LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "OOMKilled", ExitCode: 137, FinishedAt: metav1.NewTime(now.Add(-2 * time.Hour))}},
			}},
		},
	}
	report, err := InvestigateResource(context.Background(), &Cluster{Clientset: fake.NewSimpleClientset(pod)}, "Pod", "demo", "healthy")
	if err != nil {
		t.Fatal(err)
	}
	if report.State != "healthy" || len(report.Findings) != 0 {
		t.Fatalf("healthy report = state %q, findings %#v", report.State, report.Findings)
	}
	if !strings.Contains(report.Summary, "No deterministic issue") {
		t.Fatalf("unexpected summary: %q", report.Summary)
	}
}

func TestInvestigationDoesNotVerifyRecoveryForNonHealthyPodPhases(t *testing.T) {
	now := metav1.NewTime(time.Now().UTC().Add(-time.Minute))
	tests := []struct {
		name      string
		status    corev1.PodStatus
		wantState string
		wantTitle string
	}{
		{name: "failed without retained evidence", status: corev1.PodStatus{Phase: corev1.PodFailed}, wantState: "critical", wantTitle: "Pod phase is Failed"},
		{name: "evicted without retained evidence", status: corev1.PodStatus{Phase: corev1.PodFailed, Reason: "Evicted", Message: "node pressure"}, wantState: "critical", wantTitle: "Pod was evicted"},
		{name: "pending without conditions", status: corev1.PodStatus{Phase: corev1.PodPending}, wantState: "warning", wantTitle: "Pod is Pending"},
		{name: "unknown", status: corev1.PodStatus{Phase: corev1.PodUnknown}, wantState: "critical", wantTitle: "Pod state is Unknown"},
		{name: "running without ready condition", status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{Name: "app", Ready: true, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}}}}, wantState: "warning", wantTitle: "Pod is running but not ready"},
		{name: "successful finite workload", status: corev1.PodStatus{Phase: corev1.PodSucceeded}, wantState: "healthy"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "subject", Namespace: "demo", UID: "subject-uid", CreationTimestamp: now}, Status: tc.status}
			report, err := InvestigateResource(context.Background(), &Cluster{Clientset: fake.NewSimpleClientset(pod)}, "Pod", "demo", "subject")
			if err != nil {
				t.Fatal(err)
			}
			if report.State != tc.wantState {
				t.Fatalf("state = %q, want %q; findings: %#v", report.State, tc.wantState, report.Findings)
			}
			if tc.wantTitle != "" && !hasFinding(report, tc.wantTitle) {
				t.Fatalf("missing finding %q: %#v", tc.wantTitle, report.Findings)
			}
		})
	}
}

func TestInvestigationMarkdownContainsBoundedEvidence(t *testing.T) {
	report := &IncidentReport{
		Kind: "Pod", Namespace: "demo", Name: "api", State: "critical", ObservedAt: "2026-08-21T00:00:00Z", Summary: "api was OOMKilled",
		Findings:    []IncidentFinding{{Severity: "critical", Title: "api was OOMKilled", Explanation: "Exceeded memory limit.", Confidence: "high", Evidence: []string{"reason: OOMKilled"}}},
		Limitations: []string{"Events are retained by the API server for a limited time."},
	}
	markdown := FormatIncidentMarkdown(report)
	for _, want := range []string{"# Kubby incident: Pod/api", "[CRITICAL] api was OOMKilled", "reason: OOMKilled", "Evidence limits"} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("markdown missing %q:\n%s", want, markdown)
		}
	}
}

func hasFinding(report *IncidentReport, text string) bool {
	for _, finding := range report.Findings {
		if strings.Contains(finding.Title, text) {
			return true
		}
	}
	return false
}

func hasRelated(report *IncidentReport, kind, name string) bool {
	for _, related := range report.Related {
		if related.Kind == kind && related.Name == name {
			return true
		}
	}
	return false
}

func hasAction(report *IncidentReport, action string) bool {
	for _, candidate := range report.Actions {
		if candidate.Action == action {
			return true
		}
	}
	return false
}
