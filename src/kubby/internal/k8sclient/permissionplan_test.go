package k8sclient

import (
	"context"
	"fmt"
	"testing"

	authv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestPermissionPlanExplicitDenyUnknownAndDeduplication(t *testing.T) {
	client := fake.NewSimpleClientset()
	client.PrependReactor("create", "selfsubjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		review := action.(k8stesting.CreateAction).GetObject().(*authv1.SelfSubjectAccessReview)
		switch review.Spec.ResourceAttributes.Verb {
		case "patch":
			return true, &authv1.SelfSubjectAccessReview{Status: authv1.SubjectAccessReviewStatus{Allowed: false}}, nil
		case "list":
			return true, nil, fmt.Errorf("temporary authorization endpoint failure")
		default:
			return true, &authv1.SelfSubjectAccessReview{Status: authv1.SubjectAccessReviewStatus{Allowed: true}}, nil
		}
	})
	c := &Cluster{Clientset: client}
	pods := APIKind{Kind: "Pod", Namespaced: true, GVR: schema.GroupVersionResource{Resource: "pods", Version: "v1"}}

	plan := runPermissionPlan(context.Background(), c, "test", []permissionSpec{
		{ak: pods, namespace: "team-a", verb: "get"},
		{ak: pods, namespace: "team-a", verb: "get"}, // duplicate must collapse
		{ak: pods, namespace: "team-a", verb: "patch"},
		{ak: pods, namespace: "team-a", verb: "list"},
	}, nil)

	if len(plan.Requirements) != 3 {
		t.Fatalf("requirements = %d, want 3", len(plan.Requirements))
	}
	if plan.Denied != 1 || plan.Unknown != 1 || plan.Permitted {
		t.Fatalf("plan = denied %d unknown %d permitted %v", plan.Denied, plan.Unknown, plan.Permitted)
	}
	for _, requirement := range plan.Requirements {
		if requirement.Verb == "list" && (!requirement.Allowed || requirement.Checked) {
			t.Fatalf("unknown list probe must remain allowed: %#v", requirement)
		}
	}
}

func TestResourcePageOptionsClampAndPreserveToken(t *testing.T) {
	defaulted := resourcePageOptions("next", 0)
	if defaulted.Continue != "next" || defaulted.Limit != defaultResourcePageLimit {
		t.Fatalf("default options = %#v", defaulted)
	}
	clamped := resourcePageOptions("", maxResourcePageLimit+1)
	if clamped.Limit != maxResourcePageLimit {
		t.Fatalf("clamped limit = %d, want %d", clamped.Limit, maxResourcePageLimit)
	}
}

func TestPlanHelmRollbackRejectsNonPositiveRevision(t *testing.T) {
	if _, err := PlanHelmAction(context.Background(), nil, "rollback", "default", "demo", 0); err == nil {
		t.Fatal("expected rollback revision validation error")
	}
}
