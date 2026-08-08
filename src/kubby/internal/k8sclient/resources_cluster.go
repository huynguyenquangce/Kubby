package k8sclient

import (
	"context"
	"fmt"
	"strings"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type PersistentVolumeInfo struct {
	Name         string `json:"name"`
	Capacity     string `json:"capacity"`
	AccessModes  string `json:"accessModes"`
	Status       string `json:"status"`
	Claim        string `json:"claim"`
	StorageClass string `json:"storageClass"`
	IsError      bool   `json:"isError"`
	Age          string `json:"age"`
}

type StorageClassInfo struct {
	Name          string `json:"name"`
	Provisioner   string `json:"provisioner"`
	ReclaimPolicy string `json:"reclaimPolicy"`
	IsDefault     bool   `json:"isDefault"`
	Age           string `json:"age"`
}

type RoleInfo struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Rules     int    `json:"rules"`
	Age       string `json:"age"`
}

type RoleBindingInfo struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	RoleRef   string `json:"roleRef"`
	Subjects  string `json:"subjects"`
	Age       string `json:"age"`
}

type ClusterRoleInfo struct {
	Name  string `json:"name"`
	Rules int    `json:"rules"`
	Age   string `json:"age"`
}

type ClusterRoleBindingInfo struct {
	Name     string `json:"name"`
	RoleRef  string `json:"roleRef"`
	Subjects string `json:"subjects"`
	Age      string `json:"age"`
}

func ListPersistentVolumes(ctx context.Context, client kubernetes.Interface) ([]PersistentVolumeInfo, error) {
	list, err := client.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]PersistentVolumeInfo, 0, len(list.Items))
	for _, pv := range list.Items {
		capacity := ""
		if q, ok := pv.Spec.Capacity["storage"]; ok {
			capacity = q.String()
		}
		modes := make([]string, 0, len(pv.Spec.AccessModes))
		for _, m := range pv.Spec.AccessModes {
			modes = append(modes, string(m))
		}
		claim := ""
		if pv.Spec.ClaimRef != nil {
			claim = fmt.Sprintf("%s/%s", pv.Spec.ClaimRef.Namespace, pv.Spec.ClaimRef.Name)
		}
		out = append(out, PersistentVolumeInfo{
			Name: pv.Name, Capacity: capacity, AccessModes: strings.Join(modes, ", "),
			Status: string(pv.Status.Phase), Claim: claim, StorageClass: pv.Spec.StorageClassName,
			IsError: pv.Status.Phase == "Failed",
			Age:     age(pv.CreationTimestamp),
		})
	}
	return out, nil
}

func ListStorageClasses(ctx context.Context, client kubernetes.Interface) ([]StorageClassInfo, error) {
	list, err := client.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]StorageClassInfo, 0, len(list.Items))
	for _, sc := range list.Items {
		reclaim := ""
		if sc.ReclaimPolicy != nil {
			reclaim = string(*sc.ReclaimPolicy)
		}
		out = append(out, StorageClassInfo{
			Name: sc.Name, Provisioner: sc.Provisioner, ReclaimPolicy: reclaim,
			IsDefault: sc.Annotations["storageclass.kubernetes.io/is-default-class"] == "true",
			Age:       age(sc.CreationTimestamp),
		})
	}
	return out, nil
}

func ListRoles(ctx context.Context, client kubernetes.Interface, ns string) ([]RoleInfo, error) {
	list, err := client.RbacV1().Roles(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]RoleInfo, 0, len(list.Items))
	for _, r := range list.Items {
		out = append(out, RoleInfo{Namespace: r.Namespace, Name: r.Name, Rules: len(r.Rules), Age: age(r.CreationTimestamp)})
	}
	return out, nil
}

func ListRoleBindings(ctx context.Context, client kubernetes.Interface, ns string) ([]RoleBindingInfo, error) {
	list, err := client.RbacV1().RoleBindings(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]RoleBindingInfo, 0, len(list.Items))
	for _, rb := range list.Items {
		out = append(out, RoleBindingInfo{
			Namespace: rb.Namespace, Name: rb.Name,
			RoleRef:  fmt.Sprintf("%s/%s", rb.RoleRef.Kind, rb.RoleRef.Name),
			Subjects: subjectsToString(rb.Subjects),
			Age:      age(rb.CreationTimestamp),
		})
	}
	return out, nil
}

func ListClusterRoles(ctx context.Context, client kubernetes.Interface) ([]ClusterRoleInfo, error) {
	list, err := client.RbacV1().ClusterRoles().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]ClusterRoleInfo, 0, len(list.Items))
	for _, r := range list.Items {
		out = append(out, ClusterRoleInfo{Name: r.Name, Rules: len(r.Rules), Age: age(r.CreationTimestamp)})
	}
	return out, nil
}

func ListClusterRoleBindings(ctx context.Context, client kubernetes.Interface) ([]ClusterRoleBindingInfo, error) {
	list, err := client.RbacV1().ClusterRoleBindings().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]ClusterRoleBindingInfo, 0, len(list.Items))
	for _, rb := range list.Items {
		out = append(out, ClusterRoleBindingInfo{
			Name:     rb.Name,
			RoleRef:  fmt.Sprintf("%s/%s", rb.RoleRef.Kind, rb.RoleRef.Name),
			Subjects: subjectsToString(rb.Subjects),
			Age:      age(rb.CreationTimestamp),
		})
	}
	return out, nil
}

func subjectsToString(subjects []rbacv1.Subject) string {
	parts := make([]string, 0, len(subjects))
	for _, s := range subjects {
		parts = append(parts, fmt.Sprintf("%s:%s", s.Kind, s.Name))
	}
	if len(parts) == 0 {
		return "<none>"
	}
	return strings.Join(parts, ", ")
}
