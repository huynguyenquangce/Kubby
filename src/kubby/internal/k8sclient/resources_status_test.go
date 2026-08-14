package k8sclient

import (
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestPodStatusHandlesInitAndTerminationWithoutFalseErrors(t *testing.T) {
	deleting := metav1.NewTime(time.Now())
	tests := []struct {
		name      string
		pod       corev1.Pod
		want      string
		wantError bool
		restarts  int32
	}{
		{name: "ordinary init wait", pod: corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodPending, InitContainerStatuses: []corev1.ContainerStatus{{RestartCount: 2, State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "PodInitializing"}}}}}}, want: "Init:PodInitializing", restarts: 2},
		{name: "failed init", pod: corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodPending, InitContainerStatuses: []corev1.ContainerStatus{{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}}}}}, want: "Init:CrashLoopBackOff", wantError: true},
		{name: "terminating wins", pod: corev1.Pod{ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &deleting}, Status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}}}}}, want: "Terminating"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status, restarts, _ := podStatus(tc.pod)
			if status != tc.want || restarts != tc.restarts || isErroredPodStatus(status) != tc.wantError {
				t.Fatalf("status=%q restarts=%d error=%v; want %q, %d, %v", status, restarts, isErroredPodStatus(status), tc.want, tc.restarts, tc.wantError)
			}
		})
	}
}

func TestOwnedByRejectsRecreatedOwnerWithSameName(t *testing.T) {
	refs := []metav1.OwnerReference{{Kind: "ReplicaSet", Name: "api-rs", UID: types.UID("old")}}
	if ownedBy(refs, "ReplicaSet", "api-rs", types.UID("new")) {
		t.Fatal("same-name owner with a different UID must not be linked")
	}
	if !ownedBy(refs, "ReplicaSet", "api-rs", types.UID("old")) {
		t.Fatal("matching owner UID should be linked")
	}
}

func TestJobErrorUsesTerminalConditionsNotHistoricalFailures(t *testing.T) {
	complete := &batchv1.Job{Status: batchv1.JobStatus{Failed: 3, Conditions: []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}}}}
	failed := &batchv1.Job{Status: batchv1.JobStatus{Conditions: []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue}}}}
	if jobIsError(complete) {
		t.Fatal("a completed Job must not remain errored because an earlier retry failed")
	}
	if !jobIsError(failed) {
		t.Fatal("a Job with a true Failed condition must be errored")
	}
}
