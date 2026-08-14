package k8sclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
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

func TestHelmPodHealthUsesPodReadyAndDeletion(t *testing.T) {
	deleting := metav1.Now()
	client := kubefake.NewSimpleClientset(
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "gated", Namespace: "apps"}, Status: corev1.PodStatus{
			Phase:             corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{{Name: "app", Ready: true}},
			Conditions:        []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionFalse}},
		}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "terminating", Namespace: "apps", DeletionTimestamp: &deleting}, Status: corev1.PodStatus{
			Phase:             corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{{Name: "app", Ready: true}},
			Conditions:        []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
		}},
	)
	kind := APIKind{Kind: "Pod", GVR: schema.GroupVersionResource{Version: "v1", Resource: "pods"}, Namespaced: true}
	for _, name := range []string{"gated", "terminating"} {
		_, health := resourceHealth(context.Background(), &Cluster{Clientset: client}, kind, "apps", name)
		if health != "degraded" {
			t.Fatalf("pod %s health = %q, want degraded", name, health)
		}
	}
}

type blockingRoundTripper struct{}

func (blockingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	<-req.Context().Done()
	return nil, req.Context().Err()
}

func TestHelmContextRoundTripperCancelsRequests(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	transport := helmContextRoundTripper{ctx: ctx, next: blockingRoundTripper{}}
	done := make(chan error, 1)
	go func() {
		req, _ := http.NewRequest(http.MethodGet, "https://cluster.example", nil)
		_, err := transport.RoundTrip(req)
		done <- err
	}()
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("RoundTrip error = %v, want context cancellation", err)
	}
}

func TestLocateHTTPChartLoadsExactArchive(t *testing.T) {
	archivePath, err := chartutil.Save(&chart.Chart{Metadata: &chart.Metadata{APIVersion: "v2", Name: "demo", Version: "1.0.0"}}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	archive, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/index.yaml":
			_, _ = w.Write([]byte("apiVersion: v1\nentries:\n  demo:\n    - apiVersion: v2\n      name: demo\n      version: 1.0.0\n      urls: [charts/demo-1.0.0.tgz]\n"))
		case "/charts/demo-1.0.0.tgz":
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, request)
		}
	}))
	defer server.Close()
	loaded, digest, err := locateAndLoadChart(context.Background(), action.NewInstall(&action.Configuration{}), server.URL, "", "demo", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Metadata == nil || loaded.Metadata.Name != "demo" || digest != textDigest(string(archive)) {
		t.Fatalf("loaded chart = %#v digest=%q", loaded.Metadata, digest)
	}
}

func TestLocateHTTPChartCancelsBlockedDownload(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(started)
		<-request.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, _, err := locateAndLoadChart(ctx, action.NewInstall(&action.Configuration{}), server.URL, "", "demo", "1.0.0")
		done <- err
	}()
	<-started
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("blocked chart request returned %v, want context cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("blocked chart request did not stop after context cancellation")
	}
}
