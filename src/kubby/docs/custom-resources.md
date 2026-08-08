# Custom resources as sections

← [Architecture skeleton](../ARCHITECTURE.md)

Owns `custom.go` and the dynamic sidebar group. Depends on
[kind-resolution.md](kind-resolution.md) — read that first if the `Kind.group`
reference is unfamiliar.

## What this does

Every kind the cluster's CRDs define becomes a **real section**: a sidebar entry, a
command-palette destination, a list table, and the usual
Details / YAML / Events / Delete drawer. Istio's Gateways and VirtualServices,
cert-manager's Certificates, Argo's Applications — whatever is installed.

The trigger was a concrete failure: typing "virtualservice" into the command
palette returned *No matches* on a cluster that plainly had them. The palette
offers "Go to \<section\>" for each entry in `PAGE_TITLES`, and sections existed
only for hard-coded built-in kinds.

## Backend

```go
func CustomKinds(ctx, c) (*CustomKindList, error)                  // the kinds → sections
func ListCustom(ctx, c, refKind, namespace) ([]CustomObject, error) // one kind's objects
```

**The CRD list is the authority**, not a name heuristic. "Any group that doesn't
look built-in" gets it wrong in both directions: Gateway API lives under
`gateway.networking.k8s.io` and *is* a CRD, while `metrics.k8s.io` is not one.
Each CRD is then resolved through `ResolveKind`, which both confirms the kind is
actually served (a CRD can exist with no served version) and yields the plural for
the sidebar label.

`CustomObject` carries only what every resource is guaranteed to have — namespace,
name, age — plus a `Status` read from the near-universal `status.conditions`
convention (`readyCondition`). Many custom resources, Istio's included, have no
conditions at all; the column shows "—" rather than inventing something.

**No counts are fetched.** That would be one list request per CRD on every
refresh, reintroducing exactly the cost [performance.md](performance.md) removed.
`CustomKind.Count` is `-1` and the sidebar shows the number of *kinds* in the group
header instead.

`maxCustomKinds` (60) caps the sidebar. A cluster with a large operator estate can
define hundreds, past which a sidebar stops being navigation. The overflow is
**reported** in the group header, never silently dropped.

## Frontend

A view id is **`custom:<Kind.group>`**, e.g. `custom:VirtualService.networking.istio.io`.

`loadCustomSections()` runs at connect (and on cluster switch) and registers each
kind:

```js
PAGE_TITLES[view] = k.title;              // ← this alone puts it in the command palette
if (k.namespaced) NAMESPACED_VIEWS.add(view);
CUSTOM_KINDS.set(view, k);
```

Registering in `PAGE_TITLES` is *all* it takes for the palette to offer it, because
the palette iterates that map. That is the whole fix to the original complaint.

**All custom views share one DOM section, `#view-custom`.** There is no per-kind
markup to generate — only the kind being listed differs. `viewSectionId(view)` is
the mapping, and **both `selectView` and `filterCurrentTable` must go through it**;
either one bypassing it renders a custom view blank.

Deliberately not registered in `VIEW_KIND`, so the **+ Create** button hides for
custom kinds — there is no sensible per-kind template. Import YAML still works.

## Traps

- The section is **hidden entirely** when the cluster defines no CRDs. Don't make
  it appear empty.
- On a cluster switch, `CUSTOM_KINDS` must be cleared and rebuilt — a different
  cluster has different CRDs.
- A custom kind's drawer ref carries the **qualified** `Kind.group`, which is what
  makes it unambiguous. The drawer header strips the group for display.

## Verify

```powershell
go run ./cmd/kubby-cli custom-kinds
go run ./cmd/kubby-cli list-custom VirtualService.networking.istio.io -n <ns>
```

Without a real Istio install you can still exercise the whole path by applying two
minimal CRDs (group `networking.istio.io`, kinds `Gateway` and `VirtualService`,
schema `x-kubernetes-preserve-unknown-fields: true`) — no control plane needed.
