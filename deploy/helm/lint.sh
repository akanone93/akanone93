#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: Apache-2.0

# `make helm-lint` (DEP-003, DEP-004): lints the chart, renders it for every
# values file under plux/ci/, validates the output against the Kubernetes and
# CloudNativePG schemas, and checks the properties the chart promises (the
# values schema is strict, Pod Security "restricted", L-4). Needs helm and
# kubeconform (make install-helm-tools).
set -euo pipefail

HELM=${HELM:-helm}
KUBECONFORM=${KUBECONFORM:-kubeconform}
KUBE_VERSION=${KUBE_VERSION:-1.34.0}
# Schemas of the custom resources the chart renders (the CloudNativePG Cluster).
CRD_SCHEMAS='https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json'

here=$(cd "$(dirname "$0")" && pwd)
chart=$here/plux
out=$(mktemp -d)
trap 'rm -rf "$out"' EXIT

fail() { echo "✗ $*" >&2; exit 1; }

render() { "$HELM" template plux "$chart" --namespace plux --kube-version "$KUBE_VERSION" "$@"; }

shopt -s nullglob
values=("$chart"/ci/*-values.yaml)
[ "${#values[@]}" -gt 0 ] || fail "no values files under $chart/ci"

for f in "${values[@]}"; do
	name=$(basename "$f" -values.yaml)
	echo "── $name"
	"$HELM" lint --strict "$chart" -f "$f" >/dev/null || { "$HELM" lint --strict "$chart" -f "$f"; fail "helm lint $name"; }
	render -f "$f" >"$out/$name.yaml"
	"$KUBECONFORM" -kubernetes-version "$KUBE_VERSION" -strict -summary \
		-schema-location default -schema-location "$CRD_SCHEMAS" <"$out/$name.yaml"
done

echo "── values schema"
# The schema is strict: a misspelt key or a value of the wrong kind is an error.
for bad in foo=1 api.replicsa=2 postgresql.mode=mysql signing.backend=hsm valkey.sentinel.replicas=2 server.publicBaseURL=http://plux.example.com; do
	if "$HELM" lint "$chart" -f "$chart/ci/default-values.yaml" --set "$bad" >/dev/null 2>&1; then
		fail "values.schema.json accepted $bad"
	fi
done
# Required settings are named in the error, not left to a panic.
for missing in "server.publicBaseURL=" "signing.vault.address="; do
	if render -f "$chart/ci/default-values.yaml" --set "$missing" >/dev/null 2>&1; then
		fail "the chart rendered without ${missing%=}"
	fi
done

echo "── properties"
ha=$out/ha.yaml
# Pod Security "restricted": every workload runs as non-root, and nothing
# weakens the container.
workloads=$(grep -Ec '^kind: (Deployment|StatefulSet|Job|Pod)$' "$ha")
nonroot=$(grep -c 'runAsNonRoot: true' "$ha")
[ "$workloads" -eq "$nonroot" ] || fail "$workloads workloads but $nonroot pods with runAsNonRoot"
if grep -En 'privileged: true|runAsUser: 0$|allowPrivilegeEscalation: true|hostNetwork|hostPID|hostIPC|hostPath|readOnlyRootFilesystem: false|^ +add:|type: Unconfined|automountServiceAccountToken: true' "$ha"; then
	fail "a workload weakens Pod Security restricted"
fi
# L-4: the roles that run tenant code hold no credential, keys or token.
funcs=$(render -f "$chart/ci/ha-values.yaml" --show-only templates/functions.yaml)
if grep -En 'secretKeyRef|valueFrom|PLUX_|postgres-ca|name: keys|name: trust|projected' <<<"$funcs"; then
	fail "fnrunner, egress or builder holds a data credential (L-4)"
fi
# The seccomp profile is the builder's alone.
[ "$(grep -c 'type: Localhost' "$ha")" -eq 1 ] || fail "only the builder uses a Localhost seccomp profile"
# No credential is set inline: a variable that carries one is a reference.
if ! render -f "$chart/ci/ha-values.yaml" | awk '/name: PLUX_[A-Z_]*(TOKEN|PASSWORD|SECRET)/ {getline; if ($1 == "value:") bad = 1} END {exit bad}'; then
	fail "a credential is set inline"
fi
echo "✓ chart: ${#values[@]} values files rendered and validated"
