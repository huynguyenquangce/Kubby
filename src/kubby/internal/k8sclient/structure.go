package k8sclient

import (
	"context"
	"fmt"
	"sort"
	"sync"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const structureConcurrency = 6

var nodeMetadataGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "nodes"}

type StructureSummary struct {
	Nodes      int `json:"nodes"`
	Namespaces int `json:"namespaces"`
	Pods       int `json:"pods"`
	Unhealthy  int `json:"unhealthy"`
}

type StructurePod struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Status    string `json:"status"`
	Ready     string `json:"ready"`
	Restarts  int32  `json:"restarts"`
	Node      string `json:"node"`
	IsError   bool   `json:"isError"`
}

type StructureWorkload struct {
	Kind      string         `json:"kind"`
	Name      string         `json:"name"`
	Namespace string         `json:"namespace"`
	Status    string         `json:"status"`
	IsError   bool           `json:"isError"`
	Pods      []StructurePod `json:"pods"`
}

type StructureService struct {
	Name      string              `json:"name"`
	Namespace string              `json:"namespace"`
	Type      string              `json:"type"`
	Routes    []string            `json:"routes"`
	Warning   string              `json:"warning"`
	Workloads []StructureWorkload `json:"workloads"`
}

type StructureEntry struct {
	Kind      string             `json:"kind"`
	RefKind   string             `json:"refKind"`
	Name      string             `json:"name"`
	Namespace string             `json:"namespace"`
	Warning   string             `json:"warning"`
	Services  []StructureService `json:"services"`
}

// ClusterStructureData is the debug-oriented topology behind the Cluster
// structure screen. Unexposed contains workloads whose pods are not selected by
// any Service, which makes the view complete instead of silently hiding them.
type ClusterStructureData struct {
	Scope     string              `json:"scope"`
	Summary   StructureSummary    `json:"summary"`
	Entries   []StructureEntry    `json:"entries"`
	Internal  []StructureService  `json:"internal"`
	Unexposed []StructureWorkload `json:"unexposed"`
	Warnings  []string            `json:"warnings"`
}

type structureTask struct {
	name string
	run  func() error
}

// ClusterStructure gathers the entire screen behind one bound call. Core lists
// run concurrently and are joined in memory; no Service or workload triggers an
// N+1 API request.
func ClusterStructure(ctx context.Context, c *Cluster, namespace string) (*ClusterStructureData, error) {
	services := &corev1.ServiceList{}
	pods := &corev1.PodList{}
	ingresses := &netv1.IngressList{}
	endpointSlices := &discoveryv1.EndpointSliceList{}
	endpointSlicesAvailable := false
	replicaSets := &appsv1.ReplicaSetList{}
	var nodes, namespaces *metav1.PartialObjectMetadataList

	tasks := []structureTask{
		{name: "services", run: func() error {
			list, err := c.Clientset.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
			if err == nil {
				services = list
			}
			return err
		}},
		{name: "pods", run: func() error {
			list, err := c.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
			if err == nil {
				pods = list
			}
			return err
		}},
		{name: "ingresses", run: func() error {
			list, err := c.Clientset.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{})
			if err == nil {
				ingresses = list
			}
			return err
		}},
		{name: "endpoint slices", run: func() error {
			list, err := c.Clientset.DiscoveryV1().EndpointSlices(namespace).List(ctx, metav1.ListOptions{})
			if err == nil {
				endpointSlices = list
				endpointSlicesAvailable = true
			}
			return err
		}},
		{name: "replicasets", run: func() error {
			list, err := c.Clientset.AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{})
			if err == nil {
				replicaSets = list
			}
			return err
		}},
	}
	if c.Meta != nil {
		tasks = append(tasks,
			structureTask{name: "nodes", run: func() error {
				list, err := c.Meta.Resource(nodeMetadataGVR).List(ctx, metav1.ListOptions{})
				if err == nil {
					nodes = list
				}
				return err
			}},
			structureTask{name: "namespaces", run: func() error {
				list, err := c.Meta.Resource(namespaceMetadataGVR).List(ctx, metav1.ListOptions{})
				if err == nil {
					namespaces = list
				}
				return err
			}},
		)
	} else {
		tasks = append(tasks, structureTask{name: "metadata", run: func() error {
			return fmt.Errorf("metadata client is unavailable")
		}})
	}

	warnings := runStructureTasks(ctx, tasks)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	flows := networkTopologyFromLists(ctx, c, namespace, services, pods, ingresses, endpointSlices, endpointSlicesAvailable)
	out := structureFromSnapshot(namespace, flows, pods.Items, replicaSets.Items)
	out.Warnings = append(warnings, flows.Warnings...)
	sort.Strings(out.Warnings)
	if nodes != nil {
		out.Summary.Nodes = len(nodes.Items)
	}
	if namespaces != nil {
		out.Summary.Namespaces = len(namespaces.Items)
	}
	return out, nil
}

func runStructureTasks(ctx context.Context, tasks []structureTask) []string {
	semaphore := make(chan struct{}, structureConcurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	warnings := []string{}
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

func structureFromSnapshot(namespace string, flows *NetworkFlows, pods []corev1.Pod, replicaSets []appsv1.ReplicaSet) *ClusterStructureData {
	out := &ClusterStructureData{
		Scope: namespace, Entries: []StructureEntry{}, Internal: []StructureService{},
		Unexposed: []StructureWorkload{}, Warnings: []string{},
	}
	rsOwners := make(map[string]metav1.OwnerReference, len(replicaSets))
	for i := range replicaSets {
		rs := &replicaSets[i]
		if rs.DeletionTimestamp != nil {
			continue
		}
		for _, owner := range rs.OwnerReferences {
			if owner.Kind == "Deployment" {
				rsOwners[rs.Namespace+"/"+rs.Name] = owner
				break
			}
		}
	}
	servedPods := map[string]bool{}
	convertService := func(service FlowService) StructureService {
		for _, pod := range service.Pods {
			servedPods[pod.Namespace+"/"+pod.Name] = true
		}
		return structureService(service, rsOwners)
	}
	for _, entry := range flows.Ingresses {
		converted := StructureEntry{
			Kind: entry.Kind, RefKind: entry.RefKind, Name: entry.Name,
			Namespace: entry.Namespace, Warning: entry.Warning, Services: []StructureService{},
		}
		for _, service := range entry.Services {
			converted.Services = append(converted.Services, convertService(service))
		}
		out.Entries = append(out.Entries, converted)
	}
	for _, service := range flows.Services {
		out.Internal = append(out.Internal, convertService(service))
	}

	unexposedPods := []FlowPod{}
	for i := range pods {
		pod := &pods[i]
		if pod.DeletionTimestamp != nil {
			continue
		}
		status, restarts, ready := podStatus(*pod)
		ownerKind, ownerName := podOwner(pod)
		flowPod := FlowPod{
			Name: pod.Name, Namespace: pod.Namespace, Status: status, Ready: ready,
			Restarts: restarts, Node: pod.Spec.NodeName, IP: pod.Status.PodIP,
			OwnerKind: ownerKind, OwnerName: ownerName,
			IsError: isErroredPodStatus(status),
			IsReady: (status == string(corev1.PodRunning) && isFullyReady(ready)) || status == string(corev1.PodSucceeded),
		}
		out.Summary.Pods++
		if flowPod.IsError || !flowPod.IsReady {
			out.Summary.Unhealthy++
		}
		if !servedPods[pod.Namespace+"/"+pod.Name] {
			unexposedPods = append(unexposedPods, flowPod)
		}
	}
	out.Unexposed = structureWorkloads(unexposedPods, rsOwners)
	return out
}

func structureService(service FlowService, rsOwners map[string]metav1.OwnerReference) StructureService {
	return StructureService{
		Name: service.Name, Namespace: service.Namespace, Type: service.Type,
		Routes: service.Routes, Warning: service.Warning,
		Workloads: structureWorkloads(service.Pods, rsOwners),
	}
}

func structureWorkloads(pods []FlowPod, rsOwners map[string]metav1.OwnerReference) []StructureWorkload {
	byOwner := map[string]*StructureWorkload{}
	order := []string{}
	for _, pod := range pods {
		kind, name := pod.OwnerKind, pod.OwnerName
		if kind == "ReplicaSet" {
			if owner, ok := rsOwners[pod.Namespace+"/"+name]; ok {
				kind, name = owner.Kind, owner.Name
			}
		}
		if kind == "" || name == "" {
			kind, name = "Pod", pod.Name
		}
		key := pod.Namespace + "/" + kind + "/" + name
		workload := byOwner[key]
		if workload == nil {
			workload = &StructureWorkload{Kind: kind, Name: name, Namespace: pod.Namespace, Pods: []StructurePod{}}
			byOwner[key] = workload
			order = append(order, key)
		}
		isError := pod.IsError || !pod.IsReady
		workload.IsError = workload.IsError || isError
		workload.Pods = append(workload.Pods, StructurePod{
			Name: pod.Name, Namespace: pod.Namespace, Status: pod.Status, Ready: pod.Ready,
			Restarts: pod.Restarts, Node: pod.Node, IsError: isError,
		})
	}
	sort.Strings(order)
	out := make([]StructureWorkload, 0, len(order))
	for _, key := range order {
		workload := byOwner[key]
		sort.Slice(workload.Pods, func(i, j int) bool { return workload.Pods[i].Name < workload.Pods[j].Name })
		ready := 0
		completed := 0
		for _, pod := range workload.Pods {
			if !pod.IsError {
				ready++
			}
			if pod.Status == string(corev1.PodSucceeded) {
				completed++
			}
		}
		if completed == len(workload.Pods) && completed > 0 {
			workload.Status = "completed"
		} else {
			workload.Status = fmt.Sprintf("%d/%d ready", ready, len(workload.Pods))
		}
		out = append(out, *workload)
	}
	return out
}
