package k8sclient

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

// routeFixture is an ingress-nginx controller routing Ingress shop/web to
// Service api, whose Pod listens on a named port.
func routeFixture(className, controller string, extra ...runtime.Object) *kubefake.Clientset {
	class := className
	objects := []runtime.Object{
		trafficNamespace("ingress-nginx", map[string]string{"kubernetes.io/metadata.name": "ingress-nginx"}),
		trafficNamespace("shop", map[string]string{"kubernetes.io/metadata.name": "shop"}),
		&netv1.IngressClass{ObjectMeta: metav1.ObjectMeta{Name: className}, Spec: netv1.IngressClassSpec{Controller: controller}},
		trafficPod("ingress-nginx", "controller-1", "10.9.0.2", map[string]string{
			"app.kubernetes.io/name": "ingress-nginx", "app.kubernetes.io/component": "controller",
		}),
		trafficPod("shop", "api-1", "10.0.0.3", map[string]string{"app": "api"}, corev1.ContainerPort{Name: "http", ContainerPort: 8080}),
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "api"},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{"app": "api"},
				Ports:    []corev1.ServicePort{{Name: "web", Port: 80, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromString("http")}},
			},
		},
		&netv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "web"},
			Spec: netv1.IngressSpec{
				IngressClassName: &class,
				Rules: []netv1.IngressRule{{Host: "shop.test", IngressRuleValue: netv1.IngressRuleValue{HTTP: &netv1.HTTPIngressRuleValue{
					Paths: []netv1.HTTPIngressPath{{Path: "/", Backend: netv1.IngressBackend{Service: &netv1.IngressServiceBackend{
						Name: "api", Port: netv1.ServiceBackendPort{Number: 80},
					}}}},
				}}}},
			},
		},
	}
	return kubefake.NewSimpleClientset(append(objects, extra...)...)
}

func routedHop(t *testing.T, flows *NetworkFlows) (*FlowIngress, *FlowService) {
	t.Helper()
	for i := range flows.Ingresses {
		if flows.Ingresses[i].Name == "web" && len(flows.Ingresses[i].Services) == 1 {
			return &flows.Ingresses[i], &flows.Ingresses[i].Services[0]
		}
	}
	t.Fatalf("Ingress shop/web with one hop not found: %+v", flows.Ingresses)
	return nil, nil
}

func topologyFor(t *testing.T, client *kubefake.Clientset, scope string) *NetworkFlows {
	t.Helper()
	flows, err := NetworkTopology(context.Background(), &Cluster{Clientset: client}, scope)
	if err != nil {
		t.Fatal(err)
	}
	return flows
}

func TestEntryPolicyBlockedRouteIsABrokenPathNamingThePolicy(t *testing.T) {
	flows := topologyFor(t, routeFixture("nginx", "k8s.io/ingress-nginx", denyAllIngress("shop")), "")
	_, hop := routedHop(t, flows)
	if hop.EntryPolicy == nil || hop.EntryPolicy.Verdict != TrafficBlocked || hop.EntryPolicy.Blocked != 1 || hop.EntryPolicy.Total != 1 {
		t.Fatalf("entry policy = %+v; want blocked 1/1", hop.EntryPolicy)
	}
	if len(hop.EntryPolicy.Policies) != 1 || hop.EntryPolicy.Policies[0] != "deny-all" {
		t.Fatalf("blocking policies = %v; want [deny-all]", hop.EntryPolicy.Policies)
	}
	if !strings.Contains(hop.Warning, "NetworkPolicy blocks the ingress-nginx controller from every Pod (deny-all)") {
		t.Fatalf("warning = %q", hop.Warning)
	}
	if !strings.Contains(hop.EntryPolicy.Source, "1 Pod in ingress-nginx") {
		t.Fatalf("source = %q", hop.EntryPolicy.Source)
	}
}

func TestEntryPolicyAllowsControllerNamespaceWithoutAddingAWarning(t *testing.T) {
	allowController := trafficPolicy("shop", "allow-ingress-nginx", map[string]string{"app": "api"}, nil,
		[]netv1.NetworkPolicyIngressRule{{
			From: []netv1.NetworkPolicyPeer{{NamespaceSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"kubernetes.io/metadata.name": "ingress-nginx"},
			}}},
			Ports: []netv1.NetworkPolicyPort{tcpPort(intstr.FromString("http"))},
		}}, nil)
	flows := topologyFor(t, routeFixture("nginx", "k8s.io/ingress-nginx", allowController), "")
	_, hop := routedHop(t, flows)
	if hop.EntryPolicy == nil || hop.EntryPolicy.Verdict != TrafficAllowed || hop.EntryPolicy.Policies[0] != "allow-ingress-nginx" {
		t.Fatalf("entry policy = %+v; want allowed by allow-ingress-nginx", hop.EntryPolicy)
	}
	if strings.Contains(hop.Warning, "NetworkPolicy") {
		t.Fatalf("an allowed route must not gain a policy warning: %q", hop.Warning)
	}
}

func TestEntryPolicyScopedViewEvaluatesControllerNamespaceEgress(t *testing.T) {
	denyControllerEgress := trafficPolicy("ingress-nginx", "controller-egress", nil, []netv1.PolicyType{netv1.PolicyTypeEgress}, nil, nil)
	isolateAPI := trafficPolicy("shop", "api-open", map[string]string{"app": "api"}, nil,
		[]netv1.NetworkPolicyIngressRule{{}}, nil) // isolates, but allows all ingress
	flows := topologyFor(t, routeFixture("nginx", "k8s.io/ingress-nginx", denyControllerEgress, isolateAPI), "shop")
	_, hop := routedHop(t, flows)
	if hop.EntryPolicy == nil || hop.EntryPolicy.Verdict != TrafficBlocked || hop.EntryPolicy.Policies[0] != "controller-egress" {
		t.Fatalf("entry policy = %+v; want blocked by the controller's egress policy", hop.EntryPolicy)
	}
}

func TestEntryPolicyUnknownControllerIsReportedNotGuessed(t *testing.T) {
	flows := topologyFor(t, routeFixture("haproxy", "example.com/haproxy", denyAllIngress("shop")), "")
	entry, hop := routedHop(t, flows)
	if hop.EntryPolicy != nil {
		t.Fatalf("an unidentified controller must not produce a verdict: %+v", hop.EntryPolicy)
	}
	if !strings.Contains(entry.EntryNote, `"example.com/haproxy", which Kubby cannot map to its Pods`) {
		t.Fatalf("entry note = %q", entry.EntryNote)
	}
}

func TestEntryPolicyCostsNoRequestsWithoutPolicies(t *testing.T) {
	client := routeFixture("nginx", "k8s.io/ingress-nginx")
	flows := topologyFor(t, client, "")
	_, hop := routedHop(t, flows)
	if hop.EntryPolicy != nil {
		t.Fatalf("no policy, no verdict: %+v", hop.EntryPolicy)
	}
	for _, action := range client.Actions() {
		if action.GetResource().Resource == "ingressclasses" || action.GetVerb() == "get" {
			t.Fatalf("a cluster without policies must not pay for entry evaluation: %s %s", action.GetVerb(), action.GetResource().Resource)
		}
	}
}
