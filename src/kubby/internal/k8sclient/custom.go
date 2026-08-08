package k8sclient

import (
	"context"
	"fmt"
	"sort"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// A cluster's interesting kinds are not only the built-in ones. Istio, cert-manager,
// Argo, Gateway API and every operator add their own, and a tool that hard-codes a
// sidebar can never show them — which is why searching for "virtualservice" found
// nothing even with Istio installed.
//
// This file enumerates the kinds a cluster's CRDs define and lists their objects
// generically, so each one becomes a real section: navigable from the sidebar,
// findable in the command palette, and clickable through to the usual
// Details/YAML/Events/Delete drawer.
//
// Authority is the CRD list itself rather than "any API group that doesn't look
// built-in": Gateway API lives under gateway.networking.k8s.io and would be
// misfiled by a name heuristic, while metrics.k8s.io is not a CRD at all.

// maxCustomKinds bounds how many custom kinds become sections. A cluster with a
// large operator estate can define hundreds; past this point a sidebar stops
// being navigation. CustomKinds reports the overflow rather than hiding it.
const maxCustomKinds = 60

// CustomKind is one CRD-defined kind the UI can show as its own section.
type CustomKind struct {
	// RefKind is the "Kind.group" reference the rest of the app passes around
	// (see apiindex.go) — unambiguous even when two groups define one kind.
	RefKind    string `json:"refKind"`
	Kind       string `json:"kind"`
	Group      string `json:"group"`
	Title      string `json:"title"` // plural, title-cased — the sidebar label
	Namespaced bool   `json:"namespaced"`
	Count      int    `json:"count"` // -1 when not counted
}

// CustomKindList is the payload for the dynamic sidebar section.
type CustomKindList struct {
	Kinds    []CustomKind `json:"kinds"`
	Total    int          `json:"total"`    // how many the cluster defines
	Overflow int          `json:"overflow"` // how many were left out by maxCustomKinds
}

// CustomKinds lists the kinds defined by the cluster's CRDs, resolved through
// discovery so each one carries the version the API server actually serves.
//
// Counts are deliberately *not* fetched: that would be one list per kind on
// every connect, which is the cost the sidebar was just relieved of. A section's
// objects load when you open it.
func CustomKinds(ctx context.Context, c *Cluster) (*CustomKindList, error) {
	crds, err := ListCRDs(ctx, c)
	if err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	kinds := []CustomKind{}
	for _, crd := range crds {
		if crd.Kind == "" || crd.Group == "" {
			continue
		}
		ref := crd.Kind + "." + crd.Group
		if seen[ref] {
			continue
		}
		seen[ref] = true
		// Resolving confirms the kind is really served (a CRD can exist with no
		// served version) and pins the group's preferred version. It also yields
		// the plural — the resource name — which makes the sidebar label.
		ak, err := c.ResolveKind(ref)
		if err != nil {
			continue
		}
		kinds = append(kinds, CustomKind{
			RefKind: ref, Kind: crd.Kind, Group: crd.Group,
			Title:      titleFor(crd.Kind, ak.GVR.Resource),
			Namespaced: ak.Namespaced,
			Count:      -1,
		})
	}

	// Kind first so the sidebar reads alphabetically; the group only breaks ties.
	sort.Slice(kinds, func(i, j int) bool {
		if kinds[i].Kind != kinds[j].Kind {
			return kinds[i].Kind < kinds[j].Kind
		}
		return kinds[i].Group < kinds[j].Group
	})

	out := &CustomKindList{Total: len(kinds)}
	if len(kinds) > maxCustomKinds {
		out.Overflow = len(kinds) - maxCustomKinds
		kinds = kinds[:maxCustomKinds]
	}
	out.Kinds = kinds
	return out, nil
}

// titleFor turns a kind + plural into a sidebar label: "VirtualService" +
// "virtualservices" → "VirtualServices". Falls back to the kind when the CRD's
// plural is missing or is not simply the lower-cased kind with a suffix.
func titleFor(kind, plural string) string {
	if plural == "" {
		return kind
	}
	lower := strings.ToLower(kind)
	if strings.HasPrefix(plural, lower) {
		return kind + plural[len(lower):]
	}
	return kind
}

// CustomObject is one row in a custom-kind list. Only fields every resource is
// guaranteed to have — there is no schema to read column meanings from.
type CustomObject struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Age       string `json:"age"`
	Status    string `json:"status"`  // a Ready/condition summary when the object has one
	IsError   bool   `json:"isError"` // Ready=False
}

// ListCustom lists the objects of a custom kind ("Kind.group"), in the given
// namespace ("" = all).
func ListCustom(ctx context.Context, c *Cluster, refKind, namespace string) ([]CustomObject, error) {
	ak, err := c.ResolveKind(refKind)
	if err != nil {
		return nil, err
	}
	ri := c.Dynamic.Resource(ak.GVR)
	list, err := func() (*unstructured.UnstructuredList, error) {
		if !ak.Namespaced {
			return ri.List(ctx, metav1.ListOptions{})
		}
		return ri.Namespace(namespace).List(ctx, metav1.ListOptions{})
	}()
	if err != nil {
		return nil, err
	}

	out := make([]CustomObject, 0, len(list.Items))
	for i := range list.Items {
		obj := &list.Items[i]
		status, isErr := readyCondition(obj.Object)
		out = append(out, CustomObject{
			Namespace: obj.GetNamespace(), Name: obj.GetName(),
			Age:    age(metav1.Time{Time: obj.GetCreationTimestamp().Time}),
			Status: status, IsError: isErr,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Namespace+"/"+out[i].Name < out[j].Namespace+"/"+out[j].Name
	})
	return out, nil
}

// readyCondition pulls a human status out of the near-universal
// status.conditions convention. Many custom resources (Istio's, for one) have no
// conditions at all, in which case there is honestly nothing to report.
func readyCondition(object map[string]interface{}) (string, bool) {
	conds, found, err := unstructured.NestedSlice(object, "status", "conditions")
	if err != nil || !found {
		return "", false
	}
	for _, raw := range conds {
		cond, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		ctype, _, _ := unstructured.NestedString(cond, "type")
		if !strings.EqualFold(ctype, "Ready") && !strings.EqualFold(ctype, "Available") {
			continue
		}
		cstatus, _, _ := unstructured.NestedString(cond, "status")
		switch cstatus {
		case "True":
			return ctype, false
		case "False":
			reason, _, _ := unstructured.NestedString(cond, "reason")
			if reason != "" {
				return fmt.Sprintf("Not %s: %s", ctype, reason), true
			}
			return "Not " + ctype, true
		}
	}
	return "", false
}
