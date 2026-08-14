package k8sclient

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
)

// DetailField is a single label/value line in the Details tab.
type DetailField struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// ResourceDetail is the structured detail shown in the drawer's Details tab.
type ResourceDetail struct {
	Kind        string            `json:"kind"`
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	Created     string            `json:"created"`
	Age         string            `json:"age"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	Info        []DetailField     `json:"info"`
}

// EventInfo is a trimmed Kubernetes Event for the Events section.
type EventInfo struct {
	Type    string `json:"type"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
	Count   int32  `json:"count"`
	Age     string `json:"age"`
	IsWarn  bool   `json:"isWarn"`
	Object  string `json:"object"` // "Kind/name" of the involved object (cluster-wide view)
}

// GetDetail returns the structured details for a resource (Details tab).
func GetDetail(ctx context.Context, c *Cluster, kind, namespace, name string) (*ResourceDetail, error) {
	d := &ResourceDetail{Kind: kind, Name: name, Namespace: namespace}

	switch kind {
	case "Pod":
		pod, err := c.Clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		fillMeta(d, pod.ObjectMeta)
		d.Info = []DetailField{
			{"Phase", string(pod.Status.Phase)},
			{"Node", pod.Spec.NodeName},
			{"Pod IP", pod.Status.PodIP},
			{"Host IP", pod.Status.HostIP},
			{"QoS Class", string(pod.Status.QOSClass)},
			{"Service Account", pod.Spec.ServiceAccountName},
		}
		for _, ct := range pod.Spec.Containers {
			d.Info = append(d.Info, DetailField{"Container", fmt.Sprintf("%s (%s)", ct.Name, ct.Image)})
		}
	case "Deployment":
		dep, err := c.Clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		fillMeta(d, dep.ObjectMeta)
		desired := int32(0)
		if dep.Spec.Replicas != nil {
			desired = *dep.Spec.Replicas
		}
		images := []string{}
		for _, ct := range dep.Spec.Template.Spec.Containers {
			images = append(images, ct.Image)
		}
		d.Info = []DetailField{
			{"Replicas", fmt.Sprintf("%d desired / %d ready / %d up-to-date / %d available",
				desired, dep.Status.ReadyReplicas, dep.Status.UpdatedReplicas, dep.Status.AvailableReplicas)},
			{"Strategy", string(dep.Spec.Strategy.Type)},
			{"Selector", labelsToString(dep.Spec.Selector.MatchLabels)},
			{"Images", strings.Join(images, ", ")},
		}
		for _, cond := range dep.Status.Conditions {
			d.Info = append(d.Info, DetailField{
				"Condition " + string(cond.Type),
				fmt.Sprintf("%s — %s", cond.Status, cond.Message),
			})
		}
	case "ReplicaSet":
		rs, err := c.Clientset.AppsV1().ReplicaSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		fillMeta(d, rs.ObjectMeta)
		desired := int32(0)
		if rs.Spec.Replicas != nil {
			desired = *rs.Spec.Replicas
		}
		images := []string{}
		for _, ct := range rs.Spec.Template.Spec.Containers {
			images = append(images, ct.Image)
		}
		d.Info = []DetailField{
			{"Replicas", fmt.Sprintf("%d desired / %d ready / %d available", desired, rs.Status.ReadyReplicas, rs.Status.AvailableReplicas)},
			{"Selector", labelsToString(rs.Spec.Selector.MatchLabels)},
			{"Images", strings.Join(images, ", ")},
		}
	case "Service":
		svc, err := c.Clientset.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		fillMeta(d, svc.ObjectMeta)
		ports := []string{}
		for _, p := range svc.Spec.Ports {
			ports = append(ports, fmt.Sprintf("%d/%s", p.Port, p.Protocol))
		}
		d.Info = []DetailField{
			{"Type", string(svc.Spec.Type)},
			{"Cluster IP", svc.Spec.ClusterIP},
			{"Ports", strings.Join(ports, ", ")},
			{"Selector", labelsToString(svc.Spec.Selector)},
			{"Session Affinity", string(svc.Spec.SessionAffinity)},
		}
	case "Node":
		node, err := c.Clientset.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		fillMeta(d, node.ObjectMeta)
		ni := node.Status.NodeInfo
		d.Info = []DetailField{
			{"Kubelet Version", ni.KubeletVersion},
			{"OS / Arch", fmt.Sprintf("%s / %s", ni.OperatingSystem, ni.Architecture)},
			{"OS Image", ni.OSImage},
			{"Container Runtime", ni.ContainerRuntimeVersion},
			{"CPU Capacity", node.Status.Capacity.Cpu().String()},
			{"Memory Capacity", node.Status.Capacity.Memory().String()},
			{"Pods Capacity", node.Status.Capacity.Pods().String()},
		}
		for _, cond := range node.Status.Conditions {
			d.Info = append(d.Info, DetailField{"Condition " + string(cond.Type), string(cond.Status)})
		}
	case "Namespace":
		ns, err := c.Clientset.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		fillMeta(d, ns.ObjectMeta)
		d.Info = []DetailField{{"Phase", string(ns.Status.Phase)}}
	case "ConfigMap":
		cm, err := c.Clientset.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		fillMeta(d, cm.ObjectMeta)
		keys := make([]string, 0, len(cm.Data))
		for k := range cm.Data {
			keys = append(keys, k)
		}
		d.Info = []DetailField{{"Keys", strings.Join(keys, ", ")}}
	case "Secret":
		s, err := c.Clientset.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		fillMeta(d, s.ObjectMeta)
		keys := make([]string, 0, len(s.Data))
		for k := range s.Data {
			keys = append(keys, k)
		}
		d.Info = []DetailField{
			{"Type", string(s.Type)},
			{"Keys", strings.Join(keys, ", ")}, // values are never exposed
		}
	case "StatefulSet":
		s, err := c.Clientset.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		fillMeta(d, s.ObjectMeta)
		desired := int32(0)
		if s.Spec.Replicas != nil {
			desired = *s.Spec.Replicas
		}
		d.Info = []DetailField{
			{"Replicas", fmt.Sprintf("%d desired / %d ready", desired, s.Status.ReadyReplicas)},
			{"Service Name", s.Spec.ServiceName},
			{"Selector", labelsToString(s.Spec.Selector.MatchLabels)},
		}
	case "DaemonSet":
		ds, err := c.Clientset.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		fillMeta(d, ds.ObjectMeta)
		d.Info = []DetailField{
			{"Desired", fmt.Sprintf("%d", ds.Status.DesiredNumberScheduled)},
			{"Ready", fmt.Sprintf("%d", ds.Status.NumberReady)},
			{"Available", fmt.Sprintf("%d", ds.Status.NumberAvailable)},
			{"Selector", labelsToString(ds.Spec.Selector.MatchLabels)},
		}
	case "Job":
		j, err := c.Clientset.BatchV1().Jobs(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		fillMeta(d, j.ObjectMeta)
		d.Info = []DetailField{
			{"Succeeded", fmt.Sprintf("%d", j.Status.Succeeded)},
			{"Active", fmt.Sprintf("%d", j.Status.Active)},
			{"Failed", fmt.Sprintf("%d", j.Status.Failed)},
		}
	case "CronJob":
		cj, err := c.Clientset.BatchV1().CronJobs(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		fillMeta(d, cj.ObjectMeta)
		suspend := false
		if cj.Spec.Suspend != nil {
			suspend = *cj.Spec.Suspend
		}
		d.Info = []DetailField{
			{"Schedule", cj.Spec.Schedule},
			{"Suspend", fmt.Sprintf("%t", suspend)},
			{"Active Jobs", fmt.Sprintf("%d", len(cj.Status.Active))},
		}
	case "Ingress":
		ing, err := c.Clientset.NetworkingV1().Ingresses(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		fillMeta(d, ing.ObjectMeta)
		class := ""
		if ing.Spec.IngressClassName != nil {
			class = *ing.Spec.IngressClassName
		}
		d.Info = []DetailField{{"Ingress Class", class}}
		for _, r := range ing.Spec.Rules {
			d.Info = append(d.Info, DetailField{"Host", r.Host})
		}
	case "PersistentVolumeClaim":
		p, err := c.Clientset.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		fillMeta(d, p.ObjectMeta)
		sc := ""
		if p.Spec.StorageClassName != nil {
			sc = *p.Spec.StorageClassName
		}
		capacity := ""
		if q, ok := p.Status.Capacity["storage"]; ok {
			capacity = q.String()
		}
		d.Info = []DetailField{
			{"Status", string(p.Status.Phase)},
			{"Volume", p.Spec.VolumeName},
			{"Capacity", capacity},
			{"Storage Class", sc},
			{"Access Modes", fmt.Sprintf("%v", p.Spec.AccessModes)},
		}
	case "ServiceAccount":
		sa, err := c.Clientset.CoreV1().ServiceAccounts(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		fillMeta(d, sa.ObjectMeta)
		d.Info = []DetailField{{"Secrets", fmt.Sprintf("%d", len(sa.Secrets))}}
	case "PersistentVolume":
		pv, err := c.Clientset.CoreV1().PersistentVolumes().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		fillMeta(d, pv.ObjectMeta)
		capacity := ""
		if q, ok := pv.Spec.Capacity["storage"]; ok {
			capacity = q.String()
		}
		claim := ""
		if pv.Spec.ClaimRef != nil {
			claim = fmt.Sprintf("%s/%s", pv.Spec.ClaimRef.Namespace, pv.Spec.ClaimRef.Name)
		}
		d.Info = []DetailField{
			{"Status", string(pv.Status.Phase)},
			{"Capacity", capacity},
			{"Access Modes", fmt.Sprintf("%v", pv.Spec.AccessModes)},
			{"Reclaim Policy", string(pv.Spec.PersistentVolumeReclaimPolicy)},
			{"Storage Class", pv.Spec.StorageClassName},
			{"Claim", claim},
		}
	case "StorageClass":
		sc, err := c.Clientset.StorageV1().StorageClasses().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		fillMeta(d, sc.ObjectMeta)
		reclaim := ""
		if sc.ReclaimPolicy != nil {
			reclaim = string(*sc.ReclaimPolicy)
		}
		binding := ""
		if sc.VolumeBindingMode != nil {
			binding = string(*sc.VolumeBindingMode)
		}
		d.Info = []DetailField{
			{"Provisioner", sc.Provisioner},
			{"Reclaim Policy", reclaim},
			{"Volume Binding Mode", binding},
			{"Default", fmt.Sprintf("%t", sc.Annotations["storageclass.kubernetes.io/is-default-class"] == "true")},
		}
	case "Role":
		r, err := c.Clientset.RbacV1().Roles(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		fillMeta(d, r.ObjectMeta)
		d.Info = []DetailField{{"Rules", fmt.Sprintf("%d", len(r.Rules))}}
		for _, rule := range r.Rules {
			d.Info = append(d.Info, DetailField{"Rule", ruleToString(rule.Verbs, rule.Resources, rule.APIGroups)})
		}
	case "ClusterRole":
		r, err := c.Clientset.RbacV1().ClusterRoles().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		fillMeta(d, r.ObjectMeta)
		d.Info = []DetailField{{"Rules", fmt.Sprintf("%d", len(r.Rules))}}
		for _, rule := range r.Rules {
			d.Info = append(d.Info, DetailField{"Rule", ruleToString(rule.Verbs, rule.Resources, rule.APIGroups)})
		}
	case "RoleBinding":
		rb, err := c.Clientset.RbacV1().RoleBindings(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		fillMeta(d, rb.ObjectMeta)
		d.Info = []DetailField{
			{"Role Ref", fmt.Sprintf("%s/%s", rb.RoleRef.Kind, rb.RoleRef.Name)},
			{"Subjects", subjectsToString(rb.Subjects)},
		}
	case "ClusterRoleBinding":
		rb, err := c.Clientset.RbacV1().ClusterRoleBindings().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		fillMeta(d, rb.ObjectMeta)
		d.Info = []DetailField{
			{"Role Ref", fmt.Sprintf("%s/%s", rb.RoleRef.Kind, rb.RoleRef.Name)},
			{"Subjects", subjectsToString(rb.Subjects)},
		}
	case "ResourceQuota":
		q, err := c.Clientset.CoreV1().ResourceQuotas(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		fillMeta(d, q.ObjectMeta)
		for res, hard := range q.Status.Hard {
			used := q.Status.Used[res]
			d.Info = append(d.Info, DetailField{string(res), fmt.Sprintf("%s / %s", used.String(), hard.String())})
		}
	case "LimitRange":
		lr, err := c.Clientset.CoreV1().LimitRanges(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		fillMeta(d, lr.ObjectMeta)
		for _, l := range lr.Spec.Limits {
			d.Info = append(d.Info, DetailField{"Type", string(l.Type)})
		}
	case "CustomResourceDefinition":
		obj, err := c.Dynamic.Resource(crdGVR).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		fillMeta(d, metav1.ObjectMeta{
			Name: obj.GetName(), CreationTimestamp: obj.GetCreationTimestamp(),
			Labels: obj.GetLabels(), Annotations: obj.GetAnnotations(),
		})
		group, _, _ := unstructured.NestedString(obj.Object, "spec", "group")
		kind, _, _ := unstructured.NestedString(obj.Object, "spec", "names", "kind")
		plural, _, _ := unstructured.NestedString(obj.Object, "spec", "names", "plural")
		scope, _, _ := unstructured.NestedString(obj.Object, "spec", "scope")
		d.Info = []DetailField{
			{"Group", group},
			{"Kind", kind},
			{"Plural", plural},
			{"Scope", scope},
		}
	default:
		// Anything the cluster serves but this switch has no bespoke view for —
		// every custom resource, in practice. There is no schema to read, so the
		// details come from the object itself: metadata, plus whatever the
		// top-level spec/status fields happen to be. The YAML tab has the rest.
		return genericDetail(ctx, c, kind, namespace, name)
	}
	return d, nil
}

// genericDetail describes any resource generically via the dynamic client.
func genericDetail(ctx context.Context, c *Cluster, kind, namespace, name string) (*ResourceDetail, error) {
	ak, err := c.ResolveKind(kind)
	if err != nil {
		return nil, err
	}
	ri, err := resourceFor(c, kind, namespace)
	if err != nil {
		return nil, err
	}
	obj, err := ri.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	d := &ResourceDetail{Kind: ak.Kind, Name: name, Namespace: obj.GetNamespace()}
	fillMeta(d, metav1.ObjectMeta{
		Name: obj.GetName(), Namespace: obj.GetNamespace(),
		CreationTimestamp: obj.GetCreationTimestamp(),
		Labels:            obj.GetLabels(), Annotations: obj.GetAnnotations(),
	})
	d.Info = []DetailField{{"API version", obj.GetAPIVersion()}}
	d.Info = append(d.Info, summariseFields(obj.Object, "spec")...)
	d.Info = append(d.Info, summariseFields(obj.Object, "status")...)
	return d, nil
}

// summariseFields renders the top level of an arbitrary spec/status block as
// label/value lines. Scalars print as-is; lists and maps are summarised by shape
// so a large object cannot flood the panel — the YAML tab shows the detail.
func summariseFields(object map[string]interface{}, section string) []DetailField {
	block, found, err := unstructured.NestedMap(object, section)
	if err != nil || !found {
		return nil
	}
	keys := make([]string, 0, len(block))
	for k := range block {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make([]DetailField, 0, len(keys))
	for _, k := range keys {
		label := strings.ToUpper(section[:1]) + section[1:] + " · " + k
		switch v := block[k].(type) {
		case nil:
			out = append(out, DetailField{label, ""})
		case []interface{}:
			out = append(out, DetailField{label, fmt.Sprintf("%d item(s)", len(v))})
		case map[string]interface{}:
			inner := make([]string, 0, len(v))
			for ik := range v {
				inner = append(inner, ik)
			}
			sort.Strings(inner)
			out = append(out, DetailField{label, strings.Join(inner, ", ")})
		default:
			out = append(out, DetailField{label, trimTo(fmt.Sprintf("%v", v), 200)})
		}
	}
	return out
}

func trimTo(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// ListEvents returns the events involving a specific object (like `kubectl describe`).
func ListEvents(ctx context.Context, c *Cluster, kind, namespace, name string) ([]EventInfo, error) {
	selectorFields := fields.Set{
		"involvedObject.name": name,
		// Events record the plain kind — a group-qualified reference like
		// "VirtualService.networking.istio.io" would never match.
		"involvedObject.kind": bareKind(kind),
	}

	ns := namespace
	if resolved, err := c.ResolveKind(kind); err == nil {
		if !resolved.Namespaced {
			ns = "" // cluster-scoped object events live outside a resource namespace
		}
		// Name+kind are reusable identities. Add the live UID whenever it can be
		// read so events from a deleted/recreated object do not leak into the new
		// drawer. A failed UID lookup degrades to the old selector rather than
		// hiding events from a token that can list them but not get the object.
		if c.Dynamic != nil {
			var obj *unstructured.Unstructured
			if resolved.Namespaced {
				obj, err = c.Dynamic.Resource(resolved.GVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
			} else {
				obj, err = c.Dynamic.Resource(resolved.GVR).Get(ctx, name, metav1.GetOptions{})
			}
			if err == nil && obj.GetUID() != "" {
				selectorFields["involvedObject.uid"] = string(obj.GetUID())
			}
		}
	} else if clusterScopedKinds[bareKind(kind)] {
		ns = ""
	}

	list, err := c.Clientset.CoreV1().Events(ns).List(ctx, metav1.ListOptions{FieldSelector: selectorFields.AsSelector().String()})
	if err != nil {
		return nil, err
	}

	result := make([]EventInfo, 0, len(list.Items))
	for _, e := range list.Items {
		ts := e.LastTimestamp
		if ts.IsZero() {
			ts = metav1.Time{Time: e.EventTime.Time}
		}
		result = append(result, EventInfo{
			Type:    e.Type,
			Reason:  e.Reason,
			Message: e.Message,
			Count:   e.Count,
			Age:     age(ts),
			IsWarn:  e.Type == "Warning",
		})
	}
	return result, nil
}

// ClusterEvents returns the most recent events across all namespaces, newest
// first, capped at `limit`. Powers the Overview dashboard's activity feed.
func ClusterEvents(ctx context.Context, c *Cluster, limit int) ([]EventInfo, error) {
	list, err := c.Clientset.CoreV1().Events("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return recentEventInfos(list.Items, limit), nil
}

func recentEventInfos(source []corev1.Event, limit int) []EventInfo {
	items := append([]corev1.Event(nil), source...)
	sort.Slice(items, func(i, j int) bool {
		return eventTime(items[i]).After(eventTime(items[j]).Time)
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	out := make([]EventInfo, 0, len(items))
	for _, e := range items {
		out = append(out, EventInfo{
			Type:    e.Type,
			Reason:  e.Reason,
			Message: e.Message,
			Count:   e.Count,
			Age:     age(eventTime(e)),
			IsWarn:  e.Type == "Warning",
			Object:  fmt.Sprintf("%s/%s", e.InvolvedObject.Kind, e.InvolvedObject.Name),
		})
	}
	return out
}

func eventTime(e corev1.Event) metav1.Time {
	if !e.LastTimestamp.IsZero() {
		return e.LastTimestamp
	}
	return metav1.Time{Time: e.EventTime.Time}
}

func fillMeta(d *ResourceDetail, m metav1.ObjectMeta) {
	d.Created = m.CreationTimestamp.Format(time.RFC3339)
	d.Age = age(m.CreationTimestamp)
	d.Labels = m.Labels
	d.Annotations = m.Annotations
}

func ruleToString(verbs, resources, apiGroups []string) string {
	grp := strings.Join(apiGroups, ",")
	if grp == "" {
		grp = "core"
	}
	return fmt.Sprintf("[%s] %s → %s", grp, strings.Join(resources, ","), strings.Join(verbs, ","))
}

func labelsToString(m map[string]string) string {
	if len(m) == 0 {
		return "<none>"
	}
	parts := make([]string, 0, len(m))
	for k, v := range m {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, ", ")
}

// age formats a timestamp like kubectl's AGE column (e.g. 5s, 12m, 3h, 2d).
func age(t metav1.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t.Time)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
