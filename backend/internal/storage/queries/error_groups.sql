-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Grouped errors (ANL-040, CMP-041).

-- name: UpsertErrorGroup :one
INSERT INTO error_groups (id, organization_id, app_id, release_sequence, fingerprint, code, plugin_key, route, node_path,
                          frames, fatal, first_seen, last_seen)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $12)
ON CONFLICT (app_id, release_sequence, fingerprint) DO UPDATE
   SET first_seen = LEAST(error_groups.first_seen, EXCLUDED.first_seen),
       last_seen = GREATEST(error_groups.last_seen, EXCLUDED.last_seen),
       fatal = error_groups.fatal OR EXCLUDED.fatal
RETURNING id;

-- name: InsertErrorSample :execrows
INSERT INTO error_samples (group_id, organization_id, event_id, occurred_at, fields)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (group_id, event_id) DO NOTHING;

-- name: AddErrorGroupDevice :execrows
INSERT INTO error_group_devices (group_id, organization_id, device_id) VALUES ($1, $2, $3)
ON CONFLICT (group_id, device_id) DO NOTHING;

-- name: CountErrorGroupEvent :exec
UPDATE error_groups SET events = events + 1, devices = devices + sqlc.arg(new_devices)::bigint WHERE id = $1;

-- name: AddErrorGroupDay :exec
INSERT INTO error_group_days (group_id, organization_id, day, events) VALUES ($1, $2, $3, 1)
ON CONFLICT (group_id, day) DO UPDATE SET events = error_group_days.events + 1;

-- name: PruneErrorSamples :exec
DELETE FROM error_samples s
 WHERE s.group_id = $1
   AND (s.occurred_at, s.event_id) NOT IN (
       SELECT k.occurred_at, k.event_id FROM error_samples k WHERE k.group_id = $1
        ORDER BY k.occurred_at DESC, k.event_id DESC LIMIT sqlc.arg(keep)::int);

-- name: ListErrorGroups :many
SELECT * FROM error_groups
 WHERE organization_id = $1 AND app_id = $2
   AND (sqlc.arg(release_sequence)::bigint = 0 OR release_sequence = sqlc.arg(release_sequence)::bigint)
   AND last_seen >= sqlc.arg(since)::timestamptz
 ORDER BY last_seen DESC, id
 LIMIT sqlc.arg(row_limit);

-- name: GetErrorGroup :one
SELECT * FROM error_groups WHERE id = $1 AND organization_id = $2;

-- name: SetErrorGroupSymbol :exec
UPDATE error_groups SET json_path = $2, source_file = $3, symbolicated_at = $4 WHERE id = $1;

-- name: ListErrorGroupDaysFor :many
SELECT * FROM error_group_days WHERE group_id = ANY(sqlc.arg(group_ids)::uuid[]) AND day >= sqlc.arg(since)::date ORDER BY group_id, day;

-- name: ListErrorSamples :many
SELECT * FROM error_samples WHERE group_id = $1 ORDER BY occurred_at DESC, event_id DESC LIMIT sqlc.arg(row_limit);

-- name: ReleasePluginArtifacts :one
SELECT pv.bundle_sha256, pv.source_map_sha256
  FROM releases r
  JOIN release_versions rv ON rv.release_id = r.id
  JOIN plugin_versions pv ON pv.id = rv.plugin_version_id
 WHERE r.app_id = $1 AND r.sequence = $2 AND pv.plugin_key = $3;
