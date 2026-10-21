# ClickHouse Switch-Over Runbook

Move runtime analytics storage from the PostgreSQL store to the ClickHouse store of the enterprise
edition (`ANL-010`, `ANL-011`, [ADR-0080](../adr/0080-telemetry-and-analytics-storage.md),
[sizing](../operations/sizing.md#clickhouse-switch-over-point)). Use it when the event volume has
reached the point where PostgreSQL partitions and rollups compete with the transactional workload.

**Status of this runbook.** The switch is implemented as an opener and a store; how the events already
in PostgreSQL move to ClickHouse is not decided yet and is part of the ClickHouse milestone. Until that
decision is recorded, follow the steps below as a plan to confirm on a staging installation, and do not
run it against production data.

## Symptom

The signs that the switch is due, from [sizing](../operations/sizing.md#clickhouse-switch-over-point):

- Raw-event partitions dominate the database size.
- The worker's bulk writes fall behind: `plux_jobs_queue_depth` grows through the day (see the
  [telemetry backlog runbook](telemetry-backlog.md) first, to rule out a stuck consumer).
- Analytics queries in Studio exceed their time budget, because they read raw events.

The threshold in the sizing guide is roughly **10 million events a day or 1 million active devices**,
whichever comes first. Count your own stored events per day; do not use the device count alone.

## Impact

While the switch is planned, nothing is lost. During the switch, the store changes for new events only:
**events already in PostgreSQL stay there, and the switch starts a new history in ClickHouse**
(`ee/backend/analytics`). Until the migration decision is recorded, treat any period that spans the
switch as split between the two stores.

The control plane (apps, releases, governance, audit) stays in PostgreSQL in every case.

## Diagnosis

1. Confirm the threshold with counts from the installation, not from the estimate in the sizing
   guide.
2. Confirm the licence allows the store: `plux admin license --json` (needs `organization.manage`)
   must list the ClickHouse feature. Without it the opener returns no store and PostgreSQL stays in
   use, without an error.
3. Confirm the ClickHouse server is reachable from the server replicas. The [backup runbook](backup-and-restore.md)
   covers PostgreSQL and the object store, not ClickHouse: its backup is an open item to settle before
   production use. Each organisation's raw events are kept for its configured retention (13 months by
   default).

## Remediation

These are the steps the enterprise edition supports today; the migration of existing events is the
open item above.

1. Put the ClickHouse connection string in the server's secret store, not in a values file. It has the
   form of a DSN with a password, which is secret: it is never logged, and `PLUX_ANALYTICS_CLICKHOUSE_URL`
   names it. Store it as a secret of the environment that runs the enterprise server.
2. Set `PLUX_ANALYTICS_CLICKHOUSE_URL` on the enterprise server (`plux-server-ee`) and roll its replicas,
   one at a time, as in the [server upgrade notes](server.md#upgrades).
3. Check that new events land in ClickHouse: write a test event for a test organisation and query it
   through Studio's analytics views, or through `AnalyticsService` (`QueryRollups`).
4. Watch the worker's bulk writes and the query times for a day. The rollups at five minutes, one hour
   and one day are written by the same algorithm as in PostgreSQL, so the series should match.
5. Leave the events already in PostgreSQL where they are; their retention still applies to them. Any
   move of those events waits for the migration decision.

**Rollback.** Unset `PLUX_ANALYTICS_CLICKHOUSE_URL` and roll the replicas. New events return to
PostgreSQL. Events already written to ClickHouse stay there under their retention.

## Escalation

- Licence or feature not listed: the licensing contact (see the [licence expiry runbook](licence-expiry.md)).
- Events missing or split across the stores after the switch: the analytics owner, with the window and
  the organisation.
- The open migration decision: the maintainers of the ClickHouse milestone, who own the answer.
