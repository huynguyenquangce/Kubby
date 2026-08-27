package k8sclient

import (
	"context"
	"testing"

	authv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

func gvr(group, version, resource string) schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: group, Version: version, Resource: resource}
}

// The permission gate fails safe in one direction only: when Kubby does not know,
// it must allow. Getting this backwards disables the entire UI on any cluster that
// cannot answer a SelfSubjectAccessReview, and it does so silently — which is why
// it is the first thing tested here.
func TestUnknownPermissionMeansAllowed(t *testing.T) {
	cases := []struct {
		name string
		set  *AccessSet
		verb string
	}{
		{"no answer at all", nil, "delete"},
		{"answered but with no verbs", &AccessSet{}, "delete"},
		{"answered, this verb not among them", &AccessSet{Verbs: map[string]bool{"get": true}}, "exec"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.set.Allowed(tc.verb) {
				t.Fatalf("Allowed(%q) = false; an unknown answer must not disable the action", tc.verb)
			}
		})
	}
}

func TestExplicitDenyIsRespected(t *testing.T) {
	set := &AccessSet{Verbs: map[string]bool{"delete": false, "get": true}}
	if set.Allowed("delete") {
		t.Fatal(`Allowed("delete") = true; an explicit false must disable the action`)
	}
	if !set.Allowed("get") {
		t.Fatal(`Allowed("get") = false; an explicit true must permit it`)
	}
}

// Pod subresources are governed separately from the Pod, so they must be probed
// separately. A role granting `get pods` says nothing about reading logs, and the
// UI's Logs and Terminal tabs depend on that distinction being asked about.
func TestPodProbesCoverTheSubresources(t *testing.T) {
	probes := probesFor(APIKind{GVR: gvr("", "v1", "pods")})

	want := map[string]struct{ verb, sub string }{
		"logs":        {"get", "log"},
		"exec":        {"create", "exec"},
		"portforward": {"create", "portforward"},
	}
	got := map[string]accessProbe{}
	for _, p := range probes {
		got[p.key] = p
	}
	for key, w := range want {
		p, ok := got[key]
		if !ok {
			t.Fatalf("no probe for %q", key)
		}
		if p.verb != w.verb || p.subresource != w.sub {
			t.Errorf("probe %q = verb %q subresource %q; want verb %q subresource %q",
				key, p.verb, p.subresource, w.verb, w.sub)
		}
	}
}

func TestDeploymentProbeCoversScaleSubresource(t *testing.T) {
	probes := probesFor(APIKind{GVR: gvr("apps", "v1", "deployments")})
	for _, probe := range probes {
		if probe.key == "scale" {
			if probe.verb != "update" || probe.subresource != "scale" {
				t.Fatalf("scale probe = verb %q subresource %q", probe.verb, probe.subresource)
			}
			return
		}
	}
	t.Fatal("Deployment probes do not include deployments/scale")
}

func TestCanIAddressesTheExactSubresource(t *testing.T) {
	client := fake.NewSimpleClientset()
	var got authv1.ResourceAttributes
	client.PrependReactor("create", "selfsubjectaccessreviews", func(action clienttesting.Action) (bool, runtime.Object, error) {
		review := action.(clienttesting.CreateAction).GetObject().(*authv1.SelfSubjectAccessReview)
		got = *review.Spec.ResourceAttributes
		return true, &authv1.SelfSubjectAccessReview{Status: authv1.SubjectAccessReviewStatus{Allowed: true}}, nil
	})
	allowed, err := (&Cluster{Clientset: client}).canI(context.Background(), "apps", "deployments", "scale", "update", "team-a")
	if err != nil || !allowed {
		t.Fatalf("canI = %v, %v", allowed, err)
	}
	want := authv1.ResourceAttributes{Namespace: "team-a", Group: "apps", Resource: "deployments", Subresource: "scale", Verb: "update"}
	if got != want {
		t.Fatalf("resource attributes = %#v, want %#v", got, want)
	}
}

func TestKindsWithoutSpecialActionsGetNoSubresourceProbes(t *testing.T) {
	for _, ak := range []APIKind{
		{GVR: gvr("networking.istio.io", "v1beta1", "virtualservices")},
		// Same resource name, different group — must not be mistaken for core pods.
		{GVR: gvr("metrics.k8s.io", "v1beta1", "pods")},
	} {
		for _, p := range probesFor(ak) {
			if p.subresource != "" {
				t.Errorf("%s: unexpected subresource probe %q", ak.GVR, p.key)
			}
		}
	}
}
