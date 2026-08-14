package k8sclient

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// The Traffic view answers one question: "if a request arrives, where does it
// end up?" These types model that path explicitly — Ingress → Service → Pod —
// instead of reusing the generic RelationNode tree, so the UI can lay each hop
// out in its own lane with the details that matter for that hop.

// FlowPod is one pod endpoint sitting behind a Service.
type FlowPod struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Status    string `json:"status"`
	Ready     string `json:"ready"` // "1/1"
	Restarts  int32  `json:"restarts"`
	Node      string `json:"node"`
	IP        string `json:"ip"`
	OwnerKind string `json:"ownerKind"`
	OwnerName string `json:"ownerName"`
	IsError   bool   `json:"isError"`
	IsReady   bool   `json:"isReady"` // all containers ready — i.e. actually serving
}

// FlowService is a Service together with the pods its selector matches.
type FlowService struct {
	Name      string    `json:"name"`
	Namespace string    `json:"namespace"`
	Type      string    `json:"type"`
	ClusterIP string    `json:"clusterIP"`
	Ports     []string  `json:"ports"`  // "80→8080/TCP"
	Routes    []string  `json:"routes"` // "demo.local/api → :9898" (only when fronted by an entry point)
	Pods      []FlowPod `json:"pods"`
	ReadyPods int       `json:"readyPods"`
	Warning   string    `json:"warning"` // "" when the hop is healthy

	// Via names the object that created these routes when it is not the entry
	// point itself — an Istio VirtualService sits between Gateway and Service.
	Via          string `json:"via"`
	ViaKind      string `json:"viaKind"` // resolvable kind for the drawer
	ViaName      string `json:"viaName"`
	ViaNamespace string `json:"viaNamespace"`
}

// FlowIngress is one entry point into the cluster — a Kubernetes Ingress or an
// Istio Gateway — and every Service that routes behind it.
type FlowIngress struct {
	Kind      string        `json:"kind"`    // "Ingress" | "Gateway"
	RefKind   string        `json:"refKind"` // kind as the drawer resolves it (may carry the API group)
	Name      string        `json:"name"`
	Namespace string        `json:"namespace"`
	Class     string        `json:"class"`
	Address   string        `json:"address"` // load-balancer address, if assigned
	Hosts     []string      `json:"hosts"`
	Ports     []string      `json:"ports"` // listener ports, e.g. "443/HTTPS" (Istio)
	TLS       bool          `json:"tls"`
	Services  []FlowService `json:"services"`
	Warning   string        `json:"warning"`
}

// NetworkFlows is the whole Traffic view payload.
type NetworkFlows struct {
	Ingresses     []FlowIngress `json:"ingresses"`
	Services      []FlowService `json:"services"` // reachable in-cluster only (no Ingress fronts them)
	RoutedCount   int           `json:"routedCount"`
	EndpointCount int           `json:"endpointCount"`
	BrokenCount   int           `json:"brokenCount"` // hops with a warning
	Scope         string        `json:"scope"`       // "" = all namespaces
	Warnings      []string      `json:"warnings"`
}

type serviceEndpointReadiness struct {
	known        map[string]bool            // namespace/service has at least one EndpointSlice
	ready        map[string]map[string]bool // namespace/service -> pod name -> serving now
	allAvailable bool                       // a successful all-namespaces List covered every Service
	nsAvailable  map[string]bool            // successful namespace Lists, including empty results
}

type podLabelIndex map[string]map[string][]*corev1.Pod

func newPodLabelIndex(podsByNamespace map[string][]*corev1.Pod) podLabelIndex {
	index := podLabelIndex{}
	for namespace, pods := range podsByNamespace {
		index.add(namespace, pods)
	}
	return index
}

func (index podLabelIndex) add(namespace string, pods []*corev1.Pod) {
	if index[namespace] == nil {
		index[namespace] = map[string][]*corev1.Pod{}
	}
	for _, pod := range pods {
		for key, value := range pod.Labels {
			labelKey := key + "\x00" + value
			index[namespace][labelKey] = append(index[namespace][labelKey], pod)
		}
	}
}

func (index podLabelIndex) candidates(svc *corev1.Service, fallback []*corev1.Pod) []*corev1.Pod {
	var best []*corev1.Pod
	hasSelector := false
	for key, value := range svc.Spec.Selector {
		hasSelector = true
		candidate, exists := index[svc.Namespace][key+"\x00"+value]
		if !exists {
			return nil
		}
		if best == nil || len(candidate) < len(best) {
			best = candidate
		}
	}
	if !hasSelector {
		return fallback
	}
	return best
}

func newServiceEndpointReadiness(slices []discoveryv1.EndpointSlice, scope string, available bool) *serviceEndpointReadiness {
	index := &serviceEndpointReadiness{known: map[string]bool{}, ready: map[string]map[string]bool{}, nsAvailable: map[string]bool{}}
	if available {
		index.markAvailable(scope)
	}
	index.add(slices)
	return index
}

func (index *serviceEndpointReadiness) markAvailable(namespace string) {
	if index == nil {
		return
	}
	if namespace == "" {
		index.allAvailable = true
	} else {
		index.nsAvailable[namespace] = true
	}
}

func (index *serviceEndpointReadiness) add(slices []discoveryv1.EndpointSlice) {
	if index == nil {
		return
	}
	for i := range slices {
		slice := &slices[i]
		serviceName := slice.Labels[discoveryv1.LabelServiceName]
		if serviceName == "" {
			continue
		}
		key := slice.Namespace + "/" + serviceName
		index.known[key] = true
		if index.ready[key] == nil {
			index.ready[key] = map[string]bool{}
		}
		for _, endpoint := range slice.Endpoints {
			if endpoint.TargetRef == nil || endpoint.TargetRef.Kind != "Pod" || endpoint.TargetRef.Name == "" {
				continue
			}
			terminating := endpoint.Conditions.Terminating != nil && *endpoint.Conditions.Terminating
			ready := endpoint.Conditions.Ready == nil || *endpoint.Conditions.Ready
			if ready && !terminating {
				index.ready[key][endpoint.TargetRef.Name] = true
			}
		}
	}
}

func (index *serviceEndpointReadiness) podReady(serviceKey, podName string) (bool, bool) {
	if index == nil {
		return false, false
	}
	if !index.known[serviceKey] {
		namespace, _, _ := strings.Cut(serviceKey, "/")
		if index.allAvailable || index.nsAvailable[namespace] {
			return false, true
		}
		return false, false
	}
	return index.ready[serviceKey][podName], true
}

// NetworkTopology assembles the Ingress → Service → Pod topology for a namespace
// ("" = all namespaces).
//
// Everything is listed once up front and joined in memory: one List per kind
// instead of one per Service keeps the view fast on a busy cluster, and gives a
// single consistent snapshot rather than a set of reads taken seconds apart.
// Objects with a deletion timestamp are dropped — a deleted-but-still-finalizing
// Ingress or Service must not keep showing up as live routing.
func NetworkTopology(ctx context.Context, c *Cluster, namespace string) (*NetworkFlows, error) {
	var svcList *corev1.ServiceList
	var podList *corev1.PodList
	var ingList *netv1.IngressList
	var endpointSlices *discoveryv1.EndpointSliceList
	errs := make([]error, 4)
	var wg sync.WaitGroup
	tasks := []func(){
		func() { svcList, errs[0] = c.Clientset.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{}) },
		func() { podList, errs[1] = c.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{}) },
		func() {
			ingList, errs[2] = c.Clientset.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{})
		},
		func() {
			endpointSlices, errs[3] = c.Clientset.DiscoveryV1().EndpointSlices(namespace).List(ctx, metav1.ListOptions{})
		},
	}
	for _, task := range tasks {
		wg.Add(1)
		go func(run func()) { defer wg.Done(); run() }(task)
	}
	wg.Wait()
	for index, name := range []string{"services", "pods", "ingresses"} {
		if errs[index] != nil {
			return nil, fmt.Errorf("%s: %w", name, errs[index])
		}
	}
	if endpointSlices == nil {
		endpointSlices = &discoveryv1.EndpointSliceList{}
	}
	out := networkTopologyFromLists(ctx, c, namespace, svcList, podList, ingList, endpointSlices, errs[3] == nil)
	if errs[3] != nil {
		out.Warnings = append(out.Warnings, fmt.Sprintf("endpoint slices: %v; falling back to PodReady conditions", errs[3]))
	}
	return out, nil
}

// networkTopologyFromLists is shared by Traffic and Cluster structure. Keeping
// the join separate lets the structure snapshot fetch all required kinds in
// parallel while preserving exactly one List per core kind.
func networkTopologyFromLists(
	ctx context.Context,
	c *Cluster,
	namespace string,
	svcList *corev1.ServiceList,
	podList *corev1.PodList,
	ingList *netv1.IngressList,
	endpointSlices *discoveryv1.EndpointSliceList,
	endpointSlicesAvailable bool,
) *NetworkFlows {
	out := &NetworkFlows{Ingresses: []FlowIngress{}, Services: []FlowService{}, Scope: namespace, Warnings: []string{}}
	endpointReadiness := newServiceEndpointReadiness(endpointSlices.Items, namespace, endpointSlicesAvailable)

	svcByKey := map[string]*corev1.Service{}
	for i := range svcList.Items {
		svc := &svcList.Items[i]
		if svc.DeletionTimestamp != nil {
			continue
		}
		svcByKey[svc.Namespace+"/"+svc.Name] = svc
	}
	podsByNs := map[string][]*corev1.Pod{}
	for i := range podList.Items {
		pod := &podList.Items[i]
		if pod.DeletionTimestamp != nil {
			continue
		}
		podsByNs[pod.Namespace] = append(podsByNs[pod.Namespace], pod)
	}
	podLabels := newPodLabelIndex(podsByNs)

	endpoints := map[string]bool{} // ns/pod, deduped across every flow
	fronted := map[string]bool{}   // ns/svc already shown under an Ingress

	for i := range ingList.Items {
		ing := &ingList.Items[i]
		if ing.DeletionTimestamp != nil {
			continue
		}
		fi := flowIngress(ing, svcByKey, podsByNs, podLabels, endpointReadiness, fronted, endpoints)
		out.Ingresses = append(out.Ingresses, fi)
	}
	// Istio expresses the same thing with Gateway + VirtualService. Appending
	// here (before the internal-services pass) means a mesh-routed Service is
	// marked as fronted and does not also appear as "internal only".
	istioIngresses, istioWarnings := istioFlows(ctx, c, namespace, svcByKey, podsByNs, podLabels, endpointReadiness, fronted, endpoints)
	out.Ingresses = append(out.Ingresses, istioIngresses...)
	out.Warnings = append(out.Warnings, istioWarnings...)
	sort.Slice(out.Ingresses, func(i, j int) bool {
		a, b := out.Ingresses[i], out.Ingresses[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind // group the Gateways together, then the Ingresses
		}
		return a.Namespace+"/"+a.Name < b.Namespace+"/"+b.Name
	})

	keys := make([]string, 0, len(svcByKey))
	for k := range svcByKey {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if fronted[k] {
			continue
		}
		svc := svcByKey[k]
		out.Services = append(out.Services, flowService(svc, podLabels.candidates(svc, podsByNs[svc.Namespace]), endpointReadiness, endpoints))
	}

	out.RoutedCount = len(fronted)
	out.EndpointCount = len(endpoints)
	for _, fi := range out.Ingresses {
		if fi.Warning != "" {
			out.BrokenCount++
		}
		for _, s := range fi.Services {
			if s.Warning != "" {
				out.BrokenCount++
			}
		}
	}
	for _, s := range out.Services {
		if s.Warning != "" {
			out.BrokenCount++
		}
	}
	return out
}

func flowIngress(
	ing *netv1.Ingress,
	svcByKey map[string]*corev1.Service,
	podsByNs map[string][]*corev1.Pod,
	podLabels podLabelIndex,
	endpointReadiness *serviceEndpointReadiness,
	fronted map[string]bool,
	endpoints map[string]bool,
) FlowIngress {
	fi := FlowIngress{
		Kind: "Ingress", RefKind: "Ingress",
		Name: ing.Name, Namespace: ing.Namespace,
		Hosts: []string{}, Ports: []string{}, Services: []FlowService{},
		TLS: len(ing.Spec.TLS) > 0,
	}
	if ing.Spec.IngressClassName != nil {
		fi.Class = *ing.Spec.IngressClassName
	}
	for _, lb := range ing.Status.LoadBalancer.Ingress {
		if lb.Hostname != "" {
			fi.Address = lb.Hostname
		} else if lb.IP != "" {
			fi.Address = lb.IP
		}
	}

	// Collect the routes per backend service, so a service referenced by three
	// paths appears once with all three routes listed under it.
	order := []string{}
	routes := map[string][]string{}
	addRoute := func(svcName, host, path, port string) {
		if _, seen := routes[svcName]; !seen {
			order = append(order, svcName)
		}
		label := host + path
		if label == "" {
			label = "/"
		}
		if port != "" {
			label += " → :" + port
		}
		routes[svcName] = append(routes[svcName], label)
	}

	seenHost := map[string]bool{}
	for _, rule := range ing.Spec.Rules {
		if rule.Host != "" && !seenHost[rule.Host] {
			seenHost[rule.Host] = true
			fi.Hosts = append(fi.Hosts, rule.Host)
		}
		if rule.HTTP == nil {
			continue
		}
		for _, p := range rule.HTTP.Paths {
			if p.Backend.Service == nil {
				continue
			}
			addRoute(p.Backend.Service.Name, rule.Host, p.Path, backendPort(p.Backend.Service.Port))
		}
	}
	if ing.Spec.DefaultBackend != nil && ing.Spec.DefaultBackend.Service != nil {
		addRoute(ing.Spec.DefaultBackend.Service.Name, "", "(default backend)", backendPort(ing.Spec.DefaultBackend.Service.Port))
	}

	for _, svcName := range order {
		key := ing.Namespace + "/" + svcName
		fronted[key] = true
		svc, ok := svcByKey[key]
		if !ok {
			fi.Services = append(fi.Services, FlowService{
				Name: svcName, Namespace: ing.Namespace, Routes: routes[svcName],
				Pods: []FlowPod{}, Ports: []string{}, Warning: "Service not found",
			})
			continue
		}
		fs := flowService(svc, podLabels.candidates(svc, podsByNs[svc.Namespace]), endpointReadiness, endpoints)
		fs.Routes = routes[svcName]
		fi.Services = append(fi.Services, fs)
	}

	if len(fi.Services) == 0 {
		fi.Warning = "No backend Service — this Ingress routes nowhere"
	}
	return fi
}

func flowService(svc *corev1.Service, nsPods []*corev1.Pod, endpointReadiness *serviceEndpointReadiness, endpoints map[string]bool) FlowService {
	fs := FlowService{
		Name: svc.Name, Namespace: svc.Namespace,
		Type: string(svc.Spec.Type), ClusterIP: svc.Spec.ClusterIP,
		Ports: []string{}, Routes: []string{}, Pods: []FlowPod{},
	}
	for _, p := range svc.Spec.Ports {
		port := fmt.Sprintf("%d", p.Port)
		if target := p.TargetPort.String(); target != "" && target != port {
			port += "→" + target
		}
		if p.Protocol != "" && p.Protocol != corev1.ProtocolTCP {
			port += "/" + string(p.Protocol)
		}
		if p.NodePort > 0 {
			port += fmt.Sprintf(" (node %d)", p.NodePort)
		}
		fs.Ports = append(fs.Ports, port)
	}

	if len(svc.Spec.Selector) == 0 {
		// Headless/ExternalName/manually-managed endpoints: no selector means we
		// genuinely cannot resolve pods, which is different from "matched zero".
		fs.Warning = "No selector — endpoints are managed manually"
		if svc.Spec.Type == corev1.ServiceTypeExternalName {
			fs.Warning = "ExternalName → " + svc.Spec.ExternalName
		}
		return fs
	}

	sel := labels.SelectorFromSet(svc.Spec.Selector)
	for _, pod := range nsPods {
		if !sel.Matches(labels.Set(pod.Labels)) {
			continue
		}
		status, restarts, ready := podStatus(*pod)
		ownerKind, ownerName := podOwner(pod)
		isReady, endpointKnown := endpointReadiness.podReady(svc.Namespace+"/"+svc.Name, pod.Name)
		if !endpointKnown {
			isReady = podIsReady(pod)
		}
		fp := FlowPod{
			Name: pod.Name, Namespace: pod.Namespace, Status: status, Ready: ready,
			Restarts: restarts, Node: pod.Spec.NodeName, IP: pod.Status.PodIP,
			OwnerKind: ownerKind, OwnerName: ownerName,
			IsError: isErroredPodStatus(status),
			IsReady: isReady,
		}
		if fp.IsReady {
			fs.ReadyPods++
		}
		endpoints[pod.Namespace+"/"+pod.Name] = true
		fs.Pods = append(fs.Pods, fp)
	}
	sort.Slice(fs.Pods, func(i, j int) bool { return fs.Pods[i].Name < fs.Pods[j].Name })

	switch {
	case len(fs.Pods) == 0:
		fs.Warning = "No Pods match the selector — traffic here is dropped"
	case fs.ReadyPods == 0:
		fs.Warning = "No Pod is ready — traffic here fails"
	}
	return fs
}

func podOwner(pod *corev1.Pod) (string, string) {
	for _, owner := range pod.OwnerReferences {
		if owner.Controller != nil && *owner.Controller {
			return owner.Kind, owner.Name
		}
	}
	if len(pod.OwnerReferences) > 0 {
		return pod.OwnerReferences[0].Kind, pod.OwnerReferences[0].Name
	}
	return "Pod", pod.Name
}

// isFullyReady reports whether a "n/m" ready string has every container ready.
func isFullyReady(ready string) bool {
	parts := strings.SplitN(ready, "/", 2)
	return len(parts) == 2 && parts[0] == parts[1] && parts[0] != "0"
}

func backendPort(p netv1.ServiceBackendPort) string {
	if p.Name != "" {
		return p.Name
	}
	if p.Number > 0 {
		return fmt.Sprintf("%d", p.Number)
	}
	return ""
}
