package k8sclient

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// NamespaceInfo is the trimmed-down data the frontend renders.
type NamespaceInfo struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Age    string `json:"age"`
}

// NodeInfo is the trimmed-down data the frontend renders.
type NodeInfo struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Ready   bool   `json:"ready"`
	Role    string `json:"role"`
	Version string `json:"version"`
	Age     string `json:"age"`
}

// PodInfo is the trimmed-down data the frontend renders. IsError drives the red highlight.
type PodInfo struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Ready     string `json:"ready"`
	Restarts  int32  `json:"restarts"`
	IsError   bool   `json:"isError"`
	PodIP     string `json:"podIP"`
	Node      string `json:"node"`
	Age       string `json:"age"`
}

// DeploymentInfo is the trimmed-down data the frontend renders.
type DeploymentInfo struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Ready     string `json:"ready"`
	UpToDate  int32  `json:"upToDate"`
	Available int32  `json:"available"`
	IsError   bool   `json:"isError"`
	Age       string `json:"age"`
}

// ServiceInfo is the trimmed-down data the frontend renders.
type ServiceInfo struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	ClusterIP string `json:"clusterIP"`
	Ports     string `json:"ports"`
	Age       string `json:"age"`
}

// ConfigMapInfo / SecretInfo are trimmed-down list rows.
type ConfigMapInfo struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Keys      int    `json:"keys"`
	Age       string `json:"age"`
}

type SecretInfo struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Keys      int    `json:"keys"`
	Age       string `json:"age"`
}

// erroredStatuses are pod statuses treated as errors so the frontend can flag them (FR-3).
var erroredStatuses = map[string]bool{
	"CrashLoopBackOff":           true,
	"ImagePullBackOff":           true,
	"ErrImagePull":               true,
	"CreateContainerConfigError": true,
	"Failed":                     true,
	"Error":                      true,
	"Evicted":                    true,
}

func ListNamespaces(ctx context.Context, client kubernetes.Interface) ([]NamespaceInfo, error) {
	list, err := client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	result := make([]NamespaceInfo, 0, len(list.Items))
	for _, ns := range list.Items {
		result = append(result, NamespaceInfo{
			Name:   ns.Name,
			Status: string(ns.Status.Phase),
			Age:    age(ns.CreationTimestamp),
		})
	}
	return result, nil
}

func ListNodes(ctx context.Context, client kubernetes.Interface) ([]NodeInfo, error) {
	list, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	result := make([]NodeInfo, 0, len(list.Items))
	for _, node := range list.Items {
		ready := false
		status := "NotReady"
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady {
				ready = cond.Status == corev1.ConditionTrue
				if ready {
					status = "Ready"
				}
			}
		}

		role := "worker"
		if _, ok := node.Labels["node-role.kubernetes.io/control-plane"]; ok {
			role = "control-plane"
		}

		result = append(result, NodeInfo{
			Name:    node.Name,
			Status:  status,
			Ready:   ready,
			Role:    role,
			Version: node.Status.NodeInfo.KubeletVersion,
			Age:     age(node.CreationTimestamp),
		})
	}
	return result, nil
}

// ListPods lists pods in a namespace, or the whole cluster when namespace == "".
func ListPods(ctx context.Context, client kubernetes.Interface, namespace string) ([]PodInfo, error) {
	list, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	result := make([]PodInfo, 0, len(list.Items))
	for _, pod := range list.Items {
		status, restarts, ready := podStatus(pod)
		result = append(result, PodInfo{
			Namespace: pod.Namespace,
			Name:      pod.Name,
			Status:    status,
			Ready:     ready,
			Restarts:  restarts,
			IsError:   erroredStatuses[status],
			PodIP:     pod.Status.PodIP,
			Node:      pod.Spec.NodeName,
			Age:       age(pod.CreationTimestamp),
		})
	}
	return result, nil
}

func ListDeployments(ctx context.Context, client kubernetes.Interface, namespace string) ([]DeploymentInfo, error) {
	list, err := client.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	result := make([]DeploymentInfo, 0, len(list.Items))
	for _, d := range list.Items {
		desired := int32(0)
		if d.Spec.Replicas != nil {
			desired = *d.Spec.Replicas
		}
		result = append(result, DeploymentInfo{
			Namespace: d.Namespace,
			Name:      d.Name,
			Ready:     fmt.Sprintf("%d/%d", d.Status.ReadyReplicas, desired),
			UpToDate:  d.Status.UpdatedReplicas,
			Available: d.Status.AvailableReplicas,
			IsError:   d.Status.ReadyReplicas < desired,
			Age:       age(d.CreationTimestamp),
		})
	}
	return result, nil
}

func ListServices(ctx context.Context, client kubernetes.Interface, namespace string) ([]ServiceInfo, error) {
	list, err := client.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	result := make([]ServiceInfo, 0, len(list.Items))
	for _, s := range list.Items {
		ports := ""
		for i, p := range s.Spec.Ports {
			if i > 0 {
				ports += ", "
			}
			ports += fmt.Sprintf("%d/%s", p.Port, p.Protocol)
		}
		result = append(result, ServiceInfo{
			Namespace: s.Namespace,
			Name:      s.Name,
			Type:      string(s.Spec.Type),
			ClusterIP: s.Spec.ClusterIP,
			Ports:     ports,
			Age:       age(s.CreationTimestamp),
		})
	}
	return result, nil
}

func ListConfigMaps(ctx context.Context, client kubernetes.Interface, namespace string) ([]ConfigMapInfo, error) {
	list, err := client.CoreV1().ConfigMaps(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	result := make([]ConfigMapInfo, 0, len(list.Items))
	for _, cm := range list.Items {
		result = append(result, ConfigMapInfo{Namespace: cm.Namespace, Name: cm.Name, Keys: len(cm.Data), Age: age(cm.CreationTimestamp)})
	}
	return result, nil
}

func ListSecrets(ctx context.Context, client kubernetes.Interface, namespace string) ([]SecretInfo, error) {
	list, err := client.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	result := make([]SecretInfo, 0, len(list.Items))
	for _, s := range list.Items {
		result = append(result, SecretInfo{Namespace: s.Namespace, Name: s.Name, Type: string(s.Type), Keys: len(s.Data), Age: age(s.CreationTimestamp)})
	}
	return result, nil
}

// podStatus mirrors "kubectl get pods": it surfaces the container waiting reason
// (CrashLoopBackOff, ImagePullBackOff...) instead of just the generic Phase.
func podStatus(pod corev1.Pod) (status string, restarts int32, ready string) {
	status = string(pod.Status.Phase)
	if pod.DeletionTimestamp != nil {
		status = "Terminating"
	}

	total := len(pod.Status.ContainerStatuses)
	readyCount := 0
	for _, cs := range pod.Status.ContainerStatuses {
		restarts += cs.RestartCount
		if cs.Ready {
			readyCount++
		}
		if cs.State.Waiting != nil && cs.State.Waiting.Reason != "" {
			status = cs.State.Waiting.Reason
		}
		if cs.State.Terminated != nil && cs.State.Terminated.Reason != "" && cs.State.Terminated.Reason != "Completed" {
			status = cs.State.Terminated.Reason
		}
	}
	ready = fmt.Sprintf("%d/%d", readyCount, total)
	return status, restarts, ready
}
