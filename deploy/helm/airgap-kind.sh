#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: Apache-2.0

# `make airgap-kind` (DEP-040): installs Plux from an air-gapped bundle on a
# throwaway kind cluster whose outbound networking is denied, following
# docs/operations/air-gapped.md:
#
#   1. builds plux-airgap, signs a test licence and a bundle key, and builds a
#      bundle of the images the chart renders (plus the PostgreSQL image the
#      CloudNativePG operator runs);
#   2. verifies the bundle and unpacks it;
#   3. creates the cluster with a local registry that its containerd uses as
#      the mirror for kind-registry:5000, and pushes the bundle's images;
#   4. denies outbound traffic: every node drops packets that leave the kind
#      network (iptables), and the chart's internet egress rule is off;
#   5. checks that a pod cannot reach the internet, installs the CloudNativePG
#      operator (its image mirrored into the registry, as an operator would
#      for a component outside the bundle) and the chart from the bundle's
#      chart/plux.tgz, and runs `helm test`.
#
# Needs Docker, kind, helm, kubectl, skopeo, curl and Go. The cluster and the
# registry are deleted at the end; E2E_KEEP=1 keeps them.
set -Eeuo pipefail
# The failing line and command, printed last so a CI log shows it first.
trap 'failed_at="line $LINENO: $BASH_COMMAND"' ERR

HELM=${HELM:-helm}
KIND=${KIND:-kind}
KUBECTL=${KUBECTL:-kubectl}
GO=${GO:-go}
CLUSTER=${E2E_CLUSTER:-plux-airgap}
NAMESPACE=plux
IMAGE=${E2E_IMAGE:-plux-server:e2e}
CNPG_VERSION=${CNPG_VERSION:?set CNPG_VERSION}
CNPG_SHA256=${CNPG_SHA256:?set CNPG_SHA256}
# The PostgreSQL image of the Cluster; the chart's backup marker uses the same.
PG_IMAGE=${PG_IMAGE:-ghcr.io/cloudnative-pg/postgresql:18-standard-trixie}
REGISTRY_IMAGE=${REGISTRY_IMAGE:-registry:2}
REGISTRY_NAME=kind-registry
REGISTRY_PORT=5001
MIRROR=$REGISTRY_NAME:5000
VERSION=e2e

here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../.." && pwd)
chart=$here/plux
work=$(mktemp -d)

fail() { echo "✗ $*" >&2; exit 1; }

for tool in docker "$KIND" "$HELM" "$KUBECTL" "$GO" skopeo curl sha256sum; do
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
	"$KUBECTL" get pods,pvc,cluster.postgresql.cnpg.io -A -o wide >&2 || true
	"$KUBECTL" --namespace "$NAMESPACE" describe pods >&2 || true
}

cleanup() {
	status=$?
	if [ "$status" -ne 0 ]; then
		diagnostics
		echo "✗ failed at ${failed_at:-an unknown point} (exit $status)" >&2
	fi
	if [ -z "${E2E_KEEP:-}" ]; then
		"$KIND" delete cluster --name "$CLUSTER" >/dev/null 2>&1 || true
		docker rm -f "$REGISTRY_NAME" >/dev/null 2>&1 || true
	fi
	rm -rf "$work"
	exit "$status"
}
trap cleanup EXIT

# Every helm and kubectl call talks to the kind cluster through a kubeconfig of
# its own, never to whatever cluster the caller's environment points at.
export KUBECONFIG=$work/kubeconfig

# registry_path maps an image reference to its path in the local registry:
# the registry host, if the reference names one, is dropped.
registry_path() {
	local ref=$1
	if [[ $ref == */* ]]; then
		case ${ref%%/*} in *.* | *:* | localhost) ref=${ref#*/} ;; esac
	fi
	echo "$ref"
}

echo "── tools, keys and licence"
(cd "$repo/ee/backend" && "$GO" build -o "$work/plux-airgap" ./cmd/plux-airgap && "$GO" build -o "$work/plux-license" ./cmd/plux-license)
"$work/plux-airgap" keygen --out "$work/bundle.key" >/dev/null
licence_public=$("$work/plux-license" keygen --out "$work/licence.key" | sed -n 's/^public key: *//p')
cat >"$work/payload.json" <<'EOF'
{
  "licenseId": "LIC-AIRGAP-KIND", "organization": "Plux CI", "edition": "enterprise",
  "issuedAt": "2026-01-01T00:00:00Z", "notBefore": "2026-01-01T00:00:00Z", "expiresAt": "2099-01-01T00:00:00Z",
  "features": ["air-gap"], "limits": {}
}
EOF
"$work/plux-license" sign "$work/payload.json" --key "$work/licence.key" --out "$work/licence.json" >/dev/null

echo "── cluster $CLUSTER with a local registry"
"$KIND" delete cluster --name "$CLUSTER" >/dev/null 2>&1 || true
docker rm -f "$REGISTRY_NAME" >/dev/null 2>&1 || true
cat >"$work/kind.yaml" <<'EOF'
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
containerdConfigPatches:
  - |-
    [plugins."io.containerd.grpc.v1.cri".registry]
      config_path = "/etc/containerd/certs.d"
EOF
docker run -d --restart=always -p "127.0.0.1:$REGISTRY_PORT:5000" --name "$REGISTRY_NAME" "$REGISTRY_IMAGE" >/dev/null
"$KIND" create cluster --name "$CLUSTER" --config "$work/kind.yaml" --kubeconfig "$KUBECONFIG" --wait 180s
"$KUBECTL" version >/dev/null 2>&1 || fail "the kind cluster $CLUSTER is not reachable with the kubeconfig $KUBECONFIG"
kube_version=$("$KUBECTL" version -o json | sed -n 's/.*"gitVersion": *"\(v[^"]*\)".*/\1/p' | tail -n 1)
[ -n "$kube_version" ] || fail "cannot read the Kubernetes version of the cluster $CLUSTER"
docker network connect kind "$REGISTRY_NAME" 2>/dev/null || true
nodes=$("$KIND" get nodes --name "$CLUSTER")
for node in $nodes; do
	docker exec "$node" mkdir -p "/etc/containerd/certs.d/$MIRROR"
	docker exec -i "$node" sh -c "cat >/etc/containerd/certs.d/$MIRROR/hosts.toml" <<EOF
[host."http://$MIRROR"]
EOF
done

echo "── images"
set_images=(--kube-version "$kube_version" --set image.repository="${IMAGE%:*}" --set image.tag="${IMAGE##*:}")
refs=$("$HELM" template plux "$chart" -f "$chart/ci/kind-values.yaml" "${set_images[@]}" |
	sed -n 's/^ *image: *"\{0,1\}\([^" ]*\)"\{0,1\} *$/\1/p' | sed 's/@sha256:.*//' | sort -u)
refs=$(printf '%s\n%s\n' "$refs" "$PG_IMAGE" | sort -u)
archives=()
n=0
for ref in $refs; do
	n=$((n + 1))
	archive=$work/image-$n.tar
	if [ "$ref" = "$IMAGE" ]; then
		skopeo copy "docker-daemon:$ref" "oci-archive:$archive:$ref"
	else
		skopeo copy --all "docker://$ref" "oci-archive:$archive:$ref"
	fi
	archives+=(--images "$archive")
done

echo "── bundle"
"$work/plux-airgap" bundle --version "$VERSION" "${archives[@]}" --chart "$chart" --docs "$repo/docs/operations" \
	--key "$work/bundle.key" --licence "$work/licence.json" --public-key "$licence_public" --out "$work/bundle.tar"
"$work/plux-airgap" verify "$work/bundle.tar" --key "$work/bundle.key.pub" | tee "$work/verified.txt"
mkdir "$work/bundle"
tar -xf "$work/bundle.tar" -C "$work/bundle"

echo "── push the bundle's images"
while read -r kind_word ref digest; do
	[ "$kind_word" = image ] || continue
	skopeo copy --all --dest-tls-verify=false \
		"oci:$work/bundle/images:$ref" "docker://localhost:$REGISTRY_PORT/$(registry_path "$ref")"
	echo "pushed $ref ($digest)"
done <"$work/verified.txt"

echo "── mirror the CloudNativePG operator (outside the bundle)"
operator=ghcr.io/cloudnative-pg/cloudnative-pg:$CNPG_VERSION
skopeo copy --all --dest-tls-verify=false "docker://$operator" "docker://localhost:$REGISTRY_PORT/cloudnative-pg/cloudnative-pg:$CNPG_VERSION"
manifest=$work/cnpg.yaml
curl -fsSL --retry 5 -o "$manifest" \
	"https://github.com/cloudnative-pg/cloudnative-pg/releases/download/v$CNPG_VERSION/cnpg-$CNPG_VERSION.yaml"
echo "$CNPG_SHA256  $manifest" | sha256sum -c - >/dev/null || fail "cnpg-$CNPG_VERSION.yaml does not match its pinned SHA-256"
sed -i "s|ghcr.io/cloudnative-pg/cloudnative-pg|$MIRROR/cloudnative-pg/cloudnative-pg|g" "$manifest"

echo "── deny outbound traffic"
# Packets that leave a node's network interface for an address outside the
# kind network, the pod network or the service network are dropped, for the
# node's own traffic (containerd pulling an image) and for forwarded pod traffic.
# The kind network is dual-stack on current Docker: the IPv4 subnet goes to
# iptables, the IPv6 one to ip6tables, so neither family leaves the cluster.
# Name lookups are allowed too: kind points a node at the network's gateway,
# which Docker rewrites to its embedded DNS at 127.0.0.11 (still on eth0 for
# the filter), and only that resolver answers for kind-registry.
subnets=$(docker network inspect kind -f '{{range .IPAM.Config}}{{.Subnet}} {{end}}')
subnet4= subnet6=
for s in $subnets; do
	case $s in
	*:*) subnet6=$s ;;
	*) subnet4=$s ;;
	esac
done
[ -n "$subnet4" ] || fail "the kind network has no IPv4 subnet ($subnets)"
for node in $nodes; do
	for chain in OUTPUT FORWARD; do
		# Inserted at the top, ahead of kind's own rules: the DROP first, then
		# the ACCEPTs above it.
		docker exec "$node" iptables -I "$chain" -o eth0 -j DROP
		for allowed in "$subnet4" 10.244.0.0/16 10.96.0.0/12 127.0.0.11/32; do
			docker exec "$node" iptables -I "$chain" -o eth0 -d "$allowed" -j ACCEPT
		done
		docker exec "$node" ip6tables -I "$chain" -o eth0 -j DROP
		if [ -n "$subnet6" ]; then
			docker exec "$node" ip6tables -I "$chain" -o eth0 -d "$subnet6" -j ACCEPT
		fi
	done
done
"$KUBECTL" create namespace "$NAMESPACE"
"$KUBECTL" apply --server-side -f "$manifest" >/dev/null
"$KUBECTL" --namespace cnpg-system rollout status deployment/cnpg-controller-manager --timeout=300s

echo "── a pod cannot reach the internet"
"$KUBECTL" --namespace "$NAMESPACE" run egress-probe --restart=Never \
	--image "$MIRROR/curlimages/curl:8.23.0" --command -- curl -sS --max-time 10 https://example.com
"$KUBECTL" --namespace "$NAMESPACE" wait --for=jsonpath='{.status.phase}'=Failed pod/egress-probe --timeout=120s ||
	fail "the probe pod reached the internet, or did not run"
"$KUBECTL" --namespace "$NAMESPACE" delete pod egress-probe >/dev/null

echo "── install from the bundle"
"$HELM" upgrade --install plux "$work/bundle/chart/plux.tgz" --namespace "$NAMESPACE" \
	-f "$chart/ci/kind-values.yaml" \
	--set image.repository="$MIRROR/$(registry_path "${IMAGE%:*}")" --set image.tag="${IMAGE##*:}" --set image.pullPolicy=IfNotPresent \
	--set postgresql.cnpg.imageName="$MIRROR/$(registry_path "$PG_IMAGE")" \
	--set postgresql.cnpg.backup.marker.image.repository="$MIRROR/$(registry_path "${PG_IMAGE%:*}")" \
	--set valkey.sentinel.image.repository="$MIRROR/valkey/valkey" \
	--set objectStorage.seaweedfs.image.repository="$MIRROR/chrislusf/seaweedfs" \
	--set test.image.repository="$MIRROR/curlimages/curl" \
	--set networkPolicy.externalHTTPS.enabled=false \
	--wait --timeout 15m
"$KUBECTL" --namespace "$NAMESPACE" wait --for=condition=Ready cluster/plux-pg --timeout=300s

echo "── helm test"
"$HELM" test plux --namespace "$NAMESPACE" --logs --timeout 5m

echo "✓ air-gapped install: verified bundle, images from the local registry, outbound traffic denied, helm test passed"
