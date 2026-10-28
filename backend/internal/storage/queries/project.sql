-- name: GetShellSettings :one
SELECT * FROM app_shell_settings WHERE app_id = $1;

-- name: UpsertShellSettings :one
INSERT INTO app_shell_settings (app_id, organization_id, settings)
VALUES ($1, $2, $3)
ON CONFLICT (app_id) DO UPDATE SET settings = EXCLUDED.settings, updated_at = now()
RETURNING *;

-- name: SetShellGenerated :exec
-- Records what a project was generated from; settings is stored too when the
-- app had none, so that the comparison has both sides.
INSERT INTO app_shell_settings (app_id, organization_id, settings, generated, generated_at)
VALUES ($1, $2, $3, $4, now())
ON CONFLICT (app_id) DO UPDATE SET generated = $4, generated_at = now();

-- name: CreateProjectJob :one
INSERT INTO project_jobs (id, organization_id, app_id, environment_key, channel_key, format, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetProjectJob :one
SELECT * FROM project_jobs WHERE id = $1;

-- name: GetProjectJobForUpdate :one
SELECT * FROM project_jobs WHERE id = $1 FOR UPDATE;

-- name: GetActiveProjectJob :one
SELECT * FROM project_jobs WHERE app_id = $1 AND status IN ('queued', 'running');

-- name: CountProjectJobsSince :one
SELECT count(*)::bigint FROM project_jobs WHERE app_id = $1 AND created_at > $2;

-- name: StartProjectJob :execrows
UPDATE project_jobs SET status = 'running' WHERE id = $1 AND status = 'queued';

-- name: FinishProjectJob :exec
UPDATE project_jobs
   SET status = $2, error_code = $3, error_message = $4, size = $5, sha256 = $6, finished_at = now()
 WHERE id = $1;

-- name: SetProjectJobResult :exec
INSERT INTO project_job_results (job_id, organization_id, app_id, data) VALUES ($1, $2, $3, $4)
ON CONFLICT (job_id) DO UPDATE SET data = EXCLUDED.data;

-- name: GetProjectJobResult :one
SELECT data FROM project_job_results WHERE job_id = $1;
