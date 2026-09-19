# Cleanup (cluster hygiene)

← [Architecture skeleton](../ARCHITECTURE.md)

Owns `hygiene.go`, `hygiene_refs.go`, the **Cleanup** view (Cluster → Cleanup)
and `kubby-cli hygiene`.

## What it is for

A cluster accumulates objects nothing needs — a ConfigMap from a chart that was
uninstalled, a PVC whose Pod was deleted, a Service whose selector stopped
matching after a label rename, Pods that finished last month. None of it raises
an alert, because none of it is an error; it just makes every list longer and
every bill larger, and the Service case is an outage waiting to be noticed.

## The two rules

1. **Kubby never deletes anything here.** Every item carries the `kubectl`
   command; the operator runs it. The Playwright test asserts that acting on the
   view issues no delete call at all.
2. **Every group states what Kubby could not see.** A controller that reads a
   ConfigMap by name leaves no reference in any Pod spec, and a reference scan
   cannot know about it. The caveat is rendered *with* the group, not as a
   footnote — a cleanup list without its caveat is how someone deletes something
   an operator was using.

## Groups

| Category | Listed when | Severity |
|---|---|---|
| `unused-configmap` | no Pod, controller template or ReplicaSet mounts it or reads env from it | info |
| `unused-secret` | nothing above, and no ServiceAccount or Ingress names it | info |
| `unused-pvc` | no Pod mounts the claim | warning |
| `service-without-endpoints` | no ready endpoint in any EndpointSlice | warning (info with no selector) |
| `old-replicaset` | 0 replicas and older than 7 days | info |
| `finished-job` | Complete/Failed for > 24 h, no `ttlSecondsAfterFinished`, no CronJob owner | info |
| `finished-pod` | Succeeded/Failed for > 24 h | info (warning when Evicted) |
| `unpinned-image` | image is `:latest` or has no tag | warning |

## What keeps the false-positive rate down

The reference index (`hygiene_refs.go`) is deliberately generous:

- it walks **controller templates as well as live Pods** — a Deployment scaled to
  zero has no Pod, and its ConfigMap is still in use;
- it walks ReplicaSet templates, because a rollback needs what they name;
- it covers volumes (including `projected` sources and CSI node-publish secrets),
  `envFrom`, `env.valueFrom`, `imagePullSecrets`, ServiceAccount `secrets` and
  `imagePullSecrets`, and Ingress TLS.

Exclusions, each for a reason:

- objects with an **`ownerReference`** — deleting them is the owner's business;
- **`kube-root-ca.crt`**, maintained by the control plane in every namespace;
- **service-account token** Secrets (annotation) and **Helm release storage**
  (`owner=helm` label);
- **claims a StatefulSet created from a `volumeClaimTemplate`** (`<template>-<set>-<ordinal>`).
  They outlive their Pods by design — scaling a StatefulSet to zero keeps the
  data — so calling them unused would be the worst advice this feature could
  give;
- **ExternalName** Services, which have no endpoints to have.

An unused claim's detail names the risk directly: with a `Delete` reclaim policy,
the data goes with the claim.

## Secret values never enter this scan

ConfigMaps and Secrets are listed through the **metadata client**. The scan needs
names, labels, annotations and ownership; a typed Secret list would transfer
every value across the bridge for the sake of counting. `TestHygieneNeverReadsSecretValues`
fails if the typed client touches Secrets at all.

## Cost and caching

One list per resource type (14), issued concurrently under the shared
`countConcurrency` bound, then eight passes over the result. The report is cached
per connection and scope for **30 s**, so live refresh does not repeat a
fourteen-list scan every five seconds; `TestHygieneCachesItsScanPerScope` pins
that, and that a different namespace is a different question.

`BenchmarkPerfHygieneReport` measures the derivation alone: 0.49 ms for a
500-object namespace, 6.0 ms for 5 000 (median of `-count 3`) — linear, with the work dominated by
building the items rather than by the reference index.

A failed list costs only its own group: the group renders its `warning` and the
other seven still answer.

## Verify

```powershell
go test ./internal/k8sclient -run 'Hygiene|ImageRisk' -v
go test ./internal/k8sclient -run '^$' -bench 'BenchmarkPerfHygieneReport' -benchmem
go run ./cmd/kubby-cli hygiene [-n <ns>] [--category unused-pvc]
```

## Manual test

On a disposable cluster, [`../testdata/manual/hygiene.yaml`](../testdata/manual/hygiene.yaml)
creates one object per group in `kubby-demo-hygiene`, including the two cases
that must **not** be reported (a ConfigMap used by a Deployment scaled to zero,
and a StatefulSet's own claim).
