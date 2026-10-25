# Plux Helm chart

Installs Plux Server in high availability on Kubernetes (`DEP-003`, `DEP-004`; ADR-0083):
the `api` and `worker` roles, and with `functions.enabled` the `fnrunner`, `egress` and
`builder` roles, with PostgreSQL through CloudNativePG, Valkey with Sentinel and SeaweedFS,
or external services for each.

## Install

```sh
helm install plux deploy/helm/plux --namespace plux --create-namespace \
  --set server.publicBaseURL=https://plux.example.com \
  --set signing.vault.address=https://vault.example.com
```

`server.publicBaseURL` and the signing backend's settings are required; the chart refuses
to render without them. `values.schema.json` is strict: a misspelt key or a value of the
wrong kind fails `helm install`.

## Topology

| Value | Modes | Default |
|---|---|---|
| `postgresql.mode` | `cnpg` (a CloudNativePG `Cluster`; install the operator first) or `external` | `cnpg` |
| `valkey.mode` | `sentinel` or `external` | `sentinel` |
| `objectStorage.mode` | `seaweedfs` or `external` (S3; recommended for production) | `seaweedfs` |
| `signing.backend` | `vault` (Transit) or `file` | `vault` |
| `internalTLS.mode` | `generate` or `existingSecrets` | `generate` |
| `license.existingSecret` | a Secret holding the enterprise licence file (key `license.key`, default `license.json`), mounted read-only into the api and worker roles; `license.file` is set to it | none |

Every role runs at least two replicas with PodDisruptionBudgets, horizontal autoscaling and
topology spread (`scheduling.zoneSpread`, `scheduling.nodeSpread`). Pods run under Pod
Security `restricted`; NetworkPolicies deny by default, and `fnrunner` and `builder` reach
only `egress` and hold no database credential or signing capability (L-4). Credentials come
from existing Secrets, never from inline values.

## Studio state secret

`PLUX_BFF_STATE_SECRET` is required: base64 or base64url of at least 32 random bytes (`openssl rand -base64 32`, 43 characters or more). Every Studio replica must share it, because a sign-in callback can reach a different replica than the one that began it; the BFF refuses to start without it and never logs it. The chart generates one Secret (`<release>-plux-studio-state`, key `stateSecret`) on first install and keeps it across upgrades; set `studio.existingSecret` to use your own Secret with the same key. Compose generates `.secrets/studio.env` in `init-secrets.sh`; `make studio-dev` uses a fresh random value per run.

## Upgrade

`helm upgrade` runs the database migration as a pre-upgrade Job, then rolls each role.
Migrations are expand/contract, so the previous release keeps working during the roll.

## Tests

- `make helm-lint` lints the chart, renders it for every `ci/*-values.yaml` (default,
  external, HA, kind), validates the output against the Kubernetes and CloudNativePG schemas
  and checks Pod Security, L-4 and the values schema. Runs in CI on chart changes.
- `make e2e-kind` installs the chart on a kind cluster with CloudNativePG, runs `helm test`,
  a rolling upgrade and Valkey and PostgreSQL failovers (`deploy/helm/e2e-kind.sh`). Needs
  Docker; runs daily, manually and on pull requests that change the chart or the image.
- Tools: `make install-helm-tools` (pinned Helm, kind, kubeconform).

## Backup and restore

`postgresql.cnpg.backup.enabled` turns on continuous backup of the CloudNativePG cluster
(`DEP-010`): every WAL segment goes to object storage as it fills or after `archiveTimeout`
(60 s by default; the schema refuses more than five minutes, the recovery point objective), a
`ScheduledBackup` takes base backups, and `retention` expires both. Credentials come from
existing Secrets (`backup.existingSecret`, else the object storage's); with none the pod's ambient
credential (IRSA) is used. Keep the store apart from the cluster and the data bucket: `backup.bucket`,
`backup.endpoint` and `backup.region` name another store. The bundled SeaweedFS is accepted for
trials only.

**Consistency marker.** With `backup.marker.enabled` a CronJob writes, every minute, a WAL message
in PostgreSQL and an object `markers/postgresql/<time>.json` in the data bucket that name the same
instant (the WAL position, the time, the cluster). Restore PostgreSQL to a marker's position and the
rows name only objects that existed then; enable **versioning** on both buckets (external S3:
`put-bucket-versioning` plus a lifecycle rule and replication; SeaweedFS: its S3 API, and a volume
snapshot, since it has no replication here). Details and the restore steps:
[Backup and restore runbook](../../../docs/runbooks/backup-and-restore.md).

**Restore.** `postgresql.cnpg.restore.enabled` creates the cluster from the store instead of empty:
`sourceServerName` (the old cluster's folder), and `targetLSN` (a marker) or `targetTime`. Turn
`backup.enabled` off for the restored cluster, or give it a new `backup.serverName`; the chart refuses
to archive into the folder it restores from. `ci/restore-values.yaml` is the drill's second
installation.

`make restore-drill` runs the whole cycle on `kind` (nightly in CI): it seeds data, takes a base backup,
publishes for a few minutes, restores into a new namespace at a random marker from the last hour,
checks releases, bundles, the manifest path (`plux pull`) and the audit chain (`plux-server audit
verify`), and prints RPO (limit 5 min) and RTO (limit 1 h). It exits non-zero on any failure.
`DRILL_KEEP=1` keeps the cluster, `DRILL_SEED` repeats a random point, `DRILL_LOAD_SECONDS` sets the
workload length.

## Alerts and dashboards

`observability.prometheusRule.enabled` installs a `PrometheusRule` (service level objectives with
burn-rate alerts, and the infrastructure and security alerts) and `observability.dashboards.enabled` a
ConfigMap of Grafana dashboards for the sidecar (server overview per role, PostgreSQL, Valkey, object
storage, SLO burn). Both come from `deploy/observability`, which the Compose stack mounts directly;
`files/observability` holds the chart's copy (`deploy/observability/sync.sh`; `make helm-lint` fails
when they differ). Scrape every role with a job name starting `plux`; the rules also read series from
CloudNativePG, `redis_exporter`, kube-state-metrics, the kubelet and cert-manager, listed in
`deploy/observability/external-metrics.txt`. Every alert links to
[its runbook](../../../docs/runbooks/alerts/README.md).
