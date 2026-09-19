package k8sclient

import (
	"context"
	"fmt"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

// The scheduling explainer is only worth having if it names the same cause the
// scheduler would. These tests pin one predicate each, plus the two properties
// that make it usable: reasons are grouped with counts, and the number of API
// requests does not grow with the cluster.

func qty(value string) apiresource.Quantity {
	return apiresource.MustParse(value)
}

func schedNode(name, cpu, memory string, mutate ...func(*corev1.Node)) *corev1.Node {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{"kubernetes.io/hostname": name}},
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU: qty(cpu), corev1.ResourceMemory: qty(memory), corev1.ResourcePods: qty("110"),
			},
			Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
		},
	}
	for _, fn := range mutate {
		fn(node)
	}
	return node
}

func schedPod(namespace, name string, mutate ...func(*corev1.Pod)) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "main"}}},
		Status:     corev1.PodStatus{Phase: corev1.PodPending},
	}
	for _, fn := range mutate {
		fn(pod)
	}
	return pod
}

func withRequests(cpu, memory string) func(*corev1.Pod) {
	return func(pod *corev1.Pod) {
		pod.Spec.Containers[0].Resources.Requests = corev1.ResourceList{
			corev1.ResourceCPU: qty(cpu), corev1.ResourceMemory: qty(memory),
		}
	}
}

func onNode(node string) func(*corev1.Pod) {
	return func(pod *corev1.Pod) {
		pod.Spec.NodeName = node
		pod.Status.Phase = corev1.PodRunning
	}
}

func explain(t *testing.T, objects []runtime.Object, namespace, name string) *SchedulingReport {
	t.Helper()
	cluster := &Cluster{Clientset: kubefake.NewSimpleClientset(objects...)}
	report, err := ExplainScheduling(context.Background(), cluster, namespace, name)
	if err != nil {
		t.Fatalf("ExplainScheduling: %v", err)
	}
	return report
}

func reasonByCode(t *testing.T, report *SchedulingReport, code string) SchedulingReason {
	t.Helper()
	for _, reason := range report.Reasons {
		if reason.Code == code {
			return reason
		}
	}
	t.Fatalf("reason %q not reported: %+v", code, report.Reasons)
	return SchedulingReason{}
}

func nodeFit(t *testing.T, report *SchedulingReport, name string) NodeFit {
	t.Helper()
	for _, fit := range report.Nodes {
		if fit.Name == name {
			return fit
		}
	}
	t.Fatalf("node %q not reported: %+v", name, report.Nodes)
	return NodeFit{}
}

func fitCodes(fit NodeFit) []string {
	codes := make([]string, 0, len(fit.Reasons))
	for _, reason := range fit.Reasons {
		codes = append(codes, reason.Code)
	}
	return codes
}

func TestExplainSchedulingGroupsReasonsWithCounts(t *testing.T) {
	pinChecksNow(t)
	objects := []runtime.Object{
		schedNode("small-1", "4", "2Gi"),
		schedNode("small-2", "4", "2Gi"),
		schedNode("tainted", "8", "16Gi", func(node *corev1.Node) {
			node.Spec.Taints = []corev1.Taint{{Key: "node-role.kubernetes.io/control-plane", Effect: corev1.TaintEffectNoSchedule}}
		}),
		schedPod("apps", "api", withRequests("500m", "4Gi")),
	}
	report := explain(t, objects, "apps", "api")

	if report.Verdict != SchedulingUnschedulable {
		t.Fatalf("verdict = %q, want %q", report.Verdict, SchedulingUnschedulable)
	}
	if report.NodesTotal != 3 || report.NodesFit != 0 {
		t.Fatalf("nodes total/fit = %d/%d, want 3/0", report.NodesTotal, report.NodesFit)
	}
	memory := reasonByCode(t, report, "insufficient:memory")
	if memory.Count != 2 {
		t.Fatalf("insufficient memory count = %d, want 2 (%+v)", memory.Count, memory)
	}
	if !strings.Contains(memory.Detail, "needs 4.0Gi") || !strings.Contains(memory.Detail, "free") {
		t.Fatalf("memory detail does not quantify the gap: %q", memory.Detail)
	}
	taint := reasonByCode(t, report, "taint:node-role.kubernetes.io/control-plane")
	if taint.Count != 1 || len(taint.Nodes) != 1 || taint.Nodes[0] != "tainted" {
		t.Fatalf("taint group = %+v", taint)
	}
	if !strings.Contains(report.Headline, "0/3 nodes are available") {
		t.Fatalf("headline = %q", report.Headline)
	}
	// The most common reason leads, so the summary points at the fix that helps most.
	if report.Reasons[0].Code != "insufficient:memory" {
		t.Fatalf("reasons are not ordered by count: %+v", report.Reasons)
	}
	if got := nodeFit(t, report, "small-1"); got.Fits || got.MemFree != "2.0Gi" {
		t.Fatalf("small-1 fit = %+v", got)
	}
}

func TestExplainSchedulingCountsResourcesAlreadyHeld(t *testing.T) {
	pinChecksNow(t)
	objects := []runtime.Object{
		schedNode("node-1", "4", "8Gi"),
		schedPod("apps", "neighbour", withRequests("3500m", "7Gi"), onNode("node-1")),
		// A finished Pod still exists in the API but holds nothing.
		schedPod("apps", "done", withRequests("4", "8Gi"), func(pod *corev1.Pod) {
			pod.Spec.NodeName = "node-1"
			pod.Status.Phase = corev1.PodSucceeded
		}),
		schedPod("apps", "api", withRequests("1", "2Gi")),
	}
	report := explain(t, objects, "apps", "api")

	if report.NodesFit != 0 {
		t.Fatalf("a node with 500m free must not fit a 1-core Pod: %+v", report.Nodes)
	}
	fit := nodeFit(t, report, "node-1")
	if fit.CPUFree != "500m" || fit.MemFree != "1.0Gi" {
		t.Fatalf("free capacity = cpu %q mem %q, want 500m / 1.0Gi", fit.CPUFree, fit.MemFree)
	}
	if fit.Pods != "1 / 110" {
		t.Fatalf("pod slots = %q, want the Succeeded Pod excluded", fit.Pods)
	}
	codes := fitCodes(fit)
	if len(codes) != 2 || codes[0] != "insufficient:cpu" || codes[1] != "insufficient:memory" {
		t.Fatalf("every reason should be kept, got %v", codes)
	}
}

func TestExplainSchedulingReportsNodeStatusOnceNotTwice(t *testing.T) {
	pinChecksNow(t)
	objects := []runtime.Object{
		schedNode("cordoned", "8", "16Gi", func(node *corev1.Node) {
			node.Spec.Unschedulable = true
			node.Spec.Taints = []corev1.Taint{{Key: corev1.TaintNodeUnschedulable, Effect: corev1.TaintEffectNoSchedule}}
		}),
		schedNode("down", "8", "16Gi", func(node *corev1.Node) {
			node.Status.Conditions = []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionFalse}}
			node.Spec.Taints = []corev1.Taint{{Key: corev1.TaintNodeNotReady, Effect: corev1.TaintEffectNoSchedule}}
		}),
		schedPod("apps", "api"),
	}
	report := explain(t, objects, "apps", "api")

	if codes := fitCodes(nodeFit(t, report, "cordoned")); len(codes) != 1 || codes[0] != "cordoned" {
		t.Fatalf("cordoned node reasons = %v, want only the cordon", codes)
	}
	if codes := fitCodes(nodeFit(t, report, "down")); len(codes) != 1 || codes[0] != "not-ready" {
		t.Fatalf("not-ready node reasons = %v, want only the readiness", codes)
	}
}

func TestExplainSchedulingNodeSelectionPredicates(t *testing.T) {
	pinChecksNow(t)
	gpu := corev1.ResourceName("nvidia.com/gpu")
	objects := []runtime.Object{
		schedNode("zone-a", "8", "16Gi", func(node *corev1.Node) {
			node.Labels["topology.kubernetes.io/zone"] = "a"
			node.Labels["cores"] = "8"
			node.Status.Allocatable[gpu] = qty("1")
		}),
		schedNode("zone-b", "8", "16Gi", func(node *corev1.Node) {
			node.Labels["topology.kubernetes.io/zone"] = "b"
			node.Labels["cores"] = "2"
		}),
		schedPod("apps", "api", func(pod *corev1.Pod) {
			pod.Spec.NodeSelector = map[string]string{"topology.kubernetes.io/zone": "a"}
			pod.Spec.Containers[0].Resources.Requests = corev1.ResourceList{gpu: qty("2")}
		}),
	}
	report := explain(t, objects, "apps", "api")

	if codes := fitCodes(nodeFit(t, report, "zone-b")); len(codes) != 2 {
		t.Fatalf("zone-b should fail both the selector and the GPU request: %v", codes)
	}
	if codes := fitCodes(nodeFit(t, report, "zone-a")); len(codes) != 1 || codes[0] != "insufficient:nvidia.com/gpu" {
		t.Fatalf("zone-a reasons = %v, want the extended resource only", codes)
	}
	// An extended resource is counted in whole units, never rendered as memory.
	gpuReason := reasonByCode(t, report, "insufficient:nvidia.com/gpu")
	if !strings.Contains(gpuReason.Detail, "needs 2") {
		t.Fatalf("gpu detail = %q", gpuReason.Detail)
	}
}

func TestExplainSchedulingNodeAffinityOperators(t *testing.T) {
	pinChecksNow(t)
	objects := []runtime.Object{
		schedNode("big", "8", "16Gi", func(node *corev1.Node) { node.Labels["cores"] = "8" }),
		schedNode("small", "8", "16Gi", func(node *corev1.Node) { node.Labels["cores"] = "2" }),
		schedPod("apps", "api", func(pod *corev1.Pod) {
			pod.Spec.Affinity = &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{{
					MatchExpressions: []corev1.NodeSelectorRequirement{{Key: "cores", Operator: corev1.NodeSelectorOpGt, Values: []string{"4"}}},
				}}},
			}}
		}),
	}
	report := explain(t, objects, "apps", "api")

	if report.Verdict != SchedulingFits || report.NodesFit != 1 {
		t.Fatalf("verdict = %q with %d fitting nodes, want one node matching cores > 4", report.Verdict, report.NodesFit)
	}
	if fit := nodeFit(t, report, "big"); !fit.Fits {
		t.Fatalf("the 8-core node should match: %+v", fit)
	}
	if codes := fitCodes(nodeFit(t, report, "small")); len(codes) != 1 || codes[0] != "node-affinity" {
		t.Fatalf("small node reasons = %v", codes)
	}
}

func TestExplainSchedulingHostPortConflict(t *testing.T) {
	pinChecksNow(t)
	hostPort := func(port int32) func(*corev1.Pod) {
		return func(pod *corev1.Pod) {
			pod.Spec.Containers[0].Ports = []corev1.ContainerPort{{ContainerPort: port, HostPort: port}}
		}
	}
	objects := []runtime.Object{
		schedNode("node-1", "8", "16Gi"),
		schedNode("node-2", "8", "16Gi"),
		schedPod("apps", "ingress", hostPort(80), onNode("node-1")),
		schedPod("apps", "api", hostPort(80)),
	}
	report := explain(t, objects, "apps", "api")

	if codes := fitCodes(nodeFit(t, report, "node-1")); len(codes) != 1 || codes[0] != "host-port" {
		t.Fatalf("node-1 reasons = %v, want the host port conflict", codes)
	}
	if !nodeFit(t, report, "node-2").Fits {
		t.Fatalf("node-2 has the port free and should fit")
	}
}

func TestExplainSchedulingPodAntiAffinity(t *testing.T) {
	pinChecksNow(t)
	objects := []runtime.Object{
		schedNode("node-1", "8", "16Gi"),
		schedNode("node-2", "8", "16Gi"),
		schedPod("apps", "web-1", func(pod *corev1.Pod) {
			pod.Labels = map[string]string{"app": "web"}
			pod.Spec.NodeName = "node-1"
			pod.Status.Phase = corev1.PodRunning
		}),
		schedPod("apps", "web-2", func(pod *corev1.Pod) {
			pod.Labels = map[string]string{"app": "web"}
			pod.Spec.Affinity = &corev1.Affinity{PodAntiAffinity: &corev1.PodAntiAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{{
					TopologyKey:   "kubernetes.io/hostname",
					LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
				}},
			}}
		}),
	}
	report := explain(t, objects, "apps", "web-2")

	if codes := fitCodes(nodeFit(t, report, "node-1")); len(codes) != 1 || codes[0] != "pod-anti-affinity" {
		t.Fatalf("node-1 reasons = %v, want anti-affinity", codes)
	}
	if !nodeFit(t, report, "node-2").Fits {
		t.Fatalf("node-2 holds no matching Pod and should fit")
	}
}

func TestExplainSchedulingTopologySpread(t *testing.T) {
	pinChecksNow(t)
	zone := func(value string) func(*corev1.Node) {
		return func(node *corev1.Node) { node.Labels["topology.kubernetes.io/zone"] = value }
	}
	spread := schedPod("apps", "web-3", func(pod *corev1.Pod) {
		pod.Labels = map[string]string{"app": "web"}
		pod.Spec.TopologySpreadConstraints = []corev1.TopologySpreadConstraint{{
			MaxSkew: 1, TopologyKey: "topology.kubernetes.io/zone", WhenUnsatisfiable: corev1.DoNotSchedule,
			LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
		}}
	})
	placed := func(name, node string) *corev1.Pod {
		return schedPod("apps", name, func(pod *corev1.Pod) {
			pod.Labels = map[string]string{"app": "web"}
			pod.Spec.NodeName = node
			pod.Status.Phase = corev1.PodRunning
		})
	}
	objects := []runtime.Object{
		schedNode("a-1", "8", "16Gi", zone("a")), schedNode("b-1", "8", "16Gi", zone("b")),
		placed("web-1", "a-1"), placed("web-2", "a-1"), spread,
	}
	report := explain(t, objects, "apps", "web-3")

	fit := nodeFit(t, report, "a-1")
	if len(fit.Reasons) != 1 || fit.Reasons[0].Code != "topology-spread" {
		t.Fatalf("a-1 reasons = %v, want the spread constraint", fitCodes(fit))
	}
	if !strings.Contains(fit.Reasons[0].Detail, "maxSkew 1") {
		t.Fatalf("spread detail should quote the constraint: %q", fit.Reasons[0].Detail)
	}
	if !nodeFit(t, report, "b-1").Fits {
		t.Fatalf("the empty zone should accept the Pod")
	}
}

func TestExplainSchedulingVolumeFindings(t *testing.T) {
	pinChecksNow(t)
	immediate, wait := storagev1.VolumeBindingImmediate, storagev1.VolumeBindingWaitForFirstConsumer
	claimName := func(name string) func(*corev1.Pod) {
		return func(pod *corev1.Pod) {
			pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{Name: name, VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: name}}})
		}
	}
	claim := func(name, class string, phase corev1.PersistentVolumeClaimPhase) *corev1.PersistentVolumeClaim {
		return &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: name},
			Spec:       corev1.PersistentVolumeClaimSpec{StorageClassName: &class},
			Status:     corev1.PersistentVolumeClaimStatus{Phase: phase},
		}
	}
	objects := []runtime.Object{
		schedNode("node-1", "8", "16Gi"),
		&storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "fast"}, VolumeBindingMode: &immediate},
		&storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "local"}, VolumeBindingMode: &wait},
		claim("data", "fast", corev1.ClaimPending),
		claim("scratch", "local", corev1.ClaimPending),
		schedPod("apps", "api", claimName("data"), claimName("scratch"), claimName("missing")),
	}
	report := explain(t, objects, "apps", "api")

	findings := map[string]CheckFinding{}
	for _, finding := range report.Findings {
		findings[finding.Title] = finding
	}
	if finding, ok := findings["PersistentVolumeClaim missing does not exist"]; !ok || finding.Severity != SeverityCritical {
		t.Fatalf("a missing claim must be critical: %+v", report.Findings)
	}
	if finding, ok := findings["PersistentVolumeClaim data is Pending"]; !ok || finding.Severity != SeverityCritical {
		t.Fatalf("an immediately-bound Pending claim must be critical: %+v", report.Findings)
	}
	// WaitForFirstConsumer is how local volumes are meant to work; calling it a
	// problem sends the operator to the wrong place.
	finding, ok := findings["PersistentVolumeClaim scratch binds once the Pod is scheduled"]
	if !ok || finding.Severity != SeverityInfo {
		t.Fatalf("WaitForFirstConsumer must not be reported as a problem: %+v", report.Findings)
	}
}

func TestExplainSchedulingBoundVolumePinsNodes(t *testing.T) {
	pinChecksNow(t)
	objects := []runtime.Object{
		schedNode("node-1", "8", "16Gi"), schedNode("node-2", "8", "16Gi"),
		&corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{Name: "pv-local"},
			Spec: corev1.PersistentVolumeSpec{NodeAffinity: &corev1.VolumeNodeAffinity{Required: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{{MatchExpressions: []corev1.NodeSelectorRequirement{{
					Key: "kubernetes.io/hostname", Operator: corev1.NodeSelectorOpIn, Values: []string{"node-1"}}}}},
			}}},
		},
		&corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "data"},
			Spec:       corev1.PersistentVolumeClaimSpec{VolumeName: "pv-local"},
			Status:     corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
		},
		schedPod("apps", "api", func(pod *corev1.Pod) {
			pod.Spec.Volumes = []corev1.Volume{{Name: "data", VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "data"}}}}
		}),
	}
	report := explain(t, objects, "apps", "api")

	if !nodeFit(t, report, "node-1").Fits {
		t.Fatalf("the volume's own node must fit")
	}
	if codes := fitCodes(nodeFit(t, report, "node-2")); len(codes) != 1 || codes[0] != "volume-node-affinity" {
		t.Fatalf("node-2 reasons = %v, want the volume restriction", codes)
	}
}

func TestExplainSchedulingScheduledPodExplainsKubelet(t *testing.T) {
	pinChecksNow(t)
	objects := []runtime.Object{
		schedNode("node-1", "8", "16Gi"),
		schedPod("apps", "api", func(pod *corev1.Pod) {
			pod.Spec.NodeName = "node-1"
			pod.Status.ContainerStatuses = []corev1.ContainerStatus{{Name: "main", State: corev1.ContainerState{
				Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff", Message: `Back-off pulling image "app:1.2.3"`}}}}
		}),
	}
	report := explain(t, objects, "apps", "api")

	if report.Verdict != SchedulingScheduled || !report.Scheduled {
		t.Fatalf("verdict = %q, want scheduled", report.Verdict)
	}
	if len(report.Nodes) != 0 || report.NodesFit != 0 {
		t.Fatalf("a scheduled Pod needs no node simulation: %+v", report.Nodes)
	}
	if !strings.Contains(report.Headline, "ImagePullBackOff") {
		t.Fatalf("headline = %q", report.Headline)
	}
	finding := report.Findings[0]
	if finding.Severity != SeverityCritical || !strings.Contains(finding.Detail, "imagePullSecrets") {
		t.Fatalf("finding does not explain the pull failure: %+v", finding)
	}
	if !strings.Contains(strings.Join(report.Limits, " "), "already has a node") {
		t.Fatalf("limits should say scheduling is over: %v", report.Limits)
	}
}

func TestExplainSchedulingReportsTheSchedulersOwnMessage(t *testing.T) {
	pinChecksNow(t)
	objects := []runtime.Object{
		schedNode("node-1", "8", "16Gi"),
		schedPod("apps", "api", func(pod *corev1.Pod) {
			pod.Status.Conditions = []corev1.PodCondition{{
				Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Reason: "Unschedulable",
				Message: "0/1 nodes are available: 1 node(s) didn't match Pod's node affinity/selector.",
			}}
		}),
	}
	report := explain(t, objects, "apps", "api")

	if len(report.Findings) == 0 || !strings.Contains(report.Findings[0].Title, "The scheduler reports: Unschedulable") {
		t.Fatalf("the scheduler's own verdict must come first: %+v", report.Findings)
	}
	if !strings.Contains(report.Findings[0].Detail, "didn't match Pod's node affinity") {
		t.Fatalf("the scheduler's message must be kept verbatim: %+v", report.Findings[0])
	}
	// Kubby's own simulation disagrees here (the node does fit an empty Pod);
	// that disagreement is reported as "fits", never as a claim the scheduler is wrong.
	if report.Verdict != SchedulingFits {
		t.Fatalf("verdict = %q", report.Verdict)
	}
}

func TestExplainSchedulingNoNodes(t *testing.T) {
	pinChecksNow(t)
	report := explain(t, []runtime.Object{schedPod("apps", "api")}, "apps", "api")
	if report.Verdict != SchedulingUnknown {
		t.Fatalf("verdict = %q, want unknown", report.Verdict)
	}
	if len(report.Findings) == 0 || report.Findings[0].Severity != SeverityWarning {
		t.Fatalf("an empty node list should be a warning, not a verdict: %+v", report.Findings)
	}
}

// The request count must not grow with the cluster: the whole simulation runs
// on one node list and one pod list, however many nodes there are.
func TestExplainSchedulingRequestCountIsIndependentOfClusterSize(t *testing.T) {
	pinChecksNow(t)
	counts := map[int]int{}
	for _, nodes := range []int{3, 200} {
		objects := []runtime.Object{schedPod("apps", "api", withRequests("500m", "1Gi"))}
		for i := 0; i < nodes; i++ {
			objects = append(objects, schedNode(fmt.Sprintf("node-%03d", i), "4", "8Gi"))
		}
		client := kubefake.NewSimpleClientset(objects...)
		cluster := &Cluster{Clientset: client}
		if _, err := ExplainScheduling(context.Background(), cluster, "apps", "api"); err != nil {
			t.Fatalf("ExplainScheduling: %v", err)
		}
		counts[nodes] = len(client.Actions())
	}
	if counts[3] != counts[200] {
		t.Fatalf("requests grew with the cluster: %d nodes → %d requests, %d nodes → %d requests",
			3, counts[3], 200, counts[200])
	}
	if counts[3] != 4 {
		t.Fatalf("expected the Pod, the nodes, the Pods and the events — 4 requests, got %d", counts[3])
	}
}

func TestEffectivePodRequestsCountsInitContainersAndOverhead(t *testing.T) {
	always := corev1.ContainerRestartPolicyAlways
	pod := &corev1.Pod{Spec: corev1.PodSpec{
		Containers: []corev1.Container{{Name: "main", Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{corev1.ResourceCPU: qty("500m"), corev1.ResourceMemory: qty("1Gi")}}}},
		InitContainers: []corev1.Container{
			// A sidecar runs alongside the main containers, so it adds.
			{Name: "proxy", RestartPolicy: &always, Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceCPU: qty("100m")}}},
			// A plain init container runs before them, so the Pod needs the
			// larger of the two phases rather than their sum.
			{Name: "migrate", Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceCPU: qty("2"), corev1.ResourceMemory: qty("256Mi")}}},
		},
		Overhead: corev1.ResourceList{corev1.ResourceMemory: qty("64Mi")},
	}}
	amounts := resourceAmounts(effectivePodRequests(pod))

	// init 2000m + sidecar 100m = 2100m beats main 500m + sidecar 100m.
	if amounts[corev1.ResourceCPU] != 2100 {
		t.Fatalf("cpu = %dm, want 2100m", amounts[corev1.ResourceCPU])
	}
	// Memory: main 1Gi + overhead 64Mi wins over the init container's 256Mi.
	if want := int64(1024+64) << 20; amounts[corev1.ResourceMemory] != want {
		t.Fatalf("memory = %d, want %d", amounts[corev1.ResourceMemory], want)
	}
}
