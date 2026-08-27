package k8sclient

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery/fake"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestCustomResourceDiscoveryAndDynamicListing(t *testing.T) {
	widgetGVR := schema.GroupVersionResource{Group: "example.test", Version: "v1", Resource: "widgets"}
	discovery := &fake.FakeDiscovery{Fake: &k8stesting.Fake{Resources: []*metav1.APIResourceList{
		{
			GroupVersion: "apiextensions.k8s.io/v1",
			APIResources: []metav1.APIResource{{Name: "customresourcedefinitions", Kind: "CustomResourceDefinition"}},
		},
		{
			GroupVersion: "example.test/v1",
			APIResources: []metav1.APIResource{
				{Name: "widgets", Kind: "Widget", Namespaced: true},
				{Name: "widgets/status", Kind: "Widget", Namespaced: true},
			},
		},
	}}}

	crd := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apiextensions.k8s.io/v1",
		"kind":       "CustomResourceDefinition",
		"metadata":   map[string]interface{}{"name": "widgets.example.test"},
		"spec": map[string]interface{}{
			"group":    "example.test",
			"names":    map[string]interface{}{"kind": "Widget", "plural": "widgets"},
			"scope":    "Namespaced",
			"versions": []interface{}{map[string]interface{}{"name": "v1", "served": true, "storage": true}},
		},
	}}
	widget := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "example.test/v1",
		"kind":       "Widget",
		"metadata":   map[string]interface{}{"name": "broken", "namespace": "team-a"},
		"status": map[string]interface{}{"conditions": []interface{}{
			map[string]interface{}{"type": "Ready", "status": "False", "reason": "DependencyMissing"},
		}},
	}}
	dynamic := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			crdGVR:    "CustomResourceDefinitionList",
			widgetGVR: "WidgetList",
		},
		crd,
		widget,
	)
	cluster := &Cluster{Discovery: discovery, Dynamic: dynamic}

	kinds, err := CustomKinds(context.Background(), cluster)
	if err != nil {
		t.Fatal(err)
	}
	if kinds.Total != 1 || kinds.Overflow != 0 || len(kinds.Kinds) != 1 {
		t.Fatalf("custom kinds = %+v, want exactly one served kind", kinds)
	}
	gotKind := kinds.Kinds[0]
	if gotKind.RefKind != "Widget.example.test" || gotKind.Title != "Widgets" || !gotKind.Namespaced || gotKind.Count != -1 {
		t.Fatalf("custom kind = %+v", gotKind)
	}

	objects, err := ListCustom(context.Background(), cluster, gotKind.RefKind, "team-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(objects) != 1 {
		t.Fatalf("custom objects = %+v, want one", objects)
	}
	if got := objects[0]; got.Namespace != "team-a" || got.Name != "broken" || got.Status != "Not Ready: DependencyMissing" || !got.IsError {
		t.Fatalf("custom object = %+v", got)
	}

	resolved, err := cluster.ResolveGVK("example.test/v1", "Widget")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.GVR != widgetGVR {
		t.Fatalf("resolved GVR = %s, want %s", resolved.GVR, widgetGVR)
	}
}
