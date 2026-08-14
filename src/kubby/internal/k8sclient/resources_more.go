package k8sclient

import (
	"context"
	"fmt"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type StatefulSetInfo struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Ready     string `json:"ready"`
	IsError   bool   `json:"isError"`
	Age       string `json:"age"`
}

type DaemonSetInfo struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Desired   int32  `json:"desired"`
	Ready     int32  `json:"ready"`
	Available int32  `json:"available"`
	IsError   bool   `json:"isError"`
	Age       string `json:"age"`
}

type JobInfo struct {
	Namespace   string `json:"namespace"`
	Name        string `json:"name"`
	Completions string `json:"completions"`
	Succeeded   int32  `json:"succeeded"`
	Active      int32  `json:"active"`
	IsError     bool   `json:"isError"`
	Age         string `json:"age"`
}

type CronJobInfo struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Schedule  string `json:"schedule"`
	Suspend   bool   `json:"suspend"`
	Active    int    `json:"active"`
	Age       string `json:"age"`
}

type IngressInfo struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Class     string `json:"class"`
	Hosts     string `json:"hosts"`
	Age       string `json:"age"`
}

type PVCInfo struct {
	Namespace    string `json:"namespace"`
	Name         string `json:"name"`
	Status       string `json:"status"`
	Capacity     string `json:"capacity"`
	StorageClass string `json:"storageClass"`
	IsError      bool   `json:"isError"`
	Age          string `json:"age"`
}

type ServiceAccountInfo struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Secrets   int    `json:"secrets"`
	Age       string `json:"age"`
}

func ListStatefulSets(ctx context.Context, client kubernetes.Interface, ns string) ([]StatefulSetInfo, error) {
	list, err := client.AppsV1().StatefulSets(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]StatefulSetInfo, 0, len(list.Items))
	for _, s := range list.Items {
		desired := int32(0)
		if s.Spec.Replicas != nil {
			desired = *s.Spec.Replicas
		}
		out = append(out, StatefulSetInfo{
			Namespace: s.Namespace, Name: s.Name,
			Ready:   fmt.Sprintf("%d/%d", s.Status.ReadyReplicas, desired),
			IsError: s.Status.ReadyReplicas < desired,
			Age:     age(s.CreationTimestamp),
		})
	}
	return out, nil
}

func ListDaemonSets(ctx context.Context, client kubernetes.Interface, ns string) ([]DaemonSetInfo, error) {
	list, err := client.AppsV1().DaemonSets(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]DaemonSetInfo, 0, len(list.Items))
	for _, d := range list.Items {
		out = append(out, DaemonSetInfo{
			Namespace: d.Namespace, Name: d.Name,
			Desired:   d.Status.DesiredNumberScheduled,
			Ready:     d.Status.NumberReady,
			Available: d.Status.NumberAvailable,
			IsError:   d.Status.NumberReady < d.Status.DesiredNumberScheduled,
			Age:       age(d.CreationTimestamp),
		})
	}
	return out, nil
}

func ListJobs(ctx context.Context, client kubernetes.Interface, ns string) ([]JobInfo, error) {
	list, err := client.BatchV1().Jobs(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]JobInfo, 0, len(list.Items))
	for _, j := range list.Items {
		comp := "1"
		if j.Spec.Completions != nil {
			comp = fmt.Sprintf("%d", *j.Spec.Completions)
		}
		out = append(out, JobInfo{
			Namespace: j.Namespace, Name: j.Name,
			Completions: fmt.Sprintf("%d/%s", j.Status.Succeeded, comp),
			Succeeded:   j.Status.Succeeded,
			Active:      j.Status.Active,
			IsError:     jobIsError(&j),
			Age:         age(j.CreationTimestamp),
		})
	}
	return out, nil
}

func jobIsError(job *batchv1.Job) bool {
	for _, condition := range job.Status.Conditions {
		if condition.Status != corev1.ConditionTrue {
			continue
		}
		switch condition.Type {
		case batchv1.JobComplete:
			return false
		case batchv1.JobFailed:
			return true
		}
	}
	return false
}

func ListCronJobs(ctx context.Context, client kubernetes.Interface, ns string) ([]CronJobInfo, error) {
	list, err := client.BatchV1().CronJobs(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]CronJobInfo, 0, len(list.Items))
	for _, cj := range list.Items {
		suspend := false
		if cj.Spec.Suspend != nil {
			suspend = *cj.Spec.Suspend
		}
		out = append(out, CronJobInfo{
			Namespace: cj.Namespace, Name: cj.Name,
			Schedule: cj.Spec.Schedule, Suspend: suspend, Active: len(cj.Status.Active),
			Age: age(cj.CreationTimestamp),
		})
	}
	return out, nil
}

func ListIngresses(ctx context.Context, client kubernetes.Interface, ns string) ([]IngressInfo, error) {
	list, err := client.NetworkingV1().Ingresses(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]IngressInfo, 0, len(list.Items))
	for _, ing := range list.Items {
		class := ""
		if ing.Spec.IngressClassName != nil {
			class = *ing.Spec.IngressClassName
		}
		hosts := []string{}
		for _, r := range ing.Spec.Rules {
			if r.Host != "" {
				hosts = append(hosts, r.Host)
			}
		}
		out = append(out, IngressInfo{
			Namespace: ing.Namespace, Name: ing.Name, Class: class, Hosts: strings.Join(hosts, ", "),
			Age: age(ing.CreationTimestamp),
		})
	}
	return out, nil
}

func ListPVCs(ctx context.Context, client kubernetes.Interface, ns string) ([]PVCInfo, error) {
	list, err := client.CoreV1().PersistentVolumeClaims(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]PVCInfo, 0, len(list.Items))
	for _, p := range list.Items {
		sc := ""
		if p.Spec.StorageClassName != nil {
			sc = *p.Spec.StorageClassName
		}
		capacity := ""
		if q, ok := p.Status.Capacity["storage"]; ok {
			capacity = q.String()
		}
		out = append(out, PVCInfo{
			Namespace: p.Namespace, Name: p.Name,
			Status: string(p.Status.Phase), Capacity: capacity, StorageClass: sc,
			IsError: p.Status.Phase != "Bound",
			Age:     age(p.CreationTimestamp),
		})
	}
	return out, nil
}

func ListServiceAccounts(ctx context.Context, client kubernetes.Interface, ns string) ([]ServiceAccountInfo, error) {
	list, err := client.CoreV1().ServiceAccounts(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]ServiceAccountInfo, 0, len(list.Items))
	for _, sa := range list.Items {
		out = append(out, ServiceAccountInfo{
			Namespace: sa.Namespace, Name: sa.Name, Secrets: len(sa.Secrets),
			Age: age(sa.CreationTimestamp),
		})
	}
	return out, nil
}
