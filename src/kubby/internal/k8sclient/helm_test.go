package k8sclient

import (
	"context"
	"testing"

	"helm.sh/helm/v3/pkg/action"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func TestVerifyChartDigestRequiresThePreviewedArtifact(t *testing.T) {
	const digest = "sha256:0123456789abcdef"
	if err := verifyChartDigest("", digest); err == nil {
		t.Fatal("install without a preview digest should be rejected")
	}
	if err := verifyChartDigest("sha256:different", digest); err == nil {
		t.Fatal("a chart that changed after preview should be rejected")
	}
	if err := verifyChartDigest(digest, digest); err != nil {
		t.Fatalf("the exact previewed chart should be accepted: %v", err)
	}
}

func TestHelmOpenPGPVerificationRemainsDisabled(t *testing.T) {
	// Kubby uses its own preview SHA-256 gate. The Helm v3 OpenPGP implementation
	// is deprecated upstream and must not become reachable through a new option.
	options := action.ChartPathOptions{Verify: true}
	if err := configureChartSource(&options, "https://charts.example.com", "1.2.3"); err != nil {
		t.Fatalf("configure chart source: %v", err)
	}
	if options.Verify {
		t.Fatal("Helm chart provenance verification unexpectedly enabled")
	}
}

func TestValuesDigestBindsUpgradePreviewToExactInput(t *testing.T) {
	digest := textDigest("replicaCount: 2\n")
	if err := verifyTextDigest(digest, "replicaCount: 2\n", "values"); err != nil {
		t.Fatalf("exact previewed values should be accepted: %v", err)
	}
	if err := verifyTextDigest(digest, "replicaCount: 3\n", "values"); err == nil {
		t.Fatal("edited values should invalidate the upgrade preview")
	}
}

func TestHelmResourceHealthDoesNotReportFalseGreen(t *testing.T) {
	zero := int32(0)
	client := kubefake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "off", Namespace: "apps", Generation: 2},
			Spec:       appsv1.DeploymentSpec{Replicas: &zero},
			Status:     appsv1.DeploymentStatus{ObservedGeneration: 2},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "warming", Namespace: "apps"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{Name: "app", Ready: false}}},
		},
	)
	cluster := &Cluster{Clientset: client}
	_, deploymentHealth := resourceHealth(context.Background(), cluster,
		APIKind{Kind: "Deployment", GVR: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, Namespaced: true}, "apps", "off")
	if deploymentHealth != "healthy" {
		t.Fatalf("scaled-to-zero deployment should be healthy, got %q", deploymentHealth)
	}
	_, podHealth := resourceHealth(context.Background(), cluster,
		APIKind{Kind: "Pod", GVR: schema.GroupVersionResource{Version: "v1", Resource: "pods"}, Namespaced: true}, "apps", "warming")
	if podHealth != "degraded" {
		t.Fatalf("Running but NotReady pod should be degraded, got %q", podHealth)
	}
}
