package k8sclient

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	autoscalingv1 "k8s.io/api/autoscaling/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// ScaleDeployment sets the replica count of a deployment.
func ScaleDeployment(ctx context.Context, c *Cluster, namespace, name string, replicas int32) error {
	scale := &autoscalingv1.Scale{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       autoscalingv1.ScaleSpec{Replicas: replicas},
	}
	_, err := c.Clientset.AppsV1().Deployments(namespace).UpdateScale(ctx, name, scale, metav1.UpdateOptions{})
	return err
}

// RestartDeployment triggers a rolling restart the same way `kubectl rollout
// restart` does: patch a restartedAt annotation into the pod template.
func RestartDeployment(ctx context.Context, c *Cluster, namespace, name, stamp string) error {
	patch := fmt.Sprintf(
		`{"spec":{"template":{"metadata":{"annotations":{"kubby.dev/restartedAt":%q}}}}}`, stamp)
	_, err := c.Clientset.AppsV1().Deployments(namespace).Patch(
		ctx, name, types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{})
	return err
}

// RestartStatefulSet triggers a rolling restart of a StatefulSet.
func RestartStatefulSet(ctx context.Context, c *Cluster, namespace, name, stamp string) error {
	patch := fmt.Sprintf(
		`{"spec":{"template":{"metadata":{"annotations":{"kubby.dev/restartedAt":%q}}}}}`, stamp)
	_, err := c.Clientset.AppsV1().StatefulSets(namespace).Patch(
		ctx, name, types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{})
	return err
}

// RestartDaemonSet triggers a rolling restart of a DaemonSet.
func RestartDaemonSet(ctx context.Context, c *Cluster, namespace, name, stamp string) error {
	patch := fmt.Sprintf(
		`{"spec":{"template":{"metadata":{"annotations":{"kubby.dev/restartedAt":%q}}}}}`, stamp)
	_, err := c.Clientset.AppsV1().DaemonSets(namespace).Patch(
		ctx, name, types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{})
	return err
}

// SetDeploymentPaused pauses or resumes a deployment rollout.
func SetDeploymentPaused(ctx context.Context, c *Cluster, namespace, name string, paused bool) error {
	patch := fmt.Sprintf(`{"spec":{"paused":%t}}`, paused)
	_, err := c.Clientset.AppsV1().Deployments(namespace).Patch(
		ctx, name, types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{})
	return err
}

// RolloutRevision is one entry in a deployment's rollout history.
type RolloutRevision struct {
	Revision int64  `json:"revision"`
	Name     string `json:"name"` // the ReplicaSet name
	Images   string `json:"images"`
	Current  bool   `json:"current"`
	Age      string `json:"age"`
}

// RolloutHistory lists a deployment's revisions (from its ReplicaSets), newest
// first, like `kubectl rollout history`.
func RolloutHistory(ctx context.Context, c *Cluster, namespace, name string) ([]RolloutRevision, error) {
	dep, err := c.Clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	rsList, err := c.Clientset.AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	currentRev := dep.Annotations["deployment.kubernetes.io/revision"]
	out := []RolloutRevision{}
	for i := range rsList.Items {
		rs := &rsList.Items[i]
		if !ownedBy(rs.OwnerReferences, "Deployment", dep.Name, dep.UID) {
			continue
		}
		rev, _ := strconv.ParseInt(rs.Annotations["deployment.kubernetes.io/revision"], 10, 64)
		images := []string{}
		for _, ct := range rs.Spec.Template.Spec.Containers {
			images = append(images, ct.Image)
		}
		out = append(out, RolloutRevision{
			Revision: rev,
			Name:     rs.Name,
			Images:   joinComma(images),
			Current:  rs.Annotations["deployment.kubernetes.io/revision"] == currentRev,
			Age:      age(rs.CreationTimestamp),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Revision > out[j].Revision })
	return out, nil
}

// RollbackDeployment rolls a deployment back to a given revision by copying that
// ReplicaSet's pod template back onto the deployment.
func RollbackDeployment(ctx context.Context, c *Cluster, namespace, name string, revision int64) error {
	rsList, err := c.Clientset.AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	dep, err := c.Clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	for i := range rsList.Items {
		rs := &rsList.Items[i]
		if !ownedBy(rs.OwnerReferences, "Deployment", dep.Name, dep.UID) {
			continue
		}
		rev, _ := strconv.ParseInt(rs.Annotations["deployment.kubernetes.io/revision"], 10, 64)
		if rev != revision {
			continue
		}
		// Copy the target RS's template onto the deployment (drop the hash label).
		tmpl := rs.Spec.Template.DeepCopy()
		delete(tmpl.Labels, "pod-template-hash")
		dep.Spec.Template = *tmpl
		_, err = c.Clientset.AppsV1().Deployments(namespace).Update(ctx, dep, metav1.UpdateOptions{})
		return err
	}
	return fmt.Errorf("revision %d not found", revision)
}

// SetNodeSchedulable cordons (schedulable=false) or uncordons a node.
func SetNodeSchedulable(ctx context.Context, c *Cluster, name string, schedulable bool) error {
	patch := fmt.Sprintf(`{"spec":{"unschedulable":%t}}`, !schedulable)
	_, err := c.Clientset.CoreV1().Nodes().Patch(
		ctx, name, types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{})
	return err
}

// DrainNode cordons a node then evicts its pods (skipping DaemonSet-managed and
// mirror pods), like a simplified `kubectl drain`.
func DrainNode(ctx context.Context, c *Cluster, name string) error {
	if err := SetNodeSchedulable(ctx, c, name, false); err != nil {
		return err
	}
	pods, err := c.Clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		FieldSelector: "spec.nodeName=" + name,
	})
	if err != nil {
		return err
	}
	failed := make([]string, 0)
	for i := range pods.Items {
		pod := &pods.Items[i]
		if isDaemonSetPod(pod) {
			continue
		}
		if _, ok := pod.Annotations[corev1.MirrorPodAnnotationKey]; ok {
			continue
		}
		eviction := &policyv1.Eviction{
			ObjectMeta: metav1.ObjectMeta{Name: pod.Name, Namespace: pod.Namespace},
		}
		if err := c.Clientset.PolicyV1().Evictions(pod.Namespace).Evict(ctx, eviction); err != nil {
			// Best-effort: keep draining the rest, but never report a partial
			// drain as success. PDB, RBAC and throttling failures need to name
			// the pods that remain on the cordoned node.
			failed = append(failed, fmt.Sprintf("%s/%s: %v", pod.Namespace, pod.Name, err))
			continue
		}
	}
	if len(failed) > 0 {
		sort.Strings(failed)
		return fmt.Errorf("node was cordoned, but %d pod eviction(s) failed: %s", len(failed), strings.Join(failed, "; "))
	}
	return nil
}

func isDaemonSetPod(pod *corev1.Pod) bool {
	for _, r := range pod.OwnerReferences {
		if r.Kind == "DaemonSet" {
			return true
		}
	}
	return false
}

// RunCronJobNow creates a one-off Job from a CronJob's job template, like
// `kubectl create job --from=cronjob/<name>`.
func RunCronJobNow(ctx context.Context, c *Cluster, namespace, name string) error {
	cj, err := c.Clientset.BatchV1().CronJobs(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	// Preserve room for the API server's random suffix. Truncating the whole
	// name after appending a timestamp removed the unique part for long names.
	prefix := name + "-manual-"
	const maxGeneratePrefix = 58
	if len(prefix) > maxGeneratePrefix {
		prefix = strings.TrimRight(prefix[:maxGeneratePrefix-1], "-.") + "-"
	}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: prefix,
			Namespace:    namespace,
			Labels:       cj.Spec.JobTemplate.Labels,
			Annotations:  map[string]string{"cronjob.kubernetes.io/instantiate": "manual"},
		},
		Spec: cj.Spec.JobTemplate.Spec,
	}
	_, err = c.Clientset.BatchV1().Jobs(namespace).Create(ctx, job, metav1.CreateOptions{})
	return err
}

func joinComma(items []string) string {
	out := ""
	for i, s := range items {
		if i > 0 {
			out += ", "
		}
		out += s
	}
	return out
}
