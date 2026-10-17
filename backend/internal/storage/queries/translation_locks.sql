-- name: GetTranslationLockForUpdate :one
SELECT * FROM translation_locks WHERE draft_id = $1 AND locale = $2 FOR UPDATE;

-- name: GetTranslationLock :one
SELECT * FROM translation_locks WHERE draft_id = $1 AND locale = $2;

-- name: PutTranslationLock :one
INSERT INTO translation_locks (draft_id, locale, organization_id, holder_kind, holder_id, holder_display, session,
                               expires_at, previous_session, previous_holder)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (draft_id, locale) DO UPDATE
   SET holder_kind = EXCLUDED.holder_kind, holder_id = EXCLUDED.holder_id,
       holder_display = EXCLUDED.holder_display, session = EXCLUDED.session,
       acquired_at = now(), heartbeat_at = now(), expires_at = EXCLUDED.expires_at,
       previous_session = EXCLUDED.previous_session, previous_holder = EXCLUDED.previous_holder,
       requested_by = '[]'
RETURNING *;

-- name: RenewTranslationLock :one
UPDATE translation_locks SET heartbeat_at = now(), expires_at = $3 WHERE draft_id = $1 AND locale = $2 RETURNING *;

-- name: DeleteTranslationLock :execrows
DELETE FROM translation_locks WHERE draft_id = $1 AND locale = $2;

-- name: RequestTranslationLock :one
UPDATE translation_locks
   SET requested_by = CASE WHEN requested_by @> sqlc.arg(requester)::jsonb THEN requested_by
                           ELSE requested_by || sqlc.arg(requester)::jsonb END
 WHERE draft_id = $1 AND locale = $2
RETURNING *;
