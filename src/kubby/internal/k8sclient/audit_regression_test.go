package k8sclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"helm.sh/helm/v3/pkg/action"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/httpstream"
	streamspdy "k8s.io/apimachinery/pkg/util/httpstream/spdy"
	"k8s.io/client-go/kubernetes"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	clienttesting "k8s.io/client-go/testing"
	metricsv1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsfake "k8s.io/metrics/pkg/client/clientset/versioned/fake"
	"sigs.k8s.io/yaml"
)

func TestAIEvidenceDropsSerializedLastAppliedCopies(t *testing.T) {
	for _, doc := range []map[string]interface{}{
		{"apiVersion": "v1", "kind": "Secret", "data": map[string]string{"opaque": "c3ludGhldGljLW9wYXF1ZQ=="}, "stringData": map[string]string{"opaque": "synthetic-literal"}},
		{"apiVersion": "v1", "kind": "Pod", "spec": map[string]interface{}{"containers": []interface{}{map[string]interface{}{"name": "app", "env": []interface{}{map[string]string{"name": "SETTING", "value": "synthetic-env"}}}}}},
	} {
		copied, _ := json.Marshal(doc)
		doc["metadata"] = map[string]interface{}{"name": "fixture", "annotations": map[string]string{"kubectl.kubernetes.io/last-applied-configuration": string(copied), "safe.example/setting": "retained"}}
		input, _ := yaml.Marshal(doc)
		evidence := redactDiagnosticYAML(string(input))
		for _, sentinel := range []string{"c3ludGhldGljLW9wYXF1ZQ==", "synthetic-literal", "synthetic-env", "last-applied-configuration"} {
			if strings.Contains(evidence, sentinel) {
				t.Fatal("serialized sensitive copy survived redaction")
			}
		}
		if !strings.Contains(evidence, "retained") {
			t.Fatal("unrelated annotation removed")
		}
	}
}

func TestPDBNullEmptyAndExpressionSelectors(t *testing.T) {
	for _, tc := range []struct {
		name     string
		selector *metav1.LabelSelector
		count    int
	}{
		{"null", nil, 0}, {"all", &metav1.LabelSelector{}, 2},
		{"labels", &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}}, 1},
		{"expression", &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "app", Operator: metav1.LabelSelectorOpIn, Values: []string{"api"}}}}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pdb := &policyv1.PodDisruptionBudget{ObjectMeta: metav1.ObjectMeta{Name: "budget", Namespace: "shop"}, Spec: policyv1.PodDisruptionBudgetSpec{Selector: tc.selector}}
			client := kubefake.NewSimpleClientset(pdb, nodePod("shop", "api", map[string]string{"app": "api"}, nil), nodePod("shop", "other", nil, nil))
			cluster := &Cluster{Clientset: client}
			tree, err := PDBTree(context.Background(), cluster, "shop", "budget")
			if err != nil || len(tree.Children) != tc.count {
				t.Fatalf("tree count: got %v err %v", tree, err)
			}
			impact, err := DrainImpactFor(context.Background(), cluster, "worker-1")
			if err != nil {
				t.Fatal(err)
			}
			if tc.count == 0 {
				if len(impact.BlockingBudgets) != 0 {
					t.Fatal("null selector protects Pods")
				}
			} else if len(impact.BlockingBudgets) != 1 || impact.BlockingBudgets[0].PodsOnNode != tc.count {
				t.Fatalf("missing blocking budget: %+v", impact.BlockingBudgets)
			}
		})
	}
}

func TestSchedulingInitOrderChangesPeakReservation(t *testing.T) {
	always := corev1.ContainerRestartPolicyAlways
	normal := corev1.Container{Name: "init", Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: qty("2")}}}
	sidecar := corev1.Container{Name: "sidecar", RestartPolicy: &always, Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: qty("1")}}}
	for _, tc := range []struct {
		name string
		init []corev1.Container
		want string
		fits bool
	}{
		{"init-before-sidecar", []corev1.Container{normal, sidecar}, "2", true},
		{"sidecar-before-init", []corev1.Container{sidecar, normal}, "3", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pod := schedPod("shop", "pending", withRequests("500m", "1Mi"))
			pod.Spec.InitContainers = tc.init
			requests := effectivePodRequests(pod)
			cpu := requests[corev1.ResourceCPU]
			if cpu.Cmp(qty(tc.want)) != 0 {
				t.Fatalf("want %s CPU, got %s", tc.want, cpu.String())
			}
			totals := map[corev1.ResourceName]int64{}
			accumulatePodRequests(totals, pod)
			if totals[corev1.ResourceCPU] != cpu.MilliValue() {
				t.Fatal("node usage differs from Pod reservation")
			}
			report := explain(t, []runtime.Object{pod, schedNode("worker", "2500m", "1Gi")}, "shop", "pending")
			if (report.Verdict == "fits") != tc.fits {
				t.Fatalf("unexpected verdict %s", report.Verdict)
			}
		})
	}
}

func TestPodsPagesOverlapReadsAndReuseImmutableMetrics(t *testing.T) {
	pod := nodePod("shop", "api", nil, nil)
	client := kubefake.NewSimpleClientset(pod)
	metricClient := metricsfake.NewSimpleClientset(&metricsv1.PodMetrics{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "shop"}})
	started := make(chan struct{})
	metricClient.PrependReactor("list", "pods", func(clienttesting.Action) (bool, runtime.Object, error) {
		close(started)
		return true, &metricsv1.PodMetricsList{Items: []metricsv1.PodMetrics{{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "shop"}}}}, nil
	})
	client.PrependReactor("list", "pods", func(clienttesting.Action) (bool, runtime.Object, error) {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Error("Pod and metrics reads were serialized")
		}
		return false, nil, nil
	})
	cluster := &Cluster{Clientset: client, Metrics: metricClient}
	for i := 0; i < 3; i++ {
		page, err := ListPodsPage(context.Background(), cluster, "shop", "", 200)
		if err != nil || len(page.Metrics) != 1 {
			t.Fatalf("page %d missing metrics: %v", i, err)
		}
	}
	if len(metricClient.Actions()) != 1 {
		t.Fatalf("metrics lists = %d; want 1", len(metricClient.Actions()))
	}
}

func TestHelmCredentialSourcesRejectPlaintextAndUserinfo(t *testing.T) {
	for _, url := range []string{"http://charts.example.test/index.yaml", "https://user:pass@charts.example.test/index.yaml"} {
		_, err := fetchHelmHTTP(context.Background(), &action.ChartPathOptions{RepoURL: url, Username: "synthetic", Password: "fixture"}, url, 100)
		if err == nil {
			t.Fatal("unsafe authenticated source accepted")
		}
	}
	policy := helmRedirectPolicy(&action.ChartPathOptions{})
	first, _ := http.NewRequest("GET", "https://charts.example.test/index.yaml", nil)
	redirect, _ := http.NewRequest("GET", "http://charts.example.test/index.yaml", nil)
	if policy(redirect, []*http.Request{first}) == nil {
		t.Fatal("HTTPS downgrade accepted")
	}
}

func TestHelmRepoIndexCancellationAndChunkedLimit(t *testing.T) {
	// Pin Add itself, not only the helper: it must never bypass the index limit.
	t.Setenv("HELM_REPOSITORY_CONFIG", filepath.Join(t.TempDir(), "repositories.yaml"))
	t.Setenv("HELM_REPOSITORY_CACHE", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.(http.Flusher).Flush()
		_, _ = w.Write([]byte(strings.Repeat("x", int(maxHelmRepoIndexBytes)+1)))
	}))
	defer server.Close()
	if err := AddHelmRepo(context.Background(), "oversized", server.URL, "", ""); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatal("chunked overflow accepted")
	}
	started := make(chan struct{})
	cancelled := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done(); close(cancelled) }))
	defer slow.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- AddHelmRepo(ctx, "cancelled", slow.URL, "", "") }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("index download never started")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancel ignored")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("download did not cancel")
	}
	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("server request remained alive")
	}
}

func TestPortForwardCloseCancelsStalledUpgrade(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done(); close(cancelled) }))
	defer server.Close()
	config := &rest.Config{Host: server.URL}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	session, _, done, err := StartPortForward(context.Background(), &Cluster{Clientset: client, Rest: config}, "Pod", "shop", "api", 0, 8080)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("upgrade never started")
	}
	session.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stalled upgrade survived Close")
	}
	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("upgrade socket remained open")
	}
}

func TestReadyPortForwardSurvivesStartupContextCancellation(t *testing.T) {
	connections := make(chan httpstream.Connection, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(httpstream.HeaderProtocolVersion, "portforward.k8s.io")
		conn := streamspdy.NewResponseUpgrader().UpgradeResponse(w, r, func(httpstream.Stream, <-chan struct{}) error { return nil })
		if conn != nil {
			connections <- conn
		}
	}))
	defer server.Close()
	config := &rest.Config{Host: server.URL}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	session, ready, done, err := StartPortForward(ctx, &Cluster{Clientset: client, Rest: config}, "Pod", "shop", "api", 0, 8080)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	select {
	case <-ready:
	case err := <-done:
		t.Fatalf("upgrade failed: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("forward not ready")
	}
	conn := <-connections
	defer conn.Close()
	cancel()
	select {
	case <-done:
		t.Fatal("startup context killed ready tunnel")
	case <-time.After(50 * time.Millisecond):
	}
	session.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not stop ready tunnel")
	}
}
