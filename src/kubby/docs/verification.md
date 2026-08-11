# Verification & dev environment

← [Architecture skeleton](../ARCHITECTURE.md)

How to check that a change works. Read this before claiming something is done.

## The test suite

```powershell
go test ./...
cd frontend
npm test             # deferred-response ownership tests; no GUI required
cd ..
```

| File | Pins |
|---|---|
| `diagnostics_test.go` | the diagnostics report shape, and that it cannot carry the AI API key |
| `internal/k8sclient/apply_test.go` | missing-`---` detection, document splitting, diff-noise stripping, the pending-namespace explanation |
| `internal/k8sclient/access_test.go` | **unknown permission == allowed**, explicit deny respected, pod subresources probed separately |
| `internal/k8sclient/rightsizing_test.go` | unset never rendered as zero, threshold floors, severity order, quota parsing, advice grammar |
| `internal/k8sclient/detail_test.go` | YAML Save cannot change kind, namespace, name, or cluster-scoped identity |
| `internal/k8sclient/portforward_test.go` | concurrent/repeated tunnel close is idempotent and does not panic |
| `app_portforward_test.go` | global tunnel registry preserves target metadata, lists deterministically, invalidates pending starts, and synchronizes List/Stop |
| `frontend/src/request-scope.test.js` | late view/drawer/modal responses cannot overwrite a newer owner; stale Helm/chart values cannot cross modal or version boundaries |
| `frontend/src/port-forward-state.test.js` | hydration/upsert, closed-event removal, drawer-close policy, and late Start ownership |
| `frontend/src/terminal-io.test.js` | raw input ordering, write-error recovery, and valid PTY dimensions |
| `frontend/src/log-buffer.test.js` | 5,000-line cap and frame-coalesced live-log rendering |
| `frontend/src/line-diff.test.js` | diff reconstruction, pathological fallback, and 10,000-line regression budget |
| `internal/k8sclient/exec_test.go` | initial/coalesced terminal resize plus concurrent, unblocking queue close |
| `internal/k8sclient/logstream_test.go` | batch size, quiet-stream timer flush, final flush and cancel semantics |
| `internal/k8sclient/overview_test.go` | one Node/Pod list, partial failures, terminating-Pod exclusion, and 10k benchmark |

None of them need a cluster. Everything else is still verified through
`cmd/kubby-cli` against a real one — **that is a gap, not a design choice** — see
*Worth adding* at the bottom.

## Version & diagnostics

Every bug report starts with "which build". `internal/buildinfo` is the single
answer for both binaries:

```powershell
go run ./cmd/kubby-cli --version        # Kubby 0.1.0-dev go1.26.5 windows/amd64
go run ./cmd/kubby-cli diagnostics      # + the connected cluster's capabilities
```

In the app: **Settings → About** shows the same line, and **Copy diagnostics** puts
a full report on the clipboard (`App.Diagnostics` in `diagnostics.go`, cluster
probing in `internal/k8sclient/diagnostics.go`).

- `buildinfo.Version` keeps a checked-in `-dev` suffix so an unreleased build is
  never mistaken for a release. Normal checkout builds use Go's embedded VCS
  stamps when available. A release explicitly overrides version, commit, and date
  because Wails cross-builds or source archives may omit those stamps; the sole
  command and checklist live in [`docs/BUILD.md`](../../../docs/BUILD.md).
- `wails.json` carries the `info` block (`productVersion`, `companyName`,
  `copyright`), which fills the Windows exe's file-properties metadata. Its
  numeric `productVersion` must match the release version passed through ldflags;
  nothing enforces that equality automatically.

> **The report must never carry a secret.** It reads AI provider and model from
> `GetAIStatus()`, which does not return the key. It includes the API-server
> endpoint and context name because they are usually essential to a diagnosis, and
> is headed "review before sharing" so that stays the user's call. A test asserts
> the key is absent. Keep it that way when you add a field.

Every probe in `Diagnose` is best-effort: a forbidden or missing capability is
*information*, not a failure. The report is produced even from a half-working
connection — which is exactly when it is wanted.

## `kubby-cli` is the harness

It shares `internal/k8sclient` with the app, so a CLI check exercises the same code
the GUI does. Every backend feature here was verified this way.

```powershell
# reads
go run ./cmd/kubby-cli get pods -n default
go run ./cmd/kubby-cli yaml <Kind[.group]> <name> -n <ns>
go run ./cmd/kubby-cli events Pod <name> -n <ns>
go run ./cmd/kubby-cli logs <pod> -n <ns> --tail 100
go run ./cmd/kubby-cli node-pods <node>
go run ./cmd/kubby-cli ns-summary <namespace>
go run ./cmd/kubby-cli search <query>
go run ./cmd/kubby-cli counts [-n <ns>] [--cluster=false]     # timed — the perf path
go run ./cmd/kubby-cli overview                               # timed single-call dashboard snapshot
go run ./cmd/kubby-cli custom-kinds
go run ./cmd/kubby-cli list-custom <Kind.group> [-n <ns>]
go run ./cmd/kubby-cli netflows [-n <ns>]
go run ./cmd/kubby-cli diff -f <file>                         # dry-run: what would change, writes nothing
go run ./cmd/kubby-cli can-i <Kind[.group]> [-n <ns>]         # what this token may do
go run ./cmd/kubby-cli sizing [-n <ns>]                       # requests/limits vs usage
go run ./cmd/kubby-cli diag <Kind[.group]> <name> -n <ns>     # the exact AI evidence
go run ./cmd/kubby-cli helm-search <query>
go run ./cmd/kubby-cli --version
go run ./cmd/kubby-cli diagnostics                            # build + cluster capabilities

# writes
go run ./cmd/kubby-cli apply -f <file>
go run ./cmd/kubby-cli scale <deployment> -n <ns> --replicas N
go run ./cmd/kubby-cli restart <deployment> -n <ns>
go run ./cmd/kubby-cli rollout <deployment> -n <ns>
go run ./cmd/kubby-cli cordon <node> | uncordon <node>
go run ./cmd/kubby-cli port-forward <pod> -n <ns> --remote 8080 --local 0 --hold 30
go run ./cmd/kubby-cli exec <pod> -n <ns> -- "ls -la /"
```

Defaults to `$KUBECONFIG` or `~/.kube/config`; override with
`--kubeconfig <path> --context <name>`.

**Adding a feature means adding its CLI command.** A backend feature with no CLI
entry point cannot be checked without a human driving the GUI.

## Build and environment

Environment preparation, dependency installation, source checks, full Wails
builds, artifact verification, and stale-binary cleanup are owned by the
repository-level **[build guide](../../../docs/BUILD.md)**. Do not duplicate
those commands here.

This document owns behavioural verification after the source can build:
automated tests, version diagnostics, `kubby-cli` checks, real-cluster setup,
and the manual GUI limits below.

## GUI verification is not reliable from a headless session

Screenshot automation works for a passively-rendered window, but
`SetForegroundWindow` / `ShowWindow` / `MoveWindow` have triggered WebView2
focus-handling crashes and stale repaints here.

**Prefer verifying the backend through `kubby-cli`, and ask the user to confirm GUI
behaviour visually.** Do not try to automate clicks through native dialogs.

Mechanical frontend checks that *do* work: the esbuild bundle above, and a script
cross-checking every `$('id')` reference in `main.js` against the ids in
`index.html`.

Port-forward manager manual check: start one tunnel with **Keep running** enabled,
close the resource drawer, and verify the top-bar badge still exposes and can stop
it. Start another with the option disabled and verify drawer close removes it.
Finally, switch clusters during a pending/active start and verify neither the old
tunnel nor a late success appears in the new cluster's manager.

Interactive terminal manual check: open a Pod's Terminal tab and connect, type a
partial path or command and press **Tab**, use **Up** for history, send **Ctrl+C**,
then run `top` (or another cursor-addressing program) and resize the window. Input,
ANSI output, cursor placement, and the remote program's dimensions must all remain
correct. This cannot be claimed from the Node tests or a headless Wails build.
Completion/history are provided by the selected container shell; when checking a
hidden path type its leading dot, and do not expect a minimal `/bin/sh` to behave
like Bash.

Overview visual check: inspect light and dark modes at roughly 1024 px and 1920 px
width. Inter must be used consistently by navigation, buttons, form controls, KPI
values, and tables; Cluster pulse must remain readable; capacity meters must match
their percentages; the Attention panel must not stretch to Top consumers' height;
long event messages must scroll inside their section rather than widening the page.
Also confirm Vietnamese glyphs render without switching to a visibly different
fallback face. These are manual WebView checks, not claims made by `npm test`.

## This machine

- **Windows native:** probe Go, Node.js, Wails CLI and WebView2 before selecting
  it as the build host; do not rely on a stale machine note.
- **WSL2:** supports full Linux Wails builds when GTK/WebKit development packages
  are installed, and runs Docker + kind for the local test cluster
  (`kind-kubby-dev`). WSL2's localhost forwarding makes the cluster's API server
  (`https://127.0.0.1:<port>`) reachable from the Windows-side app. Export its
  kubeconfig with `kind get kubeconfig --name kubby-dev` and point Kubby at that
  file.
- **The corporate proxy blocks image pulls** from `registry.k8s.io` / `docker.io` on
  the kind node, so demo pods may sit in `ImagePullBackOff`. Useful to test
  against; not a bug.
- **metrics-server *is* installed** on the current kind cluster (`kubby-cli
  diagnostics` reports `metrics: available`). The nil-safety rule for
  `Cluster.Metrics` still stands — plenty of clusters lack it, and
  `NodeMetrics` returning `(nil, nil)` is a supported state — but do not assume
  the local cluster is one of them when testing the "no metrics" hint. Force that
  path by scaling metrics-server to zero rather than by hoping.
- **First connect takes 15–30 s** for the security-agent reason above. Expected.

### Making a restricted token (to test permission gating)

As cluster-admin every `can-i` answers `yes`, which proves nothing. Create a
ServiceAccount whose Role grants only `get/list/watch` on pods in one namespace,
mint a token for it, and point `kubby-cli` at a kubeconfig using that token:

```bash
# in WSL, against the kind cluster
kubectl create sa kubby-ro -n nexus
kubectl create role kubby-ro -n nexus --verb=get,list,watch --resource=pods
kubectl create rolebinding kubby-ro -n nexus --role=kubby-ro --serviceaccount=nexus:kubby-ro
TOKEN=$(kubectl create token kubby-ro -n nexus --duration=2h)

kind get kubeconfig --name kubby-dev > /tmp/ro.yaml
KUBECONFIG=/tmp/ro.yaml kubectl config set-credentials kubby-ro --token="$TOKEN"
KUBECONFIG=/tmp/ro.yaml kubectl config set-context ro --cluster=kind-kubby-dev --user=kubby-ro --namespace=nexus
KUBECONFIG=/tmp/ro.yaml kubectl config use-context ro
```

Then `kcli.exe --kubeconfig /path/to/ro.yaml can-i Pod -n nexus` must show
`get`/`list` yes and everything else — including **`logs`**, since the Role grants
`pods` but not `pods/log` — no. Generate the file **inside WSL and copy it out**;
piping it through PowerShell's `Out-File` adds a BOM that client-go cannot parse.

### Testing a feature without the real thing installed

CRD-based features can be exercised offline by applying **minimal CRDs** — the
group, kind, plural, scope, and a schema of
`x-kubernetes-preserve-unknown-fields: true`. The Istio Traffic view was fully
verified this way with no Istio control plane: two CRDs, a fake ingress-gateway Pod
carrying `istio: ingressgateway`, a Service in front of it, and the real Gateway /
VirtualService manifests.

## Worth adding

The highest-value missing piece is **actual tests**. The code has a lot of pure
logic that needs no cluster:

Still untested: `openAIBaseURL`, `splitKindGroup`, `titleFor`, `isFullyReady`,
`readyCondition`, `destinationService` (Istio host parsing), `podStatus`.
(`checkMissingSeparator` and `splitYAMLDocuments` are now covered.)

Frontend request ownership is covered by the built-in Node test runner. Still
untested: `documentStarts` and `parseApplyFailures` in `editor.js`, and
`collapseDiff` in `main.js`.

And above that, `k8s.io/client-go/kubernetes/fake` + `dynamic/fake` +
`metadata/fake` would let `SidebarCounts`, `NetworkTopology`, `istioFlows` and
`ApplyYAML` be tested against synthetic clusters — including the cases that
currently require hand-building CRDs on a kind cluster.
