package k8sclient

import (
	"context"
	"fmt"
	"sort"
	"sync"

	corev1 "k8s.io/api/core/v1"
	apiresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Right-sizing: what containers *asked* for versus what they actually use.
//
// Kubby already lists ResourceQuotas and LimitRanges, and already reads
// metrics-server for the pods table. Neither answers the question people
// actually have — "is this namespace's reservation anywhere near its usage?" —
// because the answer needs both halves side by side. This file joins them.
//
// Everything here is read-only and derived; nothing is written to the cluster.

// unset marks a value the cluster did not give us, and is deliberately not 0:
// a container with no CPU request and a container requesting 0 are different
// situations, and reporting the first as "0m requested" would hide the finding.
const unset int64 = -1

// Thresholds for the findings. Deliberately conservative — a false "this is
// over-provisioned" costs the user real trust.
const (
	idleRequestPct     = 15  // using ≤15% of the request → over-provisioned
	burstRequestPct    = 200 // using ≥200% of the request → under-requested
	nearLimitPct       = 90  // memory ≥90% of its limit → OOMKill risk
	minCPURequestMilli = 50  // ignore over-provisioning below this: noise
	minMemRequestBytes = 64 << 20
)

// ContainerSizing is one container's declared requests and limits next to what it
// is actually consuming.
type ContainerSizing struct {
	Namespace string `json:"namespace"`
	Pod       string `json:"pod"`
	Container string `json:"container"`
	QoS       string `json:"qos"` // Guaranteed | Burstable | BestEffort

	CPURequest int64 `json:"cpuRequest"` // millicores, or unset
	CPULimit   int64 `json:"cpuLimit"`
	MemRequest int64 `json:"memRequest"` // bytes, or unset
	MemLimit   int64 `json:"memLimit"`

	CPUUsage int64 `json:"cpuUsage"` // millicores, or unset when metrics are missing
	MemUsage int64 `json:"memUsage"` // bytes
	// MetricsObserved distinguishes this exact container from a successful but
	// partial PodMetrics list. A global "metrics API answered" flag is not enough.
	MetricsObserved bool `json:"metricsObserved"`

	// Usage as a percentage of the *request* — the amount the scheduler reserved
	// on the container's behalf. unset when either side is unknown.
	CPUPct int64 `json:"cpuPct"`
	MemPct int64 `json:"memPct"`
	// Memory as a percentage of the *limit* — the number that predicts an OOMKill.
	MemOfLimit int64 `json:"memOfLimit"`

	Restarts  int32    `json:"restarts"`
	OOMKilled bool     `json:"oomKilled"` // the last exit was an OOM kill
	Findings  []string `json:"findings"`  // most severe first; empty means nothing to say

	// Severity ranks this container against the others (higher is worse). It is
	// exported so the UI can offer "only the serious ones": on a cluster where
	// nothing declares a memory limit, every container has a finding, and a list
	// that flags everything ranks nothing.
	Severity int `json:"severity"`
}

// QuotaLine is one line of a ResourceQuota's status: what the namespace is
// allowed and what it has taken.
type QuotaLine struct {
	Quota    string `json:"quota"`
	Resource string `json:"resource"`
	Hard     string `json:"hard"`
	Used     string `json:"used"`
}

// NamespaceSizing totals a namespace, which is the level at which quota
// conversations actually happen.
type NamespaceSizing struct {
	Namespace  string `json:"namespace"`
	Containers int    `json:"containers"`
	Pods       int    `json:"pods"`

	CPURequest int64 `json:"cpuRequest"`
	CPULimit   int64 `json:"cpuLimit"`
	CPUUsage   int64 `json:"cpuUsage"`
	MemRequest int64 `json:"memRequest"`
	MemLimit   int64 `json:"memLimit"`
	MemUsage   int64 `json:"memUsage"`

	MetricsExpected int `json:"metricsExpected"`
	MetricsObserved int `json:"metricsObserved"`

	// Containers missing a request or a memory limit — the two omissions that
	// make a namespace's totals meaningless and its neighbours unsafe.
	NoCPURequest int `json:"noCpuRequest"`
	NoMemRequest int `json:"noMemRequest"`
	NoMemLimit   int `json:"noMemLimit"`

	// Undeclared counts containers missing **at least one** of the three, so it can
	// be compared against Containers. Adding the three counts instead would double
	// count — one container with nothing declared contributes to all three — and
	// produce a headline number larger than the number of containers, which is
	// what the first version of the view displayed.
	Undeclared int `json:"undeclared"`

	Quota       []QuotaLine `json:"quota"`
	LimitRanges int         `json:"limitRanges"`
}

// SizingReport is the whole view: per-namespace totals, the containers worth
// looking at, and the cluster capacity that gives the totals a scale.
type SizingReport struct {
	Scope      string            `json:"scope"` // "" = all namespaces
	Totals     NamespaceSizing   `json:"totals"`
	Namespaces []NamespaceSizing `json:"namespaces"`
	Containers []ContainerSizing `json:"containers"`

	// Advice is what the *aggregate* says, which is usually more useful than the
	// rows: "18 of 22 containers declare no memory limit" is one decision (add a
	// LimitRange), where 18 identical findings are just 18 rows.
	Advice []string `json:"advice"`

	// Allocatable across Ready nodes — "requests 12 cores" only means something
	// against what the cluster has.
	AllocCPU int64 `json:"allocCpu"` // millicores
	AllocMem int64 `json:"allocMem"` // bytes
	Nodes    int   `json:"nodes"`

	// Shares of allocatable, computed **here and only here**.
	//
	// They used to be computed twice — integer division for the advice sentence and
	// Math.round in the frontend — so the same fact appeared on one screen as both
	// "11% of the cluster" and "12% of cluster". A percentage that two places derive
	// is a percentage that will eventually disagree with itself.
	CPUReservedPct int64 `json:"cpuReservedPct"`
	CPUUsedPct     int64 `json:"cpuUsedPct"`
	MemReservedPct int64 `json:"memReservedPct"`
	MemUsedPct     int64 `json:"memUsedPct"`

	MetricsAvailable   bool   `json:"metricsAvailable"`
	MetricsComplete    bool   `json:"metricsComplete"`
	MetricsExpected    int    `json:"metricsExpected"`
	MetricsObserved    int    `json:"metricsObserved"`
	MetricsCoveragePct int64  `json:"metricsCoveragePct"`
	Note               string `json:"note"` // why usage is missing, when it is
}

// Sizing builds the right-sizing report for a namespace ("" = whole cluster).
//
// Usage is optional: without metrics-server the requests/limits half is still
// worth showing, because "no memory limit anywhere" is a finding that needs no
// measurement. The UI is told which half is missing rather than shown zeroes.
func Sizing(ctx context.Context, c *Cluster, namespace string) (*SizingReport, error) {
	var (
		pods    *corev1.PodList
		podsErr error

		usage map[string]map[string]corev1.ResourceList

		quotas   []corev1.ResourceQuota
		limitRng map[string]int

		allocCPU, allocMem int64
		readyNodes         int
	)

	var wg sync.WaitGroup
	wg.Add(4)

	go func() {
		defer wg.Done()
		pods, podsErr = c.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	}()
	go func() {
		defer wg.Done()
		usage = containerUsage(ctx, c, namespace)
	}()
	go func() {
		defer wg.Done()
		quotas, limitRng = namespaceLimits(ctx, c, namespace)
	}()
	go func() {
		defer wg.Done()
		allocCPU, allocMem, readyNodes = allocatable(ctx, c)
	}()
	wg.Wait()

	if podsErr != nil {
		return nil, podsErr
	}

	report := &SizingReport{
		Scope:            namespace,
		AllocCPU:         allocCPU,
		AllocMem:         allocMem,
		Nodes:            readyNodes,
		MetricsAvailable: usage != nil,
	}
	if usage == nil {
		report.Note = "metrics-server is not available, so only the declared requests and limits are shown."
	}

	byNamespace := map[string]*NamespaceSizing{}
	for i := range pods.Items {
		pod := &pods.Items[i]
		if !sizingCountsPod(pod) {
			continue
		}
		ns := nsTotals(byNamespace, pod.Namespace)
		ns.Pods++

		status := containerStatusByName(pod)
		for j := range pod.Spec.Containers {
			cs := containerSizing(pod, &pod.Spec.Containers[j], status, usage)
			addToNamespace(ns, &cs)
			if len(cs.Findings) > 0 {
				report.Containers = append(report.Containers, cs)
			}
		}
	}

	for _, q := range quotas {
		ns := nsTotals(byNamespace, q.Namespace)
		ns.Quota = append(ns.Quota, quotaLines(&q)...)
	}
	for nsName, n := range limitRng {
		nsTotals(byNamespace, nsName).LimitRanges = n
	}

	for _, ns := range byNamespace {
		if usage == nil || ns.MetricsObserved < ns.MetricsExpected {
			// An aggregate built from a subset is not a measurement of the
			// namespace. Keep per-container observations, but render the total as
			// unknown rather than a dangerously low number.
			ns.CPUUsage = unset
			ns.MemUsage = unset
		}
		report.Namespaces = append(report.Namespaces, *ns)
		addToTotals(&report.Totals, ns)
	}
	report.MetricsExpected = report.Totals.MetricsExpected
	report.MetricsObserved = report.Totals.MetricsObserved
	report.MetricsComplete = usage != nil && report.MetricsObserved == report.MetricsExpected
	if report.MetricsExpected > 0 {
		report.MetricsCoveragePct = int64(report.MetricsObserved * 100 / report.MetricsExpected)
	} else if usage != nil {
		report.MetricsCoveragePct = 100
	}
	if !report.MetricsComplete {
		report.Totals.CPUUsage = unset
		report.Totals.MemUsage = unset
		if usage != nil {
			report.Note = fmt.Sprintf("metrics-server returned %d of %d live container metrics (%d%% coverage), so aggregate usage and downsizing advice are withheld.",
				report.MetricsObserved, report.MetricsExpected, report.MetricsCoveragePct)
		}
	}
	sort.Slice(report.Namespaces, func(i, j int) bool {
		return report.Namespaces[i].Namespace < report.Namespaces[j].Namespace
	})
	// Worst first: a container that has been OOMKilled outranks one that is merely
	// oversized, and the list is meant to be read top-down.
	sort.SliceStable(report.Containers, func(i, j int) bool {
		a, b := report.Containers[i], report.Containers[j]
		if sa, sb := severity(&a), severity(&b); sa != sb {
			return sa > sb
		}
		if a.Namespace != b.Namespace {
			return a.Namespace < b.Namespace
		}
		if a.Pod != b.Pod {
			return a.Pod < b.Pod
		}
		return a.Container < b.Container
	})
	// Shares of allocatable, before advice() — which reads them rather than
	// recomputing them. percentOf yields unset when the divisor is 0; a share of
	// nothing is reported as 0 so the frontend need not special-case it.
	report.CPUReservedPct = pctOrZero(report.Totals.CPURequest, report.AllocCPU)
	report.CPUUsedPct = pctOrZero(report.Totals.CPUUsage, report.AllocCPU)
	report.MemReservedPct = pctOrZero(report.Totals.MemRequest, report.AllocMem)
	report.MemUsedPct = pctOrZero(report.Totals.MemUsage, report.AllocMem)

	report.Advice = advice(report)
	return report, nil
}

func pctOrZero(value, of int64) int64 {
	if p := percentOf(value, of); p != unset {
		return p
	}
	return 0
}

// advice reads the totals rather than the rows. Each line is meant to name one
// change worth making, and to be silent when there is nothing to say.
func advice(r *SizingReport) []string {
	var out []string
	t := &r.Totals
	if t.Containers == 0 {
		return nil
	}

	if t.NoMemLimit > 0 {
		missing := namespacesWithoutLimitRange(r)
		line := fmt.Sprintf("%d of %d containers %s no memory limit, so nothing stops one of them taking a node's memory.",
			t.NoMemLimit, t.Containers, verb(t.NoMemLimit, "declares", "declare"))
		if len(missing) > 0 {
			line += fmt.Sprintf(" A LimitRange would set a default in one place — %s %s none.",
				joinUpTo(missing, 4), verb(len(missing), "has", "have"))
		}
		out = append(out, line)
	}
	if t.NoCPURequest > 0 {
		out = append(out, fmt.Sprintf("%d of %d containers %s no CPU request, so the scheduler is packing them by guesswork and they are throttled first under pressure.",
			t.NoCPURequest, t.Containers, verb(t.NoCPURequest, "declares", "declare")))
	}

	// Reserved-versus-used: the sentence that starts a capacity conversation.
	// Allocatable is always cluster-wide, so a namespace-scoped report has to say
	// so — "reserves 11%" means 11% of the whole cluster either way.
	if r.AllocCPU > 0 && t.CPURequest > 0 && r.MetricsComplete {
		// Read the shares off the report rather than recomputing them — see the
		// comment on CPUReservedPct.
		reserved, used := r.CPUReservedPct, r.CPUUsedPct
		if reserved >= 10 && t.CPUUsage*3 < t.CPURequest {
			who := "CPU requests reserve"
			if r.Scope != "" {
				who = fmt.Sprintf("Namespace %s reserves", r.Scope)
			}
			out = append(out, fmt.Sprintf("%s %d%% of the cluster's CPU while actually using %d%% of it — roughly %s is reserved and idle.",
				who, reserved, used, FormatCPU(t.CPURequest-t.CPUUsage)))
		}
	}

	for i := range r.Namespaces {
		ns := &r.Namespaces[i]
		for _, q := range ns.Quota {
			if near, pctUsed := quotaNearlyFull(q); near {
				out = append(out, fmt.Sprintf("Namespace %s is at %d%% of its %s quota (%s of %s used, quota %q).",
					ns.Namespace, pctUsed, q.Resource, q.Used, q.Hard, q.Quota))
			}
		}
	}
	return out
}

// quotaNearlyFull compares two quantity strings without re-parsing units by hand:
// both come from the same ResourceQuota status, so apimachinery can measure them.
func quotaNearlyFull(q QuotaLine) (bool, int64) {
	hard, err1 := parseQuantity(q.Hard)
	used, err2 := parseQuantity(q.Used)
	if err1 != nil || err2 != nil || hard <= 0 {
		return false, 0
	}
	p := used * 100 / hard
	return p >= 80, p
}

func namespacesWithoutLimitRange(r *SizingReport) []string {
	var out []string
	for i := range r.Namespaces {
		ns := &r.Namespaces[i]
		if ns.NoMemLimit > 0 && ns.LimitRanges == 0 {
			out = append(out, ns.Namespace)
		}
	}
	return out
}

// joinUpTo lists names but refuses to print a hundred of them — joinComma lives
// in actions.go.
func joinUpTo(items []string, max int) string {
	if len(items) <= max {
		return joinComma(items)
	}
	return fmt.Sprintf("%s and %d more", joinComma(items[:max]), len(items)-max)
}

// verb picks the singular or plural form. These strings are read by a person, and
// "1 of 10 containers declare" reads as a bug in the tool.
func verb(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}

// sizingCountsPod excludes pods whose usage or reservation is no longer real: a
// finished Job holds nothing, and a terminating pod is on its way out.
func sizingCountsPod(pod *corev1.Pod) bool {
	if pod.DeletionTimestamp != nil {
		return false
	}
	switch pod.Status.Phase {
	case corev1.PodSucceeded, corev1.PodFailed:
		return false
	}
	return true
}

func containerSizing(pod *corev1.Pod, spec *corev1.Container, status map[string]*corev1.ContainerStatus, usage map[string]map[string]corev1.ResourceList) ContainerSizing {
	cs := ContainerSizing{
		Namespace:  pod.Namespace,
		Pod:        pod.Name,
		Container:  spec.Name,
		QoS:        string(pod.Status.QOSClass),
		CPURequest: quantity(spec.Resources.Requests, corev1.ResourceCPU, true),
		CPULimit:   quantity(spec.Resources.Limits, corev1.ResourceCPU, true),
		MemRequest: quantity(spec.Resources.Requests, corev1.ResourceMemory, false),
		MemLimit:   quantity(spec.Resources.Limits, corev1.ResourceMemory, false),
		CPUUsage:   unset,
		MemUsage:   unset,
		CPUPct:     unset,
		MemPct:     unset,
		MemOfLimit: unset,
	}

	if st := status[spec.Name]; st != nil {
		cs.Restarts = st.RestartCount
		if t := st.LastTerminationState.Terminated; t != nil && t.Reason == "OOMKilled" {
			cs.OOMKilled = true
		}
	}

	if usage != nil {
		if byContainer, ok := usage[pod.Namespace+"/"+pod.Name]; ok {
			if u, ok := byContainer[spec.Name]; ok {
				cs.CPUUsage = u.Cpu().MilliValue()
				cs.MemUsage = u.Memory().Value()
				cs.CPUPct = percentOf(cs.CPUUsage, cs.CPURequest)
				cs.MemPct = percentOf(cs.MemUsage, cs.MemRequest)
				cs.MemOfLimit = percentOf(cs.MemUsage, cs.MemLimit)
				cs.MetricsObserved = true
			}
		}
	}

	cs.Findings = findings(&cs)
	cs.Severity = severity(&cs)
	return cs
}

// parseQuantity turns a Kubernetes quantity string ("2", "512Mi", "1500m") into a
// comparable integer, in millis for CPU-like values and bytes otherwise. Both
// sides of a comparison go through it, so the unit only has to be consistent.
func parseQuantity(s string) (int64, error) {
	q, err := apiresource.ParseQuantity(s)
	if err != nil {
		return 0, err
	}
	return q.MilliValue(), nil
}

// findings turns the numbers into the sentences a person can act on. Ordered
// most severe first, because the UI shows the first one in a narrow column.
func findings(cs *ContainerSizing) []string {
	var out []string

	if cs.OOMKilled {
		out = append(out, "OOMKilled — it exceeded its memory limit and was killed")
	}
	if cs.MemOfLimit >= nearLimitPct {
		out = append(out, fmt.Sprintf("memory at %d%% of its limit", cs.MemOfLimit))
	}
	if cs.MemLimit == unset {
		out = append(out, "no memory limit — it can take the whole node down with it")
	}
	if cs.MemRequest == unset {
		out = append(out, "no memory request — the scheduler is placing it blind")
	}
	if cs.CPURequest == unset {
		out = append(out, "no CPU request — first to be throttled under pressure")
	}

	// Under-requested: it is using well over what was reserved for it, so it is
	// living on capacity the scheduler never promised.
	if cs.CPUPct != unset && cs.CPUPct >= burstRequestPct && cs.CPURequest >= minCPURequestMilli {
		out = append(out, fmt.Sprintf("using %d%% of its CPU request", cs.CPUPct))
	}
	// Over-provisioned: reserved capacity nothing else can use. Only worth saying
	// when the reservation is big enough to matter.
	if cs.CPUPct != unset && cs.CPUPct <= idleRequestPct && cs.CPURequest >= minCPURequestMilli {
		out = append(out, fmt.Sprintf("using %d%% of its %s CPU request", cs.CPUPct, FormatCPU(cs.CPURequest)))
	}
	if cs.MemPct != unset && cs.MemPct <= idleRequestPct && cs.MemRequest >= minMemRequestBytes {
		out = append(out, fmt.Sprintf("using %d%% of its %s memory request", cs.MemPct, FormatMem(cs.MemRequest)))
	}
	return out
}

// severity ranks a container for the sort. Only the ordering matters, not the
// numbers.
func severity(cs *ContainerSizing) int {
	switch {
	case cs.OOMKilled:
		return 5
	case cs.MemOfLimit >= nearLimitPct:
		return 4
	case cs.MemLimit == unset:
		return 3
	case cs.MemRequest == unset || cs.CPURequest == unset:
		return 2
	default:
		return 1
	}
}

func addToNamespace(ns *NamespaceSizing, cs *ContainerSizing) {
	ns.Containers++
	ns.MetricsExpected++
	if cs.MetricsObserved {
		ns.MetricsObserved++
	}
	addIfSet(&ns.CPURequest, cs.CPURequest)
	addIfSet(&ns.CPULimit, cs.CPULimit)
	addIfSet(&ns.MemRequest, cs.MemRequest)
	addIfSet(&ns.MemLimit, cs.MemLimit)
	addIfSet(&ns.CPUUsage, cs.CPUUsage)
	addIfSet(&ns.MemUsage, cs.MemUsage)
	if cs.CPURequest == unset {
		ns.NoCPURequest++
	}
	if cs.MemRequest == unset {
		ns.NoMemRequest++
	}
	if cs.MemLimit == unset {
		ns.NoMemLimit++
	}
	if cs.CPURequest == unset || cs.MemRequest == unset || cs.MemLimit == unset {
		ns.Undeclared++
	}
}

func addToTotals(t *NamespaceSizing, ns *NamespaceSizing) {
	t.Containers += ns.Containers
	t.Pods += ns.Pods
	t.CPURequest += ns.CPURequest
	t.CPULimit += ns.CPULimit
	t.CPUUsage += ns.CPUUsage
	t.MemRequest += ns.MemRequest
	t.MemLimit += ns.MemLimit
	t.MemUsage += ns.MemUsage
	t.NoCPURequest += ns.NoCPURequest
	t.NoMemRequest += ns.NoMemRequest
	t.NoMemLimit += ns.NoMemLimit
	t.Undeclared += ns.Undeclared
	t.MetricsExpected += ns.MetricsExpected
	t.MetricsObserved += ns.MetricsObserved
}

func addIfSet(sum *int64, v int64) {
	if v != unset {
		*sum += v
	}
}

func nsTotals(m map[string]*NamespaceSizing, name string) *NamespaceSizing {
	if ns, ok := m[name]; ok {
		return ns
	}
	ns := &NamespaceSizing{Namespace: name}
	m[name] = ns
	return ns
}

func containerStatusByName(pod *corev1.Pod) map[string]*corev1.ContainerStatus {
	out := make(map[string]*corev1.ContainerStatus, len(pod.Status.ContainerStatuses))
	for i := range pod.Status.ContainerStatuses {
		st := &pod.Status.ContainerStatuses[i]
		out[st.Name] = st
	}
	return out
}

func quantity(list corev1.ResourceList, name corev1.ResourceName, milli bool) int64 {
	q, ok := list[name]
	if !ok {
		return unset
	}
	if milli {
		return q.MilliValue()
	}
	return q.Value()
}

func percentOf(value, of int64) int64 {
	if value == unset || of == unset || of == 0 {
		return unset
	}
	return value * 100 / of
}

// containerUsage returns per-container usage keyed "namespace/pod" → container.
// nil (not an empty map) means metrics are unavailable, which the report reports
// rather than showing as zero usage.
func containerUsage(ctx context.Context, c *Cluster, namespace string) map[string]map[string]corev1.ResourceList {
	if c.Metrics == nil {
		return nil
	}
	list, err := c.Metrics.MetricsV1beta1().PodMetricses(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil
	}
	out := make(map[string]map[string]corev1.ResourceList, len(list.Items))
	for _, pm := range list.Items {
		byContainer := make(map[string]corev1.ResourceList, len(pm.Containers))
		for _, ct := range pm.Containers {
			byContainer[ct.Name] = ct.Usage
		}
		out[pm.Namespace+"/"+pm.Name] = byContainer
	}
	return out
}

// namespaceLimits fetches the ResourceQuotas and counts the LimitRanges. Both are
// best-effort: a token that cannot read them should still get the sizing table.
func namespaceLimits(ctx context.Context, c *Cluster, namespace string) ([]corev1.ResourceQuota, map[string]int) {
	quotas := []corev1.ResourceQuota{}
	if ql, err := c.Clientset.CoreV1().ResourceQuotas(namespace).List(ctx, metav1.ListOptions{}); err == nil {
		quotas = ql.Items
	}
	counts := map[string]int{}
	if lr, err := c.Clientset.CoreV1().LimitRanges(namespace).List(ctx, metav1.ListOptions{}); err == nil {
		for _, l := range lr.Items {
			counts[l.Namespace]++
		}
	}
	return quotas, counts
}

// quotaLines flattens a ResourceQuota's status into hard/used pairs, keeping only
// the resources that constrain compute — a quota on ConfigMap count is real but
// has nothing to do with sizing.
func quotaLines(q *corev1.ResourceQuota) []QuotaLine {
	interesting := map[corev1.ResourceName]bool{
		corev1.ResourceCPU: true, corev1.ResourceMemory: true,
		corev1.ResourceRequestsCPU: true, corev1.ResourceRequestsMemory: true,
		corev1.ResourceLimitsCPU: true, corev1.ResourceLimitsMemory: true,
		corev1.ResourcePods: true,
	}
	var out []QuotaLine
	for name, hard := range q.Status.Hard {
		if !interesting[name] {
			continue
		}
		used := "0"
		if u, ok := q.Status.Used[name]; ok {
			used = u.String()
		}
		out = append(out, QuotaLine{
			Quota:    q.Name,
			Resource: string(name),
			Hard:     hard.String(),
			Used:     used,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Resource < out[j].Resource })
	return out
}

// allocatable sums what Ready nodes can actually hand out. A NotReady node's
// capacity is not available, so counting it would overstate headroom.
func allocatable(ctx context.Context, c *Cluster) (cpu, mem int64, nodes int) {
	list, err := c.Clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return 0, 0, 0
	}
	for i := range list.Items {
		n := &list.Items[i]
		ready := false
		for _, cond := range n.Status.Conditions {
			if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
				ready = true
				break
			}
		}
		if !ready {
			continue
		}
		nodes++
		cpu += n.Status.Allocatable.Cpu().MilliValue()
		mem += n.Status.Allocatable.Memory().Value()
	}
	return cpu, mem, nodes
}

// FormatCPU and FormatMem render the units Kubernetes users read, and are
// exported so the CLI prints exactly what the GUI shows.
func FormatCPU(milli int64) string {
	if milli == unset {
		return "—"
	}
	if milli < 1000 {
		return fmt.Sprintf("%dm", milli)
	}
	return fmt.Sprintf("%.2f", float64(milli)/1000)
}

func FormatMem(bytes int64) string {
	if bytes == unset {
		return "—"
	}
	const mi = 1 << 20
	if bytes < 1024*mi {
		return fmt.Sprintf("%dMi", bytes/mi)
	}
	return fmt.Sprintf("%.1fGi", float64(bytes)/float64(1024*mi))
}
