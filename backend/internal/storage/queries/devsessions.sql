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
INSERT INTO dev_pairing_tokens (token_hash, organization_id, session_id, expires_at, code) VALUES ($1, $2, $3, $4, $5);

-- name: ConsumeDevPairingCode :one
-- Spends the unconsumed, unexpired pairing token of a code in one
-- statement, as ConsumeDevPairingToken does; only a token of a session of
-- the device's app and environment matches (row-level security keeps the
-- organisation).
UPDATE dev_pairing_tokens t SET consumed_at = sqlc.arg(at)::timestamptz
  FROM dev_sessions s
 WHERE t.code = sqlc.arg(code)::text AND t.consumed_at IS NULL AND t.expires_at > sqlc.arg(at)::timestamptz
   AND s.id = t.session_id AND s.app_id = sqlc.arg(app_id) AND s.environment_id = sqlc.arg(environment_id)
 RETURNING t.session_id;

-- name: GetDevPairingCode :one
-- The latest token of a code for the device's app and environment, to tell
-- a used code from an expired or unknown one.
SELECT t.consumed_at FROM dev_pairing_tokens t JOIN dev_sessions s ON s.id = t.session_id
 WHERE t.code = sqlc.arg(code)::text AND s.app_id = sqlc.arg(app_id) AND s.environment_id = sqlc.arg(environment_id)
 ORDER BY t.consumed_at DESC NULLS FIRST
 LIMIT 1;

-- name: LockDevPairingAttempts :exec
-- Serialises the pairing-code attempts of one device, so that concurrent
-- attempts each count the others.
SELECT pg_advisory_xact_lock(hashtextextended('dev-pairing-attempts:' || sqlc.arg(organization_id)::uuid::text || ':' || sqlc.arg(device_id)::text, 0));

-- name: DropStaleDevPairingAttempts :exec
-- Drops a device's pairing-code attempts that fell out of the window.
DELETE FROM dev_pairing_attempts WHERE organization_id = $1 AND device_id = $2 AND at <= sqlc.arg(since)::timestamptz;

-- name: InsertDevPairingAttempt :exec
INSERT INTO dev_pairing_attempts (id, organization_id, device_id, at) VALUES ($1, $2, $3, $4);

-- name: CountDevPairingAttempts :one
SELECT count(*) FROM dev_pairing_attempts WHERE organization_id = $1 AND device_id = $2 AND at > sqlc.arg(since)::timestamptz;

-- name: DeleteDevPairingAttempt :exec
-- Forgets an attempt that paired: only failures count.
DELETE FROM dev_pairing_attempts WHERE id = $1;

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

-- name: RenewDevSocketTicket :one
-- Gives a paired device a new socket ticket: the device row must be the
-- caller's own, bound to the key the proof was made with, and still paired.
UPDATE dev_session_devices SET ticket_hash = sqlc.arg(ticket_hash), ticket_expires_at = sqlc.arg(ticket_expires_at)
 WHERE id = sqlc.arg(id) AND session_id = sqlc.arg(session_id) AND device_id = sqlc.arg(device_id)
   AND dpop_thumbprint = sqlc.arg(dpop_thumbprint) AND revoked_at IS NULL
 RETURNING id;
