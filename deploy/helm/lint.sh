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
workloads=$(grep -Ec '^kind: (Deployment|StatefulSet|Job|CronJob|Pod)$' "$ha")
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

echo "── backups (DEP-010)"
# Backups archive the WAL at least every archive_timeout and take base backups
# on a schedule; the marker holds no inline credential.
for f in ha backup; do
	grep -q '^kind: ScheduledBackup$' "$out/$f.yaml" || fail "$f: no ScheduledBackup"
	grep -q 'barmanObjectStore:' "$out/$f.yaml" || fail "$f: the Cluster has no object store"
	grep -Eq '^ +archive_timeout: ' "$out/$f.yaml" || fail "$f: the Cluster sets no archive_timeout"
	grep -q 'component: backup-marker' "$out/$f.yaml" || fail "$f: no consistency marker"
done
if ! awk '/name: AWS_(ACCESS_KEY_ID|SECRET_ACCESS_KEY)/ {getline; if ($1 == "value:") bad = 1} END {exit bad}' "$out/ha.yaml"; then
	fail "the consistency marker holds an inline credential"
fi
# Off, the chart renders no backup resource: no ScheduledBackup, no archive.
if grep -Eq '^kind: ScheduledBackup$|barmanObjectStore:|component: backup-marker' "$out/default.yaml"; then
	fail "backups are rendered although postgresql.cnpg.backup.enabled is false"
fi
# A restore creates the cluster from the store, to the target, and archives nowhere.
grep -q '^    recovery:$' "$out/restore.yaml" || fail "restore: the Cluster has no recovery bootstrap"
grep -q 'targetLSN: 0/5000060' "$out/restore.yaml" || fail "restore: the recovery target is missing"
if grep -Eq '^kind: ScheduledBackup$|^ +archive_timeout: ' "$out/restore.yaml"; then
	fail "a restored cluster archives into the store it was restored from"
fi
# A cluster cannot archive into the folder it restores from.
if render -f "$chart/ci/restore-values.yaml" --set postgresql.cnpg.backup.enabled=true >/dev/null 2>&1; then
	fail "the chart rendered a restore that archives into its own source"
fi
# An archive timeout above five minutes would break the RPO; the schema refuses it.
if "$HELM" lint "$chart" -f "$chart/ci/backup-values.yaml" --set postgresql.cnpg.backup.archiveTimeout=10m >/dev/null 2>&1; then
	fail "values.schema.json accepted an archiveTimeout above five minutes"
fi

echo "── dev sessions (DEV-002)"
# The api ingress routes the WebSocket upgrade path and, on nginx, holds an idle
# upgraded connection longer than devSession.idleTimeout (120 s by default).
ing=$(render -f "$chart/ci/ha-values.yaml" --show-only templates/api.yaml)
grep -q 'path: /dev/socket' <<<"$ing" || fail "the api ingress does not route /dev/socket"
grep -q 'nginx.ingress.kubernetes.io/proxy-read-timeout: "150"' <<<"$ing" || fail "the ingress read timeout is not idleTimeout (120s) plus 30 s"
grep -q 'nginx.ingress.kubernetes.io/proxy-send-timeout: "150"' <<<"$ing" || fail "the ingress send timeout is not idleTimeout (120s) plus 30 s"
ing=$(render -f "$chart/ci/ha-values.yaml" --set devSession.idleTimeout=10m --show-only templates/api.yaml)
grep -q 'proxy-read-timeout: "630"' <<<"$ing" || fail "the ingress read timeout does not follow devSession.idleTimeout"
# The server's devSession section comes from the values.
cfg=$(render -f "$chart/ci/ha-values.yaml" --set devSession.pingInterval=20s --set 'devSession.enabledEnvironments={development,staging}' --show-only templates/configmap.yaml)
grep -Pzq 'devSession:\n +enabledEnvironments:\n +- development\n +- staging\n' <<<"$cfg" || fail "devSession.enabledEnvironments is not in the api configuration"
grep -q 'idleTimeout: 120s' <<<"$cfg" || fail "devSession.idleTimeout is not in the api configuration"
grep -q 'pingInterval: 20s' <<<"$cfg" || fail "devSession.pingInterval is not in the api configuration"
# Disabled, the ingress carries neither the path nor the timeouts; a bad duration is refused.
ing=$(render -f "$chart/ci/ha-values.yaml" --set 'devSession.enabledEnvironments=null' --show-only templates/api.yaml)
if grep -Eq '/dev/socket|proxy-read-timeout' <<<"$ing"; then
	fail "the ingress routes /dev/socket although dev sessions are disabled"
fi
if "$HELM" lint "$chart" -f "$chart/ci/default-values.yaml" --set devSession.idleTimeout=soon >/dev/null 2>&1; then
	fail "values.schema.json accepted a devSession.idleTimeout that is not a duration"
fi

echo "✓ chart: ${#values[@]} values files rendered and validated"
