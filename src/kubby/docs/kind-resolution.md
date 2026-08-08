# Kind resolution

← [Architecture skeleton](../ARCHITECTURE.md)

Owns `apiindex.go`. This is the layer that makes Kubby work with kinds nobody
hard-coded — every CRD, from Istio to cert-manager to whatever an operator
installed this morning.

## The problem it solves

Kubby started with a static `kind → GroupVersionResource` table (`gvrByKind` in
`detail.go`). A table can only ever describe built-in kinds, so anything from a
CRD was rejected with `unsupported kind "VirtualService"`. That single limitation
broke applying Istio manifests, opening a custom resource's drawer, and finding
one in search.

`ResolveKind` / `ResolveGVK` ask the API server instead.

## The two entry points

```go
// From a UI reference — the kind may carry its group.
func (c *Cluster) ResolveKind(kind string) (APIKind, error)      // "Pod", "VirtualService.networking.istio.io"

// From a YAML document — exact, version and all.
func (c *Cluster) ResolveGVK(apiVersion, kind string) (APIKind, error)

type APIKind struct {
    GVR        schema.GroupVersionResource
    Kind       string
    Namespaced bool   // ← use this, not a hard-coded scope list
}
```

`resourceFor(c, kind, namespace)` in `detail.go` wraps `ResolveKind` and hands
back a correctly-scoped `dynamic.ResourceInterface`. Most callers want that rather
than the raw resolver.

## How the index is built

One `Discovery.ServerGroupsAndResources()` call, cached on the `Cluster`, indexed
two ways:

- **`byGVK`** — every served version. A manifest pinning
  `networking.istio.io/v1beta1` is sent to *that* version, even if the cluster
  prefers `v1`.
- **`byKind`** — each group's preferred version only, keyed by bare kind. A kind
  can exist in several groups, hence a slice.

Built-in kinds short-circuit through `gvrByKind` before any of this, so the common
path costs no round-trip.

## Invariants that are easy to break

1. **A partial discovery failure is kept, not fatal.** One stale aggregated
   APIService (a leftover `metrics.k8s.io` is the usual culprit) makes discovery
   return an error *alongside* everything that worked. Dropping the whole index
   over that would break resolution for every healthy group.
2. **A miss triggers exactly one refresh and retry.** That is what lets a CRD
   installed after you connected be picked up without reconnecting. Do not turn it
   into a loop.
3. **Ambiguity is an error, not a guess.** A cluster running both Istio and
   Gateway API serves two different `Gateway`. Rather than pick one, `ResolveKind`
   fails and tells you to qualify it.

## The `Kind.group` convention

Custom resources are referenced through the UI kubectl-style:
`VirtualService.networking.istio.io`.

This is deliberate and worth preserving: it keeps the entire frontend passing a
**single kind string** — `GetDetail(kind, ns, name)`, `GetYAML(...)`,
`DeleteResource(...)`, drawer refs, palette hits — while staying unambiguous.
Threading a separate `apiVersion` parameter through every one of those signatures
was the alternative.

Two consequences to respect:

- **`bareKind(kind)`** strips the group wherever Kubernetes itself wants the plain
  kind. The live example is an Event's `involvedObject.kind` field selector, which
  would never match `VirtualService.networking.istio.io`.
- **The drawer header shows only the part before the first dot**, so the user sees
  `VirtualService`, not the qualified reference.

## Extending

- Adding a **built-in** kind: put it in `gvrByKind` (+ `clusterScopedKinds` if
  cluster-scoped) purely to skip a discovery round-trip. It works without that.
- Adding a **custom** kind: nothing to do. It already resolves.
- Needing a resolver that takes only a kind and must never be ambiguous: pass the
  qualified `Kind.group` form.

## Verify

```powershell
go run ./cmd/kubby-cli yaml VirtualService.networking.istio.io <name> -n <ns>   # qualified
go run ./cmd/kubby-cli yaml VirtualService <name> -n <ns>                        # bare, via discovery
go run ./cmd/kubby-cli list-custom Nonexistent.example.com                       # the error path
```
