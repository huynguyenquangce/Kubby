package k8sclient

import (
	"crypto/x509"
	"fmt"
	"sort"
	"sync"
	"time"

	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/metadata"
	"k8s.io/client-go/rest"
	clientcmdapi "k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

// connectTimeout bounds every API call so a slow/unreachable cluster (or a
// corporate security agent inspecting the first TLS connection) fails with a
// clear error instead of hanging the UI forever.
const connectTimeout = 30 * time.Second

// Cluster bundles the clients we use:
//   - Clientset: typed client for listing well-known resources
//   - Dynamic: generic get/update/delete of any resource kind (YAML editing)
//   - Stream: a no-timeout clientset used only for following pod logs
//   - Metrics: metrics.k8s.io client (nil-safe; may be unavailable)
//   - Discovery: what this cluster actually serves, so CRDs resolve too
//   - Meta: metadata-only lists (names, no object bodies) for counting and search
type Cluster struct {
	Clientset kubernetes.Interface
	Dynamic   dynamic.Interface
	Stream    kubernetes.Interface
	Metrics   metricsv.Interface
	Discovery discovery.DiscoveryInterface
	Meta      metadata.Interface
	// Rest is the no-timeout REST config, kept for port-forward and exec
	// (both hold long-lived connections that a 30s timeout would abort).
	Rest *rest.Config

	// Cached discovery index — see apiindex.go. Guarded because the UI fires
	// several resolutions concurrently (a drawer loads details and YAML at once).
	idxMu          sync.Mutex
	idx            *apiIndex
	optionalMu     sync.Mutex
	optionalAbsent map[string]time.Time

	// Cached SelfSubjectAccessReview answers, keyed "Kind|namespace" — see
	// access.go. Lives for the connection: RBAC does not change under us often,
	// and the apiserver remains the real enforcer either way.
	accessMu sync.Mutex
	access   map[string]*AccessSet

	// Overview shows only a small recent Event window, while Kubernetes Event
	// lists can be very large on busy clusters. Cache the already-trimmed result
	// per connection so the five-second live refresh does not relist and resort
	// the complete cluster history every time. See overview.go.
	overviewEventsMu      sync.Mutex
	overviewEvents        []EventInfo
	overviewEventsExpires time.Time

	// Global search builds a metadata-only name index. The mutex deliberately
	// covers refresh: concurrent/superseded palette queries share one fan-out
	// instead of each starting another set of cluster-wide lists. See explore.go.
	searchMu           sync.Mutex
	searchIndex        []searchIndexGroup
	searchIndexExpires time.Time

	// Health checks list every resource type for the stuck-deletion scan, so
	// their report is cached briefly per scope; the API server certificate is
	// read by a TLS handshake cached for longer. See healthchecks.go.
	checksMu       sync.Mutex
	checksCache    map[string]checksCacheEntry
	apiCertMu      sync.Mutex
	apiCert        *x509.Certificate
	apiCertErr     error
	apiCertExpires time.Time
}

// New builds a Cluster from a kubeconfig file path + chosen context.
// An empty context means "use the current-context defined in the kubeconfig".
func New(kubeconfigPath, context string) (*Cluster, error) {
	loadingRules := &clientcmdapi.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath}
	rawConfig, err := loadingRules.Load()
	if err != nil {
		return nil, err
	}
	if err := validateKubeconfigSafety(rawConfig, context); err != nil {
		return nil, err
	}
	overrides := &clientcmdapi.ConfigOverrides{}
	if context != "" {
		overrides.CurrentContext = context
	}
	restConfig, err := clientcmdapi.NewDefaultClientConfig(*rawConfig, overrides).ClientConfig()
	if err != nil {
		return nil, err
	}
	return clusterFromRest(restConfig)
}

// validateKubeconfigSafety runs before client-go creates transports or performs
// discovery. Kubeconfigs are active credential-loading configuration: exec and
// auth-provider plugins can launch processes, while file-backed credentials can
// read arbitrary local paths. Kubby has no separate high-risk approval UI yet,
// so the preview release accepts only credentials embedded in the selected
// kubeconfig and rejects those active/local-file mechanisms by default.
func validateKubeconfigSafety(cfg *api.Config, requestedContext string) error {
	contextName := requestedContext
	if contextName == "" {
		contextName = cfg.CurrentContext
	}
	ctx := cfg.Contexts[contextName]
	if ctx == nil {
		return fmt.Errorf("kubeconfig context %q was not found", contextName)
	}
	auth := &api.AuthInfo{}
	if ctx.AuthInfo != "" {
		auth = cfg.AuthInfos[ctx.AuthInfo]
	}
	if auth == nil {
		return fmt.Errorf("kubeconfig user %q was not found", ctx.AuthInfo)
	}
	unsafeCredential := ""
	switch {
	case auth.Exec != nil:
		unsafeCredential = "an exec credential plugin"
	case auth.AuthProvider != nil:
		unsafeCredential = "an auth-provider plugin"
	case auth.TokenFile != "":
		unsafeCredential = "a token file"
	case auth.ClientCertificate != "":
		unsafeCredential = "a client-certificate file"
	case auth.ClientKey != "":
		unsafeCredential = "a client-key file"
	}
	if unsafeCredential != "" {
		return fmt.Errorf("kubeconfig user %q uses %s; Kubby blocks executable and file-backed credentials until it can ask for explicit high-risk approval", ctx.AuthInfo, unsafeCredential)
	}
	cluster := cfg.Clusters[ctx.Cluster]
	if cluster == nil {
		return fmt.Errorf("kubeconfig cluster %q was not found", ctx.Cluster)
	}
	if cluster.ProxyURL != "" {
		return fmt.Errorf("kubeconfig cluster %q uses a proxy URL; Kubby blocks kubeconfig-defined proxies until it can ask for explicit high-risk approval", ctx.Cluster)
	}
	return nil
}

// NewFromContent builds a Cluster from raw kubeconfig bytes (pasted by the user).
func NewFromContent(content []byte, context string) (*Cluster, error) {
	rawConfig, err := clientcmdapi.Load(content)
	if err != nil {
		return nil, err
	}
	if err := validateKubeconfigSafety(rawConfig, context); err != nil {
		return nil, err
	}
	overrides := &clientcmdapi.ConfigOverrides{}
	if context != "" {
		overrides.CurrentContext = context
	}
	restConfig, err := clientcmdapi.NewDefaultClientConfig(*rawConfig, overrides).ClientConfig()
	if err != nil {
		return nil, err
	}
	return clusterFromRest(restConfig)
}

func clusterFromRest(restConfig *rest.Config) (*Cluster, error) {
	// client-go's default client-side rate limiter is QPS 5 / Burst 10 — tuned for
	// a controller that must not overwhelm a shared API server. For an
	// interactive UI it is the single biggest source of lag: one screen refresh
	// fans out to a couple of dozen lists, and past the burst every further
	// request sleeps, so filling the sidebar took seconds of pure throttle wait
	// (client-go logs "Waited for 1.1s due to client-side throttling").
	//
	// Kubby is one desktop app run by one person against one cluster, so it is
	// not the component that needs protecting here; the API server's own
	// priority-and-fairness does that job better. These are the limits kubectl
	// uses for its parallel paths.
	restConfig.QPS = 50
	restConfig.Burst = 100

	// A separate config without the timeout, used only for following logs
	// (a 30s timeout would abort a live stream).
	streamConfig := rest.CopyConfig(restConfig)
	streamConfig.Timeout = 0

	restConfig.Timeout = connectTimeout

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, err
	}
	dyn, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, err
	}
	stream, err := kubernetes.NewForConfig(streamConfig)
	if err != nil {
		return nil, err
	}

	disc, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return nil, err
	}
	// Metadata-only client: asks the API server for PartialObjectMetadata, so a
	// list returns names and labels without any object bodies. Counting Secrets
	// or ConfigMaps cluster-wide with the typed client transfers every value.
	meta, err := metadata.NewForConfig(restConfig)
	if err != nil {
		return nil, err
	}

	c := &Cluster{
		Clientset: clientset, Dynamic: dyn, Stream: stream,
		Discovery: disc, Meta: meta, Rest: streamConfig,
	}
	// Metrics is optional — the cluster may not have metrics-server installed.
	if m, err := metricsv.NewForConfig(restConfig); err == nil {
		c.Metrics = m
	}
	return c, nil
}

// Contexts lists the context names in a kubeconfig file so the UI can offer a picker.
func Contexts(kubeconfigPath string) (contexts []string, currentContext string, err error) {
	rawConfig, err := clientcmdapi.LoadFromFile(kubeconfigPath)
	if err != nil {
		return nil, "", err
	}
	return contextsFrom(rawConfig)
}

// ContextsFromContent lists context names from raw kubeconfig bytes.
func ContextsFromContent(content []byte) (contexts []string, currentContext string, err error) {
	rawConfig, err := clientcmdapi.Load(content)
	if err != nil {
		return nil, "", err
	}
	return contextsFrom(rawConfig)
}

func contextsFrom(cfg *api.Config) ([]string, string, error) {
	contexts := make([]string, 0, len(cfg.Contexts))
	for name := range cfg.Contexts {
		contexts = append(contexts, name)
	}
	sort.Strings(contexts)
	return contexts, cfg.CurrentContext, nil
}
