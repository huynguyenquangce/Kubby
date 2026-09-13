package k8sclient

import (
	"context"
	"fmt"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func trafficNamespace(name string, labels map[string]string) *corev1.Namespace {
	return &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels}}
}

func trafficPod(namespace, name, ip string, labels map[string]string, ports ...corev1.ContainerPort) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name, Labels: labels},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Ports: ports}}},
		Status:     corev1.PodStatus{PodIP: ip},
	}
}

func trafficPolicy(namespace, name string, selector map[string]string, types []netv1.PolicyType, ingress []netv1.NetworkPolicyIngressRule, egress []netv1.NetworkPolicyEgressRule) *netv1.NetworkPolicy {
	return &netv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Spec: netv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: selector},
			PolicyTypes: types, Ingress: ingress, Egress: egress,
		},
	}
}

func tcpPort(value intstr.IntOrString) netv1.NetworkPolicyPort {
	protocol := corev1.ProtocolTCP
	return netv1.NetworkPolicyPort{Protocol: &protocol, Port: &value}
}

// trafficFixture is a shop namespace with a web client and an API behind a
// Service whose targetPort is named, plus a debug Pod in a second namespace.
func trafficFixture(extra ...runtime.Object) *Cluster {
	objects := []runtime.Object{
		trafficNamespace("shop", map[string]string{"team": "shop"}),
		trafficNamespace("tools", map[string]string{"team": "tools"}),
		trafficPod("shop", "web", "10.0.0.2", map[string]string{"app": "web"}),
		trafficPod("shop", "api-1", "10.0.0.3", map[string]string{"app": "api"}, corev1.ContainerPort{Name: "http", ContainerPort: 8080}),
		trafficPod("tools", "debug", "10.1.0.5", map[string]string{"app": "debug"}),
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "api"},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{"app": "api"},
				Ports:    []corev1.ServicePort{{Port: 80, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromString("http")}},
			},
		},
	}
	return &Cluster{Clientset: kubefake.NewSimpleClientset(append(objects, extra...)...)}
}

func denyAllIngress(namespace string) *netv1.NetworkPolicy {
	return trafficPolicy(namespace, "deny-all", nil, []netv1.PolicyType{netv1.PolicyTypeIngress}, nil, nil)
}

func checkTraffic(t *testing.T, cluster *Cluster, sourceNamespace, source, kind, destinationNamespace, destination string, port int) *TrafficCheckResult {
	t.Helper()
	result, err := CheckTrafficPolicy(context.Background(), cluster, sourceNamespace, source, kind, destinationNamespace, destination, port, "")
	if err != nil {
		t.Fatalf("CheckTrafficPolicy: %v", err)
	}
	return result
}

func TestCheckTrafficPolicyWithoutPoliciesAllowsAndSaysWhy(t *testing.T) {
	result := checkTraffic(t, trafficFixture(), "shop", "web", "Service", "shop", "api", 80)
	if result.Verdict != TrafficAllowed || len(result.Targets) != 1 {
		t.Fatalf("verdict=%s targets=%d; want allowed with one target", result.Verdict, len(result.Targets))
	}
	target := result.Targets[0]
	if target.Ingress.Isolated || target.Egress.Isolated {
		t.Fatalf("no policy exists, so neither side may be isolated: %+v", target)
	}
	if target.Port != "http (8080)/TCP" {
		t.Fatalf("named targetPort should resolve on the Pod, got %q", target.Port)
	}
	if len(result.Limitations) == 0 || !strings.Contains(result.Limitations[0], "CNI") {
		t.Fatalf("every result must state that enforcement depends on the CNI: %v", result.Limitations)
	}
}

func TestCheckTrafficPolicyDenyAllIngressBlocksWithPolicyName(t *testing.T) {
	result := checkTraffic(t, trafficFixture(denyAllIngress("shop")), "shop", "web", "Service", "shop", "api", 80)
	if result.Verdict != TrafficBlocked {
		t.Fatalf("verdict=%s; want blocked", result.Verdict)
	}
	ingress := result.Targets[0].Ingress
	if !ingress.Isolated || ingress.Allowed || len(ingress.Selecting) != 1 || ingress.Selecting[0].Name != "deny-all" {
		t.Fatalf("ingress verdict should name the isolating policy: %+v", ingress)
	}
	if !strings.Contains(ingress.Reason, "deny-all") || !strings.Contains(ingress.Reason, "shop/web") {
		t.Fatalf("reason should name the policy and the peer: %q", ingress.Reason)
	}
}

func TestCheckTrafficPolicyPodSelectorAndNamedPortRule(t *testing.T) {
	allowWeb := trafficPolicy("shop", "allow-web", map[string]string{"app": "api"}, nil,
		[]netv1.NetworkPolicyIngressRule{{
			From:  []netv1.NetworkPolicyPeer{{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}}}},
			Ports: []netv1.NetworkPolicyPort{tcpPort(intstr.FromString("http"))},
		}}, nil)
	cluster := trafficFixture(denyAllIngress("shop"), allowWeb)

	if got := checkTraffic(t, cluster, "shop", "web", "Service", "shop", "api", 80); got.Verdict != TrafficAllowed {
		t.Fatalf("web → api should be allowed by the named-port rule, got %s: %+v", got.Verdict, got.Targets)
	}
	// A podSelector-only peer means "in the policy's namespace": the same labels
	// in another namespace must not match.
	sameLabels := trafficPod("tools", "web", "10.1.0.9", map[string]string{"app": "web"})
	cluster = trafficFixture(denyAllIngress("shop"), allowWeb, sameLabels)
	if got := checkTraffic(t, cluster, "tools", "web", "Service", "shop", "api", 80); got.Verdict != TrafficBlocked {
		t.Fatalf("a same-label Pod in another namespace must be blocked, got %s", got.Verdict)
	}
	// The numeric request resolves to the same container port name.
	if got := checkTraffic(t, cluster, "shop", "web", "Pod", "shop", "api-1", 8080); got.Verdict != TrafficAllowed {
		t.Fatalf("numeric Pod port should match the named rule, got %s", got.Verdict)
	}
}

func TestCheckTrafficPolicyNamespaceSelectorCombinedWithPodSelector(t *testing.T) {
	allowDebug := trafficPolicy("shop", "allow-tools-debug", map[string]string{"app": "api"}, nil,
		[]netv1.NetworkPolicyIngressRule{{From: []netv1.NetworkPolicyPeer{{
			NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"team": "tools"}},
			PodSelector:       &metav1.LabelSelector{MatchLabels: map[string]string{"app": "debug"}},
		}}}}, nil)
	cluster := trafficFixture(allowDebug)
	if got := checkTraffic(t, cluster, "tools", "debug", "Service", "shop", "api", 80); got.Verdict != TrafficAllowed {
		t.Fatalf("debug in tools should be allowed, got %s", got.Verdict)
	}
	if got := checkTraffic(t, cluster, "shop", "web", "Service", "shop", "api", 80); got.Verdict != TrafficBlocked {
		t.Fatalf("web's namespace does not match team=tools, got %s", got.Verdict)
	}
}

func TestCheckTrafficPolicyEgressIsolationOnSource(t *testing.T) {
	denyEgress := trafficPolicy("tools", "deny-egress", nil, []netv1.PolicyType{netv1.PolicyTypeEgress}, nil, nil)
	result := checkTraffic(t, trafficFixture(denyEgress), "tools", "debug", "Service", "shop", "api", 80)
	if result.Verdict != TrafficBlocked {
		t.Fatalf("verdict=%s; want blocked by source egress", result.Verdict)
	}
	target := result.Targets[0]
	if target.Egress.Allowed || !target.Ingress.Allowed {
		t.Fatalf("egress should block while ingress allows: %+v", target)
	}

	// Egress rules without explicit policyTypes still isolate egress.
	allowDNSOnly := trafficPolicy("tools", "dns-only", nil, nil, nil, []netv1.NetworkPolicyEgressRule{{
		Ports: []netv1.NetworkPolicyPort{{Port: intOrStringPtr(intstr.FromInt32(53)), Protocol: protocolPtr(corev1.ProtocolUDP)}},
	}})
	if got := checkTraffic(t, trafficFixture(allowDNSOnly), "tools", "debug", "Service", "shop", "api", 80); got.Targets[0].Egress.Allowed {
		t.Fatalf("a UDP/53-only egress rule must not allow TCP 8080: %+v", got.Targets[0].Egress)
	}
}

func TestCheckTrafficPolicyIPBlockExceptAndPortRange(t *testing.T) {
	ipRule := func(except ...string) *netv1.NetworkPolicy {
		endPort := int32(8100)
		port := tcpPort(intstr.FromInt32(8000))
		port.EndPort = &endPort
		return trafficPolicy("shop", "ip-range", map[string]string{"app": "api"}, nil,
			[]netv1.NetworkPolicyIngressRule{{
				From:  []netv1.NetworkPolicyPeer{{IPBlock: &netv1.IPBlock{CIDR: "10.1.0.0/16", Except: except}}},
				Ports: []netv1.NetworkPolicyPort{port},
			}}, nil)
	}
	if got := checkTraffic(t, trafficFixture(ipRule()), "tools", "debug", "Service", "shop", "api", 80); got.Verdict != TrafficAllowed {
		t.Fatalf("10.1.0.5 is inside the CIDR and 8080 inside 8000-8100, got %s", got.Verdict)
	}
	if got := checkTraffic(t, trafficFixture(ipRule("10.1.0.5/32")), "tools", "debug", "Service", "shop", "api", 80); got.Verdict != TrafficBlocked {
		t.Fatalf("an except entry must remove the address, got %s", got.Verdict)
	}
}

func TestCheckTrafficPolicyPartialWhenOnlySomeEndpointsAllow(t *testing.T) {
	canary := trafficPod("shop", "api-canary", "10.0.0.4", map[string]string{"app": "api", "track": "canary"}, corev1.ContainerPort{Name: "http", ContainerPort: 8080})
	isolateCanary := trafficPolicy("shop", "isolate-canary", map[string]string{"track": "canary"}, []netv1.PolicyType{netv1.PolicyTypeIngress}, nil, nil)
	result := checkTraffic(t, trafficFixture(canary, isolateCanary), "shop", "web", "Service", "shop", "api", 80)
	if result.Verdict != TrafficPartial || len(result.Targets) != 2 {
		t.Fatalf("verdict=%s targets=%d; want partial across two endpoints", result.Verdict, len(result.Targets))
	}
}

func TestCheckTrafficPolicyRejectsUnknownServicePort(t *testing.T) {
	_, err := CheckTrafficPolicy(context.Background(), trafficFixture(), "shop", "web", "Service", "shop", "api", 443, "")
	if err == nil || !strings.Contains(err.Error(), "80/TCP") {
		t.Fatalf("an unknown Service port should list the ports it does expose, got %v", err)
	}
}

func TestAnnotateFlowPoliciesMarksIsolatedEndpoints(t *testing.T) {
	pods := []corev1.Pod{
		*trafficPod("shop", "api-1", "10.0.0.3", map[string]string{"app": "api"}),
		*trafficPod("shop", "web", "10.0.0.2", map[string]string{"app": "web"}),
	}
	flows := &NetworkFlows{Services: []FlowService{
		{Name: "api", Namespace: "shop", Pods: []FlowPod{{Name: "api-1", Namespace: "shop"}}},
		{Name: "web", Namespace: "shop", Pods: []FlowPod{{Name: "web", Namespace: "shop"}}},
	}}
	policies := []netv1.NetworkPolicy{
		*trafficPolicy("shop", "api-ingress", map[string]string{"app": "api"}, nil, nil, nil),
		*trafficPolicy("shop", "web-egress", map[string]string{"app": "web"}, []netv1.PolicyType{netv1.PolicyTypeEgress}, nil, nil),
	}
	annotateFlowPolicies(flows, pods, policies)
	if !flows.PoliciesAvailable || flows.PolicyCount != 2 || flows.IsolatedPods != 1 {
		t.Fatalf("available=%v count=%d isolated=%d; want true, 2, 1", flows.PoliciesAvailable, flows.PolicyCount, flows.IsolatedPods)
	}
	api, web := flows.PodPolicies["shop/api-1"], flows.PodPolicies["shop/web"]
	if len(api.Ingress) != 1 || api.Ingress[0] != "api-ingress" || len(api.Egress) != 0 {
		t.Fatalf("api policies: %+v", api)
	}
	if len(web.Ingress) != 0 || len(web.Egress) != 1 {
		t.Fatalf("an egress-only policy must not mark ingress isolation: %+v", web)
	}

	// A second annotation of the same snapshot must not accumulate counts.
	annotateFlowPolicies(flows, pods, policies)
	if flows.IsolatedPods != 1 || len(flows.PodPolicies) != 2 {
		t.Fatalf("re-annotation isolated=%d entries=%d; want 1 and 2", flows.IsolatedPods, len(flows.PodPolicies))
	}
}

func TestAnnotateFlowPoliciesListsEachPodOnceAndOmitsUnisolatedPods(t *testing.T) {
	pods := []corev1.Pod{*trafficPod("shop", "api-1", "10.0.0.3", map[string]string{"app": "api"})}
	shared := FlowPod{Name: "api-1", Namespace: "shop"}
	flows := &NetworkFlows{
		Ingresses: []FlowIngress{{Services: []FlowService{{Name: "api", Namespace: "shop", Pods: []FlowPod{shared}}}}},
		Services:  []FlowService{{Name: "api-internal", Namespace: "shop", Pods: []FlowPod{shared}}},
	}
	annotateFlowPolicies(flows, pods, nil)
	if !flows.PoliciesAvailable || flows.IsolatedPods != 0 || len(flows.PodPolicies) != 0 {
		t.Fatalf("no policies: available=%v isolated=%d entries=%v", flows.PoliciesAvailable, flows.IsolatedPods, flows.PodPolicies)
	}
	annotateFlowPolicies(flows, pods, []netv1.NetworkPolicy{*denyAllIngress("shop")})
	if flows.IsolatedPods != 1 || len(flows.PodPolicies) != 1 {
		t.Fatalf("a Pod under two Services is one isolated Pod: isolated=%d entries=%v", flows.IsolatedPods, flows.PodPolicies)
	}
}

// The overlay once re-parsed every selector for every endpoint row: 10k rows ×
// 50 policies allocated 12.4 million times. Allocation counts are deterministic,
// so this guards the per-call compilation without a timing-sensitive assertion.
func TestAnnotateFlowPoliciesAllocationBudget(t *testing.T) {
	shape := perfTopologyShapes[0] // 10k endpoint rows
	pods := perfPods(shape.pods, shape.groups)
	services := perfServices(shape.services, shape.groups)
	flows := networkTopologyFromLists(context.Background(), &Cluster{}, perfNamespace, services, pods,
		&netv1.IngressList{}, &discoveryv1.EndpointSliceList{}, false)
	policies := perfPolicies(50, perfGroupSelector(shape.groups))
	allocs := testing.AllocsPerRun(3, func() { annotateFlowPolicies(flows, pods.Items, policies.Items) })
	const budget = 100_000
	if allocs > budget {
		t.Fatalf("annotateFlowPolicies allocated %.0f times for 10k rows × 50 policies; budget %d", allocs, budget)
	}
	t.Logf("allocations for 10k rows × 50 policies: %.0f (budget %d)", allocs, budget)
}

// API requests must not grow with the number of Pods behind a Service: the
// policy evaluation is CPU-only once the lists are read.
func TestCheckTrafficPolicyRequestsDoNotScaleWithEndpoints(t *testing.T) {
	namespaceRule := trafficPolicy("shop", "from-team", map[string]string{"app": "api"}, nil,
		[]netv1.NetworkPolicyIngressRule{{From: []netv1.NetworkPolicyPeer{{
			NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"team": "shop"}},
		}}}}, nil)
	requests := func(extraEndpoints int) int {
		objects := []runtime.Object{namespaceRule}
		for i := 0; i < extraEndpoints; i++ {
			objects = append(objects, trafficPod("shop", fmt.Sprintf("api-extra-%d", i), fmt.Sprintf("10.0.1.%d", i), map[string]string{"app": "api"},
				corev1.ContainerPort{Name: "http", ContainerPort: 8080}))
		}
		cluster := trafficFixture(objects...)
		client := cluster.Clientset.(*kubefake.Clientset)
		client.ClearActions()
		result := checkTraffic(t, cluster, "shop", "web", "Service", "shop", "api", 80)
		if result.Verdict != TrafficAllowed || len(result.Targets) != extraEndpoints+1 {
			t.Fatalf("verdict=%s targets=%d", result.Verdict, len(result.Targets))
		}
		return len(client.Actions())
	}
	one, many := requests(0), requests(40)
	if one != many || one > 5 {
		t.Fatalf("requests with 1 endpoint=%d, with 41=%d; want equal and at most 5", one, many)
	}
}

func intOrStringPtr(value intstr.IntOrString) *intstr.IntOrString { return &value }

func protocolPtr(protocol corev1.Protocol) *corev1.Protocol { return &protocol }
