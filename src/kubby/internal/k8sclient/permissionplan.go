package k8sclient

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/release"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

// PermissionRequirement is one exact Kubernetes authorization question behind
// a multi-resource workflow. Probe failures remain allowed but are marked
// unchecked; only an explicit API-server denial contributes to Denied.
type PermissionRequirement struct {
	Kind        string `json:"kind"`
	Namespace   string `json:"namespace"`
	Verb        string `json:"verb"`
	Subresource string `json:"subresource"`
	Allowed     bool   `json:"allowed"`
	Checked     bool   `json:"checked"`
	Reason      string `json:"reason"`
}

type PermissionPlan struct {
	Operation    string                  `json:"operation"`
	Requirements []PermissionRequirement `json:"requirements"`
	Denied       int                     `json:"denied"`
	Unknown      int                     `json:"unknown"`
	Permitted    bool                    `json:"permitted"`
	Warnings     []string                `json:"warnings"`
}

type permissionSpec struct {
	ak          APIKind
	namespace   string
	verb        string
	subresource string
}

func permissionSpecKey(spec permissionSpec) string {
	return strings.Join([]string{spec.ak.GVR.Group, spec.ak.GVR.Resource, spec.namespace, spec.subresource, spec.verb}, "\x00")
}

func runPermissionPlan(ctx context.Context, c *Cluster, operation string, specs []permissionSpec, warnings []string) *PermissionPlan {
	deduped := make([]permissionSpec, 0, len(specs))
	seen := map[string]bool{}
	for _, spec := range specs {
		if !spec.ak.Namespaced {
			spec.namespace = ""
		}
		key := permissionSpecKey(spec)
		if seen[key] {
			continue
		}
		seen[key] = true
		deduped = append(deduped, spec)
	}

	requirements := make([]PermissionRequirement, len(deduped))
	var wg sync.WaitGroup
	sem := make(chan struct{}, accessConcurrency)
	for i, spec := range deduped {
		wg.Add(1)
		go func(i int, spec permissionSpec) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			allowed, err := c.canI(ctx, spec.ak.GVR.Group, spec.ak.GVR.Resource, spec.subresource, spec.verb, spec.namespace)
			req := PermissionRequirement{
				Kind: spec.ak.Kind, Namespace: spec.namespace, Verb: spec.verb,
				Subresource: spec.subresource, Allowed: allowed, Checked: err == nil,
			}
			if err != nil {
				req.Allowed = true
				req.Reason = "The API server could not answer this permission probe; the operation remains available."
			} else if !allowed {
				target := spec.ak.Kind
				if spec.subresource != "" {
					target += "/" + spec.subresource
				}
				where := ""
				if spec.namespace != "" {
					where = " in namespace " + spec.namespace
				}
				req.Reason = fmt.Sprintf("Your token cannot %s %s%s.", spec.verb, target, where)
			}
			requirements[i] = req
		}(i, spec)
	}
	wg.Wait()

	sort.Slice(requirements, func(i, j int) bool {
		a, b := requirements[i], requirements[j]
		return a.Namespace+"\x00"+a.Kind+"\x00"+a.Subresource+"\x00"+a.Verb < b.Namespace+"\x00"+b.Kind+"\x00"+b.Subresource+"\x00"+b.Verb
	})
	plan := &PermissionPlan{Operation: operation, Requirements: requirements, Warnings: warnings}
	for _, req := range requirements {
		if !req.Checked {
			plan.Unknown++
		} else if !req.Allowed {
			plan.Denied++
		}
	}
	plan.Permitted = plan.Denied == 0
	return plan
}

func permissionSpecFor(c *Cluster, apiVersion, kind, namespace, verb, subresource string) (permissionSpec, error) {
	ak, err := c.ResolveGVK(apiVersion, kind)
	if err != nil {
		return permissionSpec{}, err
	}
	return permissionSpec{ak: ak, namespace: namespace, verb: verb, subresource: subresource}, nil
}

// PlanApplyPermissions resolves every YAML document through discovery. Server-
// side apply is an HTTP PATCH even when it creates a missing object, so patch is
// the exact RBAC verb for both cases.
func PlanApplyPermissions(ctx context.Context, c *Cluster, yamlText string) (*PermissionPlan, error) {
	docs, err := splitYAMLDocuments(yamlText)
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return nil, fmt.Errorf("no YAML content to plan")
	}
	specs := make([]permissionSpec, 0, len(docs))
	for _, doc := range docs {
		prepared, err := prepareDoc(c, doc)
		if err != nil {
			return nil, err
		}
		if prepared == nil {
			continue
		}
		spec, err := permissionSpecFor(c, prepared.obj.GetAPIVersion(), prepared.obj.GetKind(), prepared.ns, "patch", "")
		if err != nil {
			return nil, err
		}
		specs = append(specs, spec)
	}
	return runPermissionPlan(ctx, c, "apply-yaml", specs, nil), nil
}

func PlanDrainPermissions(ctx context.Context, c *Cluster, nodeName string) (*PermissionPlan, error) {
	node, err := c.ResolveKind("Node")
	if err != nil {
		return nil, err
	}
	pod, err := c.ResolveKind("Pod")
	if err != nil {
		return nil, err
	}
	specs := []permissionSpec{
		{ak: node, verb: "patch"},
		{ak: pod, verb: "list"},
	}
	warnings := []string{}
	pods, listErr := c.Clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{FieldSelector: "spec.nodeName=" + nodeName})
	if listErr != nil {
		warnings = append(warnings, "Kubby could not inspect the pods currently assigned to this node; eviction permissions could not be enumerated.")
	} else {
		for i := range pods.Items {
			candidate := &pods.Items[i]
			_, mirrorPod := candidate.Annotations["kubernetes.io/config.mirror"]
			if isDaemonSetPod(candidate) || mirrorPod {
				continue
			}
			specs = append(specs, permissionSpec{ak: pod, namespace: candidate.Namespace, verb: "create", subresource: "eviction"})
		}
	}
	return runPermissionPlan(ctx, c, "drain-node", specs, warnings), nil
}

type manifestPermissionTarget struct {
	ak        APIKind
	namespace string
	name      string
}

func manifestPermissionTargets(c *Cluster, manifest, defaultNamespace string) (map[string]manifestPermissionTarget, []string) {
	docs, err := splitYAMLDocuments(manifest)
	if err != nil {
		return nil, []string{"The rendered Helm manifest could not be split for permission planning."}
	}
	targets := map[string]manifestPermissionTarget{}
	warnings := []string{}
	for _, doc := range docs {
		var raw map[string]interface{}
		if err := yaml.Unmarshal([]byte(doc), &raw); err != nil || len(raw) == 0 {
			continue
		}
		obj := &unstructured.Unstructured{Object: raw}
		ak, err := c.ResolveGVK(obj.GetAPIVersion(), obj.GetKind())
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s could not be resolved yet; the API server remains the final authority.", obj.GetKind()))
			continue
		}
		ns := obj.GetNamespace()
		if ak.Namespaced && ns == "" {
			ns = defaultNamespace
			if ns == "" {
				ns = defaultApplyNamespace
			}
		}
		if !ak.Namespaced {
			ns = ""
		}
		key := ak.GVR.String() + "\x00" + ns + "\x00" + obj.GetName()
		targets[key] = manifestPermissionTarget{ak: ak, namespace: ns, name: obj.GetName()}
	}
	return targets, warnings
}

func helmStorageSpecs(c *Cluster, namespace string, verbs ...string) []permissionSpec {
	secret, err := c.ResolveKind("Secret")
	if err != nil {
		return nil
	}
	out := make([]permissionSpec, 0, len(verbs))
	for _, verb := range verbs {
		out = append(out, permissionSpec{ak: secret, namespace: namespace, verb: verb})
	}
	return out
}

func planHelmManifests(ctx context.Context, c *Cluster, operation, namespace, current, proposed string, hooks []*release.Hook, hookEvents ...release.HookEvent) *PermissionPlan {
	before, beforeWarnings := manifestPermissionTargets(c, current, namespace)
	after, afterWarnings := manifestPermissionTargets(c, proposed, namespace)
	warnings := append(beforeWarnings, afterWarnings...)
	specs := helmStorageSpecs(c, namespace, "get", "list", "create", "update")
	if operation == "helm-install" && namespaceNeedsCreate(ctx, c, namespace) {
		if ns, err := c.ResolveKind("Namespace"); err == nil {
			specs = append(specs, permissionSpec{ak: ns, verb: "create"})
		}
	}
	for key, target := range after {
		specs = append(specs, permissionSpec{ak: target.ak, namespace: target.namespace, verb: "get"})
		verb := "create"
		if _, exists := before[key]; exists {
			verb = "patch"
		}
		specs = append(specs, permissionSpec{ak: target.ak, namespace: target.namespace, verb: verb})
	}
	for key, target := range before {
		if _, remains := after[key]; !remains {
			specs = append(specs, permissionSpec{ak: target.ak, namespace: target.namespace, verb: "get"})
			specs = append(specs, permissionSpec{ak: target.ak, namespace: target.namespace, verb: "delete"})
		}
	}
	hookSpecs, hookWarnings := helmHookPermissionSpecs(c, namespace, hooks, hookEvents...)
	specs = append(specs, hookSpecs...)
	warnings = append(warnings, hookWarnings...)
	return runPermissionPlan(ctx, c, operation, specs, warnings)
}

func helmHookPermissionSpecs(c *Cluster, namespace string, hooks []*release.Hook, events ...release.HookEvent) ([]permissionSpec, []string) {
	wanted := map[release.HookEvent]bool{}
	for _, event := range events {
		wanted[event] = true
	}
	specs := []permissionSpec{}
	warnings := []string{}
	for _, hook := range hooks {
		matches := false
		for _, event := range hook.Events {
			matches = matches || wanted[event]
		}
		if !matches {
			continue
		}
		targets, targetWarnings := manifestPermissionTargets(c, hook.Manifest, namespace)
		warnings = append(warnings, targetWarnings...)
		logNamespace := namespace
		for _, target := range targets {
			if target.namespace != "" {
				logNamespace = target.namespace
			}
			for _, verb := range []string{"create", "get", "watch", "delete"} {
				specs = append(specs, permissionSpec{ak: target.ak, namespace: target.namespace, verb: verb})
			}
		}
		if len(hook.OutputLogPolicies) > 0 && (hook.Kind == "Pod" || hook.Kind == "Job") {
			if pod, err := c.ResolveKind("Pod"); err == nil {
				specs = append(specs,
					permissionSpec{ak: pod, namespace: logNamespace, verb: "list"},
					permissionSpec{ak: pod, namespace: logNamespace, verb: "get", subresource: "log"},
				)
			}
		}
	}
	return specs, warnings
}

// PlanHelmAction plans workflows that do not already return a dry-run diff.
func PlanHelmAction(ctx context.Context, c *Cluster, operation, namespace, name string, revision int) (*PermissionPlan, error) {
	if operation != "uninstall" && operation != "rollback" && operation != "test" {
		return nil, fmt.Errorf("unsupported Helm permission plan %q", operation)
	}
	if operation == "rollback" && revision <= 0 {
		return nil, fmt.Errorf("rollback revision must be greater than zero")
	}
	baseVerbs := []string{"get", "list", "update"}
	if operation == "uninstall" {
		baseVerbs = append(baseVerbs, "delete")
	} else if operation == "rollback" {
		baseVerbs = append(baseVerbs, "create")
	}
	base := helmStorageSpecs(c, namespace, baseVerbs...)
	cfg, err := newHelmConfig(ctx, c, namespace)
	if err != nil {
		return runPermissionPlan(ctx, c, "helm-"+operation, base, []string{"The Helm release could not be loaded for resource-level permission planning."}), nil
	}
	current, err := action.NewGet(cfg).Run(name)
	if err != nil {
		return runPermissionPlan(ctx, c, "helm-"+operation, base, []string{"The Helm release could not be loaded for resource-level permission planning."}), nil
	}

	switch operation {
	case "uninstall":
		targets, warnings := manifestPermissionTargets(c, current.Manifest, namespace)
		for _, target := range targets {
			base = append(base, permissionSpec{ak: target.ak, namespace: target.namespace, verb: "delete"})
		}
		hookSpecs, hookWarnings := helmHookPermissionSpecs(c, namespace, current.Hooks, release.HookPreDelete, release.HookPostDelete)
		base = append(base, hookSpecs...)
		warnings = append(warnings, hookWarnings...)
		return runPermissionPlan(ctx, c, "helm-uninstall", base, warnings), nil
	case "rollback":
		get := action.NewGet(cfg)
		get.Version = revision
		target, getErr := get.Run(name)
		if getErr != nil {
			return nil, getErr
		}
		return planHelmManifests(ctx, c, "helm-rollback", namespace, current.Manifest, target.Manifest, target.Hooks, release.HookPreRollback, release.HookPostRollback), nil
	case "test":
		hookSpecs, warnings := helmHookPermissionSpecs(c, namespace, current.Hooks, release.HookTest)
		base = append(base, hookSpecs...)
		return runPermissionPlan(ctx, c, "helm-test", base, warnings), nil
	}
	return nil, fmt.Errorf("unsupported Helm permission plan %q", operation)
}

func namespaceNeedsCreate(ctx context.Context, c *Cluster, namespace string) bool {
	if namespace == "" {
		return false
	}
	_, err := c.Clientset.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	return apierrors.IsNotFound(err)
}
