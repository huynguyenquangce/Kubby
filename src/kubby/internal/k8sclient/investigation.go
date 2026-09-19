package k8sclient

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
)

const (
	incidentMaxEvents   = 20
	incidentMaxTimeline = 30
)

// IncidentReport is a bounded, evidence-first diagnosis of one live resource.
// It intentionally contains no raw manifest, Secret value or application log,
// which also makes it safe to pass to FormatIncidentMarkdown for local export.
type IncidentReport struct {
	Kind        string            `json:"kind"`
	Namespace   string            `json:"namespace"`
	Name        string            `json:"name"`
	State       string            `json:"state"` // healthy | warning | critical
	Summary     string            `json:"summary"`
	ObservedAt  string            `json:"observedAt"`
	Findings    []IncidentFinding `json:"findings"`
	Timeline    []IncidentMoment  `json:"timeline"`
	Related     []IncidentRelated `json:"related"`
	Actions     []IncidentAction  `json:"actions"`
	Limitations []string          `json:"limitations"`
}

type IncidentFinding struct {
	ID          string   `json:"id"`
	Severity    string   `json:"severity"`
	Title       string   `json:"title"`
	Explanation string   `json:"explanation"`
	Confidence  string   `json:"confidence"`
	Evidence    []string `json:"evidence"`
}

type IncidentMoment struct {
	At       string `json:"at"`
	Age      string `json:"age"`
	Source   string `json:"source"`
	Title    string `json:"title"`
	Detail   string `json:"detail"`
	Severity string `json:"severity"`
}

type IncidentRelated struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	Status    string `json:"status"`
}

// Action is a frontend navigation intent, not a Kubernetes mutation. Any write
// it leads to remains owned by the existing confirmation-protected workflow.
type IncidentAction struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Action      string `json:"action"`
	Kind        string `json:"kind"`
	Namespace   string `json:"namespace"`
	Name        string `json:"name"`
}

// InvestigateResource builds one coherent report behind one frontend-bound
// call. Pods receive the deep playbook; every other kind gets a truthful generic
// Event-based report while more playbooks are added.
func InvestigateResource(ctx context.Context, c *Cluster, kind, namespace, name string) (*IncidentReport, error) {
	if bareKind(kind) == "Pod" {
		return investigatePod(ctx, c, namespace, name)
	}
	return investigateGeneric(ctx, c, kind, namespace, name)
}

func investigateGeneric(ctx context.Context, c *Cluster, kind, namespace, name string) (*IncidentReport, error) {
	detail, err := GetDetail(ctx, c, kind, namespace, name)
	if err != nil {
		return nil, err
	}
	report := newIncidentReport(kind, namespace, name)
	report.Related = []IncidentRelated{}
	report.Timeline = append(report.Timeline, incidentMoment(detail.Created, "Resource", "Resource created", fmt.Sprintf("%s %s became visible to the API server.", bareKind(kind), name), "info"))
	events, eventErr := ListEvents(ctx, c, kind, namespace, name)
	if eventErr != nil {
		report.Limitations = append(report.Limitations, "Events could not be read: "+eventErr.Error())
	}
	for i, event := range events {
		if i >= incidentMaxEvents {
			break
		}
		severity := "info"
		if event.IsWarn {
			severity = "warning"
			report.Findings = append(report.Findings, IncidentFinding{
				ID:          "event-" + stableID(event.Reason),
				Severity:    "warning",
				Title:       event.Reason,
				Explanation: "Kubernetes reported a warning for this resource.",
				Confidence:  "high",
				Evidence:    []string{event.Message, fmt.Sprintf("Observed %s; count %d", event.Age, event.Count)},
			})
		}
		report.Timeline = append(report.Timeline, IncidentMoment{Age: event.Age, Source: "Event", Title: event.Reason, Detail: event.Message, Severity: severity})
	}
	finalizeIncident(report)
	if len(report.Findings) == 0 {
		report.Summary = "No warning Events are currently retained for this resource."
		report.Limitations = append(report.Limitations, "This resource kind does not yet have a specialised deterministic playbook; the report is based on retained Kubernetes Events.")
	}
	report.Actions = []IncidentAction{
		incidentAction("yaml", "Review live YAML", "Inspect desired state and use the existing diff/confirmation flow for any change.", kind, namespace, name),
		incidentAction("ai", "Explain with AI", "Ask AI only after reviewing the deterministic evidence above.", kind, namespace, name),
	}
	return report, nil
}

func investigatePod(ctx context.Context, c *Cluster, namespace, name string) (*IncidentReport, error) {
	pod, err := c.Clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	report := newIncidentReport("Pod", namespace, name)
	report.Limitations = append(report.Limitations, "Kubby is agent-less: the timeline contains live object timestamps and Events still retained by the API server, not a complete historical audit log.")
	report.Timeline = append(report.Timeline, incidentMoment(pod.CreationTimestamp.Format(time.RFC3339), "Pod", "Pod created", fmt.Sprintf("Scheduled phase: %s", pod.Status.Phase), "info"))

	var (
		events       []corev1.Event
		eventErr     error
		services     []corev1.Service
		serviceErr   error
		slices       []discoveryv1.EndpointSlice
		sliceErr     error
		ownerRelated []IncidentRelated
		ownerErr     error
	)
	var wg sync.WaitGroup
	wg.Add(4)
	go func() {
		defer wg.Done()
		events, eventErr = incidentPodEvents(ctx, c, pod)
	}()
	go func() {
		defer wg.Done()
		list, err := c.Clientset.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
		serviceErr = err
		if err == nil {
			services = list.Items
		}
	}()
	go func() {
		defer wg.Done()
		list, err := c.Clientset.DiscoveryV1().EndpointSlices(namespace).List(ctx, metav1.ListOptions{})
		sliceErr = err
		if err == nil {
			slices = list.Items
		}
	}()
	go func() {
		defer wg.Done()
		ownerRelated, ownerErr = incidentPodOwners(ctx, c, pod)
	}()
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if eventErr != nil {
		report.Limitations = append(report.Limitations, "Pod Events could not be read: "+eventErr.Error())
	}
	if serviceErr != nil {
		report.Limitations = append(report.Limitations, "Services could not be correlated: "+serviceErr.Error())
	}
	if sliceErr != nil {
		report.Limitations = append(report.Limitations, "EndpointSlice readiness could not be correlated: "+sliceErr.Error())
	}
	if ownerErr != nil {
		report.Limitations = append(report.Limitations, "The complete workload owner chain could not be resolved: "+ownerErr.Error())
	}
	report.Related = append(report.Related, ownerRelated...)

	addPodConditionEvidence(report, pod)
	addContainerEvidence(report, pod)
	addPodEventEvidence(report, events)
	if serviceErr == nil {
		addPodServiceEvidence(report, pod, services, slices, sliceErr == nil)
	}
	// Phase-level evidence is the final safety net. Conditions, Events and
	// container statuses can be absent after eviction or termination, but a
	// non-healthy phase must never be interpreted as verified recovery.
	addPodPhaseEvidence(report, pod)
	finalizeIncident(report)
	report.Actions = incidentPodActions(report, pod)
	return report, nil
}

func addPodPhaseEvidence(report *IncidentReport, pod *corev1.Pod) {
	if pod.DeletionTimestamp != nil {
		return // addPodConditionEvidence already records termination
	}
	evidence := compactEvidence("phase: "+string(pod.Status.Phase), "reason: "+pod.Status.Reason, pod.Status.Message)
	appendFinding := func(id, severity, title, explanation string) {
		report.Findings = append(report.Findings, IncidentFinding{
			ID: id, Severity: severity, Title: title, Explanation: explanation,
			Confidence: "high", Evidence: evidence,
		})
	}
	switch pod.Status.Phase {
	case corev1.PodFailed:
		title := "Pod phase is Failed"
		if pod.Status.Reason == "Evicted" {
			title = "Pod was evicted"
		} else if pod.Status.Reason != "" {
			title = "Pod failed: " + pod.Status.Reason
		}
		appendFinding("pod-phase-failed", "critical", title, "Kubernetes reports this Pod as terminally failed; it cannot count as recovered without a new healthy Pod.")
	case corev1.PodPending:
		appendFinding("pod-phase-pending", "warning", "Pod is Pending", "The Pod has not reached a running, ready state yet.")
	case corev1.PodUnknown:
		appendFinding("pod-phase-unknown", "critical", "Pod state is Unknown", "Kubernetes cannot currently determine the Pod state, so recovery cannot be verified.")
	case corev1.PodRunning:
		if !podIsReady(pod) {
			appendFinding("pod-phase-not-ready", "warning", "Pod is running but not ready", "The process may be running, but Kubernetes has not admitted this Pod to ready serving endpoints.")
		}
	case corev1.PodSucceeded:
		// A successful terminal Pod is healthy for finite Job-style workloads.
	default:
		appendFinding("pod-phase-unreported", "warning", "Pod phase is not reported", "Kubernetes has not supplied enough state to verify recovery yet.")
	}
}

func newIncidentReport(kind, namespace, name string) *IncidentReport {
	return &IncidentReport{
		Kind: kind, Namespace: namespace, Name: name,
		State: "healthy", ObservedAt: time.Now().UTC().Format(time.RFC3339),
		Findings: []IncidentFinding{}, Timeline: []IncidentMoment{},
		Related: []IncidentRelated{}, Actions: []IncidentAction{}, Limitations: []string{},
	}
}

func addPodConditionEvidence(report *IncidentReport, pod *corev1.Pod) {
	if pod.DeletionTimestamp != nil {
		report.Findings = append(report.Findings, IncidentFinding{
			ID: "terminating", Severity: "warning", Title: "Pod is terminating",
			Explanation: "The Pod is being deleted and should not be treated as a live serving endpoint.", Confidence: "high",
			Evidence: []string{"deletionTimestamp: " + pod.DeletionTimestamp.UTC().Format(time.RFC3339)},
		})
	}
	for _, condition := range pod.Status.Conditions {
		severity := "info"
		if condition.Status != corev1.ConditionTrue && (condition.Type == corev1.PodReady || condition.Type == corev1.PodScheduled) {
			severity = "warning"
		}
		detail := strings.TrimSpace(strings.TrimSpace(condition.Reason) + ": " + strings.TrimSpace(condition.Message))
		detail = strings.Trim(detail, ": ")
		report.Timeline = append(report.Timeline, incidentMoment(condition.LastTransitionTime.Format(time.RFC3339), "Condition", string(condition.Type)+" = "+string(condition.Status), detail, severity))
		if condition.Type == corev1.PodScheduled && condition.Status == corev1.ConditionFalse {
			report.Findings = append(report.Findings, IncidentFinding{
				ID: "unscheduled", Severity: "critical", Title: "Pod cannot be scheduled",
				Explanation: "The scheduler has not found a node that satisfies this Pod's requirements.", Confidence: "high",
				Evidence: compactEvidence(condition.Reason, condition.Message),
			})
		}
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionFalse {
			report.Findings = append(report.Findings, IncidentFinding{
				ID: "pod-not-ready", Severity: "warning", Title: "Pod is not ready",
				Explanation: "Kubernetes is withholding this Pod from ready serving endpoints until its readiness condition succeeds.", Confidence: "high",
				Evidence: compactEvidence(condition.Reason, condition.Message),
			})
		}
	}
}

func addContainerEvidence(report *IncidentReport, pod *corev1.Pod) {
	statuses := append([]corev1.ContainerStatus{}, pod.Status.InitContainerStatuses...)
	statuses = append(statuses, pod.Status.ContainerStatuses...)
	for _, status := range statuses {
		if status.State.Waiting != nil && status.State.Waiting.Reason != "" {
			reason := status.State.Waiting.Reason
			severity := "critical"
			if reason == "ContainerCreating" || reason == "PodInitializing" {
				severity = "warning"
			}
			report.Findings = append(report.Findings, IncidentFinding{
				ID: "waiting-" + stableID(status.Name+"-"+reason), Severity: severity,
				Title:       fmt.Sprintf("%s is waiting: %s", status.Name, reason),
				Explanation: waitingExplanation(reason), Confidence: "high",
				Evidence: compactEvidence(status.State.Waiting.Message, fmt.Sprintf("restartCount: %d", status.RestartCount)),
			})
		}
		terminated := status.State.Terminated
		historicalTermination := false
		if terminated == nil || (terminated.ExitCode == 0 && terminated.Reason == "Completed") {
			terminated = status.LastTerminationState.Terminated
			historicalTermination = true
		}
		terminationIsRelevant := terminated != nil && (terminated.ExitCode != 0 || terminated.Reason == "OOMKilled") &&
			(!historicalTermination || !status.Ready || recentlyTerminated(terminated))
		if terminationIsRelevant {
			title := fmt.Sprintf("%s exited with code %d", status.Name, terminated.ExitCode)
			explanation := "The container process exited unsuccessfully; previous logs usually contain the closest application-level evidence."
			if terminated.Reason == "OOMKilled" {
				title = status.Name + " was OOMKilled"
				explanation = "The container exceeded its memory limit and the kernel terminated it."
			}
			report.Findings = append(report.Findings, IncidentFinding{
				ID: "terminated-" + stableID(status.Name), Severity: "critical", Title: title,
				Explanation: explanation, Confidence: "high",
				Evidence: compactEvidence("reason: "+terminated.Reason, terminated.Message, fmt.Sprintf("restartCount: %d", status.RestartCount)),
			})
			if !terminated.FinishedAt.IsZero() {
				report.Timeline = append(report.Timeline, incidentMoment(terminated.FinishedAt.Format(time.RFC3339), "Container", title, terminated.Message, "critical"))
			}
		} else if status.RestartCount > 0 && (!status.Ready || recentlyTerminated(status.LastTerminationState.Terminated)) {
			report.Findings = append(report.Findings, IncidentFinding{
				ID: "restarts-" + stableID(status.Name), Severity: "warning",
				Title:       fmt.Sprintf("%s restarted %d times", status.Name, status.RestartCount),
				Explanation: "Restarts show that this container has been unstable even if it is running now.", Confidence: "high",
				Evidence: []string{fmt.Sprintf("restartCount: %d", status.RestartCount)},
			})
		}
		if !status.Ready && status.State.Running != nil {
			report.Findings = append(report.Findings, IncidentFinding{
				ID: "not-ready-" + stableID(status.Name), Severity: "warning",
				Title:       status.Name + " is running but not ready",
				Explanation: "Traffic should not be sent to this container until its readiness condition succeeds.", Confidence: "high",
				Evidence: []string{"container state: Running", "ready: false"},
			})
		}
	}
}

func recentlyTerminated(terminated *corev1.ContainerStateTerminated) bool {
	if terminated == nil || terminated.FinishedAt.IsZero() {
		return false
	}
	return time.Since(terminated.FinishedAt.Time) <= 15*time.Minute
}

func addPodEventEvidence(report *IncidentReport, events []corev1.Event) {
	sort.Slice(events, func(i, j int) bool { return eventTime(events[i]).After(eventTime(events[j]).Time) })
	seenWarnings := map[string]bool{}
	for i, event := range events {
		if i >= incidentMaxEvents {
			break
		}
		severity := "info"
		if event.Type == corev1.EventTypeWarning {
			severity = "warning"
		}
		ts := eventTime(event)
		report.Timeline = append(report.Timeline, incidentMoment(ts.Format(time.RFC3339), "Event", event.Reason, event.Message, severity))
		if event.Type != corev1.EventTypeWarning || seenWarnings[event.Reason] {
			continue
		}
		seenWarnings[event.Reason] = true
		if findingMentions(report.Findings, event.Reason) {
			continue
		}
		report.Findings = append(report.Findings, IncidentFinding{
			ID: "event-" + stableID(event.Reason), Severity: "warning", Title: event.Reason,
			Explanation: "Kubernetes emitted a warning while reconciling this Pod.", Confidence: "high",
			Evidence: compactEvidence(event.Message, fmt.Sprintf("event count: %d", event.Count)),
		})
	}
}

func addPodServiceEvidence(report *IncidentReport, pod *corev1.Pod, services []corev1.Service, slices []discoveryv1.EndpointSlice, slicesAvailable bool) {
	podLabels := labels.Set(pod.Labels)
	for i := range services {
		service := &services[i]
		if len(service.Spec.Selector) == 0 {
			continue
		}
		selector := labels.SelectorFromSet(service.Spec.Selector)
		if !selector.Matches(podLabels) {
			continue
		}
		status := "selector matches"
		readyEndpoints, podEndpoint, podEndpointReady := incidentEndpointState(service.Name, pod, slices)
		if slicesAvailable {
			status = fmt.Sprintf("%d ready endpoints", readyEndpoints)
		}
		report.Related = append(report.Related, IncidentRelated{Kind: "Service", Namespace: service.Namespace, Name: service.Name, Role: "Selects this Pod", Status: status})
		if !slicesAvailable {
			continue
		}
		if readyEndpoints == 0 {
			evidence := []string{"Service selector: " + labels.FormatLabels(service.Spec.Selector), "EndpointSlice ready endpoints: 0"}
			if podEndpoint && !podEndpointReady {
				evidence = append(evidence, "This Pod is present in an EndpointSlice but is not ready")
			}
			report.Findings = append(report.Findings, IncidentFinding{
				ID: "service-no-endpoint-" + stableID(service.Name), Severity: "critical",
				Title:       service.Name + " has no ready endpoints",
				Explanation: "The Service selects this Pod, but Kubernetes currently has no ready backend for traffic.", Confidence: "high", Evidence: evidence,
			})
		} else if podEndpoint && !podEndpointReady {
			report.Findings = append(report.Findings, IncidentFinding{
				ID: "pod-not-endpoint-" + stableID(service.Name), Severity: "warning",
				Title:       "This Pod is not a ready endpoint for " + service.Name,
				Explanation: "Other backends may still serve traffic, but this Pod has been removed from the ready endpoint set.", Confidence: "high",
				Evidence: []string{fmt.Sprintf("Service has %d other ready endpoint(s)", readyEndpoints)},
			})
		}
	}
}

func incidentEndpointState(serviceName string, pod *corev1.Pod, slices []discoveryv1.EndpointSlice) (ready int, podPresent, podReady bool) {
	for i := range slices {
		slice := &slices[i]
		if slice.Labels[discoveryv1.LabelServiceName] != serviceName {
			continue
		}
		for _, endpoint := range slice.Endpoints {
			isReady := endpoint.Conditions.Ready == nil || *endpoint.Conditions.Ready
			isTerminating := endpoint.Conditions.Terminating != nil && *endpoint.Conditions.Terminating
			if isReady && !isTerminating {
				ready++
			}
			if endpoint.TargetRef != nil && endpoint.TargetRef.Kind == "Pod" &&
				(endpoint.TargetRef.UID == pod.UID || (endpoint.TargetRef.UID == "" && endpoint.TargetRef.Name == pod.Name)) {
				podPresent = true
				podReady = isReady && !isTerminating
			}
		}
	}
	return ready, podPresent, podReady
}

func incidentPodEvents(ctx context.Context, c *Cluster, pod *corev1.Pod) ([]corev1.Event, error) {
	selector := fields.Set{"involvedObject.kind": "Pod", "involvedObject.name": pod.Name}
	if pod.UID != "" {
		selector["involvedObject.uid"] = string(pod.UID)
	}
	list, err := c.Clientset.CoreV1().Events(pod.Namespace).List(ctx, metav1.ListOptions{FieldSelector: selector.AsSelector().String()})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func incidentPodOwners(ctx context.Context, c *Cluster, pod *corev1.Pod) ([]IncidentRelated, error) {
	result := []IncidentRelated{}
	for _, owner := range pod.OwnerReferences {
		result = append(result, IncidentRelated{Kind: owner.Kind, Namespace: pod.Namespace, Name: owner.Name, Role: "Owns this Pod", Status: "controller"})
		if owner.Kind != "ReplicaSet" {
			continue
		}
		rs, err := c.Clientset.AppsV1().ReplicaSets(pod.Namespace).Get(ctx, owner.Name, metav1.GetOptions{})
		if err != nil {
			return result, err
		}
		for _, rsOwner := range rs.OwnerReferences {
			if rsOwner.Kind == "Deployment" {
				result = append(result, IncidentRelated{Kind: "Deployment", Namespace: pod.Namespace, Name: rsOwner.Name, Role: "Rollout owner", Status: "controls ReplicaSet " + rs.Name})
			}
		}
	}
	return result, nil
}

func incidentPodActions(report *IncidentReport, pod *corev1.Pod) []IncidentAction {
	actions := []IncidentAction{}
	add := func(action IncidentAction) {
		for _, current := range actions {
			if current.ID == action.ID {
				return
			}
		}
		actions = append(actions, action)
	}
	combined := strings.ToLower(report.Summary)
	for _, finding := range report.Findings {
		combined += " " + strings.ToLower(finding.Title+" "+finding.Explanation)
	}
	if strings.Contains(combined, "crash") || strings.Contains(combined, "exit") || strings.Contains(combined, "restart") || strings.Contains(combined, "oom") {
		add(incidentAction("logs", "Inspect current and previous logs", "Confirm the closest application-level failure before changing desired state.", "Pod", pod.Namespace, pod.Name))
	}
	if strings.Contains(combined, "oom") {
		add(incidentAction("sizing", "Check memory sizing", "Compare live usage, requests and limits before choosing a new value.", "Pod", pod.Namespace, pod.Name))
	}
	if strings.Contains(combined, "image") || strings.Contains(combined, "probe") || strings.Contains(combined, "ready") || strings.Contains(combined, "schedule") {
		add(incidentAction("yaml", "Review desired state", "Inspect the live YAML; any edit still requires diff review and confirmation.", "Pod", pod.Namespace, pod.Name))
	}
	for _, related := range report.Related {
		if related.Kind == "Service" {
			add(incidentAction("topology", "Trace the traffic path", "See whether the route, Service or EndpointSlice hop is where traffic stops.", "Service", related.Namespace, related.Name))
		}
		if related.Kind == "Deployment" {
			add(incidentAction("rollout", "Review rollout and rollback", "Compare revisions and use the existing confirmation-protected rollback flow if appropriate.", related.Kind, related.Namespace, related.Name))
		}
	}
	add(incidentAction("ai", "Explain this evidence with AI", "Send the separately reviewed, redacted resource context to the configured provider.", "Pod", pod.Namespace, pod.Name))
	return actions
}

func finalizeIncident(report *IncidentReport) {
	sort.SliceStable(report.Findings, func(i, j int) bool {
		return severityRank(report.Findings[i].Severity) > severityRank(report.Findings[j].Severity)
	})
	report.Findings = uniqueFindings(report.Findings)
	sort.SliceStable(report.Timeline, func(i, j int) bool {
		left, leftErr := time.Parse(time.RFC3339, report.Timeline[i].At)
		right, rightErr := time.Parse(time.RFC3339, report.Timeline[j].At)
		if leftErr != nil {
			return false
		}
		if rightErr != nil {
			return true
		}
		return left.After(right)
	})
	if len(report.Timeline) > incidentMaxTimeline {
		report.Timeline = report.Timeline[:incidentMaxTimeline]
	}
	report.State = "healthy"
	for _, finding := range report.Findings {
		if finding.Severity == "critical" {
			report.State = "critical"
			break
		}
		if finding.Severity == "warning" {
			report.State = "warning"
		}
	}
	if len(report.Findings) > 0 {
		report.Summary = report.Findings[0].Title
	} else if report.Summary == "" {
		report.Summary = "No deterministic issue is visible in the current snapshot."
	}
}

func uniqueFindings(source []IncidentFinding) []IncidentFinding {
	seen := map[string]bool{}
	out := make([]IncidentFinding, 0, len(source))
	for _, finding := range source {
		if seen[finding.ID] {
			continue
		}
		seen[finding.ID] = true
		out = append(out, finding)
	}
	return out
}

func severityRank(severity string) int {
	switch severity {
	case "critical":
		return 3
	case "warning":
		return 2
	default:
		return 1
	}
}

func incidentMoment(at, source, title, detail, severity string) IncidentMoment {
	moment := IncidentMoment{At: at, Source: source, Title: title, Detail: strings.TrimSpace(detail), Severity: severity}
	if parsed, err := time.Parse(time.RFC3339, at); err == nil {
		moment.Age = age(metav1.NewTime(parsed))
	}
	return moment
}

func incidentAction(action, label, description, kind, namespace, name string) IncidentAction {
	return IncidentAction{ID: action + "-" + stableID(kind+"-"+namespace+"-"+name), Label: label, Description: description, Action: action, Kind: kind, Namespace: namespace, Name: name}
}

func stableID(value string) string {
	value = strings.ToLower(value)
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else if b.Len() > 0 && !strings.HasSuffix(b.String(), "-") {
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func compactEvidence(values ...string) []string {
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && value != "reason:" {
			out = append(out, trimTo(value, 400))
		}
	}
	return out
}

func waitingExplanation(reason string) string {
	switch reason {
	case "CrashLoopBackOff":
		return "The container repeatedly starts and exits; termination state and previous logs are the strongest next evidence."
	case "ImagePullBackOff", "ErrImagePull":
		return "The node cannot retrieve the configured image; verify the image reference, registry access and imagePullSecrets."
	case "CreateContainerConfigError":
		return "Kubernetes cannot construct the container configuration; referenced ConfigMaps, Secrets or keys are commonly missing."
	case "CreateContainerError":
		return "The container runtime refused to create the container; its message names the rejected part of the container spec."
	case "InvalidImageName":
		return "The image reference itself is not a valid name, so no registry is ever contacted."
	case "ContainerCreating", "PodInitializing":
		return "The container has not started yet; Events show whether image, volume or sandbox setup is blocking it."
	default:
		return "Kubernetes reports that this container cannot currently start."
	}
}

func findingMentions(findings []IncidentFinding, text string) bool {
	needle := strings.ToLower(text)
	for _, finding := range findings {
		if strings.Contains(strings.ToLower(finding.Title+" "+finding.Explanation), needle) {
			return true
		}
	}
	return false
}

// FormatIncidentMarkdown produces the local, shareable incident bundle. Keep
// this formatter constrained to IncidentReport; do not add manifests or logs
// here without applying the AI evidence redaction boundary first.
func FormatIncidentMarkdown(report *IncidentReport) string {
	if report == nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Kubby incident: %s/%s\n\n", report.Kind, report.Name)
	fmt.Fprintf(&b, "- Namespace: %s\n- State: %s\n- Observed: %s\n- Summary: %s\n\n", fallback(report.Namespace, "cluster-scoped"), report.State, report.ObservedAt, report.Summary)
	b.WriteString("## Findings\n\n")
	if len(report.Findings) == 0 {
		b.WriteString("No deterministic issue was visible in the captured snapshot.\n\n")
	}
	for _, finding := range report.Findings {
		fmt.Fprintf(&b, "### [%s] %s\n\n%s\n\nConfidence: %s\n\n", strings.ToUpper(finding.Severity), finding.Title, finding.Explanation, finding.Confidence)
		for _, evidence := range finding.Evidence {
			fmt.Fprintf(&b, "- Evidence: %s\n", evidence)
		}
		b.WriteString("\n")
	}
	if len(report.Related) > 0 {
		b.WriteString("## Related resources\n\n")
		for _, related := range report.Related {
			fmt.Fprintf(&b, "- %s %s/%s — %s (%s)\n", related.Kind, fallback(related.Namespace, "cluster-scoped"), related.Name, related.Role, related.Status)
		}
		b.WriteString("\n")
	}
	if len(report.Timeline) > 0 {
		b.WriteString("## Timeline\n\n")
		for _, moment := range report.Timeline {
			when := moment.At
			if when == "" {
				when = moment.Age
			}
			fmt.Fprintf(&b, "- %s · %s · %s", when, moment.Source, moment.Title)
			if moment.Detail != "" {
				fmt.Fprintf(&b, " — %s", moment.Detail)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	if len(report.Limitations) > 0 {
		b.WriteString("## Evidence limits\n\n")
		for _, limitation := range report.Limitations {
			fmt.Fprintf(&b, "- %s\n", limitation)
		}
	}
	return b.String()
}

func fallback(value, fallbackValue string) string {
	if value == "" {
		return fallbackValue
	}
	return value
}
