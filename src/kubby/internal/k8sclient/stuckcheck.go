package k8sclient

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// An object stuck Terminating has a deletionTimestamp but never goes away,
// almost always because a finalizer's controller will not or cannot remove its
// finalizer. Kubernetes does not say which one, so the operator greps through
// every resource type. This check lists metadata for every listable type,
// finds the terminating objects and explains the usual causes.
//
// It never removes a finalizer or force-deletes: that skips cleanup the
// controller would do. It offers the kubectl command so the decision stays the
// operator's.

// stuckAfter separates a normal, in-progress deletion from a stuck one.
const stuckAfter = 5 * time.Minute

// stuckScanSkip are resource types whose deletions are not worth scanning.
var stuckScanSkip = map[string]bool{
	"events":            true,
	"componentstatuses": true,
}

type StuckObject struct {
	Kind        string         `json:"kind"`
	RefKind     string         `json:"refKind"` // kind as the drawer resolves it
	APIVersion  string         `json:"apiVersion"`
	Namespace   string         `json:"namespace"`
	Name        string         `json:"name"`
	DeletedAt   string         `json:"deletedAt"`
	Terminating string         `json:"terminating"` // time since deletion, e.g. "3d"
	Stuck       bool           `json:"stuck"`
	Finalizers  []string       `json:"finalizers"`
	Severity    string         `json:"severity"`
	Findings    []CheckFinding `json:"findings"`
	// Command completes the deletion by force. It is shown for the operator to
	// copy and is never run by Kubby.
	Command     string `json:"command"`
	CommandNote string `json:"commandNote"`
}

type StuckReport struct {
	Objects  []StuckObject `json:"objects"`
	Scanned  int           `json:"scanned"` // resource types listed
	Failed   int           `json:"failed"`  // resource types that could not be listed
	Critical int           `json:"critical"`
	Warning  int           `json:"warning"`
	Warnings []string      `json:"warnings"`
}

type terminatingCandidate struct {
	kind       APIKind
	namespace  string
	name       string
	deleted    time.Time
	finalizers []string
}

// finalizerExplanations are the finalizers Kubernetes itself and common
// controllers add; anything else gets the generic explanation.
var finalizerExplanations = map[string]string{
	"kubernetes.io/pvc-protection": "Kubernetes keeps a claim while a Pod still uses it.",
	"kubernetes.io/pv-protection":  "Kubernetes keeps a volume while it is bound to a claim.",
	"foregroundDeletion":           "Foreground deletion: the garbage collector deletes its dependents first, and one stuck dependent holds it.",
	"orphan":                       "The garbage collector is orphaning its dependents before removing it.",
	"kubernetes":                   "The namespace controller is deleting every object in the namespace.",
	"customresourcecleanup.apiextensions.k8s.io":  "Every object of this custom resource is deleted first; one stuck object holds the CRD.",
	"service.kubernetes.io/load-balancer-cleanup": "The cloud controller manager must release the cloud load balancer first.",
	"batch.kubernetes.io/job-tracking":            "The Job controller records this Pod's completion before releasing it.",
}

func scanStuckObjects(ctx context.Context, c *Cluster, namespace string) StuckReport {
	report := StuckReport{Objects: []StuckObject{}, Warnings: []string{}}
	if c.Meta == nil {
		report.Warnings = append(report.Warnings, "The metadata client is unavailable, so terminating objects were not scanned.")
		return report
	}
	idx, err := c.apiKinds(false)
	if err != nil {
		report.Warnings = append(report.Warnings, fmt.Sprintf("API discovery failed, so terminating objects were not scanned: %v", err))
		return report
	}
	kinds := []APIKind{}
	for _, served := range idx.byKind {
		for _, ak := range served {
			if !ak.Listable || stuckScanSkip[ak.GVR.Resource] {
				continue
			}
			// A scoped view scans its namespace's objects and the namespace itself.
			if namespace != "" && !ak.Namespaced && ak.Kind != "Namespace" {
				continue
			}
			kinds = append(kinds, ak)
		}
	}
	sort.Slice(kinds, func(i, j int) bool { return kinds[i].GVR.String() < kinds[j].GVR.String() })
	report.Scanned = len(kinds)

	var mu sync.Mutex
	var wg sync.WaitGroup
	candidates := []terminatingCandidate{}
	failed := []string{}
	sem := make(chan struct{}, countConcurrency)
	for _, ak := range kinds {
		wg.Add(1)
		go func(ak APIKind) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			listNamespace := ""
			if ak.Namespaced {
				listNamespace = namespace
			}
			list, err := c.Meta.Resource(ak.GVR).Namespace(listNamespace).List(ctx, metav1.ListOptions{})
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failed = append(failed, ak.GVR.GroupResource().String())
				return
			}
			for _, item := range list.Items {
				if item.DeletionTimestamp == nil {
					continue
				}
				if ak.Kind == "Namespace" && ak.GVR.Group == "" && namespace != "" && item.Name != namespace {
					continue
				}
				candidates = append(candidates, terminatingCandidate{
					kind: ak, namespace: item.Namespace, name: item.Name,
					deleted: item.DeletionTimestamp.Time, finalizers: append([]string(nil), item.Finalizers...),
				})
			}
		}(ak)
	}
	wg.Wait()
	report.Failed = len(failed)
	if len(failed) > 0 {
		sort.Strings(failed)
		report.Warnings = append(report.Warnings, fmt.Sprintf("%d resource type(s) could not be listed and were not scanned: %s", len(failed), strings.Join(failed, ", ")))
	}

	for _, candidate := range candidates {
		object, gone := explainTerminating(ctx, c, idx, candidate)
		if gone {
			continue
		}
		switch object.Severity {
		case SeverityCritical:
			report.Critical++
		case SeverityWarning:
			report.Warning++
		}
		report.Objects = append(report.Objects, object)
	}
	sort.SliceStable(report.Objects, func(i, j int) bool {
		a, b := report.Objects[i], report.Objects[j]
		if checkSeverityRank(a.Severity) != checkSeverityRank(b.Severity) {
			return checkSeverityRank(a.Severity) > checkSeverityRank(b.Severity)
		}
		if a.DeletedAt != b.DeletedAt {
			return a.DeletedAt < b.DeletedAt // longest-terminating first
		}
		return a.Kind+a.Namespace+a.Name < b.Kind+b.Namespace+b.Name
	})
	return report
}

// explainTerminating turns one terminating object into findings. gone reports
// an object that finished deleting between the scan and the explanation.
func explainTerminating(ctx context.Context, c *Cluster, idx *apiIndex, candidate terminatingCandidate) (StuckObject, bool) {
	ak := candidate.kind
	elapsed := checksNow().Sub(candidate.deleted)
	object := StuckObject{
		Kind: ak.Kind, RefKind: ak.Kind, APIVersion: ak.GVR.GroupVersion().String(),
		Namespace: candidate.namespace, Name: candidate.name,
		DeletedAt:   candidate.deleted.UTC().Format(time.RFC3339),
		Terminating: durationText(elapsed), Stuck: elapsed >= stuckAfter,
		Finalizers: candidate.finalizers, Findings: []CheckFinding{},
	}
	if object.Finalizers == nil {
		object.Finalizers = []string{}
	}
	if len(idx.byKind[ak.Kind]) > 1 && ak.GVR.Group != "" {
		object.RefKind = ak.Kind + "." + ak.GVR.Group
	}
	add := func(severity, title, detail string) {
		object.Findings = append(object.Findings, CheckFinding{Severity: severity, Title: title, Detail: detail})
	}
	resourceArg := kubectlResourceArg(ak)
	namespaceArg := ""
	if candidate.namespace != "" {
		namespaceArg = " -n " + candidate.namespace
	}
	forcedPod := false
	critical := false

	if ak.GVR.Group == "" {
		switch ak.Kind {
		case "Namespace":
			ns, err := c.Clientset.CoreV1().Namespaces().Get(ctx, candidate.name, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				return object, true
			}
			if err == nil {
				critical = explainNamespace(ns, add)
			}
		case "Pod":
			pod, err := c.Clientset.CoreV1().Pods(candidate.namespace).Get(ctx, candidate.name, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				return object, true
			}
			if err == nil {
				forcedPod = explainPod(ctx, c, pod, add)
			}
		case "PersistentVolumeClaim":
			if containsAny(candidate.finalizers, "kubernetes.io/pvc-protection") {
				explainClaimInUse(ctx, c, candidate.namespace, candidate.name, add)
			}
		case "PersistentVolume":
			if containsAny(candidate.finalizers, "kubernetes.io/pv-protection") {
				if pv, err := c.Clientset.CoreV1().PersistentVolumes().Get(ctx, candidate.name, metav1.GetOptions{}); err == nil && pv.Spec.ClaimRef != nil {
					add(SeverityInfo, "Still bound to a claim",
						fmt.Sprintf("It is bound to PersistentVolumeClaim %s/%s; deletion completes once that claim is gone.", pv.Spec.ClaimRef.Namespace, pv.Spec.ClaimRef.Name))
				}
			}
		}
	}
	for _, finalizer := range candidate.finalizers {
		explanation, known := finalizerExplanations[finalizer]
		if !known {
			explanation = fmt.Sprintf("It is removed by the controller that added it. If that controller is uninstalled, crashing or lacks permission, deletion never completes — check the controller for %s.", finalizerOwner(finalizer))
		}
		add(SeverityInfo, "Finalizer "+finalizer, explanation)
	}
	if len(candidate.finalizers) == 0 && len(object.Findings) == 0 {
		add(SeverityInfo, "No finalizer holds it", "The controller responsible is still completing the deletion.")
	}

	switch {
	case forcedPod:
		object.Command = fmt.Sprintf("kubectl delete pod %s%s --grace-period=0 --force", candidate.name, namespaceArg)
		object.CommandNote = "Only after confirming the node is really down: on a live node the old Pod can keep running beside its replacement."
	case len(candidate.finalizers) > 0:
		object.Command = fmt.Sprintf(`kubectl patch %s %s%s --type=merge -p '{"metadata":{"finalizers":null}}'`, resourceArg, candidate.name, namespaceArg)
		object.CommandNote = "Removing finalizers skips the cleanup their controllers would do: external resources such as cloud disks or load balancers can be left behind."
	case ak.GVR.Group == "" && ak.Kind == "Namespace":
		object.CommandNote = "A namespace finishes deleting once its content is gone: resolve the terminating objects in it first."
	}

	switch {
	case !object.Stuck:
		object.Severity = SeverityInfo
	case critical:
		object.Severity = SeverityCritical
	default:
		object.Severity = SeverityWarning
	}
	return object, false
}

// explainNamespace reports the namespace controller's own conditions, which say
// exactly what is left. It returns true for a failure that blocks every
// namespace deletion in the cluster.
func explainNamespace(ns *corev1.Namespace, add func(string, string, string)) bool {
	critical := false
	for _, condition := range ns.Status.Conditions {
		if condition.Status != corev1.ConditionTrue {
			continue
		}
		switch condition.Type {
		case corev1.NamespaceDeletionDiscoveryFailure:
			critical = true
			add(SeverityCritical, "API discovery failed during deletion",
				strings.TrimSpace(condition.Message+" An unavailable aggregated APIService (often metrics.k8s.io) blocks every namespace deletion until it is fixed or removed."))
		case corev1.NamespaceContentRemaining:
			add(SeverityWarning, "Content remaining", condition.Message)
		case corev1.NamespaceFinalizersRemaining:
			add(SeverityWarning, "Content held by finalizers", condition.Message)
		case corev1.NamespaceDeletionContentFailure, corev1.NamespaceDeletionGVParsingFailure:
			add(SeverityWarning, string(condition.Type), condition.Message)
		}
	}
	return critical
}

// explainPod explains a Pod that waits for a kubelet that cannot answer. It
// returns true when force deletion is the usual resolution.
func explainPod(ctx context.Context, c *Cluster, pod *corev1.Pod, add func(string, string, string)) bool {
	if pod.Spec.NodeName == "" {
		add(SeverityInfo, "Never scheduled", "No kubelet has to confirm its termination; the deletion should complete on its own.")
		return false
	}
	node, err := c.Clientset.CoreV1().Nodes().Get(ctx, pod.Spec.NodeName, metav1.GetOptions{})
	switch {
	case apierrors.IsNotFound(err):
		add(SeverityWarning, "Its node no longer exists",
			fmt.Sprintf("Node %s is gone, so no kubelet will ever confirm this Pod stopped.", pod.Spec.NodeName))
		return true
	case err != nil:
		return false
	}
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady && condition.Status != corev1.ConditionTrue {
			add(SeverityWarning, "Its node is NotReady",
				fmt.Sprintf("Node %s cannot confirm the Pod stopped, so the API server keeps it Terminating.", node.Name))
			return true
		}
	}
	if pod.DeletionGracePeriodSeconds != nil {
		add(SeverityInfo, "Waiting for the kubelet",
			fmt.Sprintf("Node %s is Ready; the Pod had a %ds grace period to stop its containers.", node.Name, *pod.DeletionGracePeriodSeconds))
	}
	return false
}

func explainClaimInUse(ctx context.Context, c *Cluster, namespace, claim string, add func(string, string, string)) {
	pods, err := c.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return
	}
	users := []string{}
	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		for _, volume := range pod.Spec.Volumes {
			if volume.PersistentVolumeClaim != nil && volume.PersistentVolumeClaim.ClaimName == claim {
				users = append(users, pod.Name)
				break
			}
		}
	}
	if len(users) > 0 {
		sort.Strings(users)
		add(SeverityInfo, "Still used by Pods",
			fmt.Sprintf("Pod %s still mounts it; the claim is deleted once no Pod uses it.", strings.Join(users, ", ")))
	}
}

// finalizerOwner guesses the component a domain-qualified finalizer belongs to.
func finalizerOwner(finalizer string) string {
	if i := strings.Index(finalizer, "/"); i > 0 {
		return finalizer[:i]
	}
	return finalizer
}
