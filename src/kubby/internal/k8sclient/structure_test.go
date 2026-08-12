package k8sclient

import (
	"context"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func TestClusterStructureBuildsOwnerChainsAndUnexposedWorkloads(t *testing.T) {
	controller := true
	pathType := netv1.PathTypePrefix
	deleting := metav1.NewTime(time.Now())
	client := kubefake.NewSimpleClientset(
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "shop"}, Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "web"}, Ports: []corev1.ServicePort{{Port: 80, TargetPort: intstr.FromInt32(8080)}},
		}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "shop"}, Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "db"}, Ports: []corev1.ServicePort{{Port: 5432}},
		}},
		&netv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: "shop", Namespace: "shop"}, Spec: netv1.IngressSpec{
			Rules: []netv1.IngressRule{{IngressRuleValue: netv1.IngressRuleValue{HTTP: &netv1.HTTPIngressRuleValue{Paths: []netv1.HTTPIngressPath{{
				Path: "/", PathType: &pathType, Backend: netv1.IngressBackend{Service: &netv1.IngressServiceBackend{Name: "web", Port: netv1.ServiceBackendPort{Number: 80}}},
			}}}}}},
		}},
		&appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "web-7d9", Namespace: "shop", OwnerReferences: []metav1.OwnerReference{{Kind: "Deployment", Name: "web", Controller: &controller}}}},
		structureTestPod("shop", "web-7d9-a", map[string]string{"app": "web"}, "ReplicaSet", "web-7d9", true, nil),
		structureTestPod("shop", "web-7d9-b", map[string]string{"app": "web"}, "ReplicaSet", "web-7d9", false, nil),
		structureTestPod("shop", "db-0", map[string]string{"app": "db"}, "StatefulSet", "db", true, nil),
		structureTestPod("shop", "worker", map[string]string{"app": "worker"}, "Job", "worker", true, nil),
		structureTestPod("shop", "terminating", map[string]string{"app": "web"}, "ReplicaSet", "web-7d9", false, &deleting),
	)
	meta := newOverviewMetadataClient(
		partialMetadata("v1", "Node", "", "node-a"),
		partialMetadata("v1", "Namespace", "", "shop"),
	)

	got, err := ClusterStructure(context.Background(), &Cluster{Clientset: client, Meta: meta}, "shop")
	if err != nil {
		t.Fatal(err)
	}
	if got.Summary.Nodes != 1 || got.Summary.Namespaces != 1 || got.Summary.Pods != 4 || got.Summary.Unhealthy != 1 {
		t.Fatalf("summary = %#v", got.Summary)
	}
	if len(got.Entries) != 1 || len(got.Entries[0].Services) != 1 {
		t.Fatalf("entries = %#v", got.Entries)
	}
	web := got.Entries[0].Services[0]
	if len(web.Workloads) != 1 || web.Workloads[0].Kind != "Deployment" || web.Workloads[0].Name != "web" || !web.Workloads[0].IsError {
		t.Fatalf("web workload = %#v", web.Workloads)
	}
	if len(got.Internal) != 1 || got.Internal[0].Workloads[0].Kind != "StatefulSet" {
		t.Fatalf("internal = %#v", got.Internal)
	}
	if len(got.Unexposed) != 1 || got.Unexposed[0].Name != "worker" {
		t.Fatalf("unexposed = %#v", got.Unexposed)
	}
	assertOneList(t, client.Actions(), "services")
	assertOneList(t, client.Actions(), "pods")
	assertOneList(t, client.Actions(), "ingresses")
	assertOneList(t, client.Actions(), "replicasets")
}

func structureTestPod(namespace, name string, labels map[string]string, ownerKind, ownerName string, ready bool, deletion *metav1.Time) *corev1.Pod {
	controller := true
	state := corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}
	phase := corev1.PodRunning
	if !ready {
		state = corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: namespace, Labels: labels, DeletionTimestamp: deletion,
			OwnerReferences: []metav1.OwnerReference{{Kind: ownerKind, Name: ownerName, Controller: &controller}},
		},
		Spec: corev1.PodSpec{NodeName: "node-a"},
		Status: corev1.PodStatus{Phase: phase, ContainerStatuses: []corev1.ContainerStatus{{
			Name: "app", Ready: ready, RestartCount: map[bool]int32{true: 0, false: 3}[ready], State: state,
		}}},
	}
}
