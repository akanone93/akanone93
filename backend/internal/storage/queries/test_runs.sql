-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- name: InsertTestRun :one
INSERT INTO test_runs (id, organization_id, app_id, report_key, source, label, passed, failed, skipped, duration_ms, results, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
ON CONFLICT (app_id, report_key) DO NOTHING
RETURNING *;

-- name: GetTestRunByKey :one
SELECT * FROM test_runs WHERE app_id = $1 AND report_key = $2;

-- name: GetTestRun :one
SELECT * FROM test_runs WHERE app_id = $1 AND id = $2;

-- name: ListTestRuns :many
SELECT t.* FROM test_runs t
 WHERE t.app_id = $1
   AND (@before_id::uuid IS NULL OR (t.created_at, t.id) < (SELECT b.created_at, b.id FROM test_runs b WHERE b.id = @before_id::uuid))
 ORDER BY t.created_at DESC, t.id DESC
 LIMIT @page_size::int;
