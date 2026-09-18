package k8sclient

import (
	"errors"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kubefake "k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

func terminatingMetadata(apiVersion, kind, namespace, name string, deletedAgo time.Duration, finalizers ...string) *metav1.PartialObjectMetadata {
	object := partialMetadata(apiVersion, kind, namespace, name)
	deleted := metav1.NewTime(checksTestNow.Add(-deletedAgo))
	object.DeletionTimestamp = &deleted
	object.Finalizers = finalizers
	return object
}

func stuckIndex() *apiIndex {
	listable := func(group, version, resource, kind string, namespaced bool) APIKind {
		return APIKind{GVR: schema.GroupVersionResource{Group: group, Version: version, Resource: resource}, Kind: kind, Namespaced: namespaced, Listable: true}
	}
	return &apiIndex{byKind: map[string][]APIKind{
		"Namespace":             {listable("", "v1", "namespaces", "Namespace", false)},
		"Pod":                   {listable("", "v1", "pods", "Pod", true)},
		"ConfigMap":             {listable("", "v1", "configmaps", "ConfigMap", true)},
		"PersistentVolumeClaim": {listable("", "v1", "persistentvolumeclaims", "PersistentVolumeClaim", true)},
		"Event":                 {listable("", "v1", "events", "Event", true)},
		"Widget":                {listable("kubby.dev", "v1", "widgets", "Widget", true)},
		"TokenReview": {{GVR: schema.GroupVersionResource{Group: "authentication.k8s.io", Version: "v1", Resource: "tokenreviews"},
			Kind: "TokenReview"}}, // not listable
	}}
}

func stuckByName(report *ClusterChecksReport, kind, name string) *StuckObject {
	for i := range report.Stuck.Objects {
		if report.Stuck.Objects[i].Kind == kind && report.Stuck.Objects[i].Name == name {
			return &report.Stuck.Objects[i]
		}
	}
	return nil
}

func stuckFixture(t *testing.T) *Cluster {
	t.Helper()
	deleted := metav1.NewTime(checksTestNow.Add(-2 * time.Hour))
	meta := newOverviewMetadataClient(
		terminatingMetadata("v1", "Namespace", "", "doomed", 2*time.Hour),
		terminatingMetadata("v1", "Pod", "shop", "api-1", time.Hour),
		terminatingMetadata("v1", "ConfigMap", "shop", "held", 10*time.Minute, "example.com/cleanup"),
		partialMetadata("v1", "ConfigMap", "shop", "fresh"),
		terminatingMetadata("v1", "PersistentVolumeClaim", "shop", "data", time.Hour, "kubernetes.io/pvc-protection"),
		terminatingMetadata("kubby.dev/v1", "Widget", "shop", "sample", 30*time.Second, "kubby.dev/hold"),
		terminatingMetadata("v1", "Event", "shop", "noise", time.Hour, "example.com/never"),
	)
	client := kubefake.NewSimpleClientset(
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "doomed", DeletionTimestamp: &deleted},
			Status: corev1.NamespaceStatus{Phase: corev1.NamespaceTerminating, Conditions: []corev1.NamespaceCondition{
				{Type: corev1.NamespaceDeletionDiscoveryFailure, Status: corev1.ConditionTrue, Message: "metrics.k8s.io/v1beta1: the server is currently unable to handle the request"},
				{Type: corev1.NamespaceContentRemaining, Status: corev1.ConditionTrue, Message: "Some resources are remaining: configmaps has 1 resource instances"},
			}},
		},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "api-1"}, Spec: corev1.PodSpec{NodeName: "worker-1"}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-1"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{
			{Type: corev1.NodeReady, Status: corev1.ConditionUnknown},
		}}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "db"},
			Spec: corev1.PodSpec{Volumes: []corev1.Volume{{Name: "data", VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "data"},
			}}}},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)
	return &Cluster{Clientset: client, Meta: meta, idx: stuckIndex()}
}

func TestStuckDeletionsExplainTheUsualCauses(t *testing.T) {
	pinChecksNow(t)
	report := runChecks(t, stuckFixture(t), "")

	if report.Stuck.Scanned != 5 {
		t.Fatalf("scanned = %d; want 5 — Events and the non-listable TokenReview are not scanned", report.Stuck.Scanned)
	}
	namespace := stuckByName(report, "Namespace", "doomed")
	if namespace == nil || namespace.Severity != SeverityCritical ||
		!strings.Contains(findingTitles(namespace.Findings), "API discovery failed during deletion") ||
		!strings.Contains(findingTitles(namespace.Findings), "Content remaining") {
		t.Fatalf("namespace = %+v", namespace)
	}
	pod := stuckByName(report, "Pod", "api-1")
	if pod == nil || pod.Severity != SeverityWarning || !strings.Contains(findingTitles(pod.Findings), "Its node is NotReady") ||
		pod.Command != "kubectl delete pod api-1 -n shop --grace-period=0 --force" {
		t.Fatalf("pod on a NotReady node = %+v", pod)
	}
	held := stuckByName(report, "ConfigMap", "held")
	if held == nil || !held.Stuck || held.Severity != SeverityWarning ||
		held.Command != `kubectl patch configmap held -n shop --type=merge -p '{"metadata":{"finalizers":null}}'` ||
		!strings.Contains(held.Findings[0].Detail, "check the controller for example.com") {
		t.Fatalf("configmap held by a finalizer = %+v", held)
	}
	claim := stuckByName(report, "PersistentVolumeClaim", "data")
	if claim == nil || !strings.Contains(findingTitles(claim.Findings), "Still used by Pods") || !strings.Contains(claim.Findings[0].Detail, "Pod db") {
		t.Fatalf("claim still in use = %+v", claim)
	}
	widget := stuckByName(report, "Widget", "sample")
	if widget == nil || widget.Stuck || widget.Severity != SeverityInfo || widget.Command != `kubectl patch widget.kubby.dev sample -n shop --type=merge -p '{"metadata":{"finalizers":null}}'` {
		t.Fatalf("a deletion 30s old is in progress, not stuck = %+v", widget)
	}
	if stuckByName(report, "ConfigMap", "fresh") != nil || stuckByName(report, "Event", "noise") != nil {
		t.Fatal("live objects and Events are not reported")
	}
	if report.Stuck.Critical != 1 || report.Stuck.Warning != 3 || report.Stuck.Objects[0].Kind != "Namespace" {
		t.Fatalf("critical=%d warning=%d first=%s", report.Stuck.Critical, report.Stuck.Warning, report.Stuck.Objects[0].Kind)
	}
}

func TestStuckDeletionsRespectScopeAndReportUnlistableTypes(t *testing.T) {
	pinChecksNow(t)
	cluster := stuckFixture(t)
	cluster.Meta.(interface {
		PrependReactor(string, string, clienttesting.ReactionFunc)
	}).PrependReactor("list", "widgets", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("forbidden")
	})
	report := runChecks(t, cluster, "shop")
	if stuckByName(report, "Namespace", "doomed") != nil {
		t.Fatal("a view scoped to shop must not report another namespace")
	}
	if report.Stuck.Failed != 1 || !strings.Contains(strings.Join(report.Stuck.Warnings, " "), "widgets.kubby.dev") {
		t.Fatalf("failed=%d warnings=%v", report.Stuck.Failed, report.Stuck.Warnings)
	}
	if len(report.Stuck.Objects) != 3 {
		t.Fatalf("objects in shop = %d; want pod, configmap and claim", len(report.Stuck.Objects))
	}
}

func TestClusterChecksAreCachedPerScope(t *testing.T) {
	pinChecksNow(t)
	cluster := stuckFixture(t)
	client := cluster.Clientset.(*kubefake.Clientset)
	runChecks(t, cluster, "shop")
	first := len(client.Actions())
	runChecks(t, cluster, "shop")
	if len(client.Actions()) != first {
		t.Fatalf("a second call inside the cache window listed again: %d → %d actions", first, len(client.Actions()))
	}
	runChecks(t, cluster, "")
	if len(client.Actions()) == first {
		t.Fatal("another scope is a separate report")
	}
	checksNow = func() time.Time { return checksTestNow.Add(checksCacheTTL + time.Second) }
	before := len(client.Actions())
	runChecks(t, cluster, "shop")
	if len(client.Actions()) == before {
		t.Fatal("an expired cache entry must be rebuilt")
	}
}
