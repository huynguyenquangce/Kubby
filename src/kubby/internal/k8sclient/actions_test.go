package k8sclient

import (
	"context"
	"errors"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	kubefake "k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

func TestRunCronJobNowUsesServerGeneratedUniqueName(t *testing.T) {
	name := strings.Repeat("long-cronjob-", 8)
	client := kubefake.NewSimpleClientset(&batchv1.CronJob{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "jobs"}})
	var created *batchv1.Job
	client.PrependReactor("create", "jobs", func(action clienttesting.Action) (bool, runtime.Object, error) {
		created = action.(clienttesting.CreateAction).GetObject().(*batchv1.Job)
		return true, created, nil
	})
	if err := RunCronJobNow(context.Background(), &Cluster{Clientset: client}, "jobs", name); err != nil {
		t.Fatal(err)
	}
	if created == nil || created.Name != "" || created.GenerateName == "" || len(created.GenerateName) > 58 || !strings.HasSuffix(created.GenerateName, "-") {
		t.Fatalf("created Job metadata = %#v", created.ObjectMeta)
	}
}

func TestDrainNodeReportsPartialEvictionFailures(t *testing.T) {
	client := kubefake.NewSimpleClientset(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "blocked", Namespace: "apps"}, Spec: corev1.PodSpec{NodeName: "worker"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "evicted", Namespace: "apps"}, Spec: corev1.PodSpec{NodeName: "worker"}},
	)
	client.PrependReactor("create", "pods", func(action clienttesting.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() == "eviction" && action.(clienttesting.CreateAction).GetObject().(*policyv1.Eviction).Name == "blocked" {
			return true, nil, errors.New("pdb denied")
		}
		return false, nil, nil
	})
	err := DrainNode(context.Background(), &Cluster{Clientset: client}, "worker")
	if err == nil || !strings.Contains(err.Error(), "apps/blocked") || !strings.Contains(err.Error(), "pdb denied") {
		t.Fatalf("partial drain error = %v", err)
	}
}

func TestRollbackRetriesControllerConflictWithoutTouchingReplacement(t *testing.T) {
	for _, replacement := range []bool{false, true} {
		t.Run(map[bool]string{false: "controller conflict", true: "replacement UID"}[replacement], func(t *testing.T) {
			controller := true
			uid := types.UID("original")
			dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "apps", UID: uid, ResourceVersion: "1"}, Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "new"}}}}}}
			rs := &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "app-old", Namespace: "apps", OwnerReferences: []metav1.OwnerReference{{Kind: "Deployment", Name: "app", UID: uid, Controller: &controller}}, Annotations: map[string]string{"deployment.kubernetes.io/revision": "1"}}, Spec: appsv1.ReplicaSetSpec{Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "app", "pod-template-hash": "old"}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "old"}}}}}}
			client := kubefake.NewSimpleClientset(dep, rs)
			updates := 0
			client.PrependReactor("update", "deployments", func(action clienttesting.Action) (bool, runtime.Object, error) {
				updates++
				if updates > 1 {
					return false, nil, nil
				}
				obj, err := client.Tracker().Get(appsv1.SchemeGroupVersion.WithResource("deployments"), "apps", "app")
				if err != nil {
					return true, nil, err
				}
				current := obj.(*appsv1.Deployment).DeepCopy()
				current.ResourceVersion = "2"
				current.Annotations = map[string]string{"controller": "preserved"}
				if replacement {
					current.UID = "replacement"
				}
				if err := client.Tracker().Update(appsv1.SchemeGroupVersion.WithResource("deployments"), current, "apps"); err != nil {
					return true, nil, err
				}
				return true, nil, apierrors.NewConflict(schema.GroupResource{Group: "apps", Resource: "deployments"}, "app", errors.New("synthetic controller update"))
			})
			err := RollbackDeployment(context.Background(), &Cluster{Clientset: client}, "apps", "app", 1)
			current, getErr := client.AppsV1().Deployments("apps").Get(context.Background(), "app", metav1.GetOptions{})
			if getErr != nil {
				t.Fatal(getErr)
			}
			if replacement {
				if err == nil || !strings.Contains(err.Error(), "deployment changed") || updates != 1 || current.Spec.Template.Spec.Containers[0].Image != "new" {
					t.Fatal("replacement deployment was not protected")
				}
			} else {
				if err != nil || updates != 2 || current.Spec.Template.Spec.Containers[0].Image != "old" || current.Annotations["controller"] != "preserved" || current.Spec.Template.Labels["pod-template-hash"] != "" {
					t.Fatal("rollback did not retry safely:", err)
				}
			}
		})
	}
}
