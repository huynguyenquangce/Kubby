package k8sclient

import (
	"context"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
)

// Istio splits what an Ingress does into two objects: a Gateway says where
// traffic enters the mesh (which ingress-gateway workload, on which port, with
// which TLS certificate and for which hosts), and a VirtualService says where a
// given host + path is then routed to.
//
// The Traffic view flattens that pair back into the same three lanes the Ingress
// path uses — Route → Service → Pods — so a mesh cluster reads exactly like a
// plain one, with the VirtualService named on the route it created.
//
// Nothing here is Istio-specific at the client level: the objects are read
// through the dynamic client using discovery, so no Istio Go module is needed
// and a cluster without Istio simply contributes no gateways.

const (
	istioGroup       = "networking.istio.io"
	istioGatewayKind = "Gateway." + istioGroup
	istioVSKind      = "VirtualService." + istioGroup
)

// istioContext carries the per-request lookups the gateway hops need, so a
// cluster with ten Gateways sharing one ingress-gateway does the work once.
type istioContext struct {
	ctx      context.Context
	c        *Cluster
	scope    string // the namespace the view is filtered to ("" = all)
	svcByKey map[string]*corev1.Service
	podsByNs map[string][]*corev1.Pod

	workloadCache map[string]*istioWorkload  // selector string → resolved ingress gateway
	secretCache   map[string]bool            // "ns/name" → exists
	outOfScope    map[string]*corev1.Service // "ns/name" → Service fetched from outside the scope
	outOfScopePod map[string][]*corev1.Pod   // namespace → its pods, likewise
}

// istioWorkload is the ingress-gateway a Gateway's selector points at.
type istioWorkload struct {
	Pods      int
	Namespace string
	Address   string // load-balancer address of the Service fronting those pods
}

// istioFlows returns one FlowIngress per Istio Gateway that is reachable in the
// given scope. Returns nil when Istio is not installed.
//
// Gateways are always listed cluster-wide even when the view is scoped to one
// namespace: the near-universal layout puts the Gateway in istio-system and the
// VirtualServices next to the app, so scoping the Gateway list would hide the
// entry point for every namespace but one. A Gateway outside the scope is only
// shown when a VirtualService inside the scope actually binds to it.
func istioFlows(
	ctx context.Context,
	c *Cluster,
	namespace string,
	svcByKey map[string]*corev1.Service,
	podsByNs map[string][]*corev1.Pod,
	fronted map[string]bool,
	endpoints map[string]bool,
) []FlowIngress {
	gwKind, err := c.ResolveKind(istioGatewayKind)
	if err != nil {
		return nil // Istio is not installed on this cluster
	}
	vsKind, err := c.ResolveKind(istioVSKind)
	if err != nil {
		return nil
	}

	gwList, err := c.Dynamic.Resource(gwKind.GVR).Namespace("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil
	}
	vsList, err := c.Dynamic.Resource(vsKind.GVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil
	}

	gateways := map[string]*unstructured.Unstructured{}
	order := []string{}
	for i := range gwList.Items {
		gw := &gwList.Items[i]
		if gw.GetDeletionTimestamp() != nil {
			continue // held by a finalizer — deleted, not routing
		}
		key := gw.GetNamespace() + "/" + gw.GetName()
		gateways[key] = gw
		order = append(order, key)
	}
	if len(gateways) == 0 {
		return nil
	}

	ic := &istioContext{
		ctx: ctx, c: c, scope: namespace, svcByKey: svcByKey, podsByNs: podsByNs,
		workloadCache: map[string]*istioWorkload{},
		secretCache:   map[string]bool{},
		outOfScope:    map[string]*corev1.Service{},
		outOfScopePod: map[string][]*corev1.Pod{},
	}

	// Group every VirtualService under the Gateway(s) it binds to.
	bound := map[string][]*unstructured.Unstructured{}
	for i := range vsList.Items {
		vs := &vsList.Items[i]
		if vs.GetDeletionTimestamp() != nil {
			continue
		}
		for _, key := range vsGatewayKeys(vs) {
			if _, ok := gateways[key]; ok {
				bound[key] = append(bound[key], vs)
			}
		}
	}

	out := []FlowIngress{}
	for _, key := range order {
		gw := gateways[key]
		inScope := namespace == "" || gw.GetNamespace() == namespace
		if len(bound[key]) == 0 && !inScope {
			// An unrelated namespace's Gateway with nothing in scope behind it.
			continue
		}
		out = append(out, ic.flowGateway(gw, bound[key], fronted, endpoints))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Namespace+"/"+out[i].Name < out[j].Namespace+"/"+out[j].Name
	})
	return out
}

func (ic *istioContext) flowGateway(
	gw *unstructured.Unstructured,
	vss []*unstructured.Unstructured,
	fronted map[string]bool,
	endpoints map[string]bool,
) FlowIngress {
	fi := FlowIngress{
		Kind: "Gateway", RefKind: istioGatewayKind,
		Name: gw.GetName(), Namespace: gw.GetNamespace(),
		Hosts: []string{}, Ports: []string{}, Services: []FlowService{},
	}

	selector := nestedStringMap(gw.Object, "spec", "selector")
	fi.Class = labelsToString(selector)

	// --- servers: which ports, hosts and certificates this Gateway listens on
	servers, _, _ := unstructured.NestedSlice(gw.Object, "spec", "servers")
	seenHost, seenPort := map[string]bool{}, map[string]bool{}
	credentials := []string{}
	for _, raw := range servers {
		s, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		for _, h := range nestedStringSlice(s, "hosts") {
			// Istio hosts may be namespace-qualified ("./x", "*/x", "prod/x").
			if i := strings.Index(h, "/"); i >= 0 {
				h = h[i+1:]
			}
			if h != "" && !seenHost[h] {
				seenHost[h] = true
				fi.Hosts = append(fi.Hosts, h)
			}
		}
		port := istioServerPort(s)
		if port != "" && !seenPort[port] {
			seenPort[port] = true
			fi.Ports = append(fi.Ports, port)
		}
		if tls, found, _ := unstructured.NestedMap(s, "tls"); found && len(tls) > 0 {
			fi.TLS = true
			if cred, _, _ := unstructured.NestedString(s, "tls", "credentialName"); cred != "" {
				credentials = append(credentials, cred)
			}
		}
	}

	// --- the ingress-gateway workload this Gateway configures
	warnings := []string{}
	wl := ic.ingressWorkload(selector)
	switch {
	case len(selector) == 0:
		warnings = append(warnings, "No selector — this Gateway configures no ingress workload")
	case wl == nil || wl.Pods == 0:
		warnings = append(warnings, fmt.Sprintf("No Pod matches the selector %s — nothing is listening for this Gateway", fi.Class))
	default:
		fi.Address = wl.Address
	}

	// The TLS secret is read by the ingress gateway, so it must live in *its*
	// namespace — not next to the app. Getting that wrong is the classic Istio
	// HTTPS failure and shows up only as a connection reset at request time.
	if wl != nil && wl.Namespace != "" {
		for _, cred := range credentials {
			if !ic.secretExists(wl.Namespace, cred) {
				warnings = append(warnings, fmt.Sprintf("TLS secret %q not found in namespace %q, where the ingress gateway reads it", cred, wl.Namespace))
			}
		}
	}

	// --- the routes each bound VirtualService contributes
	sort.Slice(vss, func(i, j int) bool {
		return vss[i].GetNamespace()+"/"+vss[i].GetName() < vss[j].GetNamespace()+"/"+vss[j].GetName()
	})
	for _, vs := range vss {
		fi.Services = append(fi.Services, ic.flowVirtualService(vs, fronted, endpoints)...)
	}
	if len(vss) == 0 {
		// Only VirtualServices in the current scope were listed, so a scoped view
		// must not claim a Gateway is unused — it may well be serving another
		// namespace that is simply filtered out.
		if ic.scope == "" {
			warnings = append(warnings, "No VirtualService binds to this Gateway — traffic that arrives here is rejected")
		} else {
			warnings = append(warnings, fmt.Sprintf("No VirtualService in namespace %q binds to this Gateway", ic.scope))
		}
	}

	fi.Warning = strings.Join(warnings, " · ")
	return fi
}

// flowVirtualService turns one VirtualService into a hop per destination
// Service, carrying the host/path rules that reach it.
func (ic *istioContext) flowVirtualService(
	vs *unstructured.Unstructured,
	fronted map[string]bool,
	endpoints map[string]bool,
) []FlowService {
	vsNs := vs.GetNamespace()
	vsHosts := nestedStringSlice(vs.Object, "spec", "hosts")

	order := []string{}             // destination host, in first-seen order
	routes := map[string][]string{} // destination host → route labels
	for _, section := range []string{"http", "tcp", "tls"} {
		rules, _, _ := unstructured.NestedSlice(vs.Object, "spec", section)
		for _, raw := range rules {
			rule, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			hosts, paths := istioMatch(rule, vsHosts)
			dests, _, _ := unstructured.NestedSlice(rule, "route")
			for _, rawDest := range dests {
				dest, ok := rawDest.(map[string]interface{})
				if !ok {
					continue
				}
				host, _, _ := unstructured.NestedString(dest, "destination", "host")
				if host == "" {
					continue
				}
				if _, seen := routes[host]; !seen {
					order = append(order, host)
				}
				routes[host] = append(routes[host], istioRouteLabels(hosts, paths, dest)...)
			}
		}
	}

	via := "VirtualService " + vs.GetName()
	out := make([]FlowService, 0, len(order))
	for _, host := range order {
		fs := ic.destinationService(host, vsNs, endpoints, fronted)
		fs.Routes = routes[host]
		fs.Via = via
		fs.ViaKind = istioVSKind
		fs.ViaName = vs.GetName()
		fs.ViaNamespace = vsNs
		out = append(out, fs)
	}
	return out
}

// destinationService resolves a VirtualService destination host to a Service in
// the cluster. Istio accepts a short name, "name.namespace", or a full
// "name.namespace.svc.cluster.local"; anything that resolves to no Service and
// carries a dot is treated as an external host rather than a broken route.
func (ic *istioContext) destinationService(
	host, vsNamespace string,
	endpoints map[string]bool,
	fronted map[string]bool,
) FlowService {
	name, ns := host, vsNamespace
	trimmed := strings.TrimSuffix(strings.TrimSuffix(host, "."), ".svc.cluster.local")
	if parts := strings.Split(trimmed, "."); len(parts) >= 2 {
		name, ns = parts[0], parts[1]
	} else {
		name = trimmed
	}

	if svc, ok := ic.svcByKey[ns+"/"+name]; ok {
		fronted[ns+"/"+name] = true
		return flowService(svc, ic.podsByNs[svc.Namespace], endpoints)
	}
	// A VirtualService may route across namespaces, so a destination missing
	// from the scoped listing is not necessarily missing from the cluster —
	// look it up directly before calling the hop broken.
	if svc, nsPods, ok := ic.lookupOutOfScope(ns, name); ok {
		return flowService(svc, nsPods, endpoints)
	}
	if strings.Contains(host, ".") && !strings.HasSuffix(host, ".svc.cluster.local") && !strings.HasSuffix(host, ".local") {
		// Looks like a hostname outside the cluster (a ServiceEntry, or an
		// egress target) — reachable, just not something we can expand.
		return FlowService{
			Name: host, Namespace: "", Type: "External",
			Ports: []string{}, Routes: []string{}, Pods: []FlowPod{},
		}
	}
	return FlowService{
		Name: name, Namespace: ns, Type: "",
		Ports: []string{}, Routes: []string{}, Pods: []FlowPod{},
		Warning: "Service not found",
	}
}

// lookupOutOfScope fetches a Service (and its namespace's pods) that the scoped
// listing did not cover. Only reached when the view is filtered to a namespace
// and a route leaves it; results are cached so a fan-out to one namespace costs
// a single pair of calls.
func (ic *istioContext) lookupOutOfScope(namespace, name string) (*corev1.Service, []*corev1.Pod, bool) {
	if ic.scope == "" || namespace == "" || namespace == ic.scope {
		return nil, nil, false // the scoped listing already covered this namespace
	}
	key := namespace + "/" + name
	svc, cached := ic.outOfScope[key]
	if !cached {
		found, err := ic.c.Clientset.CoreV1().Services(namespace).Get(ic.ctx, name, metav1.GetOptions{})
		if err == nil && found.DeletionTimestamp == nil {
			svc = found
		}
		ic.outOfScope[key] = svc
	}
	if svc == nil {
		return nil, nil, false
	}

	pods, cached := ic.outOfScopePod[namespace]
	if !cached {
		if list, err := ic.c.Clientset.CoreV1().Pods(namespace).List(ic.ctx, metav1.ListOptions{}); err == nil {
			for i := range list.Items {
				pods = append(pods, &list.Items[i])
			}
		}
		ic.outOfScopePod[namespace] = pods
	}
	return svc, pods, true
}

// ingressWorkload finds the pods a Gateway selector points at and the address
// of the Service in front of them.
func (ic *istioContext) ingressWorkload(selector map[string]string) *istioWorkload {
	if len(selector) == 0 {
		return nil
	}
	key := labels.SelectorFromSet(selector).String()
	if wl, ok := ic.workloadCache[key]; ok {
		return wl
	}

	wl := &istioWorkload{}
	ic.workloadCache[key] = wl // cache even on failure, so one bad selector is looked up once

	// Cluster-wide: the ingress gateway usually lives outside the viewed scope.
	pods, err := ic.c.Clientset.CoreV1().Pods("").List(ic.ctx, metav1.ListOptions{LabelSelector: key})
	if err != nil || len(pods.Items) == 0 {
		return wl
	}
	wl.Pods = len(pods.Items)
	wl.Namespace = pods.Items[0].Namespace

	// The Gateway selects pods; the address users hit belongs to the Service in
	// front of those pods, so match back from the pod labels.
	svcs, err := ic.c.Clientset.CoreV1().Services(wl.Namespace).List(ic.ctx, metav1.ListOptions{})
	if err != nil {
		return wl
	}
	podLabels := labels.Set(pods.Items[0].Labels)
	for i := range svcs.Items {
		svc := &svcs.Items[i]
		if len(svc.Spec.Selector) == 0 || !labels.SelectorFromSet(svc.Spec.Selector).Matches(podLabels) {
			continue
		}
		for _, lb := range svc.Status.LoadBalancer.Ingress {
			if lb.Hostname != "" {
				wl.Address = lb.Hostname
			} else if lb.IP != "" {
				wl.Address = lb.IP
			}
		}
		if wl.Address == "" {
			wl.Address = svc.Spec.ClusterIP
		}
		break
	}
	return wl
}

func (ic *istioContext) secretExists(namespace, name string) bool {
	key := namespace + "/" + name
	if ok, cached := ic.secretCache[key]; cached {
		return ok
	}
	_, err := ic.c.Clientset.CoreV1().Secrets(namespace).Get(ic.ctx, name, metav1.GetOptions{})
	// Only a definite "not found" counts as missing: without read access to the
	// gateway's namespace we must not accuse a healthy setup of being broken.
	exists := err == nil || !apierrors.IsNotFound(err)
	ic.secretCache[key] = exists
	return exists
}

// vsGatewayKeys resolves a VirtualService's spec.gateways entries to
// "namespace/name" keys. "mesh" (the implicit sidecar gateway) is skipped — it
// is in-cluster traffic, not an entry point from outside.
func vsGatewayKeys(vs *unstructured.Unstructured) []string {
	out := []string{}
	for _, g := range nestedStringSlice(vs.Object, "spec", "gateways") {
		if g == "" || g == "mesh" {
			continue
		}
		if strings.Contains(g, "/") {
			out = append(out, g)
			continue
		}
		out = append(out, vs.GetNamespace()+"/"+g)
	}
	return out
}

// istioMatch extracts the hosts and paths a rule matches on, falling back to the
// VirtualService's own hosts and a catch-all path.
func istioMatch(rule map[string]interface{}, vsHosts []string) (hosts, paths []string) {
	matches, _, _ := unstructured.NestedSlice(rule, "match")
	for _, raw := range matches {
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if v, _, _ := unstructured.NestedString(m, "authority", "exact"); v != "" {
			hosts = append(hosts, v)
		}
		if v, _, _ := unstructured.NestedString(m, "headers", "host", "exact"); v != "" {
			hosts = append(hosts, v)
		}
		for _, sni := range nestedStringSlice(m, "sniHosts") {
			hosts = append(hosts, sni)
		}
		switch {
		case hasKey(m, "uri", "exact"):
			v, _, _ := unstructured.NestedString(m, "uri", "exact")
			paths = append(paths, v)
		case hasKey(m, "uri", "prefix"):
			v, _, _ := unstructured.NestedString(m, "uri", "prefix")
			paths = append(paths, strings.TrimSuffix(v, "/")+"/*")
		case hasKey(m, "uri", "regex"):
			v, _, _ := unstructured.NestedString(m, "uri", "regex")
			paths = append(paths, "~"+v)
		}
	}
	if len(hosts) == 0 {
		hosts = vsHosts
	}
	if len(hosts) == 0 {
		hosts = []string{""}
	}
	if len(paths) == 0 {
		paths = []string{"/*"}
	}
	return dedupe(hosts), dedupe(paths)
}

// istioRouteLabels renders "host/path → :port (weight)" strings in the same
// shape the Ingress path produces, so the UI renders both identically.
func istioRouteLabels(hosts, paths []string, dest map[string]interface{}) []string {
	port := ""
	if n, found, _ := unstructured.NestedInt64(dest, "destination", "port", "number"); found && n > 0 {
		port = fmt.Sprintf("%d", n)
	}
	if subset, _, _ := unstructured.NestedString(dest, "destination", "subset"); subset != "" {
		port = strings.TrimSuffix(port+" "+subset, " ")
	}
	weight := ""
	if w, found, _ := unstructured.NestedInt64(dest, "weight"); found && w > 0 && w < 100 {
		weight = fmt.Sprintf(" (%d%%)", w)
	}

	out := []string{}
	for _, h := range hosts {
		for _, p := range paths {
			label := h + p + weight
			if label == "" {
				label = "/"
			}
			if port != "" {
				label += " → :" + port
			}
			out = append(out, label)
		}
	}
	return out
}

func istioServerPort(server map[string]interface{}) string {
	number, _, _ := unstructured.NestedInt64(server, "port", "number")
	protocol, _, _ := unstructured.NestedString(server, "port", "protocol")
	switch {
	case number == 0 && protocol == "":
		return ""
	case protocol == "":
		return fmt.Sprintf("%d", number)
	case number == 0:
		return protocol
	}
	return fmt.Sprintf("%d/%s", number, protocol)
}

// --- small unstructured helpers -------------------------------------------

func nestedStringSlice(obj map[string]interface{}, fields ...string) []string {
	raw, found, err := unstructured.NestedSlice(obj, fields...)
	if err != nil || !found {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

func nestedStringMap(obj map[string]interface{}, fields ...string) map[string]string {
	raw, found, err := unstructured.NestedMap(obj, fields...)
	if err != nil || !found {
		return nil
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
}

func hasKey(obj map[string]interface{}, fields ...string) bool {
	_, found, err := unstructured.NestedFieldNoCopy(obj, fields...)
	return err == nil && found
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
