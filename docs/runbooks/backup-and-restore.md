# Backup and Restore Runbook

How PostgreSQL and the object store are backed up on Kubernetes, how to restore them, and how
the drill proves it (`DEP-010`, [ADR-0083](../adr/0083-kubernetes-ha-backups-and-air-gap.md)).
Targets: RPO 5 minutes, RTO 1 hour.

## What is backed up

| Data | How | Where |
|---|---|---|
| PostgreSQL | CloudNativePG archives every WAL segment (at the latest every `postgresql.cnpg.backup.archiveTimeout`, 60 s) and takes a base backup on `schedule` (every 6 hours); Barman Cloud uploads both | The backup store: `postgresql.cnpg.backup.*` |
| Bundles, deltas, assets | Content-addressed, immutable objects; bucket **versioning** keeps a deleted or overwritten object recoverable | The data bucket |
| Signing keys | Vault Transit (or the KMS) owns them; the `file` backend (development only) keeps them on a volume | Not in these backups: back them up with the key ceremony ([Key ceremony](key-ceremony.md)) |
| Secrets and values | Your Secret store and `helm get values` | Yours |

Keep the backup store apart from the cluster and from the data bucket (another account, region
or provider). The chart can use its own SeaweedFS for a trial; that store dies with the cluster.

## Object store versioning and replication

Turn versioning on for the data bucket and the backup bucket before the first publish; the drill
checks it.

- **External S3 (recommended).** `aws s3api put-bucket-versioning --bucket <bucket> --versioning-configuration Status=Enabled`;
  add a lifecycle rule that expires non-current versions after the retention (30 days) and
  cross-region or cross-account replication for the backup bucket. Equivalent on GCS (object
  versioning, dual-region buckets) and Azure Blob (versioning, object replication).
- **SeaweedFS.** One instance, no replication: it is for trials. Enable versioning through its S3
  API (`PutBucketVersioning`, as the drill does with a signed request) and copy the volume with
  your storage layer (CSI snapshots, a replicated storage class). For production use external S3.

## Consistency marker

PostgreSQL and the object store are two systems, so "restore to 10:42" needs a shared clock. Every
minute the `<release>-backup-marker` CronJob:

1. emits a WAL message `plux.backup.marker` in PostgreSQL (`pg_logical_emit_message`) and records its
   WAL position (LSN);
2. writes `markers/postgresql/<UTC time>.json` into the (versioned) data bucket:
   `{"id","time","lsn","walFile","cluster"}`.

The same instant is therefore in the WAL archive and in the object store. Restoring PostgreSQL to a
marker's `lsn` (`postgresql.cnpg.restore.targetLSN`) gives a database whose rows name only objects
that existed at the marker's time: objects are content-addressed and written before the release row
that names them commits, and versioning keeps them. If you also need the bucket as it stood, list
the object versions up to the marker's `time`. Alert: `PluxConsistencyMarkerStale`.

## Restore

1. Pick the point: the newest marker you trust, or a marker before the incident
   (`aws s3 ls s3://<data bucket>/markers/postgresql/`). For a time without a marker use
   `restore.targetTime` (RFC 3339, UTC).
2. Install the chart in a new namespace (or a new cluster name) with `postgresql.cnpg.restore.enabled=true`,
   `restore.sourceServerName=<the old cluster name>`, `restore.targetLSN=<lsn>`, the same
   `postgresql.cnpg.backup.*` store, and the **same signing keys and secrets**. Leave
   `backup.enabled=false` while you verify: a new cluster must not archive into the folder it
   restores from. Once satisfied, set `backup.enabled=true` with a new `backup.serverName`.
3. Wait for `kubectl wait --for=condition=Ready cluster/<name>`, then verify:
   `plux-server audit verify` (exit 0), `plux pull` against the new server, `/readyz`.
4. Point DNS or the ingress at the new installation.

`make restore-drill` does exactly this on a `kind` cluster at a random marker of the last hour and
fails if anything differs: every restored release and audit row must be the source's row and every
older source row must be present; every bundle the database names must be in the object store at its recorded size, and `plux pull`
checks the promoted bundles against their hashes; the audit chain must verify. It prints RPO (time for the newest write to reach the archive; limit 300 s) and
RTO (from creating the namespace to a verified server; limit 3600 s).

## When it goes wrong

| Symptom | Cause and action |
|---|---|
| `PluxWalArchiveStalled` | The archive stalled; see the [alert runbook](alerts/PluxWalArchiveStalled.md). Until it clears the RPO is not met. |
| Restore pod "expected empty archive" | The new cluster archives into the folder it restores from: set a new `backup.serverName` or disable backups for the restore. |
| Restore stops with "recovery target LSN not reached" | The WAL up to the marker is not in the archive: choose an older marker. |
| `audit verify` fails after a restore | The audit key differs from the source (wrong Vault key or key volume), or the restore point split a transaction: check the key first. |
| A bundle is missing, short or fails `plux pull`'s hash check | The object store lost or changed the object: restore the bucket version; never re-upload from another build (determinism makes this possible only for the same source). |
