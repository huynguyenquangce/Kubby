package k8sclient

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestValidateUpdateTarget(t *testing.T) {
	configMap := APIKind{
		GVR:        schema.GroupVersionResource{Version: "v1", Resource: "configmaps"},
		Kind:       "ConfigMap",
		Namespaced: true,
	}
	secret := APIKind{
		GVR:        schema.GroupVersionResource{Version: "v1", Resource: "secrets"},
		Kind:       "Secret",
		Namespaced: true,
	}
	node := APIKind{
		GVR:        schema.GroupVersionResource{Version: "v1", Resource: "nodes"},
		Kind:       "Node",
		Namespaced: false,
	}

	tests := []struct {
		name      string
		expected  APIKind
		namespace string
		object    APIKind
		objName   string
		objNS     string
		wantError bool
	}{
		{name: "same resource", expected: configMap, namespace: "team-a", object: configMap, objName: "settings", objNS: "team-a"},
		{name: "different name", expected: configMap, namespace: "team-a", object: configMap, objName: "other", objNS: "team-a", wantError: true},
		{name: "different namespace", expected: configMap, namespace: "team-a", object: configMap, objName: "settings", objNS: "team-b", wantError: true},
		{name: "different kind", expected: configMap, namespace: "team-a", object: secret, objName: "settings", objNS: "team-a", wantError: true},
		{name: "cluster scoped with namespace", expected: node, object: node, objName: "worker-1", objNS: "default", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &unstructured.Unstructured{}
			obj.SetName(tt.objName)
			obj.SetNamespace(tt.objNS)
			err := validateUpdateTarget(tt.expected, tt.namespace, "settings", tt.object, obj)
			if tt.expected.Kind == "Node" {
				err = validateUpdateTarget(tt.expected, tt.namespace, "worker-1", tt.object, obj)
			}
			if tt.wantError && (err == nil || !strings.Contains(err.Error(), "refusing stale YAML update")) {
				t.Fatalf("expected stale-update error, got %v", err)
			}
			if !tt.wantError && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
