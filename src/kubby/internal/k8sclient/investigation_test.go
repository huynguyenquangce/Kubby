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
