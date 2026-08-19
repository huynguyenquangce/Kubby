package k8sclient

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const unsafeExecKubeconfig = `apiVersion: v1
kind: Config
current-context: unsafe
clusters:
- name: cluster
  cluster:
    server: https://127.0.0.1:6443
    insecure-skip-tls-verify: true
contexts:
- name: unsafe
  context:
    cluster: cluster
    user: unsafe-user
users:
- name: unsafe-user
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1
      command: should-never-run
      interactiveMode: Never
`

func safeKubeconfigForTest() *clientcmdapi.Config {
	return &clientcmdapi.Config{
		CurrentContext: "safe",
		Contexts: map[string]*clientcmdapi.Context{
			"safe":   {Cluster: "cluster", AuthInfo: "safe-user"},
			"unsafe": {Cluster: "cluster", AuthInfo: "unsafe-user"},
		},
		Clusters: map[string]*clientcmdapi.Cluster{
			"cluster": {Server: "https://127.0.0.1:6443"},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"safe-user": {Token: "embedded-token"},
			"unsafe-user": {
				Exec: &clientcmdapi.ExecConfig{Command: "should-never-run"},
			},
		},
	}
}

func TestValidateKubeconfigSafetyAcceptsEmbeddedCredentials(t *testing.T) {
	if err := validateKubeconfigSafety(safeKubeconfigForTest(), "safe"); err != nil {
		t.Fatalf("embedded credentials should be accepted: %v", err)
	}
}

func TestValidateKubeconfigSafetyAcceptsAnonymousContext(t *testing.T) {
	cfg := safeKubeconfigForTest()
	cfg.Contexts["anonymous"] = &clientcmdapi.Context{Cluster: "cluster"}
	if err := validateKubeconfigSafety(cfg, "anonymous"); err != nil {
		t.Fatalf("a context with no credential source is safe: %v", err)
	}
}

func TestValidateKubeconfigSafetyChecksOnlySelectedContext(t *testing.T) {
	cfg := safeKubeconfigForTest()
	if err := validateKubeconfigSafety(cfg, "safe"); err != nil {
		t.Fatalf("an unused unsafe user must not block the selected safe context: %v", err)
	}
	if err := validateKubeconfigSafety(cfg, "unsafe"); err == nil || !strings.Contains(err.Error(), "exec credential") {
		t.Fatalf("selected exec credential should be rejected, got %v", err)
	}
}

func TestValidateKubeconfigSafetyRejectsActiveAndFileBackedCredentials(t *testing.T) {
	cases := []struct {
		name string
		edit func(*clientcmdapi.AuthInfo)
		want string
	}{
		{"exec", func(a *clientcmdapi.AuthInfo) { a.Exec = &clientcmdapi.ExecConfig{Command: "helper"} }, "exec credential"},
		{"auth-provider", func(a *clientcmdapi.AuthInfo) { a.AuthProvider = &clientcmdapi.AuthProviderConfig{Name: "legacy"} }, "auth-provider"},
		{"token-file", func(a *clientcmdapi.AuthInfo) { a.TokenFile = "/private/token" }, "token file"},
		{"client-certificate", func(a *clientcmdapi.AuthInfo) { a.ClientCertificate = "/private/client.crt" }, "client-certificate"},
		{"client-key", func(a *clientcmdapi.AuthInfo) { a.ClientKey = "/private/client.key" }, "client-key"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := safeKubeconfigForTest()
			cfg.AuthInfos["safe-user"] = &clientcmdapi.AuthInfo{}
			tc.edit(cfg.AuthInfos["safe-user"])
			err := validateKubeconfigSafety(cfg, "safe")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q rejection, got %v", tc.want, err)
			}
		})
	}
}

func TestValidateKubeconfigSafetyRejectsConfiguredProxy(t *testing.T) {
	cfg := safeKubeconfigForTest()
	cfg.Clusters["cluster"].ProxyURL = "http://proxy.example.test:8080"
	if err := validateKubeconfigSafety(cfg, "safe"); err == nil || !strings.Contains(err.Error(), "proxy URL") {
		t.Fatalf("configured proxy should be rejected, got %v", err)
	}
}

func TestNewFromContentRejectsUnsafeCredentialsAtEntry(t *testing.T) {
	cluster, err := NewFromContent([]byte(unsafeExecKubeconfig), "unsafe")
	if cluster != nil || err == nil || !strings.Contains(err.Error(), "exec credential") {
		t.Fatalf("NewFromContent should reject an exec credential before returning a cluster, got cluster=%v err=%v", cluster, err)
	}
}

func TestNewRejectsUnsafeCredentialsAtEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "unsafe-kubeconfig.yaml")
	if err := os.WriteFile(path, []byte(unsafeExecKubeconfig), 0o600); err != nil {
		t.Fatalf("write unsafe kubeconfig fixture: %v", err)
	}
	cluster, err := New(path, "unsafe")
	if cluster != nil || err == nil || !strings.Contains(err.Error(), "exec credential") {
		t.Fatalf("New should reject an exec credential before returning a cluster, got cluster=%v err=%v", cluster, err)
	}
}
