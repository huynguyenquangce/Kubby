# Permissions (what this token may do)

← [Architecture skeleton](../ARCHITECTURE.md)

Owns `access.go` and the greying-out it drives in the frontend.

Before this existed, a read-only kubeconfig still got the full ⋯ menu: you clicked
**Delete**, confirmed a dialog, and *then* got a 403. The gap between what the UI
offers and what the token can do was invisible until you acted.

## The rule that matters most

> **An unanswered question is not a "no".**
>
> When the cluster cannot answer a `SelfSubjectAccessReview` — an old apiserver, a
> proxy in the way, a network blip — every verb is reported as **allowed** and
> `AccessSet.Checked` is `false`.

Get this backwards and Kubby greys out its entire interface for someone who has
every permission, and does it silently. That is a far worse failure than a click
that fails at the API, which is the behaviour this feature replaced. The same rule
is repeated on the JS side (`allowed()` in `main.js`) and pinned by
`TestUnknownPermissionMeansAllowed`.

The apiserver remains the real enforcer either way. Nothing here is a security
boundary — it is a UI honesty feature.

## Contract

```go
func CanI(ctx, c, kind, namespace string) (*AccessSet, error)

type AccessSet struct {
    Kind, Namespace string
    Verbs   map[string]bool   // resource verbs plus exact subresource actions
    Checked bool
}
func (a *AccessSet) Allowed(verb string) bool   // nil-safe; absent == allowed
```

One `SelfSubjectAccessReview` per verb, fanned out concurrently (`accessConcurrency
= 8`), resolved through [kind-resolution.md](kind-resolution.md) so a custom
resource is checkable as `VirtualService.networking.istio.io` like anything else.

## Pod subresources are separate questions

`probesFor` adds three probes for core `pods`, and they are **not** verbs on the Pod:

| Key | Verb | Subresource |
|---|---|---|
| `logs` | `get` | `log` |
| `exec` | `create` | `exec` |
| `portforward` | `create` | `portforward` |

A role granting `get pods` says nothing about reading logs. A role that grants
`pods` but not `pods/log` is real and common, and was verified on the local cluster:
`can-i Pod -n nexus` under such a token returns `get yes` and `logs no`. Conversely,
read-only-plus-exec is a common and dangerous combination — checking the Pod alone
would show Terminal as available.

The group is checked too, so `metrics.k8s.io/pods` is not mistaken for core pods
(`TestNonPodKindsGetNoSubresourceProbes`).

Deployment scaling is also a separate question: key `scale` probes verb `update`
on `apps/deployments/scale`. Permission to update the Deployment object does not
imply permission to update its scale subresource, or vice versa.

## Caching

- **Go side**: `Cluster.access`, keyed `Kind|namespace`, for the life of the
  connection. RBAC rarely changes while the app is open, and a stale *allow* is
  harmless. A probe that **failed** is never cached, so a blip does not stick for
  the session.
- **JS side**: `accessResolved` + `accessPending` in `main.js`. `accessPending`
  exists to dedupe a burst — opening a view fires one probe, not one per row.
- **`clearAccessCache()` on cluster switch and disconnect.** The Go cache is per
  `Cluster` so it separates naturally; the JS one is global and would otherwise
  describe the previous cluster.

## How the UI uses it

Gating is **synchronous by design** — a menu must open on click, not after a round
trip. So:

1. `selectView` warms the probe for the view's kind.
2. `openRowMenu` reads whatever answer has arrived (`accessPeek`) and leaves the
   rest enabled. In practice the warm-up has landed; if it has not, nothing is
   wrongly disabled.
3. `openDrawer` applies gating twice — once from cache, once when the probe
   returns — guarded by `drawerRef === ref` so a fast click-through does not gate
   the wrong drawer.

Row-menu actions carry `need: '<verb>'`, and **the verb is the one the API wants,
not the one the label suggests**:

| Action | Needs | Why |
|---|---|---|
| Scale | `scale` | key for `update` on `deployments/scale` |
| Restart (rolling) | `patch` | it patches a template annotation |
| Pause / resume, cordon / uncordon | `patch` | |
| Trigger now (CronJob) | `create` on **`Job`** | it creates a Job; the CronJob's own verbs are irrelevant — that is what `needKind` is for |
| Save edited YAML | `update` | the drawer sends a full-object `Update`; Create/Import is the separate server-side-apply path |

**Disabled, not hidden.** A greyed-out Terminal tab whose tooltip reads *"Your token
cannot exec into Pod in nexus"* tells the truth. A missing tab would suggest Kubby
has no terminal.

### Two things deliberately not gated

- **Import YAML** — its content can name any kind, so there is nothing specific to
  check up front. The per-document report already names what failed.
- **Drain** — it needs `create` on `pods/eviction` in every namespace it touches,
  which is unknowable from the node's row. It is gated on `patch` on the Node, which
  is the step drain performs first. Stated here so nobody assumes the check is
  complete.

Helm actions are ungated: a release's permissions are the permissions of every
object in it plus the Secret holding its state, which is not one question.

## Verify

```powershell
go run ./cmd/kubby-cli can-i Pod -n nexus
go run ./cmd/kubby-cli can-i Node                                  # cluster-scoped
go run ./cmd/kubby-cli can-i VirtualService.networking.istio.io -n nexus
```

As cluster-admin everything answers `yes`, which proves nothing. Make a restricted
token — the recipe is in [verification.md](verification.md) — and confirm the shape
of the answer: `get`/`list` yes, writes no, `logs` no, and a different namespace
entirely no.
