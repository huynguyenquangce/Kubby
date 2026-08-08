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
`helm.sh/release.v1`. That is also how `ListHelmReleases` finds them without the
SDK.

## What is implemented

**Releases** — list; resources with live health; values / manifest / notes;
history with diff-vs-current and rollback; upgrade values with a **dry-run preview
diff**; run tests; uninstall.

**Repositories** — add / remove / update, browse and install charts.

**Artifact Hub** (`artifacthub.go`) — chart search with a version dropdown, README,
maintainer and home links.

## Decisions and traps

- **Dry-run previews** set `DryRun=true` and return the rendered manifest; the
  frontend diffs it with `lineDiff`. This is what makes an upgrade reviewable
  before it happens, and it is worth keeping for any new mutating action.
- **Chart default values are loaded from the chart**, not from Artifact Hub's
  `default_values` field, which is unreliable. Authoritative beats convenient.
- **`repo.LoadFile`'s not-found error is not matched by `os.IsNotExist` on
  Windows.** `loadOrNewRepoFile` therefore stats the file first. Removing that
  stat reintroduces a Windows-only failure on first use.
- Repo management reads and writes **the user's own `repositories.yaml`**, so
  repos added in Kubby are visible to the `helm` CLI and vice versa. Be careful
  not to clobber entries you did not write.

## Verify

```powershell
go run ./cmd/kubby-cli helm-search <query>
```

The rest of the Helm surface is exercised through the GUI; a local kind cluster
with one small chart installed is enough to cover list → values → history →
rollback.
