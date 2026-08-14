package k8sclient

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kubefake "k8s.io/client-go/kubernetes/fake"
	metadatafake "k8s.io/client-go/metadata/fake"
	clienttesting "k8s.io/client-go/testing"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsfake "k8s.io/metrics/pkg/client/clientset/versioned/fake"
)

func TestOverviewSnapshotSharesListsAndBuildsSections(t *testing.T) {
	now := metav1.NewTime(time.Now())
	deleting := metav1.NewTime(time.Now().Add(-time.Minute))
	client := kubefake.NewSimpleClientset(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}, Status: corev1.NodeStatus{
			Capacity:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("4"), corev1.ResourceMemory: resource.MustParse("8Gi")},
			NodeInfo:   corev1.NodeSystemInfo{KubeletVersion: "v1.34.0"},
			Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
		}},
		healthyPod("team-a", "healthy"),
		failingPod("team-a", "crashing", nil),
		failingPod("team-a", "terminating", &deleting),
		&corev1.Event{ObjectMeta: metav1.ObjectMeta{Name: "recent", Namespace: "team-a"},
			InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "crashing"},
			Type:           "Warning", Reason: "BackOff", Message: "retrying", LastTimestamp: now},
	)

	meta := newOverviewMetadataClient(
		partialMetadata("v1", "Namespace", "", "team-a"),
		partialMetadata("v1", "Namespace", "", "team-b"),
		partialMetadata("apps/v1", "Deployment", "team-a", "api"),
		partialMetadata("apps/v1", "Deployment", "team-b", "worker"),
	)
	nodeMetric := metricsv1beta1.NodeMetrics{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}, Usage: corev1.ResourceList{
		corev1.ResourceCPU: resource.MustParse("500m"), corev1.ResourceMemory: resource.MustParse("1Gi"),
	}}
	podMetrics := []metricsv1beta1.PodMetrics{
		*podMetric("team-a", "healthy", "100m"),
		*podMetric("team-a", "crashing", "800m"),
		*podMetric("team-a", "terminating", "2"),
	}
	metrics := metricsfake.NewSimpleClientset()
	metrics.PrependReactor("list", "nodes", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, &metricsv1beta1.NodeMetricsList{Items: []metricsv1beta1.NodeMetrics{nodeMetric}}, nil
	})
	metrics.PrependReactor("list", "pods", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, &metricsv1beta1.PodMetricsList{Items: podMetrics}, nil
	})

	snapshot, err := OverviewSnapshot(context.Background(), &Cluster{Clientset: client, Meta: meta, Metrics: metrics})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := snapshot.Stats, (OverviewStats{Nodes: 1, Namespaces: 2, Pods: 2, Deployments: 2, Errors: 1, PodsAvailable: true}); got != want {
		t.Fatalf("stats = %#v, want %#v", got, want)
	}
	if len(snapshot.FailingPods) != 1 || snapshot.FailingPods[0].Name != "crashing" {
		t.Fatalf("failing pods = %#v", snapshot.FailingPods)
	}
	if len(snapshot.NodeMetrics) != 1 || snapshot.NodeMetrics[0].CPUCapacity != 4000 {
		t.Fatalf("node metrics = %#v", snapshot.NodeMetrics)
	}
	if len(snapshot.NodeStatus) != 1 || !snapshot.NodeStatus[0].Ready || snapshot.NodeStatus[0].Pods != 2 || snapshot.NodeStatus[0].Version != "v1.34.0" {
		t.Fatalf("node status = %#v", snapshot.NodeStatus)
	}
	if len(snapshot.TopPods) != 2 || snapshot.TopPods[0].Name != "crashing" {
		t.Fatalf("top live pods = %#v", snapshot.TopPods)
	}
	if len(snapshot.Events) != 1 || snapshot.Events[0].Object != "Pod/crashing" {
		t.Fatalf("events = %#v", snapshot.Events)
	}
	if len(snapshot.Warnings) != 0 {
		t.Fatalf("warnings = %#v", snapshot.Warnings)
	}
	assertOneList(t, client.Actions(), "nodes")
	assertOneList(t, client.Actions(), "pods")
}

func TestOverviewSnapshotKeepsPartialDataWhenPodsOrMetricsFail(t *testing.T) {
	client := kubefake.NewSimpleClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}})
	client.PrependReactor("list", "pods", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "", fmt.Errorf("denied"))
	})
	meta := newOverviewMetadataClient(partialMetadata("v1", "Namespace", "", "default"))

	snapshot, err := OverviewSnapshot(context.Background(), &Cluster{Clientset: client, Meta: meta})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Stats.Nodes != 1 || snapshot.Stats.Namespaces != 1 {
		t.Fatalf("partial stats were lost: %#v", snapshot.Stats)
	}
	if snapshot.Stats.PodsAvailable {
		t.Fatal("pods should be marked unavailable after an explicit list failure")
	}
	if len(snapshot.NodeMetrics) != 0 || len(snapshot.TopPods) != 0 {
		t.Fatal("missing metrics-server should produce empty metric sections")
	}
	if len(snapshot.Warnings) == 0 || !strings.HasPrefix(snapshot.Warnings[0], "pods:") {
		t.Fatalf("warnings = %#v", snapshot.Warnings)
	}
	if !strings.Contains(snapshot.SectionErrors["pods"], "forbidden") {
		t.Fatalf("section errors should identify the unavailable pods section: %#v", snapshot.SectionErrors)
	}
}

func TestOverviewNodeStatusesExposeSchedulingPressureAndIgnoreTerminatingPods(t *testing.T) {
	deleting := metav1.NewTime(time.Now())
	nodes := []corev1.Node{{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-a"},
		Spec:       corev1.NodeSpec{Unschedulable: true},
		Status: corev1.NodeStatus{
			NodeInfo: corev1.NodeSystemInfo{KubeletVersion: "v1.34.1"},
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionFalse},
				{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionTrue},
			},
		},
	}}
	pods := &corev1.PodList{Items: []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "live"}, Spec: corev1.PodSpec{NodeName: "worker-a"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "terminating", DeletionTimestamp: &deleting}, Spec: corev1.PodSpec{NodeName: "worker-a"}},
	}}

	got := overviewNodeStatuses(nodes, pods)
	if len(got) != 1 || got[0].Ready || got[0].Schedulable || got[0].Pods != 1 {
		t.Fatalf("node status = %#v", got)
	}
	if len(got[0].Pressure) != 1 || got[0].Pressure[0] != string(corev1.NodeMemoryPressure) {
		t.Fatalf("pressure = %#v", got[0].Pressure)
	}
}

func BenchmarkOverviewSnapshot10kPods(b *testing.B) {
	pods := &corev1.PodList{Items: make([]corev1.Pod, 10_000)}
	for i := range pods.Items {
		pods.Items[i] = *healthyPod("load", fmt.Sprintf("pod-%05d", i))
	}
	client := kubefake.NewSimpleClientset()
	client.PrependReactor("list", "pods", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, pods, nil
	})
	meta := newOverviewMetadataClient()
	cluster := &Cluster{Clientset: client, Meta: meta}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := OverviewSnapshot(context.Background(), cluster); err != nil {
			b.Fatal(err)
		}
	}
}

func newOverviewMetadataClient(objects ...runtime.Object) *metadatafake.FakeMetadataClient {
	scheme := metadatafake.NewTestScheme()
	metav1.AddMetaToScheme(scheme)
	return metadatafake.NewSimpleMetadataClient(scheme, objects...)
}

func partialMetadata(apiVersion, kind, namespace, name string) *metav1.PartialObjectMetadata {
	return &metav1.PartialObjectMetadata{
		TypeMeta:   metav1.TypeMeta{APIVersion: apiVersion, Kind: kind},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
	}
}

func healthyPod(namespace, name string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Spec:       corev1.PodSpec{NodeName: "node-a"},
		Status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{
			Name: "app", Ready: true, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
		}}},
	}
}

func failingPod(namespace, name string, deletion *metav1.Time) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name, DeletionTimestamp: deletion},
		Spec:       corev1.PodSpec{NodeName: "node-a"},
		Status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{
			Name: "app", RestartCount: 4,
			State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
		}}},
	}
}

func podMetric(namespace, name, cpu string) *metricsv1beta1.PodMetrics {
	return &metricsv1beta1.PodMetrics{
		TypeMeta:   metav1.TypeMeta{APIVersion: "metrics.k8s.io/v1beta1", Kind: "PodMetrics"},
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Containers: []metricsv1beta1.ContainerMetrics{{Name: "app", Usage: corev1.ResourceList{
			corev1.ResourceCPU: resource.MustParse(cpu), corev1.ResourceMemory: resource.MustParse("128Mi"),
		}}},
	}
}

func assertOneList(t *testing.T, actions []clienttesting.Action, resourceName string) {
	t.Helper()
	count := 0
	for _, action := range actions {
		if action.GetVerb() == "list" && action.GetResource().Resource == resourceName {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("%s list calls = %d, want 1", resourceName, count)
	}
}
