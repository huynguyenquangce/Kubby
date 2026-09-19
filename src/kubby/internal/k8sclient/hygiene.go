package k8sclient

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Cluster hygiene: what is still here that nothing needs any more, and what is
// here in a form that will hurt later. A cluster accumulates these quietly —
// a ConfigMap from a chart that was uninstalled, a PVC whose Pod was deleted, a
// Service whose selector stopped matching after a label rename — and none of it
// raises an alert, because none of it is an error.
//
// Two rules make this safe to act on:
//
//	1. Kubby never deletes anything here. Every item carries the kubectl command
//	   and the operator runs it, after reading why the object was listed.
//	2. Every group states what Kubby could not see. A CRD controller that
//	   consumes a ConfigMap by name is invisible to any reference scan, and
//	   pretending otherwise would make this feature dangerous rather than useful.

const (
	// hygieneCacheTTL keeps the five-second live refresh from repeating a scan
	// that lists a dozen resource types.
	hygieneCacheTTL = 30 * time.Second
	// hygieneFinishedAge is how long a finished Pod or Job may sit before it
	// counts as left behind rather than recent.
	hygieneFinishedAge = 24 * time.Hour
	// hygieneReplicaSetAge keeps rollback history out of the report: a
	// zero-replica ReplicaSet is how a rollback works, until it is very old.
	hygieneReplicaSetAge = 7 * 24 * time.Hour
	// hygieneGroupLimit bounds one group; the count always covers everything.
	hygieneGroupLimit = 200
)

// Categories, stable because the UI and the CLI both key on them.
const (
	HygieneUnusedConfigMap = "unused-configmap"
	HygieneUnusedSecret    = "unused-secret"
	HygieneUnusedClaim     = "unused-pvc"
	HygieneDeadService     = "service-without-endpoints"
	HygieneOldReplicaSet   = "old-replicaset"
	HygieneFinishedJob     = "finished-job"
	HygieneFinishedPod     = "finished-pod"
	HygieneUnpinnedImage   = "unpinned-image"
)

// HygieneItem is one object worth a second look.
type HygieneItem struct {
	Category  string   `json:"category"`
	Kind      string   `json:"kind"`
	Namespace string   `json:"namespace"`
	Name      string   `json:"name"`
	Age       string   `json:"age"`
	Severity  string   `json:"severity"`
	Title     string   `json:"title"`
	Detail    string   `json:"detail"`
	Chips     []string `json:"chips"`
	// Command removes the object. It is shown for the operator to copy and is
	// never run by Kubby.
	Command string `json:"command"`
}

// HygieneGroup is one category with its own explanation and caveat.
type HygieneGroup struct {
	Category string        `json:"category"`
	Title    string        `json:"title"`
	Summary  string        `json:"summary"`
	Caveat   string        `json:"caveat"`
	Count    int           `json:"count"`
	Items    []HygieneItem `json:"items"`
	Warning  string        `json:"warning"` // this group could not be computed
}

// HygieneReport is the whole scan for one scope.
type HygieneReport struct {
	Scope     string         `json:"scope"` // "" = all namespaces
	CheckedAt string         `json:"checkedAt"`
	Groups    []HygieneGroup `json:"groups"`
	Total     int            `json:"total"`
	Warnings  []string       `json:"warnings"`
}

type hygieneCacheEntry struct {
	report  *HygieneReport
	expires time.Time
}

var (
	configMapMetadataGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
	secretMetadataGVR    = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}
)

// hygieneInputs are the lists the scan shares, read concurrently once.
type hygieneInputs struct {
	pods            []corev1.Pod
	configMaps      []metav1.PartialObjectMetadata
	secrets         []metav1.PartialObjectMetadata
	claims          []corev1.PersistentVolumeClaim
	services        []corev1.Service
	slices          []discoveryv1.EndpointSlice
	replicaSets     []appsv1.ReplicaSet
	deployments     []appsv1.Deployment
	statefulSets    []appsv1.StatefulSet
	daemonSets      []appsv1.DaemonSet
	jobs            []batchv1.Job
	cronJobs        []batchv1.CronJob
	serviceAccounts []corev1.ServiceAccount
	ingresses       []netv1.Ingress

	errs map[string]error
	mu   sync.Mutex
}

func (in *hygieneInputs) fail(what string, err error) {
	in.mu.Lock()
	defer in.mu.Unlock()
	in.errs[what] = err
}

func (in *hygieneInputs) failed(what string) error {
	return in.errs[what]
}

// ClusterHygiene scans one namespace ("" = all) for objects nothing needs.
// Results are cached per connection and scope for a short time, and concurrent
// callers share one scan rather than each starting another.
func ClusterHygiene(ctx context.Context, c *Cluster, namespace string) (*HygieneReport, error) {
	if c == nil || c.Clientset == nil {
		return nil, fmt.Errorf("this connection has no Kubernetes client")
	}
	c.hygieneMu.Lock()
	defer c.hygieneMu.Unlock()
	if entry, ok := c.hygieneCache[namespace]; ok && checksNow().Before(entry.expires) {
		return entry.report, nil
	}
	in := gatherHygieneInputs(ctx, c, namespace)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	report := buildHygieneReport(namespace, in)
	if c.hygieneCache == nil {
		c.hygieneCache = map[string]hygieneCacheEntry{}
	}
	c.hygieneCache[namespace] = hygieneCacheEntry{report: report, expires: checksNow().Add(hygieneCacheTTL)}
	return report, nil
}

func gatherHygieneInputs(ctx context.Context, c *Cluster, namespace string) *hygieneInputs {
	in := &hygieneInputs{errs: map[string]error{}}
	var wg sync.WaitGroup
	sem := make(chan struct{}, countConcurrency)
	run := func(what string, task func() error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if err := task(); err != nil {
				in.fail(what, err)
			}
		}()
	}
	client := c.Clientset
	run("pods", func() error {
		list, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
		if err == nil {
			in.pods = list.Items
		}
		return err
	})
	// ConfigMaps and Secrets are read metadata-only: the scan needs names,
	// ownership and labels, and a Secret's data has no business crossing this
	// boundary to be counted.
	run("configmaps", func() error {
		list, err := metadataList(ctx, c, configMapMetadataGVR, namespace)
		if err == nil {
			in.configMaps = list
		}
		return err
	})
	run("secrets", func() error {
		list, err := metadataList(ctx, c, secretMetadataGVR, namespace)
		if err == nil {
			in.secrets = list
		}
		return err
	})
	run("persistentvolumeclaims", func() error {
		list, err := client.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
		if err == nil {
			in.claims = list.Items
		}
		return err
	})
	run("services", func() error {
		list, err := client.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
		if err == nil {
			in.services = list.Items
		}
		return err
	})
	run("endpointslices", func() error {
		list, err := client.DiscoveryV1().EndpointSlices(namespace).List(ctx, metav1.ListOptions{})
		if err == nil {
			in.slices = list.Items
		}
		return err
	})
	run("replicasets", func() error {
		list, err := client.AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{})
		if err == nil {
			in.replicaSets = list.Items
		}
		return err
	})
	run("deployments", func() error {
		list, err := client.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
		if err == nil {
			in.deployments = list.Items
		}
		return err
	})
	run("statefulsets", func() error {
		list, err := client.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{})
		if err == nil {
			in.statefulSets = list.Items
		}
		return err
	})
	run("daemonsets", func() error {
		list, err := client.AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{})
		if err == nil {
			in.daemonSets = list.Items
		}
		return err
	})
	run("jobs", func() error {
		list, err := client.BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{})
		if err == nil {
			in.jobs = list.Items
		}
		return err
	})
	run("cronjobs", func() error {
		list, err := client.BatchV1().CronJobs(namespace).List(ctx, metav1.ListOptions{})
		if err == nil {
			in.cronJobs = list.Items
		}
		return err
	})
	run("serviceaccounts", func() error {
		list, err := client.CoreV1().ServiceAccounts(namespace).List(ctx, metav1.ListOptions{})
		if err == nil {
			in.serviceAccounts = list.Items
		}
		return err
	})
	run("ingresses", func() error {
		list, err := client.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{})
		if err == nil {
			in.ingresses = list.Items
		}
		return err
	})
	wg.Wait()
	return in
}

// metadataList prefers the metadata client and falls back to nothing rather
// than to a typed list: a typed Secret list would transfer every value.
func metadataList(ctx context.Context, c *Cluster, gvr schema.GroupVersionResource, namespace string) ([]metav1.PartialObjectMetadata, error) {
	if c.Meta == nil {
		return nil, fmt.Errorf("the metadata client is unavailable")
	}
	list, err := c.Meta.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func buildHygieneReport(namespace string, in *hygieneInputs) *HygieneReport {
	report := &HygieneReport{Scope: namespace, CheckedAt: checksNow().UTC().Format(time.RFC3339),
		Groups: []HygieneGroup{}, Warnings: []string{}}
	index := buildReferenceIndex(in)
	report.Groups = append(report.Groups,
		unusedConfigMapGroup(in, index),
		unusedSecretGroup(in, index),
		unusedClaimGroup(in, index),
		deadServiceGroup(in),
		oldReplicaSetGroup(in),
		finishedJobGroup(in),
		finishedPodGroup(in),
		unpinnedImageGroup(index),
	)
	for i := range report.Groups {
		group := &report.Groups[i]
		sortHygieneItems(group.Items)
		group.Count = len(group.Items)
		if group.Count > hygieneGroupLimit {
			group.Items = group.Items[:hygieneGroupLimit]
			report.Warnings = append(report.Warnings,
				fmt.Sprintf("%s: %d found, the first %d are listed.", group.Title, group.Count, hygieneGroupLimit))
		}
		report.Total += group.Count
		if group.Warning != "" {
			report.Warnings = append(report.Warnings, group.Warning)
		}
	}
	return report
}

func sortHygieneItems(items []HygieneItem) {
	sort.SliceStable(items, func(i, j int) bool {
		if checkSeverityRank(items[i].Severity) != checkSeverityRank(items[j].Severity) {
			return checkSeverityRank(items[i].Severity) > checkSeverityRank(items[j].Severity)
		}
		if items[i].Namespace != items[j].Namespace {
			return items[i].Namespace < items[j].Namespace
		}
		return items[i].Name < items[j].Name
	})
}

func hygieneAge(created metav1.Time) string {
	if created.IsZero() {
		return ""
	}
	return durationText(checksNow().Sub(created.Time))
}

func olderThan(created metav1.Time, d time.Duration) bool {
	return !created.IsZero() && checksNow().Sub(created.Time) >= d
}

// skipManagedObject is true for objects another object owns or Kubernetes
// maintains: deleting them is the owner's business, not a cleanup decision.
func skipManagedObject(object *metav1.PartialObjectMetadata) bool {
	if object.DeletionTimestamp != nil || len(object.OwnerReferences) > 0 {
		return true
	}
	return false
}

func unusedConfigMapGroup(in *hygieneInputs, index *referenceIndex) HygieneGroup {
	group := HygieneGroup{Category: HygieneUnusedConfigMap, Title: "Unreferenced ConfigMaps",
		Summary: "No Pod, controller template or ReplicaSet in this scope mounts them or reads an environment variable from them.",
		Caveat:  "A controller that reads a ConfigMap by name — an operator, a CRD, a Helm hook — leaves no reference in any Pod spec, so check what created it before deleting.",
		Items:   []HygieneItem{}}
	if err := in.failed("configmaps"); err != nil {
		group.Warning = fmt.Sprintf("ConfigMaps could not be listed: %v", err)
		return group
	}
	for i := range in.configMaps {
		item := &in.configMaps[i]
		if skipManagedObject(item) || item.Name == "kube-root-ca.crt" {
			continue // kube-root-ca.crt is maintained by the control plane
		}
		if index.configMaps[item.Namespace+"/"+item.Name] {
			continue
		}
		group.Items = append(group.Items, HygieneItem{
			Category: HygieneUnusedConfigMap, Kind: "ConfigMap", Namespace: item.Namespace, Name: item.Name,
			Age: hygieneAge(item.CreationTimestamp), Severity: SeverityInfo,
			Title:   "Nothing in this scope references it",
			Detail:  "No Pod, controller template or ReplicaSet mounts it or reads an environment variable from it.",
			Command: fmt.Sprintf("kubectl delete configmap %s -n %s", item.Name, item.Namespace),
		})
	}
	return group
}

func unusedSecretGroup(in *hygieneInputs, index *referenceIndex) HygieneGroup {
	group := HygieneGroup{Category: HygieneUnusedSecret, Title: "Unreferenced Secrets",
		Summary: "No Pod, controller template, ServiceAccount or Ingress in this scope uses them. Only names and labels were read — no Secret value is ever read by this scan.",
		Caveat:  "Service-account tokens, Helm release storage and Secrets owned by another object are excluded. A controller that reads a Secret by name still leaves no reference, so check what created it.",
		Items:   []HygieneItem{}}
	if err := in.failed("secrets"); err != nil {
		group.Warning = fmt.Sprintf("Secrets could not be listed: %v", err)
		return group
	}
	for i := range in.secrets {
		item := &in.secrets[i]
		if skipManagedObject(item) {
			continue
		}
		if item.Annotations["kubernetes.io/service-account.name"] != "" {
			continue // a service-account token, managed by Kubernetes
		}
		if item.Labels["owner"] == "helm" {
			continue // Helm release storage; the Helm view owns those
		}
		if index.secrets[item.Namespace+"/"+item.Name] {
			continue
		}
		group.Items = append(group.Items, HygieneItem{
			Category: HygieneUnusedSecret, Kind: "Secret", Namespace: item.Namespace, Name: item.Name,
			Age: hygieneAge(item.CreationTimestamp), Severity: SeverityInfo,
			Title:   "Nothing in this scope references it",
			Detail:  "No Pod, controller template, ServiceAccount or Ingress names it.",
			Command: fmt.Sprintf("kubectl delete secret %s -n %s", item.Name, item.Namespace),
		})
	}
	return group
}

func unusedClaimGroup(in *hygieneInputs, index *referenceIndex) HygieneGroup {
	group := HygieneGroup{Category: HygieneUnusedClaim, Title: "Claims no Pod mounts",
		Summary: "A PersistentVolumeClaim keeps its volume — and its bill — whether or not anything mounts it.",
		Caveat:  "Claims a StatefulSet created from a volumeClaimTemplate are excluded: they outlive their Pods by design. Deleting a claim can destroy data, depending on the StorageClass reclaim policy.",
		Items:   []HygieneItem{}}
	if err := in.failed("persistentvolumeclaims"); err != nil {
		group.Warning = fmt.Sprintf("PersistentVolumeClaims could not be listed: %v", err)
		return group
	}
	if err := in.failed("pods"); err != nil {
		group.Warning = fmt.Sprintf("Pods could not be listed, so claim usage is unknown: %v", err)
		return group
	}
	for i := range in.claims {
		claim := &in.claims[i]
		if claim.DeletionTimestamp != nil {
			continue
		}
		if index.claims[claim.Namespace+"/"+claim.Name] {
			continue
		}
		if _, kept := claimKeptByStatefulSet(claim.Name, in.statefulSets); kept {
			continue
		}
		chips := []string{string(claim.Status.Phase)}
		if capacity, ok := claim.Status.Capacity[corev1.ResourceStorage]; ok {
			chips = append(chips, capacity.String())
		}
		if claim.Spec.StorageClassName != nil && *claim.Spec.StorageClassName != "" {
			chips = append(chips, *claim.Spec.StorageClassName)
		}
		group.Items = append(group.Items, HygieneItem{
			Category: HygieneUnusedClaim, Kind: "PersistentVolumeClaim", Namespace: claim.Namespace, Name: claim.Name,
			Age: hygieneAge(claim.CreationTimestamp), Severity: SeverityWarning, Chips: chips,
			Title:   "No Pod mounts it",
			Detail:  "The volume stays allocated while nothing uses it. Check the reclaim policy before deleting: with Delete, the data goes with the claim.",
			Command: fmt.Sprintf("kubectl delete pvc %s -n %s", claim.Name, claim.Namespace),
		})
	}
	return group
}

func deadServiceGroup(in *hygieneInputs) HygieneGroup {
	group := HygieneGroup{Category: HygieneDeadService, Title: "Services with no ready endpoint",
		Summary: "The name resolves and connections are accepted, then nothing answers — usually a selector that stopped matching after a label change.",
		Caveat:  "ExternalName Services are excluded. A Service whose Pods are briefly all unready appears here too, so read the endpoint count rather than acting on the list alone.",
		Items:   []HygieneItem{}}
	if err := in.failed("services"); err != nil {
		group.Warning = fmt.Sprintf("Services could not be listed: %v", err)
		return group
	}
	if err := in.failed("endpointslices"); err != nil {
		group.Warning = fmt.Sprintf("EndpointSlices could not be listed, so endpoints were not checked: %v", err)
		return group
	}
	ready := map[string]int{}
	for i := range in.slices {
		slice := &in.slices[i]
		service := slice.Labels[discoveryv1.LabelServiceName]
		if service == "" {
			continue
		}
		key := slice.Namespace + "/" + service
		for _, endpoint := range slice.Endpoints {
			if endpoint.Conditions.Ready == nil || *endpoint.Conditions.Ready {
				ready[key] += len(endpoint.Addresses)
			}
		}
	}
	for i := range in.services {
		service := &in.services[i]
		if service.DeletionTimestamp != nil || service.Spec.Type == corev1.ServiceTypeExternalName {
			continue
		}
		if ready[service.Namespace+"/"+service.Name] > 0 {
			continue
		}
		item := HygieneItem{
			Category: HygieneDeadService, Kind: "Service", Namespace: service.Namespace, Name: service.Name,
			Age: hygieneAge(service.CreationTimestamp), Severity: SeverityWarning,
			Chips: []string{string(service.Spec.Type)},
		}
		if len(service.Spec.Selector) == 0 {
			item.Severity = SeverityInfo
			item.Title = "No selector and no endpoints"
			item.Detail = "This Service selects no Pods, and nothing maintains its endpoints by hand, so it routes nowhere."
		} else {
			item.Title = "The selector matches no ready Pod"
			item.Detail = fmt.Sprintf("selector %s matches no Pod that is ready; traffic to this Service is refused.", selectorMapText(service.Spec.Selector))
		}
		group.Items = append(group.Items, item)
	}
	return group
}

func selectorMapText(selector map[string]string) string {
	parts := make([]string, 0, len(selector))
	for key, value := range selector {
		parts = append(parts, key+"="+value)
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func oldReplicaSetGroup(in *hygieneInputs) HygieneGroup {
	group := HygieneGroup{Category: HygieneOldReplicaSet, Title: "Old empty ReplicaSets",
		Summary: fmt.Sprintf("Zero-replica ReplicaSets older than %d days. A Deployment keeps them so a rollback has something to roll back to.", int(hygieneReplicaSetAge.Hours()/24)),
		Caveat:  "Deleting one removes that rollout from `kubectl rollout history`. revisionHistoryLimit on the Deployment is the setting that prunes them automatically.",
		Items:   []HygieneItem{}}
	if err := in.failed("replicasets"); err != nil {
		group.Warning = fmt.Sprintf("ReplicaSets could not be listed: %v", err)
		return group
	}
	for i := range in.replicaSets {
		set := &in.replicaSets[i]
		if set.DeletionTimestamp != nil || set.Status.Replicas > 0 {
			continue
		}
		if set.Spec.Replicas != nil && *set.Spec.Replicas > 0 {
			continue
		}
		if !olderThan(set.CreationTimestamp, hygieneReplicaSetAge) {
			continue
		}
		owner := "no owner"
		if len(set.OwnerReferences) > 0 {
			owner = set.OwnerReferences[0].Kind + " " + set.OwnerReferences[0].Name
		}
		group.Items = append(group.Items, HygieneItem{
			Category: HygieneOldReplicaSet, Kind: "ReplicaSet", Namespace: set.Namespace, Name: set.Name,
			Age: hygieneAge(set.CreationTimestamp), Severity: SeverityInfo, Chips: []string{owner},
			Title:   "Empty and old",
			Detail:  "It runs no Pod and only exists as rollout history.",
			Command: fmt.Sprintf("kubectl delete rs %s -n %s", set.Name, set.Namespace),
		})
	}
	return group
}

func finishedJobGroup(in *hygieneInputs) HygieneGroup {
	group := HygieneGroup{Category: HygieneFinishedJob, Title: "Jobs that finished long ago",
		Summary: fmt.Sprintf("Complete or Failed for more than %d hours, and still holding their Pods.", int(hygieneFinishedAge.Hours())),
		Caveat:  "ttlSecondsAfterFinished on the Job removes them automatically; a Job kept deliberately for its logs will appear here too.",
		Items:   []HygieneItem{}}
	if err := in.failed("jobs"); err != nil {
		group.Warning = fmt.Sprintf("Jobs could not be listed: %v", err)
		return group
	}
	for i := range in.jobs {
		job := &in.jobs[i]
		if job.DeletionTimestamp != nil || job.Spec.TTLSecondsAfterFinished != nil {
			continue
		}
		result, finished := jobFinished(job)
		if !finished {
			continue
		}
		completed := job.CreationTimestamp
		if job.Status.CompletionTime != nil {
			completed = *job.Status.CompletionTime
		}
		if !olderThan(completed, hygieneFinishedAge) {
			continue
		}
		if len(job.OwnerReferences) > 0 {
			continue // a CronJob prunes its own history
		}
		group.Items = append(group.Items, HygieneItem{
			Category: HygieneFinishedJob, Kind: "Job", Namespace: job.Namespace, Name: job.Name,
			Age: hygieneAge(job.CreationTimestamp), Severity: SeverityInfo, Chips: []string{result, "finished " + hygieneAge(completed) + " ago"},
			Title:   "Finished and never cleaned up",
			Detail:  "Set ttlSecondsAfterFinished so the next one removes itself.",
			Command: fmt.Sprintf("kubectl delete job %s -n %s", job.Name, job.Namespace),
		})
	}
	return group
}

func finishedPodGroup(in *hygieneInputs) HygieneGroup {
	group := HygieneGroup{Category: HygieneFinishedPod, Title: "Pods left in a finished state",
		Summary: fmt.Sprintf("Succeeded, Failed or Evicted for more than %d hours. They hold no compute, but they crowd every list and every count.", int(hygieneFinishedAge.Hours())),
		Caveat:  "Evicted Pods are evidence of node pressure; read why they were evicted before clearing them away.",
		Items:   []HygieneItem{}}
	if err := in.failed("pods"); err != nil {
		group.Warning = fmt.Sprintf("Pods could not be listed: %v", err)
		return group
	}
	for i := range in.pods {
		pod := &in.pods[i]
		if pod.DeletionTimestamp != nil {
			continue
		}
		if pod.Status.Phase != corev1.PodSucceeded && pod.Status.Phase != corev1.PodFailed {
			continue
		}
		if !olderThan(pod.CreationTimestamp, hygieneFinishedAge) {
			continue
		}
		chips := []string{string(pod.Status.Phase)}
		severity := SeverityInfo
		if pod.Status.Reason != "" {
			chips = append(chips, pod.Status.Reason)
		}
		if pod.Status.Reason == "Evicted" {
			severity = SeverityWarning
		}
		if len(pod.OwnerReferences) > 0 {
			chips = append(chips, pod.OwnerReferences[0].Kind+" "+pod.OwnerReferences[0].Name)
		}
		group.Items = append(group.Items, HygieneItem{
			Category: HygieneFinishedPod, Kind: "Pod", Namespace: pod.Namespace, Name: pod.Name,
			Age: hygieneAge(pod.CreationTimestamp), Severity: severity, Chips: chips,
			Title:   "Finished long ago",
			Detail:  "Its logs are still readable until it is deleted.",
			Command: fmt.Sprintf("kubectl delete pod %s -n %s", pod.Name, pod.Namespace),
		})
	}
	return group
}

func unpinnedImageGroup(index *referenceIndex) HygieneGroup {
	group := HygieneGroup{Category: HygieneUnpinnedImage, Title: "Images that are not pinned",
		Summary: "A container whose image is :latest, or has no tag at all, runs whatever that tag points at today. A restart can silently change the build, and a rollback cannot bring the old one back.",
		Caveat:  "Kubby reads the declared image reference, not what the node actually pulled. A tag other than :latest can still move; only a digest is fixed.",
		Items:   []HygieneItem{}}
	seen := map[string]bool{}
	for _, template := range index.templates {
		containers := append(append([]corev1.Container{}, template.spec.InitContainers...), template.spec.Containers...)
		risky := []string{}
		reasons := map[string]bool{}
		for i := range containers {
			risk, found := imageRisk(containers[i].Image)
			if !found {
				continue
			}
			risky = append(risky, containers[i].Name+": "+containers[i].Image)
			reasons[risk] = true
		}
		if len(risky) == 0 {
			continue
		}
		key := template.kind + "/" + template.namespace + "/" + template.name
		if seen[key] {
			continue
		}
		seen[key] = true
		why := make([]string, 0, len(reasons))
		for reason := range reasons {
			why = append(why, reason)
		}
		sort.Strings(why)
		group.Items = append(group.Items, HygieneItem{
			Category: HygieneUnpinnedImage, Kind: template.kind, Namespace: template.namespace, Name: template.name,
			Severity: SeverityWarning, Chips: risky,
			Title:  "Uses " + strings.Join(why, " and "),
			Detail: "Pin the image by digest, or by an immutable tag your build produces, so the same deployment always means the same build.",
		})
	}
	return group
}
