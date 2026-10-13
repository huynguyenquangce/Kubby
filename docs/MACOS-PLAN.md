# macOS implementation plan

Target: Apple Silicon MacBooks (M1 or newer), macOS 12+, `darwin/arm64`.
Requirement: NFR-2 remains in progress until a native packaged app passes the
manual checks. All environment and build commands belong to [BUILD.md](BUILD.md).

1. **Synchronize the existing branch** — merge current `origin/main` into
   `feat/macos-support`, retaining macOS bundle metadata and Cmd shortcuts while
   keeping the latest resource sorting, safety fixes, dependencies and identity.
2. **Make native verification available** — enable the Quality gates workflow
   on branch pushes and manual dispatch; align macOS Go with the patched main
   toolchain; run backend tests/build/vet on the Mac runner before Wails packaging.
3. **Verify packaging** — validate Info.plist, bundle identifier, Mach-O arm64,
   stale bindings and the complete ZIP checksum; retain the unsigned CI preview.
4. **Verify keyboard behavior** — browser regressions cover Control+K and Meta+K,
   palette input focus, Escape dismissal and focus restoration. Native clipboard,
   CodeMirror Cmd+F and Terminal keys remain in the Mac manual checklist.
5. **Run on a MacBook** — follow BUILD.md sections 1–8; record macOS version,
   artifact hash, Finder startup, file picker, connection, themes, clipboard,
   Logs, Terminal and Port Forward. WSL cannot produce native GUI evidence.
6. **Prepare public distribution separately** — Developer ID signing,
   notarization, stapling and Gatekeeper checks are required before stable
   macOS distribution. No Apple credentials are stored in this repository.

Steps 1–4 are implemented in source. The macOS CI job has not been run from this
WSL session. Steps 5–6 remain pending a native Mac and distribution credentials.
Intel/universal packaging is outside the initial build gate.

Local verification (WSL, 2026-10-13): npm ci, all 12 frontend test files,
Vite build, all 95 Chromium E2E tests, Go tests/build/vet, formatting and
workflow YAML parsing passed. Go HTTP tests required running outside the
sandbox to bind localhost. Vite reported its existing >500 kB chunk warning.
No cluster logic changed; no cluster mutation or native macOS check was run.
