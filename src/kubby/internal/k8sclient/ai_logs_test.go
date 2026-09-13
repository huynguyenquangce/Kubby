package k8sclient

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func TestDiagnosticContextAttachesPreviousLogsOnlyForRestartedContainers(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "api-1"},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "api"}, {Name: "sidecar"}}},
		Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{
			{Name: "api", RestartCount: 3, LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "OOMKilled", ExitCode: 137}}},
			{Name: "sidecar", RestartCount: 0},
		}},
	}
	manifest := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Pod",
		"metadata": map[string]interface{}{"namespace": "shop", "name": "api-1"},
	}}
	cluster := &Cluster{
		Clientset: kubefake.NewSimpleClientset(pod),
		Dynamic:   dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), manifest),
	}

	evidence, err := DiagnosticContext(context.Background(), cluster, "Pod", "shop", "api-1")
	if err != nil {
		t.Fatal(err)
	}
	if evidence.LogContainers != 2 || evidence.PreviousLogContainers != 1 {
		t.Fatalf("log containers=%d previous=%d; want 2 current and 1 previous", evidence.LogContainers, evidence.PreviousLogContainers)
	}
	if !strings.Contains(evidence.Text, "container api (previous instance, before restart 3)") {
		t.Fatalf("the restarted container's previous logs are missing:\n%s", evidence.Text)
	}
	if strings.Contains(evidence.Text, "container sidecar (previous instance") {
		t.Fatalf("a container that never restarted has no previous instance to attach:\n%s", evidence.Text)
	}
}
