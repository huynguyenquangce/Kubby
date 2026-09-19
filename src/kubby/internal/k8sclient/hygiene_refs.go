package k8sclient

import (
	"strconv"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
)

// Deciding that an object is unused is the dangerous half of a cleanup tool:
// every reference this file fails to see becomes a deletion Kubby recommended
// and should not have. So the index is deliberately generous — it collects from
// live Pods *and* from the templates of every controller, because a Deployment
// scaled to zero still owns its ConfigMap — and whatever it cannot see is
// stated as a caveat on the group rather than silently assumed absent.

// podTemplate is one controller's Pod template, or a Pod that has no controller.
type podTemplate struct {
	kind      string
	namespace string
	name      string
	spec      *corev1.PodSpec
	account   string
}

// referenceIndex is every "namespace/name" some object points at.
type referenceIndex struct {
	configMaps map[string]bool
	secrets    map[string]bool
	claims     map[string]bool
	templates  []podTemplate
}

func newReferenceIndex() *referenceIndex {
	return &referenceIndex{
		configMaps: map[string]bool{},
		secrets:    map[string]bool{},
		claims:     map[string]bool{},
	}
}

func (index *referenceIndex) addPodSpec(namespace string, spec *corev1.PodSpec) {
	key := func(name string) string { return namespace + "/" + name }
	for i := range spec.Volumes {
		volume := &spec.Volumes[i]
		switch {
		case volume.ConfigMap != nil:
			index.configMaps[key(volume.ConfigMap.Name)] = true
		case volume.Secret != nil:
			index.secrets[key(volume.Secret.SecretName)] = true
		case volume.PersistentVolumeClaim != nil:
			index.claims[key(volume.PersistentVolumeClaim.ClaimName)] = true
		case volume.Projected != nil:
			for _, source := range volume.Projected.Sources {
				if source.ConfigMap != nil {
					index.configMaps[key(source.ConfigMap.Name)] = true
				}
				if source.Secret != nil {
					index.secrets[key(source.Secret.Name)] = true
				}
			}
		case volume.CSI != nil && volume.CSI.NodePublishSecretRef != nil:
			index.secrets[key(volume.CSI.NodePublishSecretRef.Name)] = true
		case volume.Ephemeral != nil:
			// An ephemeral volume's claim is named after the Pod and the volume.
			index.claims[key(volume.Name)] = true
		}
	}
	containers := func(list []corev1.Container) {
		for i := range list {
			container := &list[i]
			for _, source := range container.EnvFrom {
				if source.ConfigMapRef != nil {
					index.configMaps[key(source.ConfigMapRef.Name)] = true
				}
				if source.SecretRef != nil {
					index.secrets[key(source.SecretRef.Name)] = true
				}
			}
			for _, env := range container.Env {
				if env.ValueFrom == nil {
					continue
				}
				if env.ValueFrom.ConfigMapKeyRef != nil {
					index.configMaps[key(env.ValueFrom.ConfigMapKeyRef.Name)] = true
				}
				if env.ValueFrom.SecretKeyRef != nil {
					index.secrets[key(env.ValueFrom.SecretKeyRef.Name)] = true
				}
			}
		}
	}
	containers(spec.InitContainers)
	containers(spec.Containers)
	for _, pull := range spec.ImagePullSecrets {
		index.secrets[key(pull.Name)] = true
	}
}

func (index *referenceIndex) addTemplate(kind, namespace, name string, spec *corev1.PodSpec) {
	index.templates = append(index.templates, podTemplate{kind: kind, namespace: namespace, name: name, spec: spec, account: spec.ServiceAccountName})
	index.addPodSpec(namespace, spec)
}

// buildReferenceIndex walks everything that can name a ConfigMap, a Secret or a
// claim. Controller templates are walked as well as live Pods: a Deployment
// scaled to zero has no Pod, and its ConfigMap is still very much in use.
func buildReferenceIndex(in *hygieneInputs) *referenceIndex {
	index := newReferenceIndex()
	for i := range in.pods {
		pod := &in.pods[i]
		index.addPodSpec(pod.Namespace, &pod.Spec)
		if len(pod.OwnerReferences) == 0 {
			index.templates = append(index.templates, podTemplate{kind: "Pod", namespace: pod.Namespace, name: pod.Name, spec: &pod.Spec, account: pod.Spec.ServiceAccountName})
		}
	}
	for i := range in.deployments {
		item := &in.deployments[i]
		index.addTemplate("Deployment", item.Namespace, item.Name, &item.Spec.Template.Spec)
	}
	for i := range in.statefulSets {
		item := &in.statefulSets[i]
		index.addTemplate("StatefulSet", item.Namespace, item.Name, &item.Spec.Template.Spec)
	}
	for i := range in.daemonSets {
		item := &in.daemonSets[i]
		index.addTemplate("DaemonSet", item.Namespace, item.Name, &item.Spec.Template.Spec)
	}
	for i := range in.jobs {
		item := &in.jobs[i]
		index.addTemplate("Job", item.Namespace, item.Name, &item.Spec.Template.Spec)
	}
	for i := range in.cronJobs {
		item := &in.cronJobs[i]
		index.addTemplate("CronJob", item.Namespace, item.Name, &item.Spec.JobTemplate.Spec.Template.Spec)
	}
	for i := range in.replicaSets {
		// A ReplicaSet kept for rollback still names the ConfigMaps that
		// rollback would need.
		item := &in.replicaSets[i]
		index.addPodSpec(item.Namespace, &item.Spec.Template.Spec)
	}
	for i := range in.serviceAccounts {
		account := &in.serviceAccounts[i]
		for _, secret := range account.Secrets {
			index.secrets[account.Namespace+"/"+secret.Name] = true
		}
		for _, pull := range account.ImagePullSecrets {
			index.secrets[account.Namespace+"/"+pull.Name] = true
		}
	}
	for i := range in.ingresses {
		ingress := &in.ingresses[i]
		for _, tls := range ingress.Spec.TLS {
			if tls.SecretName != "" {
				index.secrets[ingress.Namespace+"/"+tls.SecretName] = true
			}
		}
	}
	return index
}

// claimKeptByStatefulSet reports a claim a StatefulSet created from a
// volumeClaimTemplate. Those outlive their Pods by design — scaling a
// StatefulSet to zero keeps the data — so calling them unused would be the
// worst advice this feature could give.
func claimKeptByStatefulSet(claim string, sets []appsv1.StatefulSet) (string, bool) {
	for i := range sets {
		set := &sets[i]
		for _, template := range set.Spec.VolumeClaimTemplates {
			prefix := template.Name + "-" + set.Name + "-"
			if !strings.HasPrefix(claim, prefix) {
				continue
			}
			if _, err := strconv.Atoi(strings.TrimPrefix(claim, prefix)); err == nil {
				return set.Name, true
			}
		}
	}
	return "", false
}

// jobFinished reports a Job that has run its course, and when.
func jobFinished(job *batchv1.Job) (string, bool) {
	for _, condition := range job.Status.Conditions {
		if condition.Status != corev1.ConditionTrue {
			continue
		}
		if condition.Type == batchv1.JobComplete {
			return "Complete", true
		}
		if condition.Type == batchv1.JobFailed {
			return "Failed", true
		}
	}
	return "", false
}

// imageRisk names an image reference that cannot be reproduced later, which is
// how a rollback quietly becomes a different build.
func imageRisk(image string) (string, bool) {
	reference := image
	if at := strings.Index(reference, "@"); at >= 0 {
		return "", false // pinned by digest: the strongest form there is
	}
	// A registry host may carry a port, so only a colon after the last slash is
	// a tag separator.
	tag := ""
	if slash := strings.LastIndex(reference, "/"); slash >= 0 {
		if colon := strings.LastIndex(reference[slash:], ":"); colon >= 0 {
			tag = reference[slash+colon+1:]
		}
	} else if colon := strings.LastIndex(reference, ":"); colon >= 0 {
		tag = reference[colon+1:]
	}
	switch tag {
	case "":
		return "no tag, so the node pulls :latest", true
	case "latest":
		return "the :latest tag", true
	}
	return "", false
}
