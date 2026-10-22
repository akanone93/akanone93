-- Rollouts (REL-050 to REL-053, ADR-0078).

-- name: InsertRollout :one
INSERT INTO rollouts (id, organization_id, app_id, environment_id, channel_id, environment_key, channel_key, candidate_sequence, salt, state,
                      stages, start_at, end_at, targeting, allow_devices, deny_devices, health, created_by, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'scheduled', $10, $11, $12, $13, $14, $15, $16, $17, $18, $18)
RETURNING *;

-- name: GetRollout :one
SELECT * FROM rollouts WHERE id = $1;

-- name: GetRolloutForUpdate :one
SELECT * FROM rollouts WHERE id = $1 FOR UPDATE;

-- name: ListRollouts :many
-- An app's rollouts, newest first; identifiers are time-ordered, so the
-- keyset is the identifier.
SELECT * FROM rollouts
 WHERE app_id = $1
   AND (NOT sqlc.arg(only_open)::boolean OR state IN ('scheduled', 'active', 'paused', 'closing'))
   AND (sqlc.narg(before)::uuid IS NULL OR id < sqlc.narg(before)::uuid)
 ORDER BY id DESC LIMIT sqlc.arg(page_size);

-- name: ListOpenRollouts :many
-- Every rollout of the organisation that still has work to do or serves a
-- candidate, oldest first.
SELECT * FROM rollouts WHERE state IN ('scheduled', 'active', 'paused', 'closing') ORDER BY id;

-- name: CountOpenRollouts :one
SELECT count(*) FROM rollouts WHERE app_id = $1 AND state IN ('scheduled', 'active', 'paused', 'closing');

-- name: ListOverlappingRollouts :many
-- The open rollouts of a channel whose window meets the given one; a
-- rollout without an end lasts for ever.
SELECT * FROM rollouts
 WHERE channel_id = $1 AND state IN ('scheduled', 'active', 'paused', 'closing')
   AND tstzrange(start_at, COALESCE(end_at, 'infinity'::timestamptz)) && tstzrange(sqlc.arg(window_start)::timestamptz, COALESCE(sqlc.narg(window_end)::timestamptz, 'infinity'::timestamptz))
 ORDER BY id;

-- name: ListServingRollouts :many
-- The rollouts of a channel that serve their candidate to some devices.
SELECT r.* FROM rollouts r JOIN channels c ON c.id = r.channel_id
 WHERE c.environment_id = $1 AND c.key = $2 AND r.state IN ('active', 'paused')
 ORDER BY r.id;

-- name: ListChannelServingRollouts :many
SELECT * FROM rollouts WHERE channel_id = $1 AND state IN ('active', 'paused') ORDER BY id;

-- name: StartRollout :one
-- Moves a scheduled rollout to its first stage; only one caller wins.
UPDATE rollouts
   SET state = 'active', previous_sequence = $2, stage_index = 0, stage_started_at = $3, basis_points = $4,
       state_reason = '', updated_at = $3
 WHERE id = $1 AND state = 'scheduled'
 RETURNING *;

-- name: AdvanceRollout :one
-- Moves an active rollout on to a later stage; only one caller wins.
UPDATE rollouts
   SET stage_index = $2, stage_started_at = $3, basis_points = $4, updated_at = $5
 WHERE id = $1 AND state = 'active' AND stage_index < $2
 RETURNING *;

-- name: SetRolloutState :one
-- Pauses or resumes a rollout. A resumed rollout starts its stage afresh.
UPDATE rollouts
   SET state = sqlc.arg(state), state_reason = sqlc.arg(reason),
       stage_started_at = COALESCE(sqlc.narg(stage_started_at)::timestamptz, stage_started_at),
       updated_at = sqlc.arg(at)
 WHERE id = sqlc.arg(id) AND state = ANY(sqlc.arg(from_states)::text[])
 RETURNING *;

-- name: BeginClosingRollout :one
-- Marks a rollout as closing: the worker then does close_action and ends the
-- rollout in close_state. Only one caller wins.
UPDATE rollouts
   SET state = 'closing', close_action = $2, close_state = $3, state_reason = $4, updated_at = $5
 WHERE id = $1 AND state IN ('scheduled', 'active', 'paused')
 RETURNING *;

-- name: FinishClosingRollout :one
UPDATE rollouts
   SET state = close_state, close_action = NULL, close_state = NULL, reverted_sequence = $2, updated_at = $3
 WHERE id = $1 AND state = 'closing'
 RETURNING *;

-- name: RecordRolloutCheck :exec
UPDATE rollouts SET last_check = $2, last_check_at = $3 WHERE id = $1;

-- name: InsertRolloutManifest :one
INSERT INTO manifests (id, organization_id, channel_id, release_sequence, signed, signatures, issued_at, expires_at, rollout_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: LatestRolloutManifest :one
SELECT * FROM manifests WHERE rollout_id = $1 ORDER BY issued_at DESC, id DESC LIMIT 1;

-- name: ClaimClosingRollout :one
-- Locks a closing rollout for the caller that finishes it. A caller that finds
-- it locked waits, then finds it no longer closing when the holder finished
-- it. Skipping a locked row is not safe: a scheduler that lost the race to
-- begin closing holds the row until it rolls back, and the winner that
-- skipped would leave the rollout closing.
SELECT * FROM rollouts WHERE id = $1 AND state = 'closing' FOR UPDATE;
