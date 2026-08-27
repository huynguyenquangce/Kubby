# Helm

← [Architecture skeleton](../ARCHITECTURE.md)

Owns `helm.go`, `helmrepo.go`, `artifacthub.go`.

## In-process, no binary

Kubby uses the **Helm Go SDK** (`helm.sh/helm/v3`) directly — no `helm` executable
to find, no version skew, no Tiller. The cost is a noticeably larger binary; the
SDK pulls a large dependency tree.

A `restClientGetter` adapts `Cluster.Rest` into the `RESTClientGetter` the SDK
wants, then:

```go
action.Configuration.Init(getter, namespace, "secret", logf)
```

`"secret"` is the Helm 3 storage driver — releases live as Secrets of type
`helm.sh/release.v1`. `ListHelmReleases` reads only partial Secret metadata,
filters to that type, and keeps the newest numeric revision for each
namespace/name pair. Never list typed Secrets here: their compressed release
payload is large and every historical revision would otherwise appear as a
separate release.

Every SDK operation owns a cancellable context. The cloned REST config binds all
API requests to that context and has a finite five-minute ceiling; Helm actions
also receive finite timeouts and Install/Upgrade use `RunWithContext`. Switching
or disconnecting the active cluster cancels registered operations, and mutating
bindings verify the expected connection ID before starting.
HTTP(S) chart index/archive acquisition uses the same operation context (while
retaining Helm's 120-second request ceiling), so cancellation also stops the
download that precedes `RunWithContext`.
Artifact Hub JSON, repository indexes, and chart archives also have explicit
decoded-body ceilings. Helm HTTP redirects are limited, reject HTTPS-to-HTTP
downgrades, re-evaluate credential scope on every hop, and never include URL
userinfo/query values in surfaced fetch errors.

## What is implemented

The frontend presents these capabilities as one **Helm workspace** with three
peer tabs: Releases, Catalog, and Repositories. Repositories are chart sources,
not cluster resources, so they show the local-machine scope explicitly instead
of inheriting the namespace filter.

**Releases** — current-revision summary and list; resources with live health;
values / manifest / notes;
history with diff-vs-current and rollback; upgrade values with a **dry-run preview
diff**; run tests; uninstall.

**Repositories** — add / remove / update, browse and install charts.

**Artifact Hub** (`artifacthub.go`) — chart search with a version dropdown, README,
maintainer and home links.

## Decisions and traps

- **Dry-run previews** set `DryRun=true` and return the rendered manifest; the
  frontend diffs it with `lineDiff`. This is what makes an upgrade reviewable
  before it happens, and it is worth keeping for any new mutating action.
- **Permission plans use those exact manifests.** Install/upgrade compare the
  dry-run result with current state; uninstall/rollback use stored current/target
  manifests; test checks only test hooks. The plan includes Helm release-storage
  Secrets and a missing install namespace. Explicit denials keep the final action
  disabled; unresolved CRDs/probe failures remain allowed warnings because the
  API server is still authoritative.
- **Install preview pins the artifact.** The backend returns the SHA-256 of the
  exact resolved chart archive. Any form edit invalidates that preview, and
  Install re-resolves then compares the archive against the approved digest
  before Helm can write. Do not make the digest optional: mutable chart repos
  otherwise create a preview/install time-of-check/time-of-use gap.
- **Upgrade preview pins both inputs.** It returns the current release revision
  and SHA-256 of the exact values text. Upgrade supplies both back to the backend,
  which rejects a stale release or changed values. Keep this validation in Go;
  disabling the frontend button is useful guidance, not a safety boundary.
- **Configured repositories retain their identity.** Browse results carry a
  `SourceID` (the repository name). Chart defaults, preview and install pass it
  back so the backend can load credentials, client certificates and TLS options
  from the user's Helm configuration. A URL alone is insufficient for a private
  repository. The UI never receives those credentials.
- **Remote payloads are bounded.** Artifact Hub search/detail responses are
  capped at 2/8 MiB, repository indexes at 20 MiB, and chart archives at 100 MiB.
  Keep both the Content-Length precheck and streaming `limit+1` guard because
  chunked and compressed responses cannot be trusted from headers alone.
- **Manifest identity is the full GVK.** Resource rows retain `apiVersion` and use
  discovery-derived scope plus a group-qualified `Kind.group` reference when
  opening the generic resource drawer. Unknown/custom kinds are dynamically read
  and report `unknown` health rather than being presented as healthy. Live health
  reads use bounded concurrency.
- **Opening a release uses one snapshot.** `HelmSnapshot` reads the release once,
  reuses that manifest for the detail panes and bounded live-health reads, and
  crosses the Wails bridge once. Do not add parallel `HelmGet` and
  `HelmReleaseResources` calls in the frontend; the latter necessarily reads the
  release for standalone callers.
- **Legacy Helm OpenPGP verification stays disabled.** Helm v3 still links the
  deprecated `golang.org/x/crypto/openpgp` implementation, for which
  `govulncheck` reports GO-2026-5932 with no fixed version. Kubby never enables
  `ChartPathOptions.Verify`; it explicitly keeps `Verify=false` and instead gates
  install on the previewed archive's SHA-256. Keep the scanner finding recorded
  until Helm removes/replaces that dependency.
- **Chart default values are loaded from the chart**, not from Artifact Hub's
  `default_values` field, which is unreliable. Authoritative beats convenient.
- **Helm dialogs reuse one modal and fixed element IDs.** Async release/chart
  responses must carry the exact modal owner scope and write only through captured
  DOM/editor handles. Upgrade stays disabled until current values load; Install is
  disabled while a requested defaults load is pending. Chart defaults are
  version-bound; changing the version invalidates an in-flight request and clears
  defaults loaded for the previous version.
- **`repo.LoadFile`'s not-found error is not matched by `os.IsNotExist` on
  Windows.** `loadOrNewRepoFile` therefore stats the file first. Removing that
  stat reintroduces a Windows-only failure on first use.
- Repo management reads and writes **the user's own `repositories.yaml`** with
  mode 0600 where supported, so
  repos added in Kubby are visible to the `helm` CLI and vice versa. Be careful
  not to clobber entries you did not write. Repo names are restricted to a safe
  identifier because Helm derives cache paths from them.
- Repository read/modify/write sequences are serialized process-wide and commit
  through a synced private temporary file plus atomic rename. This avoids lost
  updates and truncated YAML when Add/Remove overlap or the process is interrupted.
  Slow index downloads do not hold that configuration lock; Add reserves its
  name while downloading, and Update downloads at most four repository indexes
  concurrently before returning sorted failures.
  Each download lands in a unique temporary cache and is parsed before a short
  locked atomic rename publishes it; Browse therefore cannot observe Helm's
  direct partial cache writes, and overlapping Updates cannot interleave bytes.

## Verify

```powershell
go run ./cmd/kubby-cli helm-search <query>
go run ./cmd/kubby-cli helm-install <release> -n <namespace> --repo <url> --repo-name <configured-name> --chart <chart> --version <version>
```

The install command performs the same pinned preview/install sequence as the UI.
Use a disposable local kind cluster for it because it writes resources. The rest
of the Helm surface is exercised through the GUI; one small chart is enough to
cover list → resources → values → history → rollback.
