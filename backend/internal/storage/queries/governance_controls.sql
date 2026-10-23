-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- name: InsertBreakGlass :one
INSERT INTO break_glass_invocations (id, organization_id, app_id, environment_id, environment_key, production, action, subject,
                                     justification, invoked_by_kind, invoked_by_id, invoked_by, invoked_by_user, invoked_at, review_due_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
RETURNING *;

-- name: GetBreakGlassForUpdate :one
SELECT * FROM break_glass_invocations WHERE id = $1 FOR UPDATE;

-- name: ListBreakGlass :many
SELECT * FROM break_glass_invocations
 WHERE (sqlc.arg(status)::text = '' OR review_status = sqlc.arg(status)::text)
   AND (invoked_at, id) < (sqlc.arg(before_time)::timestamptz, sqlc.arg(before_id)::uuid)
 ORDER BY invoked_at DESC, id DESC
 LIMIT sqlc.arg(page_size);

-- name: ReviewBreakGlass :one
UPDATE break_glass_invocations
   SET review_status = 'reviewed', reviewed_by_id = $2, reviewed_by = $3, reviewed_at = $4, review_comment = $5
 WHERE id = $1 AND review_status = 'open'
RETURNING *;

-- name: ListOverdueBreakGlass :many
-- The open reviews whose due time has passed and that were not yet
-- escalated; the oldest first.
SELECT * FROM break_glass_invocations
 WHERE review_status = 'open' AND escalated_at IS NULL AND review_due_at <= sqlc.arg(now)::timestamptz
 ORDER BY review_due_at, id
 LIMIT sqlc.arg(page_size)
 FOR UPDATE SKIP LOCKED;

-- name: MarkBreakGlassEscalated :exec
UPDATE break_glass_invocations SET escalated_at = $2 WHERE id = $1;

-- name: InsertChangeFreeze :one
INSERT INTO change_freezes (id, organization_id, name, environment_key, time_zone, start_local, duration_minutes, recurrence, reason,
                            created_by, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $11)
ON CONFLICT (organization_id, name) DO UPDATE
   SET environment_key = EXCLUDED.environment_key, time_zone = EXCLUDED.time_zone, start_local = EXCLUDED.start_local,
       duration_minutes = EXCLUDED.duration_minutes, recurrence = EXCLUDED.recurrence, reason = EXCLUDED.reason,
       updated_at = EXCLUDED.updated_at
RETURNING *;

-- name: CountChangeFreezes :one
SELECT count(*) FROM change_freezes;

-- name: GetChangeFreezeByName :one
SELECT * FROM change_freezes WHERE name = $1;

-- name: ListChangeFreezes :many
SELECT * FROM change_freezes ORDER BY name, id;

-- name: DeleteChangeFreeze :one
DELETE FROM change_freezes WHERE id = $1 RETURNING *;

-- name: InsertProductionChange :exec
INSERT INTO production_changes (id, organization_id, app_id, environment_id, environment_key, resource, action, subject, content_hash,
                                summary, author_id, changed_by_kind, changed_by_id, changed_by, changed_at, request_id, break_glass_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17);

-- name: ListProductionChangesBetween :many
SELECT * FROM production_changes
 WHERE changed_at >= sqlc.arg(period_from)::timestamptz AND changed_at < sqlc.arg(period_to)::timestamptz
 ORDER BY changed_at, id
 LIMIT sqlc.arg(max_rows);

-- name: ListDecisionsOfRequests :many
SELECT * FROM approval_decisions WHERE request_id = ANY(sqlc.arg(request_ids)::uuid[]) ORDER BY decided_at, id;

-- name: ListBreakGlassByIDs :many
SELECT * FROM break_glass_invocations WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: InsertComplianceReport :exec
INSERT INTO compliance_reports (id, organization_id, period_from, period_to, changes, requested_by_id, requested_by, requested_at, statement)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: GetComplianceReport :one
SELECT * FROM compliance_reports WHERE id = $1;

-- name: ListComplianceReports :many
SELECT id, organization_id, period_from, period_to, changes, requested_by_id, requested_by, requested_at, key_id, signed_at
  FROM compliance_reports
 WHERE (requested_at, id) < (sqlc.arg(before_time)::timestamptz, sqlc.arg(before_id)::uuid)
 ORDER BY requested_at DESC, id DESC
 LIMIT sqlc.arg(page_size);

-- name: ListUnsignedComplianceReports :many
SELECT * FROM compliance_reports WHERE signature IS NULL ORDER BY requested_at, id LIMIT sqlc.arg(page_size);

-- name: SignComplianceReport :execrows
UPDATE compliance_reports SET key_id = $2, signature = $3, signed_at = $4 WHERE id = $1 AND signature IS NULL;
