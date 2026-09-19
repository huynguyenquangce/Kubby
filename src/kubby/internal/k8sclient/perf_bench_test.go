package k8sclient

// Performance benchmarks for the NetworkPolicy overlay, the traffic check, the
// sidebar fan-out and AI diagnostic log requests. Run with:
//
//	go test ./internal/k8sclient -run '^$' -bench 'Perf' -benchmem -count 5

import (
	"context"
	"errors"
	"fmt"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	netv1 "k8s.io/api/networking/v1"
	apiresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

const perfNamespace = "bench"

func perfPods(n, groups int) *corev1.PodList {
	list := &corev1.PodList{Items: make([]corev1.Pod, n)}
	for i := range list.Items {
		list.Items[i] = corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("pod-%05d", i), Namespace: perfNamespace,
				Labels: map[string]string{"app": fmt.Sprintf("app-%d", i%groups), "tier": fmt.Sprintf("tier-%d", i%4)},
			},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Ports: []corev1.ContainerPort{{Name: "http", ContainerPort: 8080}}}}},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning, PodIP: fmt.Sprintf("10.%d.%d.%d", i>>16&255, i>>8&255, i&255),
				Conditions:        []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
				ContainerStatuses: []corev1.ContainerStatus{{Name: "main", Ready: true}},
			},
		}
	}
	return list
}

func perfServices(n, groups int) *corev1.ServiceList {
	list := &corev1.ServiceList{Items: make([]corev1.Service, n)}
	for i := range list.Items {
		list.Items[i] = corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("svc-%04d", i), Namespace: perfNamespace},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{"app": fmt.Sprintf("app-%d", i%groups)},
				Ports:    []corev1.ServicePort{{Port: 80, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromString("http")}},
			},
		}
	}
	return list
}

// perfPolicies builds a realistic mix: matchLabels, matchExpressions and
// app+tier selectors, with podSelector, namespaceSelector and ipBlock peers.
func perfPolicies(n int, selectorFor func(i int) metav1.LabelSelector) *netv1.NetworkPolicyList {
	tcp, udp := corev1.ProtocolTCP, corev1.ProtocolUDP
	http, dns := intstr.FromInt32(8080), intstr.FromInt32(53)
	list := &netv1.NetworkPolicyList{Items: make([]netv1.NetworkPolicy, n)}
	for i := range list.Items {
		policy := netv1.NetworkPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("policy-%04d", i), Namespace: perfNamespace},
			Spec:       netv1.NetworkPolicySpec{PodSelector: selectorFor(i)},
		}
		switch i % 4 {
		case 0:
			policy.Spec.PolicyTypes = []netv1.PolicyType{netv1.PolicyTypeIngress}
			policy.Spec.Ingress = []netv1.NetworkPolicyIngressRule{{
				From:  []netv1.NetworkPolicyPeer{{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"role": "frontend"}}}},
				Ports: []netv1.NetworkPolicyPort{{Protocol: &tcp, Port: &http}},
			}}
		case 1:
			policy.Spec.PolicyTypes = []netv1.PolicyType{netv1.PolicyTypeIngress, netv1.PolicyTypeEgress}
			policy.Spec.Ingress = []netv1.NetworkPolicyIngressRule{{
				From: []netv1.NetworkPolicyPeer{{
					NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"team": "other"}},
					PodSelector:       &metav1.LabelSelector{MatchLabels: map[string]string{"app": "client"}},
				}},
			}}
			policy.Spec.Egress = []netv1.NetworkPolicyEgressRule{{
				To:    []netv1.NetworkPolicyPeer{{NamespaceSelector: &metav1.LabelSelector{}}},
				Ports: []netv1.NetworkPolicyPort{{Protocol: &udp, Port: &dns}},
			}}
		case 2:
			policy.Spec.Ingress = []netv1.NetworkPolicyIngressRule{{
				From: []netv1.NetworkPolicyPeer{{IPBlock: &netv1.IPBlock{CIDR: "192.168.0.0/16", Except: []string{"192.168.1.0/24"}}}},
			}}
		case 3:
			policy.Spec.PolicyTypes = []netv1.PolicyType{netv1.PolicyTypeEgress}
			policy.Spec.Egress = []netv1.NetworkPolicyEgressRule{{
				To: []netv1.NetworkPolicyPeer{{PodSelector: &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{
					Key: "app", Operator: metav1.LabelSelectorOpIn, Values: []string{"db", "cache"},
				}}}}},
			}}
		}
		list.Items[i] = policy
	}
	return list
}

func perfGroupSelector(groups int) func(i int) metav1.LabelSelector {
	return func(i int) metav1.LabelSelector {
		switch i % 3 {
		case 0:
			return metav1.LabelSelector{MatchLabels: map[string]string{"app": fmt.Sprintf("app-%d", i%groups)}}
		case 1:
			return metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{
				Key: "app", Operator: metav1.LabelSelectorOpIn,
				Values: []string{fmt.Sprintf("app-%d", i%groups), fmt.Sprintf("app-%d", (i+1)%groups)},
			}}}
		}
		return metav1.LabelSelector{MatchLabels: map[string]string{"app": fmt.Sprintf("app-%d", i%groups), "tier": fmt.Sprintf("tier-%d", i%4)}}
	}
}

type perfTopologyShape struct {
	name                   string
	pods, services, groups int
}

// endpoints-10k: every Service fronts 10 Pods. endpoints-100k: the documented
// 1000 Services × 10k Pods / 100 selector groups shape (10 Services per group).
var perfTopologyShapes = []perfTopologyShape{
	{"endpoints-10k", 10_000, 1_000, 1_000},
	{"endpoints-100k", 10_000, 1_000, 100},
}

func BenchmarkPerfAnnotateFlowPolicies(b *testing.B) {
	for _, shape := range perfTopologyShapes {
		pods := perfPods(shape.pods, shape.groups)
		services := perfServices(shape.services, shape.groups)
		flows := networkTopologyFromLists(context.Background(), &Cluster{}, perfNamespace, services, pods,
			&netv1.IngressList{}, &discoveryv1.EndpointSliceList{}, false)
		flowPods := 0
		for _, svc := range flows.Services {
			flowPods += len(svc.Pods)
		}
		for _, policyCount := range []int{0, 50, 500} {
			policies := perfPolicies(policyCount, perfGroupSelector(shape.groups))
			b.Run(fmt.Sprintf("%s/policies-%d", shape.name, policyCount), func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					annotateFlowPolicies(flows, pods.Items, policies.Items)
				}
				b.StopTimer()
				b.ReportMetric(float64(flowPods), "flowpods")
				b.ReportMetric(float64(flows.IsolatedPods), "isolated")
			})
		}
	}
}

func BenchmarkPerfNetworkTopologyWithPolicies(b *testing.B) {
	for _, shape := range perfTopologyShapes {
		pods := perfPods(shape.pods, shape.groups)
		services := perfServices(shape.services, shape.groups)
		for _, policyCount := range []int{0, 50} {
			policies := perfPolicies(policyCount, perfGroupSelector(shape.groups))
			client := kubefake.NewSimpleClientset()
			client.PrependReactor("list", "pods", func(clienttesting.Action) (bool, runtime.Object, error) { return true, pods, nil })
			client.PrependReactor("list", "services", func(clienttesting.Action) (bool, runtime.Object, error) { return true, services, nil })
			client.PrependReactor("list", "networkpolicies", func(clienttesting.Action) (bool, runtime.Object, error) { return true, policies, nil })
			cluster := &Cluster{Clientset: client}
			b.Run(fmt.Sprintf("%s/policies-%d", shape.name, policyCount), func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if _, err := NetworkTopology(context.Background(), cluster, perfNamespace); err != nil {
						b.Fatal(err)
					}
					if i%64 == 0 {
						client.ClearActions()
					}
				}
			})
		}
	}
}

// BenchmarkPerfCheckTrafficPolicyService checks client → Service with N backing
// Pods and P policies in the (shared) namespace, and reports API requests.
func BenchmarkPerfCheckTrafficPolicyService(b *testing.B) {
	selector := func(i int) metav1.LabelSelector {
		switch i % 4 {
		case 0:
			return metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}}
		case 1:
			return metav1.LabelSelector{MatchLabels: map[string]string{"app": "client"}}
		case 2:
			return metav1.LabelSelector{MatchLabels: map[string]string{"app": fmt.Sprintf("other-%d", i)}}
		}
		return metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{
			Key: "tier", Operator: metav1.LabelSelectorOpIn, Values: []string{fmt.Sprintf("tier-%d", i%4)},
		}}}
	}
	for _, endpoints := range []int{10, 1_000} {
		for _, policyCount := range []int{50, 500} {
			pods := perfPods(endpoints, 1)
			for i := range pods.Items {
				pods.Items[i].Labels["app"] = "api"
			}
			source := perfPods(1, 1).Items[0]
			source.Name, source.Labels = "client", map[string]string{"app": "client", "role": "frontend"}
			pods.Items = append(pods.Items, source)
			policies := perfPolicies(policyCount, selector)
			svc := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: perfNamespace},
				Spec: corev1.ServiceSpec{Selector: map[string]string{"app": "api"},
					Ports: []corev1.ServicePort{{Port: 80, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromString("http")}}},
			}
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: perfNamespace, Labels: map[string]string{"team": "bench"}}}
			client := kubefake.NewSimpleClientset(&source, svc, ns)
			client.PrependReactor("list", "pods", func(clienttesting.Action) (bool, runtime.Object, error) { return true, pods, nil })
			client.PrependReactor("list", "networkpolicies", func(clienttesting.Action) (bool, runtime.Object, error) { return true, policies, nil })
			cluster := &Cluster{Clientset: client}
			b.Run(fmt.Sprintf("endpoints-%d/policies-%d", endpoints, policyCount), func(b *testing.B) {
				client.ClearActions()
				result, err := CheckTrafficPolicy(context.Background(), cluster, perfNamespace, "client", "Service", perfNamespace, "api", 80, "")
				if err != nil {
					b.Fatal(err)
				}
				calls := len(client.Actions())
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if _, err := CheckTrafficPolicy(context.Background(), cluster, perfNamespace, "client", "Service", perfNamespace, "api", 80, ""); err != nil {
						b.Fatal(err)
					}
					if i%64 == 0 {
						client.ClearActions()
					}
				}
				b.StopTimer()
				b.ReportMetric(float64(calls), "api-calls")
				b.ReportMetric(float64(len(result.Targets)), "targets")
			})
		}
	}
}

// BenchmarkPerfSidebarCountsRequests reports the list requests and tallies per
// SidebarCounts call. It deliberately injects no latency: client-go's fake
// Invokes holds one mutex around every reactor, so sleeps would serialize and
// hide the countConcurrency=8 semaphore. Use a real cluster for wall time.
// "without-policy-kinds" fails HPA/PDB/NetworkPolicy lists instantly, which
// models the pre-change tally set.
func BenchmarkPerfSidebarCountsRequests(b *testing.B) {
	for _, includeCluster := range []bool{false, true} {
		for _, variant := range []string{"current", "without-policy-kinds"} {
			client := kubefake.NewSimpleClientset()
			meta := newOverviewMetadataClient()
			if variant == "without-policy-kinds" {
				denied := func(clienttesting.Action) (bool, runtime.Object, error) { return true, nil, errors.New("absent") }
				client.PrependReactor("list", "horizontalpodautoscalers", denied)
				client.PrependReactor("list", "poddisruptionbudgets", denied)
				meta.PrependReactor("list", "networkpolicies", denied)
			}
			cluster := &Cluster{Clientset: client, Meta: meta}
			scope := "namespace"
			if includeCluster {
				scope = "cluster"
			}
			b.Run(scope+"/"+variant, func(b *testing.B) {
				client.ClearActions()
				meta.ClearActions()
				counts, err := SidebarCounts(context.Background(), cluster, perfNamespace, includeCluster)
				if err != nil {
					b.Fatal(err)
				}
				requests := len(client.Actions()) + len(meta.Actions())
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if _, err := SidebarCounts(context.Background(), cluster, perfNamespace, includeCluster); err != nil {
						b.Fatal(err)
					}
				}
				b.StopTimer()
				client.ClearActions()
				meta.ClearActions()
				b.ReportMetric(float64(requests), "lists")
				b.ReportMetric(float64(len(counts)), "tallies")
			})
		}
	}
}

// BenchmarkPerfDiagnosticContextLogRequests counts the log requests for a Pod
// with N containers, all restarted or none. Log reads are concurrent, and the
// fake client serializes its reactors, so no latency is modeled: use a real
// cluster for wall time.
func BenchmarkPerfDiagnosticContextLogRequests(b *testing.B) {
	for _, containers := range []int{1, 4} {
		for _, restarted := range []bool{false, true} {
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: perfNamespace, Name: "api-1"}}
			for i := 0; i < containers; i++ {
				name := fmt.Sprintf("c%d", i)
				pod.Spec.Containers = append(pod.Spec.Containers, corev1.Container{Name: name})
				status := corev1.ContainerStatus{Name: name}
				if restarted {
					status.RestartCount = 3
					status.LastTerminationState.Terminated = &corev1.ContainerStateTerminated{Reason: "Error", ExitCode: 1}
				}
				pod.Status.ContainerStatuses = append(pod.Status.ContainerStatuses, status)
			}
			manifest := &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "v1", "kind": "Pod",
				"metadata": map[string]interface{}{"namespace": perfNamespace, "name": "api-1"},
			}}
			client := kubefake.NewSimpleClientset(pod)
			cluster := &Cluster{Clientset: client, Dynamic: dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), manifest)}
			b.Run(fmt.Sprintf("containers-%d/restarted-%v", containers, restarted), func(b *testing.B) {
				client.ClearActions()
				if _, err := DiagnosticContext(context.Background(), cluster, "Pod", perfNamespace, "api-1"); err != nil {
					b.Fatal(err)
				}
				logs, total := 0, len(client.Actions())
				for _, action := range client.Actions() {
					if action.GetSubresource() == "log" {
						logs++
					}
				}
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if _, err := DiagnosticContext(context.Background(), cluster, "Pod", perfNamespace, "api-1"); err != nil {
						b.Fatal(err)
					}
				}
				b.StopTimer()
				client.ClearActions()
				b.ReportMetric(float64(logs), "log-requests")
				b.ReportMetric(float64(total), "clientset-requests")
			})
		}
	}
}

// BenchmarkPerfExplainScheduling measures the scheduling explainer on a cluster
// shaped like the ones it has to answer for: many nodes, many Pods already
// placed, and a target Pod carrying anti-affinity and a spread constraint (the
// two predicates whose naive form is O(nodes × pods)). It also reports the API
// request count, which must stay flat as the cluster grows.
func BenchmarkPerfExplainScheduling(b *testing.B) {
	for _, nodes := range []int{10, 200} {
		for _, pods := range []int{500, 5000} {
			objects := perfSchedulingCluster(nodes, pods)
			b.Run(fmt.Sprintf("nodes-%d/pods-%d", nodes, pods), func(b *testing.B) {
				client := kubefake.NewSimpleClientset(objects...)
				cluster := &Cluster{Clientset: client}
				report, err := ExplainScheduling(context.Background(), cluster, perfNamespace, "target")
				if err != nil {
					b.Fatal(err)
				}
				requests := len(client.Actions())
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if _, err := ExplainScheduling(context.Background(), cluster, perfNamespace, "target"); err != nil {
						b.Fatal(err)
					}
				}
				b.StopTimer()
				b.ReportMetric(float64(requests), "api-calls")
				b.ReportMetric(float64(report.NodesFit), "fitting-nodes")
			})
		}
	}
}

// perfSchedulingCluster builds nodes across three zones, Pods spread over them,
// and one unscheduled target Pod with requests, anti-affinity and a hard spread
// constraint.
func perfSchedulingCluster(nodes, pods int) []runtime.Object {
	zones := []string{"a", "b", "c"}
	objects := make([]runtime.Object, 0, nodes+pods+1)
	for i := 0; i < nodes; i++ {
		name := fmt.Sprintf("node-%04d", i)
		objects = append(objects, &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{
				"kubernetes.io/hostname": name, "topology.kubernetes.io/zone": zones[i%len(zones)],
			}},
			Status: corev1.NodeStatus{
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    apiresource.MustParse("8"),
					corev1.ResourceMemory: apiresource.MustParse("32Gi"),
					corev1.ResourcePods:   apiresource.MustParse("110"),
				},
				Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
			},
		})
	}
	for i := 0; i < pods; i++ {
		objects = append(objects, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: perfNamespace, Name: fmt.Sprintf("placed-%05d", i),
				Labels: map[string]string{"app": fmt.Sprintf("app-%d", i%25)},
			},
			Spec: corev1.PodSpec{
				NodeName: fmt.Sprintf("node-%04d", i%nodes),
				Containers: []corev1.Container{{Name: "main", Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceCPU: apiresource.MustParse("100m"), corev1.ResourceMemory: apiresource.MustParse("256Mi")},
				}}},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		})
	}
	return append(objects, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: perfNamespace, Name: "target", Labels: map[string]string{"app": "app-0"}},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "main", Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceCPU: apiresource.MustParse("500m"), corev1.ResourceMemory: apiresource.MustParse("1Gi")},
			}}},
			Affinity: &corev1.Affinity{PodAntiAffinity: &corev1.PodAntiAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{{
					TopologyKey:   "kubernetes.io/hostname",
					LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "app-0"}},
				}},
			}},
			TopologySpreadConstraints: []corev1.TopologySpreadConstraint{{
				MaxSkew: 1, TopologyKey: "topology.kubernetes.io/zone", WhenUnsatisfiable: corev1.DoNotSchedule,
				LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "app-0"}},
			}},
		},
		Status: corev1.PodStatus{Phase: corev1.PodPending},
	})
}

// BenchmarkPerfSchedulingSimulation isolates Kubby's own work from the client:
// the inputs are built once, so what is measured is the per-node evaluation and
// the selector compilation, not the fake clientset's deep copies.
func BenchmarkPerfSchedulingSimulation(b *testing.B) {
	for _, nodes := range []int{10, 200, 1000} {
		for _, pods := range []int{500, 5000} {
			objects := perfSchedulingCluster(nodes, pods)
			in := &schedulingInputs{claims: map[string]*corev1.PersistentVolumeClaim{}}
			var target *corev1.Pod
			for _, object := range objects {
				switch typed := object.(type) {
				case *corev1.Node:
					in.nodes = append(in.nodes, *typed)
				case *corev1.Pod:
					if typed.Name == "target" {
						target = typed
						continue
					}
					in.pods = append(in.pods, *typed)
				}
			}
			b.Run(fmt.Sprintf("nodes-%d/pods-%d", nodes, pods), func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					report := buildSchedulingReport(target, in)
					if report.NodesTotal != nodes {
						b.Fatalf("nodes = %d, want %d", report.NodesTotal, nodes)
					}
				}
			})
		}
	}
}

// BenchmarkPerfHygieneReport measures the cleanup scan's own work — reference
// indexing and the eight group passes — on a namespace with many objects. The
// inputs are built once so the fake clientset's deep copies are not what is
// measured; the API cost is a fixed set of lists, pinned by the request-count
// assertion in the hygiene tests.
func BenchmarkPerfHygieneReport(b *testing.B) {
	for _, scale := range []int{500, 5000} {
		in := perfHygieneInputs(scale)
		b.Run(fmt.Sprintf("objects-%d", scale), func(b *testing.B) {
			b.ReportAllocs()
			report := buildHygieneReport(perfNamespace, in)
			b.ReportMetric(float64(report.Total), "items")
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				buildHygieneReport(perfNamespace, in)
			}
		})
	}
}

// perfHygieneInputs builds a namespace where most objects are referenced and a
// tenth are not — the shape that makes the reference index do real work.
func perfHygieneInputs(scale int) *hygieneInputs {
	in := &hygieneInputs{errs: map[string]error{}}
	for i := 0; i < scale; i++ {
		name := fmt.Sprintf("app-%05d", i)
		in.pods = append(in.pods, corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Namespace: perfNamespace, Name: name + "-pod",
				OwnerReferences: []metav1.OwnerReference{{Kind: "ReplicaSet", Name: name}}},
			Spec: corev1.PodSpec{
				Volumes: []corev1.Volume{{Name: "config", VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: name}}}}},
				Containers: []corev1.Container{{Name: "main", Image: "registry.example.com/app:1.0", EnvFrom: []corev1.EnvFromSource{{
					SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: name}}}}}},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		})
		in.deployments = append(in.deployments, appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Namespace: perfNamespace, Name: name},
			Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "main", Image: "registry.example.com/app:1.0"}}}}},
		})
		in.configMaps = append(in.configMaps, metav1.PartialObjectMetadata{
			ObjectMeta: metav1.ObjectMeta{Namespace: perfNamespace, Name: name}})
		in.secrets = append(in.secrets, metav1.PartialObjectMetadata{
			ObjectMeta: metav1.ObjectMeta{Namespace: perfNamespace, Name: name}})
		in.services = append(in.services, corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Namespace: perfNamespace, Name: name},
			Spec:       corev1.ServiceSpec{Selector: map[string]string{"app": name}}})
		ready := true
		in.slices = append(in.slices, discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{Namespace: perfNamespace, Name: name + "-abc",
				Labels: map[string]string{discoveryv1.LabelServiceName: name}},
			Endpoints: []discoveryv1.Endpoint{{Addresses: []string{"10.0.0.1"}, Conditions: discoveryv1.EndpointConditions{Ready: &ready}}}})
		if i%10 == 0 {
			in.configMaps = append(in.configMaps, metav1.PartialObjectMetadata{
				ObjectMeta: metav1.ObjectMeta{Namespace: perfNamespace, Name: name + "-orphan"}})
			in.claims = append(in.claims, corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{Namespace: perfNamespace, Name: name + "-data"},
				Status:     corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound}})
		}
	}
	return in
}
