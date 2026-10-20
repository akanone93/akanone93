-- name: GetNewestControlDocument :one
SELECT * FROM control_documents WHERE channel_id = $1 ORDER BY version DESC LIMIT 1;

-- name: GetNewestSignedControlDocument :one
SELECT * FROM control_documents WHERE channel_id = $1 AND signed IS NOT NULL ORDER BY version DESC LIMIT 1;

-- name: InsertControlDocument :one
INSERT INTO control_documents (channel_id, organization_id, version, kill_switches, mandatory_update, forced_rollback,
                               created_by_kind, created_by_id, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: StoreControlSignature :one
UPDATE control_documents
   SET signed = $3, signed_sha256 = $4, issued_at = $5, expires_at = $6
 WHERE channel_id = $1 AND version = $2 AND signed IS NULL
RETURNING *;

-- name: PruneControlDocuments :execrows
DELETE FROM control_documents d
 WHERE d.channel_id = $1
   AND d.version < (SELECT n.version FROM control_documents n
                     WHERE n.channel_id = $1 AND n.signed IS NOT NULL
                     ORDER BY n.version DESC OFFSET sqlc.arg(keep)::int LIMIT 1);

-- name: UpsertDevicePushToken :exec
INSERT INTO device_push_tokens (device_id, organization_id, app_id, environment_id, provider, ciphertext, wrapped_key, key_id, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
ON CONFLICT (device_id) DO UPDATE
   SET provider = excluded.provider, ciphertext = excluded.ciphertext, wrapped_key = excluded.wrapped_key,
       key_id = excluded.key_id, updated_at = now();

-- name: DeleteDevicePushToken :execrows
DELETE FROM device_push_tokens WHERE device_id = $1;

-- name: ListDevicePushTokens :many
SELECT * FROM device_push_tokens
 WHERE environment_id = $1 AND device_id > sqlc.arg(after_device)::uuid
 ORDER BY device_id
 LIMIT sqlc.arg(page_size);
