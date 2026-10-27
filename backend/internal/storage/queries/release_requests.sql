-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Returns no row when the app, environment and channel already have an open
-- request (F11).
-- name: InsertReleaseRequest :one
INSERT INTO release_requests (id, organization_id, app_id, environment_id, channel, state, plugins, created_by, created_by_name)
VALUES ($1, $2, $3, $4, $5, 'select', $6, $7, $8)
ON CONFLICT (app_id, environment_id, channel) WHERE state NOT IN ('released', 'rejected', 'expired', 'cancelled') DO NOTHING
RETURNING *;

-- name: GetReleaseRequest :one
SELECT * FROM release_requests WHERE id = $1 AND app_id = $2;

-- name: GetReleaseRequestForUpdate :one
SELECT * FROM release_requests WHERE id = $1 AND app_id = $2 FOR UPDATE;

-- The open request of an app, environment and channel (F11).
-- name: GetOpenReleaseRequest :one
SELECT * FROM release_requests
 WHERE app_id = $1 AND environment_id = $2 AND channel = $3
   AND state NOT IN ('released', 'rejected', 'expired', 'cancelled');

-- name: ListReleaseRequests :many
SELECT rr.* FROM release_requests rr
 WHERE rr.app_id = $1
   AND (NOT @open_only::boolean OR rr.state NOT IN ('released', 'rejected', 'expired', 'cancelled'))
   AND (@before_id::uuid IS NULL OR (rr.created_at, rr.id) < (SELECT b.created_at, b.id FROM release_requests b WHERE b.id = @before_id::uuid))
 ORDER BY rr.created_at DESC, rr.id DESC
 LIMIT @page_size::int;

-- The whole mutable part of a request in one statement; revision rises with
-- every write, and the caller holds the row's lock.
-- name: UpdateReleaseRequest :one
UPDATE release_requests SET
    environment_id = $3, channel = $4, state = $5, hold = $6, hold_reason = $7,
    plugins = $8, builds = $9, acknowledged = $10, content_hash = $11,
    changelog_notes = $12, label = $13, rollout_plan = $14,
    approval_request_id = $15, approved_hash = $16, release_sequence = $17,
    ended_at = $18, revision = revision + 1, updated_at = now()
 WHERE id = $1 AND app_id = $2
RETURNING *;

-- name: InsertVisualDiffJob :one
INSERT INTO visual_diff_jobs (id, organization_id, release_request_id, content_hash, created_by)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetVisualDiffJob :one
SELECT * FROM visual_diff_jobs WHERE id = $1;

-- name: GetVisualDiffJobForUpdate :one
SELECT * FROM visual_diff_jobs WHERE id = $1 FOR UPDATE;

-- name: GetInFlightVisualDiffJob :one
SELECT * FROM visual_diff_jobs
 WHERE release_request_id = $1 AND content_hash = $2 AND state IN ('queued', 'running');

-- The newest job of a request for one content, finished or not.
-- name: LatestVisualDiffJob :one
SELECT * FROM visual_diff_jobs
 WHERE release_request_id = $1 AND content_hash = $2
 ORDER BY created_at DESC, id DESC LIMIT 1;

-- name: CountRunningVisualDiffJobs :one
SELECT count(*)::bigint AS running FROM visual_diff_jobs WHERE organization_id = $1 AND state = 'running';

-- name: StartVisualDiffJob :exec
UPDATE visual_diff_jobs SET state = 'running', started_at = now(), attempts = attempts + 1 WHERE id = $1;

-- name: QueueVisualDiffJob :exec
UPDATE visual_diff_jobs SET state = 'queued', error = $2 WHERE id = $1;

-- name: FinishVisualDiffJob :exec
UPDATE visual_diff_jobs SET state = $2, result = $3, error = $4, finished_at = now() WHERE id = $1;

-- Serialises the starts of one organisation's render jobs, so that the limit
-- on those running at once cannot be passed by two workers at the same time.
-- name: LockVisualDiffOrganization :exec
SELECT pg_advisory_xact_lock(hashtextextended('visual-diff:' || @organization::text, 0));

-- The render worker reads a request by identifier alone.
-- name: GetReleaseRequestByID :one
SELECT * FROM release_requests WHERE id = $1;

-- name: GetReleaseRequestByIDForUpdate :one
SELECT * FROM release_requests WHERE id = $1 FOR UPDATE;
