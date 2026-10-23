-- Dev sessions and pairing (DEV-002, DEV-003, ADR-0087).

-- name: InsertDevSession :one
INSERT INTO dev_sessions (id, organization_id, app_id, environment_id, environment_key, created_by_user, created_by_kind, created_by_id, created_by,
                          created_at, expires_at, last_activity_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $10)
RETURNING *;

-- name: GetDevSession :one
SELECT * FROM dev_sessions WHERE id = $1;

-- name: GetDevSessionForUpdate :one
SELECT * FROM dev_sessions WHERE id = $1 FOR UPDATE;

-- name: TouchDevSession :exec
-- Records traffic on a session; the instant never moves backwards.
UPDATE dev_sessions SET last_activity_at = GREATEST(last_activity_at, sqlc.arg(at)::timestamptz) WHERE id = $1 AND ended_at IS NULL;

-- name: EndDevSession :one
-- Ends a session; only one caller wins.
UPDATE dev_sessions SET ended_at = $2 WHERE id = $1 AND ended_at IS NULL RETURNING *;

-- name: InsertDevPairingToken :exec
INSERT INTO dev_pairing_tokens (token_hash, organization_id, session_id, expires_at) VALUES ($1, $2, $3, $4);

-- name: ConsumeDevPairingToken :one
-- Spends a pairing token in one statement: of two devices that present the
-- same token, one wins and the other finds nothing to update.
UPDATE dev_pairing_tokens SET consumed_at = sqlc.arg(at)::timestamptz
 WHERE token_hash = $1 AND consumed_at IS NULL AND expires_at > sqlc.arg(at)::timestamptz
 RETURNING session_id;

-- name: GetDevPairingToken :one
SELECT * FROM dev_pairing_tokens WHERE token_hash = $1;

-- name: CountDevSessionDevices :one
SELECT count(*) FROM dev_session_devices WHERE session_id = $1 AND revoked_at IS NULL;

-- name: RevokeDevSessionDevice :exec
-- Unpairs the earlier pairings of a device with a session, which a new
-- pairing of the same device replaces.
UPDATE dev_session_devices SET revoked_at = $3, ticket_hash = NULL, ticket_expires_at = NULL
 WHERE session_id = $1 AND device_id = $2 AND revoked_at IS NULL;

-- name: RevokeDevSessionDevices :exec
UPDATE dev_session_devices SET revoked_at = $2, ticket_hash = NULL, ticket_expires_at = NULL
 WHERE session_id = $1 AND revoked_at IS NULL;

-- name: InsertDevSessionDevice :one
INSERT INTO dev_session_devices (id, organization_id, session_id, device_id, dpop_thumbprint, name, platform, runtime_version, paired_at,
                                 ticket_hash, ticket_expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: ListDevSessionDevices :many
SELECT * FROM dev_session_devices WHERE session_id = $1 AND revoked_at IS NULL ORDER BY paired_at, id;

-- name: FindDevSocketTicket :one
-- The organisation of the device a socket ticket was issued to, read in
-- the 'devsocket' scope before the organisation is known.
SELECT organization_id FROM dev_session_devices WHERE ticket_hash = $1;

-- name: ConsumeDevSocketTicket :one
-- Spends a socket ticket in one statement: it must be unexpired, presented
-- with the key the device paired with, and the device still paired.
UPDATE dev_session_devices SET ticket_hash = NULL, ticket_expires_at = NULL
 WHERE ticket_hash = $1 AND dpop_thumbprint = $2 AND revoked_at IS NULL AND ticket_expires_at > sqlc.arg(at)::timestamptz
 RETURNING *;
