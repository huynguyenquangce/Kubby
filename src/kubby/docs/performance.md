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

Overview follows it too: one `OverviewSnapshot()` bound call fans out under a
fixed semaphore, lists full Nodes and Pods once, and reuses those objects for
counts, failing Pods, capacity, operational Node status and metrics joins. Namespace and Deployment counts
are metadata-only. A failed section becomes a warning and does not blank the
other cards. The 15 newest Events are cached per connection for 30 seconds:
Kubernetes cannot return "the newest 15" server-side, so listing and sorting the
entire Event history on every 5-second poll made busy clusters pay for thousands
of objects to render fifteen rows. Failed Event reads are not cached. The
5-second live refresh updates the current screen; expensive
sidebar tallies have their own 30-second live TTL (manual refresh, writes and
namespace/cluster changes still refresh immediately).

The Pods screen follows the same boundary: one `PodsSnapshot(namespace)` bridge
call concurrently fetches typed Pod rows and optional metrics in Go. Live mode
is single-flight; if a five-second tick arrives while the prior screen refresh
is still running, the tick is skipped instead of adding another API fan-out.

Cluster structure follows the same rule: one bound call concurrently lists
Services, Pods, Ingresses, ReplicaSets and metadata-only Nodes/Namespaces, then
joins selectors and owner chains in memory. It must never list per Service or per
workload. The existing traffic join is shared so topology semantics cannot drift.

The resource Details tab follows it as well. One `GetDrawerSnapshotOwned` call
concurrently gathers required detail, best-effort Events, supported relationship
trees, and the Node/Namespace expansion. Secret values remain a separate explicit
reveal. The App bridge returns the snapshot as JSON so Wails does not generate and
reorder a large transitive model graph. Closing or replacing the drawer cancels
that operation by its exact owner ID; a canceled response cannot render into a
later drawer.

Traffic also starts Services, Pods, Ingresses, EndpointSlices and NetworkPolicies concurrently.
Its in-memory Service→Pod join first narrows candidates through an exact
namespace/label index, then verifies the complete selector. A missing optional
Istio kind is negatively cached for one minute so a cluster without Istio does
not force full discovery on every live refresh.

Helm repository Update downloads at most four indexes concurrently. Downloads
use unique temporary cache directories and publish a parsed index by atomic
rename under the short repository lock, so Browse never races a partial Helm
cache file and two Updates never write the same live file concurrently.

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
cluster. Its metadata-only name index lives for 15 seconds per connection. The
mutex covers a cold refresh deliberately: overlapping or superseded palette
queries share one bounded fan-out instead of multiplying it. Warm queries perform
no Kubernetes lists; they only scan cached names and resolve the few matching Pod
statuses. The short TTL bounds staleness for newly created/deleted resources and
newly installed CRDs.

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

## Rule 5b — abandon the reads of a screen the user has left

A generation counter stops a superseded response from *rendering*; the request
itself keeps running. On a large cluster a view load can hold the connection for
seconds after the user moved on, delaying the screen they did ask for.

Every read a screen makes therefore goes through `withViewCluster` in `app.go`,
which registers its context in the App's view-read registry. `refreshCurrentView`
calls the bound `CancelViewReads()` before starting the next load — awaited, so
the reads it starts are never the ones cancelled — and client-go aborts the HTTP
request instead of finishing it into a response nobody will read. The first load
of a session has nothing in flight, so it costs no extra call.

Two boundaries make this safe:

- **A write is never a view read.** `withOwnedCluster*` registers nothing: a
  screen the user left must not abandon a write they confirmed.
  `TestScreenReadsUseTheCancellableHelper` checks both directions against the
  source, so a new binding added with the wrong helper fails the build.
- **A cancelled read is not an error.** Its response belongs to a scope that is no
  longer current, so the existing `isCurrentViewRequest` guard drops it silently.

Connection transitions (switch, disconnect, shutdown) cancel the registry too,
next to the drawer snapshots.

## Rule 6 — a failing kind must not blank the screen

In `SidebarCounts` a kind that errors is simply omitted. One forbidden kind (RBAC,
a disabled API) must not fail the whole call. `NamespaceSummary` treats only Pods
as required, since a namespace whose pods you cannot list is a genuine access
problem worth surfacing.

## Rule 7 — parse selectors once, not per object

`metav1.LabelSelectorAsSelector` validates every key and value with regular
expressions. Calling it inside a Pod × policy loop made the Traffic view 25–70×
slower once policies existed: on kind with 2,000 Pods and 500 NetworkPolicies one
call went from ~90 ms to ~6.5 s, and 10k endpoint rows × 50 policies allocated
~300 MB per five-second refresh. The profile put 70% of CPU and 93% of allocated
bytes in selector parsing.

- Compile selectors and CIDRs once per call (`compiledNetworkPolicy` in
  `netpol.go`), bucketed by namespace, and match compiled selectors in the loop.
- Compute a per-object answer once per object, not once per row that shows it —
  one Pod appears under every Service that selects it.
- Send per-object data once: the overlay is `NetworkFlows.PodPolicies` keyed by
  Pod, not a copy on every endpoint row.
- Guard with a deterministic test, not a timing one:
  `TestAnnotateFlowPoliciesAllocationBudget` bounds allocations (22.5k for 10k
  rows × 50 policies, budget 100k; the per-row version made 12.4 million), and
  `TestCheckTrafficPolicyRequestsDoNotScaleWithEndpoints` pins API requests.
- Skip evaluation that cannot change the answer: route verdicts
  (`netpol_routes.go`) are not computed at all when the scope has no policy.

## Measurements

One-node kind cluster, 308 objects — the gap widens with cluster size, because
both the throttle and the payload scale with it.

| Path | Before | After |
|---|---|---|
| Fill the sidebar (all kinds, cluster-wide) | **930 ms** (23 sequential typed lists, QPS 5) | **73 ms** |
| Sidebar on a namespace change | same 930 ms | **~100 ms** (17 tallies, cluster kinds skipped) |
| Global search | 10 kinds, sequential, full objects, throttled | ~40 kinds incl. CRDs, concurrent, metadata-only |
| Overview bridge calls | 7 independent calls | **1** `OverviewSnapshot` call |
| Overview processing, synthetic 10,000 Pods | unmeasured | **33 ms/op**, 54.6 MB/op (fake-client benchmark, 3 runs) |
| Initial production JS (before opening Terminal) | **910.88 kB / 265.44 kB gzip** | **581.55 kB / 182.88 kB gzip**; xterm is a 329.31 kB on-demand chunk |
| Traffic overlay, 10k endpoint rows × 50 policies (fake client) | **511 ms**, 279 MB/op, 12.4 M allocs | **13.5 ms**, 1.9 MB/op, 22.5 k allocs |
| Traffic overlay, 100k rows × 500 policies | **53 s** (single run) | **128 ms**, 9.4 MB/op |
| `NetworkTopology`, 100k rows × 50 policies | **5.4–5.6 s** | **119–141 ms** |
| `CheckTrafficPolicy`, 1,000 Pods × 500 policies | **0.93–1.18 s**, 520 MB/op | **40–42 ms**, 34 MB/op; always 5 API requests |
| Topology indexed join, 1000 Services × 10k Pods | 79–84 ms, **67.9 MB/op** (per-row policy fields) | 83–89 ms, **46.2 MB/op** |
| Initial production JS, September 2026 | 589.14 kB / 185.52 kB gzip | **602.88 kB / 189.55 kB gzip** after the policy kinds, Check traffic, drain preview and log hints |
| Why Pending, 10 nodes × 5,000 Pods (derivation only) | **4.7 ms**, 4.8 MB/op, 20,570 allocs (per-Pod `ResourceList` accumulation) | **1.2 ms**, 31 kB/op, **568 allocs**; 4 API requests at 3 nodes and at 200 |
| Why Pending, 1,000 nodes × 5,000 Pods | — | **3.4 ms**, 12,668 allocs |
| Cleanup report, 5,000-object namespace (derivation only) | — | **6.0 ms**, 23,707 allocs; 14 lists, cached 30 s |

The September figures were measured on the same Ryzen 5 5600H under WSL2 with
`-count 3`/`-count 5`. The kind comparison used a one-node cluster seeded with
3,510 Pods, 1,503 Services, 1,000 NetworkPolicies, 250 HPAs and 250 PDBs, timing
HEAD and working-tree binaries alternately; sidebar counts on a namespace switch
rose ~20 ms (17 → 20 lists) and a full refresh did not change measurably.

The 2026-08-14 performance audit additionally measured warmed kind counts at a
24 ms median, Overview at 20.8 ms/op and 24.30 MB/op for 10,000 synthetic Pods,
and the pre-index topology join at 345.8 ms for 1,000 Services × 10,000 Pods.
The checked-in indexed benchmark models 100 selector groups and, after the final
fix, measured 89.3–102.8 ms/op (90.2 ms median across three runs) and 46.17 MB/op
on the same Ryzen 5 5600H development machine. Its output is 100,000 rendered
endpoint rows, so allocation volume is dominated by the result payload rather
than selector scanning. Against the pre-index 345.8 ms audit case, selector-join
latency is about 3.8× lower at the median.

The 2026-08-21 cache pass measured a warm name search over 10,000 metadata
objects at 225 µs/op, 96 B/op and 2 allocs/op, with zero Kubernetes lists after
the 23-list cold built-in index. A cached Overview Event read after initially
reducing 50,000 Events to the newest 15 measured 440 ns/op and 1.5 kB/op; the
action-count test pins exactly one Event list across six five-second live polls.

Reproduce with the CLI:

```powershell
go run ./cmd/kubby-cli counts                      # full, timed
go run ./cmd/kubby-cli counts -n default --cluster=false   # the namespace-switch path
go run ./cmd/kubby-cli overview                            # one Overview snapshot, timed
go test ./internal/k8sclient -run '^$' -bench BenchmarkOverviewSnapshot10kPods -benchmem
go test ./internal/k8sclient -run '^$' -bench BenchmarkNetworkTopologyIndexedJoin1000Services10kPods -benchmem
go test ./internal/k8sclient -run '^$' -bench 'Perf' -benchmem -count 3        # policy overlay, traffic check, request counts
go test ./internal/k8sclient -run 'AllocationBudget|RequestsDoNotScale' -v      # deterministic guards
go test ./internal/k8sclient -run 'RequestCountIsIndependentOfClusterSize|HygieneCachesItsScanPerScope' -v
go test . -run 'View|Screen' -v                                                # view-read cancellation
```

## Bounded resource tables

Pods and Custom Resources request 200 objects at a time using the API server's
opaque continuation token. Their page payload carries `remainingItemCount` when
the server provides it; the UI exposes both an accessible Load more control and
near-end loading. Pods still cross the Wails bridge once per page with matching
metrics filtered to that page.

Every resource table stores its loaded rows outside the DOM and mounts only the
visible window plus ten rows of overscan once it exceeds 80 rows. Top/bottom
spacer rows preserve scroll geometry; `aria-rowcount`/`aria-rowindex` preserve
table position, and filter, sort, bulk selection, refresh cleanup, and keyboard
focus operate on the detached row state rather than only mounted `<tr>` nodes.

## Known remaining costs

Not yet addressed — worth knowing before blaming something else:

- **Typed lists other than Pods remain unbounded at the Kubernetes API.** Their
  DOM cost is virtualized, but the backend still transfers the whole typed list.
- **Pod metrics do not share the Pod list continuation token.** The metrics API
  is still listed once for the scope and filtered down before the page crosses
  Wails; Pod objects and frontend payloads are bounded, but metrics-server work
  can still grow with the namespace.
- **Loaded-row memory is not yet data-only.** The virtual owner retains one
  detached row element and its listeners per loaded object even though only the
  visible window participates in DOM layout. Event delegation plus row factories
  would be the next memory reduction if users routinely load every page.
- **Filtering scans every loaded row.** Normalized row text is cached after the
  first pass, but search is not server-side and does not claim to include pages
  that have not been loaded yet.
- **The initial bundle has crossed 600 kB** (602.88 kB / 189.55 kB gzip). Most of
  it is CodeMirror (below); the September feature work added ~14 kB. Splitting
  the editor is the change that would matter; lazy-loading small modals would not.
- **Health checks list every resource type** for the stuck-deletion scan
  (metadata only, bounded concurrency), so the whole report is cached for 15 s per
  connection and scope and live refresh cannot repeat it every five seconds. The
  API server certificate handshake is cached for 10 minutes. See
  [health-checks.md](health-checks.md).
- **Why Pending lists every Node and every Pod** for one Pod's answer — 4 requests
  regardless of cluster size, and nothing per node. It is opened on demand, never
  by a refresh loop. See [scheduling.md](scheduling.md).
- **Cleanup lists fourteen resource types** (ConfigMaps and Secrets metadata-only)
  and is cached for 30 s per connection and scope, for the same reason Health
  checks is. See [hygiene.md](hygiene.md).
- **Route verdicts add requests when policies exist**: one IngressClass List, one
  Pod List per recognised ingress controller, and a NetworkPolicy List per
  controller namespace outside a scoped view, on each Traffic refresh.
- **CodeMirror remains eager** because the Welcome screen immediately needs the
  shared editor for pasted kubeconfig content. The terminal runtime is lazy: its
  dynamic xterm/FitAddon chunks load only when a Terminal tab opens, and the
  drawer scope is rechecked after loading. Splitting CodeMirror needs a separate
  Welcome lifecycle change and native WebView smoke coverage.
