-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Funnel definitions (ANL-020).

-- name: InsertFunnel :one
INSERT INTO funnels (id, organization_id, app_id, name, steps, environment_id, created_by, created_at, updated_by, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $7, $8)
RETURNING *;

-- name: GetFunnel :one
SELECT * FROM funnels WHERE id = $1 AND organization_id = $2;

-- name: DeleteFunnel :one
DELETE FROM funnels WHERE id = $1 AND organization_id = $2 RETURNING *;

-- name: CountFunnels :one
SELECT count(*) FROM funnels WHERE organization_id = $1;

-- name: ListFunnels :many
SELECT * FROM funnels
 WHERE organization_id = $1 AND (sqlc.arg(app_id)::uuid IS NULL OR app_id = sqlc.arg(app_id)::uuid)
 ORDER BY app_id, name
 LIMIT sqlc.arg(row_limit);

-- name: ListActiveAppIDs :many
SELECT id FROM apps
 WHERE organization_id = $1 AND deleted_at IS NULL AND archived_at IS NULL
 ORDER BY key, id
 LIMIT sqlc.arg(row_limit);

-- The events a funnel result reads (ANL-020): those of the funnel's steps and
-- the session starts that bound sessions, of one app and optionally one
-- environment, in a half-open time range, in the order they happened.
-- name: ListFunnelEvents :many
SELECT device_id, name FROM analytics_events
 WHERE organization_id = $1 AND app_id = $2
   AND (sqlc.narg(environment_id)::uuid IS NULL OR environment_id = sqlc.narg(environment_id)::uuid)
   AND time >= sqlc.arg(from_time)::timestamptz AND time < sqlc.arg(to_time)::timestamptz
   AND name = ANY(sqlc.arg(names)::text[])
 ORDER BY time, id
 LIMIT sqlc.arg(row_limit);
