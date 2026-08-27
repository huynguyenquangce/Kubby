# Apply YAML

← [Architecture skeleton](../ARCHITECTURE.md)

Owns `apply.go`, behind the **+ Create** and **Import YAML** flows. Depends on
[kind-resolution.md](kind-resolution.md), which is what makes "any kind" possible.

## Contract

```go
func ApplyYAML(ctx, c, yamlText) (report string, err error)
```

Applies every document in a multi-document manifest, of any kind the cluster
serves, and returns a per-document report. On failure the error message *is* the
report, so the UI can show what did and did not happen in one place.

## Server-side apply

`ri.Apply(ctx, name, obj, ApplyOptions{FieldManager: "kubby", Force: false})`.

One call that creates or merges. The important property over a whole-object
`Update` is that it **leaves server-defaulted fields alone** instead of failing on
them as immutable — a Service's `clusterIP` is the usual victim of the old
create-then-update approach.

Managed-field conflicts are returned per document instead of being resolved in
Kubby's favour. The existing Apply confirmation approves the manifest write; it
does not separately explain or approve taking fields owned by another manager.
Preview uses the same non-forcing option, so it predicts the write's ownership
semantics. A future force action would need its own explicit confirmation. A
"created" vs "updated" label comes from a preliminary `Get`.

## Two guards worth keeping

### 1. Strip server-owned metadata

```go
status, metadata.{managedFields, resourceVersion, uid, creationTimestamp, generation, selfLink}
```

Pasting `kubectl get -o yaml` output is the normal case, not an edge case, and
`resourceVersion` makes a server-side apply fail outright. `status` is a read-only
subresource.

### 2. Reject a missing `---`

`checkMissingSeparator` fails a document that contains two column-0
`apiVersion:` keys.

This is not pedantry. Two manifests pasted without a separator are **one** YAML
document with duplicate keys, and the parser silently keeps the last — the first
object vanishes with no error at all. That exact paste is what made Istio
manifests appear to "not apply". Column 0 is a reliable signal: inside a block
scalar or a nested mapping the key would be indented.

## Preview: what would this change? (`ApplyPreview`)

```go
func ApplyPreview(ctx, c, yamlText) (*ApplyDiff, error)
```

The **Preview** button in Create / Import YAML. Each document is sent as a
server-side apply with `DryRun: []string{metav1.DryRunAll}`, and the object the
server says *would* result is diffed against the object that is live now. Nothing
is written.

Both sides go through `renderForDiff`, which strips `status` and the same
`serverOwnedMetadata` list the apply strips. **That list is shared deliberately.**
Left in, every preview would show a `resourceVersion` change and a `managedFields`
reshuffle, burying the one line the user actually changed.

`ApplyDiff` reuses `HelmDiff`'s `current` / `proposed` shape so the frontend renders
both with the same line-diff view.

### Why the server has to do it

A client-side comparison of the manifest against the live object cannot show
defaulting, admission mutation (an injected sidecar), or field-ownership
resolution. The dry run shows all three. The most instructive example found while
building this: changing a Service's port from `8081` to `9090` does **not** replace
the port — the ports list merges by `port`+`protocol`, so the object ends up with
*both*. Nothing client-side would have predicted that.

### Two limits inherent to dry-run

Both are reported per document rather than hidden, because a preview that quietly
omits a document is worse than one that says why it cannot show it:

1. **A webhook can refuse the dry run.** An admission webhook that does not declare
   `sideEffects: None` or `NoneOnDryRun` makes the apiserver reject `DryRunAll`, so
   a preview can fail where the real apply would succeed.
2. **A dry run creates nothing.** A bundle that creates a Namespace and then fills
   it cannot preview the later documents — the namespace they need does not exist.
   `previewError` detects exactly this case (the missing namespace is one the bundle
   itself creates) and says so, instead of surfacing a bare `not found`. `kubectl
   diff` has the same limitation and does not explain it.

## Other behaviour

- **Every document is attempted** even if an earlier one fails, so a half-valid
  bundle applies its valid half and names the rest. `kubectl` behaves the same way.
- **Documents are split with apimachinery's own `YAMLReader`**, not
  `strings.Split(text, "\n---")` — the latter also splits on `----` and on `---`
  inside a string.
- **A namespaced object with no `metadata.namespace` goes to `default`**, like
  `kubectl`. This is stated rather than silent: the report names the namespace of
  every object it touched.
- **A namespace on a cluster-scoped object is cleared**, since the API rejects it.
- Before dispatch, `PlanApplyPermissions` resolves every document and asks the
  API server for `patch`, the exact server-side-apply RBAC verb. A checked denial
  blocks Apply; an unanswered probe remains allowed. Preview includes the same
  plan so the user can see it before the final modal action.

## Related: editing YAML in the drawer

`UpdateYAML` (in `detail.go`, see [resource-browsing.md](resource-browsing.md)) is
a different path — a plain `Update` from the drawer's editor, resolved from the
document's own `apiVersion`. It is not server-side apply, because the editor's
input is a full object read moments earlier rather than a partial intent.

## The editor itself

The YAML text box is **CodeMirror**, not a `<textarea>` — line numbers exist so
"document 3 failed" can be pointed at, and after a failed apply
`markDocumentErrors` puts a gutter marker on that document's first line. See
[frontend.md](frontend.md) for the editor and the two keyboard traps that came
with it.

`parseApplyFailures` (in `frontend/src/editor.js`) reads the `FAILED   document N:`
lines out of the report to place those markers. That couples the frontend to a
string format produced here; if the line ever changes shape the markers quietly
stop appearing, and the full report is still shown. Change both together.

## Verify

```powershell
go run ./cmd/kubby-cli apply -f manifest.yaml    # writes
go run ./cmd/kubby-cli diff  -f manifest.yaml    # writes nothing
```

Cases worth re-checking after a change: a built-in kind with no namespace (lands in
`default`), a CRD kind, a re-apply (reports `updated`), a two-manifest file without
`---` (rejected by name), and a bundle where one document is bad (the rest apply).

For `diff`, the five cases that between them cover every branch — all verified
against the local kind cluster:

| Case | Expected |
|---|---|
| Edit an existing object | `UPDATE` with only the changed lines |
| A new object | `CREATE`, showing the server's defaults too |
| Feed a live object straight back | `UNCHANGED`, no diff body |
| A CRD kind (`VirtualService`) | `UPDATE`, same as a built-in |
| `Namespace` + an object inside it | one `CREATE`, one explained failure |

Unit tests cover the pure parts in `apply_test.go` (separator detection, document
splitting, diff-noise stripping, the pending-namespace explanation). Nothing there
needs a cluster.
