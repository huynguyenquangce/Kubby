# Resource browsing

← [Architecture skeleton](../ARCHITECTURE.md)

Owns the list views and the detail drawer: `resources*.go`, `detail.go`,
`describe.go`, `relations.go`, `explore.go`, and their frontend counterparts.
**The recipe to add a new resource kind is at the bottom.**

## Backend layout

Four list files, grouped the way the sidebar is:

| File | Kinds |
|---|---|
| `resources.go` | Namespace, Node, Pod, Deployment, Service, ConfigMap, Secret — plus `podStatus()` and `erroredStatuses` |
| `resources_more.go` | StatefulSet, DaemonSet, Job, CronJob, Ingress, PVC, ServiceAccount |
| `resources_cluster.go` | PersistentVolume, StorageClass, RBAC (Role/RoleBinding/ClusterRole/ClusterRoleBinding) |
| `resources_ecosystem.go` | CRDs, Helm releases, ResourceQuotas, LimitRanges |

Each exposes `XxxInfo` + `ListXxx(ctx, client, ns)`. `XxxInfo` is a flat,
display-shaped struct — the frontend renders it directly, so put formatting
(`age`, "3/5 ready") in Go rather than in JS.

`drawer.go` owns the one-call initial Details snapshot. `App` binds that call to
the expected connection and an exactly cancellable drawer operation. It overlaps detail,
Events, Deployment/Service/Ingress relations, and Node/Namespace expansion; only
detail failure rejects the snapshot. Keep Secret data out of this payload because
reveal is an explicit user action.

Verify the same read-only snapshot against a cluster without the GUI:

```bash
go run ./cmd/kubby-cli drawer Pod <name> -n <namespace>
```

`IsError` on an `XxxInfo` is what drives the red row and the red dot on the
sidebar badge. Only some kinds have it; see [performance.md](performance.md) for
why that distinction matters to how they are counted.

## The detail drawer

Four sources, all keyed by `(kind, namespace, name)` where `kind` may be
group-qualified — see [kind-resolution.md](kind-resolution.md).

- **`GetDetail`** (`describe.go`) — a `switch` with a bespoke case per built-in
  kind. Anything unmatched falls through to **`genericDetail`**, which renders
  metadata plus a shape summary of `spec`/`status` (scalars printed, lists as
  "N item(s)", maps as their key names). That is what makes a custom resource's
  Details tab work with no per-kind code.
- **`ListEvents`** — resolves the live object first and adds
  `involvedObject.uid` to the field selector when available. This prevents events
  for a deleted object being attached to a new same-name object. It still uses
  `bareKind()` because a qualified reference would never match.
- **`GetYAML`** (`detail.go`) — strips `status` and `managedFields` before
  showing. `status` is a read-only subresource: sending it back on save produces
  apiserver warnings and never applies. `UpdateYAML` resolves from the document's
  own `apiVersion`, so editing a custom resource works.
- **Relations** (`relations.go`) — `DeploymentTree` / `ServiceTree` /
  `IngressTree` build a `RelationNode` tree whose nodes carry their namespace so
  they stay clickable. Also here: `NodeMetrics`, `TopPods`, `PodMetricsList`, all
  nil-safe because `Cluster.Metrics` may be absent.

Pod status gives deletion precedence, counts init-container restarts, and uses a
fixed failure precedence across every init/application container. Container
ordering must never let a later `ContainerCreating` hide an earlier
`CrashLoopBackOff`. Normal `Init:PodInitializing`/`ContainerCreating` remain
progress states, while generic/unschedulable `Pending`, Running-but-not-ready,
terminal failure and common image/start/OOM reasons enter the shared issue set
used by lists, counts, search, Overview and topology. Owner joins compare UID
when Kubernetes supplies it, and Job health follows true terminal conditions
rather than historical retry counts.

## Exploration

`explore.go` holds the three "answer a question about the cluster" calls:

- **`PodsOnNode`** — a `spec.nodeName` field selector, backing the Node drawer.
- **`NamespaceSummary`** — per-kind counts for one namespace. Fans out
  concurrently into fixed slots so the display order stays deterministic.
- **`SearchResources`** — the Ctrl+K global search. Covers **every kind the UI has
  a section for**, custom resources included, concurrently and metadata-only.

Both of the latter follow the rules in [performance.md](performance.md).

The Pods table uses `PodsPage(namespace, continue, limit)`: one App binding loads
a bounded Pod page and optional metrics filtered to those rows, then returns the
API server's opaque continuation token. Keep list/metrics merging inside that
page payload rather than adding a second frontend bridge call. All resource-list
renderers hand their row objects to `virtual-table.js`; sort/filter/bulk actions
must use that detached state rather than querying only the mounted row window.

## Frontend table conventions

> **Column count matters.** `main.js` appends an `Age` **and** an `Actions` header
> to every list table, and `row()` prepends a checkbox cell and appends the actions
> cell. A `cellsFn` that does not end with an age `<td>` shifts every column one to
> the left — the ⋯ button lands under "Age" and Actions is blank. This was a real
> bug in ResourceQuotas / LimitRanges / CRDs. `padRowToHeader()` backfills short
> rows as a safety net, but **emit the age cell** rather than relying on it.

`loadSimple(listFn, viewId, kind, cellsFn)` is the standard renderer; use it unless
the view needs something it cannot express.

---

## Recipe: add a new resource kind (e.g. "Endpoints")

Grep an existing kind such as `ConfigMap` and copy it.

1. **Backend list** — add `XxxInfo` + `ListXxx(ctx, client, ns)` to the
   `resources*.go` file matching its sidebar section.
2. **GVR** — add a line to `gvrByKind` in `detail.go` (and `clusterScopedKinds` if
   cluster-scoped). This is *only* a fast path that skips a discovery round-trip;
   [kind-resolution.md](kind-resolution.md) already handles the kind without it.
3. **Detail** — add a `case "Xxx"` to `GetDetail` in `describe.go`. Optional:
   without one it falls through to `genericDetail`, which is fine for a custom
   resource and thin for a built-in.
4. **Count** — add its tally to `counts.go`: `metaCountKinds` if the badge shows
   only a number, or a typed closure if it needs an error dot. **Do not add a
   per-kind bound call** ([performance.md](performance.md)).
5. **Search** — add it to `searchKinds` in `explore.go` so Ctrl+K finds it.
6. **Bind** — `func (a *App) ListXxx(ns string) (...)` in `app.go`.
7. **Frontend** — in `index.html`: a `<button class="nav-item" data-view="xxx">…`
   inside the right `.nav-section`, and a `<section id="view-xxx">`. In `main.js`:
   add to `PAGE_TITLES` (this is also what puts it in the command palette),
   `NAMESPACED_VIEWS` if namespaced, `VIEW_KIND` for **+ Create**, a `case 'xxx':`
   in `doRefresh()` (usually `loadSimple(...)`), a `case` in `templateFor()`, and
   `CLUSTER_SCOPED_KINDS` if cluster-scoped.
8. **Build** — `wails build` regenerates the bindings.

## Verify

```powershell
go run ./cmd/kubby-cli get pods -n default
go run ./cmd/kubby-cli yaml Deployment <name> -n default
go run ./cmd/kubby-cli events Pod <name> -n default
go run ./cmd/kubby-cli ns-summary <namespace>
go run ./cmd/kubby-cli search <query>
```
