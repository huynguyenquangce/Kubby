package k8sclient

import (
	"context"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func nodePod(namespace, name string, labels map[string]string, owner *metav1.OwnerReference) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name, Labels: labels},
		Spec:       corev1.PodSpec{NodeName: "worker-1"},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}
	if owner != nil {
		pod.OwnerReferences = []metav1.OwnerReference{*owner}
	}
	return pod
}

func controllerRef(kind, name string) *metav1.OwnerReference {
	controller := true
	return &metav1.OwnerReference{Kind: kind, Name: name, Controller: &controller}
}

func TestDrainImpactNamesRefusingBudgetsUnmanagedPodsAndEmptyDir(t *testing.T) {
	cache := nodePod("shop", "cache", nil, nil)
	cache.Spec.Volumes = []corev1.Volume{{Name: "scratch", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}}
	finished := nodePod("shop", "migration", nil, nil)
	finished.Status.Phase = corev1.PodSucceeded
	mirror := nodePod("kube-system", "etcd", nil, nil)
	mirror.Annotations = map[string]string{corev1.MirrorPodAnnotationKey: "hash"}
	budget := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "api"},
		Spec:       policyv1.PodDisruptionBudgetSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}}},
		Status:     policyv1.PodDisruptionBudgetStatus{DisruptionsAllowed: 1},
	}
	client := kubefake.NewSimpleClientset(
		nodePod("shop", "api-1", map[string]string{"app": "api"}, controllerRef("ReplicaSet", "api-7d9")),
		nodePod("shop", "api-2", map[string]string{"app": "api"}, controllerRef("ReplicaSet", "api-7d9")),
		nodePod("kube-system", "proxy", nil, controllerRef("DaemonSet", "kube-proxy")),
		cache, finished, mirror, budget,
	)
	impact, err := DrainImpactFor(context.Background(), &Cluster{Clientset: client}, "worker-1")
	if err != nil {
		t.Fatal(err)
	}
	if impact.Evict != 4 || impact.DaemonSetPods != 1 || impact.MirrorPods != 1 {
		t.Fatalf("evict=%d daemonset=%d mirror=%d; want 4, 1, 1", impact.Evict, impact.DaemonSetPods, impact.MirrorPods)
	}
	if len(impact.Unmanaged) != 1 || impact.Unmanaged[0] != "shop/cache" {
		t.Fatalf("unmanaged = %v; a finished Pod needs no controller", impact.Unmanaged)
	}
	if len(impact.EmptyDir) != 1 || impact.EmptyDir[0] != "shop/cache" {
		t.Fatalf("emptyDir = %v", impact.EmptyDir)
	}
	if len(impact.BlockingBudgets) != 1 || impact.BlockingBudgets[0] != (DrainBudget{Namespace: "shop", Name: "api", AllowedDisruptions: 1, PodsOnNode: 2}) {
		t.Fatalf("blocking budgets = %+v; two covered Pods against one allowed disruption", impact.BlockingBudgets)
	}
}

func TestPodContainerStatesReportsPreviousInstance(t *testing.T) {
	finishedAt := metav1.NewTime(time.Now().Add(-3 * time.Minute))
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "api-1"},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "api"}, {Name: "sidecar"}}},
		Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{
			{Name: "api", RestartCount: 7, State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
				LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "OOMKilled", ExitCode: 137, FinishedAt: finishedAt}}},
			{Name: "sidecar", Ready: true, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
		}},
	}
	states, err := PodContainerStates(context.Background(), &Cluster{Clientset: kubefake.NewSimpleClientset(pod)}, "shop", "api-1")
	if err != nil {
		t.Fatal(err)
	}
	api := states[0]
	if api.Name != "api" || !api.HasPrevious || api.RestartCount != 7 || api.State != "Waiting: CrashLoopBackOff" ||
		api.LastTermination != "OOMKilled, exit 137" || api.LastTerminationAge != "3m" {
		t.Fatalf("api state = %+v", api)
	}
	if sidecar := states[1]; sidecar.HasPrevious || sidecar.State != "Running" || !sidecar.Ready {
		t.Fatalf("sidecar state = %+v", sidecar)
	}
}

func TestPolicyRelationTreesLinkTargetsAndPods(t *testing.T) {
	replicas := int32(3)
	client := kubefake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "api"},
			Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
			Status:     appsv1.DeploymentStatus{ReadyReplicas: 2},
		},
		&autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "api"},
			Spec:       autoscalingv2.HorizontalPodAutoscalerSpec{ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{Kind: "Deployment", Name: "api"}, MaxReplicas: 5},
		},
		&autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "orphan"},
			Spec:       autoscalingv2.HorizontalPodAutoscalerSpec{ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{Kind: "Deployment", Name: "gone"}, MaxReplicas: 5},
		},
		&policyv1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "api"},
			Spec:       policyv1.PodDisruptionBudgetSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}}},
		},
		trafficPolicy("shop", "api-ingress", map[string]string{"app": "api"}, []netv1.PolicyType{netv1.PolicyTypeIngress}, nil, nil),
		trafficPod("shop", "api-1", "10.0.0.3", map[string]string{"app": "api"}),
		trafficPod("shop", "web-1", "10.0.0.4", map[string]string{"app": "web"}),
	)
	cluster := &Cluster{Clientset: client}

	hpa, err := HPATree(context.Background(), cluster, "shop", "api")
	if err != nil || len(hpa.Children) != 1 || hpa.Children[0].Kind != "Deployment" || hpa.Children[0].Status != "2/3 ready" || !hpa.Children[0].IsError {
		t.Fatalf("HPA tree = %+v, err %v", hpa, err)
	}
	orphan, err := HPATree(context.Background(), cluster, "shop", "orphan")
	if err != nil || !orphan.Children[0].IsError {
		t.Fatalf("an HPA whose target is gone must say so: %+v, err %v", orphan, err)
	}
	for name, build := range map[string]func(context.Context, *Cluster, string, string) (*RelationNode, error){
		"PodDisruptionBudget": PDBTree, "NetworkPolicy": NetworkPolicyTree,
	} {
		tree, err := build(context.Background(), cluster, "shop", map[string]string{"PodDisruptionBudget": "api", "NetworkPolicy": "api-ingress"}[name])
		if err != nil || len(tree.Children) != 1 || tree.Children[0].Name != "api-1" || tree.Children[0].Kind != "Pod" {
			t.Fatalf("%s tree = %+v, err %v; want only api-1", name, tree, err)
		}
	}
}
