# Kubby landing page

This is a static marketing page, separate from the Wails application in
`src/kubby/`. It has no build step or runtime dependency. `index.html`,
`styles.css`, and `assets/` are the deployable files. The PNG under `mockup/`
is a concept preview for design review; the live hero uses the real product
screenshot in `assets/kubby-overview.png`.

## Vercel

Import `huynguyenquangce/Kubby` as a Vercel project, set **Root Directory** to
`landing`, and use the **Other** framework preset. Leave Build Command and
environment variables unset. The site is static, so no Kubernetes credentials
or AI keys belong in Vercel. A connected Git branch can produce a preview
deployment; promote to production only after checking the page and its links.

The Download button points to GitHub releases because v0.5.0 currently
publishes a Windows x86-64 preview. The page labels Linux as source-build only
and macOS as in progress. Update that copy only when the corresponding native
artifacts are actually released and verified.
