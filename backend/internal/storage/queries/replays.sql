-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Structural session replays (ANL-050, ADR-0101).

-- name: InsertSessionReplay :execrows
INSERT INTO session_replays (id, organization_id, app_id, environment_id, device_id, started_at, duration_ms, event_count,
                             bytes, release_sequence, truncated, route, events, created_at, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
ON CONFLICT (id) DO NOTHING;

-- name: CountSessionReplaysSince :one
SELECT count(*) FROM session_replays WHERE organization_id = $1 AND device_id = $2 AND created_at >= $3;

-- name: ListSessionReplays :many
SELECT id, organization_id, app_id, environment_id, device_id, started_at, duration_ms, event_count, bytes,
       release_sequence, truncated, route, created_at, expires_at
  FROM session_replays
 WHERE app_id = sqlc.arg(app_id) AND environment_id = sqlc.arg(environment_id) AND expires_at > sqlc.arg(now)
   AND (started_at, id) < (sqlc.arg(before_time)::timestamptz, sqlc.arg(before_id)::uuid)
 ORDER BY started_at DESC, id DESC
 LIMIT sqlc.arg(page_size);

-- name: GetSessionReplay :one
SELECT * FROM session_replays WHERE id = $1 AND app_id = $2 AND expires_at > $3;

-- name: PurgeSessionReplays :execrows
DELETE FROM session_replays WHERE organization_id = $1 AND expires_at <= $2;
