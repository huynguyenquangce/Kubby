package k8sclient

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"

	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// "Is traffic from A to B blocked?" is answered here by evaluating the
// NetworkPolicy API objects exactly as the specification defines them. What
// Kubby cannot know from the API is whether the cluster's CNI enforces them at
// all, or whether CNI-specific policy kinds add further rules — every result
// says so rather than presenting the evaluation as a packet trace.

// TrafficPolicyRef names one NetworkPolicy so the UI can open it.
type TrafficPolicyRef struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

// TrafficDirectionVerdict is one side of a connection: egress from the source
// Pod or ingress to a destination Pod. Both must allow the connection.
type TrafficDirectionVerdict struct {
	Isolated  bool               `json:"isolated"` // at least one policy selects the Pod for this direction
	Allowed   bool               `json:"allowed"`
	Selecting []TrafficPolicyRef `json:"selecting"`
	Allowing  []TrafficPolicyRef `json:"allowing"`
	Reason    string             `json:"reason"`
}

// TrafficCheckTarget is the evaluation for one destination Pod. A Service
// check fans out to every Pod its selector matches, because NetworkPolicy
// sees the connection after the Service address has been translated.
type TrafficCheckTarget struct {
	Pod       string                  `json:"pod"`
	Namespace string                  `json:"namespace"`
	IP        string                  `json:"ip"`
	Port      string                  `json:"port"` // "8080/TCP" or "http (8080)/TCP"
	Egress    TrafficDirectionVerdict `json:"egress"`
	Ingress   TrafficDirectionVerdict `json:"ingress"`
	Allowed   bool                    `json:"allowed"`
}

type TrafficCheckResult struct {
	SourceNamespace      string               `json:"sourceNamespace"`
	SourcePod            string               `json:"sourcePod"`
	DestinationKind      string               `json:"destinationKind"`
	DestinationNamespace string               `json:"destinationNamespace"`
	DestinationName      string               `json:"destinationName"`
	Verdict              string               `json:"verdict"` // allowed | blocked | partial | unknown
	Summary              string               `json:"summary"`
	Targets              []TrafficCheckTarget `json:"targets"`
	Limitations          []string             `json:"limitations"`
}

// destinationPort is the port as a NetworkPolicy rule sees it: the number on
// the destination Pod plus that container port's name, so a rule written with
// a named port matches a numeric request and vice versa.
type destinationPort struct {
	Number   int32
	Name     string
	Protocol corev1.Protocol
}

func (port destinationPort) String() string {
	if port.Name != "" {
		return fmt.Sprintf("%s (%d)/%s", port.Name, port.Number, port.Protocol)
	}
	return fmt.Sprintf("%d/%s", port.Number, port.Protocol)
}

const (
	TrafficAllowed = "allowed"
	TrafficBlocked = "blocked"
	TrafficPartial = "partial"
	TrafficUnknown = "unknown"
)

var trafficBaseLimitations = []string{
	"Kubby evaluates NetworkPolicy objects; whether they are enforced depends on the cluster's CNI plugin.",
	"CNI-specific policies (Cilium, Calico), AdminNetworkPolicy and service-mesh authorization are not evaluated.",
}

// ---- Compiled policies ----
//
// Every selector and CIDR in a policy is parsed once per call. Parsing validates
// each label key and value, and doing it per Pod × policy dominated both the
// Traffic overlay and CheckTrafficPolicy: 10k endpoint rows × 50 policies spent
// 70% of CPU and 93% of allocated bytes in metav1.LabelSelectorAsSelector.

type compiledNetworkPolicy struct {
	ref             TrafficPolicyRef
	podSelector     labels.Selector
	ingress, egress bool
	ingressRules    []compiledPolicyRule
	egressRules     []compiledPolicyRule
}

type compiledPolicyRule struct {
	anyPeer bool // no from/to entries: every peer matches
	peers   []compiledPolicyPeer
	ports   []netv1.NetworkPolicyPort
}

// compiledPolicyPeer keeps the API's peer shapes apart: an ipBlock, a
// namespaceSelector with an optional podSelector, or a podSelector alone
// (meaning the policy's own namespace). A peer with none set matches nothing.
type compiledPolicyPeer struct {
	ipBlock           *compiledIPBlock
	namespaceSelector labels.Selector
	podSelector       labels.Selector
}

type compiledIPBlock struct {
	cidr   *net.IPNet
	except []*net.IPNet
}

func compileNetworkPolicy(policy *netv1.NetworkPolicy) *compiledNetworkPolicy {
	compiled := &compiledNetworkPolicy{
		ref:         TrafficPolicyRef{Namespace: policy.Namespace, Name: policy.Name},
		podSelector: parseLabelSelector(&policy.Spec.PodSelector),
	}
	compiled.ingress, compiled.egress = networkPolicyTypes(policy)
	for _, rule := range policy.Spec.Ingress {
		compiled.ingressRules = append(compiled.ingressRules, compilePolicyRule(rule.From, rule.Ports))
	}
	for _, rule := range policy.Spec.Egress {
		compiled.egressRules = append(compiled.egressRules, compilePolicyRule(rule.To, rule.Ports))
	}
	return compiled
}

// compileNetworkPolicies buckets live policies by namespace — a policy selects
// Pods only in its own namespace — and reports how many were live.
func compileNetworkPolicies(policies []netv1.NetworkPolicy) (map[string][]*compiledNetworkPolicy, int) {
	byNamespace := map[string][]*compiledNetworkPolicy{}
	live := 0
	for i := range policies {
		if policies[i].DeletionTimestamp != nil {
			continue
		}
		live++
		byNamespace[policies[i].Namespace] = append(byNamespace[policies[i].Namespace], compileNetworkPolicy(&policies[i]))
	}
	return byNamespace, live
}

func compilePolicyRule(peers []netv1.NetworkPolicyPeer, ports []netv1.NetworkPolicyPort) compiledPolicyRule {
	rule := compiledPolicyRule{anyPeer: len(peers) == 0, ports: ports}
	for _, peer := range peers {
		compiled := compiledPolicyPeer{}
		switch {
		case peer.IPBlock != nil:
			compiled.ipBlock = compileIPBlock(peer.IPBlock)
		case peer.NamespaceSelector != nil:
			compiled.namespaceSelector = parseLabelSelector(peer.NamespaceSelector)
			if peer.PodSelector != nil {
				compiled.podSelector = parseLabelSelector(peer.PodSelector)
			}
		case peer.PodSelector != nil:
			compiled.podSelector = parseLabelSelector(peer.PodSelector)
		}
		rule.peers = append(rule.peers, compiled)
	}
	return rule
}

func compileIPBlock(block *netv1.IPBlock) *compiledIPBlock {
	compiled := &compiledIPBlock{}
	if _, cidr, err := net.ParseCIDR(block.CIDR); err == nil {
		compiled.cidr = cidr
	}
	for _, except := range block.Except {
		if _, excluded, err := net.ParseCIDR(except); err == nil {
			compiled.except = append(compiled.except, excluded)
		}
	}
	return compiled
}

// parseLabelSelector maps an invalid selector to one that matches nothing: the
// API server would not have admitted it, and it must never widen a policy.
func parseLabelSelector(selector *metav1.LabelSelector) labels.Selector {
	parsed, err := metav1.LabelSelectorAsSelector(selector)
	if err != nil {
		return labels.Nothing()
	}
	return parsed
}

func (policy *compiledNetworkPolicy) selects(pod *corev1.Pod) bool {
	return policy.ref.Namespace == pod.Namespace && policy.podSelector.Matches(labels.Set(pod.Labels))
}

func (policy *compiledNetworkPolicy) isolates(direction netv1.PolicyType) bool {
	if direction == netv1.PolicyTypeEgress {
		return policy.egress
	}
	return policy.ingress
}

func (policy *compiledNetworkPolicy) allows(direction netv1.PolicyType, peer *corev1.Pod, port destinationPort, labelsFor func(string) (map[string]string, bool)) bool {
	rules := policy.ingressRules
	if direction == netv1.PolicyTypeEgress {
		rules = policy.egressRules
	}
	for _, rule := range rules {
		if rule.matchesPeer(policy.ref.Namespace, peer, labelsFor) && policyPortsMatch(rule.ports, port) {
			return true
		}
	}
	return false
}

func (rule compiledPolicyRule) matchesPeer(policyNamespace string, pod *corev1.Pod, labelsFor func(string) (map[string]string, bool)) bool {
	if rule.anyPeer {
		return true
	}
	podLabels := labels.Set(pod.Labels)
	for _, peer := range rule.peers {
		switch {
		case peer.ipBlock != nil:
			if peer.ipBlock.contains(pod.Status.PodIP) {
				return true
			}
		case peer.namespaceSelector != nil:
			namespaceSet, ok := labelsFor(pod.Namespace)
			if !ok || !peer.namespaceSelector.Matches(labels.Set(namespaceSet)) {
				continue
			}
			if peer.podSelector == nil || peer.podSelector.Matches(podLabels) {
				return true
			}
		case peer.podSelector != nil:
			if pod.Namespace == policyNamespace && peer.podSelector.Matches(podLabels) {
				return true
			}
		}
	}
	return false
}

func (block *compiledIPBlock) contains(address string) bool {
	if block.cidr == nil {
		return false
	}
	ip := net.ParseIP(address)
	if ip == nil || !block.cidr.Contains(ip) {
		return false
	}
	for _, excluded := range block.except {
		if excluded.Contains(ip) {
			return false
		}
	}
	return true
}

// selectingPolicies returns the policies that isolate pod for one direction.
func selectingPolicies(policies []*compiledNetworkPolicy, pod *corev1.Pod, direction netv1.PolicyType) []*compiledNetworkPolicy {
	selecting := []*compiledNetworkPolicy{}
	for _, policy := range policies {
		if policy.isolates(direction) && policy.selects(pod) {
			selecting = append(selecting, policy)
		}
	}
	return selecting
}

// ---- Traffic check ----

// CheckTrafficPolicy evaluates whether NetworkPolicy permits a new connection
// from a source Pod to a destination Pod or Service on a port. port 0 is
// accepted when the destination declares exactly one port.
func CheckTrafficPolicy(ctx context.Context, c *Cluster, sourceNamespace, sourcePod, destinationKind, destinationNamespace, destinationName string, port int, protocol string) (*TrafficCheckResult, error) {
	proto, err := parseTrafficProtocol(protocol)
	if err != nil {
		return nil, err
	}
	if port < 0 || port > 65535 {
		return nil, fmt.Errorf("port %d is outside 0–65535", port)
	}
	if strings.TrimSpace(sourcePod) == "" || strings.TrimSpace(destinationName) == "" {
		return nil, fmt.Errorf("both a source Pod and a destination are required")
	}
	if sourceNamespace == "" {
		sourceNamespace = "default"
	}
	if destinationNamespace == "" {
		destinationNamespace = sourceNamespace
	}
	result := &TrafficCheckResult{
		SourceNamespace: sourceNamespace, SourcePod: sourcePod,
		DestinationKind: destinationKind, DestinationNamespace: destinationNamespace, DestinationName: destinationName,
		Targets: []TrafficCheckTarget{}, Limitations: append([]string(nil), trafficBaseLimitations...),
	}

	source, err := c.Clientset.CoreV1().Pods(sourceNamespace).Get(ctx, sourcePod, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("source Pod: %w", err)
	}
	type destination struct {
		pod  *corev1.Pod
		port destinationPort
	}
	destinations := []destination{}
	switch bareKind(destinationKind) {
	case "Pod":
		pod, err := c.Clientset.CoreV1().Pods(destinationNamespace).Get(ctx, destinationName, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("destination Pod: %w", err)
		}
		resolved, err := podDestinationPort(pod, int32(port), proto)
		if err != nil {
			return nil, err
		}
		destinations = append(destinations, destination{pod, resolved})
	case "Service":
		svc, err := c.Clientset.CoreV1().Services(destinationNamespace).Get(ctx, destinationName, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("destination Service: %w", err)
		}
		servicePort, err := selectServicePort(svc, int32(port), proto, protocol != "")
		if err != nil {
			return nil, err
		}
		if len(svc.Spec.Selector) == 0 {
			return nil, fmt.Errorf("Service %s has no selector; its endpoints are managed manually, so Kubby cannot tell which Pods receive the traffic", svc.Name)
		}
		pods, err := c.Clientset.CoreV1().Pods(destinationNamespace).List(ctx, metav1.ListOptions{
			LabelSelector: labels.SelectorFromSet(svc.Spec.Selector).String(),
		})
		if err != nil {
			return nil, fmt.Errorf("Pods behind Service %s: %w", svc.Name, err)
		}
		for i := range pods.Items {
			pod := &pods.Items[i]
			if pod.DeletionTimestamp != nil {
				continue
			}
			destinations = append(destinations, destination{pod, serviceTargetPort(pod, servicePort)})
		}
		if len(destinations) == 0 {
			result.Verdict = TrafficUnknown
			result.Summary = fmt.Sprintf("No Pod matches Service %s's selector, so the connection has nowhere to go regardless of policy.", svc.Name)
			return result, nil
		}
	default:
		return nil, fmt.Errorf("destination kind must be Pod or Service, not %q", destinationKind)
	}

	sourcePolicies, err := loadCompiledPolicies(ctx, c, sourceNamespace)
	if err != nil {
		result.Verdict = TrafficUnknown
		result.Summary = fmt.Sprintf("NetworkPolicies in %s could not be read: %v", sourceNamespace, err)
		return result, nil
	}
	destinationPolicies := sourcePolicies
	if destinationNamespace != sourceNamespace {
		destinationPolicies, err = loadCompiledPolicies(ctx, c, destinationNamespace)
		if err != nil {
			result.Verdict = TrafficUnknown
			result.Summary = fmt.Sprintf("NetworkPolicies in %s could not be read: %v", destinationNamespace, err)
			return result, nil
		}
	}
	labelsFor := newNamespaceLabelLookup(ctx, c, &result.Limitations)
	if source.Spec.HostNetwork {
		result.Limitations = append(result.Limitations, fmt.Sprintf("%s uses the host network; its traffic leaves with the node's address and CNIs differ in how policy applies to it.", source.Name))
	}

	// Which policies isolate the source for egress does not depend on the
	// destination, so it is resolved once rather than per destination Pod.
	sourceEgress := selectingPolicies(sourcePolicies, source, netv1.PolicyTypeEgress)
	allowed := 0
	for _, dst := range destinations {
		target := TrafficCheckTarget{
			Pod: dst.pod.Name, Namespace: dst.pod.Namespace, IP: dst.pod.Status.PodIP, Port: dst.port.String(),
		}
		target.Egress = directionVerdict(netv1.PolicyTypeEgress, sourceEgress, source, dst.pod, dst.port, labelsFor)
		target.Ingress = directionVerdict(netv1.PolicyTypeIngress,
			selectingPolicies(destinationPolicies, dst.pod, netv1.PolicyTypeIngress), dst.pod, source, dst.port, labelsFor)
		target.Allowed = target.Egress.Allowed && target.Ingress.Allowed
		if target.Allowed {
			allowed++
		}
		if dst.pod.Spec.HostNetwork {
			result.Limitations = append(result.Limitations, fmt.Sprintf("%s uses the host network; NetworkPolicy normally does not isolate host-network Pods.", dst.pod.Name))
		}
		result.Targets = append(result.Targets, target)
	}
	sort.Slice(result.Targets, func(i, j int) bool { return result.Targets[i].Pod < result.Targets[j].Pod })

	destinationLabel := fmt.Sprintf("%s %s/%s", bareKind(destinationKind), destinationNamespace, destinationName)
	switch {
	case allowed == len(result.Targets):
		result.Verdict = TrafficAllowed
		result.Summary = fmt.Sprintf("NetworkPolicy allows %s/%s to reach %s.", sourceNamespace, sourcePod, destinationLabel)
	case allowed == 0:
		result.Verdict = TrafficBlocked
		result.Summary = fmt.Sprintf("NetworkPolicy blocks %s/%s from reaching %s.", sourceNamespace, sourcePod, destinationLabel)
	default:
		result.Verdict = TrafficPartial
		result.Summary = fmt.Sprintf("NetworkPolicy allows %d of %d Pods behind %s; the Service may intermittently fail for %s/%s.", allowed, len(result.Targets), destinationLabel, sourceNamespace, sourcePod)
	}
	return result, nil
}

// loadCompiledPolicies lists and compiles one namespace's live policies.
func loadCompiledPolicies(ctx context.Context, c *Cluster, namespace string) ([]*compiledNetworkPolicy, error) {
	list, err := c.Clientset.NetworkingV1().NetworkPolicies(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	byNamespace, _ := compileNetworkPolicies(list.Items)
	return byNamespace[namespace], nil
}

// newNamespaceLabelLookup reads namespace labels at most once per namespace for
// namespaceSelector peers, recording an unreadable namespace as a limitation.
// It is safe for concurrent use.
func newNamespaceLabelLookup(ctx context.Context, c *Cluster, limitations *[]string) func(string) (map[string]string, bool) {
	var mu sync.Mutex
	known := map[string]map[string]string{}
	unreadable := map[string]bool{}
	return func(namespace string) (map[string]string, bool) {
		mu.Lock()
		defer mu.Unlock()
		if set, ok := known[namespace]; ok {
			return set, true
		}
		if unreadable[namespace] {
			return nil, false
		}
		ns, err := c.Clientset.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
		if err != nil {
			unreadable[namespace] = true
			*limitations = append(*limitations, fmt.Sprintf("Namespace %s labels could not be read (%v); namespaceSelector rules were treated as not matching it.", namespace, err))
			return nil, false
		}
		set := map[string]string{}
		for key, value := range ns.Labels {
			set[key] = value
		}
		// The API server sets this label on every namespace since 1.21; keep the
		// evaluation correct for a namespace object read from an older server.
		if _, ok := set[corev1.LabelMetadataName]; !ok {
			set[corev1.LabelMetadataName] = namespace
		}
		known[namespace] = set
		return set, true
	}
}

func parseTrafficProtocol(protocol string) (corev1.Protocol, error) {
	switch strings.ToUpper(strings.TrimSpace(protocol)) {
	case "", "TCP":
		return corev1.ProtocolTCP, nil
	case "UDP":
		return corev1.ProtocolUDP, nil
	case "SCTP":
		return corev1.ProtocolSCTP, nil
	}
	return "", fmt.Errorf("protocol must be TCP, UDP or SCTP, not %q", protocol)
}

func podDestinationPort(pod *corev1.Pod, port int32, protocol corev1.Protocol) (destinationPort, error) {
	if port == 0 {
		declared := []corev1.ContainerPort{}
		for _, container := range pod.Spec.Containers {
			declared = append(declared, container.Ports...)
		}
		if len(declared) != 1 {
			return destinationPort{}, fmt.Errorf("Pod %s declares %d container ports; enter the port to check", pod.Name, len(declared))
		}
		port = declared[0].ContainerPort
		if declared[0].Protocol != "" {
			protocol = declared[0].Protocol
		}
	}
	return destinationPort{Number: port, Name: containerPortName(pod, port, protocol), Protocol: protocol}, nil
}

func containerPortName(pod *corev1.Pod, number int32, protocol corev1.Protocol) string {
	for _, container := range pod.Spec.Containers {
		for _, declared := range container.Ports {
			declaredProtocol := declared.Protocol
			if declaredProtocol == "" {
				declaredProtocol = corev1.ProtocolTCP
			}
			if declared.ContainerPort == number && declaredProtocol == protocol {
				return declared.Name
			}
		}
	}
	return ""
}

func containerPortByName(pod *corev1.Pod, name string, protocol corev1.Protocol) (int32, bool) {
	for _, container := range pod.Spec.Containers {
		for _, declared := range container.Ports {
			declaredProtocol := declared.Protocol
			if declaredProtocol == "" {
				declaredProtocol = corev1.ProtocolTCP
			}
			if declared.Name == name && declaredProtocol == protocol {
				return declared.ContainerPort, true
			}
		}
	}
	return 0, false
}

// selectServicePort accepts the Service port the caller would dial. The
// protocol only narrows the match when the caller chose one explicitly.
func selectServicePort(svc *corev1.Service, port int32, protocol corev1.Protocol, explicitProtocol bool) (corev1.ServicePort, error) {
	if port == 0 {
		if len(svc.Spec.Ports) == 1 {
			return svc.Spec.Ports[0], nil
		}
		return corev1.ServicePort{}, fmt.Errorf("Service %s exposes %d ports; enter the port to check", svc.Name, len(svc.Spec.Ports))
	}
	available := []string{}
	for _, candidate := range svc.Spec.Ports {
		candidateProtocol := candidate.Protocol
		if candidateProtocol == "" {
			candidateProtocol = corev1.ProtocolTCP
		}
		available = append(available, fmt.Sprintf("%d/%s", candidate.Port, candidateProtocol))
		if candidate.Port == port && (!explicitProtocol || candidateProtocol == protocol) {
			return candidate, nil
		}
	}
	return corev1.ServicePort{}, fmt.Errorf("Service %s has no port %d/%s (it exposes %s)", svc.Name, port, protocol, strings.Join(available, ", "))
}

// serviceTargetPort maps a Service port to the port on one backing Pod. A named
// targetPort can resolve to a different number on each Pod.
func serviceTargetPort(pod *corev1.Pod, servicePort corev1.ServicePort) destinationPort {
	protocol := servicePort.Protocol
	if protocol == "" {
		protocol = corev1.ProtocolTCP
	}
	switch {
	case servicePort.TargetPort.Type == intstr.String && servicePort.TargetPort.StrVal != "":
		number, _ := containerPortByName(pod, servicePort.TargetPort.StrVal, protocol)
		return destinationPort{Number: number, Name: servicePort.TargetPort.StrVal, Protocol: protocol}
	case servicePort.TargetPort.IntVal != 0:
		number := servicePort.TargetPort.IntVal
		return destinationPort{Number: number, Name: containerPortName(pod, number, protocol), Protocol: protocol}
	}
	return destinationPort{Number: servicePort.Port, Name: containerPortName(pod, servicePort.Port, protocol), Protocol: protocol}
}

// directionVerdict applies the NetworkPolicy semantics for one side, given the
// policies that isolate subject for that direction: a Pod no policy selects is
// non-isolated and allows everything; an isolated Pod allows only what the union
// of its policies' rules permits.
func directionVerdict(
	direction netv1.PolicyType,
	selecting []*compiledNetworkPolicy,
	subject, peer *corev1.Pod,
	port destinationPort,
	labelsFor func(string) (map[string]string, bool),
) TrafficDirectionVerdict {
	verdict := TrafficDirectionVerdict{Selecting: make([]TrafficPolicyRef, 0, len(selecting)), Allowing: []TrafficPolicyRef{}}
	for _, policy := range selecting {
		verdict.Selecting = append(verdict.Selecting, policy.ref)
		if policy.allows(direction, peer, port, labelsFor) {
			verdict.Allowing = append(verdict.Allowing, policy.ref)
		}
	}
	verdict.Isolated = len(verdict.Selecting) > 0
	verdict.Allowed = !verdict.Isolated || len(verdict.Allowing) > 0

	word, peerRole := "ingress", "from"
	if direction == netv1.PolicyTypeEgress {
		word, peerRole = "egress", "to"
	}
	switch {
	case !verdict.Isolated:
		verdict.Reason = fmt.Sprintf("No NetworkPolicy selects %s for %s, so it is not isolated.", subject.Name, word)
	case verdict.Allowed:
		verdict.Reason = fmt.Sprintf("Allowed by %s.", policyRefNames(verdict.Allowing))
	default:
		verdict.Reason = fmt.Sprintf("%s is isolated for %s by %s, and no rule permits %s %s/%s on %s.",
			subject.Name, word, policyRefNames(verdict.Selecting), peerRole, peer.Namespace, peer.Name, port)
	}
	return verdict
}

func policyPortsMatch(ports []netv1.NetworkPolicyPort, port destinationPort) bool {
	if len(ports) == 0 {
		return true
	}
	for _, candidate := range ports {
		protocol := corev1.ProtocolTCP
		if candidate.Protocol != nil {
			protocol = *candidate.Protocol
		}
		if protocol != port.Protocol {
			continue
		}
		if candidate.Port == nil {
			return true
		}
		if candidate.Port.Type == intstr.String {
			if port.Name != "" && candidate.Port.StrVal == port.Name {
				return true
			}
			continue
		}
		start, end := candidate.Port.IntVal, candidate.Port.IntVal
		if candidate.EndPort != nil {
			end = *candidate.EndPort
		}
		if port.Number != 0 && port.Number >= start && port.Number <= end {
			return true
		}
	}
	return false
}

func policyRefNames(refs []TrafficPolicyRef) string {
	names := make([]string, 0, len(refs))
	for _, ref := range refs {
		names = append(names, ref.Name)
	}
	return strings.Join(names, ", ")
}

// ---- Traffic view overlay ----

// annotateFlowPolicies overlays NetworkPolicy isolation onto the Traffic view.
// The result is keyed by Pod rather than repeated on every endpoint row: the
// same Pod appears under every Service that selects it, and per-row copies
// added 47% to the topology join's allocations and to the bridge payload.
// Whether a particular connection is allowed needs a source, which
// CheckTrafficPolicy takes.
func annotateFlowPolicies(flows *NetworkFlows, pods []corev1.Pod, policies []netv1.NetworkPolicy) {
	podByKey := make(map[string]*corev1.Pod, len(pods))
	for i := range pods {
		podByKey[pods[i].Namespace+"/"+pods[i].Name] = &pods[i]
	}
	byNamespace, live := compileNetworkPolicies(policies)
	flows.PoliciesAvailable = true
	flows.PolicyCount = live
	flows.IsolatedPods = 0
	flows.PodPolicies = map[string]FlowPodPolicies{}
	seen := map[string]bool{}
	visit := func(services []FlowService) {
		for si := range services {
			for pi := range services[si].Pods {
				key := services[si].Pods[pi].Namespace + "/" + services[si].Pods[pi].Name
				if seen[key] {
					continue
				}
				seen[key] = true
				pod := podByKey[key]
				if pod == nil || len(byNamespace[pod.Namespace]) == 0 {
					continue
				}
				entry := FlowPodPolicies{Ingress: []string{}, Egress: []string{}}
				podLabels := labels.Set(pod.Labels)
				for _, policy := range byNamespace[pod.Namespace] {
					if !policy.podSelector.Matches(podLabels) {
						continue
					}
					if policy.ingress {
						entry.Ingress = append(entry.Ingress, policy.ref.Name)
					}
					if policy.egress {
						entry.Egress = append(entry.Egress, policy.ref.Name)
					}
				}
				if len(entry.Ingress) == 0 && len(entry.Egress) == 0 {
					continue
				}
				flows.PodPolicies[key] = entry
				if len(entry.Ingress) > 0 {
					flows.IsolatedPods++
				}
			}
		}
	}
	for i := range flows.Ingresses {
		visit(flows.Ingresses[i].Services)
	}
	visit(flows.Services)
}
