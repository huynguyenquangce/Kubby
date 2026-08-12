# Kubby agent workflows

This repository defines durable project rules in `AGENTS.md` and seven custom
Codex agents in `.codex/agents/`. Codex discovers project agents when the
repository is trusted and the session starts from this repository.

The agents are intentionally audit-oriented. They collect independent evidence;
the primary Codex thread owns scope, trade-offs, deduplication, and any later
implementation.

## Agent catalog

| Agent | Use it for | Source changes |
|---|---|---|
| `test_engineer` | Functional/integration/regression testing and test gaps | No; build/test artifacts only |
| `ui_test_engineer` | Browser E2E, responsive/zoom, visual regression, accessibility, and native WebView2 smoke checks | No; Playwright evidence only |
| `code_reviewer` | Correctness, architecture boundaries, error handling, concurrency and maintainability | Read-only |
| `performance_engineer` | Kubernetes API latency, fan-out, payloads, Wails calls, bundle cost | No; measurement artifacts only |
| `requirements_auditor` | Spec → docs → code → test traceability | Read-only |
| `security_reviewer` | Credentials, RBAC, writes, local storage, HTTP and desktop boundaries | Read-only |
| `release_verifier` | Final build/test/version/docs/release gate | No; build/test artifacts only |

The built-in `explorer` agent remains useful for quick read-only code mapping;
there is no project duplicate of it.

## 1. Start Codex correctly

Run Codex from the repository root so it loads `AGENTS.md`, `.codex/config.toml`,
and `.codex/agents/*.toml`.

### Windows PowerShell

```powershell
cd "C:\HuyNQ263\Self Learning\kubby-ui-option-b-calm-cloud"
codex
```

Use the actual Windows path of the checkout if it differs.

### WSL/Linux

```bash
cd "/mnt/c/HuyNQ263/Self Learning/kubby-ui-option-b-calm-cloud"
codex
```

Probe both PowerShell and WSL before choosing a build host. Use native Windows
when its Go/Node/Wails toolchain is available and Windows GUI execution is
required. If it is not available, WSL can cross-build the Windows artifact; a
default WSL/Linux build instead produces a Linux binary. The exact commands and
prerequisites live only in [`BUILD.md`](BUILD.md).
WSL remains the normal host for Docker, kind, and Kubernetes-side setup.

In the interactive CLI, `/agent` shows active and completed subagent threads.
You can ask the main thread to stop or steer an agent by name.

## 2. Run one specialist

Agents are spawned through a direct prompt; do not try to execute the TOML file.

### Functional test audit

```text
Use the test_engineer agent to test FR-17 and FR-34 end to end. Do not change source. Run every safe automated check available, identify missing coverage, and return commands plus pass/fail/blocker evidence. Wait for the agent and summarize its result.
```

### UI automation audit

```text
Use the ui_test_engineer agent to run the mocked Playwright UI suite, inspect
responsive and visual-regression evidence, and report the native WebView2 checks
that remain. Do not change source or claim Kubernetes behavior from browser mocks.
```

### Performance audit

```text
Use the performance_engineer agent to audit namespace switching and sidebar refresh performance. Compare the implementation with src/kubby/docs/performance.md, measure what this environment allows, and report evidence without editing source.
```

### Requirement drift audit

```text
Use the requirements_auditor agent to audit FR-15, FR-34, FR-35, NFR-3, and NFR-4 against the current code and tests. Return a traceability table and do not modify files.
```

### Security audit

```text
Use the security_reviewer agent to review AI configuration, diagnostics, kubeconfig handling, Helm network calls, and Kubernetes write confirmation. Return only evidence-backed findings and do not edit source.
```

## 3. Run a parallel change review

Use this after a feature implementation or before opening a pull request:

```text
Review the current change with parallel subagents. Spawn test_engineer for regression and coverage, requirements_auditor for requirement drift, and security_reviewer for trust-boundary risks. If the change touches listing, search, metrics, topology, Helm, or frontend bundle size, also spawn performance_engineer. Wait for every agent, then produce one deduplicated report ordered by severity with file references. Do not implement fixes.
```

Parallelize only independent read-heavy work. If implementation is requested
after the audit, let one owner make the code changes; do not assign overlapping
files to several agents.

## 4. Run the release gate

First collect independent audits, then ask `release_verifier` for the final gate:

```text
Run a release-readiness audit. In parallel, use test_engineer, performance_engineer, requirements_auditor, and security_reviewer. Wait for all four and summarize their evidence. Then use release_verifier to run the final build/version/docs gate using those results. Do not publish, tag, commit, push, install global tools, or mutate a cluster. End with READY, READY WITH MANUAL CHECKS, or NOT READY.
```

This workflow is deliberately two-stage: the release agent should gate on the
specialists' evidence, not repeat every investigation from scratch.

## 5. Non-interactive usage

For CI-like local runs, pass the orchestration prompt to `codex exec`:

```powershell
codex exec --strict-config --ephemeral -C . "Use requirements_auditor and test_engineer in parallel to audit the current working tree. Wait for both, then return a concise release-risk report. Do not edit source."
```

```bash
codex exec --strict-config --ephemeral -C . \
  "Use requirements_auditor and test_engineer in parallel to audit the current working tree. Wait for both, then return a concise release-risk report. Do not edit source."
```

Use `--json` when another tool will consume the event stream, or
`--output-last-message <file>` when only the final report should be saved.

## 6. Verify the agent configuration

First check that Codex can load the local installation and project
configuration without exposing secrets:

```powershell
codex doctor --summary --no-color
```

Then run a strict smoke test. This starts a small model task because strict
configuration checking is supported by interactive/`exec`/`review` commands,
not by `features list`:

```powershell
codex exec --strict-config --ephemeral -C . "List the project custom agents available in this repository. Do not run them and do not modify files."
```

If the checkout does not contain usable Git metadata, add
`--skip-git-repo-check` to that command.

After changing any agent file, start a fresh Codex session so discovery is
unambiguous. Then ask:

```text
List the project custom agents available in this repository and give one sentence for when each should be used. Do not run them.
```

If an agent is missing, check:

1. Codex was started from the repository root.
2. The repository is trusted.
3. The file is under `.codex/agents/` and contains `name`, `description`, and
   `developer_instructions`.
4. `codex doctor --summary` loads the configuration and the strict smoke test
   succeeds.
5. The session was restarted after the file was added.

## 7. Commands specialists are expected to use

All build agents must follow [`BUILD.md`](BUILD.md). It is the only source for
dependency preparation, PowerShell/WSL selection, source checks, Wails targets,
artifact evidence, and safe cleanup. Do not copy its command sequence into an
agent prompt; link the agent to that file and state the required target.

Behavioural and real-cluster checks remain in
[`src/kubby/docs/verification.md`](../src/kubby/docs/verification.md).

Representative read-only real-cluster checks:

```powershell
go run ./cmd/kubby-cli diagnostics
go run ./cmd/kubby-cli counts
go run ./cmd/kubby-cli custom-kinds
go run ./cmd/kubby-cli netflows
go run ./cmd/kubby-cli sizing
go run ./cmd/kubby-cli can-i Pod -n default
```

Commands such as `apply`, `scale`, `restart`, `cordon`, `helm-install`, or
`helm-uninstall` mutate external state. An agent must not run them merely because
they appear in a test plan; the current task must explicitly authorize the
mutation and name the target cluster/namespace.

## Output standard

Specialist reports should be compact but auditable:

```text
Finding: <short title>
Severity/Gate: <P0-P3 or Critical-Low or PASS/FAIL/BLOCKED>
Requirement: <FR/NFR ID when applicable>
Evidence: <file:symbol, command, observed result>
Impact: <user or release consequence>
Recommendation: <test, fix, or decision>
Confidence/Limit: <what was not verified>
```

No findings is a valid result only when the agent states the inspected scope,
commands run, and environmental limits.
