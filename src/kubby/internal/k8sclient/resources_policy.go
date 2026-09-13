package k8sclient

import (
	"context"
	"fmt"
	"sort"
	"strings"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Scaling, disruption and network policy are built-in kinds, not CRDs, so the
// discovery-backed Custom Resources group never lists them. Each needs its own
// section and, more importantly, its own status: an HPA that cannot read
// metrics, a PDB that allows no evictions and a policy that denies all ingress
// look identical in a generic Name/Age table.

type HPAInfo struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Target    string `json:"target"` // "Deployment/web"
	Replicas  string `json:"replicas"`
	MinMax    string `json:"minMax"`
	Metrics   string `json:"metrics"` // "cpu 45%/80%, memory <unknown>/512Mi"
	Status    string `json:"status"`
	IsError   bool   `json:"isError"`
	Age       string `json:"age"`
}

type PDBInfo struct {
	Namespace          string `json:"namespace"`
	Name               string `json:"name"`
	Budget             string `json:"budget"` // "minAvailable 2" | "maxUnavailable 25%"
	AllowedDisruptions int32  `json:"allowedDisruptions"`
	Healthy            string `json:"healthy"` // "current/desired"
	Status             string `json:"status"`
	IsError            bool   `json:"isError"`
	Age                string `json:"age"`
}

type NetworkPolicyInfo struct {
	Namespace   string `json:"namespace"`
	Name        string `json:"name"`
	PodSelector string `json:"podSelector"`
	PolicyTypes string `json:"policyTypes"`
	Effect      string `json:"effect"` // "Denies all ingress · 2 egress rules"
	Age         string `json:"age"`
}

func ListHorizontalPodAutoscalers(ctx context.Context, client kubernetes.Interface, ns string) ([]HPAInfo, error) {
	list, err := client.AutoscalingV2().HorizontalPodAutoscalers(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]HPAInfo, 0, len(list.Items))
	for i := range list.Items {
		hpa := &list.Items[i]
		status, isError := hpaStatus(hpa)
		out = append(out, HPAInfo{
			Namespace: hpa.Namespace, Name: hpa.Name,
			Target:   hpa.Spec.ScaleTargetRef.Kind + "/" + hpa.Spec.ScaleTargetRef.Name,
			Replicas: hpaReplicas(hpa),
			MinMax:   fmt.Sprintf("%d–%d", hpaMinReplicas(hpa), hpa.Spec.MaxReplicas),
			Metrics:  strings.Join(hpaMetricLines(hpa), ", "),
			Status:   status, IsError: isError,
			Age: age(hpa.CreationTimestamp),
		})
	}
	return out, nil
}

func hpaMinReplicas(hpa *autoscalingv2.HorizontalPodAutoscaler) int32 {
	if hpa.Spec.MinReplicas != nil {
		return *hpa.Spec.MinReplicas
	}
	return 1
}

func hpaReplicas(hpa *autoscalingv2.HorizontalPodAutoscaler) string {
	if hpa.Status.DesiredReplicas != hpa.Status.CurrentReplicas {
		return fmt.Sprintf("%d → %d", hpa.Status.CurrentReplicas, hpa.Status.DesiredReplicas)
	}
	return fmt.Sprintf("%d", hpa.Status.CurrentReplicas)
}

// hpaStatus reports the one condition worth reading first. An HPA that cannot
// compute a replica count (ScalingActive=False, usually missing metrics) or
// cannot update its target (AbleToScale=False) is silently not autoscaling,
// which is the failure people discover only under load.
func hpaStatus(hpa *autoscalingv2.HorizontalPodAutoscaler) (string, bool) {
	limited := ""
	for _, condition := range hpa.Status.Conditions {
		switch {
		case condition.Type == autoscalingv2.ScalingActive && condition.Status == corev1.ConditionFalse:
			return conditionText("Not scaling", condition.Reason), true
		case condition.Type == autoscalingv2.AbleToScale && condition.Status == corev1.ConditionFalse:
			return conditionText("Unable to scale", condition.Reason), true
		case condition.Type == autoscalingv2.ScalingLimited && condition.Status == corev1.ConditionTrue:
			limited = conditionText("Limited", condition.Reason)
		}
	}
	if limited != "" {
		return limited, false
	}
	if len(hpa.Status.Conditions) == 0 {
		return "Awaiting first reconcile", false
	}
	return "Scaling", false
}

func conditionText(prefix, reason string) string {
	if reason == "" {
		return prefix
	}
	return prefix + ": " + reason
}

// hpaMetricLines renders each metric like kubectl's TARGETS column, pairing
// spec and status by source type and name rather than by list position.
func hpaMetricLines(hpa *autoscalingv2.HorizontalPodAutoscaler) []string {
	lines := make([]string, 0, len(hpa.Spec.Metrics))
	for _, spec := range hpa.Spec.Metrics {
		label, target := hpaMetricSpec(spec)
		current := "<unknown>"
		for _, status := range hpa.Status.CurrentMetrics {
			if value, ok := hpaMetricCurrent(spec, status); ok {
				current = value
				break
			}
		}
		lines = append(lines, fmt.Sprintf("%s %s/%s", label, current, target))
	}
	return lines
}

func hpaMetricSpec(spec autoscalingv2.MetricSpec) (string, string) {
	switch spec.Type {
	case autoscalingv2.ResourceMetricSourceType:
		if spec.Resource != nil {
			return string(spec.Resource.Name), metricTargetText(spec.Resource.Target)
		}
	case autoscalingv2.ContainerResourceMetricSourceType:
		if spec.ContainerResource != nil {
			return fmt.Sprintf("%s(%s)", spec.ContainerResource.Name, spec.ContainerResource.Container), metricTargetText(spec.ContainerResource.Target)
		}
	case autoscalingv2.PodsMetricSourceType:
		if spec.Pods != nil {
			return spec.Pods.Metric.Name, metricTargetText(spec.Pods.Target)
		}
	case autoscalingv2.ObjectMetricSourceType:
		if spec.Object != nil {
			return fmt.Sprintf("%s on %s/%s", spec.Object.Metric.Name, spec.Object.DescribedObject.Kind, spec.Object.DescribedObject.Name), metricTargetText(spec.Object.Target)
		}
	case autoscalingv2.ExternalMetricSourceType:
		if spec.External != nil {
			return spec.External.Metric.Name, metricTargetText(spec.External.Target)
		}
	}
	return string(spec.Type), "<unknown>"
}

func hpaMetricCurrent(spec autoscalingv2.MetricSpec, status autoscalingv2.MetricStatus) (string, bool) {
	if spec.Type != status.Type {
		return "", false
	}
	switch spec.Type {
	case autoscalingv2.ResourceMetricSourceType:
		if spec.Resource != nil && status.Resource != nil && spec.Resource.Name == status.Resource.Name {
			return metricValueText(status.Resource.Current), true
		}
	case autoscalingv2.ContainerResourceMetricSourceType:
		if spec.ContainerResource != nil && status.ContainerResource != nil &&
			spec.ContainerResource.Name == status.ContainerResource.Name && spec.ContainerResource.Container == status.ContainerResource.Container {
			return metricValueText(status.ContainerResource.Current), true
		}
	case autoscalingv2.PodsMetricSourceType:
		if spec.Pods != nil && status.Pods != nil && spec.Pods.Metric.Name == status.Pods.Metric.Name {
			return metricValueText(status.Pods.Current), true
		}
	case autoscalingv2.ObjectMetricSourceType:
		if spec.Object != nil && status.Object != nil && spec.Object.Metric.Name == status.Object.Metric.Name &&
			spec.Object.DescribedObject.Name == status.Object.DescribedObject.Name {
			return metricValueText(status.Object.Current), true
		}
	case autoscalingv2.ExternalMetricSourceType:
		if spec.External != nil && status.External != nil && spec.External.Metric.Name == status.External.Metric.Name {
			return metricValueText(status.External.Current), true
		}
	}
	return "", false
}

func metricTargetText(target autoscalingv2.MetricTarget) string {
	switch {
	case target.AverageUtilization != nil:
		return fmt.Sprintf("%d%%", *target.AverageUtilization)
	case target.AverageValue != nil:
		return quantityText(target.AverageValue)
	case target.Value != nil:
		return quantityText(target.Value)
	}
	return "<unknown>"
}

func metricValueText(value autoscalingv2.MetricValueStatus) string {
	switch {
	case value.AverageUtilization != nil:
		return fmt.Sprintf("%d%%", *value.AverageUtilization)
	case value.AverageValue != nil:
		return quantityText(value.AverageValue)
	case value.Value != nil:
		return quantityText(value.Value)
	}
	return "<unknown>"
}

func quantityText(q *resource.Quantity) string {
	if q == nil {
		return "<unknown>"
	}
	return q.String()
}

func ListPodDisruptionBudgets(ctx context.Context, client kubernetes.Interface, ns string) ([]PDBInfo, error) {
	list, err := client.PolicyV1().PodDisruptionBudgets(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]PDBInfo, 0, len(list.Items))
	for i := range list.Items {
		pdb := &list.Items[i]
		status, isError := pdbStatus(pdb)
		out = append(out, PDBInfo{
			Namespace: pdb.Namespace, Name: pdb.Name,
			Budget:             pdbBudget(pdb),
			AllowedDisruptions: pdb.Status.DisruptionsAllowed,
			Healthy:            fmt.Sprintf("%d/%d", pdb.Status.CurrentHealthy, pdb.Status.DesiredHealthy),
			Status:             status, IsError: isError,
			Age: age(pdb.CreationTimestamp),
		})
	}
	return out, nil
}

func pdbBudget(pdb *policyv1.PodDisruptionBudget) string {
	parts := []string{}
	if pdb.Spec.MinAvailable != nil {
		parts = append(parts, "minAvailable "+pdb.Spec.MinAvailable.String())
	}
	if pdb.Spec.MaxUnavailable != nil {
		parts = append(parts, "maxUnavailable "+pdb.Spec.MaxUnavailable.String())
	}
	if len(parts) == 0 {
		return "—"
	}
	return strings.Join(parts, ", ")
}

// pdbStatus flags a budget that currently allows no voluntary disruption while
// it covers Pods. That is exactly when `kubectl drain`, a node upgrade or a
// cluster-autoscaler scale-down stalls — including the common permanent case of
// minAvailable equal to the replica count.
func pdbStatus(pdb *policyv1.PodDisruptionBudget) (string, bool) {
	switch {
	case pdb.Status.ObservedGeneration < pdb.Generation:
		return "Awaiting controller", false
	case pdb.Status.ExpectedPods == 0:
		return "No matching Pods", false
	case pdb.Status.CurrentHealthy < pdb.Status.DesiredHealthy:
		return fmt.Sprintf("Below desired healthy (%d/%d) — evictions blocked", pdb.Status.CurrentHealthy, pdb.Status.DesiredHealthy), true
	case pdb.Status.DisruptionsAllowed == 0:
		return "Allows no disruptions — drains will block", true
	}
	return fmt.Sprintf("%d disruption(s) allowed", pdb.Status.DisruptionsAllowed), false
}

func ListNetworkPolicies(ctx context.Context, client kubernetes.Interface, ns string) ([]NetworkPolicyInfo, error) {
	list, err := client.NetworkingV1().NetworkPolicies(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]NetworkPolicyInfo, 0, len(list.Items))
	for i := range list.Items {
		policy := &list.Items[i]
		ingress, egress := networkPolicyTypes(policy)
		types := []string{}
		if ingress {
			types = append(types, "Ingress")
		}
		if egress {
			types = append(types, "Egress")
		}
		out = append(out, NetworkPolicyInfo{
			Namespace: policy.Namespace, Name: policy.Name,
			PodSelector: podSelectorText(policy.Spec.PodSelector),
			PolicyTypes: strings.Join(types, ", "),
			Effect:      networkPolicyEffect(policy),
			Age:         age(policy.CreationTimestamp),
		})
	}
	return out, nil
}

// networkPolicyTypes applies the API defaulting rule: without explicit
// policyTypes a policy always isolates ingress, and isolates egress only when
// it carries egress rules.
func networkPolicyTypes(policy *netv1.NetworkPolicy) (ingress, egress bool) {
	if len(policy.Spec.PolicyTypes) == 0 {
		return true, len(policy.Spec.Egress) > 0
	}
	for _, policyType := range policy.Spec.PolicyTypes {
		switch policyType {
		case netv1.PolicyTypeIngress:
			ingress = true
		case netv1.PolicyTypeEgress:
			egress = true
		}
	}
	return ingress, egress
}

// networkPolicyEffect names the two shapes that matter most when debugging —
// deny-all and allow-all — instead of leaving the reader to count rules.
func networkPolicyEffect(policy *netv1.NetworkPolicy) string {
	ingress, egress := networkPolicyTypes(policy)
	parts := []string{}
	if ingress {
		parts = append(parts, directionEffect("ingress", len(policy.Spec.Ingress), ingressAllowsAll(policy.Spec.Ingress)))
	}
	if egress {
		parts = append(parts, directionEffect("egress", len(policy.Spec.Egress), egressAllowsAll(policy.Spec.Egress)))
	}
	return strings.Join(parts, " · ")
}

func directionEffect(direction string, rules int, allowsAll bool) string {
	switch {
	case rules == 0:
		return "Denies all " + direction
	case allowsAll:
		return "Allows all " + direction
	case rules == 1:
		return "1 " + direction + " rule"
	}
	return fmt.Sprintf("%d %s rules", rules, direction)
}

func ingressAllowsAll(rules []netv1.NetworkPolicyIngressRule) bool {
	for _, rule := range rules {
		if len(rule.From) == 0 && len(rule.Ports) == 0 {
			return true
		}
	}
	return false
}

func egressAllowsAll(rules []netv1.NetworkPolicyEgressRule) bool {
	for _, rule := range rules {
		if len(rule.To) == 0 && len(rule.Ports) == 0 {
			return true
		}
	}
	return false
}

// podSelectorText distinguishes the empty selector (every Pod in the
// namespace), which metav1.FormatLabelSelector renders the same as "none".
func podSelectorText(selector metav1.LabelSelector) string {
	if len(selector.MatchLabels) == 0 && len(selector.MatchExpressions) == 0 {
		return "all Pods"
	}
	return labelSelectorText(&selector)
}

func labelSelectorText(selector *metav1.LabelSelector) string {
	if selector == nil {
		return ""
	}
	if len(selector.MatchLabels) == 0 && len(selector.MatchExpressions) == 0 {
		return "all"
	}
	parts := make([]string, 0, len(selector.MatchLabels)+len(selector.MatchExpressions))
	keys := make([]string, 0, len(selector.MatchLabels))
	for key := range selector.MatchLabels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		parts = append(parts, key+"="+selector.MatchLabels[key])
	}
	for _, expression := range selector.MatchExpressions {
		values := ""
		if len(expression.Values) > 0 {
			values = " (" + strings.Join(expression.Values, ",") + ")"
		}
		parts = append(parts, fmt.Sprintf("%s %s%s", expression.Key, strings.ToLower(string(expression.Operator)), values))
	}
	return strings.Join(parts, ", ")
}

// networkPolicyRuleLines renders every rule for the drawer, one line per rule.
func networkPolicyRuleLines(policy *netv1.NetworkPolicy) []DetailField {
	fields := []DetailField{}
	ingress, egress := networkPolicyTypes(policy)
	if ingress && len(policy.Spec.Ingress) == 0 {
		fields = append(fields, DetailField{"Ingress", "no rules — all ingress to the selected Pods is denied"})
	}
	for i, rule := range policy.Spec.Ingress {
		fields = append(fields, DetailField{fmt.Sprintf("Ingress rule %d", i+1),
			fmt.Sprintf("from %s on %s", peersText(rule.From), portsText(rule.Ports))})
	}
	if egress && len(policy.Spec.Egress) == 0 {
		fields = append(fields, DetailField{"Egress", "no rules — all egress from the selected Pods is denied"})
	}
	for i, rule := range policy.Spec.Egress {
		fields = append(fields, DetailField{fmt.Sprintf("Egress rule %d", i+1),
			fmt.Sprintf("to %s on %s", peersText(rule.To), portsText(rule.Ports))})
	}
	return fields
}

func peersText(peers []netv1.NetworkPolicyPeer) string {
	if len(peers) == 0 {
		return "anywhere"
	}
	parts := make([]string, 0, len(peers))
	for _, peer := range peers {
		switch {
		case peer.IPBlock != nil:
			text := "ipBlock " + peer.IPBlock.CIDR
			if len(peer.IPBlock.Except) > 0 {
				text += " except " + strings.Join(peer.IPBlock.Except, ", ")
			}
			parts = append(parts, text)
		case peer.NamespaceSelector != nil && peer.PodSelector != nil:
			parts = append(parts, fmt.Sprintf("Pods [%s] in namespaces [%s]", labelSelectorText(peer.PodSelector), labelSelectorText(peer.NamespaceSelector)))
		case peer.NamespaceSelector != nil:
			parts = append(parts, fmt.Sprintf("namespaces [%s]", labelSelectorText(peer.NamespaceSelector)))
		case peer.PodSelector != nil:
			parts = append(parts, fmt.Sprintf("Pods [%s] in this namespace", labelSelectorText(peer.PodSelector)))
		}
	}
	return strings.Join(parts, "; ")
}

func portsText(ports []netv1.NetworkPolicyPort) string {
	if len(ports) == 0 {
		return "all ports"
	}
	parts := make([]string, 0, len(ports))
	for _, port := range ports {
		protocol := corev1.ProtocolTCP
		if port.Protocol != nil {
			protocol = *port.Protocol
		}
		switch {
		case port.Port == nil:
			parts = append(parts, "all "+string(protocol))
		case port.EndPort != nil:
			parts = append(parts, fmt.Sprintf("%s-%d/%s", port.Port.String(), *port.EndPort, protocol))
		default:
			parts = append(parts, port.Port.String()+"/"+string(protocol))
		}
	}
	return strings.Join(parts, ", ")
}
