package k8sclient

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/yaml"
)

// fieldManager identifies Kubby's writes in metadata.managedFields, the same way
// kubectl registers as "kubectl-client-side-apply".
const fieldManager = "kubby"

// defaultApplyNamespace mirrors kubectl: a namespaced object whose manifest
// omits metadata.namespace lands in "default". The report names the namespace of
// every object it touched, so this is stated rather than silent.
const defaultApplyNamespace = "default"

// ApplyYAML creates the resource(s) in the given YAML, or updates them if they
// already exist, and returns a per-document report of what it did.
//
// Any kind the cluster serves is accepted, including custom resources — the
// apiVersion + kind in each document is resolved against live discovery rather
// than a fixed table, so Istio, cert-manager, Argo and friends all apply.
//
// Every document is attempted even if an earlier one fails: a manifest bundle
// that is half-valid should get its valid half applied, with the failures named.
func ApplyYAML(ctx context.Context, c *Cluster, yamlText string) (string, error) {
	docs, err := splitYAMLDocuments(yamlText)
	if err != nil {
		return "", err
	}
	if len(docs) == 0 {
		return "", fmt.Errorf("no YAML content to apply")
	}

	lines := make([]string, 0, len(docs))
	applied, failed := 0, 0
	for i, doc := range docs {
		action, ref, err := applyOne(ctx, c, doc)
		switch {
		case err != nil:
			failed++
			lines = append(lines, fmt.Sprintf("FAILED   document %d: %v", i+1, err))
		case action == "":
			// An empty document (a stray `---`) — nothing to report.
		default:
			applied++
			lines = append(lines, fmt.Sprintf("%s  %s", action, ref))
		}
	}

	total := applied + failed
	head := fmt.Sprintf("%d of %d documents applied.", applied, total)
	if failed > 0 {
		head = fmt.Sprintf("%d of %d documents applied — %d failed.", applied, total, failed)
	}
	report := head + "\n\n" + strings.Join(lines, "\n")
	if failed > 0 {
		return report, fmt.Errorf("%s", report)
	}
	return report, nil
}

func applyOne(ctx context.Context, c *Cluster, doc string) (action, ref string, err error) {
	p, err := prepareDoc(c, doc)
	if err != nil {
		return "", "", err
	}
	if p == nil {
		return "", "", nil // empty document, skip
	}

	action = "created"
	if _, getErr := p.ri.Get(ctx, p.name, metav1.GetOptions{}); getErr == nil {
		action = "updated"
	}

	// Server-side apply: one call that creates or merges, and — unlike a full
	// object Update — leaves server-defaulted fields (a Service's clusterIP, an
	// injected sidecar) alone instead of failing on them as immutable.
	// Force resolves ownership conflicts in Kubby's favour, which is what a user
	// clicking "Apply" on a manifest is asking for.
	if _, err := p.ri.Apply(ctx, p.name, p.obj, metav1.ApplyOptions{FieldManager: fieldManager, Force: true}); err != nil {
		return "", "", err
	}
	return action, p.ref, nil
}

// ---- shared document preparation ----

// preparedDoc is one YAML document parsed, validated and resolved against the
// cluster, with the server-owned bookkeeping stripped. It is everything both the
// real apply and the dry-run preview need before they diverge into "write it"
// and "show what would change".
type preparedDoc struct {
	obj  *unstructured.Unstructured
	ri   dynamic.ResourceInterface
	name string
	ns   string // "" for a cluster-scoped object
	ref  string // "Deployment nexus/api (apps/v1)"
}

// prepareDoc resolves a single document. A nil doc with a nil error means the
// document was empty (a stray `---`) and the caller should skip it.
func prepareDoc(c *Cluster, doc string) (*preparedDoc, error) {
	if err := checkMissingSeparator(doc); err != nil {
		return nil, err
	}

	var raw map[string]interface{}
	if err := yaml.Unmarshal([]byte(doc), &raw); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}
	if len(raw) == 0 {
		return nil, nil
	}
	obj := &unstructured.Unstructured{Object: raw}

	kind, apiVersion := obj.GetKind(), obj.GetAPIVersion()
	switch {
	case kind == "" && apiVersion == "":
		return nil, fmt.Errorf(`no "apiVersion" or "kind" at the top level — this does not look like a Kubernetes manifest`)
	case kind == "":
		return nil, fmt.Errorf(`missing "kind"`)
	case apiVersion == "":
		return nil, fmt.Errorf(`missing "apiVersion"`)
	}
	name := obj.GetName()
	if name == "" {
		return nil, fmt.Errorf("%s is missing metadata.name", kind)
	}

	ak, err := c.ResolveGVK(apiVersion, kind)
	if err != nil {
		return nil, err
	}

	// Strip the server-owned bookkeeping people inevitably paste in from
	// `kubectl get -o yaml`. resourceVersion in particular makes a server-side
	// apply fail outright, and status is a read-only subresource.
	unstructured.RemoveNestedField(obj.Object, "status")
	for _, f := range serverOwnedMetadata {
		unstructured.RemoveNestedField(obj.Object, "metadata", f)
	}

	var ri dynamic.ResourceInterface = c.Dynamic.Resource(ak.GVR)
	ns := ""
	if ak.Namespaced {
		ns = obj.GetNamespace()
		if ns == "" {
			ns = defaultApplyNamespace
			obj.SetNamespace(ns)
		}
		ri = c.Dynamic.Resource(ak.GVR).Namespace(ns)
	} else {
		obj.SetNamespace("") // a namespace on a cluster-scoped object is rejected
	}

	where := name
	if ns != "" {
		where = ns + "/" + name
	}
	return &preparedDoc{
		obj:  obj,
		ri:   ri,
		name: name,
		ns:   ns,
		ref:  fmt.Sprintf("%s %s (%s)", kind, where, apiVersion),
	}, nil
}

// serverOwnedMetadata is metadata the apiserver owns and rewrites on every
// write. It is stripped from a manifest before applying (resourceVersion in
// particular makes a server-side apply fail), and from both sides of a diff —
// left in, it would show a resourceVersion change and a managedFields reshuffle
// on every preview, burying the one line the user actually changed.
var serverOwnedMetadata = []string{
	"managedFields", "resourceVersion", "uid", "creationTimestamp", "generation", "selfLink",
}

// ---- dry-run preview ----

// ApplyDiffDoc is what one document of a bundle would do to the cluster.
//
// Current/Proposed mirror HelmDiff's shape deliberately: the frontend renders
// both with the same line-diff view.
type ApplyDiffDoc struct {
	Ref      string `json:"ref"`      // "Deployment nexus/api (apps/v1)", or "document 3" if unidentifiable
	Action   string `json:"action"`   // create | update | unchanged | error
	Current  string `json:"current"`  // the live object; "" for a create
	Proposed string `json:"proposed"` // what would exist after the apply
	Error    string `json:"error"`    // set when only this document could not be previewed
}

// ApplyDiff is the whole bundle's preview. Counts are carried so the UI can
// summarise without walking the documents.
type ApplyDiff struct {
	Docs      []ApplyDiffDoc `json:"docs"`
	Create    int            `json:"create"`
	Update    int            `json:"update"`
	Unchanged int            `json:"unchanged"`
	Failed    int            `json:"failed"`
}

// ApplyPreview answers "what would this YAML change?" without changing anything,
// the way `kubectl diff` does: each document is sent as a server-side apply with
// DryRun=All, and the object the server says would result is diffed against the
// object that is live now.
//
// The server does the merging, so the preview accounts for defaulting, admission
// mutation (an injected sidecar) and field-ownership resolution — none of which a
// client-side comparison of the manifest could show.
//
// Two limits are inherent to dry-run and are reported per document rather than
// hidden:
//
//   - An admission webhook that does not declare sideEffects None/NoneOnDryRun
//     makes the apiserver refuse the dry run, so a preview can fail where the
//     real apply would succeed.
//   - A dry run creates nothing, so a bundle that creates a Namespace and then
//     objects inside it cannot preview those objects. That case is detected and
//     explained instead of surfacing a bare "not found".
func ApplyPreview(ctx context.Context, c *Cluster, yamlText string) (*ApplyDiff, error) {
	docs, err := splitYAMLDocuments(yamlText)
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return nil, fmt.Errorf("no YAML content to preview")
	}

	pending := namespacesCreatedBy(docs)

	out := &ApplyDiff{Docs: make([]ApplyDiffDoc, 0, len(docs))}
	for i, doc := range docs {
		d := previewOne(ctx, c, i, doc, pending)
		if d == nil {
			continue // empty document
		}
		switch d.Action {
		case "create":
			out.Create++
		case "update":
			out.Update++
		case "unchanged":
			out.Unchanged++
		default:
			out.Failed++
		}
		out.Docs = append(out.Docs, *d)
	}
	if len(out.Docs) == 0 {
		return nil, fmt.Errorf("no YAML content to preview")
	}
	return out, nil
}

func previewOne(ctx context.Context, c *Cluster, index int, doc string, pending map[string]bool) *ApplyDiffDoc {
	p, err := prepareDoc(c, doc)
	if err != nil {
		return &ApplyDiffDoc{
			Ref:    fmt.Sprintf("document %d", index+1),
			Action: "error",
			Error:  err.Error(),
		}
	}
	if p == nil {
		return nil
	}

	current := ""
	if live, getErr := p.ri.Get(ctx, p.name, metav1.GetOptions{}); getErr == nil {
		current = renderForDiff(live)
	} else if !apierrors.IsNotFound(getErr) {
		return &ApplyDiffDoc{
			Ref:    p.ref,
			Action: "error",
			Error:  fmt.Sprintf("could not read the live object: %v", getErr),
		}
	}

	dry, err := p.ri.Apply(ctx, p.name, p.obj, metav1.ApplyOptions{
		FieldManager: fieldManager,
		Force:        true,
		DryRun:       []string{metav1.DryRunAll},
	})
	if err != nil {
		return &ApplyDiffDoc{Ref: p.ref, Action: "error", Error: previewError(p, err, pending)}
	}
	proposed := renderForDiff(dry)

	switch {
	case current == "":
		return &ApplyDiffDoc{Ref: p.ref, Action: "create", Proposed: proposed}
	case current == proposed:
		return &ApplyDiffDoc{Ref: p.ref, Action: "unchanged", Current: current, Proposed: proposed}
	default:
		return &ApplyDiffDoc{Ref: p.ref, Action: "update", Current: current, Proposed: proposed}
	}
}

// previewError turns the one dry-run failure that is expected rather than wrong
// into an explanation. Applying a bundle that creates a Namespace and then fills
// it works; previewing it cannot, because the dry run does not create the
// namespace the later documents need.
func previewError(p *preparedDoc, err error, pending map[string]bool) string {
	if apierrors.IsNotFound(err) && p.ns != "" && pending[p.ns] {
		return fmt.Sprintf("namespace %q does not exist yet — another document in this bundle creates it, and a dry run creates nothing. Applying will work; only this document's preview is unavailable.", p.ns)
	}
	return err.Error()
}

// namespacesCreatedBy collects the namespaces this bundle would create, so a
// later document failing to preview inside one of them can be explained.
// Parse failures are ignored: the document that cannot be parsed reports its own
// error through prepareDoc.
func namespacesCreatedBy(docs []string) map[string]bool {
	out := map[string]bool{}
	for _, doc := range docs {
		var raw map[string]interface{}
		if err := yaml.Unmarshal([]byte(doc), &raw); err != nil {
			continue
		}
		obj := &unstructured.Unstructured{Object: raw}
		if obj.GetKind() == "Namespace" && obj.GetName() != "" {
			out[obj.GetName()] = true
		}
	}
	return out
}

// renderForDiff turns an object into the stable YAML text the diff compares.
// Both sides go through it, so the noise it removes cannot reappear as a change.
func renderForDiff(obj *unstructured.Unstructured) string {
	clone := obj.DeepCopy()
	unstructured.RemoveNestedField(clone.Object, "status")
	for _, f := range serverOwnedMetadata {
		unstructured.RemoveNestedField(clone.Object, "metadata", f)
	}
	// sigs.k8s.io/yaml marshals through JSON, so keys come out sorted — the same
	// input always renders identically, which is what makes "unchanged" reliable.
	text, err := yaml.Marshal(clone.Object)
	if err != nil {
		return fmt.Sprintf("# could not render this object for diffing: %v\n", err)
	}
	return string(text)
}

// topLevelAPIVersion matches an `apiVersion:` key at the start of a line — i.e.
// at the document root. Inside a block scalar or a nested mapping it would be
// indented, so column 0 is a reliable signal.
var topLevelAPIVersion = regexp.MustCompile(`(?m)^apiVersion:`)

// checkMissingSeparator catches the single most common paste error: two
// manifests in one file with no `---` between them. YAML parses that as one
// document with duplicate keys and silently keeps the last, so the first object
// would vanish without any error at all.
func checkMissingSeparator(doc string) error {
	if n := len(topLevelAPIVersion.FindAllString(doc, -1)); n > 1 {
		return fmt.Errorf(`this looks like %d manifests in one document (%d top-level "apiVersion:" keys) — separate them with a line containing only ---`, n, n)
	}
	return nil
}

// splitYAMLDocuments splits a multi-document YAML stream on `---` separators,
// using the same reader Kubernetes itself uses so that a `---` inside a string
// or a comment on the separator line are both handled correctly.
func splitYAMLDocuments(text string) ([]string, error) {
	reader := utilyaml.NewYAMLReader(bufio.NewReader(strings.NewReader(text)))
	out := []string{}
	for {
		doc, err := reader.Read()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("could not split the YAML into documents: %w", err)
		}
		if strings.TrimSpace(string(doc)) == "" {
			continue
		}
		out = append(out, string(doc))
	}
}
