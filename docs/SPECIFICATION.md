# Kubby — Product & Requirements Specification

> **Status:** Living document · **Last reviewed:** August 2026
> **Audience:** the project owner and any contributor. This document explains *why Kubby exists* and *what it must do*. Start at the repository [`README`](../README.md), use [`BUILD.md`](BUILD.md) for the canonical build procedure, and see [`../src/kubby/ARCHITECTURE.md`](../src/kubby/ARCHITECTURE.md) for implementation structure.

---

## 1. Overview

**Kubby** is a **personal desktop application for managing Kubernetes clusters**. You open it (double-click, no terminal), point it at a `kubeconfig`, and it connects directly to the cluster's API server to browse, inspect, diagnose, and operate resources through a Lens-style graphical interface.

Kubby is deliberately **single-user and agent-less**: it needs nothing installed inside the cluster — only a `kubeconfig`, exactly like Lens, Headlamp, or k9s. It is not an enterprise fleet manager (unlike Rancher) and it is not a read-only viewer (unlike a bare dashboard): beyond listing resources, it performs real operations and, uniquely, offers **AI-assisted diagnosis** of failing resources.

The source lives in this repository under [`../src/kubby/`](../src/kubby/); it is not a separate repo.

## 2. Problem statement

Managing a personal or lab cluster with `kubectl` alone is slow and error-prone: you memorize commands, copy-paste names, and manually correlate events, logs, and YAML to understand *why* something is broken. Existing GUIs solve the browsing problem but stop short of explaining failures. Kubby's thesis: a personal cluster manager should not just *show* you a `CrashLoopBackOff` — it should help you *understand and fix* it.

## 3. Target users

| Persona | Description | Needs |
|---|---|---|
| **Primary — the owner** | A DevOps learner/engineer running personal or lab clusters (kind, minikube, or a small cloud cluster). | Fast visual browsing, one-click operations, log/exec/port-forward, and plain-language help when things break. |
| Usage model | One user, one machine, one active cluster at a time (multiple clusters can be registered and switched). | No multi-user, no authentication layer, no shared server. |

## 4. Product positioning

| Tool | Model | Kubby's difference |
|---|---|---|
| **Lens / OpenLens** | Desktop GUI, agent-less | Kubby is open and self-contained; its evidence-first Incident Studio diagnoses before AI, hands off to confirmed remediation, and verifies recovery. |
| **Headlamp** | Desktop/web GUI, agent-less | Kubby provides a focused troubleshooting loop — deterministic findings, causal evidence, safe next actions and recovery verification — rather than a plugin-oriented general dashboard. |
| **k9s** | Terminal UI | Kubby is a graphical desktop app for users who prefer a mouse-driven, discoverable UI. |
| **Rancher** | Enterprise fleet management, in-cluster components | Kubby is intentionally personal and agent-less — no fleet, no server, no CRDs installed. |

**One-line positioning:** *the personal, agent-less Kubernetes incident workbench: understand the evidence, fix safely, and verify recovery.*

## 5. Core user workflow

1. Launch the app (double-click) → **Welcome** screen.
2. Provide a `kubeconfig` in one of two ways: **pick a file** via the OS dialog, or **paste** its contents into a text box.
3. Choose a **context** and click **Connect**. On success the app switches to the **Dashboard**.
4. The Dashboard lists nodes, namespaces, and workloads, and visually flags resources in error states (`CrashLoopBackOff`, `ImagePullBackOff`, `Pending`, …). A **namespace selector** scopes the view.
5. Inspect and operate resources without typing `kubectl`: view YAML/details/events, stream logs, exec, port-forward, scale/restart/rollback, and ask the AI to explain a failing resource.

## 6. Scope

### 6.1 In scope

- Connecting to existing clusters via `kubeconfig` (file or pasted content), multiple contexts.
- Read/inspect of the common resource kinds across all namespaces or one namespace.
- Write operations with explicit confirmation (edit YAML, scale, restart, rollback, cordon/drain, delete, create/apply).
- Diagnostics: events, logs, live metrics, relationship graphs, network topology, and AI explanations.
- Embedded Helm: manage releases, add/browse repositories, search Artifact Hub, install charts.
- A secondary CLI (`kubby-cli`) sharing the same core logic, for quick debugging and verification.

### 6.2 Out of scope (at least for now)

- **Cluster lifecycle** — creating, upgrading, or deleting clusters.
- **Multi-user / RBAC of the tool itself**, teams, or enterprise fleet management.
- **GitOps enforcement** and a third-party **plugin system**.
- Running any component **inside** the cluster (no agent, no operator).

## 7. Functional requirements

Requirements are grouped by capability. **Status** legend: ✅ implemented · 🟡 partial · ⬜ planned.

### 7.1 Connection & clusters

| ID | Requirement | Status | Notes |
|---|---|---|---|
| FR-1 | Desktop GUI launched by double-click (not CLI-only). | ✅ | Wails app; separate **Welcome** and **Dashboard** screens. |
| FR-2 | Load a `kubeconfig` by file picker **or** by pasting its contents; choose a context; connect. | 🟡 | Embedded certificates/tokens are supported. For this preview, Kubby rejects `exec`/auth-provider plugins, token/certificate/key file references, and kubeconfig-defined proxies before connection because those mechanisms can execute commands or read local files and there is not yet a separate high-risk approval UI. First connect has a visible overlay and a 30 s timeout. |
| FR-3 | Register and switch between **multiple clusters** without returning to Welcome. | ✅ | Sidebar cluster dropdown, **+ Add cluster**, per-cluster disconnect. |
| FR-4 | Remember recent connections for one-click reconnect. | ✅ | Stored at `%AppData%/kubby/recent.json`; **only file paths + context** are persisted — pasted content (which may contain credentials) is never written to disk. |

### 7.2 Resource browsing & inspection

| ID | Requirement | Status | Notes |
|---|---|---|---|
| FR-5 | List the common resource kinds as sortable, filterable tables. | ✅ | ~24 kinds: Nodes, Namespaces, Pods, Deployments, StatefulSets, DaemonSets, Jobs, CronJobs, Services, Ingresses, ConfigMaps, Secrets, PVCs, PersistentVolumes, StorageClasses, ServiceAccounts, Roles, RoleBindings, ClusterRoles, ClusterRoleBindings, plus the **Ecosystem** group (CRDs, Helm Releases, Helm Repos, ResourceQuotas, LimitRanges). ReplicaSets remain visible through workload relationships rather than a first-class table. |
| FR-6 | Namespace selector to scope namespaced views. | ✅ | Applies to all namespaced tables. |
| FR-7 | A detail drawer per resource: metadata, labels, annotations, kind-specific fields, and **Events** (like `kubectl describe`). | ✅ | Slide-over drawer with tabbed content. |
| FR-8 | Relationship graphs with click-to-navigate. | ✅ | Deployment→ReplicaSet→Pod, Service→Pod, Ingress→Service→Pod; each node opens its own drawer. |
| FR-9 | **Node → Pods** and **Namespace overview**. | ✅ | Clicking a Node lists the pods scheduled on it; clicking a Namespace shows per-kind counts (click a card to jump into that view, scoped). |
| FR-10 | **Global search** across kinds and namespaces. | ✅ | Command palette (Ctrl+K) searches real resource names across the cluster and opens the match directly. |
| FR-11 | **Topology · Traffic routes** — the entry-point→Service→Pod topology for the cluster/namespace, for **both Kubernetes Ingress and Istio**. | ✅ | The shared *Topology* workspace under *Network* exposes this as the **Traffic routes** mode. Each request path is laid out in lanes (Route → Service → Pods) with per-hop health, host/path rules, service type/ports and pod readiness; summary tiles count entry points, routed vs. internal services, endpoint pods and **broken paths**; filter by name/host and toggle "only broken paths". A hop that routes nowhere (missing Service, selector matching no Pod, no ready Pod) is called out where it breaks. Objects being deleted are excluded. **Istio** is detected automatically and rendered in the same lanes, badged `Gateway`: listener ports and TLS from the Gateway's servers, the ingress-gateway's own address, routes attributed to the **VirtualService** that created them (clickable), destination hosts resolved short/namespaced/FQDN with out-of-cluster hosts marked *External*, and two Istio-specific checks — a Gateway selector matching no ingress-gateway Pod, and a `credentialName` TLS secret missing from the **ingress gateway's** namespace. Gateways are found cluster-wide even in a namespace-filtered view, since they conventionally live in `istio-system`. |

### 7.3 Health & diagnostics

| ID | Requirement | Status | Notes |
|---|---|---|---|
| FR-12 | Automatically flag resources in error states. | ✅ | Red row + badge for failing Pods/Deployments/etc.; sidebar count badges carry a red dot when a kind has failures. |
| FR-13 | Overview dashboard summarizing cluster health. | ✅ | The first viewport is issue-first: a compact health banner states readiness, the Attention queue lists every failing Pod with local **Inspect** and deterministic **Investigate** next steps, and aggregate CPU/memory capacity follows without duplicating node telemetry (usage requires metrics-server). Infrastructure reports Ready, scheduling enabled/disabled, pressure conditions, pod count and kubelet version. Top CPU consumers and cluster-wide recent events remain available; resource and node rows open their drawer. |
| FR-37 | **Topology · Dependencies** — debug the complete cluster as Entry point → Service → Workload → Pod. | ✅ | Opened from Overview or the shared *Topology* navigation item and namespace-scopable. The **Dependencies / Traffic routes** switch stays in the page heading instead of creating duplicate navigation. Kubernetes Ingress and Istio Gateway entry paths, internal-only Services, and workloads not selected by any Service are all retained. ReplicaSet ownership is collapsed to Deployment; StatefulSet/DaemonSet/Job/standalone Pod ownership remains explicit. Search, “only unhealthy paths”, per-hop status, and a resource inspector lead to the normal drawer/logs. Terminating objects are excluded and all typed lists are joined in memory behind one bound call; also `kubby-cli structure`. |
| FR-36 | **Right-sizing** — say whether what was reserved bears any relation to what is used. | ✅ | A dedicated view under *Cluster*. Kubby already listed ResourceQuotas and LimitRanges and already read metrics-server; neither answers the question on its own, so this joins them: per-container requests and limits against live usage, per-namespace totals with quota headroom, and the cluster's allocatable capacity (Ready nodes only) to give the totals a scale. Findings are named in words rather than left to the reader — OOMKilled, memory near its limit, no memory limit at all, a request being used at 1%. **A value the container never declared is shown as `—`, never `0`**, since several findings exist precisely because nothing was declared. Aggregate conclusions are separated into short advice lines ("18 of 22 containers declare no memory limit; a LimitRange would set a default in one place — these namespaces have none"), because a list that flags every container ranks nothing; a toggle narrows the table to OOMKills and near-limit memory. Thresholds are deliberately conservative, with floors below which "over-provisioned" is not claimed. Works without metrics-server, saying so, since the declaration findings need no measurement. Read-only and entirely derived. Also `kubby-cli sizing`. |
| FR-14 | View Pod/container logs in the UI. | ✅ | Container picker, ~500 lines, **live follow (streaming)**, in-view line filter, download to file. |
| FR-15 | **AI assistant, scoped to a resource.** Ask questions about the resource you are looking at; Kubby attaches its live events, logs and manifest to every question. | ✅ | An **Ask AI** tab in the drawer (also reachable from the row menu): starter questions tuned per kind, a multi-turn thread, copy-answer, and a header stating exactly what evidence is attached with a **"See exactly what is sent"** viewer. One redacted snapshot is reused for the whole thread. Unconfigured shows a setup card, never an error. Provider chosen in **Settings**: **Ollama (loopback-only — nothing leaves the machine)**, **Anthropic (Claude)**, or any **OpenAI-compatible** endpoint; plus an answer-language preference. Config (including the API key) is stored under the current user's config profile, requests mode 0600 where the OS supports POSIX permissions, and is sent only to the chosen provider. |
| FR-40 | **Incident Studio** — investigate a resource with deterministic, evidence-first playbooks before asking AI. | ✅ | The resource drawer exposes **Investigate** and Overview sends failing Pods there directly. One backend snapshot correlates Pod/container state, warning Events, owner rollout, matching Services and EndpointSlice readiness into ranked findings, a causal timeline, related resources and explicit next actions. Kubby states the limits of its evidence instead of claiming a root cause it cannot prove. The report can be refreshed or watched as a post-change health gate until recovery, and exported as a local Markdown incident bundle. No write is performed by the investigation itself; remediation hands off to existing confirmed YAML/rollout/topology/log flows. Also `kubby-cli investigate` for real-cluster verification. |

### 7.4 Operations (writes — always confirmed)

| ID | Requirement | Status | Notes |
|---|---|---|---|
| FR-16 | View and edit YAML, with a diff preview before saving. | ✅ | `status` and `managedFields` are stripped from the editor; **Diff** shows the change against the loaded version. The editor is a real code editor (CodeMirror), not a plain text box: YAML syntax highlighting, line numbers, folding, **Tab that indents**, and an in-editor **Ctrl/Cmd+F** search panel with next/previous match navigation. Line numbers also let a failed apply point at the document that failed, with a red gutter marker on its opening line. The same editor is used by Create, Import YAML and the Helm values boxes. |
| FR-17 | Create resources and apply arbitrary YAML — **any kind the cluster serves, including custom resources**. | ✅ | **+ Create** offers per-kind templates; **Import YAML** applies pasted/edited YAML with create-or-update semantics (server-side apply) and multi-document (`---`) support, à la Rancher. Each document's `apiVersion` + `kind` is resolved against the cluster's own API discovery, so CRD-defined kinds (Istio, cert-manager, Argo, Gateway API…) need no configuration; a CRD installed after connecting is picked up on retry. Server-owned metadata (`resourceVersion`, `uid`, `managedFields`, `status`) is stripped so `kubectl get -o yaml` output can be pasted directly; a namespaced object without `metadata.namespace` goes to `default`. Managed-field conflicts are surfaced rather than silently forcing Kubby to take another manager's fields. Every document is attempted even if one fails, and the result is a per-document report (`created`/`updated`, kind, namespace, apiVersion). Two manifests pasted **without** a `---` separator are rejected with that explanation rather than silently applying only the last. |
| FR-34 | **See what a manifest would change before applying it** — a real dry-run diff, not a client-side guess. | ✅ | A **Preview** button in Create / Import YAML sends every document as a server-side apply with `DryRun: All` and diffs the object the server says would result against the object that is live now, per document, labelled `create` / `update` / `unchanged`. Because the server does the merging, the preview shows its defaulting, admission mutation (an injected sidecar) and field-ownership resolution — none of which a client-side comparison could show. Long objects are collapsed to the changed lines plus context. Two limits are inherent to dry-run and are reported per document rather than hidden: an admission webhook that does not declare `sideEffects: None`/`NoneOnDryRun` makes the apiserver refuse the dry run, and a dry run creates nothing — so a bundle that creates a Namespace and then fills it cannot preview the later documents, which is detected and explained instead of surfacing a bare "not found". Also available as `kubby-cli diff -f <file>`. |
| FR-35 | **Offer only the actions this token can perform.** | 🟡 | Resource and Pod-subresource actions are probed through `SelfSubjectAccessReview` and explicitly denied controls are disabled, never hidden. Unknown probes count as allowed and the API server remains authoritative. Import YAML, parts of Drain, and Helm actions are not yet pre-gated because they span dynamic multi-resource plans — see `docs/permissions.md`. |
| FR-18 | Delete resources with confirmation; bulk-select + bulk delete. | ✅ | Checkbox selection across a table; a single confirm dialog. |
| FR-19 | Workload operations. | ✅ | **Deployment:** scale, restart, pause/resume rollout, rollout history + rollback. **StatefulSet & DaemonSet:** rolling restart. **CronJob:** trigger now. |
| FR-20 | Node operations. | ✅ | Cordon, uncordon, drain (evicts non-DaemonSet pods). |
| FR-21 | Interactive **exec** and **port-forward** from the drawer. | ✅ | Opening a Pod's Terminal tab auto-attaches to its first container and discovers a shell Bash-first (`/bin/bash`, `/usr/bin/bash`, `/bin/ash`, `/bin/sh`), while retaining explicit container/shell overrides and reconnect/disconnect controls. Exec is a real PTY-backed terminal with ANSI rendering, raw shell keys (including Tab/history sequences), Ctrl shortcuts, standard selection copy/paste controls and shortcuts, right-click clipboard behavior, and resize propagation. Clipboard reads use the Wails runtime rather than relying on WebView page permission. The chosen shell decides whether those keys provide completion/history; minimal `sh`/`ash` implementations may not. Multiple concurrent tunnels are presented as compact endpoint cards; a global manager keeps background forwards visible and stoppable after the resource drawer closes. A per-drawer option controls whether a new tunnel survives closing its drawer. |
| FR-22 | Reveal decoded Secret values on demand. | ✅ | Values decoded server-side, shown per key with reveal/hide. |

### 7.5 Helm

| ID | Requirement | Status | Notes |
|---|---|---|---|
| FR-23 | Manage installed releases. | ✅ | The Helm workspace lists only each release's latest revision and separates deployed, pending and failed status. Release detail owns **Resources + live health**, **Values / Manifest / Notes**, and **History** with diff-vs-current and **rollback**; **Upgrade values** requires an exact revision-and-values-bound dry-run preview; **run tests** and **uninstall** require confirmation. |
| FR-24 | Discover and install charts. | ✅ | The workspace Catalog searches **Artifact Hub** or a configured repository; install provides a version dropdown, real default `values.yaml`, README/maintainer/home metadata where available, and an exact chart-digest-bound dry-run preview. A successful install selects its namespace and opens the release. |
| FR-25 | Manage chart repositories. | ✅ | The workspace Repositories tab identifies its **Local machine** scope; add / remove / update repos writes the user's `repositories.yaml`; browse hands off to Catalog. Private-repository credentials remain backend-side and are reused for defaults, preview and install. Repository URLs are clickable. |

Helm is implemented **in-process** via the Helm Go SDK — no `helm` binary and no Tiller. Search/install/browse require internet access.

### 7.6 User experience

| ID | Requirement | Status | Notes |
|---|---|---|---|
| FR-26 | Instant per-view filter, row counts, and mouse/keyboard sorting on every column. | ✅ | Sortable headers expose buttons and `aria-sort`; resource rows, selection, and per-row actions remain reachable without a mouse. |
| FR-27 | Information-rich tables (Age column, per-row ⋯ action menu; Pods also show live CPU/memory, IP, node). | ✅ | Lens/Headlamp-style. |
| FR-28 | Command palette (Ctrl+K) for navigation, namespace/cluster switching, actions, and resource search. | ✅ | Keyboard-driven. Offers every section, including the dynamic custom-resource ones, and the resource search covers **all** kinds the UI has a section for — custom resources included. |
| FR-33 | **Custom resources are first-class sections.** Any kind the cluster's CRDs define gets its own sidebar entry, list view and drawer. | ✅ | A **Custom Resources** group is built from the CRD list at connect time (Istio, cert-manager, Argo, Gateway API…), each with a generic Namespace / Name / Status / Age table drilling into the usual Details / YAML / Events / Delete drawer, and each findable in the command palette. Hidden entirely on a cluster with no CRDs. Deliberately no count badges — that would be one list request per CRD on every refresh. Capped at 60 kinds, with the overflow stated rather than hidden. |
| FR-29 | One clear navigation owner, collapsible sidebar groups, consistent line icons, and polished light/dark modes. | ✅ | The sidebar is the desktop and off-canvas navigation owner; groups persist their state and expand when a deep link is selected. Settings/theme are not duplicated in a second rail. |
| FR-30 | Live auto-refresh mode with visible freshness feedback. | ✅ | Refreshes the current view every 5 s; pauses while a selection/drawer/modal/palette is open. A same-scope refresh preserves the current result while reporting that it is refreshing; first load, empty, partial and failed states remain distinguishable. |
| FR-31 | Native browser popups replaced by in-app dialogs. | ✅ | One theme-aware modal system provides task hierarchy, fixed header/footer, compact/standard/editor/wide sizes, aligned responsive forms, inline errors, focus restoration and focus trapping. Stackable confirm/alert dialogs use explicit success/error/danger tone; destructive confirmation initially focuses Cancel. |
| FR-38 | The complete desktop workflow remains usable under browser/WebView zoom and narrow windows. | ✅ | The shell retains its single navigation owner through an off-canvas sidebar when the persistent sidebar no longer fits; contextual page actions remain reachable; dashboard, topology, drawer and every modal size adapt to the remaining workspace rather than the outer window width. Modal grids collapse to one column while actions remain reachable. Supported verification matrix: 80–200% zoom on common 1366×768 and 1920×1080 displays. |
| FR-39 | The primary workflow is issue-first and accessible. | ✅ | Overview prioritizes the actionable issue queue ahead of aggregate telemetry and keeps diagnosis bound to the selected resource. The resource drawer and command palette own and restore focus, expose dialog/listbox/tab semantics, and do not leave the obscured workspace keyboard-active. Operational text uses a readable type floor and semantic colours retain WCAG AA contrast in both themes. |

### 7.7 Tooling

| ID | Requirement | Status | Notes |
|---|---|---|---|
| FR-32 | A secondary CLI sharing the app's core logic for quick debugging and verification. | ✅ | `kubby-cli` (get, yaml, logs, events, apply, scale, restart, port-forward, exec, cordon/uncordon, rollout, helm-*, search, node-pods, ns-summary, netflows, diag). Not the primary interface — a developer aid. |

## 8. Non-functional requirements

| ID | Requirement | Status | Notes |
|---|---|---|---|
| NFR-1 | **Agent-less.** No components installed in the cluster; only a `kubeconfig`. | ✅ | Matches Lens/Headlamp/k9s. |
| NFR-2 | **Local-only.** Runs on a personal Windows machine with no external infrastructure. | ✅ | Optional outbound dependencies are the user-configured AI provider, Artifact Hub search, and configured Helm repositories/chart downloads. |
| NFR-3 | **Safety.** No write to a resource without explicit user confirmation. | ✅ | Every write path, including YAML Save, pause/resume, cordon/uncordon and Helm Test, crosses a tested confirmation boundary; stale resource/cluster ownership is rechecked before dispatch. Ordinary Apply also refuses to force-take managed fields because that stronger decision has no separate confirmation. |
| NFR-4 | **Credential hygiene.** Never persist pasted kubeconfig content; store the AI API key in the current user's protected configuration area. | ✅ | `recent.json` stores only file paths; `ai.json` lives under the user's config profile and requests mode 0600 where supported (Windows relies on the inherited user-profile ACL), its key is write-only from the WebView, hosted-key endpoints require HTTPS, Ollama is loopback-only, and AI evidence is redacted before preview/transmission. |
| NFR-5 | **Responsiveness under a slow first connection.** The UI must not appear hung during a 15–30 s first TLS handshake. | ✅ | 30 s connect timeout + explanatory overlay. |
| NFR-8 | **A problem must be reportable.** The running build must be identifiable, and its environment capturable, without the user knowing where to look. | ✅ | `internal/buildinfo` is the single version identity for both binaries (`-dev` suffix on unreleased builds; commit/date from Go's embedded VCS stamps once the source is a checkout). **Settings → About** shows it; **Copy diagnostics** captures version, platform, the connected cluster's Kubernetes version and capabilities, the AI provider, and a credential-redacted last error. From the CLI: `--version` and `diagnostics`. Every probe is best-effort. The report never reads the stored API key or kubeconfig content directly, but the last error can contain server/resource details, so it is headed "review before sharing" and says so explicitly. |
| NFR-7 | **A view or namespace change must feel immediate.** Refreshing the sidebar and the current view must not visibly stall the UI. | 🟡 | Primary list/sidebar/Overview paths use **QPS 50 / Burst 100**, concurrent Go fan-out, metadata-only lists, and one bound snapshot per main screen; sidebar fill measured 930 ms → 73 ms on a one-node kind cluster. Command-palette names use a 15-second metadata index; Details uses one concurrent snapshot and cancels it by exact drawer owner on close/replacement. Kubernetes Lists and large DOM tables remain unbounded, so pagination/virtualization and broader view-request cancellation are still open. |
| NFR-6 | **Open source with a clear license.** | ✅ | The repository is released under the MIT License; the authoritative terms are in `LICENSE` at the repository root. |
| NFR-9 | **UI regressions are reproducible before packaging.** | ✅ | Playwright drives the real frontend against a deterministic mock of the generated Wails binding surface. CI gates Go format/test/build/vet and the complete frontend suite before a full Windows Wails build, rejects stale bindings, verifies PE x86-64, and publishes a checksummed short-lived artifact. Mocked browser evidence is explicitly separate from native WebView2 and real-cluster verification. |

## 9. Architecture summary

Kubby is a **Wails v2** desktop application: a **Go** backend compiled into a single executable, with an **HTML/CSS/JS** frontend rendered in the OS WebView (WebView2 on Windows). The frontend never talks to Kubernetes directly — it calls Go methods bound onto the `App` struct, which delegate to `internal/k8sclient`, the package that holds *all* Kubernetes logic (via `client-go`, a dynamic client, a metrics client, and the Helm SDK). The same package backs `kubby-cli`.

```mermaid
flowchart LR
    UI["Frontend (HTML/JS in WebView)"] -->|bound calls| App["App methods (app.go)"]
    App --> K8s["internal/k8sclient"]
    K8s --> API["kube-apiserver (via kubeconfig)"]
    CLI["kubby-cli"] --> K8s
    App -.->|events: logs / exec / port-forward| UI
```

Full detail lives in the developer docs: [`../src/kubby/ARCHITECTURE.md`](../src/kubby/ARCHITECTURE.md) is the skeleton (data flow, directory map, cross-cutting invariants) and routes to one branch per feature area under [`../src/kubby/docs/`](../src/kubby/docs/) — including the "add a resource type" recipe in [`resource-browsing.md`](../src/kubby/docs/resource-browsing.md).

## 10. Glossary

| Term | Meaning |
|---|---|
| **kubeconfig** | The file/credentials describing how to reach a cluster; Kubby's only required input. |
| **Context** | A named (cluster, user, namespace) tuple inside a kubeconfig. |
| **Drawer** | The slide-over panel showing one resource's details/YAML/logs/terminal/port-forward. |
| **Agent-less** | Kubby installs nothing in the cluster; it talks to the API server from the desktop. |
| **Wails** | A framework for building desktop apps with a Go backend and a web frontend in the OS WebView. |
| **Ecosystem group** | The sidebar section for CRDs, Helm, ResourceQuotas, and LimitRanges. |

## 11. Open decisions

1. **Frontend framework** — currently vanilla JS + Vite; migrate to a framework (React/Svelte) only if UI complexity demands it.

---

## Appendix — Owner's notes

<!-- Free space for the project owner. Add or amend requirements above directly; keep this section for scratch ideas and decisions. Not to be pre-filled by tooling. -->
