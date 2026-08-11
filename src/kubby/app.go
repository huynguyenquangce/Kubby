package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"kubby/internal/k8sclient"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the Wails application backend. Its exported methods are bound and
// callable from the frontend as async JS functions.
type App struct {
	ctx        context.Context
	cluster    *k8sclient.Cluster // the active cluster (nil if none)
	activeName string             // display name (context) of the active cluster
	clusters   map[string]*k8sclient.Cluster
	order      []string // connection order, for a stable dropdown
	logMu      sync.Mutex
	logCancel  context.CancelFunc // cancels the active log stream, if any
	logEpoch   uint64             // invalidates callbacks from an older stream
	pfMu       sync.Mutex
	pfSessions map[string]*portForwardEntry
	pfCluster  *k8sclient.Cluster
	pfEpoch    uint64
	execMu     sync.Mutex
	execSess   *k8sclient.ExecSession // the active exec session, if any
	execEpoch  uint64                 // invalidates callbacks from an older shell
}

func NewApp() *App {
	return &App{
		pfSessions: map[string]*portForwardEntry{},
		clusters:   map[string]*k8sclient.Cluster{},
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) shutdown(context.Context) {
	a.StopLogStream()
	a.StopExec()
	a.stopAllPortForwards()
}

// ContextsResult is returned to the frontend so it can render a context picker.
type ContextsResult struct {
	Contexts       []string `json:"contexts"`
	CurrentContext string   `json:"currentContext"`
}

// PickKubeconfigFile opens the native file dialog and returns the chosen path (FR-1).
func (a *App) PickKubeconfigFile() (string, error) {
	return wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Select a kubeconfig file",
	})
}

// ContextsFromPath lists contexts inside a kubeconfig file (FR-6).
func (a *App) ContextsFromPath(path string) (*ContextsResult, error) {
	contexts, current, err := k8sclient.Contexts(path)
	if err != nil {
		return nil, err
	}
	return &ContextsResult{Contexts: contexts, CurrentContext: current}, nil
}

// ContextsFromContent lists contexts inside pasted kubeconfig text (FR-6).
func (a *App) ContextsFromContent(content string) (*ContextsResult, error) {
	contexts, current, err := k8sclient.ContextsFromContent([]byte(content))
	if err != nil {
		return nil, err
	}
	return &ContextsResult{Contexts: contexts, CurrentContext: current}, nil
}

// ConnectWithPath connects to a cluster using a kubeconfig file path (FR-1).
func (a *App) ConnectWithPath(path, kubeContext string) error {
	cluster, err := k8sclient.New(path, kubeContext)
	if err != nil {
		return fmt.Errorf("could not build client: %w", err)
	}
	if err := a.verifyAndStore(kubeContext, cluster); err != nil {
		return err
	}
	a.rememberConnection(a.activeName, path, kubeContext)
	return nil
}

// ConnectWithContent connects to a cluster using pasted kubeconfig text (FR-1).
func (a *App) ConnectWithContent(content, kubeContext string) error {
	cluster, err := k8sclient.NewFromContent([]byte(content), kubeContext)
	if err != nil {
		return fmt.Errorf("could not build client: %w", err)
	}
	return a.verifyAndStore(kubeContext, cluster)
}

func (a *App) verifyAndStore(name string, cluster *k8sclient.Cluster) error {
	// Prove the connection actually works, not just that the config parsed.
	if _, err := k8sclient.ListNamespaces(a.ctx, cluster.Clientset); err != nil {
		return fmt.Errorf("connected but the API call failed: %w", err)
	}
	if name == "" {
		name = "default"
	}
	if _, exists := a.clusters[name]; !exists {
		a.order = append(a.order, name)
	}
	a.clusters[name] = cluster
	a.StopLogStream()
	a.StopExec()
	a.resetPortForwardsForCluster(cluster)
	a.activeName = name
	a.cluster = cluster
	return nil
}

// ClusterInfo describes a connected cluster for the switcher dropdown.
type ClusterInfo struct {
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

// ConnectedClusters lists all currently-connected clusters (FR-6).
func (a *App) ConnectedClusters() []ClusterInfo {
	out := make([]ClusterInfo, 0, len(a.order))
	for _, name := range a.order {
		out = append(out, ClusterInfo{Name: name, Active: name == a.activeName})
	}
	return out
}

// SwitchCluster makes an already-connected cluster the active one (FR-6).
func (a *App) SwitchCluster(name string) error {
	c, ok := a.clusters[name]
	if !ok {
		return fmt.Errorf("cluster %q is not connected", name)
	}
	// Stop anything tied to the previous cluster.
	a.StopLogStream()
	a.StopExec()
	a.resetPortForwardsForCluster(c)
	a.activeName = name
	a.cluster = c
	return nil
}

// DisconnectCluster drops a connected cluster. Returns the name of the new
// active cluster ("" if none remain, meaning the UI should show Welcome).
func (a *App) DisconnectCluster(name string) string {
	if _, ok := a.clusters[name]; !ok {
		return a.activeName
	}
	if name == a.activeName {
		a.StopLogStream()
		a.StopExec()
		a.resetPortForwardsForCluster(nil)
	}
	delete(a.clusters, name)
	for i, n := range a.order {
		if n == name {
			a.order = append(a.order[:i], a.order[i+1:]...)
			break
		}
	}
	if name == a.activeName {
		if len(a.order) > 0 {
			a.activeName = a.order[0]
			a.cluster = a.clusters[a.activeName]
		} else {
			a.activeName = ""
			a.cluster = nil
		}
		a.resetPortForwardsForCluster(a.cluster)
	}
	return a.activeName
}

func (a *App) requireCluster() error {
	if a.cluster == nil {
		return fmt.Errorf("not connected to any cluster")
	}
	return nil
}

func (a *App) ListNodes() ([]k8sclient.NodeInfo, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.ListNodes(a.ctx, a.cluster.Clientset)
}

func (a *App) ListNamespaces() ([]k8sclient.NamespaceInfo, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.ListNamespaces(a.ctx, a.cluster.Clientset)
}

// ListPods lists pods in a namespace ("" means all namespaces) (FR-2, FR-3).
func (a *App) ListPods(namespace string) ([]k8sclient.PodInfo, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.ListPods(a.ctx, a.cluster.Clientset, namespace)
}

func (a *App) ListDeployments(namespace string) ([]k8sclient.DeploymentInfo, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.ListDeployments(a.ctx, a.cluster.Clientset, namespace)
}

func (a *App) ListServices(namespace string) ([]k8sclient.ServiceInfo, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.ListServices(a.ctx, a.cluster.Clientset, namespace)
}

func (a *App) ListConfigMaps(namespace string) ([]k8sclient.ConfigMapInfo, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.ListConfigMaps(a.ctx, a.cluster.Clientset, namespace)
}

func (a *App) ListSecrets(namespace string) ([]k8sclient.SecretInfo, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.ListSecrets(a.ctx, a.cluster.Clientset, namespace)
}

// GetDetail returns structured details for the drawer's Details tab.
func (a *App) GetDetail(kind, namespace, name string) (*k8sclient.ResourceDetail, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.GetDetail(a.ctx, a.cluster, kind, namespace, name)
}

// ListEvents returns events involving a specific resource (like kubectl describe).
func (a *App) ListEvents(kind, namespace, name string) ([]k8sclient.EventInfo, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.ListEvents(a.ctx, a.cluster, kind, namespace, name)
}

// DeleteResource deletes a resource by kind/namespace/name.
func (a *App) DeleteResource(kind, namespace, name string) error {
	if err := a.requireCluster(); err != nil {
		return err
	}
	return k8sclient.DeleteResource(a.ctx, a.cluster, kind, namespace, name)
}

// ---- Feature 1: deployment actions ----

func (a *App) ScaleDeployment(namespace, name string, replicas int) error {
	if err := a.requireCluster(); err != nil {
		return err
	}
	return k8sclient.ScaleDeployment(a.ctx, a.cluster, namespace, name, int32(replicas))
}

func (a *App) RestartDeployment(namespace, name string) error {
	if err := a.requireCluster(); err != nil {
		return err
	}
	return k8sclient.RestartDeployment(a.ctx, a.cluster, namespace, name, time.Now().Format(time.RFC3339))
}

func (a *App) RestartStatefulSet(namespace, name string) error {
	if err := a.requireCluster(); err != nil {
		return err
	}
	return k8sclient.RestartStatefulSet(a.ctx, a.cluster, namespace, name, time.Now().Format(time.RFC3339))
}

func (a *App) RestartDaemonSet(namespace, name string) error {
	if err := a.requireCluster(); err != nil {
		return err
	}
	return k8sclient.RestartDaemonSet(a.ctx, a.cluster, namespace, name, time.Now().Format(time.RFC3339))
}

func (a *App) SetDeploymentPaused(namespace, name string, paused bool) error {
	if err := a.requireCluster(); err != nil {
		return err
	}
	return k8sclient.SetDeploymentPaused(a.ctx, a.cluster, namespace, name, paused)
}

func (a *App) RolloutHistory(namespace, name string) ([]k8sclient.RolloutRevision, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.RolloutHistory(a.ctx, a.cluster, namespace, name)
}

func (a *App) RollbackDeployment(namespace, name string, revision int64) error {
	if err := a.requireCluster(); err != nil {
		return err
	}
	return k8sclient.RollbackDeployment(a.ctx, a.cluster, namespace, name, revision)
}

// ---- Node actions ----

func (a *App) SetNodeSchedulable(name string, schedulable bool) error {
	if err := a.requireCluster(); err != nil {
		return err
	}
	return k8sclient.SetNodeSchedulable(a.ctx, a.cluster, name, schedulable)
}

func (a *App) DrainNode(name string) error {
	if err := a.requireCluster(); err != nil {
		return err
	}
	return k8sclient.DrainNode(a.ctx, a.cluster, name)
}

// ---- CronJob run-now ----

func (a *App) RunCronJobNow(namespace, name string) error {
	if err := a.requireCluster(); err != nil {
		return err
	}
	return k8sclient.RunCronJobNow(a.ctx, a.cluster, namespace, name, time.Now().Format("20060102-150405"))
}

// ---- Secret reveal ----

func (a *App) SecretData(namespace, name string) ([]k8sclient.SecretEntry, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.SecretData(a.ctx, a.cluster, namespace, name)
}

// ---- Feature 3: more resource types ----

func (a *App) ListStatefulSets(ns string) ([]k8sclient.StatefulSetInfo, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.ListStatefulSets(a.ctx, a.cluster.Clientset, ns)
}
func (a *App) ListDaemonSets(ns string) ([]k8sclient.DaemonSetInfo, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.ListDaemonSets(a.ctx, a.cluster.Clientset, ns)
}
func (a *App) ListJobs(ns string) ([]k8sclient.JobInfo, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.ListJobs(a.ctx, a.cluster.Clientset, ns)
}
func (a *App) ListCronJobs(ns string) ([]k8sclient.CronJobInfo, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.ListCronJobs(a.ctx, a.cluster.Clientset, ns)
}
func (a *App) ListIngresses(ns string) ([]k8sclient.IngressInfo, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.ListIngresses(a.ctx, a.cluster.Clientset, ns)
}
func (a *App) ListPVCs(ns string) ([]k8sclient.PVCInfo, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.ListPVCs(a.ctx, a.cluster.Clientset, ns)
}
func (a *App) ListServiceAccounts(ns string) ([]k8sclient.ServiceAccountInfo, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.ListServiceAccounts(a.ctx, a.cluster.Clientset, ns)
}

// ---- Storage + RBAC resource types ----

func (a *App) ListPersistentVolumes() ([]k8sclient.PersistentVolumeInfo, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.ListPersistentVolumes(a.ctx, a.cluster.Clientset)
}
func (a *App) ListStorageClasses() ([]k8sclient.StorageClassInfo, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.ListStorageClasses(a.ctx, a.cluster.Clientset)
}
func (a *App) ListRoles(ns string) ([]k8sclient.RoleInfo, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.ListRoles(a.ctx, a.cluster.Clientset, ns)
}
func (a *App) ListRoleBindings(ns string) ([]k8sclient.RoleBindingInfo, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.ListRoleBindings(a.ctx, a.cluster.Clientset, ns)
}
func (a *App) ListClusterRoles() ([]k8sclient.ClusterRoleInfo, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.ListClusterRoles(a.ctx, a.cluster.Clientset)
}
func (a *App) ListClusterRoleBindings() ([]k8sclient.ClusterRoleBindingInfo, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.ListClusterRoleBindings(a.ctx, a.cluster.Clientset)
}

// ---- Ecosystem: CRDs, Helm, quotas ----

func (a *App) ListCRDs() ([]k8sclient.CRDInfo, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.ListCRDs(a.ctx, a.cluster)
}
func (a *App) ListHelmReleases(ns string) ([]k8sclient.HelmReleaseInfo, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.ListHelmReleases(a.ctx, a.cluster.Clientset, ns)
}

// ---- Helm SDK: release management + search + install ----

func (a *App) HelmGet(namespace, name string) (*k8sclient.HelmReleaseDetail, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.HelmGet(a.cluster, namespace, name)
}
func (a *App) HelmHistory(namespace, name string) ([]k8sclient.HelmRevision, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.HelmHistory(a.cluster, namespace, name)
}
func (a *App) HelmRollback(namespace, name string, revision int) error {
	if err := a.requireCluster(); err != nil {
		return err
	}
	return k8sclient.HelmRollback(a.cluster, namespace, name, revision)
}
func (a *App) HelmUninstall(namespace, name string) error {
	if err := a.requireCluster(); err != nil {
		return err
	}
	return k8sclient.HelmUninstall(a.cluster, namespace, name)
}
func (a *App) HelmUpgradeValues(namespace, name, valuesYAML string) error {
	if err := a.requireCluster(); err != nil {
		return err
	}
	return k8sclient.HelmUpgradeValues(a.cluster, namespace, name, valuesYAML)
}
func (a *App) HelmInstall(namespace, releaseName, repoURL, chartName, version, valuesYAML, expectedDigest string) error {
	if err := a.requireCluster(); err != nil {
		return err
	}
	return k8sclient.HelmInstall(a.cluster, namespace, releaseName, repoURL, chartName, version, valuesYAML, expectedDigest)
}

// SearchCharts queries Artifact Hub (needs internet). No cluster required.
func (a *App) SearchCharts(query string) ([]k8sclient.ChartSearchResult, error) {
	return k8sclient.SearchCharts(a.ctx, query)
}
func (a *App) ChartDetails(repo, chartName string) (*k8sclient.ChartDetail, error) {
	return k8sclient.ChartDetails(a.ctx, repo, chartName)
}
func (a *App) ChartDefaultValues(repoURL, chartName, version string) (string, error) {
	return k8sclient.ChartDefaultValues(repoURL, chartName, version)
}
func (a *App) HelmInstallPreview(namespace, releaseName, repoURL, chartName, version, valuesYAML string) (*k8sclient.HelmDiff, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.HelmInstallPreview(a.cluster, namespace, releaseName, repoURL, chartName, version, valuesYAML)
}
func (a *App) HelmUpgradePreview(namespace, name, valuesYAML string) (*k8sclient.HelmDiff, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.HelmUpgradePreview(a.cluster, namespace, name, valuesYAML)
}
func (a *App) HelmGetRevision(namespace, name string, revision int) (*k8sclient.HelmReleaseDetail, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.HelmGetRevision(a.cluster, namespace, name, revision)
}
func (a *App) HelmReleaseResources(namespace, name string) ([]k8sclient.HelmResource, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.HelmReleaseResources(a.ctx, a.cluster, namespace, name)
}
func (a *App) HelmTest(namespace, name string) (string, error) {
	if err := a.requireCluster(); err != nil {
		return "", err
	}
	return k8sclient.HelmTest(a.cluster, namespace, name)
}

// ---- Helm repositories ----

func (a *App) ListHelmRepos() ([]k8sclient.HelmRepo, error) {
	return k8sclient.ListHelmRepos()
}
func (a *App) AddHelmRepo(name, url, username, password string) error {
	return k8sclient.AddHelmRepo(name, url, username, password)
}
func (a *App) RemoveHelmRepo(name string) error {
	return k8sclient.RemoveHelmRepo(name)
}
func (a *App) UpdateHelmRepos() error {
	return k8sclient.UpdateHelmRepos()
}
func (a *App) BrowseHelmRepo(name string) ([]k8sclient.ChartSearchResult, error) {
	return k8sclient.BrowseHelmRepo(name)
}
func (a *App) ListResourceQuotas(ns string) ([]k8sclient.ResourceQuotaInfo, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.ListResourceQuotas(a.ctx, a.cluster.Clientset, ns)
}
func (a *App) ListLimitRanges(ns string) ([]k8sclient.LimitRangeInfo, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.ListLimitRanges(a.ctx, a.cluster.Clientset, ns)
}

// ---- Feature 4: relations + metrics ----

// OverviewSnapshot returns the complete dashboard payload in one bound call;
// Kubernetes fan-out and partial-error handling live in k8sclient.
func (a *App) OverviewSnapshot() (*k8sclient.OverviewData, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.OverviewSnapshot(a.ctx, a.cluster)
}

func (a *App) DeploymentTree(namespace, name string) (*k8sclient.RelationNode, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.DeploymentTree(a.ctx, a.cluster, namespace, name)
}

func (a *App) NodeMetrics() ([]k8sclient.NodeMetric, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.NodeMetrics(a.ctx, a.cluster)
}

// TopPods returns the top N pods by CPU usage (Overview dashboard).
func (a *App) TopPods(limit int) ([]k8sclient.PodMetric, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.TopPods(a.ctx, a.cluster, limit)
}

// PodMetricsList returns per-pod CPU/mem for a namespace (merged into the pods table).
func (a *App) PodMetricsList(namespace string) ([]k8sclient.PodMetric, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.PodMetricsList(a.ctx, a.cluster, namespace)
}

// ClusterEvents returns the most recent events across the cluster (Overview).
func (a *App) ClusterEvents(limit int) ([]k8sclient.EventInfo, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.ClusterEvents(a.ctx, a.cluster, limit)
}

// ServiceTree returns a Service → Pod relations tree.
func (a *App) ServiceTree(namespace, name string) (*k8sclient.RelationNode, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.ServiceTree(a.ctx, a.cluster, namespace, name)
}

// IngressTree returns an Ingress → Service → Pod relations tree.
func (a *App) IngressTree(namespace, name string) (*k8sclient.RelationNode, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.IngressTree(a.ctx, a.cluster, namespace, name)
}

// ---- Explore: node pods, namespace summary, global search, network topology ----

func (a *App) PodsOnNode(nodeName string) ([]k8sclient.PodInfo, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.PodsOnNode(a.ctx, a.cluster, nodeName)
}

func (a *App) NamespaceSummary(ns string) ([]k8sclient.NsKindCount, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.NamespaceSummary(a.ctx, a.cluster, ns)
}

func (a *App) SearchResources(query string) ([]k8sclient.SearchHit, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.SearchResources(a.ctx, a.cluster, query)
}

// SidebarCounts returns every sidebar tally in one call. The frontend used to
// make one bound call per kind (~25 round-trips, each serialising every object
// just to take .length), which is what made switching namespace stall.
// includeCluster=false skips the cluster-scoped kinds, whose counts cannot
// change when only the namespace does.
func (a *App) SidebarCounts(namespace string, includeCluster bool) ([]k8sclient.NavCount, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.SidebarCounts(a.ctx, a.cluster, namespace, includeCluster)
}

// CustomKinds lists the kinds this cluster's CRDs define, so each becomes its own
// sidebar section (and command-palette entry). No counts — see CustomKinds' doc.
func (a *App) CustomKinds() (*k8sclient.CustomKindList, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.CustomKinds(a.ctx, a.cluster)
}

// ListCustom lists the objects of a custom kind ("Kind.group"), namespace "" = all.
func (a *App) ListCustom(refKind, namespace string) ([]k8sclient.CustomObject, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.ListCustom(a.ctx, a.cluster, refKind, namespace)
}

func (a *App) NetworkFlows(namespace string) (*k8sclient.NetworkFlows, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.NetworkTopology(a.ctx, a.cluster, namespace)
}

// ---- FR-7: AI assistant (resource-scoped Q&A) ----

func (a *App) GetAIConfig() AIConfigView {
	cfg := loadAIConfig()
	return AIConfigView{
		Provider: cfg.Provider, Endpoint: cfg.Endpoint, Model: cfg.Model,
		Language: cfg.Language, HasAPIKey: strings.TrimSpace(cfg.APIKey) != "",
	}
}

func (a *App) SaveAIConfig(provider, endpoint, apiKey, model, language string) error {
	next, err := mergeAIConfig(loadAIConfig(), AIConfig{
		Provider: provider, Endpoint: endpoint, APIKey: apiKey,
		Model: model, Language: language,
	})
	if err != nil {
		return err
	}
	return saveAIConfig(next)
}

// AIStatus is what the assistant header needs to describe itself, without ever
// handing the API key back to the frontend.
type AIStatus struct {
	Configured bool   `json:"configured"`
	Provider   string `json:"provider"`
	Label      string `json:"label"`
	Model      string `json:"model"`
	Local      bool   `json:"local"` // true = nothing leaves this machine
}

func (a *App) GetAIStatus() AIStatus {
	cfg := loadAIConfig()
	model := cfg.Model
	if model == "" {
		model = defaultModel(cfg.Provider)
	}
	return AIStatus{
		Configured: cfg.Provider != "",
		Provider:   cfg.Provider,
		Label:      providerLabel(cfg.Provider),
		Model:      model,
		Local:      cfg.Provider == "ollama",
	}
}

// AIResourceContext returns the exact evidence Kubby would send about a
// resource, so the assistant can show it before anything is transmitted.
func (a *App) AIResourceContext(kind, namespace, name string) (*k8sclient.AIContext, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.DiagnosticContext(a.ctx, a.cluster, kind, namespace, name)
}

// AskAboutResource answers a question about one resource. The resource's
// events/logs/YAML go into the system prompt rather than the thread, so every
// follow-up is grounded in the same snapshot and the frontend only has to replay
// the visible conversation.
func (a *App) AskAboutResource(kind, namespace, name string, history []AIMessage) (string, error) {
	if err := a.requireCluster(); err != nil {
		return "", err
	}
	cfg := loadAIConfig()
	if cfg.Provider == "" {
		return "", fmt.Errorf("no AI provider configured — open Settings (⚙) to pick one")
	}
	if len(history) == 0 {
		return "", fmt.Errorf("no question to ask")
	}
	diag, err := k8sclient.DiagnosticContext(a.ctx, a.cluster, kind, namespace, name)
	if err != nil {
		return "", err
	}

	where := kind + " " + name
	if namespace != "" {
		where += " in namespace " + namespace
	}
	system := strings.Join([]string{
		"You are a senior Kubernetes/DevOps engineer helping someone inspect a live cluster",
		"from a desktop UI. The user is looking at " + where + ".",
		"",
		"Answer only from the evidence below plus general Kubernetes knowledge. If the evidence",
		"does not contain the answer, say so and name the command or view that would reveal it —",
		"never invent field values, log lines or events.",
		"",
		"Be concise and practical: short paragraphs or bullets, concrete kubectl commands in",
		"backticks, and the single most likely cause first. Use markdown. " + languageRule(cfg.Language),
		"",
		"--- EVIDENCE (live, collected just now) ---",
		diag.Text,
	}, "\n")

	return callAI(a.ctx, cfg, system, history)
}

// languageRule turns the Settings preference into a prompt instruction.
func languageRule(lang string) string {
	switch lang {
	case "en":
		return "Always answer in English."
	case "vi":
		return "Luôn trả lời bằng tiếng Việt (giữ nguyên thuật ngữ Kubernetes bằng tiếng Anh)."
	default:
		return "Answer in the same language the user wrote their question in."
	}
}

// ---- Feature 2: log streaming + save ----

// LogStreamBatch crosses the Wails bridge at most once per backend batch. The
// frontend-provided stream ID prevents a late batch from an old Pod/container
// being rendered into its replacement drawer.
type LogStreamBatch struct {
	StreamID string   `json:"streamId"`
	Lines    []string `json:"lines"`
}

type LogStreamError struct {
	StreamID string `json:"streamId"`
	Message  string `json:"message"`
}

// StartLogStream begins following a container's logs. Batches are emitted as
// "loglines" events; the stream stops on StopLogStream, a new stream, or
// disconnect.
func (a *App) StartLogStream(namespace, name, container, streamID string) error {
	if err := a.requireCluster(); err != nil {
		return err
	}
	if strings.TrimSpace(streamID) == "" {
		return fmt.Errorf("log stream ID is required")
	}

	a.logMu.Lock()
	if a.logCancel != nil {
		a.logCancel()
	}
	a.logEpoch++
	epoch := a.logEpoch
	ctx, cancel := context.WithCancel(a.ctx)
	a.logCancel = cancel
	cluster := a.cluster
	a.logMu.Unlock()

	go func() {
		err := k8sclient.StreamLogs(ctx, cluster, namespace, name, container, func(lines []string) {
			if a.isCurrentLogStream(epoch) {
				wailsruntime.EventsEmit(a.ctx, "loglines", LogStreamBatch{StreamID: streamID, Lines: lines})
			}
		})
		if err != nil && ctx.Err() == nil && a.isCurrentLogStream(epoch) {
			wailsruntime.EventsEmit(a.ctx, "logerror", LogStreamError{StreamID: streamID, Message: err.Error()})
		}
	}()
	return nil
}

func (a *App) isCurrentLogStream(epoch uint64) bool {
	a.logMu.Lock()
	defer a.logMu.Unlock()
	return a.logEpoch == epoch && a.logCancel != nil
}

// StopLogStream cancels the active log stream, if any.
func (a *App) StopLogStream() {
	a.logMu.Lock()
	defer a.logMu.Unlock()
	a.logEpoch++
	if a.logCancel != nil {
		a.logCancel()
		a.logCancel = nil
	}
}

// SaveTextToFile opens a save dialog and writes content to the chosen path.
// Returns the path saved, or "" if the user cancelled.
func (a *App) SaveTextToFile(defaultName, content string) (string, error) {
	path, err := wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		DefaultFilename: defaultName,
		Title:           "Save logs",
	})
	if err != nil || path == "" {
		return "", err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// GetYAML returns a resource rendered as YAML for the detail/edit panel.
func (a *App) GetYAML(kind, namespace, name string) (string, error) {
	if err := a.requireCluster(); err != nil {
		return "", err
	}
	return k8sclient.GetYAML(a.ctx, a.cluster, kind, namespace, name)
}

// UpdateYAML applies edited YAML only when the active cluster and resource still
// match the drawer that loaded it.  The local cluster pointer is captured after
// the check so a later UI switch cannot redirect this call to the new cluster.
func (a *App) UpdateYAML(expectedCluster, expectedKind, expectedNamespace, expectedName, yamlText string) error {
	if err := a.requireCluster(); err != nil {
		return err
	}
	if expectedCluster == "" || expectedCluster != a.activeName {
		return fmt.Errorf("refusing stale YAML update: expected cluster %q, active cluster is %q; reload the open resource", expectedCluster, a.activeName)
	}
	cluster := a.cluster
	return k8sclient.UpdateYAML(a.ctx, cluster, expectedKind, expectedNamespace, expectedName, yamlText)
}

// ApplyYAML creates or updates the resource(s) in the given YAML (create-or-
// update, multi-document, any kind the cluster serves including CRDs). Backs the
// "Create" and "Import YAML" flows. Returns a per-document report of what it did.
func (a *App) ApplyYAML(yamlText string) (string, error) {
	if err := a.requireCluster(); err != nil {
		return "", err
	}
	return k8sclient.ApplyYAML(a.ctx, a.cluster, yamlText)
}

// ApplyPreview answers "what would this YAML change?" without changing anything,
// via a server-side dry run diffed against the live objects. Backs the Preview
// step of "Import YAML", the same way HelmUpgradePreview backs Helm's.
func (a *App) ApplyPreview(yamlText string) (*k8sclient.ApplyDiff, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.ApplyPreview(a.ctx, a.cluster, yamlText)
}

// Sizing reports declared requests/limits against actual usage for a namespace
// ("" = whole cluster). Backs the Right-sizing view.
func (a *App) Sizing(namespace string) (*k8sclient.SizingReport, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.Sizing(a.ctx, a.cluster, namespace)
}

// CanI reports what the connected token may do to a kind in a namespace, so the
// UI can disable actions instead of offering them and failing at the API.
func (a *App) CanI(kind, namespace string) (*k8sclient.AccessSet, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.CanI(a.ctx, a.cluster, kind, namespace)
}

// PodContainers lists a pod's container names (for the log container picker).
func (a *App) PodContainers(namespace, name string) ([]string, error) {
	if err := a.requireCluster(); err != nil {
		return nil, err
	}
	return k8sclient.PodContainers(a.ctx, a.cluster, namespace, name)
}

// PodLogs returns the last `tailLines` log lines for a pod container.
func (a *App) PodLogs(namespace, name, container string, tailLines int) (string, error) {
	if err := a.requireCluster(); err != nil {
		return "", err
	}
	return k8sclient.PodLogs(a.ctx, a.cluster, namespace, name, container, int64(tailLines))
}

// ---- Port-forward ----

// PortForwardInfo describes an active forward for the UI.
type PortForwardInfo struct {
	Key         string `json:"key"`
	Kind        string `json:"kind"`
	Namespace   string `json:"namespace"`
	Name        string `json:"name"`
	PodName     string `json:"podName"`
	LocalPort   int    `json:"localPort"`
	RemotePort  int    `json:"remotePort"`
	KeepRunning bool   `json:"keepRunning"`
}

type portForwardEntry struct {
	session *k8sclient.PortForwardSession
	info    PortForwardInfo
}

func (a *App) detachPortForwardsLocked() []*k8sclient.PortForwardSession {
	sessions := make([]*k8sclient.PortForwardSession, 0, len(a.pfSessions))
	for _, entry := range a.pfSessions {
		if entry.session != nil {
			sessions = append(sessions, entry.session)
		}
	}
	a.pfSessions = map[string]*portForwardEntry{}
	a.pfEpoch++
	return sessions
}

func closePortForwardSessions(sessions []*k8sclient.PortForwardSession) {
	for _, session := range sessions {
		session.Close()
	}
}

// resetPortForwardsForCluster atomically invalidates pending starts, changes the
// cluster new starts belong to, and detaches every existing tunnel.
func (a *App) resetPortForwardsForCluster(cluster *k8sclient.Cluster) {
	a.pfMu.Lock()
	sessions := a.detachPortForwardsLocked()
	a.pfCluster = cluster
	a.pfMu.Unlock()
	closePortForwardSessions(sessions)
}

func (a *App) stopAllPortForwards() {
	a.pfMu.Lock()
	sessions := a.detachPortForwardsLocked()
	a.pfMu.Unlock()
	closePortForwardSessions(sessions)
}

func (a *App) portForwardEpoch() uint64 {
	a.pfMu.Lock()
	defer a.pfMu.Unlock()
	return a.pfEpoch
}

// StartPortForward opens a forward from localPort (0 = auto) to remotePort of a
// pod backing the target Pod/Service. Returns the info including the bound local
// port once the tunnel is ready.
func (a *App) StartPortForward(kind, namespace, name string, localPort, remotePort int, keepRunning bool) (*PortForwardInfo, error) {
	a.pfMu.Lock()
	cluster, epoch := a.pfCluster, a.pfEpoch
	a.pfMu.Unlock()
	if cluster == nil {
		return nil, fmt.Errorf("not connected to any cluster")
	}
	session, ready, errCh, err := k8sclient.StartPortForward(a.ctx, cluster, kind, namespace, name, localPort, remotePort)
	if err != nil {
		return nil, err
	}

	select {
	case <-ready:
	case err := <-errCh:
		session.Close()
		return nil, fmt.Errorf("port-forward failed: %w", err)
	case <-time.After(15 * time.Second):
		session.Close()
		return nil, fmt.Errorf("port-forward did not become ready in time")
	}

	key := fmt.Sprintf("%s/%s/%s:%d→%d", kind, namespace, name, session.LocalPort, remotePort)
	info := PortForwardInfo{
		Key: key, Kind: kind, Namespace: namespace, Name: name,
		PodName: session.PodName, LocalPort: session.LocalPort, RemotePort: remotePort,
		KeepRunning: keepRunning,
	}
	entry := &portForwardEntry{session: session, info: info}
	a.pfMu.Lock()
	if epoch != a.pfEpoch || cluster != a.pfCluster {
		a.pfMu.Unlock()
		session.Close()
		return nil, fmt.Errorf("port-forward start was superseded by a cluster change")
	}
	a.pfSessions[key] = entry
	a.pfMu.Unlock()

	// Report an async teardown if the tunnel dies later.
	go func(expected *portForwardEntry) {
		<-errCh
		a.pfMu.Lock()
		current, ok := a.pfSessions[key]
		if ok && current == expected {
			delete(a.pfSessions, key)
		}
		a.pfMu.Unlock()
		if ok && current == expected {
			wailsruntime.EventsEmit(a.ctx, "portforward-closed", key)
		}
	}(entry)

	return &info, nil
}

// StopPortForward tears down a forward by its key.
func (a *App) StopPortForward(key string) {
	a.pfMu.Lock()
	entry, ok := a.pfSessions[key]
	if ok {
		delete(a.pfSessions, key)
	}
	a.pfMu.Unlock()
	if ok && entry.session != nil {
		entry.session.Close()
	}
}

// ListPortForwards returns the currently active forwards.
func (a *App) ListPortForwards() []PortForwardInfo {
	a.pfMu.Lock()
	defer a.pfMu.Unlock()
	out := make([]PortForwardInfo, 0, len(a.pfSessions))
	for _, entry := range a.pfSessions {
		out = append(out, entry.info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// ---- Exec (interactive shell) ----

// StartExec opens an interactive shell into a container. Output is emitted as
// "exec-output" events; "exec-closed" fires when the shell exits.
func (a *App) StartExec(namespace, pod, container, shell string, cols, rows int) error {
	if err := a.requireCluster(); err != nil {
		return err
	}

	// Reserve a generation before opening the stream. Stop/switch can invalidate
	// the pending session without allowing its output into a newer drawer.
	a.execMu.Lock()
	previous := a.execSess
	a.execSess = nil
	a.execEpoch++
	epoch := a.execEpoch
	cluster := a.cluster
	a.execMu.Unlock()
	if previous != nil {
		previous.Close()
	}

	session, err := k8sclient.StartExec(a.ctx, cluster, namespace, pod, container, shell, cols, rows,
		func(out string) {
			a.execMu.Lock()
			current := a.execEpoch == epoch
			a.execMu.Unlock()
			if current {
				wailsruntime.EventsEmit(a.ctx, "exec-output", out)
			}
		},
		func(err error) {
			a.execMu.Lock()
			if a.execEpoch != epoch {
				a.execMu.Unlock()
				return
			}
			a.execSess = nil
			a.execEpoch++
			a.execMu.Unlock()
			msg := ""
			if err != nil {
				msg = err.Error()
			}
			wailsruntime.EventsEmit(a.ctx, "exec-closed", msg)
		})
	if err != nil {
		return err
	}
	a.execMu.Lock()
	if a.execEpoch != epoch {
		a.execMu.Unlock()
		session.Close()
		return fmt.Errorf("exec session was cancelled while connecting")
	}
	a.execSess = session
	a.execMu.Unlock()
	return nil
}

// ExecWrite forwards keystrokes to the active exec session's stdin.
func (a *App) ExecWrite(data string) error {
	a.execMu.Lock()
	session := a.execSess
	a.execMu.Unlock()
	if session == nil {
		return nil
	}
	return session.Write(data)
}

// ExecResize propagates xterm's measured dimensions to the active remote PTY.
func (a *App) ExecResize(cols, rows int) {
	a.execMu.Lock()
	session := a.execSess
	a.execMu.Unlock()
	if session != nil {
		session.Resize(cols, rows)
	}
}

// StopExec ends the active exec session, if any.
func (a *App) StopExec() {
	a.execMu.Lock()
	session := a.execSess
	a.execSess = nil
	a.execEpoch++
	a.execMu.Unlock()
	if session != nil {
		session.Close()
	}
}
