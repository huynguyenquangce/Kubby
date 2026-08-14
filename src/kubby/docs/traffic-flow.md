# Traffic flow

← [Architecture skeleton](../ARCHITECTURE.md)

Owns `netflow.go` (Kubernetes Ingress) and `istioflow.go` (Istio Gateway +
VirtualService), plus the Traffic view in the frontend.

## What the view answers

*"If a request arrives, where does it end up — and where does it stop?"*

Every path is laid out in three lanes, **Route → Service → Pods**, with the hop
that breaks flagged at the hop that breaks. Both ingress styles render in the same
lanes, badged so you can tell them apart.

## The model

Purpose-built types rather than the generic `RelationNode` tree, so each hop
carries what that hop needs:

```go
FlowIngress  // one entry point: Ingress or Istio Gateway
  Kind, RefKind      // "Ingress"|"Gateway"; RefKind is what the drawer resolves
  Class, Address, Hosts, Ports, TLS
  Services []FlowService
  Warning
FlowService  // a Service and the pods its selector matches
  Routes []string    // "demo.local/api → :9898"
  Via*               // the VirtualService that created these routes (Istio only)
  Pods []FlowPod, ReadyPods, Warning
FlowPod      // one endpoint, with readiness
```

`NetworkTopology(ctx, c, namespace)` lists **everything once and joins in memory** —
one List per kind, not one per Service. That is both faster and a single
consistent snapshot rather than reads taken seconds apart.
The four core Lists start concurrently. Service selectors use a namespace/label
candidate index before the complete selector is checked, avoiding a full Pod
scan for every Service on large clusters.

## Invariants

1. **Terminating objects are excluded.** Any object with a `DeletionTimestamp` is
   skipped. Without this, an Ingress or Gateway held by a finalizer keeps showing
   as live routing after you delete it — this was the original bug report.
2. **The frontend carries a request-generation guard** (`trafficReqId`) so a slow
   reply cannot repaint stale topology over a newer one.
3. **A warning names the hop that breaks**, not the path. "Service not found",
   "No Pods match the selector", "No Pod is ready" are per-hop.
4. **EndpointSlice is routing truth.** When slices are available, endpoint
   `ready` and `terminating` conditions decide which selected Pods are serving;
   PodReady is only the explicit fallback when EndpointSlice cannot be read.
   This includes the Istio ingress-gateway workload behind its fronting Service,
   not only application destination Services.
   A successful empty List is available data: a Service with zero slices is
   known-unready. Only a failed List permits the PodReady fallback.

## Istio

Istio splits what an Ingress does into two objects: a **Gateway** (where traffic
enters — which ingress-gateway workload, which port, which TLS certificate, which
hosts) and a **VirtualService** (where a host + path is then routed).
`istioflow.go` flattens that pair back into the same three lanes.

- **No Istio Go module is a dependency.** Objects are read through the dynamic
  client via discovery, so a cluster without Istio simply contributes nothing.
  A genuinely absent CRD is quiet, while discovery, RBAC, timeout and list errors
  are returned as partial-view warnings rather than disguised as "no Istio".
  An absent optional kind is negatively cached for one minute; after that TTL a
  refresh re-runs discovery so a newly installed Istio CRD becomes visible.
- **Gateways are listed cluster-wide even in a scoped view.** The standard layout
  puts the Gateway in `istio-system` and the VirtualServices next to the app;
  scoping the Gateway list would hide the entry point for every namespace but one.
  An out-of-scope Gateway is only shown when something in scope binds to it.
- **Scope-sensitive claims are hedged.** "No VirtualService binds to this Gateway"
  names the namespace when the view is filtered, because only in-scope
  VirtualServices were listed. A destination outside the scope is fetched directly
  (`lookupOutOfScope`) rather than reported as a missing Service.
- **Destination hosts resolve short, namespaced or FQDN** (`nexus`,
  `nexus.nexus`, `nexus.nexus.svc.cluster.local`). A host that resolves to nothing
  and looks external is marked *External*, not broken.
- `mesh` in a VirtualService's `gateways` list is skipped — that is in-cluster
  traffic, not an entry point.

### The two checks that earn this view its keep

Both are classic Istio failures that otherwise surface only as a connection reset
at request time:

- **A Gateway selector matching no ingress-gateway Pod** — nothing is listening.
- **A `credentialName` TLS secret absent from the *ingress gateway's* namespace**
  rather than the app's. Getting that wrong is the single most common Istio HTTPS
  mistake.

## Extending to another ingress implementation

`istioflow.go` is the template. Produce `FlowIngress` values with `Kind` set,
`RefKind` set to something [kind-resolution.md](kind-resolution.md) can resolve,
mark backing Services in the shared `fronted` map so they don't also appear as
"internal only", and append in `NetworkTopology`. Gateway API
(`gateway.networking.k8s.io` — Gateway/HTTPRoute) is the obvious next one.

## Verify

```powershell
go run ./cmd/kubby-cli netflows            # all namespaces
go run ./cmd/kubby-cli netflows -n <ns>    # scoped — check the hedged warnings
```

Worth re-checking after a change: a Gateway in `istio-system` with its
VirtualService elsewhere; a route to a Service in a third namespace; a deleted-but-
finalizing object; a destination host outside the cluster.
