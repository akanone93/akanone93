-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Raw events, rollups, retention and export (ANL-010, ANL-011, ADR-0080).

-- One event of a batch; the caller sends a batch of these in one round
-- trip. An event that was already written, because the queue delivers at
-- least once, is skipped.
-- name: InsertTelemetryEvent :batchexec
INSERT INTO analytics_events (id, organization_id, app_id, environment_id, device_id, name, time, release_sequence,
                              plugin_key, route, fields, device_class, locale, category)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
ON CONFLICT (id, time) DO NOTHING;

-- name: GetDeviceDimensions :one
SELECT device_class, locale FROM devices WHERE id = $1;

-- name: SetDeviceDimensions :exec
UPDATE devices SET device_class = $2, locale = $3 WHERE id = $1;

-- name: EnsureEventsPartition :one
SELECT plux_events_partition(sqlc.arg(month)::date)::text AS name;

-- name: DropEventsPartition :one
SELECT plux_drop_events_partition(sqlc.arg(month)::date) AS dropped;

-- name: DeleteRawEventsBefore :execrows
DELETE FROM analytics_events WHERE organization_id = $1 AND time < $2;

-- The five-minute buckets that events received in (after, until] fall in.
-- name: TouchedBuckets :many
SELECT DISTINCT date_bin('5 minutes', time, TIMESTAMPTZ '2000-01-01 00:00:00+00')::timestamptz AS bucket
  FROM analytics_events
 WHERE organization_id = $1 AND received_at > sqlc.arg(after)::timestamptz AND received_at <= sqlc.arg(until)::timestamptz
 ORDER BY 1;

-- The raw events of a time window, aggregated into five-minute buckets by
-- dimensions and histogram bucket. A NULL hist_bucket is the group of
-- events that carry no duration.
-- name: AggregateRawEvents :many
SELECT date_bin('5 minutes', e.time, TIMESTAMPTZ '2000-01-01 00:00:00+00')::timestamptz AS bucket,
       e.app_id, e.environment_id, e.name, e.release_sequence, e.plugin_key, e.route AS page, e.device_class, e.locale,
       COALESCE(plux_value_bucket(plux_event_value(e.name, e.fields)), -1)::integer AS hist_bucket,
       count(*)::bigint AS events,
       count(plux_event_value(e.name, e.fields))::bigint AS value_count,
       COALESCE(sum(plux_event_value(e.name, e.fields)), 0)::double precision AS value_sum,
       COALESCE(min(plux_event_value(e.name, e.fields)), 0)::double precision AS value_min,
       COALESCE(max(plux_event_value(e.name, e.fields)), 0)::double precision AS value_max
  FROM analytics_events e
 WHERE e.organization_id = $1 AND e.time >= sqlc.arg(from_time)::timestamptz AND e.time < sqlc.arg(to_time)::timestamptz
 GROUP BY 1, 2, 3, 4, 5, 6, 7, 8, 9, 10;

-- name: DeleteRollups :exec
DELETE FROM analytics_rollups
 WHERE organization_id = $1 AND granularity = $2 AND bucket >= sqlc.arg(from_time)::timestamptz AND bucket < sqlc.arg(to_time)::timestamptz;

-- name: UpsertRollup :exec
INSERT INTO analytics_rollups (organization_id, app_id, environment_id, granularity, event_name, bucket, release_sequence, plugin_key,
                               page, device_class, locale, events, value_count, value_sum, value_min, value_max, histogram)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
ON CONFLICT (app_id, environment_id, granularity, event_name, bucket, release_sequence, plugin_key, page, device_class, locale)
DO UPDATE SET events = EXCLUDED.events, value_count = EXCLUDED.value_count, value_sum = EXCLUDED.value_sum,
              value_min = EXCLUDED.value_min, value_max = EXCLUDED.value_max, histogram = EXCLUDED.histogram;

-- name: ListRollupsInWindow :many
SELECT * FROM analytics_rollups
 WHERE organization_id = $1 AND granularity = $2 AND bucket >= sqlc.arg(from_time)::timestamptz AND bucket < sqlc.arg(to_time)::timestamptz;

-- name: QueryRollups :many
SELECT * FROM analytics_rollups
 WHERE app_id = $1 AND granularity = $2 AND event_name = $3
   AND (sqlc.narg(environment_id)::uuid IS NULL OR environment_id = sqlc.narg(environment_id)::uuid)
   AND bucket >= sqlc.arg(from_time)::timestamptz AND bucket < sqlc.arg(to_time)::timestamptz
 ORDER BY bucket
 LIMIT sqlc.arg(row_limit);

-- name: DeleteRollupsBefore :execrows
DELETE FROM analytics_rollups WHERE organization_id = $1 AND bucket < $2;

-- name: GetWatermark :one
SELECT rolled_until FROM analytics_watermarks WHERE organization_id = $1;

-- name: SetWatermark :exec
INSERT INTO analytics_watermarks (organization_id, rolled_until) VALUES ($1, $2)
ON CONFLICT (organization_id) DO UPDATE SET rolled_until = EXCLUDED.rolled_until;

-- name: ListRetentionSettings :many
SELECT * FROM retention_settings WHERE organization_id = $1 ORDER BY data_class;

-- name: GetRetentionSetting :one
SELECT * FROM retention_settings WHERE organization_id = $1 AND data_class = $2;

-- name: UpsertRetentionSetting :exec
INSERT INTO retention_settings (organization_id, data_class, days, changed_by) VALUES ($1, $2, $3, $4)
ON CONFLICT (organization_id, data_class) DO UPDATE
   SET days = EXCLUDED.days, changed_by = EXCLUDED.changed_by, changed_at = now();

-- name: InsertRetentionChange :exec
INSERT INTO retention_changes (id, organization_id, data_class, days, state, requested_by, requested_at, apply_at, finished_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: GetRetentionChange :one
SELECT * FROM retention_changes WHERE id = $1 AND organization_id = $2;

-- name: ListRetentionChanges :many
SELECT * FROM retention_changes
 WHERE organization_id = $1 AND (sqlc.arg(only_scheduled)::boolean = false OR state = 'scheduled')
 ORDER BY requested_at DESC, id DESC
 LIMIT sqlc.arg(row_limit);

-- name: FinishRetentionChange :one
UPDATE retention_changes SET state = $3, finished_at = $4
 WHERE id = $1 AND organization_id = $2 AND state = 'scheduled'
RETURNING *;

-- name: ScheduledRetentionChangesSince :many
SELECT * FROM retention_changes
 WHERE organization_id = $1 AND state = 'scheduled' AND apply_at <= $2
 ORDER BY apply_at, id;

-- name: SupersedeScheduledChange :execrows
UPDATE retention_changes SET state = 'cancelled', finished_at = $3
 WHERE organization_id = $1 AND data_class = $2 AND state = 'scheduled';

-- name: GetAnalyticsExport :one
SELECT * FROM analytics_exports WHERE organization_id = $1;

-- name: UpsertAnalyticsExport :exec
INSERT INTO analytics_exports (organization_id, endpoint, enabled, exported_until, changed_by)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (organization_id) DO UPDATE
   SET endpoint = EXCLUDED.endpoint, enabled = EXCLUDED.enabled, changed_by = EXCLUDED.changed_by, changed_at = now();

-- name: DeleteAnalyticsExport :execrows
DELETE FROM analytics_exports WHERE organization_id = $1;

-- name: RecordExportAttempt :exec
UPDATE analytics_exports
   SET exported_until = $2, last_error = $3, last_attempt_at = $4
 WHERE organization_id = $1;
