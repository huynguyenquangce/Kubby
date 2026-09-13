package k8sclient

import (
	"context"
	"fmt"
	"sort"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Relation trees for the policy kinds. Each answers the question an operator
// asks first when looking at one: which workload does this autoscaler drive,
// which Pods does this budget protect, which Pods does this policy isolate —
// and each child opens its own drawer.

// HPATree links an autoscaler to the workload it scales.
func HPATree(ctx context.Context, c *Cluster, namespace, name string) (*RelationNode, error) {
	hpa, err := c.Clientset.AutoscalingV2().HorizontalPodAutoscalers(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	root := &RelationNode{
		Kind: "HorizontalPodAutoscaler", Name: hpa.Name, Namespace: hpa.Namespace,
		Status:   fmt.Sprintf("%s replicas · range %d–%d", hpaReplicas(hpa), hpaMinReplicas(hpa), hpa.Spec.MaxReplicas),
		Children: []*RelationNode{},
	}
	target := &RelationNode{Kind: hpa.Spec.ScaleTargetRef.Kind, Name: hpa.Spec.ScaleTargetRef.Name, Namespace: hpa.Namespace, Children: []*RelationNode{}}
	switch target.Kind {
	case "Deployment":
		if dep, err := c.Clientset.AppsV1().Deployments(namespace).Get(ctx, target.Name, metav1.GetOptions{}); err == nil {
			desired := int32(1)
			if dep.Spec.Replicas != nil {
				desired = *dep.Spec.Replicas
			}
			target.Status = fmt.Sprintf("%d/%d ready", dep.Status.ReadyReplicas, desired)
			target.IsError = dep.Status.ReadyReplicas < desired
		} else {
			target.Status, target.IsError = relationTargetError(err), true
		}
	case "StatefulSet":
		if set, err := c.Clientset.AppsV1().StatefulSets(namespace).Get(ctx, target.Name, metav1.GetOptions{}); err == nil {
			desired := int32(1)
			if set.Spec.Replicas != nil {
				desired = *set.Spec.Replicas
			}
			target.Status = fmt.Sprintf("%d/%d ready", set.Status.ReadyReplicas, desired)
			target.IsError = set.Status.ReadyReplicas < desired
		} else {
			target.Status, target.IsError = relationTargetError(err), true
		}
	}
	root.Children = append(root.Children, target)
	return root, nil
}

func relationTargetError(err error) string {
	if apierrors.IsNotFound(err) {
		return "not found — this autoscaler has nothing to scale"
	}
	return "could not be read"
}

// PDBTree lists the Pods a disruption budget protects.
func PDBTree(ctx context.Context, c *Cluster, namespace, name string) (*RelationNode, error) {
	pdb, err := c.Clientset.PolicyV1().PodDisruptionBudgets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	root := &RelationNode{
		Kind: "PodDisruptionBudget", Name: pdb.Name, Namespace: pdb.Namespace,
		Status:   fmt.Sprintf("allows %d disruption(s)", pdb.Status.DisruptionsAllowed),
		IsError:  pdb.Status.ExpectedPods > 0 && pdb.Status.DisruptionsAllowed == 0,
		Children: []*RelationNode{},
	}
	selector, err := metav1.LabelSelectorAsSelector(pdb.Spec.Selector)
	if err != nil || selector.Empty() {
		root.Status += " · selects no Pods"
		return root, nil
	}
	pods, err := c.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return nil, err
	}
	for i := range pods.Items {
		status, _, _ := podStatus(pods.Items[i])
		root.Children = append(root.Children, &RelationNode{
			Kind: "Pod", Name: pods.Items[i].Name, Namespace: pods.Items[i].Namespace,
			Status: status, IsError: isErroredPodStatus(status),
		})
	}
	sortRelationChildren(root)
	return root, nil
}

// NetworkPolicyTree lists the Pods a policy currently selects.
func NetworkPolicyTree(ctx context.Context, c *Cluster, namespace, name string) (*RelationNode, error) {
	policy, err := c.Clientset.NetworkingV1().NetworkPolicies(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	root := &RelationNode{
		Kind: "NetworkPolicy", Name: policy.Name, Namespace: policy.Namespace,
		Status: networkPolicyEffect(policy), Children: []*RelationNode{},
	}
	pods, err := c.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	compiled := compileNetworkPolicy(policy)
	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.DeletionTimestamp != nil || !compiled.selects(pod) {
			continue
		}
		status, _, _ := podStatus(*pod)
		root.Children = append(root.Children, &RelationNode{
			Kind: "Pod", Name: pod.Name, Namespace: pod.Namespace, Status: status, IsError: isErroredPodStatus(status),
		})
	}
	if len(root.Children) == 0 {
		root.Status += " · selects no Pods right now"
	}
	sortRelationChildren(root)
	return root, nil
}

func sortRelationChildren(node *RelationNode) {
	sort.Slice(node.Children, func(i, j int) bool { return node.Children[i].Name < node.Children[j].Name })
}
