#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: Apache-2.0

# `make walkthrough-kind` (X1, X5): the scripted regulated-industry audit
# walkthrough of phase 9 against the Helm chart on kind.
#
#   1. a throwaway kind cluster with CloudNativePG, and the chart installed with
#      two replicas of every role (ci/kind-values.yaml plus the overrides
#      below), PostgreSQL in three instances and Valkey with Sentinel;
#   2. an administrator, an organisation and an app are seeded; the api is
#      forwarded to localhost, where the synthetic devices reach it;
#   3. tools/cmd/walkthrough runs the nine steps (docs/compliance/walkthrough.md)
#      with the plux CLI and plux-load, starting an OIDC test provider and a TLS
#      syslog sink on the kind network's gateway address;
#   4. the signed evidence bundle is verified with the key written beside it.
#
# Two hooks need a person's browser and are supplied by the environment, because
# they drive a virtual WebAuthn authenticator that does not belong in this
# script:
#
#   WALKTHROUGH_TOKEN_CMD    prints an access token for $WALKTHROUGH_ACTOR
#                            ($WALKTHROUGH_EMAIL), signed in through the test
#                            provider with its second factor;
#   WALKTHROUGH_STEPUP_CMD   completes the step-up at $WALKTHROUGH_STEPUP_URL
#                            for $WALKTHROUGH_ACTOR.
#
# The enterprise features the steps use (custom roles, SCIM, federation) need
# an enterprise server image and a licence file. WALKTHROUGH_HELM_ARGS holds
# the extra helm arguments that supply them (for example --set image.repository
# and a --values file that mounts the licence and sets config.license.file).
# WALKTHROUGH_VIEWER_CMD, when set, enables the optional emulator step.
#
# Any failure exits non-zero and prints diagnostics. Needs Docker, kind, helm,
# kubectl, go and curl. The cluster is deleted at the end; WALKTHROUGH_KEEP=1
# keeps it. The evidence is written to WALKTHROUGH_OUT (default
# walkthrough-out): evidence/, evidence-bundle.json and evidence-bundle.pub.
# The signing key of the bundle exists only for the run.
set -euo pipefail
export LC_ALL=C

# `walkthrough-kind.sh --dry-run` prints the plan and touches nothing.
if [ "${1:-}" = --dry-run ]; then
	cd "$(dirname "$0")/../../tools"
	exec "${GO:-go}" run ./cmd/walkthrough run -dry-run
fi

HELM=${HELM:-helm}
KIND=${KIND:-kind}
KUBECTL=${KUBECTL:-kubectl}
GO=${GO:-go}
CLUSTER=${WALKTHROUGH_CLUSTER:-plux-walkthrough}
NAMESPACE=plux
IMAGE=${E2E_IMAGE:-plux-server:e2e}
CNPG_VERSION=${CNPG_VERSION:?set CNPG_VERSION}
CNPG_SHA256=${CNPG_SHA256:?set CNPG_SHA256}
API_PORT=${WALKTHROUGH_API_PORT:-18080}
FLEET_ENV=${WALKTHROUGH_FLEET_ENV:-staging}
DEVICES=${WALKTHROUGH_DEVICES:-300}
HELM_ARGS=${WALKTHROUGH_HELM_ARGS:-}

here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../.." && pwd)
chart=$repo/deploy/helm/plux
OUT=${WALKTHROUGH_OUT:-$repo/walkthrough-out}
repository=${IMAGE%:*}
tag=${IMAGE##*:}

fail() { echo "✗ $*" >&2; exit 1; }
step() { echo "── $*"; }

# install [extra helm args]: the HA installation. It is also the rolling
# upgrade, with a pod annotation that rolls every role (the pre-upgrade hook
# migrates the database first).
install() {
	# shellcheck disable=SC2086 # HELM_ARGS is a word list by design
	"$HELM" upgrade --install plux "$chart" --namespace "$NAMESPACE" \
		-f "$chart/ci/kind-values.yaml" \
		--set image.repository="$repository" --set image.tag="$tag" --set image.pullPolicy=Never \
		--set server.publicBaseURL="http://127.0.0.1:$API_PORT" \
		--set api.replicas=2 --set worker.replicas=2 \
		--set config.attestation.developmentProvider=true \
		$HELM_ARGS --wait --timeout 15m "$@"
}

# `walkthrough-kind.sh upgrade` is the upgrade hook the walkthrough runs in
# step 8.
if [ "${1:-}" = upgrade ]; then
	install --set api.podAnnotations.walkthrough-roll="$(date -u +%s)" --set worker.podAnnotations.walkthrough-roll="$(date -u +%s)"
	exit 0
fi

work=$(mktemp -d)
pids=()

for tool in docker "$KIND" "$HELM" "$KUBECTL" "$GO" curl; do
	command -v "$tool" >/dev/null || fail "$tool is required"
done
docker info >/dev/null 2>&1 || fail "the Docker daemon is not running"
[ -n "${WALKTHROUGH_TOKEN_CMD:-}" ] || fail "set WALKTHROUGH_TOKEN_CMD (see the header of this script)"
[ -n "${WALKTHROUGH_STEPUP_CMD:-}" ] || fail "set WALKTHROUGH_STEPUP_CMD (see the header of this script)"

diagnostics() {
	echo "── diagnostics" >&2
	"$KUBECTL" get pods,pvc,cluster.postgresql.cnpg.io -A -o wide >&2 || true
	for pod in $("$KUBECTL" --namespace "$NAMESPACE" get pods -o name 2>/dev/null); do
		echo "── logs $pod" >&2
		"$KUBECTL" --namespace "$NAMESPACE" logs "$pod" --all-containers --tail=40 >&2 || true
	done
}

cleanup() {
	status=$?
	for pid in "${pids[@]:-}"; do [ -n "$pid" ] && kill "$pid" 2>/dev/null || true; done
	if [ "$status" -ne 0 ]; then diagnostics; fi
	if [ -z "${WALKTHROUGH_KEEP:-}" ]; then "$KIND" delete cluster --name "$CLUSTER" >/dev/null 2>&1 || true; fi
	rm -rf "$work"
	exit "$status"
}
trap cleanup EXIT

k() { "$KUBECTL" "$@"; }

# gateway_address: the IPv4 address of the kind network's gateway, where the
# cluster reaches the test provider and the syslog sink running on this host.
gateway_address() {
	docker network inspect kind -f '{{range .IPAM.Config}}{{.Gateway}} {{end}}' |
		tr ' ' '\n' | grep -E '^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$' | head -n 1
}

step "build the CLI, the device fleet and the walkthrough"
(cd "$repo/backend" && "$GO" build -o "$work/plux" ./cmd/plux && "$GO" build -o "$work/plux-load" ./cmd/plux-load)
(cd "$repo/tools" && "$GO" build -o "$work/walkthrough" ./cmd/walkthrough)

step "cluster $CLUSTER"
"$KIND" delete cluster --name "$CLUSTER" >/dev/null 2>&1 || true
"$KIND" create cluster --name "$CLUSTER" --wait 180s
"$KIND" load docker-image "$IMAGE" --name "$CLUSTER"

step "CloudNativePG $CNPG_VERSION"
curl -fsSL --retry 5 -o "$work/cnpg.yaml" \
	"https://github.com/cloudnative-pg/cloudnative-pg/releases/download/v$CNPG_VERSION/cnpg-$CNPG_VERSION.yaml"
echo "$CNPG_SHA256  $work/cnpg.yaml" | sha256sum -c - >/dev/null || fail "cnpg-$CNPG_VERSION.yaml does not match its pinned SHA-256"
k apply --server-side -f "$work/cnpg.yaml" >/dev/null
k --namespace cnpg-system rollout status deployment/cnpg-controller-manager --timeout=300s

step "installation (two replicas of every role)"
k create namespace "$NAMESPACE"
install
k --namespace "$NAMESPACE" wait --for=condition=Ready cluster/plux-pg --timeout=300s

step "seed"
seeded=$(k --namespace "$NAMESPACE" exec deploy/plux-worker -- /usr/local/bin/plux-server seed -config /etc/plux/plux.yaml -out -)
value() { printf '%s\n' "$seeded" | sed -n "s/^PLUX_DEV_$1=//p" | head -n 1; }
ORG=$(value ORGANIZATION)
APP=$(value APP)
export WALKTHROUGH_ADMIN_TOKEN
WALKTHROUGH_ADMIN_TOKEN=$(value TOKEN)
[ -n "$ORG" ] && [ -n "$APP" ] && [ -n "$WALKTHROUGH_ADMIN_TOKEN" ] || fail "seed did not print an organisation, an app and a token"

step "forward the api to localhost:$API_PORT"
k --namespace "$NAMESPACE" port-forward svc/plux-api "$API_PORT:8080" >/dev/null 2>&1 &
pids+=("$!")
curl -fsS --retry 30 --retry-all-errors --retry-delay 2 --max-time 5 "http://127.0.0.1:$API_PORT/livez" >/dev/null ||
	fail "the port-forward to the api does not answer"

# The audit public key, as `plux compliance verify` takes it, read from the
# installation itself.
cat >"$work/audit-key.sh" <<EOF
#!/bin/sh
exec "$KUBECTL" --namespace "$NAMESPACE" exec deploy/plux-worker -- /usr/local/bin/plux-server audit public-key -config /etc/plux/plux.yaml
EOF
chmod +x "$work/audit-key.sh"

step "the walkthrough"
gateway=$(gateway_address)
[ -n "$gateway" ] || fail "the kind network has no IPv4 gateway"
with=""
viewer=()
if [ -n "${WALKTHROUGH_VIEWER_CMD:-}" ]; then
	with="07b-viewer-on-emulator"
	viewer=(-viewer-cmd "$WALKTHROUGH_VIEWER_CMD")
fi
status=0
"$work/walkthrough" run \
	-server "http://127.0.0.1:$API_PORT" -org "$ORG" -app "$APP" -fleet-env "$FLEET_ENV" -devices "$DEVICES" \
	-plux "$work/plux" -plux-load "$work/plux-load" -out "$OUT" -project "$repo/schema/testdata/documents/plux_bank" \
	-advertise "$gateway" \
	-token-cmd "$WALKTHROUGH_TOKEN_CMD" -stepup-cmd "$WALKTHROUGH_STEPUP_CMD" \
	-audit-key-cmd "$work/audit-key.sh" -upgrade-cmd "$here/walkthrough-kind.sh upgrade" \
	-with "$with" ${viewer[@]+"${viewer[@]}"} || status=$?

step "verify the evidence bundle"
"$work/walkthrough" verify -dir "$OUT"
[ "$status" -eq 0 ] || fail "the walkthrough failed (exit $status); the failure is recorded in the signed bundle"
echo "✓ walkthrough: nine steps passed; evidence in $OUT"
