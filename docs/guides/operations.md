<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Operations Guide

How to run a Plux installation you own: the Helm chart, high availability, backups and the restore
drill, air-gapped installs, infrastructure as code, sizing, observability with its service levels,
and the list of every connection the server makes. The requirements are `DEP-003`, `DEP-004`,
`DEP-010`, `DEP-040`, `DEP-050`, `DEP-051`, `NFR-023`, `NFR-040`, `OBS-004`, `OBS-005`,
`SEC-163`, and [ADR-0083](../adr/0083-kubernetes-ha-backups-and-air-gap.md) and
[ADR-0085](../adr/0085-slos-dashboards-and-alert-rules.md).

Start with the [on-premises reference architecture](../operations/on-premises.md), which shows the
whole picture. This guide points to the detail.

## 1. The chart

The Helm chart lives in `deploy/helm/plux`. It runs five deployments, one per role:

| Role | What it does | Isolation |
|---|---|---|
| `api` | Serves the API and the device endpoints. | Stateless. |
| `worker` | Runs jobs: publishing, signing, schedules, exports. Holds the signing roles. | Stateless. |
| `fnrunner` | Runs customer functions in WebAssembly. | No database, no signing key, NetworkPolicy-isolated. |
| `egress` | Makes the outbound calls the server is allowed to make. | Only the destinations on the [egress list](../operations/egress.md). |
| `builder` | Builds functions in a sandbox. | No database, no signing key, NetworkPolicy-isolated. |

The chart is hardened by default: Pod Security `restricted`, NetworkPolicies that isolate
`fnrunner` and `builder`, PodDisruptionBudgets, horizontal autoscaling, topology spread, and
checksum annotations so a configuration change rolls the pods. Secrets come from existing
Kubernetes Secrets or an external secret store. A `values.schema.json` rejects an invalid value at
install time.

Install with your values file, then run the chart's tests:

- `helm upgrade --install plux <chart> --namespace plux --create-namespace -f values.yaml`
- `helm test plux --namespace plux` runs the chart's test hooks.
- `make e2e-kind` installs the chart on a local `kind` cluster with CloudNativePG, runs `helm test`,
  performs a rolling upgrade and two database failovers. It needs Docker and `kubectl`.

## 2. High availability

The reference deployment has at least two replicas of every role, and three `api` replicas spread
across zones. The data layer is:

- **PostgreSQL** through CloudNativePG: three instances, with a synchronous replica as an option.
- **Valkey** with Sentinel, for the DPoP replay cache and the revocation of sessions.
- **Object storage**: SeaweedFS with replication, or your own S3-compatible service, which is
  recommended for production.

A **rolling upgrade** uses readiness gates and connection draining, so a deployment that moves
pods does not fail a request. Migrations run as a pre-upgrade job. Each migration is
backward-compatible for one minor release (expand, then contract), so the old pods keep working
while the new ones start. `make e2e-kind` performs a rolling upgrade to check this.

Devices keep working when the server is unreachable: they run the release they have cached, and a
CDN keeps serving bundles (`G5`).

## 3. Backups and the restore drill

- **Continuous WAL archiving** of PostgreSQL to object storage gives point-in-time recovery.
- **Object-store versioning or replication** keeps the bundles and their history.
- A **consistency marker** ties the database and the object store together, so a restore can
  check that the two match.

The targets are a **recovery point objective of 5 minutes** and a **recovery time objective of one
hour** (`DEP-010`). They are measured, not assumed: `make restore-drill` restores the installation
into a new namespace on `kind` from its backups, at a random point in the last hour. It then
verifies the manifests, the bundle hashes and the audit chain, and reports the RPO and RTO it
achieved. The drill needs Docker and `kubectl`.

We recommend running the drill on a schedule and after any change to the backup configuration. The step-by-step
recovery, and what to do when it goes wrong, is in the [backup and restore runbook](../runbooks/backup-and-restore.md).

## 4. Air-gapped installation (enterprise)

For an installation with no route to the internet, the air-gapped bundle carries everything: the
container images as one OCI layout, the packaged chart, the operations documents, the egress list,
and a signed manifest that lists every other file with its size and SHA-256. Verify the bundle
inside the gap before you install it; verification refuses a bundle with a missing, extra or changed
file. Then push the images to a local registry, point the chart at it, and keep egress denied.

The full procedure, including upgrades, is in the [air-gapped guide](../operations/air-gapped.md).
`make airgap-kind` builds a bundle and installs it on `kind` with networking denied.

## 5. Infrastructure as code

OpenTofu reference modules for AWS, Google Cloud and Azure are in `deploy/iac/aws`, `deploy/iac/gcp`
and `deploy/iac/azure`. Each creates a private Kubernetes cluster, a database (or leaves it to
CloudNativePG on the cluster), a versioned object-storage bucket, a Valkey-compatible cache, the
key management keys and the networking. Its output is a `helm_values` object whose keys match
`deploy/helm/plux/values.yaml`. The modules are references, not a product: copy one into your own
root module, pin its provider versions and back up its state before use. Read `deploy/iac/README.md` for the secure defaults each module sets.

## 6. Sizing

[The sizing guide](../operations/sizing.md) gives the requests per second and the resources per tier,
derived from what one device costs, and the point at which analytics should move to ClickHouse
(section 3 of the [analytics guide](analytics.md)). Its last section records that the full measured
run is still pending, so treat the figures as derived until you check them. `make load-test` runs a fleet of synthetic devices against the Compose stack
and prints a JSON report, so you can check the numbers on your own hardware before you trust them.

## 7. Observability and service levels

The objectives are set in [ADR-0085](../adr/0085-slos-dashboards-and-alert-rules.md):

| Objective | Target |
|---|---|
| Manifest availability | 99.95% |
| Function success | 99.9% |
| Publish job success | 99.5% |
| Control-document propagation | at most 60 s at p95 (`NFR-024`) |

Each objective is a Prometheus recording rule with burn-rate alerts over paired windows, in
`deploy/observability/rules/plux.rules.yml`. Infrastructure alerts (replication lag, backup age,
queue depth, certificate expiry, licence expiry, disk) sit beside them. Every alert links to a
runbook in [the alert runbooks](../runbooks/alerts/README.md).

Dashboards are Grafana JSON in `deploy/observability/dashboards`, provisioned by the chart and by
Compose: server overview, SLO burn, PostgreSQL, Valkey, object storage, and the analytics
dashboards. The control-propagation objective is measured by `make bench-control`, which compares
polling with silent push through a fake FCM.

Two paths carry alerts, and they are separate on purpose:

- **Infrastructure and service alerts** go through Alertmanager.
- **Product alerts** that you write about your own app's events go through the notification
  channels ([notifications guide](notifications.md)).

## 8. The egress list

By default no data leaves a self-hosted installation except the calls you configure (`SEC-163`).
Every outbound connection is built for one kind of destination, and a kind with fixed hosts refuses
any other host. The complete list, with the hosts or the addresses you enter for each kind, is
[the egress list](../operations/egress.md). It is generated from the code's registry of
destinations, and CI checks that the two agree, so a new destination cannot appear without being
listed.

Review the list before you approve an installation for a regulated environment. The
[personal data map](../operations/personal-data.md) says what is stored, and where.
