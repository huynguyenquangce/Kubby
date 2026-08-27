package k8sclient

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

func TestDrawerSnapshotGetsInitialEvidenceInOneBackendOperation(t *testing.T) {
	client := kubefake.NewSimpleClientset(
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "demo"}, Status: corev1.PodStatus{Phase: corev1.PodRunning}},
		&corev1.Event{ObjectMeta: metav1.ObjectMeta{Name: "started", Namespace: "demo"}, InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "api"}, Type: "Normal", Reason: "Started"},
	)
	var podGets, eventLists int
	client.PrependReactor("get", "pods", func(action clienttesting.Action) (bool, runtime.Object, error) {
		podGets++
		return false, nil, nil
	})
	client.PrependReactor("list", "events", func(action clienttesting.Action) (bool, runtime.Object, error) {
		eventLists++
		return false, nil, nil
	})

	snapshot, err := GetDrawerSnapshot(context.Background(), &Cluster{Clientset: client}, "Pod", "demo", "api")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Detail == nil || snapshot.Detail.Name != "api" {
		t.Fatalf("detail = %#v", snapshot.Detail)
	}
	if len(snapshot.Events) != 1 || snapshot.Events[0].Reason != "Started" {
		t.Fatalf("events = %#v", snapshot.Events)
	}
	if podGets != 1 || eventLists != 1 {
		t.Fatalf("API actions: pod gets=%d event lists=%d, want one each", podGets, eventLists)
	}
}
