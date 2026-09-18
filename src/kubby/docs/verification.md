# Verification & dev environment

← [Architecture skeleton](../ARCHITECTURE.md)

How to check that a change works. Read this before claiming something is done.

## The test suite

```powershell
go test ./...
cd frontend
npm test             # deferred-response ownership tests; no GUI required
npm run test:e2e     # Chromium + deterministic Wails/Kubernetes mock
cd ..
```

| File | Pins |
|---|---|
| `diagnostics_test.go` | the diagnostics report shape, and credential redaction for its free-form last error |
| `ai_test.go` | write-only provider keys, hosted endpoint transport rules, loopback-only Ollama, redirect refusal, and exact reviewed evidence in the resource prompt |
| `internal/k8sclient/ai_test.go` | Secret/env/free-text credential redaction before AI preview or transmission |
| `internal/k8sclient/client_safety_test.go` | the selected kubeconfig context rejects executable, file-backed, and proxy credential paths before transport creation |
| `internal/k8sclient/apply_test.go` | missing-`---` detection, document splitting, shared metadata stripping, non-forcing managed-field semantics, and pending-namespace preview explanation |
| `internal/k8sclient/access_test.go` | **unknown permission == allowed**, explicit deny respected, pod subresources probed separately |
| `internal/k8sclient/rightsizing_test.go` | unset never rendered as zero, threshold floors, severity order, quota parsing, advice grammar, and partial-metrics suppression |
| `internal/k8sclient/detail_test.go` | YAML Save cannot change kind, namespace, name, or cluster-scoped identity |
| `internal/k8sclient/portforward_test.go` | concurrent/repeated tunnel close is idempotent; Service resolution follows ready non-terminating EndpointSlices with PodReady fallback |
| `internal/k8sclient/actions_test.go` | partial drain failures and server-generated unique CronJob runs |
| `internal/k8sclient/resources_status_test.go` | order-independent Pod failure precedence, Pending/readiness/init/terminating states, terminal Job conditions, and UID-qualified ownership |
| `internal/k8sclient/netflow_test.go` | EndpointSlice readiness remains authoritative over Pod condition guesses |
| `app_portforward_test.go` | global tunnel registry preserves target metadata, lists deterministically, invalidates pending starts, and synchronizes List/Stop |
| `app_connection_test.go` | duplicate context names retain stable independent IDs; registry switching is race-safe; pending exec shell discovery/port-forward/Helm work cannot survive cluster transitions; stale write ownership is rejected |
| `app_binding_test.go` | exported cluster-write bindings expose only connection-owned variants; Helm writes reject a missing owner ID |
| `app_drawer_test.go` | drawer snapshot cancellation owns one exact operation and stale cleanup cannot release a replacement |
| `release_identity_test.go` | Windows file metadata version remains aligned with the shared checked-in build identity |
| `recent_test.go` | concurrent recent updates remain valid, capped and private |
| `frontend/src/request-scope.test.js` | late view/drawer/modal responses cannot overwrite a newer owner; stale destructive completions cannot close a newer drawer; stale Helm/chart values cannot cross modal or version boundaries |
| `frontend/src/keyed-request.test.js` | same-resource remount preserves an AI request owner while a newer request/resource invalidates stale work |
| `frontend/src/markup.test.js` | one Helm workspace owns Releases/Catalog/Repositories; exact-preview ownership; shared modal hierarchy/focus contract; late loader ownership; lazy xterm imports; and cluster-switch keyboard focus |
| `frontend/src/port-forward-state.test.js` | hydration/upsert, closed-event removal, drawer-close policy, and late Start ownership |
| `frontend/src/terminal-io.test.js` | raw input ordering, write-error recovery, and valid PTY dimensions |
| `frontend/src/log-buffer.test.js` | 5,000-line cap, frame coalescing, incremental append and oldest-chunk trimming |
| `frontend/src/line-diff.test.js` | diff reconstruction, pathological fallback, and 10,000-line regression budget |
| `frontend/src/responsive.test.js` | canonical off-canvas navigation, reachable contextual page actions, workspace container breakpoints, viewport-bounded overlays, low-height Terminal compaction, and single-column modal forms/actions |
| `frontend/e2e/ui.spec.js` | paste/connect workflow, every built-in view, two-axis responsive/zoom matrix, paged/virtualized Pods and Custom Resources, explicit apply-permission denial, content-owned table scrolling, mobile navigation ownership, Structure filtering, Incident Studio evidence/export/safe hand-off, Pod drawer/YAML with exact snapshot cancellation, stale-delete isolation, Terminal behavior, Settings draft ownership, and Helm detail/install/late-values/repository/destructive-dialog behavior |
| `frontend/e2e/visual.spec.js` | stable Chromium baselines for the compact command bar, light/dark Overview, Cluster Structure, Incident Studio, the Pod Terminal, and narrow light/dark Pods; failures retain screenshot, video, and trace evidence |
| `internal/k8sclient/exec_test.go` | Bash-first shell selection/fallback/error reporting, initial/coalesced terminal resize, and concurrent unblocking queue close |
| `internal/k8sclient/logstream_test.go` | rate-limited full batches, quiet-stream timer flush, final flush and cancel semantics |
| `internal/k8sclient/overview_test.go` | one Node/Pod list, 30-second recent-Event reuse, partial failures, terminating-Pod exclusion, and 10k benchmark |
| `internal/k8sclient/explore_test.go` | global-search metadata index reuse and concurrent cold-refresh coalescing |
| `internal/k8sclient/investigation_test.go` | Pod phase recovery guard, failure/owner/EndpointSlice evidence, safe next actions, and bounded export |
| `internal/k8sclient/structure_test.go` | complete entry/internal/unexposed topology, ReplicaSet→Deployment collapse, unhealthy propagation, terminating-Pod exclusion, and one list per kind |
| `internal/k8sclient/helmreleases_test.go` | metadata-only Helm storage listing, newest-revision deduplication, terminating-release exclusion, and pending status classification |
| `internal/k8sclient/helm_test.go` | chart-source setup, pinned preview digests, PodReady/termination health, and request-context cancellation |
| `internal/k8sclient/httpbody_test.go`, `artifacthub_test.go` | exact response-size ceilings, chunked overflow, and bounded Artifact Hub decoding |
| `internal/k8sclient/drawer_test.go` | one concurrent Details snapshot carries required detail plus best-effort Events/relationship expansions without duplicate API actions |
| `internal/k8sclient/custom_integration_test.go` | fake discovery and dynamic clients prove group-qualified CRDs flow from sidebar discovery through namespaced listing/status |
| `internal/k8sclient/permissionplan_test.go` | permission-plan deduplication, explicit deny, unknown-is-allowed, and page-limit/token invariants |
| `frontend/src/virtual-table.test.js` | virtual table accessibility, paging callback, detached sort and selection contracts |
| `frontend/e2e/accessibility.spec.js` | narrow light/dark accessible names, duplicate IDs, viewport containment, table row counts, modal inertness and focus trapping |

None of the unit/browser tests need a cluster. The Playwright suite needs its
pinned Chromium and Linux system libraries installed as described in
`docs/BUILD.md`. Behaviour beyond the disposable smoke below is still verified
through `cmd/kubby-cli` or the documented native/manual checks; those remaining
gaps are not treated as proof.

CI additionally creates a disposable kind cluster pinned by image digest and
runs `scripts/verify-kind.sh`. It applies synthetic Namespace/Pod/CRD fixtures,
then checks diagnostics, Pods, drawer, CRD discovery, custom-resource listing,
and apply permission planning through the built `kubby-cli`. The fixture has no
credentials or Secret data and the cluster is deleted by the script's exit trap.

## Version & diagnostics

Every bug report starts with "which build". `internal/buildinfo` is the single
answer for both binaries:

```powershell
go run ./cmd/kubby-cli --version        # Kubby 0.2.0-dev go1.26.6 windows/amd64
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
  `copyright`), which fills the Windows exe's file-properties metadata.
  `release_identity_test.go` enforces that its numeric `productVersion` matches
  the checked-in `buildinfo.Version` after the required `-dev` suffix is removed;
  release ldflags must use that same numeric version.

> **Treat diagnostics as user-reviewed data.** It reads AI provider and model
> from `GetAIStatus()`, which does not return the configured key, and the
> credential redactor covers that key plus common token/password patterns. It
> includes the API-server endpoint, context name, and a best-effort-redacted
> free-form last error because they are useful for diagnosis. That error may
> still contain resource or server-derived details under an unfamiliar shape,
> so the report is headed "review before sharing". Keep the redaction tests and
> this warning current whenever a field is added.

Every probe in `Diagnose` is best-effort: a forbidden or missing capability is
*information*, not a failure. The report is produced even from a half-working
connection — which is exactly when it is wanted.

## `kubby-cli` is the harness

It shares `internal/k8sclient` with the app, so a CLI check exercises the same code
the GUI does. Use the matching command whenever one exists; some multi-resource
write workflows still rely on focused tests plus the documented native check.

```powershell
# reads
go run ./cmd/kubby-cli get pods -n default
go run ./cmd/kubby-cli yaml <Kind[.group]> <name> -n <ns>
go run ./cmd/kubby-cli events Pod <name> -n <ns>
go run ./cmd/kubby-cli logs <pod> -n <ns> --tail 100 [--previous]
go run ./cmd/kubby-cli get hpas|pdbs|networkpolicies [-n <ns>]
go run ./cmd/kubby-cli node-pods <node>
go run ./cmd/kubby-cli ns-summary <namespace>
go run ./cmd/kubby-cli search <query>
go run ./cmd/kubby-cli counts [-n <ns>] [--cluster=false]     # timed — the perf path
go run ./cmd/kubby-cli overview                               # timed single-call dashboard snapshot
go run ./cmd/kubby-cli drawer <Kind[.group]> <name> -n <ns>  # one Details/Events/relationship snapshot
go run ./cmd/kubby-cli investigate Pod <name> -n <ns>        # deterministic incident report; read-only
go run ./cmd/kubby-cli structure [-n <ns>]                   # full debug topology snapshot
go run ./cmd/kubby-cli custom-kinds
go run ./cmd/kubby-cli list-custom <Kind.group> [-n <ns>]
go run ./cmd/kubby-cli netflows [-n <ns>]
go run ./cmd/kubby-cli netpol-check <pod> Pod|Service/<name> -n <ns> [--to-namespace <ns>] [--port N]  # NetworkPolicy verdict
go run ./cmd/kubby-cli containers <pod> -n <ns>                 # restart count, last exit, previous logs available
go run ./cmd/kubby-cli drain-impact <node>                      # refusing PDBs, unmanaged Pods, emptyDir; read-only
go run ./cmd/kubby-cli checks [-n <ns>]                          # webhooks, certificates, stuck deletions; read-only
go run ./cmd/kubby-cli diff -f <file>                         # dry-run: what would change, writes nothing
go run ./cmd/kubby-cli can-i <Kind[.group]> [-n <ns>]         # what this token may do
go run ./cmd/kubby-cli plan-apply -f <file>                    # exact server-side-apply RBAC plan
go run ./cmd/kubby-cli plan-drain <node>                       # Node/list/per-namespace eviction plan
go run ./cmd/kubby-cli plan-helm <action> <release> -n <ns>   # uninstall/rollback/test plan
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

**A backend capability that needs real-cluster verification should add its CLI
command.** Until it does, record the test/manual evidence explicitly rather than
claiming it was exercised through `kubby-cli`.

## Build and environment

Environment preparation, dependency installation, source checks, full Wails
builds, artifact verification, and stale-binary cleanup are owned by the
repository-level **[build guide](../../../docs/BUILD.md)**. Do not duplicate
those commands here.

This document owns behavioural verification after the source can build:
automated tests, version diagnostics, `kubby-cli` checks, real-cluster setup,
and the manual GUI limits below.

## Browser automation is not native WebView verification

The Playwright suite loads the production Vite entry point and injects a
deterministic implementation of `window.go.main.App` plus Wails runtime events
before `main.js` executes. It is safe to click and gives reproducible coverage of
DOM behavior, async binding ownership, keyboard dismissal, CSS breakpoints, and
visual layout without a kubeconfig or cluster:

```bash
cd frontend
npm run test:e2e
```

The supported responsive matrix uses CSS-viewport equivalents of 80–200% zoom
on 1366×768 and 1920×1080 windows, plus a 390 px narrow shell. Visual baselines
live next to `visual.spec.js`; update them only after inspecting and approving an
intentional UI change with `npm run test:e2e:update`. CI retains the HTML report,
trace, screenshot, and video only on failure. Generated failure evidence is
ignored by Git.

This proves frontend behavior against the mocked binding contract. It does not
prove native file dialogs, clipboard behavior, WebView2 focus/paint behavior,
PTY/ANSI rendering, or Kubernetes correctness.

Native window automation is still not reliable from a headless session.

Screenshot automation works for a passively-rendered window, but
`SetForegroundWindow` / `ShowWindow` / `MoveWindow` have triggered WebView2
focus-handling crashes and stale repaints here.

**Prefer verifying the backend through `kubby-cli`, and ask the user to confirm GUI
behaviour visually.** Do not try to automate clicks through native dialogs.

The browser suite is therefore a release gate alongside the native manual checks
below, not a replacement for them.

Port-forward manager manual check: start one tunnel with **Keep running** enabled,
close the resource drawer, and verify the top-bar badge still exposes and can stop
it. Start another with the option disabled and verify drawer close removes it.
Finally, switch clusters during a pending/active start and verify neither the old
tunnel nor a late success appears in the new cluster's manager.

Multi-cluster identity manual check: add two kubeconfigs whose selected context is
named `default`. The dropdown must show `default` and `default (2)`; switching each
must reach its own API server, and disconnecting one must leave the other usable.

Interactive terminal manual check: open a Pod's Terminal tab and verify it attaches
without a Connect click, prefers Bash when the image provides it, and reports the
fallback shell truthfully otherwise. Type a partial path or command and press
**Tab**, use **Up** for history, select output and copy it through the button and
Ctrl/Cmd+Shift+C, paste a multi-line command through the button and
Ctrl/Cmd+Shift+V, verify right-click copies a selection or pastes with no selection,
then send plain **Ctrl+C** to the remote process,
then run `top` (or another cursor-addressing program) and resize the window. Input,
ANSI output, cursor placement, and the remote program's dimensions must all remain
correct. This cannot be claimed from the Node tests or a headless Wails build.
Completion/history are provided by the selected container shell; when checking a
hidden path type its leading dot, and do not expect a minimal `/bin/sh` to behave
like Bash.

Overview visual check: inspect light and dark modes at roughly 1024 px and 1920 px
width. Inter must be used consistently by navigation, buttons, form controls, KPI
values, and tables; the health banner and issue-local next steps must remain readable; capacity meters must match
their percentages; Node status must show readiness/pressure without repeating the
capacity meters; the Attention panel must not stretch to Top consumers' height;
long event messages must scroll inside their section rather than widening the page.
Also confirm Vietnamese glyphs render without switching to a visibly different
fallback face. These are manual WebView checks, not claims made by `npm test`.

Incident Studio manual check: use a Pod with a retained warning Event and a
matching Service/EndpointSlice. Open it from Overview's **Investigate** action;
confirm the highest-severity finding is first, related resources open the exact
drawer identity, and remediation buttons lead to the existing logs/YAML/rollout
or topology flow without writing. Start **Watch recovery**, fix the Pod through a
separately confirmed action, and confirm Kubby reports recovery only after a
fresh healthy snapshot. Export the Markdown report and verify it contains no raw
manifest, Secret value, kubeconfig or application logs. Repeat at narrow width
and in both themes. Native WebView behaviour remains a manual check.

Topology Dependencies visual check: open it from Overview, switch between all namespaces
and a busy namespace, filter by a pod/service name, then enable **Only unhealthy
paths**. At 1024 px and 1920 px in light/dark mode, all four lanes must stay legible;
select each hop and confirm the inspector identity matches it. **Open full details**
must open the same resource, and **View logs** must appear only for Pods. Confirm
internal Services and unexposed workloads remain present. This requires a native
WebView and is not proven by the headless build.

Responsive/zoom visual check: on Windows WebView2, use 1366×768 and 1920×1080
windows at 80%, 100%, 125%, 150%, 175% and 200%. Visit Overview, both Topology modes,
a resource table, a Pod drawer (Details/YAML/Terminal/Port Forward), Settings
and a Helm modal. No shell control may leave the window; only the resource table's
own scrolling region may scroll horizontally. When the sidebar collapses, the menu
button must expose the same namespace, cluster and dynamically discovered resource
navigation, then close through selection, backdrop and Escape. This manual native
check remains required even when the passive Edge layout measurements pass.

Helm visual check: in light and dark mode, visit Releases, Catalog, and
Repositories at desktop width and at 150–200% zoom. Confirm the workspace tabs
retain focus/selection, summary cards do not imply pending releases are failures,
and repository scope says Local machine. Open a release and traverse every detail
tab; resource health must distinguish healthy, pending, degraded, missing,
forbidden, and unknown. For Install and Upgrade, edit a field after preview and
confirm the primary action disables until previewed again. Installing into a new
namespace must select that namespace and open the new release. Private-repository
checks require a test repository and must confirm credentials never render in the
DOM, errors, or screenshots. This is a native WebView check; headless tests and a
successful Wails build do not prove it.

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

Still untested: `openAIBaseURL`, `isFullyReady`, and `destinationService`
(Istio host parsing). The custom-resource integration test now exercises
`splitKindGroup`, `titleFor`, and `readyCondition` through their public workflow.
(`checkMissingSeparator` and `splitYAMLDocuments` are now covered.)

Frontend request ownership is covered by the built-in Node test runner. Still
untested: `documentStarts` and `parseApplyFailures` in `editor.js`, and
`collapseDiff` in `main.js`.

`k8s.io/client-go/kubernetes/fake` + `dynamic/fake` + `metadata/fake` now cover
the first end-to-end custom-resource discovery/listing path. Extending that
synthetic-cluster layer to `SidebarCounts`, `NetworkTopology`, `istioFlows` and
`ApplyYAML` would cover cases that still require hand-building CRDs on a kind
cluster.
