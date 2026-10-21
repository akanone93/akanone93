# Telemetry Backlog Runbook

Runtime telemetry is accepted quickly and written to analytics storage later. A backlog means
accepted batches wait in the queue for the workers that write them, so dashboards, rollups and
health gates see old data (`ANL-010`, `ANL-011`, [ADR-0080](../adr/0080-telemetry-and-analytics-storage.md)).
Use it when the analytics views lag, when the telemetry stream in Valkey grows, or when the queue
depth alert fires for telemetry work.

## Symptom

- Analytics and the rollup-based views show data that stops advancing while devices keep reporting.
- `plux_jobs_queue_depth` grows through the day, and `PluxJobQueueBacklog` fires.
- Valkey memory rises: `PluxValkeyMemoryHigh` fires.
- Telemetry quotas (`telemetry.eventsPerDevicePerHour`, `telemetry.batchBytes`) are checked on
  ingestion; a backlog in the queue is not, by itself, a quota event.

## Impact

Telemetry is delayed, not lost, while the queue is healthy. Two things turn a delay into loss:

- The stream can be bounded (`MaxLen`). Trimming drops batches no worker has written yet, and it is
  the last resort against a Valkey that is full.
- A batch the workers never acknowledge is claimed again after `ClaimAfter`, so a stuck consumer keeps
  its batches pending but does not lose them, as long as the stream is not trimmed.

Telemetry carries no payloads and no personal data beyond the device identifier it is recorded with
(see the [personal-data map](../operations/personal-data.md)). Erasure of a device's events is done
through the data subject request, not through this queue.

## Diagnosis

The telemetry stream is `plux:telemetry:events` and its consumer group is `telemetry-writers`
(`internal/telemetry`). With the Valkey CLI on the installation's Valkey:

1. `XLEN plux:telemetry:events`: the number of batches waiting. A steady or falling number is a
   healthy queue; a number that only rises is a backlog.
2. `XINFO GROUPS plux:telemetry:events`: the group's pending count and last delivered ID. Growing
   pending with no progress means the consumers are not acknowledging.
3. `XINFO CONSUMERS plux:telemetry:events telemetry-writers`: which worker consumers exist and how
   long since each last read. No consumer means no worker is running the writer.
4. Check the workers' logs and the job queue alert. A worker that cannot reach PostgreSQL acknowledges
   nothing; see the [server runbook](server.md) for the dependencies.
5. Check Valkey memory (`PluxValkeyMemoryHigh`) and the replay-cache runbook if Valkey itself is
   degraded: [replay-cache outage](replay-cache-outage.md).

## Remediation

- **No consumer.** Start the worker role. Once it reads, the backlog drains in order. Nothing needs
  to be re-enqueued.
- **Consumers present, pending not moving.** Look for the cause in the worker's logs (usually the
  database). Fix the database write path first. Batches that a crashed consumer had not acknowledged
  are claimed again after `ClaimAfter`; wait at least that long before deciding they are stuck.
- **Valkey filling up.** Free memory by fixing the consumers, not by trimming the stream. Trimming the
  stream with `MaxLen` or `XTRIM` discards batches no worker has written. Treat it as a last resort,
  record it as an incident, and explain the gap to the owners of the affected analytics.
- **Throughput too low for the volume.** Add worker capacity. If the backlog is the sign that the
  event volume has outgrown PostgreSQL, see the [ClickHouse switch-over runbook](clickhouse-switch-over.md).
- After the backlog drains, check that the rollups for the backlog window are complete before a health
  gate or a report reads them.

## Escalation

- Worker or database faults: the platform on-call, with the [server runbook](server.md).
- Any trimming of the stream, or telemetry that will not drain: the platform owner, because the data
  gap is permanent.
- Analytics that show wrong numbers after recovery: the analytics owner, with the window of the backlog.
