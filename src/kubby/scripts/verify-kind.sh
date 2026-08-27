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

[[ "$diagnostics" =~ reachable[[:space:]]+true ]]
[[ "$pods" == *"ci-pod"* ]]
[[ "$objects" == *"sample"* ]]
[[ "$drawer" == *'"name": "ci-pod"'* ]]
[[ "$permission_plan" == *'"operation":"apply-yaml"'* ]]
[[ "$permission_plan" == *'"verb":"patch"'* ]]

printf 'kind smoke passed: diagnostics, Pods, drawer, CRD discovery, custom-resource listing, and permission planning\n'
