package k8sclient

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// "Why is this Pod Pending?" is the question an operator asks most often, and
// the one Kubernetes answers worst. The scheduler emits a single FailedScheduling
// event that compresses a whole cluster into one line — "0/12 nodes are
// available: 7 Insufficient memory, 3 node(s) had untolerated taint" — and
// kubectl describe repeats it without saying which nodes, how much was missing,
// or how close the Pod came to fitting.
//
// This file re-runs the feasibility half of scheduling against the objects the
// API server reports, node by node, and keeps every reason instead of the first.
// It is read-only and derived: it schedules nothing. Where the scheduler has
// already spoken its message is reported first, because it — not this
// simulation — is the authority on what happened.

const (
	// schedulingNodeLimit bounds the per-node detail sent to the UI. The
	// aggregate counts always cover every node; only the list is trimmed.
	schedulingNodeLimit = 100
	// schedulingReasonNodes is how many node names one reason group quotes.
	schedulingReasonNodes = 8
	// schedulingEvents is how many of the Pod's events are carried back.
	schedulingEvents = 6
)

// Verdicts. "fits" means this simulation found a feasible node while the Pod is
// still unscheduled — which is real, and usually means the scheduler is
// mid-cycle or a plugin Kubby does not model rejected it.
const (
	SchedulingScheduled     = "scheduled"
	SchedulingUnschedulable = "unschedulable"
	SchedulingFits          = "fits"
	SchedulingUnknown       = "unknown"
)

// SchedulingRequest is one resource the Pod reserves, as the scheduler counts it.
type SchedulingRequest struct {
	Resource string `json:"resource"`
	Request  string `json:"request"`
}

// NodeFitReason is one node's reason for rejecting the Pod.
type NodeFitReason struct {
	Code   string `json:"code"`
	Text   string `json:"text"`
	Detail string `json:"detail"`
}

// NodeFit is one node's verdict.
type NodeFit struct {
	Name        string          `json:"name"`
	Fits        bool            `json:"fits"`
	Ready       bool            `json:"ready"`
	Schedulable bool            `json:"schedulable"`
	CPUFree     string          `json:"cpuFree"`
	MemFree     string          `json:"memFree"`
	Pods        string          `json:"pods"` // "12 / 110"
	Reasons     []NodeFitReason `json:"reasons"`
}

// SchedulingReason groups the same rejection across nodes, which is what makes
// the answer usable: "7 nodes: insufficient memory" beats seven separate lines.
type SchedulingReason struct {
	Code   string   `json:"code"`
	Title  string   `json:"title"`
	Detail string   `json:"detail"`
	Count  int      `json:"count"`
	Nodes  []string `json:"nodes"`
}

// SchedulingReport answers the question for one Pod.
type SchedulingReport struct {
	Namespace  string              `json:"namespace"`
	Name       string              `json:"name"`
	Phase      string              `json:"phase"`
	NodeName   string              `json:"nodeName"`
	Scheduled  bool                `json:"scheduled"`
	Verdict    string              `json:"verdict"`
	Headline   string              `json:"headline"`
	Requests   []SchedulingRequest `json:"requests"`
	NodesTotal int                 `json:"nodesTotal"`
	NodesFit   int                 `json:"nodesFit"`
	Nodes      []NodeFit           `json:"nodes"`
	Reasons    []SchedulingReason  `json:"reasons"`
	Findings   []CheckFinding      `json:"findings"`
	Events     []EventInfo         `json:"events"`
	Limits     []string            `json:"limits"`
	Warnings   []string            `json:"warnings"`
	CheckedAt  string              `json:"checkedAt"`
}

// schedulingInputs are the objects the simulation reads, fetched once.
type schedulingInputs struct {
	nodes      []corev1.Node
	pods       []corev1.Pod // hold a node, cluster-wide
	namespaces []corev1.Namespace
	claims     map[string]*corev1.PersistentVolumeClaim // nil value = does not exist
	volumes    map[string]*corev1.PersistentVolume
	classes    map[string]*storagev1.StorageClass
	events     []EventInfo
	warnings   []string
	mu         sync.Mutex
}

func (in *schedulingInputs) warn(format string, args ...interface{}) {
	in.mu.Lock()
	defer in.mu.Unlock()
	in.warnings = append(in.warnings, fmt.Sprintf(format, args...))
}

// ExplainScheduling reports why a Pod is where it is: a node-by-node
// feasibility verdict while it is unscheduled, and what is holding it otherwise.
func ExplainScheduling(ctx context.Context, c *Cluster, namespace, name string) (*SchedulingReport, error) {
	if c == nil || c.Clientset == nil {
		return nil, fmt.Errorf("this connection has no Kubernetes client")
	}
	if namespace == "" || name == "" {
		return nil, fmt.Errorf("a namespace and a Pod name are required")
	}
	pod, err := c.Clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	in := gatherSchedulingInputs(ctx, c, pod)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return buildSchedulingReport(pod, in), nil
}

func gatherSchedulingInputs(ctx context.Context, c *Cluster, pod *corev1.Pod) *schedulingInputs {
	in := &schedulingInputs{
		claims:  map[string]*corev1.PersistentVolumeClaim{},
		volumes: map[string]*corev1.PersistentVolume{},
		classes: map[string]*storagev1.StorageClass{},
	}
	var wg sync.WaitGroup
	run := func(task func()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			task()
		}()
	}
	run(func() {
		list, err := c.Clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		if err != nil {
			in.warn("Nodes could not be listed, so no node could be evaluated: %v", err)
			return
		}
		in.nodes = list.Items
	})
	run(func() {
		// Only Pods that still hold a node reserve anything on it. The field
		// selector keeps finished Pods off the wire; the same test is applied
		// again below because not every client honours field selectors.
		list, err := c.Clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{
			FieldSelector: "status.phase!=Succeeded,status.phase!=Failed",
		})
		if err != nil {
			in.warn("Pods could not be listed, so what each node already holds is unknown: %v", err)
			return
		}
		for i := range list.Items {
			if podHoldsNode(&list.Items[i]) {
				in.pods = append(in.pods, list.Items[i])
			}
		}
	})
	if podNeedsNamespaceLabels(pod) {
		run(func() {
			list, err := c.Clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
			if err != nil {
				in.warn("Namespaces could not be listed, so a pod affinity namespaceSelector was not evaluated: %v", err)
				return
			}
			in.namespaces = list.Items
		})
	}
	for _, volume := range pod.Spec.Volumes {
		if volume.PersistentVolumeClaim == nil {
			continue
		}
		claimName := volume.PersistentVolumeClaim.ClaimName
		run(func() { in.readClaim(ctx, c, pod.Namespace, claimName) })
	}
	run(func() {
		events, err := ListEvents(ctx, c, "Pod", pod.Namespace, pod.Name)
		if err != nil {
			in.warn("Events for this Pod could not be read: %v", err)
			return
		}
		if len(events) > schedulingEvents {
			events = events[:schedulingEvents]
		}
		in.events = events
	})
	wg.Wait()
	return in
}

// readClaim reads one claim and the two objects that decide whether it can
// block or pin scheduling: the volume it is bound to (its node affinity) and
// its StorageClass (its binding mode).
func (in *schedulingInputs) readClaim(ctx context.Context, c *Cluster, namespace, name string) {
	claim, err := c.Clientset.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			in.warn("PersistentVolumeClaim %s could not be read: %v", name, err)
			return
		}
		in.mu.Lock()
		in.claims[name] = nil
		in.mu.Unlock()
		return
	}
	in.mu.Lock()
	in.claims[name] = claim
	in.mu.Unlock()
	if claim.Spec.VolumeName != "" {
		if volume, err := c.Clientset.CoreV1().PersistentVolumes().Get(ctx, claim.Spec.VolumeName, metav1.GetOptions{}); err == nil {
			in.mu.Lock()
			in.volumes[claim.Spec.VolumeName] = volume
			in.mu.Unlock()
		}
	}
	if claim.Spec.StorageClassName != nil && *claim.Spec.StorageClassName != "" {
		if class, err := c.Clientset.StorageV1().StorageClasses().Get(ctx, *claim.Spec.StorageClassName, metav1.GetOptions{}); err == nil {
			in.mu.Lock()
			in.classes[class.Name] = class
			in.mu.Unlock()
		}
	}
}

// podHoldsNode is true while a Pod still occupies its node.
func podHoldsNode(pod *corev1.Pod) bool {
	if pod.Spec.NodeName == "" {
		return false
	}
	return pod.Status.Phase != corev1.PodSucceeded && pod.Status.Phase != corev1.PodFailed
}

func podNeedsNamespaceLabels(pod *corev1.Pod) bool {
	for _, term := range podAffinityTerms(pod) {
		if term.term.NamespaceSelector != nil {
			return true
		}
	}
	return false
}

func buildSchedulingReport(pod *corev1.Pod, in *schedulingInputs) *SchedulingReport {
	report := &SchedulingReport{
		Namespace: pod.Namespace, Name: pod.Name, Phase: string(pod.Status.Phase),
		NodeName: pod.Spec.NodeName, Scheduled: pod.Spec.NodeName != "",
		Requests: requestList(effectivePodRequests(pod)),
		Nodes:    []NodeFit{}, Reasons: []SchedulingReason{}, Findings: []CheckFinding{},
		Events: in.events, Warnings: in.warnings, NodesTotal: len(in.nodes),
		CheckedAt: checksNow().UTC().Format(time.RFC3339),
	}
	if report.Events == nil {
		report.Events = []EventInfo{}
	}
	if report.Warnings == nil {
		report.Warnings = []string{}
	}
	addSchedulerVerdict(report, pod)
	volumes := analyzeVolumes(report, pod, in)
	if report.Scheduled {
		explainScheduledPod(report, pod, in)
	} else {
		simulateScheduling(report, pod, in, volumes)
	}
	report.Limits = schedulingLimits(!report.Scheduled)
	return report
}

// addSchedulerVerdict reports what the scheduler itself said. Its message is
// the ground truth; everything below is Kubby explaining it.
func addSchedulerVerdict(report *SchedulingReport, pod *corev1.Pod) {
	for i := range pod.Status.Conditions {
		condition := &pod.Status.Conditions[i]
		if condition.Type != corev1.PodScheduled || condition.Status == corev1.ConditionTrue {
			continue
		}
		detail := strings.TrimSpace(condition.Message)
		if detail == "" {
			detail = "The scheduler has not placed this Pod yet."
		}
		title := "The scheduler reports: " + condition.Reason
		if condition.Reason == "" {
			title = "The scheduler has not placed this Pod"
		}
		report.Findings = append(report.Findings, CheckFinding{Severity: SeverityCritical, Title: title, Detail: detail})
	}
}

// explainScheduledPod answers the question for a Pod that already has a node:
// scheduling is over, so what is holding it is the kubelet's work.
func explainScheduledPod(report *SchedulingReport, pod *corev1.Pod, in *schedulingInputs) {
	report.Verdict = SchedulingScheduled
	node := nodeByName(in.nodes, pod.Spec.NodeName)
	if node == nil {
		report.Findings = append(report.Findings, CheckFinding{SeverityWarning,
			fmt.Sprintf("Node %s is not in this cluster's node list", pod.Spec.NodeName),
			"The node may have been removed while the Pod still references it."})
	} else if !nodeReady(node) {
		report.Findings = append(report.Findings, CheckFinding{SeverityCritical,
			fmt.Sprintf("Node %s is not Ready", pod.Spec.NodeName),
			"A Pod on a node whose kubelet stopped reporting is neither started nor rescheduled until the node recovers or the Pod is deleted."})
	}
	waiting := waitingContainers(pod)
	switch {
	case pod.Status.Phase == corev1.PodRunning && len(waiting) == 0:
		report.Headline = fmt.Sprintf("Scheduling is done: this Pod runs on %s.", pod.Spec.NodeName)
	case len(waiting) > 0:
		report.Headline = fmt.Sprintf("Scheduled on %s — the kubelet has not started it: %s.", pod.Spec.NodeName, waiting[0].reason)
		for _, container := range waiting {
			detail := container.message
			if explanation := waitingExplanation(container.reason); explanation != "" {
				detail = strings.TrimSpace(explanation + " " + detail)
			}
			report.Findings = append(report.Findings, CheckFinding{
				Severity: waitingSeverity(container.reason),
				Title:    fmt.Sprintf("Container %s is waiting: %s", container.name, container.reason),
				Detail:   detail,
			})
		}
	default:
		report.Headline = fmt.Sprintf("Scheduled on %s; its phase is %s.", pod.Spec.NodeName, pod.Status.Phase)
	}
	if pod.Status.Reason != "" {
		report.Findings = append(report.Findings, CheckFinding{SeverityWarning,
			"The kubelet reports: " + pod.Status.Reason, strings.TrimSpace(pod.Status.Message)})
	}
}

type waitingContainer struct{ name, reason, message string }

func waitingContainers(pod *corev1.Pod) []waitingContainer {
	out := []waitingContainer{}
	collect := func(statuses []corev1.ContainerStatus) {
		for i := range statuses {
			state := statuses[i].State.Waiting
			if state == nil || state.Reason == "PodInitializing" {
				continue
			}
			out = append(out, waitingContainer{name: statuses[i].Name, reason: state.Reason, message: strings.TrimSpace(state.Message)})
		}
	}
	collect(pod.Status.InitContainerStatuses)
	collect(pod.Status.ContainerStatuses)
	return out
}

func waitingSeverity(reason string) string {
	switch reason {
	case "ContainerCreating":
		return SeverityWarning
	}
	return SeverityCritical
}

func schedulingLimits(simulated bool) []string {
	limits := []string{
		"Read-only: this explains the cluster, it never schedules or evicts anything.",
	}
	if !simulated {
		return append(limits, "This Pod already has a node, so scheduling is finished; what remains is the kubelet's work on that node.")
	}
	return append(limits,
		"Only required rules decide feasibility. Preferred rules (preferredDuringScheduling, ScheduleAnyway spread) change ranking, not whether a node fits.",
		"Resource arithmetic uses the requests of the Pods the API server currently reports; a scheduling cycle in flight can change the answer within seconds.",
		"Scheduler extenders, custom plugins, CSI volume-count limits and dynamic resource allocation are not modelled.",
	)
}

// simulateScheduling evaluates every node and groups the rejections.
func simulateScheduling(report *SchedulingReport, pod *corev1.Pod, in *schedulingInputs, volumes *volumeConstraints) {
	if len(in.nodes) == 0 {
		report.Verdict = SchedulingUnknown
		report.Headline = "No node could be evaluated."
		report.Findings = append(report.Findings, CheckFinding{SeverityWarning, "This connection reported no nodes",
			"Either the cluster has no nodes, or this token may not list them."})
		return
	}
	requests := sortedResourceRequests(effectivePodRequests(pod))
	usage := nodeUsageIndex(in.pods)
	affinity := compilePodAffinity(pod, in)
	spread := compileTopologySpread(pod, in)

	fits := []NodeFit{}
	rejected := []NodeFit{}
	groups := map[string]*reasonGroup{}
	for i := range in.nodes {
		node := &in.nodes[i]
		fit := evaluateNodeFit(node, pod, requests, usage[node.Name], affinity, spread, volumes)
		if fit.Fits {
			fits = append(fits, fit)
			continue
		}
		rejected = append(rejected, fit)
		for _, reason := range fit.Reasons {
			group := groups[reason.Code]
			if group == nil {
				group = &reasonGroup{code: reason.Code, title: reason.Text}
				groups[reason.Code] = group
			}
			group.count++
			group.nodes = append(group.nodes, node.Name)
			if reason.Detail != "" && group.detail == "" {
				group.detail = reason.Detail
			}
		}
	}
	report.NodesFit = len(fits)
	report.Reasons = sortedReasonGroups(groups)
	report.Nodes = limitNodeFits(fits, rejected)
	if len(report.Nodes) < len(in.nodes) {
		report.Warnings = append(report.Warnings,
			fmt.Sprintf("%d of %d nodes are listed; the counts above cover all of them.", len(report.Nodes), len(in.nodes)))
	}
	if report.NodesFit > 0 {
		report.Verdict = SchedulingFits
		report.Headline = fmt.Sprintf("%s could take this Pod, so something outside these checks is holding it — a scheduler plugin, a quota, or a cycle still in flight.",
			countNodes(report.NodesFit))
		return
	}
	report.Verdict = SchedulingUnschedulable
	report.Headline = fmt.Sprintf("No node fits: %s", reasonSummary(report.Reasons, len(in.nodes)))
}

type reasonGroup struct {
	code, title, detail string
	count               int
	nodes               []string
}

func sortedReasonGroups(groups map[string]*reasonGroup) []SchedulingReason {
	out := make([]SchedulingReason, 0, len(groups))
	for _, group := range groups {
		sort.Strings(group.nodes)
		nodes := group.nodes
		if len(nodes) > schedulingReasonNodes {
			nodes = nodes[:schedulingReasonNodes]
		}
		out = append(out, SchedulingReason{Code: group.code, Title: group.title, Detail: group.detail,
			Count: group.count, Nodes: append([]string(nil), nodes...)})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Code < out[j].Code
	})
	return out
}

// reasonSummary reads like the scheduler's own one-liner, which is the sentence
// operators already know: "0/12 nodes are available: 7 Insufficient memory…".
func reasonSummary(reasons []SchedulingReason, nodes int) string {
	parts := make([]string, 0, len(reasons))
	for i, reason := range reasons {
		if i == 3 {
			parts = append(parts, "…")
			break
		}
		parts = append(parts, fmt.Sprintf("%d %s", reason.Count, strings.ToLower(reason.Title)))
	}
	if len(parts) == 0 {
		return fmt.Sprintf("0/%d nodes are available.", nodes)
	}
	return fmt.Sprintf("0/%d nodes are available: %s.", nodes, strings.Join(parts, ", "))
}

// limitNodeFits keeps the list bounded and useful: nodes that fit first, then
// the near misses, because a node rejected for one reason is the one to fix.
func limitNodeFits(fits, rejected []NodeFit) []NodeFit {
	sort.SliceStable(rejected, func(i, j int) bool {
		if len(rejected[i].Reasons) != len(rejected[j].Reasons) {
			return len(rejected[i].Reasons) < len(rejected[j].Reasons)
		}
		return rejected[i].Name < rejected[j].Name
	})
	sort.SliceStable(fits, func(i, j int) bool { return fits[i].Name < fits[j].Name })
	out := append(append(make([]NodeFit, 0, len(fits)+len(rejected)), fits...), rejected...)
	if len(out) > schedulingNodeLimit {
		out = out[:schedulingNodeLimit]
	}
	return out
}

func countNodes(n int) string {
	if n == 1 {
		return "1 node"
	}
	return fmt.Sprintf("%d nodes", n)
}
