#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: Apache-2.0

# `make restore-drill` (DEP-010, X4): proves that a Plux installation can be
# restored from its backups, and measures how long and how much it costs.
#
#   1. a throwaway kind cluster with CloudNativePG, and the chart installed with
#      continuous WAL archiving, base backups and the consistency marker
#      (ci/backup-values.yaml); versioning is switched on for both buckets;
#   2. an administrator, an organisation and apps are seeded, a base backup is
#      taken, and a workload publishes and promotes a release every few seconds
#      (new audit entries, releases and bundles) for DRILL_LOAD_SECONDS;
#   3. RPO: the newest committed write must reach the WAL archive within
#      RPO_LIMIT seconds (default 300) while the primary is quiet;
#   4. a restore point is drawn at random from the consistency markers written
#      within the last DRILL_WINDOW_SECONDS (default 3600) after the base backup;
#   5. RTO clock starts: a new namespace gets the signing keys, and the chart
#      installed with ci/restore-values.yaml creates the database from the
#      archive to the marker's WAL position and starts the server on it;
#   6. the restored installation is verified: it is ready; every release row
#      and audit entry it holds is the source's row, and every source row older
#      than the restore point is present; every bundle it names is in the object
#      store, whole; `plux pull` fetches the channel's release with its bundles
#      checked against the hashes the server recorded;
#      `plux-server audit verify` walks the audit chains and checkpoints;
#   7. RTO clock stops. RPO and RTO are printed; RTO must be at most RTO_LIMIT
#      seconds (default 3600).
#
# Any failure exits non-zero. Needs Docker, kind, helm, kubectl, go, curl and
# the image built by the make target. The cluster is deleted at the end;
# DRILL_KEEP=1 keeps it. DRILL_SEED=<n> repeats a drill's random restore point.
set -Eeuo pipefail
# The failing line and command, printed last so a CI log shows it first.
trap 'failed_at="line $LINENO: $BASH_COMMAND"' ERR
export LC_ALL=C

HELM=${HELM:-helm}
KIND=${KIND:-kind}
KUBECTL=${KUBECTL:-kubectl}
GO=${GO:-go}
CLUSTER=${DRILL_CLUSTER:-plux-drill}
SRC_NS=plux
DST_NS=plux-restore
IMAGE=${E2E_IMAGE:-plux-server:e2e}
CNPG_VERSION=${CNPG_VERSION:?set CNPG_VERSION}
CNPG_SHA256=${CNPG_SHA256:?set CNPG_SHA256}
LOAD_SECONDS=${DRILL_LOAD_SECONDS:-420}
INTERVAL=${DRILL_PUBLISH_INTERVAL:-20}
WINDOW_SECONDS=${DRILL_WINDOW_SECONDS:-3600}
RPO_LIMIT=${RPO_LIMIT:-300}
RTO_LIMIT=${RTO_LIMIT:-3600}
SEED=${DRILL_SEED:-$((RANDOM * 32768 + RANDOM))}
# The tool pod's image: the chart's test image, which has curl and a tar.
TOOLS_IMAGE=${DRILL_TOOLS_IMAGE:-curlimages/curl:8.23.0@sha256:a134550be58ab783810e613ae96fd253c38f7bdd16d9bf9be350700c7a29ac06}
PG=plux-pg
DATA_BUCKET=plux
BACKUP_BUCKET=plux-backups
MARKER_PREFIX=markers/postgresql
EMPTY_SHA=e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855

here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../.." && pwd)
chart=$here/plux
work=$(mktemp -d)
pids=()

fail() { echo "✗ $*" >&2; exit 1; }
step() { echo "── $*"; }
now() { date -u +%s; }

for tool in docker "$KIND" "$HELM" "$KUBECTL" "$GO" curl sha256sum awk; do
	command -v "$tool" >/dev/null || fail "$tool is required"
done
docker info >/dev/null 2>&1 || fail "the Docker daemon is not running"

diagnostics() {
	echo "── diagnostics" >&2
	# Why pods are not running: the warning events and the description of
	# every pod that is neither running nor finished, in every namespace.
	echo "── warning events" >&2
	"$KUBECTL" get events -A --field-selector type=Warning --sort-by=.lastTimestamp 2>/dev/null | tail -n 40 >&2 || true
	"$KUBECTL" get pods -A --field-selector=status.phase!=Running,status.phase!=Succeeded \
		-o custom-columns=NS:.metadata.namespace,NAME:.metadata.name --no-headers 2>/dev/null |
		while read -r ns name; do
			echo "── describe $ns/$name" >&2
			"$KUBECTL" --namespace "$ns" describe pod "$name" 2>/dev/null | sed -n '/^Events:/,$p' >&2 || true
		done
	"$KUBECTL" get pods,pvc,cluster.postgresql.cnpg.io,backup.postgresql.cnpg.io -A -o wide >&2 || true
	for ns in "$SRC_NS" "$DST_NS"; do
		for pod in $("$KUBECTL" --namespace "$ns" get pods -o name 2>/dev/null); do
			echo "── logs $ns/$pod" >&2
			"$KUBECTL" --namespace "$ns" logs "$pod" --all-containers --tail=40 >&2 || true
		done
	done
}

cleanup() {
	status=$?
	for pid in "${pids[@]:-}"; do [ -n "$pid" ] && kill "$pid" 2>/dev/null || true; done
	if [ "$status" -ne 0 ]; then
		diagnostics
		echo "✗ failed at ${failed_at:-an unknown point} (exit $status)" >&2
	fi
	if [ -z "${DRILL_KEEP:-}" ]; then "$KIND" delete cluster --name "$CLUSTER" >/dev/null 2>&1 || true; fi
	rm -rf "$work"
	exit "$status"
}
trap cleanup EXIT

# Every helm and kubectl call talks to the kind cluster through a kubeconfig of
# its own, never to whatever cluster the caller's environment points at.
export KUBECONFIG=$work/kubeconfig

# ── helpers ────────────────────────────────────────────────────────────────

k() { "$KUBECTL" "$@"; }

# forward <namespace> <service> <local port>: the service's port 8080 on
# localhost. Returns once the forward answers /livez; starts it again if
# kubectl exits (for example because no pod backs the service yet).
forward() {
	local attempt pid
	for attempt in $(seq 1 30); do
		k --namespace "$1" port-forward "svc/$2" "$3:8080" >/dev/null 2>&1 &
		pid=$!
		pids+=("$pid")
		if wait_for_forward "$pid" "$3"; then return 0; fi
		kill "$pid" 2>/dev/null || true
		sleep 2
	done
	fail "port-forward to $1/$2 does not answer"
}

# wait_for_forward <pid> <local port>: succeeds once the port-forward process
# <pid> is alive and the port answers /livez, within about a minute.
wait_for_forward() {
	local i
	for i in $(seq 1 30); do
		kill -0 "$1" 2>/dev/null || return 1
		if curl -fsS --max-time 5 "http://127.0.0.1:$2/livez" >/dev/null 2>&1; then return 0; fi
		sleep 2
	done
	return 1
}

# primary <namespace>: the pod of the PostgreSQL primary.
primary() { k --namespace "$1" get pod -l "cnpg.io/cluster=$PG,cnpg.io/instanceRole=primary" -o name | head -n 1; }

# sql <namespace> <statement>: rows as |-separated lines, as the postgres user over the pod's socket.
sql() { k --namespace "$1" exec "$(primary "$1")" -c postgres -- psql -X -q -A -t -F '|' -v ON_ERROR_STOP=1 -d plux -c "$2"; }

# s3get <url-path> [curl args]: a signed GET inside the cluster, from a pod the
# chart's NetworkPolicies treat as the worker.
s3get() {
	local path=$1
	shift
	k --namespace "$SRC_NS" exec drill-tools -- sh -c \
		'url=$1; shift; exec curl -fsS --max-time 60 --aws-sigv4 "aws:amz:us-east-1:s3" --user "$AWS_ACCESS_KEY_ID:$AWS_SECRET_ACCESS_KEY" -H "x-amz-content-sha256: '"$EMPTY_SHA"'" "$@" "$url"' \
		sh "http://plux-seaweedfs:8333/$path" "$@"
}

# enable_versioning <bucket>: turns object versioning on and checks that it took.
enable_versioning() {
	local bucket=$1 body sum
	body='<VersioningConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Status>Enabled</Status></VersioningConfiguration>'
	sum=$(printf '%s' "$body" | sha256sum | cut -d' ' -f1)
	k --namespace "$SRC_NS" exec drill-tools -- sh -c \
		'exec curl -fsS --max-time 60 --aws-sigv4 "aws:amz:us-east-1:s3" --user "$AWS_ACCESS_KEY_ID:$AWS_SECRET_ACCESS_KEY" -H "x-amz-content-sha256: $1" -X PUT -d "$2" "http://plux-seaweedfs:8333/$3?versioning"' \
		sh "$sum" "$body" "$bucket" >/dev/null || fail "could not enable versioning on bucket $bucket"
	s3get "$bucket?versioning" | grep -q '<Status>Enabled</Status>' || fail "bucket $bucket is not versioned"
	echo "bucket $bucket: versioning enabled"
}

# ── 1. cluster, operator, source installation ──────────────────────────────

step "build the CLI"
(cd "$repo/backend" && "$GO" build -o "$work/plux" ./cmd/plux)

step "cluster $CLUSTER"
"$KIND" delete cluster --name "$CLUSTER" >/dev/null 2>&1 || true
"$KIND" create cluster --name "$CLUSTER" --kubeconfig "$KUBECONFIG" --wait 180s
"$KUBECTL" version >/dev/null 2>&1 || fail "the kind cluster $CLUSTER is not reachable with the kubeconfig $KUBECONFIG"
"$KIND" load docker-image "$IMAGE" --name "$CLUSTER"

step "CloudNativePG $CNPG_VERSION"
curl -fsSL --retry 5 -o "$work/cnpg.yaml" \
	"https://github.com/cloudnative-pg/cloudnative-pg/releases/download/v$CNPG_VERSION/cnpg-$CNPG_VERSION.yaml"
echo "$CNPG_SHA256  $work/cnpg.yaml" | sha256sum -c - >/dev/null || fail "cnpg-$CNPG_VERSION.yaml does not match its pinned SHA-256"
k apply --server-side -f "$work/cnpg.yaml" >/dev/null
k --namespace cnpg-system rollout status deployment/cnpg-controller-manager --timeout=300s

step "source installation with backups"
repository=${IMAGE%:*}
tag=${IMAGE##*:}
k create namespace "$SRC_NS"
"$HELM" upgrade --install plux "$chart" --namespace "$SRC_NS" \
	-f "$chart/ci/kind-values.yaml" -f "$chart/ci/backup-values.yaml" \
	--set functions.enabled=false \
	--set image.repository="$repository" --set image.tag="$tag" --set image.pullPolicy=Never \
	--wait --timeout 15m
k --namespace "$SRC_NS" wait --for=condition=Ready "cluster/$PG" --timeout=300s

# Tools: curl for the object store and tar for the signing keys. The labels make
# the chart's NetworkPolicies treat it as the worker (object store access).
k apply -f - <<EOF >/dev/null
apiVersion: v1
kind: Pod
metadata:
  name: drill-tools
  namespace: $SRC_NS
  labels: {app.kubernetes.io/name: plux, app.kubernetes.io/instance: plux, app.kubernetes.io/component: worker}
spec:
  automountServiceAccountToken: false
  securityContext: {runAsNonRoot: true, runAsUser: 65532, runAsGroup: 65532, fsGroup: 65532, seccompProfile: {type: RuntimeDefault}}
  containers:
    - name: tools
      image: $TOOLS_IMAGE
      command: [sleep, "7200"]
      securityContext: {allowPrivilegeEscalation: false, readOnlyRootFilesystem: true, capabilities: {drop: [ALL]}}
      env:
        - {name: AWS_ACCESS_KEY_ID, valueFrom: {secretKeyRef: {name: plux-seaweedfs-s3, key: accessKeyID}}}
        - {name: AWS_SECRET_ACCESS_KEY, valueFrom: {secretKeyRef: {name: plux-seaweedfs-s3, key: secretAccessKey}}}
      volumeMounts: [{name: keys, mountPath: /keys, readOnly: true}]
  volumes:
    - {name: keys, persistentVolumeClaim: {claimName: plux-keys}}
---
# The restored installation, in its own namespace, reads the source's object
# store (the bundles) and its backup folder.
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata: {name: drill-allow-restore, namespace: $SRC_NS}
spec:
  podSelector: {matchLabels: {app.kubernetes.io/component: seaweedfs}}
  policyTypes: [Ingress]
  ingress:
    - from: [{namespaceSelector: {matchLabels: {kubernetes.io/metadata.name: $DST_NS}}}]
      ports: [{protocol: TCP, port: 8333}]
EOF
k --namespace "$SRC_NS" wait --for=condition=Ready pod/drill-tools --timeout=300s

step "object storage versioning (DEP-010)"
enable_versioning "$DATA_BUCKET"
enable_versioning "$BACKUP_BUCKET"

# ── 2. data, base backup, workload ─────────────────────────────────────────

step "seed"
seeded=$(k --namespace "$SRC_NS" exec deploy/plux-worker -- /usr/local/bin/plux-server seed -config /etc/plux/plux.yaml -out -)
value() { printf '%s\n' "$seeded" | sed -n "s/^PLUX_DEV_$1=//p" | head -n 1; }
ORG=$(value ORGANIZATION)
APP=$(value APP)
export PLUX_TOKEN
PLUX_TOKEN=$(value TOKEN)
[ -n "$ORG" ] && [ -n "$APP" ] && [ -n "$PLUX_TOKEN" ] || fail "seed did not print an organisation, an app and a token"

cp -r "$repo/schema/testdata/documents/loan-calculator" "$work/project"
forward "$SRC_NS" plux-api 18081

# Publishing and promoting need a step-up (SEC-103): the CLI asks the server
# for a request and waits for a person to confirm it with a security key in a
# browser. The drill has no person, so a background loop plays the browser as
# the server's tests do: it records the outcome of the ceremony on every
# pending CLI request. The CLI's side (the request, the poll, the grant bound
# to its key) runs for real. The ceremony itself is covered by the auth tests.
(
	while :; do
		sql "$SRC_NS" "update step_up_requests set state = 'approved', approved_at = now(), challenge = null where kind = 'cli' and state = 'pending'" >/dev/null 2>&1 || true
		sleep 1
	done
) &
pids+=("$!")

# The CLI reaches the server through the port-forward, but the server compares
# the htu of its DPoP proofs with its configured public base URL (SEC-022), so
# the CLI is told that URL (the chart's server.publicBaseURL in ci/*-values.yaml).
PUBLIC_URL=https://plux.example.com
publish() { PLUX_PUBLIC_URL=$PUBLIC_URL "$work/plux" publish --server http://127.0.0.1:18081 --org "$ORG" --app "$APP" -C "$work/project" --promote staging --notes "$1" >/dev/null; }
publish "drill initial"

step "base backup"
k apply -f - <<EOF >/dev/null
apiVersion: postgresql.cnpg.io/v1
kind: Backup
metadata: {name: drill-base, namespace: $SRC_NS}
spec:
  cluster: {name: $PG}
  method: barmanObjectStore
  target: prefer-standby
EOF
k --namespace "$SRC_NS" wait --for=jsonpath='{.status.phase}'=completed backup/drill-base --timeout=900s
stopped=$(k --namespace "$SRC_NS" get backup/drill-base -o jsonpath='{.status.stoppedAt}')
[ -n "$stopped" ] || fail "the base backup has no stop time"
echo "base backup finished at $stopped"

step "workload for ${LOAD_SECONDS}s"
load_start=$(now)
n=0
while [ $(($(now) - load_start)) -lt "$LOAD_SECONDS" ]; do
	n=$((n + 1))
	publish "drill $n"
	sleep "$INTERVAL"
done
load_end=$(now)
echo "$n publishes"

# ── 3. RPO ─────────────────────────────────────────────────────────────────

step "RPO"
# The segment that holds the newest committed write must be in the archive
# within RPO_LIMIT seconds although nothing forces it out but archive_timeout.
last_write=$(sql "$SRC_NS" "select extract(epoch from max(occurred_at))::bigint from audit_log")
[ -n "$last_write" ] || fail "the audit log is empty: the workload wrote nothing"
want=$(sql "$SRC_NS" "select pg_walfile_name(pg_current_wal_lsn())")
rpo=-1
while :; do
	row=$(sql "$SRC_NS" "select coalesce(last_archived_wal >= '$want', false), coalesce(extract(epoch from last_archived_time)::bigint, 0) from pg_stat_archiver")
	if [ "${row%%|*}" = t ]; then
		rpo=$(( ${row##*|} - last_write ))
		[ "$rpo" -ge 0 ] || rpo=0
		break
	fi
	[ $(($(now) - last_write)) -lt "$RPO_LIMIT" ] || break
	sleep 2
done
[ "$rpo" -ge 0 ] || fail "the newest write was not archived within ${RPO_LIMIT}s (RPO)"
failed_archives=$(sql "$SRC_NS" "select failed_count from pg_stat_archiver")
[ "$failed_archives" -eq 0 ] || echo "note: $failed_archives archive attempts failed and were retried"

# ── 4. the restore point ───────────────────────────────────────────────────

step "restore point (seed $SEED)"
stamp() { date -u -d "@$1" +%Y%m%dT%H%M%SZ; }
low=$(date -u -d "$stopped" +%s)
low=$((low + 5))
oldest=$(($(now) - WINDOW_SECONDS))
[ "$low" -ge "$oldest" ] || low=$oldest
# A marker ends the window 30 s before the workload does, so every row the
# marker's time covers has been archived and committed.
high=$((load_end - 30))
mapfile -t markers < <(s3get "$DATA_BUCKET?list-type=2&prefix=$MARKER_PREFIX/" | grep -o '<Key>[^<]*</Key>' | sed 's|<Key>||;s|</Key>||' | sort)
candidates=()
for key in "${markers[@]}"; do
	id=$(basename "$key" .json)
	if [[ "$id" > "$(stamp "$low")" && "$id" < "$(stamp "$high")" ]]; then candidates+=("$key"); fi
done
[ "${#candidates[@]}" -gt 0 ] || fail "no consistency marker between the base backup and the end of the workload (${#markers[@]} markers in the bucket)"
pick=$(awk -v seed="$SEED" -v n="${#candidates[@]}" 'BEGIN { srand(seed); print int(rand() * n) }')
marker_key=${candidates[$pick]}
marker=$(s3get "$DATA_BUCKET/$marker_key")
LSN=$(printf '%s' "$marker" | sed -n 's/.*"lsn":"\([^"]*\)".*/\1/p')
MARKER_TIME=$(printf '%s' "$marker" | sed -n 's/.*"time":"\([^"]*\)".*/\1/p')
[ -n "$LSN" ] && [ -n "$MARKER_TIME" ] || fail "marker $marker_key is malformed"
marker_epoch=$(date -u -d "$MARKER_TIME" +%s)
echo "marker $marker_key: WAL position $LSN at $MARKER_TIME ($(( ($(now) - marker_epoch) / 60 )) min ago, ${#candidates[@]} candidates)"

# What the source holds, to compare with later. Rows are immutable: releases
# and the audit log are append-only.
dump_source() {
	sql "$SRC_NS" "select id, encode(bundle_sha256, 'hex'), encode(signature, 'hex'), extract(epoch from created_at)::bigint from plugin_versions order by id" >"$work/src-releases.txt"
	sql "$SRC_NS" "select coalesce(organization_id::text, ''), sequence, entry_hash, extract(epoch from occurred_at)::bigint from audit_log order by 1, 2" >"$work/src-audit.txt"
}
dump_source

# ── 5. restore ─────────────────────────────────────────────────────────────

step "restore into namespace $DST_NS"
restore_start=$(now)
k create namespace "$DST_NS"
# The credential for the source's object store, the signing keys (the audit
# checkpoints and the sealed values are bound to them) and the installation.
k --namespace "$DST_NS" create secret generic plux-source-s3 \
	--from-literal=accessKeyID="$(k --namespace "$SRC_NS" get secret plux-seaweedfs-s3 -o jsonpath='{.data.accessKeyID}' | base64 -d)" \
	--from-literal=secretAccessKey="$(k --namespace "$SRC_NS" get secret plux-seaweedfs-s3 -o jsonpath='{.data.secretAccessKey}' | base64 -d)" >/dev/null
k apply -f - <<EOF >/dev/null
apiVersion: v1
kind: PersistentVolumeClaim
metadata: {name: plux-keys, namespace: $DST_NS}
spec:
  accessModes: [ReadWriteOnce]
  resources: {requests: {storage: 1Gi}}
---
apiVersion: v1
kind: Pod
metadata: {name: drill-keys, namespace: $DST_NS}
spec:
  automountServiceAccountToken: false
  securityContext: {runAsNonRoot: true, runAsUser: 65532, runAsGroup: 65532, fsGroup: 65532, seccompProfile: {type: RuntimeDefault}}
  containers:
    - name: tools
      image: $TOOLS_IMAGE
      command: [sleep, "600"]
      securityContext: {allowPrivilegeEscalation: false, readOnlyRootFilesystem: true, capabilities: {drop: [ALL]}}
      volumeMounts: [{name: keys, mountPath: /keys}]
  volumes:
    - {name: keys, persistentVolumeClaim: {claimName: plux-keys}}
EOF
k --namespace "$DST_NS" wait --for=condition=Ready pod/drill-keys --timeout=300s
k --namespace "$SRC_NS" exec drill-tools -- tar -C /keys -cf - . | k --namespace "$DST_NS" exec -i drill-keys -- tar -C /keys -xf -
k --namespace "$DST_NS" delete pod drill-keys --wait=true >/dev/null

"$HELM" upgrade --install plux "$chart" --namespace "$DST_NS" \
	-f "$chart/ci/kind-values.yaml" -f "$chart/ci/restore-values.yaml" \
	--set functions.enabled=false \
	--set postgresql.cnpg.restore.targetLSN="$LSN" \
	--set image.repository="$repository" --set image.tag="$tag" --set image.pullPolicy=Never \
	--wait --timeout 30m
k --namespace "$DST_NS" wait --for=condition=Ready "cluster/$PG" --timeout=600s

# ── 6. verification ────────────────────────────────────────────────────────

step "verify: the restored server is ready"
forward "$DST_NS" plux-api 18082
curl -fsS --max-time 10 http://127.0.0.1:18082/readyz >/dev/null || fail "the restored server is not ready"

step "verify: releases and audit entries against the source"
# Every row the restored database holds is the source's row (nothing invented,
# nothing altered); every source row older than the restore point, less a
# margin for a commit in flight, is present (nothing lost).
cut=$((marker_epoch - 30))
compare() { # name restored-query source-file
	local name=$1 query=$2 src=$3
	sql "$DST_NS" "$query" >"$work/dst-$name.txt"
	local rows
	rows=$(wc -l <"$work/dst-$name.txt")
	[ "$rows" -gt 0 ] || fail "the restored $name are empty: nothing to verify"
	sort -u "$src" >"$work/s-$name.txt"
	sort -u "$work/dst-$name.txt" >"$work/d-$name.txt"
	if [ -n "$(comm -13 "$work/s-$name.txt" "$work/d-$name.txt")" ]; then
		comm -13 "$work/s-$name.txt" "$work/d-$name.txt" | head -n 5 >&2
		fail "the restored $name hold rows the source does not"
	fi
	awk -F'|' -v cut="$cut" '$NF <= cut' "$work/s-$name.txt" | sort -u >"$work/s-cut-$name.txt"
	if [ -n "$(comm -23 "$work/s-cut-$name.txt" "$work/d-$name.txt")" ]; then
		comm -23 "$work/s-cut-$name.txt" "$work/d-$name.txt" | head -n 5 >&2
		fail "the restored $name miss rows the source had before the restore point"
	fi
	echo "$name: $rows restored rows match the source ($(wc -l <"$work/s-cut-$name.txt") older than the restore point)"
}
compare releases "select id, encode(bundle_sha256, 'hex'), encode(signature, 'hex'), extract(epoch from created_at)::bigint from plugin_versions order by id" "$work/src-releases.txt"
compare audit "select coalesce(organization_id::text, ''), sequence, entry_hash, extract(epoch from occurred_at)::bigint from audit_log order by 1, 2" "$work/src-audit.txt"

step "verify: bundles in the object store"
# A bundle's key is its bundle hash (BND-005), which covers the header and the
# section digests, not the file's bytes; the hash is checked below by plux pull.
# Here every bundle the restored database names must exist, whole: same size.
checked=0
while IFS='|' read -r hash size; do
	got=$(s3get "$DATA_BUCKET/bundles/${hash:0:2}/$hash" | wc -c)
	[ "$got" -eq "$size" ] || fail "bundle $hash: the object store holds $got bytes, the database says $size"
	checked=$((checked + 1))
done < <(sql "$DST_NS" "select distinct encode(bundle_sha256, 'hex'), bundle_size from plugin_versions order by 1")
[ "$checked" -gt 0 ] || fail "the restored database names no bundle"
echo "$checked bundles in the object store have the size the restored database records"

step "verify: manifests and bundles through the restored server (plux pull)"
"$work/plux" pull --server http://127.0.0.1:18082 --org "$ORG" --app "$APP" --env staging -o "$work/pulled" >/dev/null ||
	fail "plux pull against the restored server failed"
ls "$work/pulled/bundles" >/dev/null 2>&1 || fail "plux pull wrote no bundles"

step "verify: the audit chain"
k --namespace "$DST_NS" exec deploy/plux-worker -- /usr/local/bin/plux-server audit verify -config /etc/plux/plux.yaml ||
	fail "plux-server audit verify failed on the restored database"

# ── 7. result ──────────────────────────────────────────────────────────────

restore_end=$(now)
rto=$((restore_end - restore_start))
size=$(sql "$DST_NS" "select pg_database_size('plux')")
echo
echo "restore point : $MARKER_TIME (WAL $LSN), seed $SEED"
echo "RPO           : ${rpo}s (limit ${RPO_LIMIT}s): the newest write was in the archive after ${rpo}s"
echo "RTO           : ${rto}s (limit ${RTO_LIMIT}s) for a database of $size bytes"
[ "$rto" -le "$RTO_LIMIT" ] || fail "RTO ${rto}s exceeds ${RTO_LIMIT}s"
[ "$rpo" -le "$RPO_LIMIT" ] || fail "RPO ${rpo}s exceeds ${RPO_LIMIT}s"
echo "✓ restore drill: restored, verified (releases, bundles, manifests, audit chain), RPO and RTO within limits"
