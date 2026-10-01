# Cluster & clients

← [Architecture skeleton](../ARCHITECTURE.md)

Owns `client.go` (the `Cluster` struct and how it is built) and the multi-cluster
registry in `app.go`. Nearly every other branch starts by picking one of these
clients, so **the choice table below is the part worth remembering**.

## Which client to use

`Cluster` bundles six clients plus a `rest.Config`. They are not interchangeable;
using the wrong one is the cause of several past bugs.

| Field | Use it for | Never use it for |
|---|---|---|
| `Clientset` | Typed list/get of built-in kinds, where you need computed status (a Pod's phase, a Deployment's readiness) | Counting. It transfers every object body. |
| `Meta` | **Counting and name lookups.** Returns `PartialObjectMetadata` — names and labels, no bodies | Anything needing `spec`/`status` |
| `Dynamic` | Generic get/update/delete/list of *any* kind, including CRDs | Kinds where the typed client gives you a better shape for free |
| `Discovery` | What this cluster actually serves. Feeds the cached kind index | Direct use — go through [kind-resolution.md](kind-resolution.md) instead |
| `Metrics` | metrics.k8s.io usage data | Assuming it exists — **it may be nil** |
| `Stream` | Following logs only | Anything else |
| `Rest` | Port-forward, exec, Helm — long-lived connections | Ordinary requests |

Two of these carry a rule strong enough to have caused a real bug:

- **`Metrics` may be nil.** metrics-server is not installed everywhere. Every
  caller must be nil-safe; `NodeMetrics` returns `(nil, nil)` and the UI shows a
  hint rather than an error. The local kind cluster *does* have it, so this path
  needs forcing to test — see [verification.md](verification.md).
- **`Stream` and `Rest` have no timeout.** `Clientset`/`Dynamic` carry a 30 s
  timeout, which would abort a live log tail, a held port-forward tunnel, or an
  interactive exec session. See [streaming.md](streaming.md).

## Timeouts and rate limits

```go
restConfig.QPS   = 50   // client-go default: 5
restConfig.Burst = 100  // client-go default: 10
restConfig.Timeout = connectTimeout // 30s
```

**Do not restore the client-go defaults.** They are tuned for a controller that
must not overwhelm a shared API server; for an interactive UI they were the single
largest source of lag. Full reasoning and measurements in
[performance.md](performance.md).

The **30 s connect timeout** exists because on this machine a corporate security
agent inspects the first TLS connection from a new binary, making the first
connect take 15–30 s. That is expected, not a hang — the welcome screen shows a
"Connecting…" overlay saying so. Shortening this timeout will make Kubby look
broken on its author's own laptop.

## Connecting

Two entry points, both ending in `clusterFromRest`:

- `New(kubeconfigPath, context)` — from a file path.
- `NewFromContent(bytes, context)` — from pasted kubeconfig text.

An empty `context` means "use the kubeconfig's current-context".
`Contexts(path)` / `ContextsFromContent(bytes)` list the available contexts so the
welcome screen can offer a picker before connecting.

## Multi-cluster

`App` (in `app.go`) holds the registry:

```go
clusters map[string]*clusterEntry // stable connection ID → entry
order    []string                 // connection IDs → stable dropdown order
activeID string
```

The ID is deliberately separate from the context/display name. Two unrelated
kubeconfigs commonly both call their context `default` or `admin`; context name
therefore cannot be a registry key. The first display label stays `default`, the
next becomes `default (2)`, while frontend option values and backend switching use
the stable IDs.

Cluster state is protected by `stateMu`. A bound method calls `requireCluster()`
once and keeps the returned pointer as its immutable snapshot for the whole I/O;
it must not re-read active state after the call has started. Every UI write also
carries the expected connection ID into an `*Owned` App method, so a confirmation
opened on cluster A cannot mutate a same-named object after switching to B. Add, switch and
disconnect serialize through `transitionMu`, advance `connectionEpoch`, publish
one active entry, and invalidate the old log/exec/port-forward lifecycle. Session
close happens outside its registry lock.

On switching or disconnecting a cluster the frontend must rebuild everything
cluster-derived — namespace options, sidebar counts, and the dynamic custom-resource
sections. Forgetting the last two left the sidebar describing the previous cluster
(a real bug).

Connectivity is proved with `Discovery.ServerVersion`, not a cluster-wide
Namespace list. A namespace-scoped ServiceAccount is a valid connection; normal
RBAC probes and API errors describe what it may access afterwards.

## Recent connections

`recent.go` persists to `kubby/recent.json` under `os.UserConfigDir()`
(`%AppData%` on Windows, `~/Library/Application Support` on macOS).

Kubeconfigs are not passive data. Before creating a transport, `New` and
`NewFromContent` validate only the selected context and reject exec/auth-provider
plugins, token/certificate/key file references, and kubeconfig-defined proxies.
Those fields can launch a process or read local files under the desktop user's
identity. Supporting them requires an explicit high-risk approval UI; do not
weaken this check merely to make a cloud-provider config connect silently.

The file is updated under one App mutex with a private temporary file, `Sync`,
and atomic rename. This prevents overlapping welcome-screen connects/forgets from
losing updates or exposing a partially-written JSON document.

> **Security rule:** it stores **only file paths + context names**. Pasted
> kubeconfig *content* is never written to disk — it can contain client
> certificates and tokens. If you extend this file, keep that boundary.

## Verify

```powershell
go run ./cmd/kubby-cli get pods -n default --kubeconfig <path> --context <ctx>
```
A successful list proves the whole connection path. See
[verification.md](verification.md).
