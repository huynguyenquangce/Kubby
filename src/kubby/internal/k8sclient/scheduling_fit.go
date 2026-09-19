package k8sclient

import (
	"fmt"
	"sort"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// The predicates. Each one mirrors a kube-scheduler filter plugin closely
// enough to name the same cause in the same words, and stops there: nothing
// here scores or ranks, because feasibility is what the operator is asking
// about. Anything that cannot be decided from API objects is left out and
// stated in the report's limits rather than guessed.

// Taints Kubernetes manages itself. They are reported through the node's own
// status (Ready, cordoned) instead of as an untolerated taint, so one condition
// produces one reason rather than two.
var schedulingStatusTaints = map[string]bool{
	corev1.TaintNodeNotReady:      true,
	corev1.TaintNodeUnschedulable: true,
}

// nodeUsage is what the Pods already on a node hold.
type nodeUsage struct {
	amounts   map[corev1.ResourceName]int64
	pods      int
	hostPorts map[string]map[string]bool // "TCP/8080" → host IPs in use
}

func nodeUsageIndex(pods []corev1.Pod) map[string]*nodeUsage {
	index := make(map[string]*nodeUsage, 8)
	for i := range pods {
		pod := &pods[i]
		usage := index[pod.Spec.NodeName]
		if usage == nil {
			usage = &nodeUsage{amounts: map[corev1.ResourceName]int64{}, hostPorts: map[string]map[string]bool{}}
			index[pod.Spec.NodeName] = usage
		}
		usage.pods++
		accumulatePodRequests(usage.amounts, pod)
		for _, port := range podHostPorts(pod) {
			ips := usage.hostPorts[port.key]
			if ips == nil {
				ips = map[string]bool{}
				usage.hostPorts[port.key] = ips
			}
			ips[port.ip] = true
		}
	}
	return index
}

type hostPortUse struct{ key, ip string }

func podHostPorts(pod *corev1.Pod) []hostPortUse {
	out := []hostPortUse{}
	collect := func(containers []corev1.Container) {
		for i := range containers {
			for _, port := range containers[i].Ports {
				if port.HostPort == 0 {
					continue
				}
				protocol := port.Protocol
				if protocol == "" {
					protocol = corev1.ProtocolTCP
				}
				ip := port.HostIP
				if ip == "" {
					ip = "0.0.0.0"
				}
				out = append(out, hostPortUse{key: fmt.Sprintf("%s/%d", protocol, port.HostPort), ip: ip})
			}
		}
	}
	collect(pod.Spec.InitContainers)
	collect(pod.Spec.Containers)
	return out
}

// effectivePodRequests is what the scheduler reserves for a Pod: the regular
// containers plus restartable init containers (sidecars) run together, while a
// plain init container runs alone before them, so the Pod needs whichever is
// larger — plus any declared pod overhead.
func effectivePodRequests(pod *corev1.Pod) corev1.ResourceList {
	total := corev1.ResourceList{}
	sidecars := corev1.ResourceList{}
	for i := range pod.Spec.Containers {
		addResources(total, pod.Spec.Containers[i].Resources.Requests)
	}
	for i := range pod.Spec.InitContainers {
		init := &pod.Spec.InitContainers[i]
		if init.RestartPolicy != nil && *init.RestartPolicy == corev1.ContainerRestartPolicyAlways {
			addResources(total, init.Resources.Requests)
			addResources(sidecars, init.Resources.Requests)
		}
	}
	for i := range pod.Spec.InitContainers {
		init := &pod.Spec.InitContainers[i]
		if init.RestartPolicy != nil && *init.RestartPolicy == corev1.ContainerRestartPolicyAlways {
			continue
		}
		phase := corev1.ResourceList{}
		addResources(phase, init.Resources.Requests)
		addResources(phase, sidecars)
		maxResources(total, phase)
	}
	addResources(total, pod.Spec.Overhead)
	return total
}

func addResources(into corev1.ResourceList, from corev1.ResourceList) {
	for name, quantity := range from {
		current, ok := into[name]
		if !ok {
			into[name] = quantity.DeepCopy()
			continue
		}
		current.Add(quantity)
		into[name] = current
	}
}

func maxResources(into corev1.ResourceList, from corev1.ResourceList) {
	for name, quantity := range from {
		current, ok := into[name]
		if !ok || quantity.Cmp(current) > 0 {
			into[name] = quantity.DeepCopy()
		}
	}
}

// accumulatePodRequests adds one Pod's effective requests to a running total.
// It runs once per Pod in the cluster, so the ordinary case — containers only —
// skips building the intermediate ResourceLists that effectivePodRequests needs
// for init containers and overhead.
func accumulatePodRequests(into map[corev1.ResourceName]int64, pod *corev1.Pod) {
	if len(pod.Spec.InitContainers) > 0 || len(pod.Spec.Overhead) > 0 {
		for name, amount := range resourceAmounts(effectivePodRequests(pod)) {
			into[name] += amount
		}
		return
	}
	for i := range pod.Spec.Containers {
		for name, quantity := range pod.Spec.Containers[i].Resources.Requests {
			if name == corev1.ResourceCPU {
				into[name] += quantity.MilliValue()
				continue
			}
			into[name] += quantity.Value()
		}
	}
}

// resourceAmounts flattens a ResourceList to integers the same way the
// scheduler compares them: CPU in millicores, everything else in its own unit.
func resourceAmounts(list corev1.ResourceList) map[corev1.ResourceName]int64 {
	out := make(map[corev1.ResourceName]int64, len(list))
	for name, quantity := range list {
		if name == corev1.ResourceCPU {
			out[name] = quantity.MilliValue()
			continue
		}
		out[name] = quantity.Value()
	}
	return out
}

// resourceRequest is one requested resource, kept in a sorted slice rather than
// a map so a node's reasons come out in the same order every time.
type resourceRequest struct {
	name   corev1.ResourceName
	amount int64
}

func sortedResourceRequests(list corev1.ResourceList) []resourceRequest {
	amounts := resourceAmounts(list)
	out := make([]resourceRequest, 0, len(amounts))
	for name, amount := range amounts {
		out = append(out, resourceRequest{name: name, amount: amount})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

func requestList(list corev1.ResourceList) []SchedulingRequest {
	amounts := resourceAmounts(list)
	out := make([]SchedulingRequest, 0, len(amounts))
	for name, amount := range amounts {
		out = append(out, SchedulingRequest{Resource: string(name), Request: formatResource(name, amount)})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Resource < out[j].Resource })
	return out
}

// formatResource prints an amount in the unit its reader expects.
func formatResource(name corev1.ResourceName, amount int64) string {
	switch name {
	case corev1.ResourceCPU:
		return FormatCPU(amount)
	case corev1.ResourceMemory, corev1.ResourceEphemeralStorage, corev1.ResourceStorage:
		return FormatMem(amount)
	}
	return strconv.FormatInt(amount, 10)
}

func nodeByName(nodes []corev1.Node, name string) *corev1.Node {
	for i := range nodes {
		if nodes[i].Name == name {
			return &nodes[i]
		}
	}
	return nil
}

func nodeReady(node *corev1.Node) bool {
	for i := range node.Status.Conditions {
		condition := &node.Status.Conditions[i]
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

// evaluateNodeFit collects every reason this node would reject the Pod. The
// scheduler stops at the first failing plugin; an operator wants all of them,
// because fixing one only to meet the next is the frustrating part.
func evaluateNodeFit(node *corev1.Node, pod *corev1.Pod, requests []resourceRequest,
	usage *nodeUsage, affinity []compiledAffinityTerm, spread []compiledSpread, volumes *volumeConstraints) NodeFit {
	if usage == nil {
		usage = &nodeUsage{amounts: map[corev1.ResourceName]int64{}, hostPorts: map[string]map[string]bool{}}
	}
	allocatable := resourceAmounts(node.Status.Allocatable)
	fit := NodeFit{
		Name: node.Name, Ready: nodeReady(node), Schedulable: !node.Spec.Unschedulable,
		CPUFree: FormatCPU(allocatable[corev1.ResourceCPU] - usage.amounts[corev1.ResourceCPU]),
		MemFree: FormatMem(allocatable[corev1.ResourceMemory] - usage.amounts[corev1.ResourceMemory]),
		Pods:    fmt.Sprintf("%d / %d", usage.pods, allocatable[corev1.ResourcePods]),
		Reasons: []NodeFitReason{},
	}
	add := func(code, text, detail string) {
		fit.Reasons = append(fit.Reasons, NodeFitReason{Code: code, Text: text, Detail: detail})
	}

	if !fit.Schedulable {
		add("cordoned", "Cordoned (unschedulable)", "kubectl uncordon "+node.Name+" makes it available again.")
	}
	if !fit.Ready {
		add("not-ready", "Node is not Ready", "Its kubelet is not reporting healthy, so the scheduler will not place work on it.")
	}
	for _, taint := range node.Spec.Taints {
		if taint.Effect != corev1.TaintEffectNoSchedule && taint.Effect != corev1.TaintEffectNoExecute {
			continue // PreferNoSchedule changes ranking, not feasibility
		}
		if schedulingStatusTaints[taint.Key] {
			continue // already reported as Ready / cordoned
		}
		if toleratesTaint(pod.Spec.Tolerations, taint) {
			continue
		}
		add("taint:"+taint.Key, "Untolerated taint "+taintText(taint),
			"Add a toleration for this taint, or choose a node without it.")
	}
	if !nodeMatchesPodSelectors(node, pod) {
		add("node-affinity", "Labels do not match nodeSelector or node affinity",
			"This node's labels do not satisfy the Pod's required node selection.")
	}
	for _, request := range requests {
		if request.name == corev1.ResourcePods {
			continue
		}
		free := allocatable[request.name] - usage.amounts[request.name]
		if request.amount <= free {
			continue
		}
		add("insufficient:"+string(request.name), "Insufficient "+string(request.name),
			fmt.Sprintf("needs %s, %s free of %s", formatResource(request.name, request.amount),
				formatResource(request.name, free), formatResource(request.name, allocatable[request.name])))
	}
	if capacity := allocatable[corev1.ResourcePods]; capacity > 0 && int64(usage.pods) >= capacity {
		add("pod-capacity", "No room for another Pod",
			fmt.Sprintf("%d of %d Pod slots are taken.", usage.pods, capacity))
	}
	for _, port := range podHostPorts(pod) {
		ips := usage.hostPorts[port.key]
		if len(ips) == 0 {
			continue
		}
		if port.ip == "0.0.0.0" || ips["0.0.0.0"] || ips[port.ip] {
			add("host-port", "Host port "+port.key+" is already in use",
				"Another Pod on this node publishes the same host port.")
			break
		}
	}
	for _, term := range affinity {
		if reason, ok := term.rejects(node); ok {
			add(reason.Code, reason.Text, reason.Detail)
		}
	}
	for _, constraint := range spread {
		if reason, ok := constraint.rejects(node); ok {
			add(reason.Code, reason.Text, reason.Detail)
		}
	}
	if volumes != nil {
		for _, restriction := range volumes.nodeAffinity {
			if nodeMatchesSelector(node, restriction.selector) {
				continue
			}
			add("volume-node-affinity", "The bound volume is not reachable from this node",
				fmt.Sprintf("PersistentVolume %s restricts which nodes can mount it.", restriction.volume))
		}
	}
	fit.Fits = len(fit.Reasons) == 0
	return fit
}

func taintText(taint corev1.Taint) string {
	if taint.Value == "" {
		return fmt.Sprintf("%s:%s", taint.Key, taint.Effect)
	}
	return fmt.Sprintf("%s=%s:%s", taint.Key, taint.Value, taint.Effect)
}

// toleratesTaint implements the toleration match: an empty key with Exists
// tolerates everything, an empty effect tolerates every effect.
func toleratesTaint(tolerations []corev1.Toleration, taint corev1.Taint) bool {
	for _, toleration := range tolerations {
		if toleration.Effect != "" && toleration.Effect != taint.Effect {
			continue
		}
		if toleration.Key == "" {
			if toleration.Operator == corev1.TolerationOpExists {
				return true
			}
			continue
		}
		if toleration.Key != taint.Key {
			continue
		}
		switch toleration.Operator {
		case corev1.TolerationOpExists:
			return true
		case corev1.TolerationOpEqual, "":
			if toleration.Value == taint.Value {
				return true
			}
		}
	}
	return false
}

// nodeMatchesPodSelectors covers both ways a Pod names the nodes it accepts:
// the flat nodeSelector map and required node affinity.
func nodeMatchesPodSelectors(node *corev1.Node, pod *corev1.Pod) bool {
	for key, value := range pod.Spec.NodeSelector {
		if node.Labels[key] != value {
			return false
		}
	}
	if pod.Spec.Affinity == nil || pod.Spec.Affinity.NodeAffinity == nil {
		return true
	}
	required := pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
	if required == nil || len(required.NodeSelectorTerms) == 0 {
		return true
	}
	return nodeMatchesSelector(node, required)
}

// nodeMatchesSelector evaluates a NodeSelector: terms are ORed, and the
// expressions inside one term are ANDed.
func nodeMatchesSelector(node *corev1.Node, selector *corev1.NodeSelector) bool {
	if selector == nil || len(selector.NodeSelectorTerms) == 0 {
		return true
	}
	for _, term := range selector.NodeSelectorTerms {
		if nodeMatchesTerm(node, term) {
			return true
		}
	}
	return false
}

func nodeMatchesTerm(node *corev1.Node, term corev1.NodeSelectorTerm) bool {
	for _, expression := range term.MatchExpressions {
		if !nodeSelectorRequirementMatches(node.Labels[expression.Key], hasLabel(node.Labels, expression.Key), expression) {
			return false
		}
	}
	for _, expression := range term.MatchFields {
		value, present := "", false
		if expression.Key == "metadata.name" {
			value, present = node.Name, true
		}
		if !nodeSelectorRequirementMatches(value, present, expression) {
			return false
		}
	}
	return len(term.MatchExpressions) > 0 || len(term.MatchFields) > 0
}

func hasLabel(set map[string]string, key string) bool {
	_, ok := set[key]
	return ok
}

func nodeSelectorRequirementMatches(value string, present bool, requirement corev1.NodeSelectorRequirement) bool {
	switch requirement.Operator {
	case corev1.NodeSelectorOpIn:
		return present && containsString(requirement.Values, value)
	case corev1.NodeSelectorOpNotIn:
		return !present || !containsString(requirement.Values, value)
	case corev1.NodeSelectorOpExists:
		return present
	case corev1.NodeSelectorOpDoesNotExist:
		return !present
	case corev1.NodeSelectorOpGt, corev1.NodeSelectorOpLt:
		if !present || len(requirement.Values) == 0 {
			return false
		}
		have, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return false
		}
		want, err := strconv.ParseInt(requirement.Values[0], 10, 64)
		if err != nil {
			return false
		}
		if requirement.Operator == corev1.NodeSelectorOpGt {
			return have > want
		}
		return have < want
	}
	return false
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

// ---- inter-pod affinity -----------------------------------------------------

type affinityTermRef struct {
	term corev1.PodAffinityTerm
	anti bool
}

func podAffinityTerms(pod *corev1.Pod) []affinityTermRef {
	out := []affinityTermRef{}
	if pod.Spec.Affinity == nil {
		return out
	}
	if pod.Spec.Affinity.PodAffinity != nil {
		for _, term := range pod.Spec.Affinity.PodAffinity.RequiredDuringSchedulingIgnoredDuringExecution {
			out = append(out, affinityTermRef{term: term})
		}
	}
	if pod.Spec.Affinity.PodAntiAffinity != nil {
		for _, term := range pod.Spec.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution {
			out = append(out, affinityTermRef{term: term, anti: true})
		}
	}
	return out
}

// compiledAffinityTerm holds one required affinity term with its selectors
// resolved and the matching Pods already counted per topology domain. The
// per-node check is then a map lookup — the cost of compiling selectors once
// instead of per node is the difference between instant and unusable on a large
// cluster.
type compiledAffinityTerm struct {
	anti        bool
	topologyKey string
	counts      map[string]int // topology value → matching Pods
	matched     int
}

func compilePodAffinity(pod *corev1.Pod, in *schedulingInputs) []compiledAffinityTerm {
	terms := podAffinityTerms(pod)
	if len(terms) == 0 {
		return nil
	}
	nodeDomain := make(map[string]map[string]string, len(in.nodes)) // node → labels
	for i := range in.nodes {
		nodeDomain[in.nodes[i].Name] = in.nodes[i].Labels
	}
	namespaceLabels := make(map[string]map[string]string, len(in.namespaces))
	for i := range in.namespaces {
		namespaceLabels[in.namespaces[i].Name] = in.namespaces[i].Labels
	}
	compiled := make([]compiledAffinityTerm, 0, len(terms))
	for _, ref := range terms {
		term := compiledAffinityTerm{anti: ref.anti, topologyKey: ref.term.TopologyKey, counts: map[string]int{}}
		selector := parseLabelSelector(ref.term.LabelSelector)
		inScope := namespaceScope(ref.term, pod.Namespace, namespaceLabels)
		for i := range in.pods {
			other := &in.pods[i]
			if !inScope(other.Namespace) || !selector.Matches(labels.Set(other.Labels)) {
				continue
			}
			value, ok := nodeDomain[other.Spec.NodeName][term.topologyKey]
			if !ok {
				continue
			}
			term.counts[value]++
			term.matched++
		}
		compiled = append(compiled, term)
	}
	return compiled
}

// namespaceScope resolves which namespaces a term's selector looks at: the
// explicit list, the namespaceSelector, or — when neither is set — the Pod's
// own namespace.
func namespaceScope(term corev1.PodAffinityTerm, podNamespace string, namespaceLabels map[string]map[string]string) func(string) bool {
	if len(term.Namespaces) > 0 {
		allowed := make(map[string]bool, len(term.Namespaces))
		for _, name := range term.Namespaces {
			allowed[name] = true
		}
		if term.NamespaceSelector == nil {
			return func(namespace string) bool { return allowed[namespace] }
		}
	}
	if term.NamespaceSelector != nil {
		selector, err := metav1.LabelSelectorAsSelector(term.NamespaceSelector)
		if err != nil {
			return func(string) bool { return false }
		}
		explicit := make(map[string]bool, len(term.Namespaces))
		for _, name := range term.Namespaces {
			explicit[name] = true
		}
		return func(namespace string) bool {
			if explicit[namespace] {
				return true
			}
			return selector.Matches(labels.Set(namespaceLabels[namespace]))
		}
	}
	return func(namespace string) bool { return namespace == podNamespace }
}

func (term compiledAffinityTerm) rejects(node *corev1.Node) (NodeFitReason, bool) {
	value, labelled := node.Labels[term.topologyKey]
	if term.anti {
		if !labelled {
			return NodeFitReason{}, false
		}
		if term.counts[value] > 0 {
			return NodeFitReason{Code: "pod-anti-affinity", Text: "Pod anti-affinity: a matching Pod is already here",
				Detail: fmt.Sprintf("%d matching Pod(s) share this node's %s=%s.", term.counts[value], term.topologyKey, value)}, true
		}
		return NodeFitReason{}, false
	}
	if !labelled {
		return NodeFitReason{Code: "pod-affinity", Text: "Pod affinity: this node has no " + term.topologyKey + " label",
			Detail: "A node outside the topology cannot satisfy the required affinity."}, true
	}
	if term.counts[value] == 0 {
		detail := fmt.Sprintf("No matching Pod runs in %s=%s.", term.topologyKey, value)
		if term.matched == 0 {
			detail = "No Pod anywhere matches the required affinity, so no node can satisfy it."
		}
		return NodeFitReason{Code: "pod-affinity", Text: "Pod affinity: no matching Pod in this topology", Detail: detail}, true
	}
	return NodeFitReason{}, false
}

// ---- topology spread --------------------------------------------------------

// compiledSpread is one hard spread constraint with its domain counts resolved.
type compiledSpread struct {
	topologyKey string
	maxSkew     int32
	counts      map[string]int
	min         int
	eligible    map[string]bool // topology values that count towards the skew
}

func compileTopologySpread(pod *corev1.Pod, in *schedulingInputs) []compiledSpread {
	compiled := []compiledSpread{}
	for _, constraint := range pod.Spec.TopologySpreadConstraints {
		if constraint.WhenUnsatisfiable != corev1.DoNotSchedule {
			continue // ScheduleAnyway only changes ranking
		}
		selector := parseLabelSelector(constraint.LabelSelector)
		spread := compiledSpread{topologyKey: constraint.TopologyKey, maxSkew: constraint.MaxSkew,
			counts: map[string]int{}, eligible: map[string]bool{}}
		// Domains come from the nodes the Pod could otherwise use: the default
		// nodeAffinityPolicy is Honor, so nodes the Pod's own node selection
		// already excludes do not take part in the spread.
		nodeDomain := map[string]string{}
		for i := range in.nodes {
			node := &in.nodes[i]
			value, ok := node.Labels[constraint.TopologyKey]
			if !ok {
				continue
			}
			nodeDomain[node.Name] = value
			if nodeMatchesPodSelectors(node, pod) {
				spread.eligible[value] = true
			}
		}
		for i := range in.pods {
			other := &in.pods[i]
			if other.Namespace != pod.Namespace || !selector.Matches(labels.Set(other.Labels)) {
				continue
			}
			value, ok := nodeDomain[other.Spec.NodeName]
			if !ok || !spread.eligible[value] {
				continue
			}
			spread.counts[value]++
		}
		spread.min = minimumDomainCount(spread)
		if constraint.MinDomains != nil && int32(len(spread.counts)) < *constraint.MinDomains {
			// Fewer domains hold a matching Pod than minDomains requires, so the
			// missing domains count as empty and the skew is measured from zero.
			spread.min = 0
		}
		compiled = append(compiled, spread)
	}
	return compiled
}

func minimumDomainCount(spread compiledSpread) int {
	min := -1
	for value := range spread.eligible {
		count := spread.counts[value]
		if min == -1 || count < min {
			min = count
		}
	}
	if min == -1 {
		return 0
	}
	return min
}

func (spread compiledSpread) rejects(node *corev1.Node) (NodeFitReason, bool) {
	value, labelled := node.Labels[spread.topologyKey]
	if !labelled {
		return NodeFitReason{Code: "topology-spread", Text: "Topology spread: this node has no " + spread.topologyKey + " label",
			Detail: "Nodes outside the topology cannot take a Pod with a DoNotSchedule spread constraint."}, true
	}
	skew := spread.counts[value] + 1 - spread.min
	if int32(skew) > spread.maxSkew {
		return NodeFitReason{Code: "topology-spread", Text: "Topology spread would be uneven",
			Detail: fmt.Sprintf("%s=%s already holds %d matching Pod(s) against a minimum of %d; one more exceeds maxSkew %d.",
				spread.topologyKey, value, spread.counts[value], spread.min, spread.maxSkew)}, true
	}
	return NodeFitReason{}, false
}

// ---- volumes ----------------------------------------------------------------

// volumeConstraints are the node restrictions a Pod inherits from its volumes.
type volumeConstraints struct {
	nodeAffinity []volumeNodeAffinity
}

type volumeNodeAffinity struct {
	volume   string
	selector *corev1.NodeSelector
}

// analyzeVolumes reports the claims that stop a Pod outright and returns the
// node restrictions the bound ones impose. A claim waiting for its first
// consumer is deliberately not a problem: that is how WaitForFirstConsumer is
// meant to work, and calling it an error sends people to the wrong place.
func analyzeVolumes(report *SchedulingReport, pod *corev1.Pod, in *schedulingInputs) *volumeConstraints {
	constraints := &volumeConstraints{}
	names := make([]string, 0, len(in.claims))
	for name := range in.claims {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		claim := in.claims[name]
		if claim == nil {
			report.Findings = append(report.Findings, CheckFinding{SeverityCritical,
				fmt.Sprintf("PersistentVolumeClaim %s does not exist", name),
				"A Pod whose claim is missing is never scheduled; create the claim or fix the volume reference."})
			continue
		}
		if claim.DeletionTimestamp != nil {
			report.Findings = append(report.Findings, CheckFinding{SeverityCritical,
				fmt.Sprintf("PersistentVolumeClaim %s is being deleted", name),
				"The Pod cannot use a claim that is terminating."})
		}
		switch claim.Status.Phase {
		case corev1.ClaimBound:
			volume := in.volumes[claim.Spec.VolumeName]
			if volume != nil && volume.Spec.NodeAffinity != nil && volume.Spec.NodeAffinity.Required != nil {
				constraints.nodeAffinity = append(constraints.nodeAffinity,
					volumeNodeAffinity{volume: volume.Name, selector: volume.Spec.NodeAffinity.Required})
			}
		case corev1.ClaimPending:
			if waitsForConsumer(claim, in.classes) {
				report.Findings = append(report.Findings, CheckFinding{SeverityInfo,
					fmt.Sprintf("PersistentVolumeClaim %s binds once the Pod is scheduled", name),
					"Its StorageClass uses volumeBindingMode WaitForFirstConsumer, so a Pending claim here is expected and is not what blocks scheduling."})
				continue
			}
			report.Findings = append(report.Findings, CheckFinding{SeverityCritical,
				fmt.Sprintf("PersistentVolumeClaim %s is Pending", name),
				"Its StorageClass binds immediately, so nothing can be scheduled until a volume is provisioned. Its own events say why."})
		default:
			report.Findings = append(report.Findings, CheckFinding{SeverityWarning,
				fmt.Sprintf("PersistentVolumeClaim %s is %s", name, claim.Status.Phase),
				"Only a Bound claim can be mounted."})
		}
	}
	return constraints
}

func waitsForConsumer(claim *corev1.PersistentVolumeClaim, classes map[string]*storagev1.StorageClass) bool {
	if claim.Spec.StorageClassName == nil || *claim.Spec.StorageClassName == "" {
		return false
	}
	class := classes[*claim.Spec.StorageClassName]
	if class == nil || class.VolumeBindingMode == nil {
		return false
	}
	return *class.VolumeBindingMode == storagev1.VolumeBindingWaitForFirstConsumer
}
