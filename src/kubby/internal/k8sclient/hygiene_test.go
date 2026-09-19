package k8sclient

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

// A cleanup tool is only as good as its false-positive rate: every test here
// pins something Kubby must NOT call unused, next to the thing it must.

const hygieneNamespace = "apps"

func hygieneMeta(kind, name string, mutate ...func(*metav1.PartialObjectMetadata)) *metav1.PartialObjectMetadata {
	object := partialMetadata("v1", kind, hygieneNamespace, name)
	object.CreationTimestamp = metav1.NewTime(checksTestNow.Add(-90 * 24 * time.Hour))
	for _, fn := range mutate {
		fn(object)
	}
	return object
}

func hygieneRun(t *testing.T, typed []runtime.Object, meta []runtime.Object, mutate ...func(*kubefake.Clientset)) *HygieneReport {
	t.Helper()
	client := kubefake.NewSimpleClientset(typed...)
	for _, fn := range mutate {
		fn(client)
	}
	cluster := &Cluster{Clientset: client, Meta: newOverviewMetadataClient(meta...)}
	report, err := ClusterHygiene(context.Background(), cluster, hygieneNamespace)
	if err != nil {
		t.Fatalf("ClusterHygiene: %v", err)
	}
	return report
}

func hygieneGroup(t *testing.T, report *HygieneReport, category string) HygieneGroup {
	t.Helper()
	for _, group := range report.Groups {
		if group.Category == category {
			return group
		}
	}
	t.Fatalf("group %q is missing from the report", category)
	return HygieneGroup{}
}

func hygieneNames(group HygieneGroup) []string {
	names := make([]string, 0, len(group.Items))
	for _, item := range group.Items {
		names = append(names, item.Name)
	}
	return names
}

func hygieneHas(group HygieneGroup, name string) bool {
	for _, item := range group.Items {
		if item.Name == name {
			return true
		}
	}
	return false
}

func TestHygieneConfigMapReferences(t *testing.T) {
	pinChecksNow(t)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: hygieneNamespace, Name: "api-1"},
		Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{{Name: "config", VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "mounted"}}}}},
			Containers: []corev1.Container{{Name: "main", Env: []corev1.EnvVar{{Name: "K", ValueFrom: &corev1.EnvVarSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "env-key"}}}}}}},
		},
	}
	// A Deployment scaled to zero has no Pod, and its ConfigMap is still in use.
	zero := int32(0)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Namespace: hygieneNamespace, Name: "worker"},
		Spec: appsv1.DeploymentSpec{Replicas: &zero, Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "main", EnvFrom: []corev1.EnvFromSource{{
				ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "scaled-to-zero"}}}}}},
		}}},
	}
	report := hygieneRun(t, []runtime.Object{pod, deployment}, []runtime.Object{
		hygieneMeta("ConfigMap", "mounted"),
		hygieneMeta("ConfigMap", "env-key"),
		hygieneMeta("ConfigMap", "scaled-to-zero"),
		hygieneMeta("ConfigMap", "kube-root-ca.crt"),
		hygieneMeta("ConfigMap", "owned", func(object *metav1.PartialObjectMetadata) {
			object.OwnerReferences = []metav1.OwnerReference{{Kind: "Cluster", Name: "db"}}
		}),
		hygieneMeta("ConfigMap", "orphan"),
	})

	group := hygieneGroup(t, report, HygieneUnusedConfigMap)
	if names := hygieneNames(group); len(names) != 1 || names[0] != "orphan" {
		t.Fatalf("unused ConfigMaps = %v, want only the orphan", names)
	}
	item := group.Items[0]
	if item.Command != "kubectl delete configmap orphan -n apps" {
		t.Fatalf("command = %q", item.Command)
	}
	if item.Severity != SeverityInfo {
		t.Fatalf("an unused ConfigMap is untidy, not broken: %q", item.Severity)
	}
	if group.Caveat == "" || !strings.Contains(group.Caveat, "by name") {
		t.Fatalf("the group must state what a reference scan cannot see: %q", group.Caveat)
	}
}

func TestHygieneSecretReferencesAndExclusions(t *testing.T) {
	pinChecksNow(t)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: hygieneNamespace, Name: "api-1"},
		Spec: corev1.PodSpec{
			ImagePullSecrets: []corev1.LocalObjectReference{{Name: "registry"}},
			Containers: []corev1.Container{{Name: "main", EnvFrom: []corev1.EnvFromSource{{
				SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "env-secret"}}}}}},
		},
	}
	account := &corev1.ServiceAccount{
		ObjectMeta:       metav1.ObjectMeta{Namespace: hygieneNamespace, Name: "deployer"},
		Secrets:          []corev1.ObjectReference{{Name: "account-secret"}},
		ImagePullSecrets: []corev1.LocalObjectReference{{Name: "account-pull"}},
	}
	ingress := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Namespace: hygieneNamespace, Name: "web"},
		Spec:       netv1.IngressSpec{TLS: []netv1.IngressTLS{{SecretName: "web-tls"}}},
	}
	report := hygieneRun(t, []runtime.Object{pod, account, ingress}, []runtime.Object{
		hygieneMeta("Secret", "registry"), hygieneMeta("Secret", "env-secret"),
		hygieneMeta("Secret", "account-secret"), hygieneMeta("Secret", "account-pull"),
		hygieneMeta("Secret", "web-tls"),
		hygieneMeta("Secret", "sa-token", func(object *metav1.PartialObjectMetadata) {
			object.Annotations = map[string]string{"kubernetes.io/service-account.name": "default"}
		}),
		hygieneMeta("Secret", "sh.helm.release.v1.web.v3", func(object *metav1.PartialObjectMetadata) {
			object.Labels = map[string]string{"owner": "helm"}
		}),
		hygieneMeta("Secret", "orphan"),
	})

	group := hygieneGroup(t, report, HygieneUnusedSecret)
	if names := hygieneNames(group); len(names) != 1 || names[0] != "orphan" {
		t.Fatalf("unused Secrets = %v, want only the orphan", names)
	}
}

// Secret values must never reach this scan. The metadata client is the only
// path allowed to list them; a typed list would transfer every value.
func TestHygieneNeverReadsSecretValues(t *testing.T) {
	pinChecksNow(t)
	client := kubefake.NewSimpleClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: hygieneNamespace, Name: "database"},
		Data:       map[string][]byte{"password": []byte("hunter2")},
	})
	cluster := &Cluster{Clientset: client, Meta: newOverviewMetadataClient(hygieneMeta("Secret", "database"))}
	if _, err := ClusterHygiene(context.Background(), cluster, hygieneNamespace); err != nil {
		t.Fatal(err)
	}
	for _, action := range client.Actions() {
		if action.GetResource().Resource == "secrets" {
			t.Fatalf("the typed client must not touch Secrets, got %s %s", action.GetVerb(), action.GetResource().Resource)
		}
	}
}

func TestHygieneClaimsKeptByStatefulSetAreNotUnused(t *testing.T) {
	pinChecksNow(t)
	claim := func(name string) *corev1.PersistentVolumeClaim {
		return &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Namespace: hygieneNamespace, Name: name,
				CreationTimestamp: metav1.NewTime(checksTestNow.Add(-30 * 24 * time.Hour))},
			Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound,
				Capacity: corev1.ResourceList{corev1.ResourceStorage: qty("8Gi")}},
		}
	}
	mounted := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: hygieneNamespace, Name: "api-1"},
		Spec: corev1.PodSpec{Volumes: []corev1.Volume{{Name: "d", VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "in-use"}}}}},
	}
	set := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Namespace: hygieneNamespace, Name: "db"},
		Spec: appsv1.StatefulSetSpec{VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
			{ObjectMeta: metav1.ObjectMeta{Name: "data"}}}},
	}
	report := hygieneRun(t, []runtime.Object{mounted, set, claim("in-use"), claim("data-db-0"), claim("left-behind")}, nil)

	group := hygieneGroup(t, report, HygieneUnusedClaim)
	if names := hygieneNames(group); len(names) != 1 || names[0] != "left-behind" {
		t.Fatalf("unused claims = %v; a StatefulSet's own claim outlives its Pod by design", names)
	}
	item := group.Items[0]
	if item.Severity != SeverityWarning {
		t.Fatalf("an unused claim costs storage, so it is a warning: %q", item.Severity)
	}
	if !strings.Contains(item.Detail, "reclaim policy") {
		t.Fatalf("deleting a claim can destroy data and must say so: %q", item.Detail)
	}
}

func TestHygieneServicesWithoutEndpoints(t *testing.T) {
	pinChecksNow(t)
	service := func(name string, selector map[string]string, mutate ...func(*corev1.Service)) *corev1.Service {
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Namespace: hygieneNamespace, Name: name},
			Spec:       corev1.ServiceSpec{Selector: selector, Type: corev1.ServiceTypeClusterIP},
		}
		for _, fn := range mutate {
			fn(svc)
		}
		return svc
	}
	ready := true
	notReady := false
	slice := func(service string, readyState *bool) *discoveryv1.EndpointSlice {
		return &discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{Namespace: hygieneNamespace, Name: service + "-x",
				Labels: map[string]string{discoveryv1.LabelServiceName: service}},
			Endpoints: []discoveryv1.Endpoint{{Addresses: []string{"10.0.0.1"}, Conditions: discoveryv1.EndpointConditions{Ready: readyState}}},
		}
	}
	report := hygieneRun(t, []runtime.Object{
		service("healthy", map[string]string{"app": "web"}),
		service("all-unready", map[string]string{"app": "worker"}),
		service("renamed-labels", map[string]string{"app": "gone"}),
		service("manual", nil),
		service("external", nil, func(svc *corev1.Service) {
			svc.Spec.Type = corev1.ServiceTypeExternalName
			svc.Spec.ExternalName = "db.example.com"
		}),
		slice("healthy", &ready), slice("all-unready", &notReady),
	}, nil)

	group := hygieneGroup(t, report, HygieneDeadService)
	if hygieneHas(group, "healthy") || hygieneHas(group, "external") {
		t.Fatalf("a served Service and an ExternalName must not be listed: %v", hygieneNames(group))
	}
	for _, name := range []string{"all-unready", "renamed-labels", "manual"} {
		if !hygieneHas(group, name) {
			t.Fatalf("%s should be listed: %v", name, hygieneNames(group))
		}
	}
	for _, item := range group.Items {
		if item.Name == "renamed-labels" {
			if item.Severity != SeverityWarning || !strings.Contains(item.Detail, "app=gone") {
				t.Fatalf("a broken selector must be quoted: %+v", item)
			}
		}
		if item.Name == "manual" && item.Severity != SeverityInfo {
			t.Fatalf("a Service with no selector at all is a note, not a warning: %+v", item)
		}
	}
}

func TestHygieneLeftoverWorkloadObjects(t *testing.T) {
	pinChecksNow(t)
	old := metav1.NewTime(checksTestNow.Add(-30 * 24 * time.Hour))
	recent := metav1.NewTime(checksTestNow.Add(-2 * time.Hour))
	replicas := func(n int32) *int32 { return &n }
	ttl := int32(60)

	objects := []runtime.Object{
		&appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Namespace: hygieneNamespace, Name: "api-old", CreationTimestamp: old,
			OwnerReferences: []metav1.OwnerReference{{Kind: "Deployment", Name: "api"}}},
			Spec: appsv1.ReplicaSetSpec{Replicas: replicas(0)}},
		&appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Namespace: hygieneNamespace, Name: "api-current", CreationTimestamp: old},
			Spec: appsv1.ReplicaSetSpec{Replicas: replicas(3)}, Status: appsv1.ReplicaSetStatus{Replicas: 3}},
		&appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Namespace: hygieneNamespace, Name: "api-yesterday", CreationTimestamp: recent},
			Spec: appsv1.ReplicaSetSpec{Replicas: replicas(0)}},

		&batchv1.Job{ObjectMeta: metav1.ObjectMeta{Namespace: hygieneNamespace, Name: "migrate", CreationTimestamp: old},
			Status: batchv1.JobStatus{CompletionTime: &old, Conditions: []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}}}},
		&batchv1.Job{ObjectMeta: metav1.ObjectMeta{Namespace: hygieneNamespace, Name: "self-cleaning", CreationTimestamp: old},
			Spec:   batchv1.JobSpec{TTLSecondsAfterFinished: &ttl},
			Status: batchv1.JobStatus{CompletionTime: &old, Conditions: []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}}}},
		&batchv1.Job{ObjectMeta: metav1.ObjectMeta{Namespace: hygieneNamespace, Name: "cron-run-1", CreationTimestamp: old,
			OwnerReferences: []metav1.OwnerReference{{Kind: "CronJob", Name: "nightly"}}},
			Status: batchv1.JobStatus{CompletionTime: &old, Conditions: []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}}}},
		&batchv1.Job{ObjectMeta: metav1.ObjectMeta{Namespace: hygieneNamespace, Name: "running", CreationTimestamp: old},
			Status: batchv1.JobStatus{Active: 1}},

		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: hygieneNamespace, Name: "done", CreationTimestamp: old},
			Status: corev1.PodStatus{Phase: corev1.PodSucceeded}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: hygieneNamespace, Name: "evicted", CreationTimestamp: old},
			Status: corev1.PodStatus{Phase: corev1.PodFailed, Reason: "Evicted"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: hygieneNamespace, Name: "finished-today", CreationTimestamp: recent},
			Status: corev1.PodStatus{Phase: corev1.PodSucceeded}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: hygieneNamespace, Name: "serving", CreationTimestamp: old},
			Status: corev1.PodStatus{Phase: corev1.PodRunning}},
	}
	report := hygieneRun(t, objects, nil)

	if names := hygieneNames(hygieneGroup(t, report, HygieneOldReplicaSet)); len(names) != 1 || names[0] != "api-old" {
		t.Fatalf("old ReplicaSets = %v", names)
	}
	if names := hygieneNames(hygieneGroup(t, report, HygieneFinishedJob)); len(names) != 1 || names[0] != "migrate" {
		t.Fatalf("finished Jobs = %v; TTL and CronJob-owned Jobs clean themselves up", names)
	}
	pods := hygieneGroup(t, report, HygieneFinishedPod)
	if names := hygieneNames(pods); len(names) != 2 {
		t.Fatalf("finished Pods = %v", names)
	}
	for _, item := range pods.Items {
		// An eviction is evidence of node pressure, so it ranks above a Pod
		// that simply completed.
		if item.Name == "evicted" && item.Severity != SeverityWarning {
			t.Fatalf("evicted Pod severity = %q", item.Severity)
		}
		if item.Name == "done" && item.Severity != SeverityInfo {
			t.Fatalf("completed Pod severity = %q", item.Severity)
		}
	}
	if pods.Items[0].Name != "evicted" {
		t.Fatalf("items are not ordered by severity: %v", hygieneNames(pods))
	}
}

func TestHygieneUnpinnedImages(t *testing.T) {
	pinChecksNow(t)
	deployment := func(name, image string) *appsv1.Deployment {
		return &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Namespace: hygieneNamespace, Name: name},
			Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "main", Image: image}}}}},
		}
	}
	report := hygieneRun(t, []runtime.Object{
		deployment("floating", "registry.example.com:5000/team/api:latest"),
		deployment("untagged", "nginx"),
		deployment("pinned", "nginx@sha256:0000000000000000000000000000000000000000000000000000000000000000"),
		deployment("released", "registry.example.com:5000/team/api:1.4.2"),
	}, nil)

	group := hygieneGroup(t, report, HygieneUnpinnedImage)
	if names := hygieneNames(group); len(names) != 2 {
		t.Fatalf("unpinned images = %v, want the :latest and the untagged one", names)
	}
	if !hygieneHas(group, "floating") || !hygieneHas(group, "untagged") {
		t.Fatalf("wrong workloads reported: %v", hygieneNames(group))
	}
	for _, item := range group.Items {
		if item.Command != "" {
			t.Fatalf("an unpinned image is fixed by editing the workload, not by deleting it: %q", item.Command)
		}
		if len(item.Chips) == 0 || !strings.Contains(item.Chips[0], "main:") {
			t.Fatalf("the offending container and image must be named: %+v", item.Chips)
		}
	}
}

func TestImageRiskRecognisesRegistryPorts(t *testing.T) {
	cases := map[string]bool{
		"nginx":                           true,
		"nginx:latest":                    true,
		"nginx:1.27":                      false,
		"registry:5000/app":               true,
		"registry:5000/app:1.0":           false,
		"registry:5000/app:latest":        true,
		"nginx@sha256:abc":                false,
		"registry:5000/app@sha256:abcdef": false,
	}
	for image, want := range cases {
		if _, risky := imageRisk(image); risky != want {
			t.Fatalf("imageRisk(%q) = %v, want %v", image, risky, want)
		}
	}
}

// One failed list must cost only its own group. A token that cannot read
// EndpointSlices still gets every other answer.
func TestHygieneOneFailedListDoesNotBlankTheReport(t *testing.T) {
	pinChecksNow(t)
	report := hygieneRun(t,
		[]runtime.Object{
			&corev1.Service{ObjectMeta: metav1.ObjectMeta{Namespace: hygieneNamespace, Name: "web"},
				Spec: corev1.ServiceSpec{Selector: map[string]string{"app": "web"}}},
			&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: hygieneNamespace, Name: "done",
				CreationTimestamp: metav1.NewTime(checksTestNow.Add(-72 * time.Hour))},
				Status: corev1.PodStatus{Phase: corev1.PodSucceeded}},
		}, nil,
		func(client *kubefake.Clientset) {
			client.PrependReactor("list", "endpointslices", func(clienttesting.Action) (bool, runtime.Object, error) {
				return true, nil, errors.New("endpointslices is forbidden")
			})
		})

	services := hygieneGroup(t, report, HygieneDeadService)
	if services.Warning == "" || len(services.Items) != 0 {
		t.Fatalf("the Services group should report why it is empty: %+v", services)
	}
	if !strings.Contains(strings.Join(report.Warnings, " "), "EndpointSlices") {
		t.Fatalf("report warnings = %v", report.Warnings)
	}
	if len(hygieneGroup(t, report, HygieneFinishedPod).Items) != 1 {
		t.Fatalf("an unrelated group lost its results")
	}
}

// The scan lists a dozen resource types, so the live refresh must not repeat it
// every five seconds.
func TestHygieneCachesItsScanPerScope(t *testing.T) {
	pinChecksNow(t)
	client := kubefake.NewSimpleClientset()
	cluster := &Cluster{Clientset: client, Meta: newOverviewMetadataClient()}
	if _, err := ClusterHygiene(context.Background(), cluster, hygieneNamespace); err != nil {
		t.Fatal(err)
	}
	first := len(client.Actions())
	if first == 0 {
		t.Fatal("the first scan made no requests")
	}
	if _, err := ClusterHygiene(context.Background(), cluster, hygieneNamespace); err != nil {
		t.Fatal(err)
	}
	if len(client.Actions()) != first {
		t.Fatalf("the second scan repeated the lists: %d → %d requests", first, len(client.Actions()))
	}
	// Another scope is a different question and must be scanned.
	if _, err := ClusterHygiene(context.Background(), cluster, ""); err != nil {
		t.Fatal(err)
	}
	if len(client.Actions()) == first {
		t.Fatal("a different namespace scope reused the cached report")
	}
}
