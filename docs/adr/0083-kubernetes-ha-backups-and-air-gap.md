# 0083. Kubernetes: the Helm chart, HA components, backups, air-gapped bundles and IaC modules

- **Status:** Accepted
- **Date:** 2026-10-18
- **Requirements:** `DEP-003`, `DEP-004`, `DEP-010`, `DEP-040`, `DEP-050`, `DEP-051`, `NFR-023`, `NFR-040`, `SEC-120`

## Context and problem

Until now the supported install is the Compose stack. A regulated organisation runs Kubernetes,
expects no downtime on upgrade, a restore it can rehearse, and, for some, an install with no
internet at all. The phase exit criterion is a self-hosted HA install that passes an audit
walkthrough on a `kind` cluster. The questions: how is the server packaged, which components
provide HA for each stateful dependency, how do backups work, and how are cloud and air-gapped
installs supported without cloud accounts in CI?

## Decision drivers

- The role isolation of ADR-0006 and L-4 holds in the cluster: `fnrunner` and `builder` have no
  database and no signing capability.
- Rolling upgrades with no failed request (`DEP-004`).
- RPO of 5 minutes and RTO of 1 hour (`DEP-010`).
- Everything verifiable in CI without a cloud account.
- Pinned tools and recorded dependencies (`dependencies.md`).

## Considered options

1. A Helm chart with CloudNativePG, Valkey with Sentinel and replicated SeaweedFS or external S3; OpenTofu modules per cloud.
2. Plain manifests with Kustomize overlays, and operator-less PostgreSQL via a StatefulSet.
3. A Kubernetes operator written for Plux.

## Decision

Chosen option: **1** (maintainer decisions Q19, Q20, Q21; approvals A2 to A4).

- **Chart (`DEP-003`).** `deploy/helm/plux` with `api`, `worker`, `fnrunner`, `egress` and
  `builder` deployments; a values schema (`values.schema.json`); NetworkPolicies isolating
  `fnrunner` and `builder`; Pod Security `restricted`; PodDisruptionBudgets; HPA; topology
  spread; secrets from existing Secrets or external secret stores; checksum annotations for
  config rollouts; `helm test` hooks.
- **HA (`DEP-004`).** At least 2 replicas per role. CloudNativePG cluster (3 instances,
  synchronous replica option); Valkey with Sentinel; SeaweedFS with replication, or external S3
  (recommended for production). Rolling upgrades use readiness gates and connection draining.
  Migrations run as a pre-upgrade Job that is backward compatible for one minor release
  (expand/contract), tested by upgrading mid-traffic.
- **Backups (`DEP-010`).** CloudNativePG continuous WAL archiving to object storage (PITR),
  object-storage versioning or replication, and a consistency marker tying both. `make
  restore-drill` restores to a new namespace at a random point within the last hour and verifies
  manifests, bundle hashes and the audit chain.
- **Air-gapped (`DEP-040`, enterprise).** `plux airgap bundle` produces a signed archive of images
  (OCI layout), the chart, docs and the egress list; `plux airgap verify` checks it; CI installs
  from the bundle on `kind` with networking denied.
- **IaC (`DEP-050`, low priority, last in E9).** OpenTofu modules `deploy/iac/{aws,gcp,azure}`
  (managed Kubernetes, managed PostgreSQL or CloudNativePG, object storage, Valkey, KMS for
  signing keys where available, networking) and an on-premises reference architecture.
  Verified without cloud accounts by `tofu validate`, `tofu test` with `mock_provider`, and a
  policy check (no public buckets, encryption on, private networking).
- **Sizing (`DEP-051`, `NFR-023`, `NFR-040`).** A load generator (`test/load`) simulates devices.
  CI runs a scaled test with a documented extrapolation; one full run on maintainer hardware
  fills the top rows of the guide.
- **CI budget.** Heavy jobs (`kind`, drill, walkthrough, load) run nightly and on pull requests
  whose paths affect them (ADR-0043 rules).

## Consequences

- **Positive:** a rehearsed HA install; operators of known operators (CNPG) instead of
  home-grown failover; the whole chain testable on `kind`.
- **Negative:** chart dependencies to track (CloudNativePG, Valkey, SeaweedFS images); new pinned
  tools (Helm, `kind`, `kubeconform`, `chart-testing`, OpenTofu); mock-provider IaC tests prove
  the plan, not the cloud's behaviour.
- **Follow-up:** `dependencies.md` rows; `mk/` targets; restore, upgrade-rollback and air-gapped
  upgrade runbooks; the sizing guide.

## Options in detail

### Option 1 — Helm with established operators

Helm is the common packaging for self-hosters; CNPG and Sentinel give tested failover.

### Option 2 — Kustomize and hand-built StatefulSets

Fewer tools, but failover, WAL archiving and PITR would be our code to get right. Rejected.

### Option 3 — a Plux operator

Large engineering and security surface for a problem existing operators solve. Rejected.
