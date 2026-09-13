package k8sclient

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/yaml"
)

// gvrByKind maps the resource kinds the UI supports to their GroupVersionResource.
// Keeping an explicit map avoids a discovery round-trip and keeps behaviour obvious.
var gvrByKind = map[string]schema.GroupVersionResource{
	"Pod":                   {Group: "", Version: "v1", Resource: "pods"},
	"Service":               {Group: "", Version: "v1", Resource: "services"},
	"Namespace":             {Group: "", Version: "v1", Resource: "namespaces"},
	"Node":                  {Group: "", Version: "v1", Resource: "nodes"},
	"ConfigMap":             {Group: "", Version: "v1", Resource: "configmaps"},
	"Secret":                {Group: "", Version: "v1", Resource: "secrets"},
	"ServiceAccount":        {Group: "", Version: "v1", Resource: "serviceaccounts"},
	"PersistentVolumeClaim": {Group: "", Version: "v1", Resource: "persistentvolumeclaims"},
	"Deployment":            {Group: "apps", Version: "v1", Resource: "deployments"},
	"ReplicaSet":            {Group: "apps", Version: "v1", Resource: "replicasets"},
	"StatefulSet":           {Group: "apps", Version: "v1", Resource: "statefulsets"},
	"DaemonSet":             {Group: "apps", Version: "v1", Resource: "daemonsets"},
	"Job":                   {Group: "batch", Version: "v1", Resource: "jobs"},
	"CronJob":               {Group: "batch", Version: "v1", Resource: "cronjobs"},
	"Ingress":               {Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"},
	"NetworkPolicy":         {Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"},

	"HorizontalPodAutoscaler": {Group: "autoscaling", Version: "v2", Resource: "horizontalpodautoscalers"},
	"PodDisruptionBudget":     {Group: "policy", Version: "v1", Resource: "poddisruptionbudgets"},

	"PersistentVolume": {Group: "", Version: "v1", Resource: "persistentvolumes"},
	"StorageClass":     {Group: "storage.k8s.io", Version: "v1", Resource: "storageclasses"},

	"Role":               {Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "roles"},
	"RoleBinding":        {Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "rolebindings"},
	"ClusterRole":        {Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterroles"},
	"ClusterRoleBinding": {Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterrolebindings"},

	"ResourceQuota":            {Group: "", Version: "v1", Resource: "resourcequotas"},
	"LimitRange":               {Group: "", Version: "v1", Resource: "limitranges"},
	"CustomResourceDefinition": {Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"},
}

// clusterScopedKinds are not namespaced.
var clusterScopedKinds = map[string]bool{
	"Namespace":                true,
	"Node":                     true,
	"PersistentVolume":         true,
	"StorageClass":             true,
	"ClusterRole":              true,
	"ClusterRoleBinding":       true,
	"CustomResourceDefinition": true,
}

func gvrFor(kind string) (schema.GroupVersionResource, error) {
	gvr, ok := gvrByKind[kind]
	if !ok {
		return schema.GroupVersionResource{}, fmt.Errorf("unsupported kind %q", kind)
	}
	return gvr, nil
}

// resourceFor returns the dynamic client scoped correctly for a kind, resolving
// custom resources through discovery (see apiindex.go). The kind may carry its
// API group — "VirtualService.networking.istio.io".
func resourceFor(c *Cluster, kind, namespace string) (dynamic.ResourceInterface, error) {
	ak, err := c.ResolveKind(kind)
	if err != nil {
		return nil, err
	}
	if !ak.Namespaced {
		return c.Dynamic.Resource(ak.GVR), nil
	}
	return c.Dynamic.Resource(ak.GVR).Namespace(namespace), nil
}

// GetYAML fetches a resource and returns it as YAML, stripping the noisy
// managedFields block so the editor stays readable.
func GetYAML(ctx context.Context, c *Cluster, kind, namespace, name string) (string, error) {
	ri, err := resourceFor(c, kind, namespace)
	if err != nil {
		return "", err
	}
	obj, err := ri.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	// Strip noise so the editor shows a clean, editable spec:
	// managedFields is verbose bookkeeping, and status is a read-only
	// subresource (sending it back on update only triggers warnings).
	unstructured.RemoveNestedField(obj.Object, "metadata", "managedFields")
	unstructured.RemoveNestedField(obj.Object, "status")

	out, err := yaml.Marshal(obj.Object)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// UpdateYAML parses edited YAML and applies it back to the resource that opened
// the editor.  The expected identity is an explicit precondition: editing the
// document may change spec, labels, or annotations, but it must never silently
// retarget Save to another object.
func UpdateYAML(ctx context.Context, c *Cluster, expectedKind, expectedNamespace, expectedName, yamlText string) error {
	var raw map[string]interface{}
	if err := yaml.Unmarshal([]byte(yamlText), &raw); err != nil {
		return fmt.Errorf("invalid YAML: %w", err)
	}
	obj := &unstructured.Unstructured{Object: raw}

	// The edited document carries its own apiVersion, so it resolves exactly —
	// including custom resources the static table knows nothing about.
	ak, err := c.ResolveGVK(obj.GetAPIVersion(), obj.GetKind())
	if err != nil {
		return err
	}
	expected, err := c.ResolveKind(expectedKind)
	if err != nil {
		return err
	}
	if err := validateUpdateTarget(expected, expectedNamespace, expectedName, ak, obj); err != nil {
		return err
	}
	if !ak.Namespaced {
		_, err = c.Dynamic.Resource(ak.GVR).Update(ctx, obj, metav1.UpdateOptions{})
		return err
	}
	_, err = c.Dynamic.Resource(ak.GVR).Namespace(obj.GetNamespace()).Update(ctx, obj, metav1.UpdateOptions{})
	return err
}

func validateUpdateTarget(expected APIKind, expectedNamespace, expectedName string, actual APIKind, obj *unstructured.Unstructured) error {
	stale := func(detail string) error {
		return fmt.Errorf("refusing stale YAML update: %s; reload the open resource", detail)
	}
	if expectedName == "" || obj.GetName() != expectedName {
		return stale(fmt.Sprintf("expected name %q, YAML names %q", expectedName, obj.GetName()))
	}
	if expected.Namespaced != actual.Namespaced || expected.GVR.Group != actual.GVR.Group || expected.GVR.Resource != actual.GVR.Resource {
		return stale(fmt.Sprintf("expected %s, YAML addresses %s", expected.GVR.GroupResource(), actual.GVR.GroupResource()))
	}
	if expected.Namespaced {
		if obj.GetNamespace() != expectedNamespace {
			return stale(fmt.Sprintf("expected namespace %q, YAML names %q", expectedNamespace, obj.GetNamespace()))
		}
	} else if expectedNamespace != "" || obj.GetNamespace() != "" {
		return stale("a cluster-scoped resource cannot carry a namespace")
	}
	return nil
}

// DeleteResource deletes a resource by kind/namespace/name.
func DeleteResource(ctx context.Context, c *Cluster, kind, namespace, name string) error {
	ri, err := resourceFor(c, kind, namespace)
	if err != nil {
		return err
	}
	return ri.Delete(ctx, name, metav1.DeleteOptions{})
}

// SecretEntry is one decoded key/value of a Secret.
type SecretEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// SecretData returns a secret's decoded key/value pairs (the client-go typed
// client already base64-decodes Data). Used by the drawer's "reveal" action.
func SecretData(ctx context.Context, c *Cluster, namespace, name string) ([]SecretEntry, error) {
	s, err := c.Clientset.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]SecretEntry, 0, len(s.Data))
	for k, v := range s.Data {
		out = append(out, SecretEntry{Key: k, Value: string(v)})
	}
	return out, nil
}

// PodContainers returns the container names of a pod (init + regular) so the
// UI can offer a picker for multi-container pods.
func PodContainers(ctx context.Context, c *Cluster, namespace, name string) ([]string, error) {
	pod, err := c.Clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(pod.Spec.Containers))
	for _, ct := range pod.Spec.Containers {
		names = append(names, ct.Name)
	}
	return names, nil
}

// ContainerState is one regular container's runtime state for the Logs and
// Terminal tabs: enough to say that a container has restarted, how its last
// instance ended, and whether that instance's logs can be read.
type ContainerState struct {
	Name               string `json:"name"`
	Ready              bool   `json:"ready"`
	State              string `json:"state"` // "Running", "Waiting: CrashLoopBackOff", "Terminated: Completed"
	RestartCount       int32  `json:"restartCount"`
	LastTermination    string `json:"lastTermination"`    // "OOMKilled, exit 137"
	LastTerminationAge string `json:"lastTerminationAge"` // "3m"
	HasPrevious        bool   `json:"hasPrevious"`
}

// PodContainerStates returns every regular container with its current state,
// in spec order, from one Pod read.
func PodContainerStates(ctx context.Context, c *Cluster, namespace, name string) ([]ContainerState, error) {
	pod, err := c.Clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	statuses := map[string]corev1.ContainerStatus{}
	for _, status := range pod.Status.ContainerStatuses {
		statuses[status.Name] = status
	}
	out := make([]ContainerState, 0, len(pod.Spec.Containers))
	for _, container := range pod.Spec.Containers {
		status, known := statuses[container.Name]
		state := ContainerState{Name: container.Name, Ready: status.Ready, RestartCount: status.RestartCount}
		switch {
		case !known:
			state.State = "Unknown"
		case status.State.Running != nil:
			state.State = "Running"
		case status.State.Waiting != nil:
			state.State = conditionText("Waiting", status.State.Waiting.Reason)
		case status.State.Terminated != nil:
			state.State = conditionText("Terminated", status.State.Terminated.Reason)
		}
		// The kubelet keeps the previous instance only once a container has
		// restarted, and records how that instance ended alongside it.
		if last := status.LastTerminationState.Terminated; last != nil && status.RestartCount > 0 {
			state.HasPrevious = true
			reason := last.Reason
			if reason == "" {
				reason = "Terminated"
			}
			state.LastTermination = fmt.Sprintf("%s, exit %d", reason, last.ExitCode)
			if !last.FinishedAt.IsZero() {
				state.LastTerminationAge = age(last.FinishedAt)
			}
		}
		out = append(out, state)
	}
	return out, nil
}

// PodLogs returns the last tailLines lines of a container's logs. previous
// reads the instance that ran before the most recent restart — for a
// CrashLoopBackOff that is where the failure was written, while the current
// instance may not have logged anything yet.
func PodLogs(ctx context.Context, c *Cluster, namespace, name, container string, tailLines int64, previous bool) (string, error) {
	opts := &corev1.PodLogOptions{TailLines: &tailLines, Previous: previous}
	if container != "" {
		opts.Container = container
	}
	req := c.Clientset.CoreV1().Pods(namespace).GetLogs(name, opts)
	data, err := req.DoRaw(ctx)
	if err != nil {
		if previous {
			return "", previousLogsError(container, err)
		}
		return "", err
	}
	return string(data), nil
}

// previousLogsError replaces the kubelet's "previous terminated container ...
// not found" with what it means, since that is the ordinary answer for a
// container that has never restarted rather than a failure.
func previousLogsError(container string, err error) error {
	if !strings.Contains(err.Error(), "previous terminated container") {
		return err
	}
	subject := "This container"
	if container != "" {
		subject = fmt.Sprintf("Container %q", container)
	}
	return fmt.Errorf("%s has no previous instance: it has not restarted, or the kubelet has already discarded the terminated container's logs", subject)
}
