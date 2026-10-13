-- name: PutImportedKey :exec
INSERT INTO imported_keys (role, key_id, team_id, wrapped_data_key, wrap_key_id, nonce, ciphertext, public_key_sha256, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (role) DO UPDATE SET
    key_id            = EXCLUDED.key_id,
    team_id           = EXCLUDED.team_id,
    wrapped_data_key  = EXCLUDED.wrapped_data_key,
    wrap_key_id       = EXCLUDED.wrap_key_id,
    nonce             = EXCLUDED.nonce,
    ciphertext        = EXCLUDED.ciphertext,
    public_key_sha256 = EXCLUDED.public_key_sha256,
    created_at        = EXCLUDED.created_at;

-- name: GetImportedKey :one
SELECT role, key_id, team_id, wrapped_data_key, wrap_key_id, nonce, ciphertext, public_key_sha256, created_at
FROM imported_keys WHERE role = $1;
