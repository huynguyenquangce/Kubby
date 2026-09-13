package k8sclient

import (
	"context"
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// DrainImpact is what a drain would do to the Pods on one Node, read before
// the confirmation so the operator sees the consequences that `kubectl drain`
// would stop and ask about: disruption budgets that will refuse evictions,
// Pods no controller will recreate, and emptyDir data that is deleted.
type DrainImpact struct {
	Node            string        `json:"node"`
	Evict           int           `json:"evict"`
	DaemonSetPods   int           `json:"daemonSetPods"`
	MirrorPods      int           `json:"mirrorPods"`
	Unmanaged       []string      `json:"unmanaged"` // "namespace/name" of Pods with no controller
	EmptyDir        []string      `json:"emptyDir"`  // "namespace/name" of Pods whose emptyDir data is lost
	BlockingBudgets []DrainBudget `json:"blockingBudgets"`
	Warnings        []string      `json:"warnings"`
}

// DrainBudget is a PodDisruptionBudget that allows fewer disruptions than the
// number of its Pods on the Node, so some evictions will be refused.
type DrainBudget struct {
	Namespace          string `json:"namespace"`
	Name               string `json:"name"`
	AllowedDisruptions int32  `json:"allowedDisruptions"`
	PodsOnNode         int    `json:"podsOnNode"`
}

// DrainImpactFor previews a drain without changing anything. It lists the
// Node's Pods once and each affected namespace's budgets once.
func DrainImpactFor(ctx context.Context, c *Cluster, nodeName string) (*DrainImpact, error) {
	pods, err := c.Clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{FieldSelector: "spec.nodeName=" + nodeName})
	if err != nil {
		return nil, err
	}
	impact := &DrainImpact{
		Node: nodeName, Unmanaged: []string{}, EmptyDir: []string{},
		BlockingBudgets: []DrainBudget{}, Warnings: []string{},
	}
	evictable := map[string][]*corev1.Pod{}
	for i := range pods.Items {
		pod := &pods.Items[i]
		switch {
		case isDaemonSetPod(pod):
			impact.DaemonSetPods++
			continue
		case pod.Annotations[corev1.MirrorPodAnnotationKey] != "":
			impact.MirrorPods++
			continue
		}
		impact.Evict++
		evictable[pod.Namespace] = append(evictable[pod.Namespace], pod)
		key := pod.Namespace + "/" + pod.Name
		finished := pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed
		if !finished && metav1.GetControllerOf(pod) == nil {
			impact.Unmanaged = append(impact.Unmanaged, key)
		}
		for _, volume := range pod.Spec.Volumes {
			if volume.EmptyDir != nil {
				impact.EmptyDir = append(impact.EmptyDir, key)
				break
			}
		}
	}

	namespaces := make([]string, 0, len(evictable))
	for namespace := range evictable {
		namespaces = append(namespaces, namespace)
	}
	sort.Strings(namespaces)
	for _, namespace := range namespaces {
		budgets, err := c.Clientset.PolicyV1().PodDisruptionBudgets(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			impact.Warnings = append(impact.Warnings, fmt.Sprintf("PodDisruptionBudgets in %s could not be read (%v); evictions there may still be refused.", namespace, err))
			continue
		}
		for i := range budgets.Items {
			if budget := blockingBudget(&budgets.Items[i], evictable[namespace]); budget != nil {
				impact.BlockingBudgets = append(impact.BlockingBudgets, *budget)
			}
		}
	}
	sort.Strings(impact.Unmanaged)
	sort.Strings(impact.EmptyDir)
	return impact, nil
}

// blockingBudget reports a budget covering more of the Node's Pods than it
// currently allows to be disrupted. Evictions are issued one after another, so
// every eviction past the allowance is refused until replacements are healthy.
func blockingBudget(pdb *policyv1.PodDisruptionBudget, pods []*corev1.Pod) *DrainBudget {
	if pdb.DeletionTimestamp != nil || pdb.Spec.Selector == nil {
		return nil
	}
	selector, err := metav1.LabelSelectorAsSelector(pdb.Spec.Selector)
	if err != nil || selector.Empty() {
		return nil
	}
	covered := 0
	for _, pod := range pods {
		if selector.Matches(labels.Set(pod.Labels)) {
			covered++
		}
	}
	if covered == 0 || int32(covered) <= pdb.Status.DisruptionsAllowed {
		return nil
	}
	return &DrainBudget{Namespace: pdb.Namespace, Name: pdb.Name, AllowedDisruptions: pdb.Status.DisruptionsAllowed, PodsOnNode: covered}
}
