package k8sclient

import (
	"context"
	"fmt"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// RelationNode is one node in the Deployment → ReplicaSet → Pod tree.
type RelationNode struct {
	Kind      string          `json:"kind"`
	Name      string          `json:"name"`
	Namespace string          `json:"namespace"`
	Status    string          `json:"status"`
	IsError   bool            `json:"isError"`
	Children  []*RelationNode `json:"children"`
}

// DeploymentTree builds the ReplicaSet → Pod tree owned by a deployment.
func DeploymentTree(ctx context.Context, c *Cluster, namespace, name string) (*RelationNode, error) {
	dep, err := c.Clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	root := &RelationNode{Kind: "Deployment", Name: dep.Name, Namespace: dep.Namespace, Status: "", Children: []*RelationNode{}}

	// ReplicaSets owned by this deployment.
	rsList, err := c.Clientset.AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	// Pods in the namespace (matched to their owning ReplicaSet by ownerRef).
	podList, err := c.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	for i := range rsList.Items {
		rs := &rsList.Items[i]
		if !ownedBy(rs.OwnerReferences, "Deployment", dep.Name) {
			continue
		}
		// Skip old scaled-down ReplicaSets with no replicas to reduce noise.
		if rs.Status.Replicas == 0 && (rs.Spec.Replicas == nil || *rs.Spec.Replicas == 0) {
			continue
		}
		rsNode := &RelationNode{
			Kind:      "ReplicaSet",
			Name:      rs.Name,
			Namespace: rs.Namespace,
			Status:    fmt.Sprintf("%d/%d ready", rs.Status.ReadyReplicas, rs.Status.Replicas),
			Children:  []*RelationNode{},
		}
		for j := range podList.Items {
			pod := podList.Items[j]
			if !ownedBy(pod.OwnerReferences, "ReplicaSet", rs.Name) {
				continue
			}
			status, _, _ := podStatus(pod)
			rsNode.Children = append(rsNode.Children, &RelationNode{
				Kind:      "Pod",
				Name:      pod.Name,
				Namespace: pod.Namespace,
				Status:    status,
				IsError:   erroredStatuses[status],
			})
		}
		root.Children = append(root.Children, rsNode)
	}
	return root, nil
}

// ServiceTree builds a Service → Pod tree (pods matched by the service's
// selector), so the drawer can show what a service actually routes to.
func ServiceTree(ctx context.Context, c *Cluster, namespace, name string) (*RelationNode, error) {
	svc, err := c.Clientset.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	root := &RelationNode{Kind: "Service", Name: svc.Name, Namespace: svc.Namespace, Children: []*RelationNode{}}
	if len(svc.Spec.Selector) == 0 {
		root.Status = "no selector"
		return root, nil
	}
	sel := labels.SelectorFromSet(svc.Spec.Selector).String()
	pods, err := c.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: sel})
	if err != nil {
		return nil, err
	}
	root.Status = fmt.Sprintf("%d pod(s)", len(pods.Items))
	for i := range pods.Items {
		pod := pods.Items[i]
		status, _, _ := podStatus(pod)
		root.Children = append(root.Children, &RelationNode{
			Kind: "Pod", Name: pod.Name, Namespace: pod.Namespace, Status: status, IsError: erroredStatuses[status],
		})
	}
	return root, nil
}

// IngressTree builds an Ingress → Service → Pod tree following the backend
// service references in the ingress rules.
func IngressTree(ctx context.Context, c *Cluster, namespace, name string) (*RelationNode, error) {
	ing, err := c.Clientset.NetworkingV1().Ingresses(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	root := &RelationNode{Kind: "Ingress", Name: ing.Name, Namespace: ing.Namespace, Children: []*RelationNode{}}
	seen := map[string]bool{}
	for _, rule := range ing.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}
		for _, p := range rule.HTTP.Paths {
			if p.Backend.Service == nil {
				continue
			}
			svcName := p.Backend.Service.Name
			if seen[svcName] {
				continue
			}
			seen[svcName] = true
			svcNode, err := ServiceTree(ctx, c, namespace, svcName)
			if err != nil {
				// Service may not exist; show it as a broken link rather than failing.
				svcNode = &RelationNode{Kind: "Service", Name: svcName, Namespace: namespace, Status: "not found", IsError: true}
			}
			root.Children = append(root.Children, svcNode)
		}
	}
	return root, nil
}

func ownedBy(refs []metav1.OwnerReference, kind, name string) bool {
	for _, r := range refs {
		if r.Kind == kind && r.Name == name {
			return true
		}
	}
	return false
}

// NodeMetric is CPU/memory usage for a node (requires metrics-server).
type NodeMetric struct {
	Name        string `json:"name"`
	CPUMilli    int64  `json:"cpuMilli"`
	MemMi       int64  `json:"memMi"`
	CPUCapacity int64  `json:"cpuCapacity"` // millicores
	MemCapacity int64  `json:"memCapacity"` // Mi
}

// NodeMetrics returns per-node usage plus capacity. Returns (nil, nil) when
// metrics-server is not installed so the UI can show a friendly hint.
func NodeMetrics(ctx context.Context, c *Cluster) ([]NodeMetric, error) {
	if c.Metrics == nil {
		return nil, nil
	}
	usage, err := c.Metrics.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, nil // metrics API not available — treat as "no data"
	}

	nodes, err := c.Clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	capByName := map[string]NodeMetric{}
	for _, n := range nodes.Items {
		capByName[n.Name] = NodeMetric{
			CPUCapacity: n.Status.Capacity.Cpu().MilliValue(),
			MemCapacity: n.Status.Capacity.Memory().Value() / (1024 * 1024),
		}
	}

	out := make([]NodeMetric, 0, len(usage.Items))
	for _, m := range usage.Items {
		cap := capByName[m.Name]
		out = append(out, NodeMetric{
			Name:        m.Name,
			CPUMilli:    m.Usage.Cpu().MilliValue(),
			MemMi:       m.Usage.Memory().Value() / (1024 * 1024),
			CPUCapacity: cap.CPUCapacity,
			MemCapacity: cap.MemCapacity,
		})
	}
	return out, nil
}

// PodMetric is aggregate CPU/memory usage for a pod (requires metrics-server).
type PodMetric struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	CPUMilli  int64  `json:"cpuMilli"`
	MemMi     int64  `json:"memMi"`
}

// PodMetricsList returns CPU/mem usage for every pod in a namespace ("" = all).
// Returns (nil, nil) when metrics-server is unavailable. Used to merge live
// usage into the pods table.
func PodMetricsList(ctx context.Context, c *Cluster, namespace string) ([]PodMetric, error) {
	if c.Metrics == nil {
		return nil, nil
	}
	list, err := c.Metrics.MetricsV1beta1().PodMetricses(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, nil
	}
	out := make([]PodMetric, 0, len(list.Items))
	for _, pm := range list.Items {
		var cpu, mem int64
		for _, ct := range pm.Containers {
			cpu += ct.Usage.Cpu().MilliValue()
			mem += ct.Usage.Memory().Value() / (1024 * 1024)
		}
		out = append(out, PodMetric{Namespace: pm.Namespace, Name: pm.Name, CPUMilli: cpu, MemMi: mem})
	}
	return out, nil
}

// TopPods returns the top `limit` pods by CPU usage. Returns (nil, nil) when
// metrics-server is unavailable so the UI can show a friendly hint.
func TopPods(ctx context.Context, c *Cluster, limit int) ([]PodMetric, error) {
	if c.Metrics == nil {
		return nil, nil
	}
	list, err := c.Metrics.MetricsV1beta1().PodMetricses("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, nil
	}
	out := make([]PodMetric, 0, len(list.Items))
	for _, pm := range list.Items {
		var cpu, mem int64
		for _, ct := range pm.Containers {
			cpu += ct.Usage.Cpu().MilliValue()
			mem += ct.Usage.Memory().Value() / (1024 * 1024)
		}
		out = append(out, PodMetric{Namespace: pm.Namespace, Name: pm.Name, CPUMilli: cpu, MemMi: mem})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CPUMilli > out[j].CPUMilli })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
