# Build Kubby

This is the single source of truth for preparing a build environment, verifying
the source, producing Kubby binaries, and cleaning stale build artifacts.

Run project commands from `src/kubby/` unless a step explicitly says otherwise.

## Outputs

| Target | Command target | Canonical artifact |
|---|---|---|
| Windows x86-64 | `windows/amd64` | `src/kubby/build/bin/kubby.exe` |
| Windows x86-64 installer | `windows/amd64` with `-nsis` | `src/kubby/build/bin/kubby-amd64-installer.exe` |
| Linux x86-64 | `linux/amd64` | `src/kubby/build/bin/kubby` |

The installer is what end users download. It installs per user into
`%LOCALAPPDATA%\Programs\Kubby` without an administrator prompt, adds Start Menu
and Desktop shortcuts and a **Settings → Apps** entry, offers to launch Kubby,
and installs the WebView2 runtime when it is missing. Its script is
`src/kubby/build/windows/installer/project.nsi`. Wails regenerates
`wails_tools.nsh` and a `tmp/` WebView2 bootstrapper beside it on every installer
build; both are ignored and are not source.

A cross-built Windows executable still needs a native Windows visual check.
Headless validation proves its format and compilation, not GUI behaviour.

## 1. Prerequisites

Required for every target:

- Go 1.26.9 or newer, matching the security-patched `toolchain` directive in
  `src/kubby/go.mod`. Go's toolchain selection can download that patch release
  automatically when an older Go 1.26 command starts the build.
- Node.js `^20.19.0` or `>=22.12.0`, required by the locked Vite version.
- npm and network access for the first dependency installation.
- Wails CLI v2.13.0, matching `src/kubby/go.mod`.

Native Windows builds also require WebView2. Windows 11 normally provides it.

The Windows installer additionally requires NSIS 3 (`makensis`) on the build
host. Wails only prints a warning and exits successfully when `makensis` is
missing, so always confirm the installer file exists (section 6).

```bash
sudo apt-get install -y nsis        # WSL/Linux cross-build
```

```powershell
winget install NSIS.NSIS            # native Windows; then reopen the shell
```

Browser UI checks require Playwright's pinned Chromium. After `npm ci`, install
it once per machine. On Windows use `npx playwright install chromium`; on
WSL/Linux also install its system libraries:

```bash
cd frontend
npx playwright install --with-deps chromium
cd ..
```

Linux GUI builds require a C compiler, `pkg-config`, GTK3, and WebKitGTK. On an
Ubuntu release that provides WebKitGTK 4.1:

```bash
sudo apt-get update
sudo apt-get install -y build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev
```

A Windows cross-build from WSL does not require the Linux GTK/WebKit headers.

## 2. Detect the usable host

Probe native PowerShell:

```powershell
Get-Command go,node,npm,wails -ErrorAction SilentlyContinue
go version
node --version
npm --version
wails doctor
```

Probe WSL/Linux:

```bash
command -v go node npm wails
go version
node --version
npm --version
pkg-config --modversion gtk+-3.0 2>/dev/null || true
pkg-config --modversion webkit2gtk-4.1 2>/dev/null || true
```

Use native Windows when its toolchain is available and the GUI must be run
there. If PowerShell lacks Go/Wails, WSL can cross-build `kubby.exe`.

If Go is available but a global Wails command is not, use the pinned CLI without
installing it globally:

```bash
go run github.com/wailsapp/wails/v2/cmd/wails@v2.13.0 doctor
```

Alternatively, install the matching CLI:

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@v2.13.0
```

## 3. Prepare dependencies

Install the frontend tree exactly from the lockfile before starting Go or Wails
commands:

```bash
cd frontend
npm ci
cd ..
```

Important notes:

- Do not run `npm ci` concurrently with `go build ./...`. It replaces
  `node_modules`, so a concurrent filesystem walk can see an incomplete tree.
- Do not hand-edit `node_modules/`, `frontend/wailsjs/`, `frontend/dist/`, or
  `build/bin/`; they are generated.
- In a restricted session where the default Go cache is read-only, use one
  writable task-specific cache for all Go commands:

  ```bash
  export GOCACHE=/tmp/kubby-build-go-cache
  export GOTMPDIR=/tmp
  ```

- Reuse that cache sequentially. Multiple complete Go caches can exhaust `/tmp`.
- Never print kubeconfig contents, tokens, Secret values, or AI API keys in build
  output.

## 4. Run source checks

These checks are required for a verified build even though Wails also compiles
the frontend and backend:

```bash
go test ./...
go build ./...
go vet ./...
gofmt -l *.go internal/k8sclient/*.go internal/buildinfo/*.go cmd/kubby-cli/*.go

cd frontend
npm test
npm run build
npm run test:e2e
cd ..
```

`gofmt -l` passes only when it prints no filenames. `npm run build` may emit a
bundle-size warning; record it separately from build failure.

Why run these separately from Wails:

- Wails does not run Go tests or `go vet`.
- Wails does not enforce `gofmt`.
- `src/kubby/wails.json` also uses `npm ci`, so direct Wails builds and the
  explicit frontend preparation both install exactly the lockfile tree.
- Playwright runs the real frontend through a deterministic Wails binding mock;
  it needs no kubeconfig or cluster. Screenshots, traces, and videos on failure
  are test evidence, not source artifacts.

## 5. Build the application

Choose one target command.

Native Windows:

```powershell
wails build
```

Windows x86-64 from WSL:

```bash
go run github.com/wailsapp/wails/v2/cmd/wails@v2.13.0 build -platform windows/amd64
```

Add `-nsis` to either Windows command to also produce the installer from the
same executable. Kubby always installs per user; `project.nsi` fixes that scope,
so `-installscope user` is accepted but not required:

```bash
go run github.com/wailsapp/wails/v2/cmd/wails@v2.13.0 build -platform windows/amd64 -nsis
```

For a tagged release, override all three build identity fields and trim local
paths. Substitute the version, seven-character commit, and commit date that will
be tagged; do not build a release from a dirty tree:

```bash
go run github.com/wailsapp/wails/v2/cmd/wails@v2.13.0 build \
  -platform windows/amd64 -trimpath \
  -ldflags "-X kubby/internal/buildinfo.Version=0.2.0 -X kubby/internal/buildinfo.Commit=abcdef0 -X kubby/internal/buildinfo.Date=2026-08-19"
```

The checked-in `0.5.1-dev` value is intentional. Keep the `-dev` suffix in source;
the release ldflags remove it so ordinary local builds cannot masquerade as a tag.

Linux x86-64 on Ubuntu with WebKitGTK 4.1:

```bash
go run github.com/wailsapp/wails/v2/cmd/wails@v2.13.0 build -tags webkit2_41
```

Older Linux distributions that still provide WebKitGTK 4.0 can use the default
Wails tags and their corresponding 4.0 development package.

A complete Wails build must finish bindings, frontend compilation, application
assets where applicable, application compilation, and packaging. Passing only
the Go checks or frontend bundle is not a full application build.

## 6. Verify the new artifact

For a Windows artifact built from WSL:

```bash
file build/bin/kubby.exe
stat -c '%y %s %n' build/bin/kubby.exe
sha256sum build/bin/kubby.exe
```

`file` must report a PE32+ Windows GUI x86-64 executable.

For a Windows installer build, also check the installer and its payload:

```bash
file build/bin/kubby-amd64-installer.exe
sha256sum build/bin/kubby-amd64-installer.exe
```

`file` must report a `Nullsoft Installer self-extracting archive`. A missing file
means `makensis` was not found even if Wails reported success.

For a Linux artifact:

```bash
file build/bin/kubby
stat -c '%y %s %n' build/bin/kubby
sha256sum build/bin/kubby
ldd build/bin/kubby | grep 'not found' || true
```

`file` must report an ELF x86-64 executable and `ldd` must have no `not found`
entries.

Record the retained artifact's target, size, timestamp, and SHA-256 in the build
report. Do not claim GUI behaviour from a headless check.

## 7. Remove stale artifacts

Never delete the previous runnable artifact before its replacement has compiled
and passed the checks above. Inspect `build/bin/`, keep the canonical artifact
for the requested target, and remove only explicitly identified stale files.

Known legacy filenames can be removed after a successful replacement:

```powershell
Get-ChildItem build/bin
Remove-Item build/bin/kubby-option-b.exe,build/bin/kubby-option-b-latest.exe -ErrorAction SilentlyContinue
```

```bash
find build/bin -maxdepth 1 -type f -printf '%f\n'
rm -f build/bin/kubby-option-b.exe build/bin/kubby-option-b-latest.exe
```

Do not use wildcard cleanup. Generated binaries are recoverable by rebuilding,
but source and user-owned files are not cleanup targets.

When temporary Go cache space is no longer needed:

```bash
go clean -cache
```

If a custom cache was selected, pass the same `GOCACHE` value to this command.

## 8. Manual checks

The full Wails build verifies compilation and packaging. Before a Windows
release, open `kubby.exe` on native Windows and visually confirm startup,
WebView2 rendering, cluster connection, and the changed UI workflow.

For a release, perform that check through the installer: run
`kubby-windows-amd64-setup.exe`, confirm there is no administrator prompt, launch
Kubby from the finish page and from the Start Menu, and finally uninstall it from
**Settings → Apps** and confirm the shortcuts disappear. Install over an older
version once with Kubby still running; the installer must ask you to close it and
continue after **Retry**. Uninstalling intentionally keeps the user's
`%APPDATA%\kubby` settings (recent kubeconfig paths and AI configuration). Native
dialog click automation is unreliable on this machine, so record the manual
check rather than claiming it from a headless session.

For Kubernetes-backed behaviour, use the corresponding read-only `kubby-cli`
command described in [`../src/kubby/docs/verification.md`](../src/kubby/docs/verification.md).

## 9. Publish a GitHub release

This section is the release checklist. A successful `go build`, frontend bundle,
or cross-build alone is not release readiness. Releases are built by the
**Release** GitHub Actions workflow (`.github/workflows/release.yml`) on a native
Windows runner; section 9.4 is the fallback when Actions is unavailable.

Every release publishes four assets with stable names, which the landing page
download button depends on:

| Asset | Purpose |
|---|---|
| `kubby-windows-amd64-setup.exe` | Installer most users download |
| `kubby-windows-amd64-setup.exe.sha256` | Installer checksum |
| `kubby-windows-amd64.exe` | Portable executable, no installation |
| `kubby-windows-amd64.exe.sha256` | Portable checksum |

### 9.1 Freeze and audit the release commit

Update the intended version in `wails.json`, set the checked-in
`internal/buildinfo.Version` to the same number with its `-dev` suffix, prepare
user-facing notes under `docs/releases/vX.Y.Z.md` (published verbatim as the
release body: title it `# Kubby vX.Y.Z` and leave no "draft" wording), and
commit/push every source and documentation change to `main`. The release build overrides the version
only in the binaries.

Confirm locally that the commit to be released is clean, matches `origin/main`,
and that the version has not been released before:

```bash
git status --short --branch
git rev-parse HEAD
git rev-parse origin/main
git ls-remote --tags origin refs/tags/vX.Y.Z
gh release list --limit 20
```

Run the project release-readiness audits described in `AGENTS.md` and review
dependency advisories:

```bash
go run golang.org/x/vuln/cmd/govulncheck@latest ./...

cd frontend
npm audit --json
cd ..
```

Do not silently ignore a scanner failure. Upgrade a dependency when a fix exists.
If upstream has no fix, release only after proving the vulnerable path is disabled
or unreachable, adding a regression test, and disclosing the residual in that
version's release notes.

### 9.2 Build a draft with the Release workflow

Start the workflow from `main`, in the GitHub **Actions → Release → Run workflow**
page or with the CLI:

```bash
gh workflow run release.yml --ref main -f version=X.Y.Z -f prerelease=true
gh run watch
```

Keep `prerelease=true` while the binaries are unsigned. The workflow:

1. refuses anything but `main`, a version that differs from `wails.json` or
   `buildinfo.Version`, missing release notes, or an existing tag;
2. reruns every Quality gates job (Go, frontend, Playwright, kind smoke) on the
   same commit;
3. builds `kubby.exe` and the installer on `windows-latest` with
   `-trimpath` and release ldflags for the version, seven-character commit, and
   commit date, and rejects stale bindings or any modified tracked file;
4. silently installs the installer, checks that the installed `kubby.exe` is
   byte-identical to the portable asset, checks the Start Menu shortcut, the
   per-user Apps entry and file version, then silently uninstalls and checks
   that nothing is left behind;
5. verifies formats and checksums, refuses to duplicate an existing release or
   draft, and creates a **draft** release targeting the built commit with the
   four assets and `docs/releases/vX.Y.Z.md` as notes.

A failed run publishes nothing; fix the cause on `main` and run it again. The
run's summary lists the commit the draft targets.

The `.sha256` files are byte-for-byte fingerprints. They detect an incomplete,
corrupted, or changed download. On Windows, compare:

```powershell
Get-FileHash .\kubby-windows-amd64-setup.exe -Algorithm SHA256
Get-Content .\kubby-windows-amd64-setup.exe.sha256
```

Matching hashes prove file integrity, not publisher identity. A checksum does not
replace Authenticode code signing, because an attacker who can replace both
assets could publish a matching checksum too.

### 9.3 Native check, publish, and verify

A draft has no tag yet. Download the draft's installer and portable EXE, verify
their checksums, and perform the native Windows checks in section 8. In
**Settings → About**, the runtime identity must show the release version, the
commit and date from the workflow summary, and `windows/amd64`.

Publish only after that manual gate passes:

```bash
gh release edit vX.Y.Z --draft=false
```

GitHub creates the tag on the draft's target commit at that moment. Then prove
the tag and assets:

```bash
git fetch --tags origin
git rev-parse vX.Y.Z^{commit}
gh release view vX.Y.Z --json name,tagName,isDraft,isPrerelease,url,targetCommitish,assets,publishedAt
```

The tag must resolve to the audited commit, the release must not be a draft, and
all four assets must report `uploaded`. Open the landing page and confirm its
download button now names the new version and links to
`releases/download/vX.Y.Z/kubby-windows-amd64-setup.exe`.

If the manual gate fails, delete the draft (`gh release delete vX.Y.Z`); no tag
exists yet, so the version can be rebuilt after the fix.

### 9.4 Manual fallback

Use this only when GitHub Actions cannot run. Complete sections 1–8 on the clean
release commit with `-nsis` and the release ldflags from section 5, then stage
the same asset names:

```bash
cp build/bin/kubby-amd64-installer.exe build/bin/kubby-windows-amd64-setup.exe
cp build/bin/kubby.exe build/bin/kubby-windows-amd64.exe

cd build/bin
for f in kubby-windows-amd64-setup.exe kubby-windows-amd64.exe; do
  sha256sum "$f" > "$f.sha256"
  sha256sum -c "$f.sha256"
done
cd ../..
```

Install the staged installer on native Windows and perform section 8, then
create the draft from `src/kubby/` and continue with section 9.3:

```bash
gh release create vX.Y.Z \
  build/bin/kubby-windows-amd64-setup.exe \
  build/bin/kubby-windows-amd64-setup.exe.sha256 \
  build/bin/kubby-windows-amd64.exe \
  build/bin/kubby-windows-amd64.exe.sha256 \
  --draft --prerelease --target "$(git rev-parse HEAD)" \
  --title "Kubby vX.Y.Z" \
  --notes-file ../../docs/releases/vX.Y.Z.md
```

The release-named copies and checksums are upload assets, not source, and remain
ignored by Git. Run section 7 afterward to remove identified legacy binaries and
temporary caches.

## Troubleshooting

- `webkit2gtk-4.0 not found` on a recent Ubuntu: install WebKitGTK 4.1 development
  packages and add `-tags webkit2_41`.
- `go: failed to trim cache ... read-only file system`: select one writable
  task-specific `GOCACHE` as shown above.
- `no space left on device`: clean abandoned task caches and rerun sequentially
  with one cache.
- `npm ci` fails with `EAI_AGAIN`: network access to the npm registry is blocked;
  retry only after network access is available.
- `esbuild ... EPERM` on a mounted Windows drive: ensure dependency installation
  completes before retrying and do not overlap it with other build processes.
- `Warning: Cannot create installer: makensis not found`: install NSIS
  (section 1). Wails still exits 0, so the installer file is simply absent.
- The installer reports that Kubby is running: close every Kubby window, then
  choose **Retry**. A silent install (`/S`) exits with code 5 instead.
- PowerShell cannot find Go or Wails: use WSL cross-build for `windows/amd64`, or
  install the native Windows toolchain before attempting native GUI execution.
