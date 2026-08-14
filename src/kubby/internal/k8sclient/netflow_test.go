package k8sclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestNetworkTopologyStartsCoreListsConcurrently(t *testing.T) {
	started := make(chan string, 4)
	release := make(chan struct{})
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		resource := request.URL.Path[strings.LastIndex(request.URL.Path, "/")+1:]
		started <- resource
		<-release
		apiVersion, kind := "v1", "ServiceList"
		switch resource {
		case "pods":
			kind = "PodList"
		case "ingresses":
			apiVersion, kind = "networking.k8s.io/v1", "IngressList"
		case "endpointslices":
			apiVersion, kind = "discovery.k8s.io/v1", "EndpointSliceList"
		}
		body := fmt.Sprintf(`{"apiVersion":%q,"kind":%q,"items":[]}`, apiVersion, kind)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    request,
		}, nil
	})
	client, err := kubernetes.NewForConfig(&rest.Config{
		Host:          "https://cluster.test",
		ContentConfig: rest.ContentConfig{ContentType: "application/json"},
		Transport:     transport,
	})
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := NetworkTopology(context.Background(), &Cluster{Clientset: client}, "")
		done <- err
	}()
	seen := map[string]bool{}
	for len(seen) < 4 {
		select {
		case resource := <-started:
			seen[resource] = true
		case <-time.After(time.Second):
			close(release)
			t.Fatalf("only %d/4 list calls started before one was released: %v", len(seen), seen)
		}
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestOptionalIstioKindUsesCachedDiscoveryBeforeRefreshing(t *testing.T) {
	c := &Cluster{idx: &apiIndex{byKind: map[string][]APIKind{}}}
	if _, installed, err := optionalIstioKind(c, istioGatewayKind); err != nil || installed {
		t.Fatalf("first lookup: installed=%v err=%v", installed, err)
	}
	if _, installed, err := optionalIstioKind(c, istioGatewayKind); err != nil || installed {
		t.Fatalf("cached lookup: installed=%v err=%v", installed, err)
	}
}

func TestPodLabelIndexNarrowsCandidatesAndPreservesFullSelector(t *testing.T) {
	pods := map[string][]*corev1.Pod{"apps": {
		{ObjectMeta: metav1.ObjectMeta{Name: "api-prod", Namespace: "apps", Labels: map[string]string{"app": "api", "env": "prod"}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "api-dev", Namespace: "apps", Labels: map[string]string{"app": "api", "env": "dev"}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "web-prod", Namespace: "apps", Labels: map[string]string{"app": "web", "env": "prod"}}},
	}}
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "apps"}, Spec: corev1.ServiceSpec{Selector: map[string]string{"app": "api", "env": "prod"}}}
	candidates := newPodLabelIndex(pods).candidates(svc, pods["apps"])
	if len(candidates) != 2 {
		t.Fatalf("candidate count = %d, want smallest indexed set of 2", len(candidates))
	}
	flow := flowService(svc, candidates, nil, map[string]bool{})
	if len(flow.Pods) != 1 || flow.Pods[0].Name != "api-prod" {
		t.Fatalf("full selector was not preserved: %#v", flow.Pods)
	}
	missing := svc.DeepCopy()
	missing.Spec.Selector["track"] = "canary"
	if got := newPodLabelIndex(pods).candidates(missing, pods["apps"]); len(got) != 0 {
		t.Fatalf("missing indexed label returned %d candidates, want none", len(got))
	}
}

func BenchmarkNetworkTopologyIndexedJoin1000Services10kPods(b *testing.B) {
	pods := make([]*corev1.Pod, 10_000)
	for i := range pods {
		pods[i] = &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("pod-%d", i), Namespace: "bench",
			Labels: map[string]string{"app": fmt.Sprintf("app-%d", i%100)},
		}}
	}
	podsByNamespace := map[string][]*corev1.Pod{"bench": pods}
	index := newPodLabelIndex(podsByNamespace)
	services := make([]corev1.Service, 1_000)
	for i := range services {
		services[i] = corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("service-%d", i), Namespace: "bench"}, Spec: corev1.ServiceSpec{Selector: map[string]string{"app": fmt.Sprintf("app-%d", i%100)}}}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		endpoints := map[string]bool{}
		for serviceIndex := range services {
			svc := &services[serviceIndex]
			_ = flowService(svc, index.candidates(svc, pods), nil, endpoints)
		}
	}
}

func TestFlowServiceUsesEndpointSliceReadiness(t *testing.T) {
	ready, notReady, terminating := true, false, true
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "apps"}, Spec: corev1.ServiceSpec{Selector: map[string]string{"app": "api"}}}
	pods := []*corev1.Pod{
		flowTestPod("ready-by-pod-condition", true),
		flowTestPod("ready-by-endpoint", false),
		flowTestPod("not-ready-endpoint", true),
		flowTestPod("terminating-endpoint", true),
	}
	index := newServiceEndpointReadiness([]discoveryv1.EndpointSlice{{
		ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Labels: map[string]string{discoveryv1.LabelServiceName: "api"}},
		Endpoints: []discoveryv1.Endpoint{
			{TargetRef: &corev1.ObjectReference{Kind: "Pod", Name: "ready-by-endpoint"}, Conditions: discoveryv1.EndpointConditions{Ready: &ready}},
			{TargetRef: &corev1.ObjectReference{Kind: "Pod", Name: "not-ready-endpoint"}, Conditions: discoveryv1.EndpointConditions{Ready: &notReady}},
			{TargetRef: &corev1.ObjectReference{Kind: "Pod", Name: "terminating-endpoint"}, Conditions: discoveryv1.EndpointConditions{Ready: &ready, Terminating: &terminating}},
		},
	}}, "apps", true)
	got := flowService(svc, pods, index, map[string]bool{})
	if got.ReadyPods != 1 || len(got.Pods) != 4 {
		t.Fatalf("service readiness = %d/%d, want EndpointSlice truth 1/4: %#v", got.ReadyPods, len(got.Pods), got.Pods)
	}
	for _, pod := range got.Pods {
		if pod.Name == "ready-by-pod-condition" && pod.IsReady {
			t.Fatal("PodReady must not override a known EndpointSlice that omits the pod")
		}
	}
}

func TestFlowServiceTreatsSuccessfulEmptyEndpointSliceListAsKnownUnready(t *testing.T) {
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "apps"}, Spec: corev1.ServiceSpec{Selector: map[string]string{"app": "api"}}}
	got := flowService(svc, []*corev1.Pod{flowTestPod("api", true)}, newServiceEndpointReadiness(nil, "apps", true), map[string]bool{})
	if got.ReadyPods != 0 || len(got.Pods) != 1 || got.Pods[0].IsReady {
		t.Fatalf("successful empty EndpointSlice list fell back to PodReady: %#v", got)
	}
}

func TestIstioIngressWorkloadUsesFrontingServiceEndpointReadiness(t *testing.T) {
	notReady := false
	pod := flowTestPod("gateway", true)
	pod.Namespace = "istio-system"
	pod.Labels = map[string]string{"istio": "ingressgateway"}
	client := kubefake.NewSimpleClientset(
		pod,
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "istio-ingressgateway", Namespace: "istio-system"}, Spec: corev1.ServiceSpec{Selector: map[string]string{"istio": "ingressgateway"}, ClusterIP: "10.96.0.10"}},
	)
	ic := &istioContext{
		ctx: context.Background(), c: &Cluster{Clientset: client}, scope: "istio-system",
		endpointReadiness: newServiceEndpointReadiness([]discoveryv1.EndpointSlice{{
			ObjectMeta: metav1.ObjectMeta{Name: "gateway", Namespace: "istio-system", Labels: map[string]string{discoveryv1.LabelServiceName: "istio-ingressgateway"}},
			Endpoints:  []discoveryv1.Endpoint{{TargetRef: &corev1.ObjectReference{Kind: "Pod", Name: "gateway"}, Conditions: discoveryv1.EndpointConditions{Ready: &notReady}}},
		}}, "istio-system", true),
		workloadCache: map[string]*istioWorkload{}, outOfScopeEndpoints: map[string]bool{}, warningSet: map[string]bool{},
	}
	wl := ic.ingressWorkload(map[string]string{"istio": "ingressgateway"})
	if wl == nil || wl.Pods != 0 {
		t.Fatalf("gateway readiness = %#v, want zero serving Pods from EndpointSlice truth", wl)
	}
	ic.workloadCache = map[string]*istioWorkload{}
	ic.endpointReadiness = newServiceEndpointReadiness(nil, "istio-system", true)
	if empty := ic.ingressWorkload(map[string]string{"istio": "ingressgateway"}); empty == nil || empty.Pods != 0 {
		t.Fatalf("successful empty EndpointSlice list fell back to PodReady for gateway: %#v", empty)
	}
}

func flowTestPod(name string, ready bool) *corev1.Pod {
	condition := corev1.ConditionFalse
	if ready {
		condition = corev1.ConditionTrue
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "apps", Labels: map[string]string{"app": "api"}},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning, Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: condition}}, ContainerStatuses: []corev1.ContainerStatus{{Name: "app", Ready: ready}}},
	}
}
