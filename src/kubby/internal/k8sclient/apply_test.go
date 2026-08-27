package k8sclient

import (
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// The failure this catches is silent: YAML parses two manifests without a `---`
// as one document with duplicate keys and keeps the last, so the first object
// disappears with no error anywhere. It has to be rejected, not applied.
func TestTwoManifestsWithoutASeparatorAreRejected(t *testing.T) {
	doc := `apiVersion: networking.istio.io/v1beta1
kind: Gateway
metadata:
  name: gw
apiVersion: networking.istio.io/v1beta1
kind: VirtualService
metadata:
  name: vs
`
	err := checkMissingSeparator(doc)
	if err == nil {
		t.Fatal("two manifests in one document were accepted; the first would vanish silently")
	}
	if !strings.Contains(err.Error(), "---") {
		t.Errorf("the error must say how to fix it; got %q", err)
	}
}

func TestASingleManifestIsAccepted(t *testing.T) {
	// An `apiVersion:` nested inside a field must not be mistaken for a second
	// document — only column 0 counts.
	doc := `apiVersion: v1
kind: ConfigMap
metadata:
  name: cm
data:
  embedded: |
    apiVersion: v1
    kind: Pod
`
	if err := checkMissingSeparator(doc); err != nil {
		t.Fatalf("a single manifest embedding YAML in a value was rejected: %v", err)
	}
}

func TestSplitYAMLDocumentsSkipsEmptyOnes(t *testing.T) {
	text := "---\napiVersion: v1\nkind: A\n---\n\n---\napiVersion: v1\nkind: B\n---\n"
	docs, err := splitYAMLDocuments(text)
	if err != nil {
		t.Fatalf("split failed: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("got %d documents, want 2 (stray separators must not count): %q", len(docs), docs)
	}
	if !strings.Contains(docs[0], "kind: A") || !strings.Contains(docs[1], "kind: B") {
		t.Errorf("documents came back in the wrong order or content: %q", docs)
	}
}

// Both sides of a diff go through renderForDiff, so anything it fails to strip
// shows up as a change on every single preview and buries the real one.
func TestRenderForDiffRemovesTheServersBookkeeping(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name":              "cm",
			"namespace":         "nexus",
			"resourceVersion":   "918273",
			"uid":               "3f2b1c4d-0000-0000-0000-000000000000",
			"generation":        int64(4),
			"creationTimestamp": "2026-07-29T00:00:00Z",
			"selfLink":          "/api/v1/namespaces/nexus/configmaps/cm",
			"managedFields":     []interface{}{map[string]interface{}{"manager": "kubectl"}},
		},
		"data":   map[string]interface{}{"key": "value"},
		"status": map[string]interface{}{"phase": "Whatever"},
	}}

	out := renderForDiff(obj)
	for _, gone := range []string{"resourceVersion", "uid", "generation", "creationTimestamp", "selfLink", "managedFields", "status"} {
		if strings.Contains(out, gone) {
			t.Errorf("%s survived into the diff text:\n%s", gone, out)
		}
	}
	for _, kept := range []string{"name: cm", "namespace: nexus", "key: value"} {
		if !strings.Contains(out, kept) {
			t.Errorf("%q was stripped but should have been kept:\n%s", kept, out)
		}
	}
}

// The same list is used to clean a manifest before applying it and to clean both
// sides of a diff. If they ever diverge, a preview stops matching the apply.
func TestApplyAndDiffStripTheSameFields(t *testing.T) {
	if len(serverOwnedMetadata) == 0 {
		t.Fatal("serverOwnedMetadata is empty; resourceVersion alone would break every apply")
	}
	for _, f := range []string{"managedFields", "resourceVersion"} {
		found := false
		for _, g := range serverOwnedMetadata {
			if g == f {
				found = true
			}
		}
		if !found {
			t.Errorf("%q must be stripped: it is rewritten on every write", f)
		}
	}
}

func TestApplyDoesNotSilentlyTakeAnotherManagersFields(t *testing.T) {
	write := kubbyApplyOptions(false)
	if write.FieldManager != fieldManager {
		t.Fatalf("field manager = %q, want %q", write.FieldManager, fieldManager)
	}
	if write.Force {
		t.Fatal("ordinary Apply must surface managed-field conflicts instead of forcing ownership")
	}
	if len(write.DryRun) != 0 {
		t.Fatalf("write unexpectedly has dry-run options: %v", write.DryRun)
	}

	preview := kubbyApplyOptions(true)
	if preview.Force {
		t.Fatal("Preview must model the same non-forcing ownership semantics as Apply")
	}
	if len(preview.DryRun) != 1 || preview.DryRun[0] != "All" {
		t.Fatalf("preview dry-run = %v, want [All]", preview.DryRun)
	}
}

// A bundle that creates a namespace and then fills it cannot be fully previewed,
// because a dry run creates nothing. Detecting that case is what turns a bare
// "not found" into an explanation.
func TestNamespacesCreatedByABundle(t *testing.T) {
	docs := []string{
		"apiVersion: v1\nkind: Namespace\nmetadata:\n  name: brandnew\n",
		"apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: settings\n  namespace: brandnew\n",
		"this is not: [valid yaml", // must be skipped, not fatal
	}
	got := namespacesCreatedBy(docs)
	if !got["brandnew"] {
		t.Errorf("the Namespace in the bundle was not detected: %v", got)
	}
	if len(got) != 1 {
		t.Errorf("got %v; only the Namespace document should contribute", got)
	}
}

func TestPreviewExplainsThePendingNamespace(t *testing.T) {
	p := &preparedDoc{ns: "brandnew", ref: "ConfigMap brandnew/settings (v1)"}
	pending := map[string]bool{"brandnew": true}

	msg := previewError(p, notFoundErr("brandnew"), pending)
	if !strings.Contains(msg, "another document in this bundle creates it") {
		t.Errorf("the pending-namespace case was not explained; got %q", msg)
	}
	if !strings.Contains(msg, "Applying will work") {
		t.Errorf("the message must say the apply itself is fine; got %q", msg)
	}

	// A namespace nothing in the bundle creates is a genuine error and must be
	// passed through unchanged rather than excused.
	msg = previewError(&preparedDoc{ns: "elsewhere"}, notFoundErr("elsewhere"), pending)
	if strings.Contains(msg, "another document in this bundle") {
		t.Errorf("an unrelated missing namespace was wrongly excused: %q", msg)
	}
}

// The errors a person actually hits when pasting. Each must name what is missing
// rather than failing generically — this is the path that produced the original
// "it says there is no kind" confusion.
func TestPreparedDocRejectsManifestsWithoutIdentity(t *testing.T) {
	cases := []struct {
		name, doc, want string
	}{
		{"no apiVersion and no kind", "foo: bar\n", "does not look like a Kubernetes manifest"},
		{"kind only", "kind: Pod\n", `missing "apiVersion"`},
		{"apiVersion only", "apiVersion: v1\n", `missing "kind"`},
		{"no name", "apiVersion: v1\nkind: Pod\n", "missing metadata.name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// prepareDoc needs a Cluster only after these checks pass, so a nil one
			// is enough to exercise them.
			_, err := prepareDoc(nil, tc.doc)
			if err == nil {
				t.Fatalf("accepted %q", tc.doc)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q; want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestPreparedDocSkipsAnEmptyDocument(t *testing.T) {
	p, err := prepareDoc(nil, "# just a comment\n")
	if err != nil {
		t.Fatalf("a comment-only document errored: %v", err)
	}
	if p != nil {
		t.Errorf("a comment-only document produced %+v; want nil so the caller skips it", p)
	}
}

// notFoundErr produces the real thing apierrors.IsNotFound recognises, so
// previewError's branch is exercised exactly as the apiserver would trigger it.
func notFoundErr(name string) error {
	return apierrors.NewNotFound(schema.GroupResource{Resource: "namespaces"}, name)
}
