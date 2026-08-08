# Performance contract

← [Architecture skeleton](../ARCHITECTURE.md)

Owns `counts.go` and the rate-limit settings in `client.go`, but the rules here
apply to **anything that lists resources**. Read this before adding a feature that
fetches more than one kind.

The app used to stall for a second or more on every namespace change. Three
compounding causes, each with a rule that must not be undone.

## Rule 1 — `QPS: 50` / `Burst: 100`. Do not remove.

client-go's default is **QPS 5 / Burst 10**, tuned for a controller that must not
overwhelm a shared API server. For an interactive UI it is the single largest
source of lag: one screen refresh fans out to a couple of dozen lists, and past
the burst *every further request sleeps*. client-go says so itself:

```
Waited for 1.1129839s due to client-side throttling, not priority and fairness
```

Kubby is one desktop app run by one person against one cluster. It is not the
component that needs protecting — the API server's own priority-and-fairness is
the right place for that, and it is not something a client can opt out of.

## Rule 2 — fan out in Go, not in JavaScript

Sidebar badges used to be **one bound call per kind**: ~25 round-trips over the
Wails bridge, each serialising every object of that kind so the frontend could
take `.length`. Counting Secrets meant transferring every Secret's value.

Now one `SidebarCounts(namespace, includeCluster)` call fans out concurrently in
Go under a `countConcurrency` semaphore (8 — enough to overlap round-trips,
not enough to burst two dozen requests at a proxy).

> **When you add a resource kind, add its tally to `counts.go`. Do not add another
> per-kind bound call.**

The same shape applies to `NamespaceSummary` and `SearchResources`, both of which
were converted for the same reason.

## Rule 3 — metadata-only where a number or a name is enough

`Cluster.Meta` returns `PartialObjectMetadata`: names and labels, no object
bodies. Use it for counting and for name search.

The split in `counts.go` is explicit:

- **Typed lists** for kinds whose badge shows a red error dot — Pods, Deployments,
  StatefulSets, DaemonSets, Jobs, PVCs, Nodes, PVs, Helm releases. The dot needs
  each object's computed status, which metadata cannot give.
- **Metadata lists** (`metaCountKinds`) for everything else.

`SearchResources` goes further: *everything* is a metadata list, and the error flag
for the handful of matched Pods is resolved afterwards with targeted `Get`s
(`markFailingPods`). That keeps the red highlight without listing every Pod in the
cluster.

## Rule 4 — do not refetch what cannot have changed

A namespace change passes `includeCluster=false`. Nodes, PVs, StorageClasses,
ClusterRoles/Bindings and CRDs are cluster-scoped; their counts cannot change when
only the namespace does, so their badges are left alone.

## Rule 5 — guard against stale repaints

Any async render that can be superseded needs a generation counter. Two exist:

```js
let navCountsReqId = 0;   // sidebar counts
let trafficReqId   = 0;   // Traffic view
```

Increment on request, compare before painting, drop if superseded. Without this,
flicking through namespaces can leave older numbers on top of newer ones — and in
the Traffic view a slow reply once repainted an Ingress that had just been deleted.

## Rule 6 — a failing kind must not blank the screen

In `SidebarCounts` a kind that errors is simply omitted. One forbidden kind (RBAC,
a disabled API) must not fail the whole call. `NamespaceSummary` treats only Pods
as required, since a namespace whose pods you cannot list is a genuine access
problem worth surfacing.

## Measurements

One-node kind cluster, 308 objects — the gap widens with cluster size, because
both the throttle and the payload scale with it.

| Path | Before | After |
|---|---|---|
| Fill the sidebar (all kinds, cluster-wide) | **930 ms** (23 sequential typed lists, QPS 5) | **73 ms** |
| Sidebar on a namespace change | same 930 ms | **~100 ms** (17 tallies, cluster kinds skipped) |
| Global search | 10 kinds, sequential, full objects, throttled | ~40 kinds incl. CRDs, concurrent, metadata-only |

Reproduce with the CLI:

```powershell
go run ./cmd/kubby-cli counts                      # full, timed
go run ./cmd/kubby-cli counts -n default --cluster=false   # the namespace-switch path
```

## Known remaining costs

Not yet addressed — worth knowing before blaming something else:

- **Every list is unbounded.** No `Limit`, no `continue`. A production cluster
  with thousands of pods transfers all of them.
- **One `<tr>` per object in the DOM**, each with event listeners.
- **`filterCurrentTable` runs on every keystroke** and reads `tr.textContent` for
  every row — quadratic-feeling on a large table. Caching the search text in a
  `dataset` attribute at render time and debouncing the input are the cheap fixes;
  server-side `Limit` or virtualised rendering is the real one.
