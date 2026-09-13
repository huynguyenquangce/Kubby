package k8sclient

import (
	"errors"
	"strings"
	"testing"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestHPAStatusFlagsOnlyInabilityToScale(t *testing.T) {
	tests := []struct {
		name       string
		conditions []autoscalingv2.HorizontalPodAutoscalerCondition
		want       string
		wantError  bool
	}{
		{name: "metrics missing", conditions: []autoscalingv2.HorizontalPodAutoscalerCondition{
			{Type: autoscalingv2.AbleToScale, Status: corev1.ConditionTrue},
			{Type: autoscalingv2.ScalingActive, Status: corev1.ConditionFalse, Reason: "FailedGetResourceMetric"},
		}, want: "Not scaling: FailedGetResourceMetric", wantError: true},
		{name: "cannot update target", conditions: []autoscalingv2.HorizontalPodAutoscalerCondition{
			{Type: autoscalingv2.AbleToScale, Status: corev1.ConditionFalse, Reason: "FailedGetScale"},
		}, want: "Unable to scale: FailedGetScale", wantError: true},
		{name: "at max is informational", conditions: []autoscalingv2.HorizontalPodAutoscalerCondition{
			{Type: autoscalingv2.ScalingActive, Status: corev1.ConditionTrue},
			{Type: autoscalingv2.ScalingLimited, Status: corev1.ConditionTrue, Reason: "TooManyReplicas"},
		}, want: "Limited: TooManyReplicas"},
		{name: "new object", want: "Awaiting first reconcile"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hpa := &autoscalingv2.HorizontalPodAutoscaler{Status: autoscalingv2.HorizontalPodAutoscalerStatus{Conditions: tc.conditions}}
			got, isError := hpaStatus(hpa)
			if got != tc.want || isError != tc.wantError {
				t.Fatalf("hpaStatus = %q, %v; want %q, %v", got, isError, tc.want, tc.wantError)
			}
		})
	}
}

func TestHPAMetricLinesPairSpecAndStatusByName(t *testing.T) {
	utilization := int32(80)
	memoryTarget := resource.MustParse("512Mi")
	memoryCurrent := resource.MustParse("300Mi")
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{Metrics: []autoscalingv2.MetricSpec{
			{Type: autoscalingv2.ResourceMetricSourceType, Resource: &autoscalingv2.ResourceMetricSource{
				Name: corev1.ResourceCPU, Target: autoscalingv2.MetricTarget{Type: autoscalingv2.UtilizationMetricType, AverageUtilization: &utilization}}},
			{Type: autoscalingv2.ResourceMetricSourceType, Resource: &autoscalingv2.ResourceMetricSource{
				Name: corev1.ResourceMemory, Target: autoscalingv2.MetricTarget{Type: autoscalingv2.AverageValueMetricType, AverageValue: &memoryTarget}}},
		}},
		// Status lists only memory, and in a different position than the spec.
		Status: autoscalingv2.HorizontalPodAutoscalerStatus{CurrentMetrics: []autoscalingv2.MetricStatus{
			{Type: autoscalingv2.ResourceMetricSourceType, Resource: &autoscalingv2.ResourceMetricStatus{
				Name: corev1.ResourceMemory, Current: autoscalingv2.MetricValueStatus{AverageValue: &memoryCurrent}}},
		}},
	}
	got := strings.Join(hpaMetricLines(hpa), ", ")
	if got != "cpu <unknown>/80%, memory 300Mi/512Mi" {
		t.Fatalf("metric lines = %q", got)
	}
}

func TestPDBStatusFlagsBudgetsThatBlockEviction(t *testing.T) {
	tests := []struct {
		name      string
		status    policyv1.PodDisruptionBudgetStatus
		wantError bool
		contains  string
	}{
		{name: "healthy with headroom", status: policyv1.PodDisruptionBudgetStatus{ExpectedPods: 3, CurrentHealthy: 3, DesiredHealthy: 2, DisruptionsAllowed: 1}, contains: "1 disruption"},
		{name: "minAvailable equals replicas", status: policyv1.PodDisruptionBudgetStatus{ExpectedPods: 2, CurrentHealthy: 2, DesiredHealthy: 2}, wantError: true, contains: "drains will block"},
		{name: "unhealthy", status: policyv1.PodDisruptionBudgetStatus{ExpectedPods: 2, CurrentHealthy: 1, DesiredHealthy: 2}, wantError: true, contains: "Below desired healthy"},
		{name: "selects nothing", status: policyv1.PodDisruptionBudgetStatus{}, contains: "No matching Pods"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, isError := pdbStatus(&policyv1.PodDisruptionBudget{Status: tc.status})
			if isError != tc.wantError || !strings.Contains(got, tc.contains) {
				t.Fatalf("pdbStatus = %q, %v; want error=%v containing %q", got, isError, tc.wantError, tc.contains)
			}
		})
	}
}

func TestNetworkPolicyEffectAppliesPolicyTypeDefaulting(t *testing.T) {
	tests := []struct {
		name   string
		policy netv1.NetworkPolicy
		want   string
	}{
		{name: "empty policy denies ingress", want: "Denies all ingress"},
		{name: "egress rules imply egress type", policy: netv1.NetworkPolicy{Spec: netv1.NetworkPolicySpec{
			Egress: []netv1.NetworkPolicyEgressRule{{To: []netv1.NetworkPolicyPeer{{IPBlock: &netv1.IPBlock{CIDR: "0.0.0.0/0"}}}}},
		}}, want: "Denies all ingress · 1 egress rule"},
		{name: "empty rule allows all", policy: netv1.NetworkPolicy{Spec: netv1.NetworkPolicySpec{
			PolicyTypes: []netv1.PolicyType{netv1.PolicyTypeIngress},
			Ingress:     []netv1.NetworkPolicyIngressRule{{}},
		}}, want: "Allows all ingress"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := networkPolicyEffect(&tc.policy); got != tc.want {
				t.Fatalf("effect = %q; want %q", got, tc.want)
			}
		})
	}
	if got := podSelectorText(metav1.LabelSelector{}); got != "all Pods" {
		t.Fatalf("empty pod selector = %q; want all Pods", got)
	}
}

func TestPreviousLogsErrorExplainsMissingPreviousInstance(t *testing.T) {
	kubelet := errors.New(`previous terminated container "api" in pod "api-1" not found`)
	if got := previousLogsError("api", kubelet).Error(); !strings.Contains(got, `Container "api" has no previous instance`) {
		t.Fatalf("friendly error = %q", got)
	}
	other := errors.New("pods \"api-1\" is forbidden")
	if got := previousLogsError("api", other); got != other {
		t.Fatalf("unrelated errors must pass through unchanged, got %v", got)
	}
}
