-- name: LockEnvironmentForExperiments :one
SELECT id FROM environments WHERE id = $1 FOR UPDATE;

-- name: InsertFeatureFlag :one
INSERT INTO feature_flags (id, organization_id, app_id, environment_id, key, type, description, json_schema,
                           default_value, segments, created_by_kind, created_by_id, created_by,
                           updated_by_kind, updated_by_id, updated_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $11, $12, $13)
RETURNING *;

-- name: GetFeatureFlag :one
SELECT * FROM feature_flags WHERE environment_id = $1 AND key = $2;

-- name: GetFeatureFlagForUpdate :one
SELECT * FROM feature_flags WHERE environment_id = $1 AND key = $2 FOR UPDATE;

-- name: ListFeatureFlags :many
SELECT * FROM feature_flags WHERE environment_id = $1 ORDER BY key;

-- name: UpdateFeatureFlag :one
UPDATE feature_flags
   SET description = $3, json_schema = $4, default_value = $5, segments = $6,
       updated_by_kind = $7, updated_by_id = $8, updated_by = $9, updated_at = now()
 WHERE environment_id = $1 AND key = $2
RETURNING *;

-- name: DeleteFeatureFlag :execrows
DELETE FROM feature_flags WHERE environment_id = $1 AND key = $2;

-- name: CountFeatureFlagKeys :one
SELECT count(DISTINCT key) FROM feature_flags WHERE app_id = $1;

-- name: FeatureFlagKeyExistsInApp :one
SELECT EXISTS (SELECT 1 FROM feature_flags WHERE app_id = $1 AND key = $2);

-- name: InsertExperiment :one
INSERT INTO experiments (id, organization_id, app_id, environment_id, key, definition, salt, layer, layer_share_bp,
                         created_by_kind, created_by_id, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING *;

-- name: GetExperiment :one
SELECT * FROM experiments WHERE id = $1;

-- name: GetExperimentForUpdate :one
SELECT * FROM experiments WHERE id = $1 FOR UPDATE;

-- name: ListExperiments :many
SELECT * FROM experiments
 WHERE app_id = $1
   AND (sqlc.arg(environment_id)::uuid IS NULL OR environment_id = sqlc.arg(environment_id)::uuid)
   AND (sqlc.arg(state)::text = '' OR state = sqlc.arg(state)::text)
 ORDER BY environment_id, key;

-- name: ListRunningExperiments :many
SELECT * FROM experiments WHERE environment_id = $1 AND state = 'running' ORDER BY key;

-- name: ListRunningExperimentsInLayer :many
SELECT * FROM experiments
 WHERE environment_id = $1 AND state = 'running' AND layer = $2
 ORDER BY layer_start;

-- name: CountRunningExperiments :one
SELECT count(*) FROM experiments WHERE app_id = $1 AND state = 'running';

-- name: StartExperiment :one
UPDATE experiments
   SET state = 'running', layer_share_bp = $2, layer_start = $3, layer_end = $4, segment_rule = $5,
       started_at = $6, updated_at = $6
 WHERE id = $1 AND state = 'draft'
RETURNING *;

-- name: StopExperiment :one
UPDATE experiments
   SET state = 'stopped', layer_start = NULL, layer_end = NULL, stopped_at = $2, updated_at = $2
 WHERE id = $1 AND state = 'running'
RETURNING *;

-- name: ConcludeExperiment :one
UPDATE experiments
   SET state = 'concluded', winner = $2, layer_start = NULL, layer_end = NULL,
       stopped_at = COALESCE(stopped_at, $3), updated_at = $3
 WHERE id = $1 AND state IN ('running', 'stopped')
RETURNING *;

-- name: SetExperimentPromoteJob :one
UPDATE experiments SET promote_job_id = $2 WHERE id = $1 AND state = 'concluded'
RETURNING *;

-- name: DeleteDraftExperiment :execrows
DELETE FROM experiments WHERE id = $1 AND state = 'draft';

-- name: GetExperimentHoldout :one
SELECT * FROM experiment_holdouts WHERE environment_id = $1;

-- name: UpsertExperimentHoldout :one
INSERT INTO experiment_holdouts (environment_id, organization_id, basis_points, salt,
                                 updated_by_kind, updated_by_id, updated_by, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (environment_id) DO UPDATE
   SET basis_points = excluded.basis_points, updated_by_kind = excluded.updated_by_kind,
       updated_by_id = excluded.updated_by_id, updated_by = excluded.updated_by, updated_at = excluded.updated_at
RETURNING *;
