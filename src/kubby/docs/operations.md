# Operations (writes)

← [Architecture skeleton](../ARCHITECTURE.md)

Owns `actions.go` and the write paths reached from the drawer and the ⋯ row menu.

## The rule

> **No write happens without an explicit confirmation.** This is NFR-3 in the
> [specification](../../../docs/SPECIFICATION.md), not a style preference. Every
> action here goes through `showConfirm()` before it calls anything.

Destructive actions use `danger: true`, which turns the OK button red.

## What exists

| Action | Kinds | Notes |
|---|---|---|
| Scale | Deployment | Modal prefilled with the current replica count |
| Restart (rolling) | Deployment, StatefulSet, DaemonSet | Patches `kubectl.kubernetes.io/restartedAt`, the same mechanism `kubectl rollout restart` uses |
| Pause / resume rollout | Deployment | |
| Rollout history + rollback | Deployment | History from the owned ReplicaSets |
| Cordon / uncordon | Node | |
| Drain | Node | Evicts non-DaemonSet pods |
| Trigger now | CronJob | Creates a Job from the CronJob's template |
| Delete | any kind | Single row, or bulk via checkbox selection |
| Edit YAML | any kind | See [apply-yaml.md](apply-yaml.md) |

Delete resolves its target through [kind-resolution.md](kind-resolution.md), so it
works for custom resources too.

## Adding an action

1. `actions.go` — a function taking `(ctx, c, namespace, name, …)`. Use the typed
   client where a typed API exists (scale has a subresource; a rolling restart is a
   patch), the dynamic client otherwise.
2. `app.go` — a thin bound method after `a.requireCluster()`.
3. `main.js` — a button in the drawer or an entry in `wireRowActions`, wrapped in
   `showConfirm()`, then `refreshCurrentView()` afterwards so the table reflects it.
4. `cmd/kubby-cli` — a command, so it can be verified without the GUI.

## Gaps worth knowing

Not implemented; listed so nobody assumes they are:

- **Delete shows no impact preview.** Deleting a Deployment and deleting a
  Namespace are very different acts, and the dialog treats them the same. A
  namespace delete should say how many objects it takes with it, and ideally
  require the name to be typed.
- **No audit trail.** Nothing records what Kubby wrote, when. For a tool pointed
  at production that is a cheap and valuable addition.
- **No RBAC awareness.** Actions the current token cannot perform are still
  offered and fail at the API. `SelfSubjectAccessReview` would let them be disabled
  up front.

## Verify

```powershell
go run ./cmd/kubby-cli scale <deployment> -n <ns> --replicas N
go run ./cmd/kubby-cli restart <deployment> -n <ns>
go run ./cmd/kubby-cli rollout <deployment> -n <ns>
go run ./cmd/kubby-cli cordon <node>   /   uncordon <node>
```
