-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- name: GetGitSettings :one
SELECT * FROM git_settings WHERE app_id = $1;

-- name: UpsertGitSettings :one
INSERT INTO git_settings (app_id, organization_id, enabled, remote_url, branch, username,
                          credential_environment_id, credential_key, updated_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (app_id) DO UPDATE
   SET enabled = EXCLUDED.enabled, remote_url = EXCLUDED.remote_url, branch = EXCLUDED.branch,
       username = EXCLUDED.username, credential_environment_id = EXCLUDED.credential_environment_id,
       credential_key = EXCLUDED.credential_key, updated_by = EXCLUDED.updated_by, updated_at = now()
RETURNING *;

-- name: InsertGitExport :one
INSERT INTO git_exports (id, organization_id, app_id, version_id)
VALUES ($1, $2, $3, $4)
ON CONFLICT (app_id, version_id) DO NOTHING
RETURNING *;

-- name: GetGitExportForUpdate :one
SELECT * FROM git_exports WHERE app_id = $1 AND version_id = $2 FOR UPDATE;

-- name: MarkGitExportAttempt :exec
UPDATE git_exports SET attempts = attempts + 1, updated_at = now() WHERE id = $1;

-- name: MarkGitExportPushed :exec
UPDATE git_exports
   SET state = 'pushed', commit_sha = $2, error_code = 0, updated_at = now()
 WHERE id = $1;

-- name: MarkGitExportFailed :exec
UPDATE git_exports SET state = 'failed', error_code = $2, updated_at = now() WHERE id = $1;

-- name: ListGitExports :many
SELECT * FROM git_exports WHERE app_id = $1 ORDER BY created_at DESC, id DESC LIMIT sqlc.arg(page_size);
