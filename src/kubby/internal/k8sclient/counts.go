package k8sclient

import (
	"context"
	"sort"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// The sidebar shows a count (and a red dot on failures) next to every resource
// kind. Fetching those used to mean one bound call per kind from JavaScript —
// ~25 round-trips over the Wails bridge, each serialising every object of that
// kind so the frontend could take `.length`. Switching namespace fired all of
// them again, which is what made the app feel like it stalled.
//
// This file does the whole job in one call:
//   - every kind is fetched concurrently, so the wall clock is the slowest
//     single list rather than the sum of all of them;
//   - kinds that only need a number are counted with the metadata client, which
//     transfers names instead of object bodies (a cluster-wide Secret list is
//     the worst offender);
//   - cluster-scoped kinds can be skipped, because their counts cannot change
//     when you switch namespace.

// NavCount is one sidebar tally.
type NavCount struct {
	View   string `json:"view"` // the frontend view id the badge belongs to
	Count  int    `json:"count"`
	Errors int    `json:"errors"`
}

// countConcurrency bounds how many list requests are in flight at once. High
// enough that the round-trips overlap, low enough not to burst ~25 requests at
// an API server (or a corporate proxy) simultaneously.
const countConcurrency = 8

// metaCountKind is a kind counted with the metadata client: the sidebar shows
// only a number for it, so the object bodies are never needed.
type metaCountKind struct {
	view       string
	gvr        schema.GroupVersionResource
	namespaced bool
}

var metaCountKinds = []metaCountKind{
	{"namespaces", schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}, false},
	{"services", schema.GroupVersionResource{Version: "v1", Resource: "services"}, true},
	{"configmaps", schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}, true},
	{"secrets", schema.GroupVersionResource{Version: "v1", Resource: "secrets"}, true},
	{"serviceaccounts", schema.GroupVersionResource{Version: "v1", Resource: "serviceaccounts"}, true},
	{"resourcequotas", schema.GroupVersionResource{Version: "v1", Resource: "resourcequotas"}, true},
	{"limitranges", schema.GroupVersionResource{Version: "v1", Resource: "limitranges"}, true},
	{"cronjobs", schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "cronjobs"}, true},
	{"ingresses", schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"}, true},
	{"networkpolicies", schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"}, true},
	{"storageclasses", schema.GroupVersionResource{Group: "storage.k8s.io", Version: "v1", Resource: "storageclasses"}, false},
	{"roles", schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "roles"}, true},
	{"rolebindings", schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "rolebindings"}, true},
	{"clusterroles", schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterroles"}, false},
	{"clusterrolebindings", schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterrolebindings"}, false},
	{"crds", schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}, false},
}

// SidebarCounts returns every sidebar tally for the given namespace ("" = all).
//
// includeCluster=false skips the cluster-scoped kinds, whose counts are
// invariant under a namespace change — the frontend keeps the previous values.
// A kind that fails (RBAC, a disabled API) is simply omitted rather than
// failing the whole call, so one forbidden kind cannot blank the sidebar.
func SidebarCounts(ctx context.Context, c *Cluster, namespace string, includeCluster bool) ([]NavCount, error) {
	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		sem = make(chan struct{}, countConcurrency)
		out = []NavCount{}
	)
	add := func(n NavCount) {
		mu.Lock()
		out = append(out, n)
		mu.Unlock()
	}
	// go1 runs fn under the concurrency limit.
	go1 := func(fn func()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			fn()
		}()
	}

	// --- kinds whose rows can be in an error state: counted from the typed
	// lists, because the red dot needs each object's computed status.
	go1(func() {
		if x, err := ListPods(ctx, c.Clientset, namespace); err == nil {
			add(NavCount{"pods", len(x), countErrors(len(x), func(i int) bool { return x[i].IsError })})
		}
	})
	go1(func() {
		if x, err := ListDeployments(ctx, c.Clientset, namespace); err == nil {
			add(NavCount{"deployments", len(x), countErrors(len(x), func(i int) bool { return x[i].IsError })})
		}
	})
	go1(func() {
		if x, err := ListStatefulSets(ctx, c.Clientset, namespace); err == nil {
			add(NavCount{"statefulsets", len(x), countErrors(len(x), func(i int) bool { return x[i].IsError })})
		}
	})
	go1(func() {
		if x, err := ListDaemonSets(ctx, c.Clientset, namespace); err == nil {
			add(NavCount{"daemonsets", len(x), countErrors(len(x), func(i int) bool { return x[i].IsError })})
		}
	})
	go1(func() {
		if x, err := ListJobs(ctx, c.Clientset, namespace); err == nil {
			add(NavCount{"jobs", len(x), countErrors(len(x), func(i int) bool { return x[i].IsError })})
		}
	})
	go1(func() {
		if x, err := ListPVCs(ctx, c.Clientset, namespace); err == nil {
			add(NavCount{"pvcs", len(x), countErrors(len(x), func(i int) bool { return x[i].IsError })})
		}
	})
	go1(func() {
		if x, err := ListHorizontalPodAutoscalers(ctx, c.Clientset, namespace); err == nil {
			add(NavCount{"hpas", len(x), countErrors(len(x), func(i int) bool { return x[i].IsError })})
		}
	})
	go1(func() {
		if x, err := ListPodDisruptionBudgets(ctx, c.Clientset, namespace); err == nil {
			add(NavCount{"pdbs", len(x), countErrors(len(x), func(i int) bool { return x[i].IsError })})
		}
	})
	// Helm reads and decodes release Secrets through the Helm SDK — by far the
	// slowest tally, which is exactly why it must not block the others.
	go1(func() {
		if x, err := ListHelmReleases(ctx, c.Meta, namespace); err == nil {
			add(NavCount{"helm", len(x), countErrors(len(x), func(i int) bool { return x[i].IsError })})
		}
	})

	if includeCluster {
		go1(func() {
			if x, err := ListNodes(ctx, c.Clientset); err == nil {
				add(NavCount{"nodes", len(x), countErrors(len(x), func(i int) bool { return !x[i].Ready })})
			}
		})
		go1(func() {
			if x, err := ListPersistentVolumes(ctx, c.Clientset); err == nil {
				add(NavCount{"pvs", len(x), countErrors(len(x), func(i int) bool { return x[i].IsError })})
			}
		})
	}

	// --- count-only kinds: metadata lists, no object bodies transferred.
	for _, k := range metaCountKinds {
		if !k.namespaced && !includeCluster {
			continue
		}
		kind := k
		go1(func() {
			ns := ""
			if kind.namespaced {
				ns = namespace
			}
			n, err := c.metaCount(ctx, kind.gvr, ns)
			if err != nil {
				return
			}
			add(NavCount{kind.view, n, 0})
		})
	}

	wg.Wait()
	sort.Slice(out, func(i, j int) bool { return out[i].View < out[j].View })
	return out, nil
}

// metaCount lists a resource's metadata only and returns how many there are.
func (c *Cluster) metaCount(ctx context.Context, gvr schema.GroupVersionResource, namespace string) (int, error) {
	list, err := c.Meta.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return 0, err
	}
	return len(list.Items), nil
}

// countErrors counts indices for which pred reports an error state. Keeps the
// per-kind closures above to one line each.
func countErrors(n int, pred func(int) bool) int {
	e := 0
	for i := 0; i < n; i++ {
		if pred(i) {
			e++
		}
	}
	return e
}
