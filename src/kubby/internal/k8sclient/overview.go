package k8sclient

import (
	"context"
	"fmt"
	"sort"
	"sync"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

const (
	overviewConcurrency  = 6
	overviewTopPods      = 8
	overviewRecentEvents = 15
)

var (
	namespaceMetadataGVR  = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}
	deploymentMetadataGVR = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
)

type OverviewStats struct {
	Nodes         int  `json:"nodes"`
	Namespaces    int  `json:"namespaces"`
	Pods          int  `json:"pods"`
	Deployments   int  `json:"deployments"`
	Errors        int  `json:"errors"`
	PodsAvailable bool `json:"podsAvailable"`
}

// OverviewData is the complete payload for the Overview screen. Warnings are
// section-local failures: one forbidden or unavailable API must not blank the
// remaining dashboard.
type OverviewData struct {
	Stats       OverviewStats        `json:"stats"`
	FailingPods []PodInfo            `json:"failingPods"`
	NodeMetrics []NodeMetric         `json:"nodeMetrics"`
	NodeStatus  []OverviewNodeStatus `json:"nodeStatus"`
	TopPods     []PodMetric          `json:"topPods"`
	Events      []EventInfo          `json:"events"`
	Warnings    []string             `json:"warnings"`
}

// OverviewNodeStatus keeps the infrastructure section operational: readiness,
// scheduling and pressure are more useful for debugging than repeating the CPU
// and memory utilization already shown in Cluster pulse.
type OverviewNodeStatus struct {
	Name        string   `json:"name"`
	Ready       bool     `json:"ready"`
	Schedulable bool     `json:"schedulable"`
	Pods        int      `json:"pods"`
	Pressure    []string `json:"pressure"`
	Version     string   `json:"version"`
}

type overviewTask struct {
	name string
	run  func() error
}

// OverviewSnapshot gathers the whole dashboard behind one bound call. API
// calls overlap under a fixed semaphore, full Nodes/Pods are each listed once,
// and count-only resources use the metadata client.
func OverviewSnapshot(ctx context.Context, c *Cluster) (*OverviewData, error) {
	var (
		nodes       *corev1.NodeList
		pods        *corev1.PodList
		events      *corev1.EventList
		namespaces  *metav1.PartialObjectMetadataList
		deployments *metav1.PartialObjectMetadataList
		nodeUsage   *metricsv1beta1.NodeMetricsList
		podUsage    *metricsv1beta1.PodMetricsList
	)

	tasks := []overviewTask{
		{name: "nodes", run: func() error {
			list, err := c.Clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
			if err == nil {
				nodes = list
			}
			return err
		}},
		{name: "pods", run: func() error {
			list, err := c.Clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
			if err == nil {
				pods = list
			}
			return err
		}},
		{name: "events", run: func() error {
			list, err := c.Clientset.CoreV1().Events("").List(ctx, metav1.ListOptions{})
			if err == nil {
				events = list
			}
			return err
		}},
	}
	if c.Meta != nil {
		tasks = append(tasks,
			overviewTask{name: "namespaces", run: func() error {
				list, err := c.Meta.Resource(namespaceMetadataGVR).List(ctx, metav1.ListOptions{})
				if err == nil {
					namespaces = list
				}
				return err
			}},
			overviewTask{name: "deployments", run: func() error {
				list, err := c.Meta.Resource(deploymentMetadataGVR).Namespace("").List(ctx, metav1.ListOptions{})
				if err == nil {
					deployments = list
				}
				return err
			}},
		)
	} else {
		tasks = append(tasks, overviewTask{name: "metadata", run: func() error {
			return fmt.Errorf("metadata client is unavailable")
		}})
	}
	if c.Metrics != nil {
		tasks = append(tasks,
			overviewTask{name: "node metrics", run: func() error {
				list, err := c.Metrics.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{})
				if err == nil {
					nodeUsage = list
				}
				return err
			}},
			overviewTask{name: "pod metrics", run: func() error {
				list, err := c.Metrics.MetricsV1beta1().PodMetricses("").List(ctx, metav1.ListOptions{})
				if err == nil {
					podUsage = list
				}
				return err
			}},
		)
	}

	warnings := runOverviewTasks(ctx, tasks)
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	result := &OverviewData{Warnings: warnings}
	if nodes != nil {
		result.Stats.Nodes = len(nodes.Items)
	}
	if namespaces != nil {
		result.Stats.Namespaces = len(namespaces.Items)
	}
	if deployments != nil {
		result.Stats.Deployments = len(deployments.Items)
	}
	if pods != nil {
		result.Stats.PodsAvailable = true
		result.Stats.Pods = len(pods.Items)
		result.FailingPods = failingPodInfos(pods.Items)
		result.Stats.Errors = len(result.FailingPods)
	}
	if nodes != nil && nodeUsage != nil {
		result.NodeMetrics = nodeMetricsFrom(nodes.Items, nodeUsage.Items)
	}
	if nodes != nil {
		result.NodeStatus = overviewNodeStatuses(nodes.Items, pods)
	}
	if podUsage != nil {
		result.TopPods = topLivePodMetrics(podUsage.Items, pods, overviewTopPods)
	}
	if events != nil {
		result.Events = recentEventInfos(events.Items, overviewRecentEvents)
	}
	return result, nil
}

func overviewNodeStatuses(nodes []corev1.Node, pods *corev1.PodList) []OverviewNodeStatus {
	podCounts := map[string]int{}
	if pods != nil {
		for i := range pods.Items {
			pod := &pods.Items[i]
			if pod.DeletionTimestamp == nil && pod.Spec.NodeName != "" {
				podCounts[pod.Spec.NodeName]++
			}
		}
	}
	out := make([]OverviewNodeStatus, 0, len(nodes))
	for i := range nodes {
		node := &nodes[i]
		status := OverviewNodeStatus{
			Name: node.Name, Schedulable: !node.Spec.Unschedulable,
			Pods: podCounts[node.Name], Pressure: []string{}, Version: node.Status.NodeInfo.KubeletVersion,
		}
		for _, condition := range node.Status.Conditions {
			switch condition.Type {
			case corev1.NodeReady:
				status.Ready = condition.Status == corev1.ConditionTrue
			case corev1.NodeMemoryPressure, corev1.NodeDiskPressure, corev1.NodePIDPressure, corev1.NodeNetworkUnavailable:
				if condition.Status == corev1.ConditionTrue {
					status.Pressure = append(status.Pressure, string(condition.Type))
				}
			}
		}
		out = append(out, status)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func runOverviewTasks(ctx context.Context, tasks []overviewTask) []string {
	semaphore := make(chan struct{}, overviewConcurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	warnings := make([]string, 0)
	for _, task := range tasks {
		task := task
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				return
			}
			if err := task.run(); err != nil {
				mu.Lock()
				warnings = append(warnings, fmt.Sprintf("%s: %v", task.name, err))
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	sort.Strings(warnings)
	return warnings
}

func failingPodInfos(items []corev1.Pod) []PodInfo {
	out := make([]PodInfo, 0)
	for _, pod := range items {
		if pod.DeletionTimestamp != nil {
			continue
		}
		status, restarts, ready := podStatus(pod)
		if !erroredStatuses[status] {
			continue
		}
		out = append(out, PodInfo{
			Namespace: pod.Namespace,
			Name:      pod.Name,
			Status:    status,
			Ready:     ready,
			Restarts:  restarts,
			IsError:   true,
			PodIP:     pod.Status.PodIP,
			Node:      pod.Spec.NodeName,
			Age:       age(pod.CreationTimestamp),
		})
	}
	return out
}

func topLivePodMetrics(items []metricsv1beta1.PodMetrics, pods *corev1.PodList, limit int) []PodMetric {
	metrics := podMetricsFrom(items)
	if pods != nil {
		live := make(map[string]bool, len(pods.Items))
		for _, pod := range pods.Items {
			if pod.DeletionTimestamp == nil {
				live[pod.Namespace+"\x00"+pod.Name] = true
			}
		}
		filtered := metrics[:0]
		for _, metric := range metrics {
			if live[metric.Namespace+"\x00"+metric.Name] {
				filtered = append(filtered, metric)
			}
		}
		metrics = filtered
	}
	sort.Slice(metrics, func(i, j int) bool { return metrics[i].CPUMilli > metrics[j].CPUMilli })
	if limit > 0 && len(metrics) > limit {
		metrics = metrics[:limit]
	}
	return metrics
}
