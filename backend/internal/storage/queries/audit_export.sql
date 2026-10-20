-- name: CountAuditExportDestinations :one
SELECT count(*) FROM audit_export_destinations WHERE organization_id = $1;

-- name: InsertAuditExportDestination :one
INSERT INTO audit_export_destinations (
    id, organization_id, name, kind, address, enabled, allow_private, ca_pem,
    credentials_ciphertext, credentials_wrapped_key, credentials_key_id, credentials_hint,
    cursor_sequence, cursor_hash)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
RETURNING *;

-- name: GetAuditExportDestination :one
SELECT * FROM audit_export_destinations WHERE id = $1;

-- name: LockAuditExportDestination :one
-- Takes the destination for one delivery; a second worker that finds it
-- locked skips it, so one destination is never delivered to twice at once.
SELECT * FROM audit_export_destinations
 WHERE id = $1
 FOR UPDATE SKIP LOCKED;

-- name: ListAuditExportDestinations :many
SELECT * FROM audit_export_destinations
 WHERE organization_id = $1 AND name > sqlc.arg(after_name)::text
 ORDER BY name
 LIMIT sqlc.arg(page_size);

-- name: ListDueAuditExportDestinations :many
-- The enabled destinations whose backoff has passed.
SELECT id FROM audit_export_destinations
 WHERE organization_id = $1 AND enabled AND next_attempt_at <= sqlc.arg(now)::timestamptz
 ORDER BY id;

-- name: UpdateAuditExportDestination :one
-- A change of settings lets the next run try at once.
UPDATE audit_export_destinations
   SET address = $2, enabled = $3, allow_private = $4, ca_pem = $5,
       credentials_ciphertext = $6, credentials_wrapped_key = $7, credentials_key_id = $8,
       credentials_hint = $9,
       failures = 0, next_attempt_at = now(), last_error = '', version = version + 1, updated_at = now()
 WHERE id = $1
RETURNING *;

-- name: DeleteAuditExportDestination :execrows
DELETE FROM audit_export_destinations WHERE id = $1;

-- name: AdvanceAuditExportCursor :exec
-- Records an acknowledged batch: the cursor moves and the failures clear.
UPDATE audit_export_destinations
   SET cursor_sequence = $2, cursor_hash = $3, exported_count = exported_count + $4,
       failures = 0, next_attempt_at = $5, last_error = '', last_success_at = $5, updated_at = $5
 WHERE id = $1;

-- name: RecordAuditExportFailure :exec
-- Records a refused batch: the cursor stays and the next attempt waits.
UPDATE audit_export_destinations
   SET failures = $2, next_attempt_at = $3, last_error = $4, last_error_at = $5, updated_at = $5
 WHERE id = $1;
