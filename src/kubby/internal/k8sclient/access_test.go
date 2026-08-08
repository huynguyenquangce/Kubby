package k8sclient

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
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

func TestNonPodKindsGetNoSubresourceProbes(t *testing.T) {
	for _, ak := range []APIKind{
		{GVR: gvr("apps", "v1", "deployments")},
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
