package k8sclient

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// PodsOnNode lists the pods scheduled on a node (the Node → Pods view in Lens).
func PodsOnNode(ctx context.Context, c *Cluster, nodeName string) ([]PodInfo, error) {
	list, err := c.Clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		FieldSelector: "spec.nodeName=" + nodeName,
	})
	if err != nil {
		return nil, err
	}
	out := make([]PodInfo, 0, len(list.Items))
	for _, pod := range list.Items {
		status, restarts, ready := podStatus(pod)
		out = append(out, PodInfo{
			Namespace: pod.Namespace, Name: pod.Name, Status: status, Ready: ready,
			Restarts: restarts, IsError: isErroredPodStatus(status),
			PodIP: pod.Status.PodIP, Node: pod.Spec.NodeName, Age: age(pod.CreationTimestamp),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Namespace+out[i].Name < out[j].Namespace+out[j].Name })
	return out, nil
}

// NsKindCount is one resource-kind tally inside a namespace.
type NsKindCount struct {
	Kind   string `json:"kind"`
	View   string `json:"view"` // frontend view id to navigate to
	Count  int    `json:"count"`
	Errors int    `json:"errors"`
}

// NamespaceSummary counts the key resource kinds inside one namespace, so
// clicking a namespace gives an at-a-glance picture of what lives in it.
//
// The tallies are gathered concurrently and the count-only ones come from
// metadata lists, for the same reason SidebarCounts does (see counts.go): eleven
// sequential full-object lists made opening a namespace feel like a stall.
// Results are written into fixed slots so the order stays deterministic.
func NamespaceSummary(ctx context.Context, c *Cluster, ns string) ([]NsKindCount, error) {
	// Order here is the order shown in the drawer.
	slots := []NsKindCount{
		{Kind: "Pods", View: "pods"},
		{Kind: "Deployments", View: "deployments"},
		{Kind: "StatefulSets", View: "statefulsets"},
		{Kind: "DaemonSets", View: "daemonsets"},
		{Kind: "Services", View: "services"},
		{Kind: "Ingresses", View: "ingresses"},
		{Kind: "ConfigMaps", View: "configmaps"},
		{Kind: "Secrets", View: "secrets"},
		{Kind: "PVCs", View: "pvcs"},
		{Kind: "Jobs", View: "jobs"},
		{Kind: "CronJobs", View: "cronjobs"},
	}
	// A kind that fails contributes 0 rather than failing the whole summary; only
	// Pods is treated as required, since a namespace we cannot list pods in is a
	// genuine access problem worth surfacing.
	var podErr error

	// Kinds whose rows can be failing need the typed list to compute status.
	typed := map[int]func() (int, int, error){
		0: func() (int, int, error) {
			x, err := ListPods(ctx, c.Clientset, ns)
			return len(x), countErrors(len(x), func(i int) bool { return x[i].IsError }), err
		},
		1: func() (int, int, error) {
			x, err := ListDeployments(ctx, c.Clientset, ns)
			return len(x), countErrors(len(x), func(i int) bool { return x[i].IsError }), err
		},
		2: func() (int, int, error) {
			x, err := ListStatefulSets(ctx, c.Clientset, ns)
			return len(x), countErrors(len(x), func(i int) bool { return x[i].IsError }), err
		},
		3: func() (int, int, error) {
			x, err := ListDaemonSets(ctx, c.Clientset, ns)
			return len(x), countErrors(len(x), func(i int) bool { return x[i].IsError }), err
		},
		8: func() (int, int, error) {
			x, err := ListPVCs(ctx, c.Clientset, ns)
			return len(x), countErrors(len(x), func(i int) bool { return x[i].IsError }), err
		},
		9: func() (int, int, error) {
			x, err := ListJobs(ctx, c.Clientset, ns)
			return len(x), countErrors(len(x), func(i int) bool { return x[i].IsError }), err
		},
	}
	// Count-only kinds: metadata lists, no object bodies.
	metaOnly := map[int]schema.GroupVersionResource{
		4:  {Version: "v1", Resource: "services"},
		5:  {Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"},
		6:  {Version: "v1", Resource: "configmaps"},
		7:  {Version: "v1", Resource: "secrets"},
		10: {Group: "batch", Version: "v1", Resource: "cronjobs"},
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, countConcurrency)
	run := func(fn func()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			fn()
		}()
	}
	for idx, fn := range typed {
		i, get := idx, fn
		run(func() {
			count, errors, err := get()
			if err != nil {
				if i == 0 {
					podErr = err
				}
				return
			}
			slots[i].Count, slots[i].Errors = count, errors
		})
	}
	for idx, gvr := range metaOnly {
		i, g := idx, gvr
		run(func() {
			if n, err := c.metaCount(ctx, g, ns); err == nil {
				slots[i].Count = n
			}
		})
	}
	wg.Wait()

	if podErr != nil {
		return nil, podErr
	}
	return slots, nil
}

// SearchHit is one resource matched by a global name search.
type SearchHit struct {
	Kind      string `json:"kind"`
	View      string `json:"view"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	IsError   bool   `json:"isError"`
}

// searchKind is one kind the global search covers: which resource to list, and
// which frontend view a hit navigates to.
type searchKind struct {
	kind string
	view string
	gvr  schema.GroupVersionResource
}

// searchKinds is every built-in kind that has a section in the UI — the search
// covers the sidebar, not a subset of it. Custom resources are appended at call
// time from the cluster's CRDs.
var searchKinds = []searchKind{
	{"Pod", "pods", schema.GroupVersionResource{Version: "v1", Resource: "pods"}},
	{"Deployment", "deployments", schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}},
	{"StatefulSet", "statefulsets", schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}},
	{"DaemonSet", "daemonsets", schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}},
	{"Job", "jobs", schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "jobs"}},
	{"CronJob", "cronjobs", schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "cronjobs"}},
	{"Service", "services", schema.GroupVersionResource{Version: "v1", Resource: "services"}},
	{"Ingress", "ingresses", schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"}},
	{"ConfigMap", "configmaps", schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}},
	{"Secret", "secrets", schema.GroupVersionResource{Version: "v1", Resource: "secrets"}},
	{"ResourceQuota", "resourcequotas", schema.GroupVersionResource{Version: "v1", Resource: "resourcequotas"}},
	{"LimitRange", "limitranges", schema.GroupVersionResource{Version: "v1", Resource: "limitranges"}},
	{"PersistentVolumeClaim", "pvcs", schema.GroupVersionResource{Version: "v1", Resource: "persistentvolumeclaims"}},
	{"PersistentVolume", "pvs", schema.GroupVersionResource{Version: "v1", Resource: "persistentvolumes"}},
	{"StorageClass", "storageclasses", schema.GroupVersionResource{Group: "storage.k8s.io", Version: "v1", Resource: "storageclasses"}},
	{"ServiceAccount", "serviceaccounts", schema.GroupVersionResource{Version: "v1", Resource: "serviceaccounts"}},
	{"Role", "roles", schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "roles"}},
	{"RoleBinding", "rolebindings", schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "rolebindings"}},
	{"ClusterRole", "clusterroles", schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterroles"}},
	{"ClusterRoleBinding", "clusterrolebindings", schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterrolebindings"}},
	{"CustomResourceDefinition", "crds", schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}},
	{"Node", "nodes", schema.GroupVersionResource{Version: "v1", Resource: "nodes"}},
	{"Namespace", "namespaces", schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}},
}

const searchIndexTTL = 15 * time.Second

type searchIndexItem struct {
	namespace string
	name      string
}

// searchIndexGroup retains the deliberate built-in/custom kind ordering while
// storing only the two metadata fields a name search needs. It is immutable
// after publication, so readers can safely use it after the cache mutex unlocks.
type searchIndexGroup struct {
	kind  string
	view  string
	items []searchIndexItem
}

// SearchResources finds resources whose name contains the query, across all
// namespaces and **every kind the UI has a section for**, custom resources
// included — powering the Ctrl+K "find any resource".
//
// Three things make this affordable to run on a debounced keystroke. Every kind
// is listed **concurrently** (bounded, so it overlaps round-trips without
// bursting the API server), every list is **metadata-only**, and the resulting
// name index is reused briefly across settled queries. A search used to pull
// every Secret's data and every Pod's full status cluster-wide, one kind after
// another; later it still repeated the metadata fan-out for every query.
//
// The cost of metadata-only is that a hit carries no computed status, so the
// error flag is resolved afterwards — for the handful of Pods that matched only.
func SearchResources(ctx context.Context, c *Cluster, query string) ([]SearchHit, error) {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil, nil
	}
	const perKind = 8
	const maxTotal = 60
	index, err := c.cachedSearchIndex(ctx)
	if err != nil {
		return nil, err
	}

	hits := []SearchHit{}
	for _, group := range index {
		matched := 0
		for _, item := range group.items {
			if !strings.Contains(strings.ToLower(item.name), q) {
				continue
			}
			hits = append(hits, SearchHit{group.kind, group.view, item.namespace, item.name, false})
			matched++
			if matched >= perKind || len(hits) >= maxTotal {
				break
			}
		}
		if len(hits) >= maxTotal {
			break
		}
	}
	markFailingPods(ctx, c, hits)
	return hits, nil
}

// cachedSearchIndex serializes cold refreshes per connection. A second query
// arriving while the first is listing waits for and reuses that same snapshot;
// it never starts another countConcurrency-sized fan-out.
func (c *Cluster) cachedSearchIndex(ctx context.Context) ([]searchIndexGroup, error) {
	c.searchMu.Lock()
	defer c.searchMu.Unlock()
	if time.Now().Before(c.searchIndexExpires) {
		return c.searchIndex, nil
	}
	index, err := buildSearchIndex(ctx, c)
	if err != nil {
		return nil, err
	}
	c.searchIndex = index
	c.searchIndexExpires = time.Now().Add(searchIndexTTL)
	return index, nil
}

func buildSearchIndex(ctx context.Context, c *Cluster) ([]searchIndexGroup, error) {
	if c.Meta == nil {
		return nil, fmt.Errorf("metadata client is unavailable")
	}
	kinds := append([]searchKind(nil), searchKinds...)
	// Custom resources are why "virtualservice" used to find nothing. Failing to
	// enumerate them must not fail the index over built-in kinds.
	if c.Dynamic != nil {
		if custom, err := CustomKinds(ctx, c); err == nil {
			for _, ck := range custom.Kinds {
				if ak, err := c.ResolveKind(ck.RefKind); err == nil {
					kinds = append(kinds, searchKind{ck.Kind, "custom:" + ck.RefKind, ak.GVR})
				}
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	index := make([]searchIndexGroup, len(kinds))
	var wg sync.WaitGroup
	sem := make(chan struct{}, countConcurrency)
	for i := range kinds {
		idx, sk := i, kinds[i]
		index[idx] = searchIndexGroup{kind: sk.kind, view: sk.view}
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			list, err := c.Meta.Resource(sk.gvr).Namespace("").List(ctx, metav1.ListOptions{})
			if err != nil {
				return // forbidden, or the kind is not served — just contributes nothing
			}
			items := make([]searchIndexItem, 0, len(list.Items))
			for _, item := range list.Items {
				items = append(items, searchIndexItem{namespace: item.GetNamespace(), name: item.GetName()})
			}
			index[idx].items = items
		}()
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return index, nil
}

// markFailingPods sets IsError on the Pod hits, which metadata-only lists cannot
// tell us. Only matched pods are fetched — at most `perKind` of them — so the
// red highlight survives without listing every Pod in the cluster.
func markFailingPods(ctx context.Context, c *Cluster, hits []SearchHit) {
	var wg sync.WaitGroup
	for i := range hits {
		if hits[i].Kind != "Pod" {
			continue
		}
		h := &hits[i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			pod, err := c.Clientset.CoreV1().Pods(h.Namespace).Get(ctx, h.Name, metav1.GetOptions{})
			if err != nil {
				return
			}
			status, _, _ := podStatus(*pod)
			h.IsError = isErroredPodStatus(status)
		}()
	}
	wg.Wait()
}

// The Ingress → Service → Pod topology behind the Traffic view lives in
// netflow.go.
