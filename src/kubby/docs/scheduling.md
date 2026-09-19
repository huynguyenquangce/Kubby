# Why Pending (scheduling explainer)

← [Architecture skeleton](../ARCHITECTURE.md)

Owns `scheduling.go`, `scheduling_fit.go`, the **Why Pending?** modal and the
`kubby-cli why-pending` command.

## The question

A Pending Pod is the most common failure an operator meets, and the one
Kubernetes answers worst. The scheduler compresses a whole cluster into one
event line:

```
0/12 nodes are available: 7 Insufficient memory, 3 node(s) had untolerated taint,
2 node(s) didn't match Pod's node affinity/selector.
```

That says what, never *which* and never *by how much*. Kubby re-runs the
feasibility half of scheduling against the API objects, node by node, and keeps
**every** reason a node failed rather than the first — because fixing one only
to meet the next is the frustrating part.

Everything here is read-only. Kubby schedules nothing, evicts nothing, patches
nothing.

## Order of the answer

`ExplainScheduling(namespace, name)` returns one report, and the UI renders it in
the order the answer is read:

1. **Verdict + headline** — `unschedulable`, `fits`, `scheduled` or `unknown`.
2. **The scheduler's own message**, from the `PodScheduled=False` condition. It
   is the authority; everything below is Kubby explaining it. Where the
   simulation disagrees (verdict `fits` while the Pod sits Pending) the report
   says so instead of claiming the scheduler is wrong — the usual causes are a
   cycle in flight, a quota, or a plugin Kubby does not model.
3. **Reason groups with counts** — "2 nodes: Insufficient memory — needs 8.0Gi,
   2.0Gi free of 8.0Gi", with the node names.
4. **Per-node table** — free CPU, free memory, Pod slots, and every reason.
5. **The Pod's events**, then the limits of the simulation.

A Pod that already has a node is a different question, and the report switches
to it: scheduling is over, so what remains is the kubelet's work. Waiting
container reasons (`ImagePullBackOff`, `CreateContainerConfigError`, …) are
explained with the shared `waitingExplanation` that Incident Studio also uses, so
both features say the same thing about the same reason.

## Predicates

Mirroring the kube-scheduler filter plugins, in `scheduling_fit.go`:

| Reason code | What it checks |
|---|---|
| `cordoned` | `spec.unschedulable` |
| `not-ready` | the node's `Ready` condition |
| `taint:<key>` | `NoSchedule`/`NoExecute` taints against the Pod's tolerations |
| `node-affinity` | `nodeSelector` plus required node affinity (`In`, `NotIn`, `Exists`, `DoesNotExist`, `Gt`, `Lt`, `matchFields`) |
| `insufficient:<resource>` | allocatable − what the node's Pods request, for **every** resource the Pod asks for, including extended ones such as `nvidia.com/gpu` |
| `pod-capacity` | allocatable `pods` against the Pods already there |
| `host-port` | a host port already published on that node, wildcard IPs included |
| `pod-affinity` / `pod-anti-affinity` | required inter-pod terms, with `namespaces` / `namespaceSelector` scope |
| `topology-spread` | `DoNotSchedule` constraints, including `minDomains` |
| `volume-node-affinity` | node affinity of a bound PersistentVolume |

Two deliberate details:

- **Cordoned and NotReady are reported once, not twice.** Kubernetes adds
  `node.kubernetes.io/unschedulable` and `node.kubernetes.io/not-ready` taints
  for those states; both keys are skipped in the taint loop so one condition
  produces one reason.
- **`PreferNoSchedule`, preferred affinity and `ScheduleAnyway` spread are
  ignored.** They change ranking, not feasibility, and listing them as reasons
  would send people to fix something that was never blocking.

Pod requests are what the scheduler reserves: regular containers plus
restartable init containers (sidecars), or a plain init container's phase when
that is larger, plus `spec.overhead`.

## Volumes

Claims are read only for the volumes the Pod actually references:

- missing claim → **critical** (nothing is ever scheduled);
- Pending with `volumeBindingMode: Immediate` → **critical**;
- Pending with `WaitForFirstConsumer` → **info**, explicitly *not* a problem —
  that is how local volumes are meant to work, and calling it an error sends the
  operator to the wrong place;
- Bound to a PV with node affinity → a per-node restriction.

## Cost

One Pod Get, one Node list, one Pod list (field-selected to exclude finished
Pods), the Pod's events, and a Get per referenced claim. **Nothing is per node**:
selectors are compiled once and affinity/spread domains are counted once, so the
per-node loop is map lookups.

`TestExplainSchedulingRequestCountIsIndependentOfClusterSize` pins the request
count at 4 for 3 nodes and for 200. Measured with `BenchmarkPerfSchedulingSimulation`
(inputs prebuilt, so the fake client's copies are not what is measured; median of `-count 3`, Ryzen 5 5600H under WSL2):

| Nodes × Pods | Time | Allocations |
|---|---|---|
| 10 × 5 000 | 1.2 ms | 568 |
| 200 × 5 000 | 1.6 ms | 2 903 |
| 1 000 × 5 000 | 3.4 ms | 12 668 |

The first version accumulated each Pod's requests through a `ResourceList`, which
cost 20 570 allocations at 5 000 Pods; `accumulatePodRequests` sums integers
directly instead.

## Where it is offered

- **Overview → Attention queue**: a Pod whose status is `Pending` or
  `Unschedulable` gets **Why Pending?** in place of Investigate. A Pod with no
  node has no logs, no container state and no events beyond the scheduler's, so
  Investigate has nothing to work with.
- **Any Pod row → ⋯ → Why Pending?** The menu entry is gated on `get` for the Pod. The per-node answer also lists Nodes and Pods cluster-wide; a token that cannot gets a report that says which input it could not read, not a guess.
- `kubby-cli why-pending <pod> -n <ns>`.

## Limits, stated in the report

Read-only; required rules only; resource arithmetic from the Pods the API server
reports at that moment; scheduler extenders, custom plugins, CSI volume-count
limits and dynamic resource allocation are not modelled.

## Verify

```powershell
go test ./internal/k8sclient -run 'Scheduling|EffectivePod' -v
go test ./internal/k8sclient -run '^$' -bench 'BenchmarkPerfScheduling|BenchmarkPerfExplain' -benchmem
go run ./cmd/kubby-cli why-pending <pod> -n <ns>
```

## Manual test

On a disposable cluster, [`../testdata/manual/scheduling.yaml`](../testdata/manual/scheduling.yaml)
creates Pending Pods for five distinct causes in `kubby-demo-sched`.
