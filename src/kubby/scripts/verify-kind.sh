#!/usr/bin/env bash
set -euo pipefail

repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
kind_bin="${KIND_BIN:-kind}"
cluster_name="kubby-ci"
kubeconfig_file="$(mktemp)"
cli_bin="$(mktemp)"

cleanup() {
    "$kind_bin" delete cluster --name "$cluster_name" >/dev/null 2>&1 || true
    rm -f "$kubeconfig_file" "$cli_bin"
}
trap cleanup EXIT

"$kind_bin" create cluster \
    --name "$cluster_name" \
    --image "kindest/node:v1.34.3@sha256:08497ee19eace7b4b5348db5c6a1591d7752b164530a36f855cb0f2bdcbadd48" \
    --wait 120s
"$kind_bin" get kubeconfig --name "$cluster_name" >"$kubeconfig_file"

cd "$repo_dir"
go build -trimpath -o "$cli_bin" ./cmd/kubby-cli

run_cli() {
    "$cli_bin" --kubeconfig "$kubeconfig_file" "$@"
}

run_cli apply -f testdata/kind/foundation.yaml

custom_kinds=""
for _ in {1..30}; do
    custom_kinds="$(run_cli custom-kinds)"
    if [[ "$custom_kinds" == *"Widget.kubby.dev"* ]]; then
        break
    fi
    sleep 2
done
[[ "$custom_kinds" == *"Widget.kubby.dev"* ]]

run_cli apply -f testdata/kind/custom-resource.yaml

diagnostics="$(run_cli diagnostics)"
pods="$(run_cli get pods -n kubby-ci)"
objects="$(run_cli list-custom Widget.kubby.dev -n kubby-ci)"
drawer="$(run_cli drawer Pod ci-pod -n kubby-ci)"
permission_plan="$(run_cli plan-apply -f testdata/kind/custom-resource.yaml)"
network_policies="$(run_cli get networkpolicies -n kubby-ci)"
disruption_budgets="$(run_cli get pdbs -n kubby-ci)"
autoscalers="$(run_cli get hpas -n kubby-ci)"
traffic_check="$(run_cli netpol-check ci-client Pod/ci-pod -n kubby-ci --port 8080)"
routes="$(run_cli netflows -n kubby-ci)"
containers="$(run_cli containers ci-pod -n kubby-ci)"
# The smoke cluster has one node, which schedules every fixture Pod.
drain_impact="$(run_cli drain-impact "${cluster_name}-control-plane")"

[[ "$routes" == *"policy: blocked from ingress-nginx controller"* ]]
[[ "$routes" == *"ci-deny-ingress"* ]]
[[ "$containers" == *"PREVIOUS-LOGS"*"hold"* ]]
[[ "$drain_impact" == *'"name": "ci-pdb"'* ]]

# Delete the finalizer-held ConfigMap without waiting: it stays Terminating.
# kubectl comes from the kind node image, so the smoke needs none on the host.
docker exec "${cluster_name}-control-plane" kubectl --kubeconfig /etc/kubernetes/admin.conf \
    delete configmap ci-stuck -n kubby-ci --wait=false >/dev/null
checks="$(run_cli checks)"

[[ "$checks" == *"critical ValidatingWebhookConfiguration ci-broken-webhook / broken.kubby.dev"* ]]
[[ "$checks" == *"Service kubby-ci-webhook/missing-webhook does not exist"* ]]
[[ "$checks" == *"critical TLS Secret kubby-ci/ci-expired-tls"* ]]
[[ "$checks" == *"ConfigMap kubby-ci/ci-stuck terminating"* ]]
[[ "$checks" == *"kubby.dev/ci-hold"* ]]

[[ "$network_policies" == *"ci-deny-ingress"*"Denies all ingress"* ]]
[[ "$disruption_budgets" == *"ci-pdb"* ]]
[[ "$autoscalers" == *"ci-hpa"*"Deployment/ci-missing"* ]]
[[ "$traffic_check" == BLOCKED:* ]]
[[ "$traffic_check" == *"ci-deny-ingress"* ]]

[[ "$diagnostics" =~ reachable[[:space:]]+true ]]
[[ "$pods" == *"ci-pod"* ]]
[[ "$objects" == *"sample"* ]]
[[ "$drawer" == *'"name": "ci-pod"'* ]]
[[ "$permission_plan" == *'"operation":"apply-yaml"'* ]]
[[ "$permission_plan" == *'"verb":"patch"'* ]]

printf 'kind smoke passed: diagnostics, Pods, drawer, CRD discovery, custom-resource listing, permission planning, policy kinds, NetworkPolicy evaluation, blocked-route verdicts, container state, drain preview, and health checks\n'
