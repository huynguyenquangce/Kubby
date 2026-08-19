# Kubby

A **personal Kubernetes desktop app**: open it, load a `kubeconfig`, and immediately see your cluster's nodes, namespaces, and pods — and know at a glance which ones are broken. No in-cluster agent, no hand-typed `kubectl`.

- **Why it exists & what it must do:** [`../../docs/SPECIFICATION.md`](../../docs/SPECIFICATION.md)
- **How it's built (read before changing code):** [`ARCHITECTURE.md`](ARCHITECTURE.md) — the architecture skeleton, which routes to one doc per feature area under [`docs/`](docs/)

## Highlights

- **Agent-less** — connects straight to the API server with your `kubeconfig`, like Lens/Headlamp/k9s.
- **AI failure explanations** — click a broken pod and get a plain-language "what's wrong and how to fix it" (§ *AI setup* below). This is Kubby's differentiator.
- **Embedded Helm** — manage releases, browse repositories, search Artifact Hub, and install charts, all in-process (no `helm` binary).
- **Diagnose and operate** — logs, exec, port-forward, scale/restart/rollback, drain, and more, each behind a confirmation.

---

## Development and build

The canonical dependency and full-build procedure is maintained at the
repository level in [`../../docs/BUILD.md`](../../docs/BUILD.md). It covers
native PowerShell, WSL/Linux, Windows cross-builds, verification, and artifact
cleanup. Build commands are intentionally not duplicated here.

After preparing the environment from that guide, use `wails dev` for the local
hot-reload development loop.

## First use

On the **Welcome** screen, load a `kubeconfig` one of two ways:

- **Select file** → *Browse for kubeconfig file…*, or
- **Paste kubeconfig** → paste the file's contents.

Pick a **Context**, then **Connect**. The first connection may take 15–30 s if a security agent inspects it — a "Connecting…" overlay explains this; later connections are instant.

> **Try it with a local cluster (kind):**
> ```bash
> kind create cluster --name kubby-dev
> kind get kubeconfig --name kubby-dev > kubby-dev-kubeconfig.yaml
> ```
> Then load `kubby-dev-kubeconfig.yaml` in the Welcome screen.

## Feature tour

**Browse.** A Lens-style sidebar groups ~25 resource kinds (Cluster / Workloads / Network / Config / Storage / Access / Ecosystem) — groups collapse and remember their state. Each item shows a live count with a red dot when something is failing. Tables filter instantly, sort on any column header, and carry an Age column plus a per-row ⋯ menu; the Pods table also shows live CPU/memory, IP, and node.

**Custom resources get their own sections.** A **Custom Resources** group is built from the cluster's CRDs at connect time — Istio's Gateways and VirtualServices, cert-manager's Certificates, Argo's Applications, whatever is installed — each with a generic Namespace / Name / Status / Age table that drills into the same Details / YAML / Events / Delete drawer. They are findable in the command palette like any other section, and the group stays hidden on a cluster that defines no CRDs.

**Search covers every section.** <kbd>Ctrl</kbd>+<kbd>K</kbd> finds resources of **all** the kinds above, custom resources included, across all namespaces.

**See what a manifest would do before it does it.** **+ Create** and **Import YAML** both have a **Preview** button that asks the cluster itself — a server-side dry run — and shows a per-document diff against what is live now, labelled *create* / *update* / *unchanged*. Because the server does the merging, you also see the defaults it fills in and anything admission would rewrite; a client-side comparison cannot show those. Nothing is written until you press Apply.

**Only the actions your token can perform are offered.** Kubby asks the cluster what your kubeconfig is allowed to do and greys out the rest, with the reason in the tooltip — no more clicking Delete, confirming, and *then* getting a 403. Pod logs and exec are checked separately from the Pod itself, because permission to read a Pod is not permission to shell into it. If Kubby cannot get an answer it leaves everything enabled rather than guessing.

**Right-sizing** (Cluster → Right-sizing). Whether what you reserved bears any relation to what you use: requests and limits against live usage per container, totals per namespace with quota headroom, and the cluster's spare capacity. It says things in words — *OOMKilled*, *memory at 97% of its limit*, *no memory limit at all*, *using 1% of its 2-core request* — and separates the one-line-per-cluster conclusions ("18 of 22 containers declare no memory limit; a LimitRange would fix that in one place") from the per-container ones, so a real problem is not buried under a policy problem. Works without metrics-server, and tells you when it is missing rather than showing zeroes.

**Overview dashboard.** One Cluster pulse owns aggregate CPU/memory capacity
(usage needs metrics-server), while the infrastructure rows show each node's
readiness, scheduling state, pressure conditions, pod count, and kubelet version.
Node names are clickable; top pods by CPU, cluster-wide recent events, and current
failing pods keep the next debugging step close by.

**Detail drawer** (click any row):
- **Details** — metadata, labels, annotations, kind-specific fields, **Events**, and a **relations tree** (Deployment→ReplicaSet→Pod, Service→Pod, Ingress→Service→Pod) whose nodes are clickable. A Node shows the **pods scheduled on it**; a Namespace shows a **per-kind summary** you can click into.
- **YAML** — a real editor: syntax highlighting, line numbers, folding, and Tab that indents. Edit + save, with a **Diff** against the loaded version.
- **Logs** (pods) — container picker, live **follow**, line filter, download.
- **Terminal** (pods) — interactive resize-aware PTY rendered by xterm.js, with
  shell completion/history, ANSI output, and full-screen TUI support where the
  selected container has a compatible shell.
- **Port Forward** (pods/services) — open `localhost` tunnels, several at once.
- **Ask AI** — a conversation about *this* resource; see below.

**Operate** (each confirmed): edit YAML, delete, **+ Create** (templated YAML), **Import YAML**; **Deployment** scale / restart / pause-resume / rollout history + rollback; **StatefulSet & DaemonSet** rolling restart; **Node** cordon/uncordon/drain; **CronJob** trigger now; **bulk select + delete**; reveal Secret values.

**Apply any kind.** **Import YAML** accepts anything the cluster serves — built-in kinds and custom resources alike (Istio, cert-manager, Argo, Gateway API…), with no per-kind configuration: the `apiVersion` + `kind` of each document is resolved against the cluster's own API discovery, and a CRD installed after you connected is picked up on the next attempt. Details worth knowing:

- Separate documents with a line containing only `---`. Two manifests pasted **without** a separator are one YAML document with duplicate keys, and only the last would survive — Kubby refuses that instead, naming the problem.
- It is a **create-or-update** (server-side apply), so re-applying an edited manifest is safe and server-defaulted fields such as a Service's `clusterIP` are left alone.
- Pasting `kubectl get -o yaml` output works: `resourceVersion`, `uid`, `managedFields` and `status` are stripped for you.
- A namespaced object with no `metadata.namespace` goes to `default`, like `kubectl`.
- You get a per-document report of what happened — `created` / `updated`, the kind, the namespace and the apiVersion — and every document is attempted even if one fails, so a half-valid bundle applies its valid half and names the rest.

**Reporting a problem.** **⚙ Settings → About** shows the exact build you are running, and **Copy diagnostics** puts a report on your clipboard: version, platform, the connected cluster's Kubernetes version and capabilities (nodes, API groups, CRD kinds, whether metrics-server is there), the AI provider in use, and the last error shown. Kubby never reads the configured API key or kubeconfig content into that report and redacts common credential patterns, but a free-form server error can still contain resource-derived details. The report is headed **review before sharing**; inspect it before posting. The same information from the CLI: `kubby-cli --version` and `kubby-cli diagnostics`.

**Navigate fast.** Multi-cluster dropdown (+ Add cluster, disconnect), **Command palette (Ctrl+K)** for views/namespaces/clusters/actions and **global resource search** (type a name, jump straight to it), **Live** auto-refresh, **dark mode** (Nord theme), and recent-connection reconnect on Welcome.

**Traffic flow** (Network → Traffic flow): where a request actually ends up. Each path is laid out in lanes — **Route → Service → Pods** — with the host/path rules, the service type and ports, and every backing pod's readiness. Summary tiles count entry points, routed vs. internal services, endpoint pods, and **broken paths**; a hop that goes nowhere (Service missing, selector matching nothing, no ready Pod) is flagged at the hop that breaks. Filter by entry point/host/service/pod name, or tick **Only show broken paths**. Every node opens its drawer.

Both ingress styles appear side by side, badged so you can tell them apart:

| Entry point | What Kubby shows | Extra checks |
|---|---|---|
| **Ingress** | class, hosts, TLS, load-balancer address | backend Service missing / no ready Pod |
| **Istio Gateway** | pod selector, listener ports (`443/HTTPS`), TLS, the ingress-gateway's address | selector matching **no** ingress-gateway Pod; `credentialName` TLS secret absent from the **ingress gateway's** namespace; no VirtualService bound |

For Istio, the routes on each hop come from the **VirtualService** that created them — it is named on the route (`via VirtualService nexus`) and clicking it opens that object. Destination hosts are resolved whether written short (`nexus`), namespaced (`nexus.nexus`) or fully qualified (`nexus.nexus.svc.cluster.local`); a host outside the cluster is marked *External* rather than reported broken. Gateways are found cluster-wide even when you filter to one namespace, because the usual layout keeps the Gateway in `istio-system` and the VirtualServices next to the app. Istio is detected automatically — no configuration, and nothing changes on a cluster without it.

**Helm** (Ecosystem):
- **Helm Releases** — ⋯ menu: **Resources + live health**, **Values/Manifest/Notes**, **History + diff-vs-current + Rollback**, **Upgrade values** with a dry-run **preview diff**, **run tests**, **uninstall**. **🔍 Search & install charts** queries Artifact Hub with a version dropdown, real default values, README, and a preview.
- **Helm Repos** — add/remove/update repositories, browse and install charts, clickable URLs.

## The AI assistant (FR-15)

Open any resource and pick the **Ask AI** tab. It is a conversation about *that* resource: Kubby collects its events, recent logs and manifest, attaches them to every question you ask, and shows you what it collected.

**The flow**

1. **Starter questions** appear first, tuned to the kind — "What is wrong with this Pod?", "Why are not all replicas ready?", "Why does this Ingress return 404?" — so you never face an empty prompt box.
2. **The header states the evidence**: `4 events · 120 log lines from 2 containers · manifest YAML`, next to **See exactly what is sent**, which prints the verbatim text before you trust an answer.
3. **Ask follow-ups.** The thread keeps its context; <kbd>Enter</kbd> sends, <kbd>Shift</kbd>+<kbd>Enter</kbd> adds a line. **Copy** lifts any answer out. **New chat** clears the thread; switching resources starts a fresh one.

**Set it up once** — the tab shows a setup card, not an error, until you do. Choose a provider in **⚙ Settings** (topbar, or the gear in the tab header):

| Provider | Needs | Default model |
|---|---|---|
| **Ollama (local)** | Ollama running locally + a pulled model — no key, **nothing leaves your machine** | `llama3.1` |
| **Anthropic (Claude)** | API key | `claude-haiku-4-5-20251001` |
| **OpenAI-compatible** | Base URL + API key (OpenAI, Azure OpenAI, or an internal gateway) | `gpt-4o-mini` |

**Answer language** is a setting too: match the question (default), always English, or always Vietnamese.

The config — including the key — is stored only on this machine at `%AppData%/kubby/ai.json`. Kubby requests mode `0600` where the OS supports Unix permission bits; on Windows, confidentiality depends on the inherited ACL of your user profile. Ollama endpoints must resolve to loopback and AI requests never follow redirects; other resource evidence goes only to the provider you chose. Answers can still be wrong: verify before acting on one.

## `kubby-cli` (developer aid)

A secondary CLI that shares the app's core logic — handy for quick checks without opening the UI. It is **not** the primary interface.

```bash
go run ./cmd/kubby-cli get pods -n default
go run ./cmd/kubby-cli yaml Deployment my-app -n default        # any kind
go run ./cmd/kubby-cli logs my-pod -n default --tail 100
go run ./cmd/kubby-cli events Pod my-pod -n default
go run ./cmd/kubby-cli apply -f edited.yaml                      # create-or-update
go run ./cmd/kubby-cli diff  -f edited.yaml                      # what it WOULD change; writes nothing
go run ./cmd/kubby-cli can-i Pod -n default                      # what your token may do
go run ./cmd/kubby-cli sizing [-n <ns>]                          # requests/limits vs real usage
go run ./cmd/kubby-cli scale my-deploy -n default --replicas 3
go run ./cmd/kubby-cli port-forward my-pod -n default --remote 8080 --local 0 --hold 30
go run ./cmd/kubby-cli exec my-pod -n default -- "ls -la /"
go run ./cmd/kubby-cli node-pods <node>                          # pods on a node
go run ./cmd/kubby-cli ns-summary <namespace>                    # per-kind counts
go run ./cmd/kubby-cli search <query>                            # global name search (all kinds incl. CRDs)
go run ./cmd/kubby-cli counts [-n <ns>] [--cluster=false]        # the sidebar tallies, timed
go run ./cmd/kubby-cli custom-kinds                              # the CRD-defined kinds shown as sections
go run ./cmd/kubby-cli list-custom <Kind.group> [-n <ns>]        # objects of a custom kind
go run ./cmd/kubby-cli --version                                 # which build is this
go run ./cmd/kubby-cli diagnostics                               # build + cluster capabilities
go run ./cmd/kubby-cli netflows [-n <ns>]                        # Ingress / Istio Gateway → Service → Pod topology
go run ./cmd/kubby-cli diag Pod <name> -n <ns>                   # the exact evidence the AI tab sends
go run ./cmd/kubby-cli helm-search <query>                       # Artifact Hub
```

Defaults to `$KUBECONFIG` or `~/.kube/config`; override with `--kubeconfig <path>` and `--context <name>`.

## Project layout

```
src/kubby/
├── app.go                  # App struct — every method bound for the frontend
├── main.go                 # Wails bootstrap
├── recent.go, ai.go        # local config (recent connections, AI provider)
├── internal/k8sclient/     # all Kubernetes logic (shared with kubby-cli)
├── cmd/kubby-cli/          # secondary CLI
└── frontend/               # index.html + src/{main.js, app.css, style.css}
```

See [`ARCHITECTURE.md`](ARCHITECTURE.md) for the full map and data flow; the recipe to add a new resource type is in [`docs/resource-browsing.md`](docs/resource-browsing.md).

## License

Released under the [MIT License](../../LICENSE).
