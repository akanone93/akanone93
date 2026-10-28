<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# On-premises reference architecture (`DEP-050`)

This page describes a Plux installation on your own Kubernetes cluster, for
regulated environments that cannot use a managed cloud. The cloud modules in
`deploy/iac/` are the equivalent for AWS, Google Cloud and Azure. The chart
(`deploy/helm/plux`) is the same in both cases.

## Components

| Component | Runs as | Notes |
|---|---|---|
| Plux roles (api, worker, fnrunner, egress, builder, and the render worker of visual diffs when enabled) | Chart Deployments, at least two replicas each | Default-deny NetworkPolicies; Pod Security "restricted" |
| PostgreSQL | CloudNativePG `Cluster`, three instances, synchronous replication | Requires the CloudNativePG operator (`postgresql.mode: cnpg`) |
| Valkey | Chart-managed, three pods with a Sentinel each, TLS 1.3 | `valkey.mode: sentinel`; an external Valkey is also supported |
| Object storage | SeaweedFS run by the chart, or any S3 API | SeaweedFS is for development and small sites; use an external S3 for production (`objectStorage.mode: external`) |
| Signing | HashiCorp Vault Transit, highly available | `signing.backend: vault`; the `file` backend is for development only |
| Registry | Your own, reachable from the cluster | Required for air-gapped installs (`DEP-040`) |

## Sizing

Start from the chart's defaults and size from measurements, not from this page.
The [sizing guide](sizing.md) (`DEP-051`, `NFR-023`, `NFR-040`) gives roles,
replicas and storage by device count and says how to measure your own numbers
with `make load-test`.

## Networking

- Ingress: one controller in front of the api role; TLS terminates there and
  inside the cluster the roles speak mutual TLS (`SEC-043`).
- Default-deny NetworkPolicies, with explicit allows between roles. The
  cluster's network plugin must enforce them.
- Egress: the egress role is the only route to outside hosts, and only to the
  domains the installation allows. Air-gapped sites deny all egress and install
  from the signed bundle.
- Devices reach the installation through the ingress; the database, the cache
  and the signing service are never exposed.

## Backups

- PostgreSQL: continuous WAL archiving with point-in-time recovery to object
  storage, through the CloudNativePG backup configuration.
- Object storage: versioning on, and replication or a second site for the
  bundles and deltas.
- Consistency: a restore is verified with `make restore-drill`, which restores
  into a new namespace at a random point in the last hour and checks releases,
  bundle hashes and the audit chain (`DEP-010`).

The backup policy and the recovery objectives are set out in
`docs/adr/0083-kubernetes-ha-backups-and-air-gap.md`.
