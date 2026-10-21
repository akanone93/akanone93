<!-- SPDX-FileCopyrightText: 2026 Plux contributors -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

# Air-gapped installation

This guide installs Plux on a Kubernetes cluster that has no route to the internet (`DEP-040`, [ADR-0083](../adr/0083-kubernetes-ha-backups-and-air-gap.md)). It needs the enterprise edition to build the bundle; verifying and installing one needs no licence.

The idea is one signed archive, carried across the gap on removable media, and a registry inside the gap that the cluster pulls from. Nothing in the installation reaches out unless you enable it, and the cluster's network policy keeps it that way.

## What is in the bundle

`plux-airgap-<version>.tar` holds:

| Path | Content |
|---|---|
| `manifest.json` | Every other file with its size and SHA-256, the image list, and an Ed25519 signature. |
| `images/` | All container images as one OCI image layout (`oci-layout`, `index.json`, `blobs/sha256/…`); a layer shared by two images is stored once. |
| `chart/plux.tgz` | The Helm chart. |
| `docs/operations/` | This guide, the [egress list](egress.md) and the other operations documents. |

The bundle does not contain the CloudNativePG operator or the Kubernetes distribution. Mirror those the way you mirror the rest of your platform; the chart's PostgreSQL, Valkey and SeaweedFS images are in the bundle.

## 1. Verify the bundle first

On a machine inside the gap, with the bundle signer's public key (`bundle.key.pub`, handed over by a different route than the bundle):

```sh
plux-airgap verify plux-airgap-0.0.1.tar --key bundle.key.pub
```

The command checks the signature, then every file: it refuses a file that is altered, missing or not listed, and anything in the archive that is not a regular file. It exits non-zero on any failure and prints the images on success:

```text
verified: Plux 0.0.1, 63 files
image ghcr.io/nightcode42/plux-server:0.0.1 sha256:…
image valkey/valkey:8.1.9-trixie sha256:…
```

Do not unpack or load a bundle that fails. The signing key is the operator's own; create it with `plux-airgap keygen --out bundle.key` and keep the private half offline.

## 2. Push the images into a local registry

Unpack the archive; `images/` is an OCI layout. The tools below are examples, not requirements; anything that can read an OCI layout works.

```sh
mkdir bundle && tar -xf plux-airgap-0.0.1.tar -C bundle
REGISTRY=registry.internal.example:5000

# One image, by the name the layout gives it, keeping its digest (skopeo):
skopeo copy --all --dest-tls-verify=true \
  oci:bundle/images:ghcr.io/nightcode42/plux-server:0.0.1 \
  docker://$REGISTRY/nightcode42/plux-server:0.0.1

# Or with oras:
oras cp --from-oci-layout bundle/images:ghcr.io/nightcode42/plux-server:0.0.1 \
  $REGISTRY/nightcode42/plux-server:0.0.1
```

Copy every image of the list that `verify` printed. Keep each repository path and tag: the chart pins several images by digest, and a registry preserves a digest only when the manifest is copied unchanged (`--all` copies every platform of a multi-platform image).

## 3. Point the chart at the registry

Install from the packaged chart with the images redirected, and with a pull secret if the registry needs one:

```yaml
# airgap-values.yaml
image:
  repository: registry.internal.example:5000/nightcode42/plux-server
builderImage:
  repository: registry.internal.example:5000/nightcode42/plux-builder
imagePullSecrets:
  - name: internal-registry
networkPolicy:
  externalHTTPS:
    enabled: false
postgresql:
  cnpg:
    # The PostgreSQL image the CloudNativePG operator runs; empty would make it
    # pull its built-in default from the internet.
    imageName: registry.internal.example:5000/cloudnative-pg/postgresql:18-standard-trixie
    backup:
      marker:
        image:
          repository: registry.internal.example:5000/cloudnative-pg/postgresql
valkey:
  sentinel:
    image:
      repository: registry.internal.example:5000/valkey/valkey
objectStorage:
  seaweedfs:
    image:
      repository: registry.internal.example:5000/chrislusf/seaweedfs
test:
  image:
    repository: registry.internal.example:5000/curlimages/curl
```

```sh
helm upgrade --install plux bundle/chart/plux.tgz --namespace plux --create-namespace -f airgap-values.yaml
```

The values paths above are those of the chart's `values.yaml`; if a release moves one, `helm template` shows every `image:` it renders, and that list is the set to mirror.

## 4. Keep egress denied

By default no data leaves the installation except calls you configure (`SEC-163`). The [egress list](egress.md) names every kind of outbound connection and the setting that enables it. In an air-gapped cluster:

- leave push, notification channels, webhooks, analytics and audit export unconfigured, or point them at endpoints inside the gap;
- for Android attestation, set `attestation.android.statusListFile` to a copy of Google's attestation status list that you carry across the gap, so the worker never fetches it (`SEC-002`);
- keep the chart's `networkPolicy` on and turn off its one internet rule with `networkPolicy.externalHTTPS.enabled: false` (it lets the api and worker role reach public HTTPS for attestation providers, OpenID Connect and Vault); then add a `networkPolicy.extraEgress` rule only for an endpoint inside your network;
- use a local model through Ollama for AI features instead of a hosted provider.

`make airgap-kind` in the repository installs a bundle on `kind` with the chart's internet egress rule off and a deny-all-egress network policy around the Plux namespace and checks that it still works.

## 5. Upgrade

Build or obtain the newer bundle, run step 1 on it, push its images (step 2), and run `helm upgrade` with the new chart (step 3). The chart's pre-upgrade hook migrates the database. Keep the previous bundle until the upgrade has passed `helm test plux --namespace plux`; a rollback is `helm rollback` with the images of the previous bundle still in the registry.
