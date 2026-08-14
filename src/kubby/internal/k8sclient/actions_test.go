package k8sclient

import (
	"context"
	"errors"
	"strings"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
