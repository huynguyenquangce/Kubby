package k8sclient

import (
	"context"
	"errors"
	"sync"
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

func TestPortForwardSessionCloseIsIdempotentAndConcurrentSafe(t *testing.T) {
	stopCh := make(chan struct{})
	session := &PortForwardSession{stopCh: stopCh}
	var wg sync.WaitGroup

	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			session.Close()
		}()
	}
	wg.Wait()

	select {
	case <-stopCh:
	default:
		t.Fatal("Close() did not close the stop channel")
	}
}

func TestResolveServiceForwardUsesReadyNonTerminatingEndpointSlice(t *testing.T) {
	ready, notReady, terminating := true, false, true
	client := kubefake.NewSimpleClientset(
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "apps"}, Spec: corev1.ServiceSpec{Selector: map[string]string{"app": "api"}}},
		&discoveryv1.EndpointSlice{ObjectMeta: metav1.ObjectMeta{Name: "api-1", Namespace: "apps", Labels: map[string]string{discoveryv1.LabelServiceName: "api"}}, Endpoints: []discoveryv1.Endpoint{
			{TargetRef: &corev1.ObjectReference{Kind: "Pod", Name: "z-not-ready"}, Conditions: discoveryv1.EndpointConditions{Ready: &notReady}},
			{TargetRef: &corev1.ObjectReference{Kind: "Pod", Name: "y-terminating"}, Conditions: discoveryv1.EndpointConditions{Ready: &ready, Terminating: &terminating}},
			{TargetRef: &corev1.ObjectReference{Kind: "Pod", Name: "b-ready"}, Conditions: discoveryv1.EndpointConditions{Ready: &ready}},
			{TargetRef: &corev1.ObjectReference{Kind: "Pod", Name: "a-ready"}, Conditions: discoveryv1.EndpointConditions{Ready: &ready}},
		}},
	)
	got, err := resolvePodForForward(context.Background(), &Cluster{Clientset: client}, "Service", "apps", "api")
	if err != nil || got != "a-ready" {
		t.Fatalf("resolved pod = %q, err=%v; want deterministic ready endpoint a-ready", got, err)
	}
}

func TestResolveServiceForwardFallsBackToPodReady(t *testing.T) {
	client := kubefake.NewSimpleClientset(
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "apps"}, Spec: corev1.ServiceSpec{Selector: map[string]string{"app": "api"}}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "ready", Namespace: "apps", Labels: map[string]string{"app": "api"}}, Status: corev1.PodStatus{Phase: corev1.PodRunning, Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "running-not-ready", Namespace: "apps", Labels: map[string]string{"app": "api"}}, Status: corev1.PodStatus{Phase: corev1.PodRunning}},
	)
	client.PrependReactor("list", "endpointslices", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("forbidden")
	})
	got, err := resolvePodForForward(context.Background(), &Cluster{Clientset: client}, "Service", "apps", "api")
	if err != nil || got != "ready" {
		t.Fatalf("fallback pod = %q, err=%v; want PodReady pod", got, err)
	}
}
