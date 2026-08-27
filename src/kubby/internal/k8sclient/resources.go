package k8sclient

import (
	"context"
	"fmt"
	"strings"

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

// ResourcePageMeta carries the opaque Kubernetes continuation token for one
// bounded list response. Remaining is -1 when the API server did not report an
// exact remainingItemCount.
type ResourcePageMeta struct {
	Continue  string `json:"continue"`
	Remaining int64  `json:"remaining"`
}

const (
	defaultResourcePageLimit int64 = 200
	maxResourcePageLimit     int64 = 500
)

func resourcePageOptions(continueToken string, limit int64) metav1.ListOptions {
	if limit <= 0 {
		limit = defaultResourcePageLimit
	}
	if limit > maxResourcePageLimit {
		limit = maxResourcePageLimit
	}
	return metav1.ListOptions{Continue: continueToken, Limit: limit}
}

func resourcePageMeta(continueToken string, remaining *int64) ResourcePageMeta {
	count := int64(-1)
	if remaining != nil {
		count = *remaining
	}
	return ResourcePageMeta{Continue: continueToken, Remaining: count}
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

// erroredStatuses are Pod statuses treated as active issues so the frontend can
// flag them consistently in lists, counts, search, Overview and topology.
var erroredStatuses = map[string]bool{
	"CrashLoopBackOff":           true,
	"ImagePullBackOff":           true,
	"ErrImagePull":               true,
	"CreateContainerConfigError": true,
	"CreateContainerError":       true,
	"RunContainerError":          true,
	"ContainerCannotRun":         true,
	"InvalidImageName":           true,
	"OOMKilled":                  true,
	"Unschedulable":              true,
	"NotReady":                   true,
	"Pending":                    true,
	"Unknown":                    true,
	"Failed":                     true,
	"Error":                      true,
	"Evicted":                    true,
}

// podStatusPrecedence makes the displayed status independent of container
// ordering. A progress state from one container must never hide a real failure
// in another container just because it appeared later in status.containerStatuses.
var podStatusPrecedence = map[string]int{
	"CrashLoopBackOff":           120,
	"OOMKilled":                  115,
	"ImagePullBackOff":           110,
	"ErrImagePull":               110,
	"InvalidImageName":           110,
	"CreateContainerConfigError": 105,
	"CreateContainerError":       105,
	"RunContainerError":          105,
	"ContainerCannotRun":         105,
	"Error":                      100,
	"Failed":                     95,
	"Evicted":                    95,
	"Unschedulable":              90,
	"Unknown":                    80,
	"NotReady":                   60,
	"ContainerCreating":          40,
	"PodInitializing":            40,
	"Pending":                    30,
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

	return podInfos(list.Items), nil
}

func podInfos(items []corev1.Pod) []PodInfo {
	result := make([]PodInfo, 0, len(items))
	for _, pod := range items {
		status, restarts, ready := podStatus(pod)
		result = append(result, PodInfo{
			Namespace: pod.Namespace,
			Name:      pod.Name,
			Status:    status,
			Ready:     ready,
			Restarts:  restarts,
			IsError:   isErroredPodStatus(status),
			PodIP:     pod.Status.PodIP,
			Node:      pod.Spec.NodeName,
			Age:       age(pod.CreationTimestamp),
		})
	}
	return result
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

	total := len(pod.Status.ContainerStatuses)
	readyCount := 0
	for _, cs := range pod.Status.InitContainerStatuses {
		restarts += cs.RestartCount
	}
	for _, cs := range pod.Status.ContainerStatuses {
		restarts += cs.RestartCount
		if cs.Ready {
			readyCount++
		}
	}
	ready = fmt.Sprintf("%d/%d", readyCount, total)

	// Deletion is the highest-precedence state. A terminating pod must never be
	// painted back to Running/CrashLoopBackOff by a container state below.
	if pod.DeletionTimestamp != nil {
		return "Terminating", restarts, ready
	}

	priority := podStatusPrecedence[status]
	choose := func(candidate string, init bool) {
		if candidate == "" || candidate == "Completed" {
			return
		}
		candidatePriority := podStatusPrecedence[candidate]
		if init && candidatePriority >= 90 {
			candidatePriority++ // a failing init container blocks every app container
		}
		display := candidate
		if init {
			display = "Init:" + candidate
		}
		if candidatePriority > priority || (candidatePriority == priority && display < status) {
			status = display
			priority = candidatePriority
		}
	}

	// An explicit scheduling denial is more useful than the generic Pending
	// phase, even when container statuses have not been populated yet.
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodScheduled && condition.Status == corev1.ConditionFalse {
			if condition.Reason == "Unschedulable" {
				choose("Unschedulable", false)
			} else {
				choose("Pending", false)
			}
		}
		if condition.Type == corev1.PodReady && condition.Status != corev1.ConditionTrue && pod.Status.Phase == corev1.PodRunning {
			choose("NotReady", false)
		}
	}

	for _, cs := range pod.Status.InitContainerStatuses {
		if cs.State.Waiting != nil && cs.State.Waiting.Reason != "" {
			choose(cs.State.Waiting.Reason, true)
		}
		if cs.State.Terminated != nil && cs.State.Terminated.ExitCode != 0 {
			reason := cs.State.Terminated.Reason
			if reason == "" || !erroredStatuses[reason] {
				reason = "Error"
			}
			choose(reason, true)
		}
	}

	for _, cs := range pod.Status.ContainerStatuses {
		if cs.State.Waiting != nil && cs.State.Waiting.Reason != "" {
			choose(cs.State.Waiting.Reason, false)
		}
		if cs.State.Terminated != nil && cs.State.Terminated.ExitCode != 0 {
			reason := cs.State.Terminated.Reason
			if reason == "" || !erroredStatuses[reason] {
				reason = "Error"
			}
			choose(reason, false)
		}
	}
	if pod.Status.Phase == corev1.PodRunning && (total == 0 || readyCount < total) {
		choose("NotReady", false)
	}
	return status, restarts, ready
}

func isErroredPodStatus(status string) bool {
	if erroredStatuses[status] {
		return true
	}
	if strings.HasPrefix(status, "Init:") {
		return erroredStatuses[strings.TrimPrefix(status, "Init:")]
	}
	return false
}

func podIsReady(pod *corev1.Pod) bool {
	if pod == nil || pod.DeletionTimestamp != nil || pod.Status.Phase != corev1.PodRunning {
		return false
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}
