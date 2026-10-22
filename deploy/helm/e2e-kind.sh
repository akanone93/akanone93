#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: Apache-2.0

# `make e2e-kind` (DEP-003, DEP-004): installs the chart on a throwaway kind
# cluster with its dependencies and checks it:
#
#   1. the CloudNativePG operator at the pinned release, its manifest verified
#      against a pinned SHA-256 before it is applied;
#   2. the chart with ci/kind-values.yaml and the locally built server image:
#      three PostgreSQL instances, Valkey with Sentinel, SeaweedFS, the api,
#      worker, fnrunner and egress roles (two replicas each);
#   3. `helm test` (the api answers ready and refuses an unauthenticated
#      manifest request);
#   4. a rolling upgrade, whose pre-upgrade hook migrates the database, then
#      `helm test` again;
#   5. failover: the Valkey master and the PostgreSQL primary are deleted, the
#      installation recovers and `helm test` passes again.
#
# Needs Docker, kind, helm and kubectl. The cluster is deleted at the end;
# E2E_KEEP=1 keeps it.
set -euo pipefail

HELM=${HELM:-helm}
KIND=${KIND:-kind}
KUBECTL=${KUBECTL:-kubectl}
CLUSTER=${E2E_CLUSTER:-plux-e2e}
NAMESPACE=plux
IMAGE=${E2E_IMAGE:-plux-server:e2e}
CNPG_VERSION=${CNPG_VERSION:?set CNPG_VERSION}
CNPG_SHA256=${CNPG_SHA256:?set CNPG_SHA256}

here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../.." && pwd)
chart=$here/plux
work=$(mktemp -d)

fail() { echo "✗ $*" >&2; exit 1; }

for tool in docker "$KIND" "$HELM" "$KUBECTL" curl sha256sum; do
	command -v "$tool" >/dev/null || fail "$tool is required"
done
docker info >/dev/null 2>&1 || fail "the Docker daemon is not running"

diagnostics() {
	echo "── diagnostics" >&2
	"$KUBECTL" get pods,pvc,cluster.postgresql.cnpg.io -A -o wide >&2 || true
	"$KUBECTL" --namespace "$NAMESPACE" describe pods >&2 || true
	for pod in $("$KUBECTL" --namespace "$NAMESPACE" get pods -o name 2>/dev/null); do
		echo "── logs $pod" >&2
		"$KUBECTL" --namespace "$NAMESPACE" logs "$pod" --all-containers --tail=60 >&2 || true
	done
}

cleanup() {
	status=$?
	if [ "$status" -ne 0 ]; then diagnostics; fi
	if [ -z "${E2E_KEEP:-}" ]; then "$KIND" delete cluster --name "$CLUSTER" >/dev/null 2>&1 || true; fi
	rm -rf "$work"
	exit "$status"
}
trap cleanup EXIT

# Every helm and kubectl call talks to the kind cluster through a kubeconfig of
# its own, never to whatever cluster the caller's environment points at.
export KUBECONFIG=$work/kubeconfig

echo "── cluster $CLUSTER"
"$KIND" delete cluster --name "$CLUSTER" >/dev/null 2>&1 || true
"$KIND" create cluster --name "$CLUSTER" --kubeconfig "$KUBECONFIG" --wait 180s
"$KUBECTL" version >/dev/null 2>&1 || fail "the kind cluster $CLUSTER is not reachable with the kubeconfig $KUBECONFIG"
"$KIND" load docker-image "$IMAGE" --name "$CLUSTER"

echo "── CloudNativePG $CNPG_VERSION"
manifest=$work/cnpg.yaml
curl -fsSL --retry 5 -o "$manifest" \
	"https://github.com/cloudnative-pg/cloudnative-pg/releases/download/v$CNPG_VERSION/cnpg-$CNPG_VERSION.yaml"
echo "$CNPG_SHA256  $manifest" | sha256sum -c - >/dev/null || fail "cnpg-$CNPG_VERSION.yaml does not match its pinned SHA-256"
"$KUBECTL" apply --server-side -f "$manifest" >/dev/null
"$KUBECTL" --namespace cnpg-system rollout status deployment/cnpg-controller-manager --timeout=300s

echo "── install"
"$KUBECTL" create namespace "$NAMESPACE"
repository=${IMAGE%:*}
tag=${IMAGE##*:}
install() {
	"$HELM" upgrade --install plux "$chart" --namespace "$NAMESPACE" \
		-f "$chart/ci/kind-values.yaml" \
		--set image.repository="$repository" --set image.tag="$tag" --set image.pullPolicy=Never \
		--wait --timeout 15m "$@"
}
install
"$KUBECTL" --namespace "$NAMESPACE" wait --for=condition=Ready cluster/plux-pg --timeout=300s

test_chart() { "$HELM" test plux --namespace "$NAMESPACE" --logs --timeout 5m; }
echo "── helm test"
test_chart

echo "── rolling upgrade"
# A new pod annotation rolls every role; the pre-upgrade hook migrates first.
install --set api.podAnnotations.e2e-roll=1 --set worker.podAnnotations.e2e-roll=1
test_chart

echo "── failover: Valkey master"
master=$("$KUBECTL" --namespace "$NAMESPACE" exec plux-valkey-1 -c sentinel -- sh -c \
	'valkey-cli --tls --cacert /tls/ca.crt -p 26379 sentinel get-master-addr-by-name plux | head -n 1')
echo "master: $master"
"$KUBECTL" --namespace "$NAMESPACE" delete pod "${master%%.*}" --wait=true
"$KUBECTL" --namespace "$NAMESPACE" rollout status statefulset/plux-valkey --timeout=300s
test_chart

echo "── failover: PostgreSQL primary"
"$KUBECTL" --namespace "$NAMESPACE" delete pod -l cnpg.io/cluster=plux-pg,cnpg.io/instanceRole=primary --wait=true
"$KUBECTL" --namespace "$NAMESPACE" wait --for=condition=Ready cluster/plux-pg --timeout=300s
test_chart

echo "✓ kind end-to-end: install, helm test, rolling upgrade and two failovers"
