package k8sclient

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/metadata"
)

var crdGVR = schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}

type CRDInfo struct {
	Name     string `json:"name"`
	Group    string `json:"group"`
	Kind     string `json:"kind"`
	Scope    string `json:"scope"`
	Versions string `json:"versions"`
	Age      string `json:"age"`
}

// ListCRDs lists CustomResourceDefinitions via the dynamic client (no typed
// apiextensions client needed).
func ListCRDs(ctx context.Context, c *Cluster) ([]CRDInfo, error) {
	list, err := c.Dynamic.Resource(crdGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]CRDInfo, 0, len(list.Items))
	for i := range list.Items {
		item := list.Items[i]
		group, _, _ := unstructured.NestedString(item.Object, "spec", "group")
		kind, _, _ := unstructured.NestedString(item.Object, "spec", "names", "kind")
		scope, _, _ := unstructured.NestedString(item.Object, "spec", "scope")
		versions := []string{}
		if vers, found, _ := unstructured.NestedSlice(item.Object, "spec", "versions"); found {
			for _, v := range vers {
				if m, ok := v.(map[string]interface{}); ok {
					if name, ok := m["name"].(string); ok {
						versions = append(versions, name)
					}
				}
			}
		}
		out = append(out, CRDInfo{
			Name: item.GetName(), Group: group, Kind: kind, Scope: scope,
			Versions: strings.Join(versions, ", "), Age: age(metav1.Time{Time: item.GetCreationTimestamp().Time}),
		})
	}
	return out, nil
}

type HelmReleaseInfo struct {
	Namespace  string `json:"namespace"`
	Name       string `json:"name"`
	Revision   string `json:"revision"`
	Status     string `json:"status"`
	Updated    string `json:"updated"`
	SecretName string `json:"secretName"` // the backing Secret (for drill-in)
	IsError    bool   `json:"isError"`
	IsPending  bool   `json:"isPending"`
}

// ListHelmReleases returns one current row per Helm 3 release. Helm stores every
// revision as a separate Secret, so rendering the raw list would duplicate a
// release after each upgrade and inflate the sidebar count. Metadata is enough:
// the labels identify the release, revision and status without transferring the
// compressed chart payload kept in Secret.data.
func ListHelmReleases(ctx context.Context, client metadata.Interface, namespace string) ([]HelmReleaseInfo, error) {
	list, err := client.Resource(schema.GroupVersionResource{Version: "v1", Resource: "secrets"}).Namespace(namespace).
		List(ctx, metav1.ListOptions{LabelSelector: "owner=helm"})
	if err != nil {
		return nil, err
	}
	latest := make(map[string]metav1.PartialObjectMetadata, len(list.Items))
	for _, s := range list.Items {
		if s.DeletionTimestamp != nil || s.Labels["name"] == "" {
			continue
		}
		key := s.Namespace + "\x00" + s.Labels["name"]
		current, ok := latest[key]
		if !ok || helmRevisionAfter(s, current) {
			latest[key] = s
		}
	}
	out := make([]HelmReleaseInfo, 0, len(latest))
	for _, s := range latest {
		status := strings.ToLower(s.Labels["status"])
		pending := strings.HasPrefix(status, "pending-") || status == "uninstalling"
		out = append(out, HelmReleaseInfo{
			Namespace: s.Namespace, Name: s.Labels["name"], Revision: s.Labels["version"],
			Status: status, Updated: age(s.CreationTimestamp), SecretName: s.Name,
			IsError: status == "failed", IsPending: pending,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace != out[j].Namespace {
			return out[i].Namespace < out[j].Namespace
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func helmRevisionAfter(candidate, current metav1.PartialObjectMetadata) bool {
	candidateRevision, candidateErr := strconv.Atoi(candidate.Labels["version"])
	currentRevision, currentErr := strconv.Atoi(current.Labels["version"])
	if candidateErr == nil && currentErr == nil && candidateRevision != currentRevision {
		return candidateRevision > currentRevision
	}
	return candidate.CreationTimestamp.After(current.CreationTimestamp.Time)
}

type ResourceQuotaInfo struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Summary   string `json:"summary"` // e.g. "pods 3/10, cpu 500m/2"
	Age       string `json:"age"`
}

func ListResourceQuotas(ctx context.Context, client kubernetes.Interface, namespace string) ([]ResourceQuotaInfo, error) {
	list, err := client.CoreV1().ResourceQuotas(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]ResourceQuotaInfo, 0, len(list.Items))
	for _, q := range list.Items {
		// Sort by resource name: ranging a map directly would reshuffle the
		// summary text on every refresh.
		names := make([]string, 0, len(q.Status.Hard))
		for res := range q.Status.Hard {
			names = append(names, string(res))
		}
		sort.Strings(names)
		parts := make([]string, 0, len(names))
		for _, res := range names {
			hard := q.Status.Hard[corev1.ResourceName(res)]
			used := q.Status.Used[corev1.ResourceName(res)]
			parts = append(parts, fmt.Sprintf("%s %s/%s", res, used.String(), hard.String()))
		}
		out = append(out, ResourceQuotaInfo{
			Namespace: q.Namespace, Name: q.Name, Summary: strings.Join(parts, ", "),
			Age: age(q.CreationTimestamp),
		})
	}
	return out, nil
}

type LimitRangeInfo struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Limits    int    `json:"limits"`
	Types     string `json:"types"` // e.g. "Container, Pod" — what the limits apply to
	Age       string `json:"age"`
}

func ListLimitRanges(ctx context.Context, client kubernetes.Interface, namespace string) ([]LimitRangeInfo, error) {
	list, err := client.CoreV1().LimitRanges(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]LimitRangeInfo, 0, len(list.Items))
	for _, lr := range list.Items {
		seen := map[string]bool{}
		types := []string{}
		for _, l := range lr.Spec.Limits {
			t := string(l.Type)
			if t == "" || seen[t] {
				continue
			}
			seen[t] = true
			types = append(types, t)
		}
		out = append(out, LimitRangeInfo{
			Namespace: lr.Namespace, Name: lr.Name, Limits: len(lr.Spec.Limits),
			Types: strings.Join(types, ", "), Age: age(lr.CreationTimestamp),
		})
	}
	return out, nil
}
