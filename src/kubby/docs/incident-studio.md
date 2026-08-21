# Incident Studio

← [Architecture skeleton](../ARCHITECTURE.md)

Owns `internal/k8sclient/investigation.go`, the **Investigate** drawer tab and
the `kubby-cli investigate` verification command.

## Product contract

Incident Studio answers three questions in order:

1. **What is failing?** Deterministic playbooks interpret Kubernetes state and
   warning Events; AI is not required.
2. **What evidence supports that conclusion?** Every finding carries compact
   evidence and a confidence label. The timeline and related-resource list stay
   clickable in the UI.
3. **What is the safest next step?** Actions navigate to existing logs, YAML,
   rollout, topology, right-sizing or AI workflows. Investigation itself never
   writes to the cluster, so all mutation still crosses the existing preview and
   confirmation boundaries.

The first deep playbook is Pod-focused. It correlates container waiting and
termination state, readiness/scheduling conditions, restart counts, direct and
ReplicaSet-collapsed ownership, matching Services, EndpointSlice readiness and
UID-scoped Events. Other resource kinds get a truthful generic event report
instead of fabricated Pod-level conclusions.

## Snapshot and recovery semantics

`InvestigateResource` is one frontend-bound call. Fan-out stays in Go after the
target object is resolved, so the report is one coherent point-in-time payload.
The UI can re-run that call as a bounded recovery watch after a user performs a
separately confirmed operation. Recovery is verified only when the deterministic
report becomes healthy; a timeout is reported as inconclusive, never success.

Kubby is agent-less and does not pretend to own historical telemetry. The
timeline contains timestamps retained by the live object and Kubernetes Events.
It cannot reconstruct changes that the API server has already discarded.

## Export boundary

`FormatIncidentMarkdown` exports only the already-trimmed report: resource
identity, summary, findings/evidence, related resource names, timeline, actions
and limitations. It does not include manifests, Secret values, kubeconfig,
credentials or raw application logs. The normal OS save dialog owns the path.

## Verify

```powershell
go run ./cmd/kubby-cli investigate Pod <name> -n <namespace>
go test ./internal/k8sclient -run Investigation
```
