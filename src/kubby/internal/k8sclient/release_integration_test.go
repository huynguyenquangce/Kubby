package k8sclient

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
)

// Opt-in only: creates/deletes one uniquely named namespace in the existing kind
// lab. No existing resource is modified. Never point it at a production context.
func TestReleaseDisposableLifecycle(t *testing.T) {
	path := os.Getenv("KUBBY_RELEASE_TEST_KUBECONFIG")
	if path == "" {
		t.Skip("set KUBBY_RELEASE_TEST_KUBECONFIG for disposable kind lifecycle checks")
	}
	cluster, err := New(path, "kind-kubby-dev")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	namespace := fmt.Sprintf("kubby-release-test-%d", time.Now().UnixNano())
	_, err = cluster.Clientset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace, Labels: map[string]string{"kubby.dev/disposable-test": "true"}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cleanCancel()
		if err := cluster.Clientset.CoreV1().Namespaces().Delete(cleanCtx, namespace, metav1.DeleteOptions{}); err != nil {
			t.Errorf("cleanup namespace: %v", err)
			return
		}
		if err := wait.PollUntilContextCancel(cleanCtx, time.Second, true, func(ctx context.Context) (bool, error) {
			_, err := cluster.Clientset.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		}); err != nil {
			t.Errorf("namespace cleanup not complete: %v", err)
		}
	})
	t.Log("disposable namespace created; cleanup registered")
	// Reuse an image already running in this lab; Never avoids image pulls.
	const image = "public.ecr.aws/docker/library/redis:7.2.8-alpine"
	manifest := fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: synthetic
  namespace: %s
spec:
  replicas: 0
  selector:
    matchLabels: {app: kubby-release-test}
  template:
    metadata:
      labels: {app: kubby-release-test}
    spec:
      containers:
        - name: redis
          image: %s
          imagePullPolicy: Never
          command: [redis-server, --save, "", --appendonly, "no"]
          ports: [{containerPort: 6379}]
          resources:
            requests: {cpu: 10m, memory: 16Mi}
            limits: {cpu: 100m, memory: 64Mi}
`, namespace, image)
	if _, err := ApplyYAML(ctx, cluster, manifest); err != nil {
		t.Fatal(err)
	}
	if err := ScaleDeployment(ctx, cluster, namespace, "synthetic", 1); err != nil {
		t.Fatal(err)
	}
	var pod string
	if err := wait.PollUntilContextCancel(ctx, time.Second, true, func(ctx context.Context) (bool, error) {
		pods, err := cluster.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: "app=kubby-release-test"})
		if err != nil {
			return false, err
		}
		for _, p := range pods.Items {
			if p.DeletionTimestamp == nil {
				for _, cond := range p.Status.Conditions {
					if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
						pod = p.Name
						return true, nil
					}
				}
			}
		}
		return false, nil
	}); err != nil {
		t.Fatal("synthetic deployment not ready:", err)
	}
	t.Log("apply/scale and live readiness passed")
	// Real SPDY tunnel and Redis request/response, not only creation success.
	forward, ready, forwardErrors, err := StartPortForward(ctx, cluster, "Pod", namespace, pod, 0, 6379)
	if err != nil {
		t.Fatal(err)
	}
	defer forward.Close()
	select {
	case <-ready:
	case err := <-forwardErrors:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", forward.LocalPort), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	_, err = conn.Write([]byte("*1\r\n$4\r\nPING\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	response := make([]byte, 7)
	n, err := io.ReadFull(conn, response)
	_ = conn.Close()
	if err != nil || string(response[:n]) != "+PONG\r\n" {
		t.Fatal("forwarded Redis request did not succeed")
	}
	forward.Close()
	forward.Close()
	t.Log("real port-forward request and repeated close passed")
	// Real PTY, shell detection, ordered input and terminal resize.
	var outputMu sync.Mutex
	var output strings.Builder
	session, shell, err := StartExec(ctx, cluster, namespace, pod, "redis", "auto", 80, 24, func(chunk string) { outputMu.Lock(); output.WriteString(chunk); outputMu.Unlock() }, func(error) {})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if shell == "" {
		t.Fatal("shell was not resolved")
	}
	session.Resize(100, 30)
	if err := session.Write("printf 'release-%s\\n' smoke; stty size\n"); err != nil {
		t.Fatal(err)
	}
	if err := wait.PollUntilContextCancel(ctx, 100*time.Millisecond, true, func(context.Context) (bool, error) {
		outputMu.Lock()
		defer outputMu.Unlock()
		return strings.Contains(output.String(), "release-smoke") && strings.Contains(output.String(), "30 100"), nil
	}); err != nil {
		t.Fatal("PTY output/resize did not arrive:", err)
	}
	session.Close()
	t.Log("live shell/PTY output/resize passed (native WebView not tested)")
	logCtx, stopLogs := context.WithCancel(ctx)
	defer stopLogs()
	received := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() {
		done <- StreamLogs(logCtx, cluster, namespace, pod, "redis", func(lines []string) {
			if len(lines) > 0 {
				select {
				case received <- struct{}{}:
				default:
				}
			}
		})
	}()
	select {
	case <-received:
	case err := <-done:
		t.Fatalf("log follow ended before lines: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	stopLogs()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("log cancellation did not finish")
	}
	t.Log("live log follow and cancellation passed")
	for _, paused := range []bool{true, false} {
		if err := SetDeploymentPaused(ctx, cluster, namespace, "synthetic", paused); err != nil {
			t.Fatal(err)
		}
		dep, err := cluster.Clientset.AppsV1().Deployments(namespace).Get(ctx, "synthetic", metav1.GetOptions{})
		if err != nil || dep.Spec.Paused != paused {
			t.Fatal("pause/resume state not reflected")
		}
	}
	if err := RestartDeployment(ctx, cluster, namespace, "synthetic", "synthetic-release-stamp"); err != nil {
		t.Fatal(err)
	}
	if err := wait.PollUntilContextCancel(ctx, time.Second, true, func(ctx context.Context) (bool, error) {
		history, err := RolloutHistory(ctx, cluster, namespace, "synthetic")
		return len(history) >= 2, err
	}); err != nil {
		t.Fatal(err)
	}
	if err := RollbackDeployment(ctx, cluster, namespace, "synthetic", 1); err != nil {
		t.Fatal(err)
	}
	dep, err := cluster.Clientset.AppsV1().Deployments(namespace).Get(ctx, "synthetic", metav1.GetOptions{})
	if err != nil || dep.Spec.Template.Annotations["kubby.dev/restartedAt"] != "" {
		t.Fatal("rollback did not restore original template")
	}
	t.Log("pause/resume/restart/history/rollback passed")
	// Restricted token remains in memory; no credential file/log/fixture output.
	_, err = cluster.Clientset.CoreV1().ServiceAccounts(namespace).Create(ctx, &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "reader"}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = cluster.Clientset.RbacV1().Roles(namespace).Create(ctx, &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: "reader"}, Rules: []rbacv1.PolicyRule{{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get", "list", "watch"}}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = cluster.Clientset.RbacV1().RoleBindings(namespace).Create(ctx, &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "reader"}, RoleRef: rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "Role", Name: "reader"}, Subjects: []rbacv1.Subject{{Kind: "ServiceAccount", Name: "reader", Namespace: namespace}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	token, err := cluster.Clientset.CoreV1().ServiceAccounts(namespace).CreateToken(ctx, "reader", &authenticationv1.TokenRequest{Spec: authenticationv1.TokenRequestSpec{}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal("create restricted token failed")
	}
	cfg := rest.CopyConfig(cluster.Rest)
	cfg.CertData = nil
	cfg.KeyData = nil
	cfg.CertFile = ""
	cfg.KeyFile = ""
	cfg.BearerToken = token.Status.Token
	cfg.BearerTokenFile = ""
	restricted, err := clusterFromRest(cfg)
	if err != nil {
		t.Fatal("restricted client creation failed")
	}
	access, err := CanI(ctx, restricted, "Pod", namespace)
	if err != nil {
		t.Fatal("restricted permission probe failed")
	}
	if !access.Checked || !access.Verbs["get"] || access.Verbs["delete"] || access.Verbs["logs"] || access.Verbs["exec"] {
		t.Fatal("restricted RBAC/subresource verdict incorrect")
	}
	if err := DeleteResource(ctx, restricted, "Pod", namespace, pod); !apierrors.IsForbidden(err) {
		t.Fatal("API server did not deny restricted delete")
	}
	t.Log("live restricted RBAC and API write denial passed")
	// Local synthetic chart repo, no external credentials/repository edits.
	ch := &chart.Chart{Metadata: &chart.Metadata{APIVersion: "v2", Name: "synthetic", Version: "0.1.0"}, Values: map[string]interface{}{"message": "first"}, Raw: []*chart.File{{Name: "values.yaml", Data: []byte("message: first\n")}}, Templates: []*chart.File{
		{Name: "templates/config.yaml", Data: []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: {{ .Release.Name }}\ndata:\n  message: {{ .Values.message | quote }}\n")},
		{Name: "templates/test.yaml", Data: []byte(fmt.Sprintf("apiVersion: v1\nkind: Pod\nmetadata:\n  name: {{ .Release.Name }}-test\n  annotations:\n    helm.sh/hook: test\n    helm.sh/hook-delete-policy: before-hook-creation,hook-succeeded\nspec:\n  restartPolicy: Never\n  containers:\n    - name: test\n      image: %s\n      imagePullPolicy: Never\n      command: [/bin/sh, -c, 'exit 0']\n", image))},
	}}
	archive, err := chartutil.Save(ch, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/index.yaml" {
			_, _ = fmt.Fprint(w, "apiVersion: v1\nentries:\n  synthetic:\n    - apiVersion: v2\n      name: synthetic\n      version: 0.1.0\n      urls: [synthetic-0.1.0.tgz]\n")
			return
		}
		http.ServeFile(w, r, archive)
	}))
	defer server.Close()
	preview, err := HelmInstallPreview(ctx, cluster, namespace, "synthetic-chart", server.URL, "", "synthetic", "0.1.0", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := HelmInstall(ctx, cluster, namespace, "synthetic-chart", server.URL, "", "synthetic", "0.1.0", "", "sha256:changed"); err == nil {
		t.Fatal("changed chart accepted")
	}
	if err := HelmInstall(ctx, cluster, namespace, "synthetic-chart", server.URL, "", "synthetic", "0.1.0", "", preview.ChartDigest); err != nil {
		t.Fatal(err)
	}
	values := "message: second\n"
	upgrade, err := HelmUpgradePreview(ctx, cluster, namespace, "synthetic-chart", values)
	if err != nil {
		t.Fatal(err)
	}
	if err := HelmUpgradeValues(ctx, cluster, namespace, "synthetic-chart", "message: changed\n", upgrade.ReleaseRevision, upgrade.ValuesDigest); err == nil {
		t.Fatal("unreviewed values accepted")
	}
	if err := HelmUpgradeValues(ctx, cluster, namespace, "synthetic-chart", values, upgrade.ReleaseRevision, upgrade.ValuesDigest); err != nil {
		t.Fatal(err)
	}
	cm, err := cluster.Clientset.CoreV1().ConfigMaps(namespace).Get(ctx, "synthetic-chart", metav1.GetOptions{})
	if err != nil || cm.Data["message"] != "second" {
		t.Fatal("Helm upgrade state not reflected")
	}
	if err := HelmUpgradeValues(ctx, cluster, namespace, "synthetic-chart", values, upgrade.ReleaseRevision, upgrade.ValuesDigest); err == nil {
		t.Fatal("stale revision accepted")
	}
	if err := HelmRollback(ctx, cluster, namespace, "synthetic-chart", 1); err != nil {
		t.Fatal(err)
	}
	cm, err = cluster.Clientset.CoreV1().ConfigMaps(namespace).Get(ctx, "synthetic-chart", metav1.GetOptions{})
	if err != nil || cm.Data["message"] != "first" {
		t.Fatal("Helm rollback state not reflected")
	}
	result, err := HelmTest(ctx, cluster, namespace, "synthetic-chart")
	if err != nil || !strings.Contains(result, "Succeeded") {
		t.Fatal("Helm test hook did not succeed")
	}
	if err := HelmUninstall(ctx, cluster, namespace, "synthetic-chart"); err != nil {
		t.Fatal(err)
	}
	if _, err := cluster.Clientset.CoreV1().ConfigMaps(namespace).Get(ctx, "synthetic-chart", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatal("Helm uninstall left chart resource")
	}
	t.Log("live Helm preview/digest/install/upgrade/stale-revision/rollback/test/uninstall passed")
	if err := DeleteResource(ctx, cluster, "Deployment", namespace, "synthetic"); err != nil {
		t.Fatal(err)
	}
	if _, err := cluster.Clientset.AppsV1().Deployments(namespace).Get(ctx, "synthetic", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatal("delete left deployment")
	}
	t.Log("confirmed test scope delete passed; namespace cleanup follows")
}
