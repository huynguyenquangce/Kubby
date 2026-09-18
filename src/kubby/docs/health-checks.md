# Health checks

← [Architecture skeleton](../ARCHITECTURE.md)

Owns `healthchecks.go`, `webhookcheck.go`, `certcheck.go`, `stuckcheck.go` and the
**Health checks** view (Cluster → Health checks). Three faults that stop a cluster
or a deletion without an obvious error, found before an operator goes looking:

| Tab | Question | Source of truth |
|---|---|---|
| Admission webhooks | Will a webhook reject requests because its backend is gone? | `Validating`/`MutatingWebhookConfiguration`, backend Service, EndpointSlices |
| Certificates | Which certificate expires soon, and is anything renewing it? | TLS Secrets, cert-manager `Certificate`, kubeconfig, API server, webhook `caBundle` |
| Stuck deletions | What is Terminating, and what holds it? | Metadata of every listable resource type, plus targeted reads |

Everything is read-only. The only "action" is copying a kubectl command.

## One call, cached

`ClusterChecks(namespace)` is one bound call. It starts the shared Lists
concurrently (Namespaces, both webhook configuration kinds, TLS Secrets, Ingresses,
cert-manager Certificates when installed, the stuck scan) and derives all three
reports from them. The result is cached per connection and scope for **15 s**: the
stuck scan lists every resource type, which live mode must not repeat every five
seconds. A refresh inside the window shows the same "Checked …" time.

A failed input is a section warning, never a failed call: one forbidden List must
not blank the other two tabs.

## Admission webhooks

Each `webhooks[]` entry is graded on its own, with the v1 defaults applied
(`failurePolicy: Fail`, `timeoutSeconds: 10`, port 443).

- **Backend** — the Service must exist, expose the configured port, and have at
  least one ready EndpointSlice endpoint. `Fail` + broken backend is **critical**
  and names the blast radius ("rejects every matching request (CREATE pods) in all
  namespaces"); `Ignore` + broken backend is a **warning**, because the policy is
  silently not enforced and each request can wait for the timeout.
- **Scope** — `namespaceSelector` is evaluated against the live namespaces (with the
  `kubernetes.io/metadata.name` label) to say which namespaces are affected.
- **Self-dependency** — a `Fail` webhook that intercepts Pod creation in the
  namespace of its own Service cannot recreate its own Pods while it is down.
- **kube-system** — a `Fail` webhook on Pod creation in kube-system can block
  control-plane add-ons.
- **Timeout** above 10 s, **empty `caBundle`** on a Service webhook, and a
  **`caBundle` certificate** that is expired (critical/warning by failure policy)
  or expires within 30 days.
- **URL webhooks** are reported but not probed: Kubby does not call arbitrary
  endpoints from the desktop, and only the URL's host is shown (paths can carry
  tokens).

## Certificates

Grading: expired or ≤ 7 days → critical; ≤ 30 days → warning; not yet valid →
warning. A chain is graded by whichever certificate expires first.

- **TLS Secrets** are listed with `fieldSelector=type=kubernetes.io/tls` so other
  Secrets' data is not transferred, and the type is re-checked client-side. Only
  `tls.crt` is parsed; private keys never enter the report (a test pins this).
- **cert-manager** is optional (discovery, negative-cached like Istio). A managed
  Secret inside its renewal window whose Certificate is Ready with a future
  `renewalTime` is *info*, not a warning — that is normal. `Ready=False` is
  critical within 30 days of expiry. A Certificate whose Secret does not exist is
  "not issued yet" (critical if an Ingress uses it).
- **Ingress TLS** — a referenced Secret that does not exist is a warning (the
  controller serves its default certificate). Absence is confirmed with a Get, so
  a Secret of another type is reported as such rather than as missing.
- **Kubeconfig client certificate** — from the connection's embedded cert data.
- **API server certificate** — read by a TLS handshake that sends no request and no
  credentials, deliberately without verification (it only reads dates), cached per
  connection for 10 minutes.
- **Webhook CA bundles** — so a CA expiry is visible next to other certificates.

Not visible agent-less: kubelet serving/client certificates, etcd and other
control-plane component certificates on the nodes.

## Stuck deletions

- **Scan** — every resource type discovery marks `list`able (Events and
  ComponentStatuses excluded) is listed through the **metadata client** under the
  usual concurrency bound. A scoped view scans that namespace's objects and the
  Namespace itself. Types that cannot be listed are counted and named.
- **Stuck** — Terminating for ≥ 5 minutes. Younger deletions are shown as *info*.
- **Explanations** — known finalizers (`kubernetes.io/pvc-protection`,
  `pv-protection`, `foregroundDeletion`, `orphan`, the namespace `kubernetes`
  finalizer, CRD cleanup, load-balancer cleanup, Job tracking) are explained; any
  other finalizer names the controller domain to check. Targeted reads add:
  - **Namespace** — the namespace controller's own conditions. A discovery failure
    (typically an unavailable aggregated APIService) is **critical**, because it
    blocks every namespace deletion in the cluster.
  - **Pod** — a missing or NotReady node that can never confirm termination.
  - **PVC** — the Pods still mounting it.
  - **PV** — the claim it is still bound to.
- **Command** — `kubectl patch … finalizers:null` for a finalizer, or
  `kubectl delete pod … --grace-period=0 --force` for a Pod on a dead node, each
  with the consequence spelled out. Kubby never runs it.

## Verify

```powershell
go test ./internal/k8sclient -run 'Webhook|Certificate|Stuck|ClusterChecks|APIServer' -v
go run ./cmd/kubby-cli checks [-n <ns>]
```

The kind smoke (`scripts/verify-kind.sh`) applies a broken `Fail` webhook scoped to
a labelled namespace, an expired TLS Secret and a finalizer-held ConfigMap, deletes
the ConfigMap, and asserts all three appear in `kubby-cli checks`.

## Manual test

Use a disposable cluster (kind). The scenarios live in
[`../testdata/manual/health-checks.yaml`](../testdata/manual/health-checks.yaml)
and affect only `kubby-demo-*` namespaces.

```bash
kubectl apply -f testdata/manual/health-checks.yaml
kubectl delete configmap held-by-finalizer -n kubby-demo-checks --wait=false
kubectl delete namespace kubby-demo-stuck-ns --wait=false
# optional: a certificate that expires in 5 days
openssl req -x509 -newkey rsa:2048 -nodes -days 5 -subj /CN=soon.kubby.test \
  -keyout /tmp/soon.key -out /tmp/soon.crt
kubectl create secret tls soon-tls -n kubby-demo-checks --cert /tmp/soon.crt --key /tmp/soon.key
```

Clean up (removing the demo finalizers is safe: nothing owns them):

```bash
kubectl delete validatingwebhookconfiguration kubby-demo-broken-fail
kubectl delete mutatingwebhookconfiguration kubby-demo-ignored
kubectl patch configmap held-by-finalizer -n kubby-demo-checks --type=merge -p '{"metadata":{"finalizers":null}}'
kubectl patch configmap blocks-namespace -n kubby-demo-stuck-ns --type=merge -p '{"metadata":{"finalizers":null}}'
kubectl delete namespace kubby-demo-checks
```
