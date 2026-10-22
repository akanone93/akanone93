#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: Apache-2.0

# `make walkthrough-kind` (X1, X5): the scripted regulated-industry audit
# walkthrough of phase 9 against the Helm chart on kind.
#
#   1. a throwaway kind cluster with CloudNativePG, and the chart installed with
#      two replicas of every role (ci/kind-values.yaml plus the overrides
#      below), PostgreSQL in three instances and Valkey with Sentinel; the api
#      terminates TLS with a certificate of a CA made for the run, so the
#      public base URL, PLUX_SERVER and the DPoP htu are all https;
#   2. an administrator, an organisation and an app are seeded; the api is
#      forwarded to localhost, where the synthetic devices reach it;
#   3. tools/cmd/walkthrough runs the nine steps (docs/compliance/walkthrough.md)
#      with the plux CLI and plux-load, starting an OIDC test provider and a TLS
#      syslog sink on the kind network's gateway address;
#   4. the signed evidence bundle is verified with the key written beside it.
#
# Two hooks need a person's browser. They default to tools/cmd/webauthn-driver,
# a software WebAuthn authenticator built for the run, and may be replaced
# through the environment:
#
#   WALKTHROUGH_TOKEN_CMD    prints an access token for $WALKTHROUGH_ACTOR
#                            ($WALKTHROUGH_EMAIL), signed in through the test
#                            provider with its second factor;
#   WALKTHROUGH_STEPUP_CMD   completes the step-up at $WALKTHROUGH_STEPUP_URL
#                            for $WALKTHROUGH_ACTOR.
#
# The enterprise features the steps use (custom roles, SCIM, federation) need
# the enterprise server image (E2E_IMAGE, built from the server-ee target) and
# a signed licence: WALKTHROUGH_LICENSE_FILE names the file, which becomes the
# Secret plux-license (license.existingSecret). No signing key for licences
# exists in the repository; CI takes the file from the secret
# PLUX_TEST_LICENSE. WALKTHROUGH_HELM_ARGS holds further helm arguments.
# WALKTHROUGH_VIEWER_CMD, when set, enables the optional emulator step.
#
# Any failure exits non-zero and prints diagnostics. Needs Docker, kind, helm,
# kubectl, go, curl and openssl. The cluster is deleted at the end; WALKTHROUGH_KEEP=1
# keeps it. The evidence is written to WALKTHROUGH_OUT (default
# walkthrough-out): evidence/, evidence-bundle.json and evidence-bundle.pub.
# The signing key of the bundle exists only for the run.
set -Eeuo pipefail
# The failing line and command, printed last so a CI log shows it first.
trap 'failed_at="line $LINENO: $BASH_COMMAND"' ERR
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
LICENSE_FILE=${WALKTHROUGH_LICENSE_FILE:-}
TLS_SECRET=plux-api-tls
LICENSE_SECRET=plux-license

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
		--set server.publicBaseURL="https://127.0.0.1:$API_PORT" \
		--set server.tls.existingSecret="$TLS_SECRET" \
		--set license.existingSecret="$LICENSE_SECRET" \
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

for tool in docker "$KIND" "$HELM" "$KUBECTL" "$GO" curl openssl; do
	command -v "$tool" >/dev/null || fail "$tool is required"
done
docker info >/dev/null 2>&1 || fail "the Docker daemon is not running"
[ -n "$LICENSE_FILE" ] && [ -s "$LICENSE_FILE" ] ||
	fail "set WALKTHROUGH_LICENSE_FILE to a signed enterprise licence (CI: the secret PLUX_TEST_LICENSE)"

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
	"$KUBECTL" get pods,pvc,cluster.postgresql.cnpg.io -A -o wide >&2 || true
	for pod in $("$KUBECTL" --namespace "$NAMESPACE" get pods -o name 2>/dev/null); do
		echo "── logs $pod" >&2
		"$KUBECTL" --namespace "$NAMESPACE" logs "$pod" --all-containers --tail=40 >&2 || true
	done
}

cleanup() {
	status=$?
	for pid in "${pids[@]:-}"; do [ -n "$pid" ] && kill "$pid" 2>/dev/null || true; done
	if [ "$status" -ne 0 ]; then
		diagnostics
		echo "✗ failed at ${failed_at:-an unknown point} (exit $status)" >&2
	fi
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

# make_ca: a CA made for the run and a certificate for the api, valid for the
# address the port-forward serves on. Both keys stay in $work.
make_ca() {
	openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -days 1 \
		-subj "/CN=plux walkthrough CA" -keyout "$work/ca.key" -out "$work/ca.pem" 2>/dev/null
	openssl req -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes \
		-subj "/CN=plux-api" -keyout "$work/tls.key" -out "$work/tls.csr" 2>/dev/null
	printf 'subjectAltName=IP:127.0.0.1,DNS:localhost\nextendedKeyUsage=serverAuth\n' >"$work/tls.ext"
	openssl x509 -req -in "$work/tls.csr" -CA "$work/ca.pem" -CAkey "$work/ca.key" -CAcreateserial \
		-days 1 -extfile "$work/tls.ext" -out "$work/tls.crt" 2>/dev/null
}

step "build the CLI, the device fleet and the walkthrough"
(cd "$repo/backend" && "$GO" build -o "$work/plux" ./cmd/plux && "$GO" build -o "$work/plux-load" ./cmd/plux-load)
(cd "$repo/tools" && "$GO" build -o "$work/walkthrough" ./cmd/walkthrough && "$GO" build -o "$work/webauthn-driver" ./cmd/webauthn-driver)

# The two person-facing hooks default to the software authenticator. Its keys
# live in this directory (mode 0700) for the run and are deleted with it; the
# organisation key is the one `plux-server seed` gives its organisation.
mkdir -m 700 "$work/keys"
export WALKTHROUGH_DRIVER_DIR=$work/keys
export WALKTHROUGH_ORG_KEY=${WALKTHROUGH_ORG_KEY:-acme}
TOKEN_CMD=${WALKTHROUGH_TOKEN_CMD:-"$work/webauthn-driver token"}
STEPUP_CMD=${WALKTHROUGH_STEPUP_CMD:-"$work/webauthn-driver stepup"}

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

step "installation (two replicas of every role, TLS with the run's CA)"
make_ca
k create namespace "$NAMESPACE"
k --namespace "$NAMESPACE" create secret tls "$TLS_SECRET" --cert="$work/tls.crt" --key="$work/tls.key"
k --namespace "$NAMESPACE" create secret generic "$LICENSE_SECRET" --from-file=license.json="$LICENSE_FILE"
install
k --namespace "$NAMESPACE" wait --for=condition=Ready cluster/plux-pg --timeout=300s

step "seed"
# The administrator has no account at the test provider: the step-up hook
# signs them in with the seeded email and password.
export WALKTHROUGH_ADMIN_EMAIL=admin@walkthrough.example
seeded=$(k --namespace "$NAMESPACE" exec deploy/plux-worker -- /usr/local/bin/plux-server seed -config /etc/plux/plux.yaml -email "$WALKTHROUGH_ADMIN_EMAIL" -out -)
value() { printf '%s\n' "$seeded" | sed -n "s/^PLUX_DEV_$1=//p" | head -n 1; }
ORG=$(value ORGANIZATION)
APP=$(value APP)
export WALKTHROUGH_ADMIN_TOKEN WALKTHROUGH_ADMIN_PASSWORD
WALKTHROUGH_ADMIN_TOKEN=$(value TOKEN)
WALKTHROUGH_ADMIN_PASSWORD=$(value PASSWORD)
[ -n "$ORG" ] && [ -n "$APP" ] && [ -n "$WALKTHROUGH_ADMIN_TOKEN" ] && [ -n "$WALKTHROUGH_ADMIN_PASSWORD" ] ||
	fail "seed did not print an organisation, an app, a token and a password"

step "forward the api to localhost:$API_PORT"
k --namespace "$NAMESPACE" port-forward svc/plux-api "$API_PORT:8080" >/dev/null 2>&1 &
pids+=("$!")
curl -fsS --cacert "$work/ca.pem" --retry 30 --retry-all-errors --retry-delay 2 --max-time 5 "https://127.0.0.1:$API_PORT/livez" >/dev/null ||
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
	-server "https://127.0.0.1:$API_PORT" -ca-file "$work/ca.pem" -org "$ORG" -app "$APP" -fleet-env "$FLEET_ENV" -devices "$DEVICES" \
	-plux "$work/plux" -plux-load "$work/plux-load" -out "$OUT" -project "$repo/schema/testdata/documents/plux_bank" \
	-advertise "$gateway" \
	-token-cmd "$TOKEN_CMD" -stepup-cmd "$STEPUP_CMD" \
	-audit-key-cmd "$work/audit-key.sh" -upgrade-cmd "$here/walkthrough-kind.sh upgrade" \
	-with "$with" ${viewer[@]+"${viewer[@]}"} || status=$?

step "verify the evidence bundle"
"$work/walkthrough" verify -dir "$OUT"
[ "$status" -eq 0 ] || fail "the walkthrough failed (exit $status); the failure is recorded in the signed bundle"
echo "✓ walkthrough: nine steps passed; evidence in $OUT"
