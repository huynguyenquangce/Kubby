# Kubby repository guidance

These instructions apply to the whole repository. Kubby is a personal,
agent-less Kubernetes desktop application. The product source is under
`src/kubby/`; `docs/SPECIFICATION.md` is the product contract.

## Read this first

Before changing code, read:

1. `docs/SPECIFICATION.md` for requirements and scope.
2. `src/kubby/ARCHITECTURE.md` for the system map and cross-cutting invariants.
3. Only the relevant branch document under `src/kubby/docs/`.
4. `src/kubby/docs/verification.md` before claiming a change works.

Do not treat generated files or binaries as source. Never hand-edit
`frontend/wailsjs/`, `frontend/dist/`, `build/bin/`, `node_modules/`, or
`src/kubby/kubby-cli.exe`.

## Architecture boundaries

- The frontend never talks to Kubernetes directly. It calls exported methods on
  `App` in `src/kubby/app.go` through Wails bindings.
- Keep `App` thin. Kubernetes and Helm behavior belongs in
  `src/kubby/internal/k8sclient/` so `kubby-cli` can exercise the same code.
- Add or update a `kubby-cli` command when adding backend capability that needs
  real-cluster verification.
- Resolve arbitrary/custom resources through discovery. `Kind.group` is the
  unambiguous resource reference; do not make the static built-in kind table the
  only resolver.
- Prefer one bound call per screen and concurrent fan-out in Go. Use the metadata
  client when only names or counts are needed.
- Keep `QPS: 50` and `Burst: 100` in `clusterFromRest` unless a measured,
  documented performance decision replaces them.

## Safety and product invariants

- Every cluster write or destructive operation requires explicit user
  confirmation in the UI.
- A permission probe that cannot be completed counts as allowed. An explicit
  denial counts as denied. The API server remains the final authority.
- Never persist pasted kubeconfig content. Never print or include AI API keys in
  diagnostics, logs, fixtures, screenshots, or reports.
- Missing requests/limits are `-1` and render as `—`; they must never be silently
  converted to zero.
- All YAML inputs use the shared CodeMirror editor. Read them through the editor
  handle, not DOM `.value`.
- Preserve `[hidden] { display: none !important; }` in `frontend/src/app.css`.
- Non-resource-list tables use `class="plain"`.
- Exclude terminating objects where the feature describes current/live state.
- Do not claim GUI behavior was verified from a headless session. State the
  manual visual check that remains.

## Change discipline

- Make the smallest coherent change and preserve unrelated user modifications.
- Update the owning branch document only when a design decision, invariant, or
  known trap changes.
- Log fixed defects in `docs/bug.txt` with the root cause, not just the symptom.
- If requirements change, update `docs/SPECIFICATION.md` first or in the same
  change, including the relevant requirement ID/status.
- Findings and handoffs must cite concrete files/symbols and separate verified
  facts from hypotheses.

## Verification baseline

[`docs/BUILD.md`](docs/BUILD.md) is the only source of truth for dependencies,
PowerShell/WSL selection, backend and frontend checks, full Wails builds,
artifact verification, and stale-binary cleanup. Follow it in order and do not
duplicate or improvise a second build procedure in another document.

Read `src/kubby/docs/verification.md` for tests, real-cluster checks, environment
quirks, and GUI verification limits. Do not claim the app is fully built unless
the requested platform artifact was produced and verified as required by
`docs/BUILD.md`.

For behavior that talks to Kubernetes, use the matching `kubby-cli` command
against the local kind cluster. Use read-only CLI commands by default. Do not run
write commands unless the task explicitly authorizes cluster mutation.

## Using subagents

Custom project agents live in `.codex/agents/`. Use them when the user asks for
the corresponding audit, or for a release/readiness review spanning multiple
independent quality dimensions.

- `test_engineer`: functional, integration, regression, and test-gap analysis.
- `code_reviewer`: correctness, architecture boundaries, maintainability, error
  handling, concurrency, and resource lifecycle.
- `performance_engineer`: latency, API fan-out, allocations, bundle/build size,
  and measurement quality.
- `requirements_auditor`: trace implementation and tests to requirement IDs.
- `security_reviewer`: credentials, RBAC, Kubernetes write safety, network calls,
  and local persistence.
- `release_verifier`: final build/test/version/docs/release gate.

For a comprehensive audit, delegate independent read-heavy checks in parallel,
wait for every requested agent, then synthesize one deduplicated report. Keep the
primary agent responsible for scope, prioritization, and final decisions. Do not
let multiple agents edit overlapping files. Audit agents report findings; they do
not implement fixes unless the user separately asks for implementation.

Every audit finding must include severity, evidence, impact, and a concrete
verification or remediation recommendation. If no finding exists, say what was
checked and what could not be checked.
