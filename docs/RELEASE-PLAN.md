# Release preparation plan

Scope: prepare the 0.5.1 preview (v0.5.0 already exists) for review without publishing a tag or
GitHub release. Preserve the existing uncommitted changes. Follow BUILD.md for
dependencies, source checks, packaging and release identity.

1. Fix scheduling's contradictory missing-PVC verdict and log-follow completion;
   add regression tests covering normal completion and stale ownership.
2. Split the 32-view navigation traversal into bounded groups; check in the two
   missing zoom points and make unexpected Wails mock calls fail explicitly.
3. Add provider HTTP contract tests and browser AI/Helm/write-flow tests for
   success, cancellation, failure and stale preview/target ownership.
4. Exercise writes/Helm/streaming using disposable synthetic resources, never
   existing workloads; verify cleanup and read-only scheduling regression on kind.
5. Run source checks, dependency advisory scans and a complete Windows Wails
   build. Request independent release-readiness audits and resolve findings.
6. Record results and preview limitations in release notes. Native Windows
   manual verification and the clean committed release identity remain gates
   before tagging or publishing.

Status: implementation, automated checks, disposable kind lifecycle, and
Windows x86-64 packaging are complete for a local 0.5.1-dev candidate.
Publication remains gated on a clean committed release identity and native
Windows manual verification. See [release readiness](RELEASE-READINESS.md).
