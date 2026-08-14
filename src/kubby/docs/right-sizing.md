# Right-sizing (requested vs used)

← [Architecture skeleton](../ARCHITECTURE.md)

Owns `rightsizing.go` and the **Right-sizing** view under *Cluster*.

Kubby already listed ResourceQuotas and LimitRanges, and already read
metrics-server for the pods table. Neither answers the question people actually
have — *is this namespace's reservation anywhere near its usage?* — because the
answer needs both halves side by side. This file joins them.

Entirely **read-only and derived**. Nothing here writes to the cluster.

## The rule that everything else depends on

> **"Not declared" and "declared as zero" are different facts.**
>
> A missing request is `unset` (`-1`), never `0`. Several findings exist purely
> because a container declared nothing, and rendering that as `0m` would erase
> them. Both the Go formatters and the JS ones return `—` for `unset`, and
> `percentOf` returns `unset` rather than dividing.

`TestUnsetIsNeverRenderedAsZero` and `TestPercentOfRefusesToInvent` pin this.

## Shape

```go
func Sizing(ctx, c, namespace string) (*SizingReport, error)   // "" = whole cluster
```

```
SizingReport
├── Totals      NamespaceSizing     — the scope, summed
├── Namespaces  []NamespaceSizing   — per namespace, with its Quota lines
├── Containers  []ContainerSizing   — only those with a finding, worst first
├── Advice      []string            — what the aggregate says
├── AllocCPU / AllocMem / Nodes     — Ready nodes only
├── MetricsAvailable / MetricsComplete
├── MetricsExpected / MetricsObserved / MetricsCoveragePct
└── Note
```

Four independent reads run concurrently: pods, pod metrics, quotas + limit ranges,
nodes. Only the pod list is fatal; the rest degrade.

**Allocatable counts Ready nodes only.** A NotReady node's capacity is not
available, so including it would overstate headroom.

**Metrics are optional and coverage is explicit.** Without metrics-server the requests/limits half is still
worth showing — *"nothing declares a memory limit"* needs no measurement — so the
report sets `MetricsAvailable: false` and a `Note`, and the UI shows `—` rather
than zeroes. A successful metrics List does not mean every container was observed:
aggregate usage and usage-based advice are suppressed unless observed equals
expected, and namespace/container rows retain the same coverage ownership.

## Do not add the three "missing" counts together

`NoCPURequest + NoMemRequest + NoMemLimit` is **not** a count of anything. One
container that declares nothing contributes to all three, so the sum overlaps — the
first version of the view displayed `45` on a cluster with **22 containers**, which
is nonsense the moment you read it next to the container count.

`NamespaceSizing.Undeclared` is the honest figure: containers missing **at least
one** of the three. It is what the view and the CLI show, as `18 of 22`. Keep the
three individual counts for the advice lines (each names a *specific* fix), and keep
`Undeclared` for anything shown as a headline.

## Presenting it: a stat tile per resource, and the meter is a *meter*

Three layouts were tried. The two that were thrown away are recorded because both
mistakes are easy to repeat.

**Attempt 1 — six equal number tiles.** The comparison *is* the content, and six
tiles made the reader do arithmetic across them to find it.

**Attempt 2 — one bar per resource scaled to the cluster's allocatable capacity,**
usage as the fill, the reservation as a tick. Principled, useless: on a 10-core
cluster using 282m, *both* quantities landed inside the leftmost 3% of a very wide
bar. 97% of the graphic was empty.

**Attempt 3 — two bars sharing a local `max(requested, used)` scale.** Fixed the
scale, broke the reading: the muted fill for `requested` looked exactly like an
**empty track**, so the tile read as a bar that had failed to load.

The form was wrong all three times, and the reason is in the dataviz guidance:

> A single ratio against a limit → **meter** (same-ramp track).

The reservation is not a second series. It is the **limit** — so it is the *track*,
and usage is the *fill*. One bar, and the track cannot be misread as empty because
being the limit is its job.

```
CPU                          11% of the cluster's 10.00 cores
935m                            ← the consequence, in the resource's own unit
reserved and sitting idle
████████░░░░░░░░░░░░░░░░░░░░░   ← track = 1.15 reserved, fill = 227m used
227m used of 1.15 reserved
```

Two tiles in an `auto-fit` grid, so neither is a lone graphic stretched across a
1500px window, and they stack on a narrow one.

The **headline is the consequence**, not an input: `935m reserved and sitting idle`,
or `122Mi more than it reserved`. That is the number a reader would otherwise compute
by subtraction. A fill past the track is clamped to 100% and recoloured — the exact
overage is already the headline, so the bar does not need to misrepresent its own
scale to carry it.

Per the same guidance the headline uses **proportional** figures, not
`tabular-nums`: equal-width digits make a display-size number look loose, and nothing
lines up beneath it. `tabular-nums` stays on the table rows, where numbers do align.

The same meter shrinks into a table cell (`.sz-cell-meter`) — one bar, not two.

## Percentages of allocatable are computed in Go, once

`CPUReservedPct` / `CPUUsedPct` / `MemReservedPct` / `MemUsedPct` live on the report,
and both the advice sentences and the frontend tiles read them.

They used to be derived twice: Go's advice used integer division (`1150*100/10000` →
**11**) and the frontend used `Math.round` (→ **12**). The same fact appeared on one
screen as "reserves 11% of the cluster" and "12% of cluster", two lines apart. **A
percentage two places derive is a percentage that will eventually disagree with
itself** — if the view needs a share of allocatable, add a field, do not recompute.

### Never print a ratio against zero

The cell used to render `9Mi of 0Mi` for a container that requested nothing —
istio-system's pod, on the local cluster. A ratio against zero is not a fact. The
cell now says **`9Mi used · no request`**, and a namespace with neither a request nor
usage shows `—`. Same rule as `unset` above: say what is true, do not compute
something that looks like it.

## Advice, not just rows

The first version flagged **22 of 22 containers** on the local cluster, almost all
for the same reason. A list that flags everything ranks nothing.

So the aggregate conclusions moved into `Advice`, one sentence each:

> 18 of 22 containers declare no memory limit, so nothing stops one of them taking
> a node's memory. A LimitRange would set a default in one place — istio-system,
> kube-system, local-path-storage have none.

That is **one decision** (add a LimitRange), where 18 identical rows were 18 rows.
`ContainerSizing.Severity` is exported so the view's *"only OOMKills and near-limit
memory"* toggle can hide the declaration noise; severity ≥ 4 is the serious band.

Advice is **silent when there is nothing to say** — a well-declared, well-used scope
produces an empty list, and `TestAdviceIsSilentWhenThereIsNothingToSay` keeps it
that way. Singular/plural is handled by `verb()`, because *"1 of 10 containers
declare"* reads as a bug in the tool.

## Thresholds, and why they are conservative

| Constant | Value | Meaning |
|---|---|---|
| `idleRequestPct` | 15 | using ≤15% of the request → over-provisioned |
| `burstRequestPct` | 200 | using ≥200% → under-requested |
| `nearLimitPct` | 90 | memory ≥90% of its **limit** → OOMKill risk |
| `minCPURequestMilli` | 50 | below this, "idle" is noise |
| `minMemRequestBytes` | 64Mi | same |

A false *"this is over-provisioned"* costs real trust, so the floors matter as much
as the percentages: flagging a 10m request as idle would bury the findings that
count (`TestTinyRequestsAreNotCalledOverProvisioned`).

Note the two denominators, which are different questions:

- **usage ÷ request** → is the reservation right? (`CPUPct`, `MemPct`)
- **usage ÷ limit** → is it about to be killed? (`MemOfLimit`)

## What is deliberately left out

- **Init containers.** Their requests do count toward scheduling (max of init vs
  sum of app), but a row per init container is noise. Not counted, not listed.
- **Succeeded / Failed pods.** A finished Job reserves nothing; including them would
  flood the table on any cluster that runs CronJobs.
- **Terminating pods** (`DeletionTimestamp != nil`), same as the Traffic view.
- **Non-compute quota lines.** A quota on ConfigMap count is real but has nothing to
  do with sizing, so `quotaLines` keeps only cpu/memory/pods.

## Quota comparison

`quotaNearlyFull` parses both `Hard` and `Used` with `apiresource.ParseQuantity`
rather than comparing strings. Both come from the same ResourceQuota status, so the
unit only has to be *consistent* — which is why `MilliValue()` is fine for memory
too. Unparseable values report "not full" rather than guessing
(`TestQuotaNearlyFull`).

This is the check that found something real on the local cluster: namespace
`default` sitting at **90% of its pods quota (9 of 10)**.

## Verify

```powershell
go run ./cmd/kubby-cli sizing                  # whole cluster
go run ./cmd/kubby-cli sizing -n kube-system   # one namespace
go test ./internal/k8sclient/ -run Sizing      # thresholds, formatting, advice
```

The CLI prints the same numbers the view shows, deliberately — `FormatCPU` /
`FormatMem` are exported for that reason.

To exercise the no-metrics path, scale metrics-server to zero rather than assuming
it is absent (it **is** installed on the local kind cluster — see
[verification.md](verification.md)):

```powershell
kubectl -n kube-system scale deploy metrics-server --replicas=0
```

Expected: every usage column becomes `—`, the `Note` appears, and the
requests/limits findings still work.
