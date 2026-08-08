package k8sclient

import (
	"fmt"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
)

// Kubby ships a static kind→resource table (gvrByKind) for the ~25 kinds the
// sidebar knows about. That table is fast and predictable, but it can only ever
// describe built-in kinds — anything installed by a CRD (Istio, cert-manager,
// Argo, Gateway API…) was simply "unsupported kind".
//
// This file asks the cluster instead. Discovery lists every resource the API
// server actually serves, including CRDs, together with its plural name and
// whether it is namespaced — exactly what the dynamic client needs. The result
// is cached because discovery is chatty (one request per API group version).

// APIKind is one resource type the connected cluster serves.
type APIKind struct {
	GVR        schema.GroupVersionResource
	Kind       string
	Namespaced bool
}

// GroupVersionKind is the GVK this resource is addressed by.
func (a APIKind) GroupVersionKind() schema.GroupVersionKind {
	return a.GVR.GroupVersion().WithKind(a.Kind)
}

// apiIndex is a snapshot of the cluster's served resources.
type apiIndex struct {
	// byGVK holds every served version, so YAML that pins an older API version
	// (networking.istio.io/v1beta1 while the cluster prefers v1) still resolves
	// to the version the author asked for.
	byGVK map[schema.GroupVersionKind]APIKind
	// byKind holds only each group's preferred version, keyed by bare kind. A
	// kind can appear in several groups (Gateway is both an Istio and a Gateway
	// API kind), hence the slice.
	byKind map[string][]APIKind
}

// apiKinds returns the cached index, rebuilding it when refresh is set (a CRD
// installed after we connected would otherwise stay invisible).
func (c *Cluster) apiKinds(refresh bool) (*apiIndex, error) {
	c.idxMu.Lock()
	defer c.idxMu.Unlock()
	if c.idx != nil && !refresh {
		return c.idx, nil
	}
	if c.Discovery == nil {
		return nil, fmt.Errorf("this connection has no discovery client")
	}

	groups, lists, err := c.Discovery.ServerGroupsAndResources()
	// A single broken aggregated APIService (a stale metrics.k8s.io is the usual
	// culprit) makes discovery return an error alongside everything that *did*
	// work. Dropping the whole index over that would break resolution for every
	// healthy group, so partial results are kept.
	if err != nil && !discovery.IsGroupDiscoveryFailedError(err) {
		return nil, err
	}
	if len(lists) == 0 {
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("the API server returned no resource types")
	}

	preferred := make(map[string]string, len(groups)) // group → preferred version
	for _, g := range groups {
		preferred[g.Name] = g.PreferredVersion.Version
	}

	idx := &apiIndex{
		byGVK:  map[schema.GroupVersionKind]APIKind{},
		byKind: map[string][]APIKind{},
	}
	for _, l := range lists {
		gv, parseErr := schema.ParseGroupVersion(l.GroupVersion)
		if parseErr != nil {
			continue
		}
		for _, r := range l.APIResources {
			if strings.Contains(r.Name, "/") {
				continue // a subresource (pods/log, deployments/scale) — not addressable on its own
			}
			ak := APIKind{GVR: gv.WithResource(r.Name), Kind: r.Kind, Namespaced: r.Namespaced}
			idx.byGVK[gv.WithKind(r.Kind)] = ak
			if preferred[gv.Group] != gv.Version {
				continue
			}
			if !hasGroup(idx.byKind[r.Kind], gv.Group) {
				idx.byKind[r.Kind] = append(idx.byKind[r.Kind], ak)
			}
		}
	}
	c.idx = idx
	return idx, nil
}

func hasGroup(known []APIKind, group string) bool {
	for _, k := range known {
		if k.GVR.Group == group {
			return true
		}
	}
	return false
}

// ResolveKind maps a kind name to the resource the cluster serves.
//
// The name may carry its API group to disambiguate, kubectl-style:
// "Gateway.networking.istio.io". That form lets the whole UI keep passing a
// single kind string around while still addressing CRDs unambiguously — a
// cluster running both Istio and Gateway API serves two different "Gateway".
func (c *Cluster) ResolveKind(kind string) (APIKind, error) {
	name, group := splitKindGroup(kind)
	if name == "" {
		return APIKind{}, fmt.Errorf("no kind given")
	}
	// Built-in kinds resolve from the static table: no discovery round-trip, and
	// it settles the "Service"/"Node"-style names that CRDs also like to use.
	if group == "" {
		if gvr, ok := gvrByKind[name]; ok {
			return APIKind{GVR: gvr, Kind: name, Namespaced: !clusterScopedKinds[name]}, nil
		}
	}

	var candidates []APIKind
	for _, refresh := range []bool{false, true} {
		idx, err := c.apiKinds(refresh)
		if err != nil {
			return APIKind{}, err
		}
		candidates = nil
		for _, ak := range idx.byKind[name] {
			if group == "" || ak.GVR.Group == group {
				candidates = append(candidates, ak)
			}
		}
		if len(candidates) > 0 {
			break
		}
	}

	switch len(candidates) {
	case 0:
		if group != "" {
			return APIKind{}, fmt.Errorf("the cluster serves no %q resource in API group %q", name, group)
		}
		return APIKind{}, fmt.Errorf("the cluster serves no %q resource — if it comes from an operator, check its CRD is installed", name)
	case 1:
		return candidates[0], nil
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].GVR.Group < candidates[j].GVR.Group })
	groups := make([]string, 0, len(candidates))
	for _, ak := range candidates {
		groups = append(groups, ak.GVR.Group)
	}
	return APIKind{}, fmt.Errorf("kind %q is ambiguous — it exists in %s; qualify it as %s.%s",
		name, strings.Join(groups, " and "), name, groups[0])
}

// ResolveGVK maps an apiVersion + kind straight from a YAML document to the
// resource it addresses. Unlike ResolveKind this is exact: the version written
// in the manifest is the version the object is sent to.
func (c *Cluster) ResolveGVK(apiVersion, kind string) (APIKind, error) {
	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return APIKind{}, fmt.Errorf("invalid apiVersion %q: %w", apiVersion, err)
	}
	gvk := gv.WithKind(kind)
	for _, refresh := range []bool{false, true} {
		idx, err := c.apiKinds(refresh)
		if err != nil {
			return APIKind{}, err
		}
		if ak, ok := idx.byGVK[gvk]; ok {
			return ak, nil
		}
		// Not in the cached snapshot: a CRD may have been installed since we
		// connected, so the second pass re-reads discovery before giving up.
	}

	// Distinguish "wrong version" from "not installed at all" — the first is a
	// one-word fix in the manifest, the second means installing an operator.
	if idx, err := c.apiKinds(false); err == nil {
		if served := idx.byKind[kind]; len(served) > 0 {
			versions := []string{}
			for g := range idx.byGVK {
				if g.Kind == kind && g.Group == gv.Group {
					versions = append(versions, g.GroupVersion().String())
				}
			}
			if len(versions) > 0 {
				sort.Strings(versions)
				return APIKind{}, fmt.Errorf("the cluster does not serve %s — this kind is served as %s",
					gvk.GroupVersion().String()+" "+kind, strings.Join(versions, ", "))
			}
		}
	}
	return APIKind{}, fmt.Errorf("the cluster does not serve %q with apiVersion %q — is the CRD that defines it installed?", kind, apiVersion)
}

// splitKindGroup splits "Gateway.networking.istio.io" into ("Gateway",
// "networking.istio.io"). A bare "Gateway" yields an empty group.
func splitKindGroup(kind string) (string, string) {
	kind = strings.TrimSpace(kind)
	if i := strings.Index(kind, "."); i > 0 {
		return kind[:i], kind[i+1:]
	}
	return kind, ""
}

// bareKind drops any API group suffix. Used where Kubernetes itself wants the
// plain kind — an Event's involvedObject.kind, for instance.
func bareKind(kind string) string {
	name, _ := splitKindGroup(kind)
	return name
}
