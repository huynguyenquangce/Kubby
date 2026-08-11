# Build Kubby

This is the single source of truth for preparing a build environment, verifying
the source, producing Kubby binaries, and cleaning stale build artifacts.

Run project commands from `src/kubby/` unless a step explicitly says otherwise.

## Outputs

| Target | Command target | Canonical artifact |
|---|---|---|
| Windows x86-64 | `windows/amd64` | `src/kubby/build/bin/kubby.exe` |
| Linux x86-64 | `linux/amd64` | `src/kubby/build/bin/kubby` |

A cross-built Windows executable still needs a native Windows visual check.
Headless validation proves its format and compilation, not GUI behaviour.

## 1. Prerequisites

Required for every target:

- Go 1.26.5 or newer, matching the security-patched `toolchain` directive in
  `src/kubby/go.mod`. Go's toolchain selection can download that patch release
  automatically when an older Go 1.26 command starts the build.
- Node.js `^20.19.0` or `>=22.12.0`, required by the locked Vite version.
- npm and network access for the first dependency installation.
- Wails CLI v2.13.0, matching `src/kubby/go.mod`.

Native Windows builds also require WebView2. Windows 11 normally provides it.

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
cd ..
```

`gofmt -l` passes only when it prints no filenames. `npm run build` may emit a
bundle-size warning; record it separately from build failure.

Why run these separately from Wails:

- Wails does not run Go tests or `go vet`.
- Wails does not enforce `gofmt`.
- `src/kubby/wails.json` uses `npm install`; the explicit `npm ci` verifies the
  lockfile reproducibly.

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

For a tagged release, override all three build identity fields and trim local
paths. Substitute the version, seven-character commit, and commit date that will
be tagged; do not build a release from a dirty tree:

```bash
go run github.com/wailsapp/wails/v2/cmd/wails@v2.13.0 build \
  -platform windows/amd64 -trimpath \
  -ldflags "-X kubby/internal/buildinfo.Version=0.1.0 -X kubby/internal/buildinfo.Commit=abcdef0 -X kubby/internal/buildinfo.Date=2026-08-11"
```

The checked-in `0.1.0-dev` value is intentional and must not be edited for a
release; the ldflags prevent ordinary local builds from masquerading as a tag.

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
WebView2 rendering, cluster connection, and the changed UI workflow. Native
dialog click automation is unreliable on this machine, so record the manual
check rather than claiming it from a headless session.

For Kubernetes-backed behaviour, use the corresponding read-only `kubby-cli`
command described in [`../src/kubby/docs/verification.md`](../src/kubby/docs/verification.md).

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
- PowerShell cannot find Go or Wails: use WSL cross-build for `windows/amd64`, or
  install the native Windows toolchain before attempting native GUI execution.
