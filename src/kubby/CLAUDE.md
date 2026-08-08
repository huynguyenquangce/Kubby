# CLAUDE.md — Kubby

Guidance for Claude Code working in `k8s/application/src/kubby/` — the Kubby
desktop app itself. (The parent repo is a Vietnamese course-notes repo with a very
different convention; see the root `CLAUDE.md`.)

## What this is

Kubby is a **Wails v2 desktop app** (Go backend + vanilla-JS frontend) for managing
Kubernetes clusters personally — load a kubeconfig, browse/edit/act on resources
through a Lens-like UI. No in-cluster agent.

## How to find your way around

**The docs are a skeleton plus branches. Read the skeleton, then open only the
branch you are working in** — that split exists so a Traffic-view change does not
require loading the Helm and AI documentation too.

1. [`ARCHITECTURE.md`](ARCHITECTURE.md) — **start here.** The map: data flow,
   directory layout, cross-cutting invariants, and a table routing you to the right
   branch.
2. [`docs/*.md`](docs/) — one branch per feature area. The branch map is in the
   skeleton; each branch carries its own decisions, traps, and how to verify it.
3. [`README.md`](README.md) — user-facing: how to run, feature tour, `kubby-cli`.
4. [`../../docs/SPECIFICATION.md`](../../docs/SPECIFICATION.md) — the product spec
   (why it exists, requirement status). Edit this first when scope changes.

Do not answer an architectural question from memory of this file — the branch is
the source of truth, and this file deliberately does not repeat it.

## Commands

Run from `k8s/application/src/kubby/` on **native Windows** (not WSL):

```powershell
wails doctor                    # verify Go/Node/WebView2
wails dev                       # hot-reload dev mode
wails build                     # production build → build/bin/kubby.exe (regenerates bindings)
go build ./... ; go vet ./...   # fast backend-only compile check
go test ./...                   # ~35 tests: diagnostics, apply, access, right-sizing
```

The test suite is **one file**. Verification is `cmd/kubby-cli` against a real cluster —
full command list, the local kind setup, and this machine's quirks (slow first
build, blocked image pulls, unreliable GUI screenshots) are in
[`docs/verification.md`](docs/verification.md). **Read that before claiming a change
works.**

## Invariants worth carrying in your head

Everything else lives in a branch. These are the ones most often broken by a change
made elsewhere:

1. **`QPS: 50` / `Burst: 100` in `clusterFromRest` must stay.** client-go's default
   5/10 throttles an interactive UI into a visible stall.
   → [`docs/performance.md`](docs/performance.md)
2. **Fan out in Go, one bound call per screen** — never one bound call per kind from
   JavaScript, and use the metadata client where only a count or a name is needed.
   → [`docs/performance.md`](docs/performance.md)
3. **`[hidden] { display: none !important }` in `app.css`** — without it, a
   `display:flex` rule beats the HTML `hidden` attribute and the element never
   disappears. → [`docs/frontend.md`](docs/frontend.md)
4. **Any kind the cluster serves is addressable**; a custom resource is referenced
   as `Kind.group`. Do not reintroduce the static kind table as the only resolver.
   → [`docs/kind-resolution.md`](docs/kind-resolution.md)
5. **Every YAML box is a CodeMirror instance, not a `<textarea>`.** Read it through
   its handle, never `.value`, and remember its editable element is a
   `contenteditable` div — a global key handler that tests for `TEXTAREA` will let
   Enter submit a half-typed manifest. → [`docs/frontend.md`](docs/frontend.md)
6. **A permission Kubby could not check counts as allowed.** Never grey out on a
   failed probe. → [`docs/permissions.md`](docs/permissions.md)

## When you finish a change

- Update the **branch** you touched, not this file — and only if a decision or trap
  changed, not for every line.
- Add a `cmd/kubby-cli` command for any new backend feature, or it cannot be
  verified.
- Log fixed bugs in [`../../docs/bug.txt`](../../docs/bug.txt) with the cause, not
  just the symptom.
