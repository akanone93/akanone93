-- name: BumpConfigEpoch :one
INSERT INTO config_environments (environment_id, organization_id, app_id, epoch, live_epoch, strict_epoch)
VALUES (
    sqlc.arg(environment_id), sqlc.arg(organization_id), sqlc.arg(app_id), 1,
    CASE WHEN sqlc.arg(live)::boolean THEN 1 ELSE 0 END,
    CASE WHEN sqlc.arg(strict)::boolean THEN 1 ELSE 0 END)
ON CONFLICT (environment_id) DO UPDATE
   SET epoch = config_environments.epoch + 1,
       live_epoch = CASE WHEN sqlc.arg(live)::boolean THEN config_environments.epoch + 1 ELSE config_environments.live_epoch END,
       strict_epoch = CASE WHEN sqlc.arg(strict)::boolean THEN config_environments.epoch + 1 ELSE config_environments.strict_epoch END,
       updated_at = now()
RETURNING *;

-- name: GetConfigEnvironment :one
SELECT * FROM config_environments WHERE environment_id = $1;

-- name: LockConfigEnvironment :one
SELECT * FROM config_environments WHERE environment_id = $1 FOR UPDATE;

-- name: UpsertConfigValue :one
INSERT INTO config_values
    (environment_id, key, organization_id, app_id, tier, value_type, sensitive, strict, value,
     ciphertext, wrapped_key, key_id, updated_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
ON CONFLICT (environment_id, key) DO UPDATE
   SET tier = EXCLUDED.tier, value_type = EXCLUDED.value_type, sensitive = EXCLUDED.sensitive,
       strict = EXCLUDED.strict, value = EXCLUDED.value, ciphertext = EXCLUDED.ciphertext,
       wrapped_key = EXCLUDED.wrapped_key, key_id = EXCLUDED.key_id, updated_by = EXCLUDED.updated_by,
       version = config_values.version + 1, updated_at = now()
RETURNING *;

-- name: GetConfigValue :one
SELECT * FROM config_values WHERE environment_id = $1 AND key = $2;

-- name: GetConfigValueForUpdate :one
SELECT * FROM config_values WHERE environment_id = $1 AND key = $2 FOR UPDATE;

-- name: ListConfigValues :many
SELECT * FROM config_values
 WHERE environment_id = $1 AND key > sqlc.arg(after_key)::text
 ORDER BY key
 LIMIT sqlc.arg(page_size);

-- name: ListEnvironmentConfigValues :many
SELECT * FROM config_values WHERE environment_id = $1 ORDER BY key;

-- name: CountConfigValues :one
SELECT count(*) FROM config_values WHERE environment_id = $1;

-- name: DeleteConfigValue :one
DELETE FROM config_values WHERE environment_id = $1 AND key = $2
RETURNING *;

-- name: EnsureConfigEnvironment :exec
INSERT INTO config_environments (environment_id, organization_id, app_id)
VALUES ($1, $2, $3)
ON CONFLICT (environment_id) DO NOTHING;
