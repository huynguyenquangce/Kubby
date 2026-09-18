package main

import (
	"context"
	"encoding/json"
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
	ctx context.Context

	// transitionMu makes add/switch/disconnect one lifecycle boundary. Session
	// starts reserve their ownership through the same mutex, so a transition
	// cannot leave a newly-started stream attached to the previous connection.
	transitionMu       sync.Mutex
	stateMu            sync.RWMutex
	clusters           map[string]*clusterEntry // stable connection ID -> connection
	order              []string                 // connection IDs in dropdown order
	activeID           string
	connectionSeq      uint64
	connectionEpoch    uint64
	portForwardStarter portForwardStarter
	execStarter        execStarter
	logMu              sync.Mutex
	logCancel          context.CancelFunc // cancels the active log stream, if any
	logEpoch           uint64             // invalidates callbacks from an older stream
	pfMu               sync.Mutex
	pfSessions         map[string]*portForwardEntry
	pfPending          map[string]*pendingPortForward
	pfCluster          *k8sclient.Cluster
	pfEpoch            uint64
	execMu             sync.Mutex
	execSess           *k8sclient.ExecSession // the active exec session, if any
	execCancel         context.CancelFunc     // also cancels shell discovery before a session exists
	execEpoch          uint64                 // invalidates callbacks from an older shell
	recentMu           sync.Mutex
	recentPath         func() (string, error)
	helmMu             sync.Mutex
	helmOps            map[uint64]context.CancelFunc
	helmSeq            uint64
	drawerReadMu       sync.Mutex
	drawerReads        map[string]*drawerRead
}

type drawerRead struct {
	cancel context.CancelFunc
}

type portForwardStarter func(context.Context, *k8sclient.Cluster, string, string, string, int, int) (*k8sclient.PortForwardSession, <-chan struct{}, <-chan error, error)
type execStarter func(context.Context, *k8sclient.Cluster, string, string, string, string, int, int, func(string), func(error)) (*k8sclient.ExecSession, string, error)

type clusterEntry struct {
	id      string
	name    string
	context string
	cluster *k8sclient.Cluster
}

func NewApp() *App {
	return &App{
		ctx:                context.Background(),
		pfSessions:         map[string]*portForwardEntry{},
		pfPending:          map[string]*pendingPortForward{},
		clusters:           map[string]*clusterEntry{},
		portForwardStarter: k8sclient.StartPortForward,
		execStarter:        k8sclient.StartExec,
		recentPath:         recentFilePath,
		helmOps:            map[uint64]context.CancelFunc{},
		drawerReads:        map[string]*drawerRead{},
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) shutdown(context.Context) {
	a.cancelAllDrawerSnapshots()
	a.cancelHelmOperations()
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
	entry, err := a.verifyAndStore(kubeContext, cluster)
	if err != nil {
		return err
	}
	a.rememberConnection(entry.name, path, kubeContext)
	return nil
}

// ConnectWithContent connects to a cluster using pasted kubeconfig text (FR-1).
func (a *App) ConnectWithContent(content, kubeContext string) error {
	cluster, err := k8sclient.NewFromContent([]byte(content), kubeContext)
	if err != nil {
		return fmt.Errorf("could not build client: %w", err)
	}
	_, err = a.verifyAndStore(kubeContext, cluster)
	return err
}

func (a *App) verifyAndStore(name string, cluster *k8sclient.Cluster) (*clusterEntry, error) {
	// Prove transport/authentication without requiring permission to list a
	// cluster-scoped resource. Namespace-only service accounts are valid
	// connections; their narrower access is handled by the normal RBAC gating.
	if cluster.Discovery == nil {
		return nil, fmt.Errorf("connected but the discovery client is unavailable")
	}
	if _, err := cluster.Discovery.ServerVersion(); err != nil {
		return nil, fmt.Errorf("connected but the API discovery call failed: %w", err)
	}
	if name == "" {
		name = "default"
	}
	return a.storeVerifiedCluster(name, cluster), nil
}

func (a *App) storeVerifiedCluster(name string, cluster *k8sclient.Cluster) *clusterEntry {
	a.transitionMu.Lock()
	defer a.transitionMu.Unlock()

	// Invalidate all lifecycle state before publishing the replacement active
	// connection. New starts cannot interleave because they also reserve through
	// transitionMu.
	a.StopLogStream()
	a.StopExec()
	a.stopAllPortForwards()
	a.cancelHelmOperations()
	a.cancelAllDrawerSnapshots()

	a.stateMu.Lock()
	a.connectionSeq++
	id := fmt.Sprintf("connection-%d", a.connectionSeq)
	displayName := a.uniqueConnectionNameLocked(name)
	entry := &clusterEntry{id: id, name: displayName, context: name, cluster: cluster}
	a.clusters[id] = entry
	a.order = append(a.order, id)
	a.activeID = id
	a.connectionEpoch++
	a.stateMu.Unlock()
	a.resetPortForwardsForCluster(cluster)
	return entry
}

func (a *App) uniqueConnectionNameLocked(contextName string) string {
	count := 0
	for _, id := range a.order {
		if entry := a.clusters[id]; entry != nil && entry.context == contextName {
			count++
		}
	}
	if count == 0 {
		return contextName
	}
	return fmt.Sprintf("%s (%d)", contextName, count+1)
}

// ClusterInfo describes a connected cluster for the switcher dropdown.
type ClusterInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

// ConnectedClusters lists all currently-connected clusters (FR-6).
func (a *App) ConnectedClusters() []ClusterInfo {
	a.stateMu.RLock()
	defer a.stateMu.RUnlock()
	out := make([]ClusterInfo, 0, len(a.order))
	for _, id := range a.order {
		entry := a.clusters[id]
		if entry != nil {
			out = append(out, ClusterInfo{ID: id, Name: entry.name, Active: id == a.activeID})
		}
	}
	return out
}

// SwitchCluster makes an already-connected cluster the active one (FR-6).
func (a *App) SwitchCluster(id string) error {
	a.transitionMu.Lock()
	defer a.transitionMu.Unlock()

	a.stateMu.RLock()
	entry, ok := a.clusters[id]
	a.stateMu.RUnlock()
	if !ok {
		return fmt.Errorf("connection %q is not connected", id)
	}
	// Stop anything tied to the previous cluster.
	a.StopLogStream()
	a.StopExec()
	a.stopAllPortForwards()
	a.cancelHelmOperations()
	a.cancelAllDrawerSnapshots()
	a.stateMu.Lock()
	a.activeID = id
	a.connectionEpoch++
	a.stateMu.Unlock()
	a.resetPortForwardsForCluster(entry.cluster)
	return nil
}

// DisconnectCluster drops a connected cluster. Returns the name of the new
// active cluster ("" if none remain, meaning the UI should show Welcome).
func (a *App) DisconnectCluster(id string) string {
	a.transitionMu.Lock()
	defer a.transitionMu.Unlock()

	a.stateMu.RLock()
	_, exists := a.clusters[id]
	activeID := a.activeID
	a.stateMu.RUnlock()
	if !exists {
		return activeID
	}
	if id == activeID {
		a.StopLogStream()
		a.StopExec()
		a.stopAllPortForwards()
		a.cancelHelmOperations()
		a.cancelAllDrawerSnapshots()
	}

	a.stateMu.Lock()
	delete(a.clusters, id)
	for i, connectionID := range a.order {
		if connectionID == id {
			a.order = append(a.order[:i], a.order[i+1:]...)
			break
		}
	}
	var nextCluster *k8sclient.Cluster
	if id == a.activeID {
		if len(a.order) > 0 {
			a.activeID = a.order[0]
			nextCluster = a.clusters[a.activeID].cluster
		} else {
			a.activeID = ""
		}
		a.connectionEpoch++
	}
	nextID := a.activeID
	a.stateMu.Unlock()
	if id == activeID {
		a.resetPortForwardsForCluster(nextCluster)
	}
	return nextID
}

func (a *App) activeConnectionSnapshot() (id, name string, cluster *k8sclient.Cluster, epoch uint64) {
	a.stateMu.RLock()
	defer a.stateMu.RUnlock()
	entry := a.clusters[a.activeID]
	if entry == nil {
		return "", "", nil, a.connectionEpoch
	}
	return entry.id, entry.name, entry.cluster, a.connectionEpoch
}

func (a *App) requireCluster() (*k8sclient.Cluster, error) {
	_, _, cluster, _ := a.activeConnectionSnapshot()
	if cluster == nil {
		return nil, fmt.Errorf("not connected to any cluster")
	}
	return cluster, nil
}

func (a *App) requireExpectedCluster(expectedID string) (*k8sclient.Cluster, error) {
	a.stateMu.RLock()
	defer a.stateMu.RUnlock()
	entry := a.clusters[a.activeID]
	if entry == nil {
		return nil, fmt.Errorf("not connected to any cluster")
	}
	if expectedID == "" || entry.id != expectedID {
		return nil, fmt.Errorf("the active cluster changed before the action started")
	}
	return entry.cluster, nil
}

// The bound methods below are deliberately thin: check that a cluster is
// connected, then delegate to internal/k8sclient. These four helpers hold that
// check in one place, so a new binding cannot forget it — and so the context a
// call receives can be changed once rather than in every method.
//
// They are free functions because Go methods cannot take type parameters. The
// generic zero value returned on failure is exactly what the hand-written
// version returned: nil for slices and pointers, "" for strings.

// withCluster runs fn against the currently active cluster.
func withCluster[T any](a *App, fn func(context.Context, *k8sclient.Cluster) (T, error)) (T, error) {
	var zero T
	cluster, err := a.requireCluster()
	if err != nil {
		return zero, err
	}
	return fn(a.ctx, cluster)
}

// withClusterErr is withCluster for a call that returns only an error.
func withClusterErr(a *App, fn func(context.Context, *k8sclient.Cluster) error) error {
	cluster, err := a.requireCluster()
	if err != nil {
		return err
	}
	return fn(a.ctx, cluster)
}

// withOwnedCluster runs fn only while the expected connection is still the
// active one — the ownership contract every write crosses.
func withOwnedCluster[T any](a *App, expectedConnectionID string, fn func(context.Context, *k8sclient.Cluster) (T, error)) (T, error) {
	var zero T
	cluster, err := a.requireExpectedCluster(expectedConnectionID)
	if err != nil {
		return zero, err
	}
	return fn(a.ctx, cluster)
}

// withOwnedClusterErr is withOwnedCluster for a call that returns only an error.
func withOwnedClusterErr(a *App, expectedConnectionID string, fn func(context.Context, *k8sclient.Cluster) error) error {
	cluster, err := a.requireExpectedCluster(expectedConnectionID)
	if err != nil {
		return err
	}
	return fn(a.ctx, cluster)
}

func (a *App) ListNodes() ([]k8sclient.NodeInfo, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) ([]k8sclient.NodeInfo, error) {
		return k8sclient.ListNodes(ctx, cluster.Clientset)
	})
}

func (a *App) ListNamespaces() ([]k8sclient.NamespaceInfo, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) ([]k8sclient.NamespaceInfo, error) {
		return k8sclient.ListNamespaces(ctx, cluster.Clientset)
	})
}

// ListPods lists pods in a namespace ("" means all namespaces) (FR-2, FR-3).
func (a *App) ListPods(namespace string) ([]k8sclient.PodInfo, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) ([]k8sclient.PodInfo, error) {
		return k8sclient.ListPods(ctx, cluster.Clientset, namespace)
	})
}

// PodsSnapshot returns the Pods screen's rows and optional metrics in one
// bridge call. The client performs the independent Kubernetes reads in
// parallel.
func (a *App) PodsSnapshot(namespace string) (*k8sclient.PodsSnapshot, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) (*k8sclient.PodsSnapshot, error) {
		return k8sclient.ListPodsSnapshot(ctx, cluster, namespace)
	})
}

// PodsPage returns one bounded Pods-screen page and its continuation token.
func (a *App) PodsPage(namespace, continueToken string, limit int64) (*k8sclient.PodsPage, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) (*k8sclient.PodsPage, error) {
		return k8sclient.ListPodsPage(ctx, cluster, namespace, continueToken, limit)
	})
}

func (a *App) ListDeployments(namespace string) ([]k8sclient.DeploymentInfo, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) ([]k8sclient.DeploymentInfo, error) {
		return k8sclient.ListDeployments(ctx, cluster.Clientset, namespace)
	})
}

func (a *App) ListServices(namespace string) ([]k8sclient.ServiceInfo, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) ([]k8sclient.ServiceInfo, error) {
		return k8sclient.ListServices(ctx, cluster.Clientset, namespace)
	})
}

func (a *App) ListConfigMaps(namespace string) ([]k8sclient.ConfigMapInfo, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) ([]k8sclient.ConfigMapInfo, error) {
		return k8sclient.ListConfigMaps(ctx, cluster.Clientset, namespace)
	})
}

func (a *App) ListSecrets(namespace string) ([]k8sclient.SecretInfo, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) ([]k8sclient.SecretInfo, error) {
		return k8sclient.ListSecrets(ctx, cluster.Clientset, namespace)
	})
}

// GetDetail returns structured details for the drawer's Details tab.
func (a *App) GetDetail(kind, namespace, name string) (*k8sclient.ResourceDetail, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) (*k8sclient.ResourceDetail, error) {
		return k8sclient.GetDetail(ctx, cluster, kind, namespace, name)
	})
}

// GetDrawerSnapshotOwned returns the initial Details-tab evidence behind one
// bridge call; the backend overlaps independent Kubernetes reads. Both the
// connection and operation ID bind the response to the drawer that requested it.
func (a *App) GetDrawerSnapshotOwned(expectedConnectionID, operationID, kind, namespace, name string) (string, error) {
	cluster, err := a.requireExpectedCluster(expectedConnectionID)
	if err != nil {
		return "", err
	}
	ctx, done, err := a.beginDrawerSnapshot(operationID)
	if err != nil {
		return "", err
	}
	defer done()
	snapshot, err := k8sclient.GetDrawerSnapshot(ctx, cluster, kind, namespace, name)
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(snapshot)
	return string(payload), err
}

func (a *App) beginDrawerSnapshot(operationID string) (context.Context, func(), error) {
	if strings.TrimSpace(operationID) == "" {
		return nil, nil, fmt.Errorf("drawer snapshot operation ID is required")
	}
	ctx, cancel := context.WithCancel(a.ctx)
	read := &drawerRead{cancel: cancel}
	a.drawerReadMu.Lock()
	if _, exists := a.drawerReads[operationID]; exists {
		a.drawerReadMu.Unlock()
		cancel()
		return nil, nil, fmt.Errorf("drawer snapshot operation %q is already running", operationID)
	}
	a.drawerReads[operationID] = read
	a.drawerReadMu.Unlock()
	done := func() {
		a.drawerReadMu.Lock()
		if a.drawerReads[operationID] == read {
			delete(a.drawerReads, operationID)
		}
		a.drawerReadMu.Unlock()
		cancel()
	}
	return ctx, done, nil
}

func (a *App) CancelDrawerSnapshot(operationID string) {
	a.drawerReadMu.Lock()
	read := a.drawerReads[operationID]
	delete(a.drawerReads, operationID)
	a.drawerReadMu.Unlock()
	if read != nil {
		read.cancel()
	}
}

func (a *App) cancelAllDrawerSnapshots() {
	a.drawerReadMu.Lock()
	cancels := make([]context.CancelFunc, 0, len(a.drawerReads))
	for operationID, read := range a.drawerReads {
		cancels = append(cancels, read.cancel)
		delete(a.drawerReads, operationID)
	}
	a.drawerReadMu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}

// ListEvents returns events involving a specific resource (like kubectl describe).
func (a *App) ListEvents(kind, namespace, name string) ([]k8sclient.EventInfo, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) ([]k8sclient.EventInfo, error) {
		return k8sclient.ListEvents(ctx, cluster, kind, namespace, name)
	})
}

func (a *App) DeleteResourceOwned(expectedConnectionID, kind, namespace, name string) error {
	return withOwnedClusterErr(a, expectedConnectionID, func(ctx context.Context, cluster *k8sclient.Cluster) error {
		return k8sclient.DeleteResource(ctx, cluster, kind, namespace, name)
	})
}

// ---- Feature 1: deployment actions ----

func (a *App) ScaleDeploymentOwned(expectedConnectionID, namespace, name string, replicas int) error {
	return withOwnedClusterErr(a, expectedConnectionID, func(ctx context.Context, cluster *k8sclient.Cluster) error {
		return k8sclient.ScaleDeployment(ctx, cluster, namespace, name, int32(replicas))
	})
}

func (a *App) RestartDeploymentOwned(expectedConnectionID, namespace, name string) error {
	return withOwnedClusterErr(a, expectedConnectionID, func(ctx context.Context, cluster *k8sclient.Cluster) error {
		return k8sclient.RestartDeployment(ctx, cluster, namespace, name, time.Now().UTC().Format(time.RFC3339Nano))
	})
}

func (a *App) RestartStatefulSetOwned(expectedConnectionID, namespace, name string) error {
	return withOwnedClusterErr(a, expectedConnectionID, func(ctx context.Context, cluster *k8sclient.Cluster) error {
		return k8sclient.RestartStatefulSet(ctx, cluster, namespace, name, time.Now().UTC().Format(time.RFC3339Nano))
	})
}

func (a *App) RestartDaemonSetOwned(expectedConnectionID, namespace, name string) error {
	return withOwnedClusterErr(a, expectedConnectionID, func(ctx context.Context, cluster *k8sclient.Cluster) error {
		return k8sclient.RestartDaemonSet(ctx, cluster, namespace, name, time.Now().UTC().Format(time.RFC3339Nano))
	})
}

func (a *App) SetDeploymentPausedOwned(expectedConnectionID, namespace, name string, paused bool) error {
	return withOwnedClusterErr(a, expectedConnectionID, func(ctx context.Context, cluster *k8sclient.Cluster) error {
		return k8sclient.SetDeploymentPaused(ctx, cluster, namespace, name, paused)
	})
}

func (a *App) RolloutHistory(namespace, name string) ([]k8sclient.RolloutRevision, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) ([]k8sclient.RolloutRevision, error) {
		return k8sclient.RolloutHistory(ctx, cluster, namespace, name)
	})
}

func (a *App) RollbackDeploymentOwned(expectedConnectionID, namespace, name string, revision int64) error {
	return withOwnedClusterErr(a, expectedConnectionID, func(ctx context.Context, cluster *k8sclient.Cluster) error {
		return k8sclient.RollbackDeployment(ctx, cluster, namespace, name, revision)
	})
}

// ---- Node actions ----

func (a *App) SetNodeSchedulableOwned(expectedConnectionID, name string, schedulable bool) error {
	return withOwnedClusterErr(a, expectedConnectionID, func(ctx context.Context, cluster *k8sclient.Cluster) error {
		return k8sclient.SetNodeSchedulable(ctx, cluster, name, schedulable)
	})
}

func (a *App) DrainNodeOwned(expectedConnectionID, name string) error {
	return withOwnedClusterErr(a, expectedConnectionID, func(ctx context.Context, cluster *k8sclient.Cluster) error {
		return k8sclient.DrainNode(ctx, cluster, name)
	})
}

// ---- CronJob run-now ----

func (a *App) RunCronJobNowOwned(expectedConnectionID, namespace, name string) error {
	return withOwnedClusterErr(a, expectedConnectionID, func(ctx context.Context, cluster *k8sclient.Cluster) error {
		return k8sclient.RunCronJobNow(ctx, cluster, namespace, name)
	})
}

// ---- Secret reveal ----

func (a *App) SecretData(namespace, name string) ([]k8sclient.SecretEntry, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) ([]k8sclient.SecretEntry, error) {
		return k8sclient.SecretData(ctx, cluster, namespace, name)
	})
}

// ---- Feature 3: more resource types ----

func (a *App) ListStatefulSets(ns string) ([]k8sclient.StatefulSetInfo, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) ([]k8sclient.StatefulSetInfo, error) {
		return k8sclient.ListStatefulSets(ctx, cluster.Clientset, ns)
	})
}
func (a *App) ListDaemonSets(ns string) ([]k8sclient.DaemonSetInfo, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) ([]k8sclient.DaemonSetInfo, error) {
		return k8sclient.ListDaemonSets(ctx, cluster.Clientset, ns)
	})
}
func (a *App) ListJobs(ns string) ([]k8sclient.JobInfo, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) ([]k8sclient.JobInfo, error) {
		return k8sclient.ListJobs(ctx, cluster.Clientset, ns)
	})
}
func (a *App) ListCronJobs(ns string) ([]k8sclient.CronJobInfo, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) ([]k8sclient.CronJobInfo, error) {
		return k8sclient.ListCronJobs(ctx, cluster.Clientset, ns)
	})
}
func (a *App) ListIngresses(ns string) ([]k8sclient.IngressInfo, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) ([]k8sclient.IngressInfo, error) {
		return k8sclient.ListIngresses(ctx, cluster.Clientset, ns)
	})
}
func (a *App) ListPVCs(ns string) ([]k8sclient.PVCInfo, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) ([]k8sclient.PVCInfo, error) {
		return k8sclient.ListPVCs(ctx, cluster.Clientset, ns)
	})
}
func (a *App) ListServiceAccounts(ns string) ([]k8sclient.ServiceAccountInfo, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) ([]k8sclient.ServiceAccountInfo, error) {
		return k8sclient.ListServiceAccounts(ctx, cluster.Clientset, ns)
	})
}

// ---- Storage + RBAC resource types ----

func (a *App) ListPersistentVolumes() ([]k8sclient.PersistentVolumeInfo, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) ([]k8sclient.PersistentVolumeInfo, error) {
		return k8sclient.ListPersistentVolumes(ctx, cluster.Clientset)
	})
}
func (a *App) ListStorageClasses() ([]k8sclient.StorageClassInfo, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) ([]k8sclient.StorageClassInfo, error) {
		return k8sclient.ListStorageClasses(ctx, cluster.Clientset)
	})
}
func (a *App) ListRoles(ns string) ([]k8sclient.RoleInfo, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) ([]k8sclient.RoleInfo, error) {
		return k8sclient.ListRoles(ctx, cluster.Clientset, ns)
	})
}
func (a *App) ListRoleBindings(ns string) ([]k8sclient.RoleBindingInfo, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) ([]k8sclient.RoleBindingInfo, error) {
		return k8sclient.ListRoleBindings(ctx, cluster.Clientset, ns)
	})
}
func (a *App) ListClusterRoles() ([]k8sclient.ClusterRoleInfo, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) ([]k8sclient.ClusterRoleInfo, error) {
		return k8sclient.ListClusterRoles(ctx, cluster.Clientset)
	})
}
func (a *App) ListClusterRoleBindings() ([]k8sclient.ClusterRoleBindingInfo, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) ([]k8sclient.ClusterRoleBindingInfo, error) {
		return k8sclient.ListClusterRoleBindings(ctx, cluster.Clientset)
	})
}

// ---- Ecosystem: CRDs, Helm, quotas ----

func (a *App) ListCRDs() ([]k8sclient.CRDInfo, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) ([]k8sclient.CRDInfo, error) {
		return k8sclient.ListCRDs(ctx, cluster)
	})
}
func (a *App) ListHelmReleases(ns string) ([]k8sclient.HelmReleaseInfo, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) ([]k8sclient.HelmReleaseInfo, error) {
		return k8sclient.ListHelmReleases(ctx, cluster.Meta, ns)
	})
}

// ---- Helm SDK: release management + search + install ----

const helmAppOperationTimeout = 5 * time.Minute

func (a *App) beginHelmOperation(expectedConnectionID string) (context.Context, *k8sclient.Cluster, func(), error) {
	a.transitionMu.Lock()
	var cluster *k8sclient.Cluster
	var err error
	if expectedConnectionID == "" {
		cluster, err = a.requireCluster()
	} else {
		cluster, err = a.requireExpectedCluster(expectedConnectionID)
	}
	if err != nil {
		a.transitionMu.Unlock()
		return nil, nil, nil, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, helmAppOperationTimeout)
	a.helmMu.Lock()
	a.helmSeq++
	id := a.helmSeq
	a.helmOps[id] = cancel
	a.helmMu.Unlock()
	a.transitionMu.Unlock()
	return ctx, cluster, func() {
		a.helmMu.Lock()
		if registered := a.helmOps[id]; registered != nil {
			delete(a.helmOps, id)
			registered()
		}
		a.helmMu.Unlock()
	}, nil
}

// beginOwnedHelmOperation is the write-side entry point. Unlike read snapshots,
// a Helm mutation must never fall back to whichever cluster happens to be active
// when a delayed renderer call arrives.
func (a *App) beginOwnedHelmOperation(expectedConnectionID string) (context.Context, *k8sclient.Cluster, func(), error) {
	if expectedConnectionID == "" {
		return nil, nil, nil, fmt.Errorf("the expected connection ID is required for a Helm write")
	}
	return a.beginHelmOperation(expectedConnectionID)
}

func (a *App) cancelHelmOperations() {
	a.helmMu.Lock()
	operations := a.helmOps
	a.helmOps = map[uint64]context.CancelFunc{}
	a.helmMu.Unlock()
	for _, cancel := range operations {
		cancel()
	}
}

func (a *App) HelmGet(namespace, name string) (*k8sclient.HelmReleaseDetail, error) {
	ctx, cluster, done, err := a.beginHelmOperation("")
	if err != nil {
		return nil, err
	}
	defer done()
	return k8sclient.HelmGet(ctx, cluster, namespace, name)
}
func (a *App) HelmSnapshot(namespace, name string) (*k8sclient.HelmReleaseSnapshot, error) {
	ctx, cluster, done, err := a.beginHelmOperation("")
	if err != nil {
		return nil, err
	}
	defer done()
	return k8sclient.HelmSnapshot(ctx, cluster, namespace, name)
}
func (a *App) HelmHistory(namespace, name string) ([]k8sclient.HelmRevision, error) {
	ctx, cluster, done, err := a.beginHelmOperation("")
	if err != nil {
		return nil, err
	}
	defer done()
	return k8sclient.HelmHistory(ctx, cluster, namespace, name)
}
func (a *App) HelmRollbackOwned(expectedConnectionID, namespace, name string, revision int) error {
	ctx, cluster, done, err := a.beginOwnedHelmOperation(expectedConnectionID)
	if err != nil {
		return err
	}
	defer done()
	return k8sclient.HelmRollback(ctx, cluster, namespace, name, revision)
}
func (a *App) HelmUninstallOwned(expectedConnectionID, namespace, name string) error {
	ctx, cluster, done, err := a.beginOwnedHelmOperation(expectedConnectionID)
	if err != nil {
		return err
	}
	defer done()
	return k8sclient.HelmUninstall(ctx, cluster, namespace, name)
}
func (a *App) HelmUpgradeValuesOwned(expectedConnectionID, namespace, name, valuesYAML string, expectedRevision int, expectedValuesDigest string) error {
	ctx, cluster, done, err := a.beginOwnedHelmOperation(expectedConnectionID)
	if err != nil {
		return err
	}
	defer done()
	return k8sclient.HelmUpgradeValues(ctx, cluster, namespace, name, valuesYAML, expectedRevision, expectedValuesDigest)
}
func (a *App) HelmInstallOwned(expectedConnectionID, namespace, releaseName, repoURL, repoName, chartName, version, valuesYAML, expectedDigest string) error {
	ctx, cluster, done, err := a.beginOwnedHelmOperation(expectedConnectionID)
	if err != nil {
		return err
	}
	defer done()
	return k8sclient.HelmInstall(ctx, cluster, namespace, releaseName, repoURL, repoName, chartName, version, valuesYAML, expectedDigest)
}

// SearchCharts queries Artifact Hub (needs internet). No cluster required.
func (a *App) SearchCharts(query string) ([]k8sclient.ChartSearchResult, error) {
	return k8sclient.SearchCharts(a.ctx, query)
}
func (a *App) ChartDetails(repo, chartName string) (*k8sclient.ChartDetail, error) {
	return k8sclient.ChartDetails(a.ctx, repo, chartName)
}
func (a *App) ChartDefaultValues(repoURL, repoName, chartName, version string) (string, error) {
	return k8sclient.ChartDefaultValues(repoURL, repoName, chartName, version)
}
func (a *App) HelmInstallPreview(namespace, releaseName, repoURL, repoName, chartName, version, valuesYAML string) (*k8sclient.HelmDiff, error) {
	ctx, cluster, done, err := a.beginHelmOperation("")
	if err != nil {
		return nil, err
	}
	defer done()
	return k8sclient.HelmInstallPreview(ctx, cluster, namespace, releaseName, repoURL, repoName, chartName, version, valuesYAML)
}
func (a *App) HelmUpgradePreview(namespace, name, valuesYAML string) (*k8sclient.HelmDiff, error) {
	ctx, cluster, done, err := a.beginHelmOperation("")
	if err != nil {
		return nil, err
	}
	defer done()
	return k8sclient.HelmUpgradePreview(ctx, cluster, namespace, name, valuesYAML)
}
func (a *App) HelmGetRevision(namespace, name string, revision int) (*k8sclient.HelmReleaseDetail, error) {
	ctx, cluster, done, err := a.beginHelmOperation("")
	if err != nil {
		return nil, err
	}
	defer done()
	return k8sclient.HelmGetRevision(ctx, cluster, namespace, name, revision)
}

func (a *App) PlanHelmPermissions(operation, namespace, name string, revision int) (*k8sclient.PermissionPlan, error) {
	ctx, cluster, done, err := a.beginHelmOperation("")
	if err != nil {
		return nil, err
	}
	defer done()
	return k8sclient.PlanHelmAction(ctx, cluster, operation, namespace, name, revision)
}
func (a *App) HelmReleaseResources(namespace, name string) ([]k8sclient.HelmResource, error) {
	ctx, cluster, done, err := a.beginHelmOperation("")
	if err != nil {
		return nil, err
	}
	defer done()
	return k8sclient.HelmReleaseResources(ctx, cluster, namespace, name)
}
func (a *App) HelmTestOwned(expectedConnectionID, namespace, name string) (string, error) {
	ctx, cluster, done, err := a.beginOwnedHelmOperation(expectedConnectionID)
	if err != nil {
		return "", err
	}
	defer done()
	return k8sclient.HelmTest(ctx, cluster, namespace, name)
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
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) ([]k8sclient.ResourceQuotaInfo, error) {
		return k8sclient.ListResourceQuotas(ctx, cluster.Clientset, ns)
	})
}
func (a *App) ListLimitRanges(ns string) ([]k8sclient.LimitRangeInfo, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) ([]k8sclient.LimitRangeInfo, error) {
		return k8sclient.ListLimitRanges(ctx, cluster.Clientset, ns)
	})
}

// ---- Scaling, disruption and network policy ----

func (a *App) ListHorizontalPodAutoscalers(ns string) ([]k8sclient.HPAInfo, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) ([]k8sclient.HPAInfo, error) {
		return k8sclient.ListHorizontalPodAutoscalers(ctx, cluster.Clientset, ns)
	})
}
func (a *App) ListPodDisruptionBudgets(ns string) ([]k8sclient.PDBInfo, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) ([]k8sclient.PDBInfo, error) {
		return k8sclient.ListPodDisruptionBudgets(ctx, cluster.Clientset, ns)
	})
}
func (a *App) ListNetworkPolicies(ns string) ([]k8sclient.NetworkPolicyInfo, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) ([]k8sclient.NetworkPolicyInfo, error) {
		return k8sclient.ListNetworkPolicies(ctx, cluster.Clientset, ns)
	})
}

// CheckTrafficPolicy answers "does NetworkPolicy let this Pod reach that Pod or
// Service on this port?" for the Topology view. Read-only.
func (a *App) CheckTrafficPolicy(sourceNamespace, sourcePod, destinationKind, destinationNamespace, destinationName string, port int, protocol string) (*k8sclient.TrafficCheckResult, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) (*k8sclient.TrafficCheckResult, error) {
		return k8sclient.CheckTrafficPolicy(ctx, cluster, sourceNamespace, sourcePod, destinationKind, destinationNamespace, destinationName, port, protocol)
	})
}

// ClusterChecks runs the Health checks screen — admission webhooks,
// certificates and stuck deletions — in one read-only call.
func (a *App) ClusterChecks(namespace string) (*k8sclient.ClusterChecksReport, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) (*k8sclient.ClusterChecksReport, error) {
		return k8sclient.ClusterChecks(ctx, cluster, namespace)
	})
}

// DrainImpact previews what draining a Node would do — refusing budgets,
// unmanaged Pods, emptyDir data — for the Drain confirmation. Read-only.
func (a *App) DrainImpact(nodeName string) (*k8sclient.DrainImpact, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) (*k8sclient.DrainImpact, error) {
		return k8sclient.DrainImpactFor(ctx, cluster, nodeName)
	})
}

// ---- Feature 4: relations + metrics ----

// OverviewSnapshot returns the complete dashboard payload in one bound call;
// Kubernetes fan-out and partial-error handling live in k8sclient.
func (a *App) OverviewSnapshot() (*k8sclient.OverviewData, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) (*k8sclient.OverviewData, error) {
		return k8sclient.OverviewSnapshot(ctx, cluster)
	})
}

// InvestigateResource returns one evidence-first incident snapshot. The deep
// playbook currently targets Pods; other kinds degrade to retained Events.
func (a *App) InvestigateResource(kind, namespace, name string) (string, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) (string, error) {
		report, err := k8sclient.InvestigateResource(ctx, cluster, kind, namespace, name)
		if err != nil {
			return "", err
		}
		payload, err := json.Marshal(report)
		return string(payload), err
	})
}

func (a *App) DeploymentTree(namespace, name string) (*k8sclient.RelationNode, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) (*k8sclient.RelationNode, error) {
		return k8sclient.DeploymentTree(ctx, cluster, namespace, name)
	})
}

func (a *App) NodeMetrics() ([]k8sclient.NodeMetric, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) ([]k8sclient.NodeMetric, error) {
		return k8sclient.NodeMetrics(ctx, cluster)
	})
}

// TopPods returns the top N pods by CPU usage (Overview dashboard).
func (a *App) TopPods(limit int) ([]k8sclient.PodMetric, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) ([]k8sclient.PodMetric, error) {
		return k8sclient.TopPods(ctx, cluster, limit)
	})
}

// PodMetricsList returns per-pod CPU/mem for a namespace (merged into the pods table).
func (a *App) PodMetricsList(namespace string) ([]k8sclient.PodMetric, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) ([]k8sclient.PodMetric, error) {
		return k8sclient.PodMetricsList(ctx, cluster, namespace)
	})
}

// ClusterEvents returns the most recent events across the cluster (Overview).
func (a *App) ClusterEvents(limit int) ([]k8sclient.EventInfo, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) ([]k8sclient.EventInfo, error) {
		return k8sclient.ClusterEvents(ctx, cluster, limit)
	})
}

// ServiceTree returns a Service → Pod relations tree.
func (a *App) ServiceTree(namespace, name string) (*k8sclient.RelationNode, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) (*k8sclient.RelationNode, error) {
		return k8sclient.ServiceTree(ctx, cluster, namespace, name)
	})
}

// IngressTree returns an Ingress → Service → Pod relations tree.
func (a *App) IngressTree(namespace, name string) (*k8sclient.RelationNode, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) (*k8sclient.RelationNode, error) {
		return k8sclient.IngressTree(ctx, cluster, namespace, name)
	})
}

// ---- Explore: node pods, namespace summary, global search, network topology ----

func (a *App) PodsOnNode(nodeName string) ([]k8sclient.PodInfo, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) ([]k8sclient.PodInfo, error) {
		return k8sclient.PodsOnNode(ctx, cluster, nodeName)
	})
}

func (a *App) NamespaceSummary(ns string) ([]k8sclient.NsKindCount, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) ([]k8sclient.NsKindCount, error) {
		return k8sclient.NamespaceSummary(ctx, cluster, ns)
	})
}

func (a *App) SearchResources(query string) ([]k8sclient.SearchHit, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) ([]k8sclient.SearchHit, error) {
		return k8sclient.SearchResources(ctx, cluster, query)
	})
}

// SidebarCounts returns every sidebar tally in one call. The frontend used to
// make one bound call per kind (~25 round-trips, each serialising every object
// just to take .length), which is what made switching namespace stall.
// includeCluster=false skips the cluster-scoped kinds, whose counts cannot
// change when only the namespace does.
func (a *App) SidebarCounts(namespace string, includeCluster bool) ([]k8sclient.NavCount, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) ([]k8sclient.NavCount, error) {
		return k8sclient.SidebarCounts(ctx, cluster, namespace, includeCluster)
	})
}

// CustomKinds lists the kinds this cluster's CRDs define, so each becomes its own
// sidebar section (and command-palette entry). No counts — see CustomKinds' doc.
func (a *App) CustomKinds() (*k8sclient.CustomKindList, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) (*k8sclient.CustomKindList, error) {
		return k8sclient.CustomKinds(ctx, cluster)
	})
}

// ListCustom lists the objects of a custom kind ("Kind.group"), namespace "" = all.
func (a *App) ListCustom(refKind, namespace string) ([]k8sclient.CustomObject, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) ([]k8sclient.CustomObject, error) {
		return k8sclient.ListCustom(ctx, cluster, refKind, namespace)
	})
}

func (a *App) ListCustomPage(refKind, namespace, continueToken string, limit int64) (*k8sclient.CustomObjectPage, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) (*k8sclient.CustomObjectPage, error) {
		return k8sclient.ListCustomPage(ctx, cluster, refKind, namespace, continueToken, limit)
	})
}

func (a *App) NetworkFlows(namespace string) (*k8sclient.NetworkFlows, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) (*k8sclient.NetworkFlows, error) {
		return k8sclient.NetworkTopology(ctx, cluster, namespace)
	})
}

// ClusterStructure returns the debug topology in one bound call. The backend
// owns the cross-resource joins so the WebView never talks to Kubernetes or
// creates an N+1 request pattern.
func (a *App) ClusterStructure(namespace string) (*k8sclient.ClusterStructureData, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) (*k8sclient.ClusterStructureData, error) {
		return k8sclient.ClusterStructure(ctx, cluster, namespace)
	})
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
	_, localEndpointErr := localOllamaBaseURL(cfg.Endpoint)
	return AIStatus{
		Configured: cfg.Provider != "",
		Provider:   cfg.Provider,
		Label:      providerLabel(cfg.Provider),
		Model:      model,
		Local:      cfg.Provider == "ollama" && localEndpointErr == nil,
	}
}

// AIResourceContext returns the exact evidence Kubby would send about a
// resource, so the assistant can show it before anything is transmitted.
func (a *App) AIResourceContext(kind, namespace, name string) (*k8sclient.AIContext, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) (*k8sclient.AIContext, error) {
		return k8sclient.DiagnosticContext(ctx, cluster, kind, namespace, name)
	})
}

// AskAboutResource answers a question about one resource. The resource's
// events/logs/YAML go into the system prompt rather than the thread, so every
// follow-up is grounded in the same snapshot and the frontend only has to replay
// the visible conversation.
func (a *App) AskAboutResource(kind, namespace, name, evidence string, history []AIMessage) (string, error) {
	if _, err := a.requireCluster(); err != nil {
		return "", err
	}
	cfg := loadAIConfig()
	if cfg.Provider == "" {
		return "", fmt.Errorf("no AI provider configured — open Settings (⚙) to pick one")
	}
	if len(history) == 0 {
		return "", fmt.Errorf("no question to ask")
	}
	if strings.TrimSpace(evidence) == "" {
		return "", fmt.Errorf("resource evidence is not ready — wait for the attachment summary and try again")
	}
	const maxAIEvidence = 32 << 10
	if len(evidence) > maxAIEvidence {
		return "", fmt.Errorf("resource evidence exceeded the 32 KiB safety limit")
	}

	where := kind + " " + name
	if namespace != "" {
		where += " in namespace " + namespace
	}
	system := aiResourceSystemPrompt(where, evidence, cfg.Language)

	return callAI(a.ctx, cfg, system, history)
}

func aiResourceSystemPrompt(where, evidence, language string) string {
	return strings.Join([]string{
		"You are a senior Kubernetes/DevOps engineer helping someone inspect a live cluster",
		"from a desktop UI. The user is looking at " + where + ".",
		"",
		"Answer only from the evidence below plus general Kubernetes knowledge. If the evidence",
		"does not contain the answer, say so and name the command or view that would reveal it —",
		"never invent field values, log lines or events.",
		"",
		"Be concise and practical: short paragraphs or bullets, concrete kubectl commands in",
		"backticks, and the single most likely cause first. Use markdown. " + languageRule(language),
		"",
		"--- EVIDENCE (snapshot reviewed in the resource drawer) ---",
		evidence,
	}, "\n")
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

type ExecOutputEvent struct {
	SessionID string `json:"sessionId"`
	Data      string `json:"data"`
}

type ExecClosedEvent struct {
	SessionID string `json:"sessionId"`
	Message   string `json:"message"`
}

// StartLogStream begins following a container's logs. Batches are emitted as
// "loglines" events; the stream stops on StopLogStream, a new stream, or
// disconnect.
func (a *App) StartLogStream(namespace, name, container, streamID string) error {
	if strings.TrimSpace(streamID) == "" {
		return fmt.Errorf("log stream ID is required")
	}

	a.transitionMu.Lock()
	cluster, err := a.requireCluster()
	if err != nil {
		a.transitionMu.Unlock()
		return err
	}
	a.logMu.Lock()
	if a.logCancel != nil {
		a.logCancel()
	}
	a.logEpoch++
	epoch := a.logEpoch
	ctx, cancel := context.WithCancel(a.ctx)
	a.logCancel = cancel
	a.logMu.Unlock()
	a.transitionMu.Unlock()

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

// SaveIncidentReport writes the bounded IncidentReport formatter rather than
// accepting arbitrary evidence from the WebView. The report intentionally
// contains no manifests, credentials, Secret values or raw application logs.
func (a *App) SaveIncidentReport(reportJSON string) (string, error) {
	var report k8sclient.IncidentReport
	if err := json.Unmarshal([]byte(reportJSON), &report); err != nil {
		return "", fmt.Errorf("decode incident report: %w", err)
	}
	defaultName := fmt.Sprintf("kubby-incident-%s-%s.md", strings.ToLower(report.Kind), safeFilename(report.Name))
	path, err := wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		DefaultFilename: defaultName,
		Title:           "Save incident report",
	})
	if err != nil || path == "" {
		return "", err
	}
	if err := os.WriteFile(path, []byte(k8sclient.FormatIncidentMarkdown(&report)), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func safeFilename(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "resource"
	}
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			return r
		}
		return '-'
	}, value)
}

// GetYAML returns a resource rendered as YAML for the detail/edit panel.
func (a *App) GetYAML(kind, namespace, name string) (string, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) (string, error) {
		return k8sclient.GetYAML(ctx, cluster, kind, namespace, name)
	})
}

// UpdateYAML applies edited YAML only when the active cluster and resource still
// match the drawer that loaded it.  The local cluster pointer is captured after
// the check so a later UI switch cannot redirect this call to the new cluster.
func (a *App) UpdateYAML(expectedCluster, expectedKind, expectedNamespace, expectedName, yamlText string) error {
	activeID, activeName, cluster, _ := a.activeConnectionSnapshot()
	if cluster == nil {
		return fmt.Errorf("not connected to any cluster")
	}
	if expectedCluster == "" || expectedCluster != activeID {
		return fmt.Errorf("refusing stale YAML update: expected connection %q, active connection is %q (%s); reload the open resource", expectedCluster, activeID, activeName)
	}
	return k8sclient.UpdateYAML(a.ctx, cluster, expectedKind, expectedNamespace, expectedName, yamlText)
}

// ApplyYAMLOwned creates or updates YAML only on the connection that owned the
// Create/Import modal when it opened. Capturing the cluster before the write
// prevents a concurrent switch from redirecting the manifest.
func (a *App) ApplyYAMLOwned(expectedCluster, yamlText string) (string, error) {
	return withOwnedCluster(a, expectedCluster, func(ctx context.Context, cluster *k8sclient.Cluster) (string, error) {
		return k8sclient.ApplyYAML(ctx, cluster, yamlText)
	})
}

// ApplyPreview answers "what would this YAML change?" without changing anything,
// via a server-side dry run diffed against the live objects. Backs the Preview
// step of "Import YAML", the same way HelmUpgradePreview backs Helm's.
func (a *App) ApplyPreview(yamlText string) (*k8sclient.ApplyDiff, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) (*k8sclient.ApplyDiff, error) {
		return k8sclient.ApplyPreview(ctx, cluster, yamlText)
	})
}

func (a *App) PlanApplyPermissions(yamlText string) (*k8sclient.PermissionPlan, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) (*k8sclient.PermissionPlan, error) {
		return k8sclient.PlanApplyPermissions(ctx, cluster, yamlText)
	})
}

func (a *App) PlanDrainPermissions(nodeName string) (*k8sclient.PermissionPlan, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) (*k8sclient.PermissionPlan, error) {
		return k8sclient.PlanDrainPermissions(ctx, cluster, nodeName)
	})
}

// Sizing reports declared requests/limits against actual usage for a namespace
// ("" = whole cluster). Backs the Right-sizing view.
func (a *App) Sizing(namespace string) (*k8sclient.SizingReport, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) (*k8sclient.SizingReport, error) {
		return k8sclient.Sizing(ctx, cluster, namespace)
	})
}

// CanI reports what the connected token may do to a kind in a namespace, so the
// UI can disable actions instead of offering them and failing at the API.
func (a *App) CanI(kind, namespace string) (*k8sclient.AccessSet, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) (*k8sclient.AccessSet, error) {
		return k8sclient.CanI(ctx, cluster, kind, namespace)
	})
}

// PodContainers lists a pod's container names (for the log container picker).
func (a *App) PodContainers(namespace, name string) ([]string, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) ([]string, error) {
		return k8sclient.PodContainers(ctx, cluster, namespace, name)
	})
}

// PodContainerStates lists a pod's containers with restart and last-exit state,
// shared by the Logs and Terminal tabs through one drawer request.
func (a *App) PodContainerStates(namespace, name string) ([]k8sclient.ContainerState, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) ([]k8sclient.ContainerState, error) {
		return k8sclient.PodContainerStates(ctx, cluster, namespace, name)
	})
}

// PodLogs returns the last `tailLines` log lines for a pod container; previous
// reads the container instance from before its last restart.
func (a *App) PodLogs(namespace, name, container string, tailLines int, previous bool) (string, error) {
	return withCluster(a, func(ctx context.Context, cluster *k8sclient.Cluster) (string, error) {
		return k8sclient.PodLogs(ctx, cluster, namespace, name, container, int64(tailLines), previous)
	})
}

// ---- Port-forward ----

// PortForwardInfo describes an active forward for the UI.
type PortForwardInfo struct {
	ConnectionID string `json:"connectionId"`
	Key          string `json:"key"`
	Kind         string `json:"kind"`
	Namespace    string `json:"namespace"`
	Name         string `json:"name"`
	PodName      string `json:"podName"`
	LocalPort    int    `json:"localPort"`
	RemotePort   int    `json:"remotePort"`
	KeepRunning  bool   `json:"keepRunning"`
}

type portForwardEntry struct {
	session *k8sclient.PortForwardSession
	info    PortForwardInfo
}

type pendingPortForward struct {
	cancel  context.CancelFunc
	session *k8sclient.PortForwardSession
	cluster *k8sclient.Cluster
	epoch   uint64
}

type PortForwardClosedEvent struct {
	ConnectionID string `json:"connectionId"`
	Key          string `json:"key"`
}

func (a *App) detachPortForwardsLocked() ([]*k8sclient.PortForwardSession, []context.CancelFunc) {
	sessions := make([]*k8sclient.PortForwardSession, 0, len(a.pfSessions))
	for _, entry := range a.pfSessions {
		if entry.session != nil {
			sessions = append(sessions, entry.session)
		}
	}
	cancels := make([]context.CancelFunc, 0, len(a.pfPending))
	for _, pending := range a.pfPending {
		if pending.cancel != nil {
			cancels = append(cancels, pending.cancel)
		}
		if pending.session != nil {
			sessions = append(sessions, pending.session)
		}
	}
	a.pfSessions = map[string]*portForwardEntry{}
	a.pfPending = map[string]*pendingPortForward{}
	a.pfEpoch++
	return sessions, cancels
}

func closePortForwardSessions(sessions []*k8sclient.PortForwardSession, cancels []context.CancelFunc) {
	for _, cancel := range cancels {
		cancel()
	}
	for _, session := range sessions {
		session.Close()
	}
}

// resetPortForwardsForCluster atomically invalidates pending starts, changes the
// cluster new starts belong to, and detaches every existing tunnel.
func (a *App) resetPortForwardsForCluster(cluster *k8sclient.Cluster) {
	a.pfMu.Lock()
	sessions, cancels := a.detachPortForwardsLocked()
	a.pfCluster = cluster
	a.pfMu.Unlock()
	closePortForwardSessions(sessions, cancels)
}

func (a *App) stopAllPortForwards() {
	a.pfMu.Lock()
	sessions, cancels := a.detachPortForwardsLocked()
	a.pfMu.Unlock()
	closePortForwardSessions(sessions, cancels)
}

func (a *App) portForwardEpoch() uint64 {
	a.pfMu.Lock()
	defer a.pfMu.Unlock()
	return a.pfEpoch
}

// StartPortForward opens a forward from localPort (0 = auto) to remotePort of a
// pod backing the target Pod/Service. Returns the info including the bound local
// port once the tunnel is ready.
func (a *App) StartPortForward(operationID, kind, namespace, name string, localPort, remotePort int, keepRunning bool) (*PortForwardInfo, error) {
	if strings.TrimSpace(operationID) == "" {
		return nil, fmt.Errorf("port-forward operation ID is required")
	}

	a.transitionMu.Lock()
	connectionID, _, activeCluster, _ := a.activeConnectionSnapshot()
	a.pfMu.Lock()
	cluster, epoch := a.pfCluster, a.pfEpoch
	if cluster == nil || cluster != activeCluster {
		a.pfMu.Unlock()
		a.transitionMu.Unlock()
		return nil, fmt.Errorf("not connected to any cluster")
	}
	if _, exists := a.pfPending[operationID]; exists {
		a.pfMu.Unlock()
		a.transitionMu.Unlock()
		return nil, fmt.Errorf("port-forward operation %q already exists", operationID)
	}
	startCtx, cancel := context.WithCancel(a.ctx)
	pending := &pendingPortForward{cancel: cancel, cluster: cluster, epoch: epoch}
	a.pfPending[operationID] = pending
	a.pfMu.Unlock()
	a.transitionMu.Unlock()

	removePending := func() {
		a.pfMu.Lock()
		if a.pfPending[operationID] == pending {
			delete(a.pfPending, operationID)
		}
		a.pfMu.Unlock()
		cancel()
	}

	starter := a.portForwardStarter
	if starter == nil {
		starter = k8sclient.StartPortForward
	}
	session, ready, errCh, err := starter(startCtx, cluster, kind, namespace, name, localPort, remotePort)
	if err != nil {
		removePending()
		return nil, err
	}
	a.pfMu.Lock()
	if a.pfPending[operationID] != pending || epoch != a.pfEpoch || cluster != a.pfCluster {
		a.pfMu.Unlock()
		session.Close()
		cancel()
		return nil, fmt.Errorf("port-forward start was cancelled")
	}
	pending.session = session
	a.pfMu.Unlock()

	select {
	case <-ready:
	case err := <-errCh:
		session.Close()
		removePending()
		return nil, fmt.Errorf("port-forward failed: %w", err)
	case <-startCtx.Done():
		session.Close()
		removePending()
		return nil, fmt.Errorf("port-forward start was cancelled")
	case <-time.After(15 * time.Second):
		session.Close()
		removePending()
		return nil, fmt.Errorf("port-forward did not become ready in time")
	}

	key := fmt.Sprintf("%s:%s/%s/%s:%d→%d", connectionID, kind, namespace, name, session.LocalPort, remotePort)
	info := PortForwardInfo{
		ConnectionID: connectionID,
		Key:          key, Kind: kind, Namespace: namespace, Name: name,
		PodName: session.PodName, LocalPort: session.LocalPort, RemotePort: remotePort,
		KeepRunning: keepRunning,
	}
	entry := &portForwardEntry{session: session, info: info}
	a.pfMu.Lock()
	if a.pfPending[operationID] != pending || epoch != a.pfEpoch || cluster != a.pfCluster {
		a.pfMu.Unlock()
		session.Close()
		cancel()
		return nil, fmt.Errorf("port-forward start was superseded by a cluster change")
	}
	delete(a.pfPending, operationID)
	a.pfSessions[key] = entry
	a.pfMu.Unlock()
	cancel()

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
			wailsruntime.EventsEmit(a.ctx, "portforward-closed", PortForwardClosedEvent{ConnectionID: connectionID, Key: key})
		}
	}(entry)

	return &info, nil
}

// CancelPortForwardStart cancels a StartPortForward call that has not reached
// readiness yet. It is safe to call after completion or more than once.
func (a *App) CancelPortForwardStart(operationID string) {
	a.pfMu.Lock()
	pending := a.pfPending[operationID]
	if pending != nil {
		delete(a.pfPending, operationID)
	}
	a.pfMu.Unlock()
	if pending == nil {
		return
	}
	pending.cancel()
	if pending.session != nil {
		pending.session.Close()
	}
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
func (a *App) StartExec(sessionID, namespace, pod, container, shell string, cols, rows int) (string, error) {
	if strings.TrimSpace(sessionID) == "" {
		return "", fmt.Errorf("exec session ID is required")
	}
	a.transitionMu.Lock()
	cluster, err := a.requireCluster()
	if err != nil {
		a.transitionMu.Unlock()
		return "", err
	}
	// Reserve a generation before opening the stream. Stop/switch can invalidate
	// the pending session without allowing its output into a newer drawer.
	a.execMu.Lock()
	previous := a.execSess
	previousCancel := a.execCancel
	execCtx, execCancel := context.WithCancel(a.ctx)
	a.execSess = nil
	a.execCancel = execCancel
	a.execEpoch++
	epoch := a.execEpoch
	a.execMu.Unlock()
	a.transitionMu.Unlock()
	if previousCancel != nil {
		previousCancel()
	}
	if previous != nil {
		previous.Close()
	}

	starter := a.execStarter
	if starter == nil {
		starter = k8sclient.StartExec
	}
	session, resolvedShell, err := starter(execCtx, cluster, namespace, pod, container, shell, cols, rows,
		func(out string) {
			a.execMu.Lock()
			current := a.execEpoch == epoch
			a.execMu.Unlock()
			if current {
				wailsruntime.EventsEmit(a.ctx, "exec-output", ExecOutputEvent{SessionID: sessionID, Data: out})
			}
		},
		func(err error) {
			a.execMu.Lock()
			if a.execEpoch != epoch {
				a.execMu.Unlock()
				return
			}
			cancel := a.execCancel
			a.execSess = nil
			a.execCancel = nil
			a.execEpoch++
			a.execMu.Unlock()
			if cancel != nil {
				cancel()
			}
			msg := ""
			if err != nil {
				msg = err.Error()
			}
			wailsruntime.EventsEmit(a.ctx, "exec-closed", ExecClosedEvent{SessionID: sessionID, Message: msg})
		})
	if err != nil {
		a.execMu.Lock()
		if a.execEpoch == epoch {
			a.execCancel = nil
			a.execEpoch++
		}
		a.execMu.Unlock()
		execCancel()
		return "", err
	}
	a.execMu.Lock()
	if a.execEpoch != epoch {
		a.execMu.Unlock()
		execCancel()
		session.Close()
		return "", fmt.Errorf("exec session was cancelled while connecting")
	}
	a.execSess = session
	a.execMu.Unlock()
	return resolvedShell, nil
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
	cancel := a.execCancel
	a.execSess = nil
	a.execCancel = nil
	a.execEpoch++
	a.execMu.Unlock()
	if cancel != nil {
		cancel()
	}
	if session != nil {
		session.Close()
	}
}
