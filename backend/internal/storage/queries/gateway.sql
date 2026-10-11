-- name: InsertGatewayUpstream :one
INSERT INTO gateway_upstreams
    (id, organization_id, app_id, environment_id, name, base_url, auth_kind, auth,
     allow_private_addresses, timeout_ms, allowed_methods, requires_assurance, operations, audience)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
RETURNING *;

-- name: GetGatewayUpstream :one
SELECT * FROM gateway_upstreams WHERE id = $1;

-- name: GetGatewayUpstreamByName :one
SELECT * FROM gateway_upstreams WHERE environment_id = $1 AND name = $2;

-- name: ListGatewayUpstreams :many
SELECT * FROM gateway_upstreams
 WHERE app_id = $1
   AND (sqlc.narg(environment_id)::uuid IS NULL OR environment_id = sqlc.narg(environment_id)::uuid)
   AND (environment_id, name) > (sqlc.arg(after_environment)::uuid, sqlc.arg(after_name)::text)
 ORDER BY environment_id, name
 LIMIT sqlc.arg(page_size);

-- name: UpdateGatewayUpstream :one
UPDATE gateway_upstreams
   SET name = $2, base_url = $3, auth_kind = $4, auth = $5, allow_private_addresses = $6,
       timeout_ms = $7, allowed_methods = $8, requires_assurance = $9, operations = $10,
       audience = $11, version = version + 1, updated_at = now()
 WHERE id = $1
RETURNING *;

-- name: DeleteGatewayUpstream :execrows
DELETE FROM gateway_upstreams WHERE id = $1;

-- name: UpsertDeviceSCAKey :exec
INSERT INTO device_sca_keys (device_id, organization_id, public_key, jkt, key_storage, attestation_provider)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (device_id) DO UPDATE
   SET public_key = EXCLUDED.public_key, jkt = EXCLUDED.jkt, key_storage = EXCLUDED.key_storage,
       attestation_provider = EXCLUDED.attestation_provider, enrolled_at = now();

-- name: GetDeviceSCAKey :one
SELECT * FROM device_sca_keys WHERE device_id = $1;

-- name: UpsertUserIdentityProvider :exec
INSERT INTO user_identity_providers (environment_id, organization_id, app_id, issuer, jwks_url, audiences, forward_claims)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (environment_id) DO UPDATE
   SET issuer = EXCLUDED.issuer, jwks_url = EXCLUDED.jwks_url, audiences = EXCLUDED.audiences,
       forward_claims = EXCLUDED.forward_claims, updated_at = now();

-- name: GetUserIdentityProvider :one
SELECT * FROM user_identity_providers WHERE environment_id = $1;

-- name: DeleteUserIdentityProvider :execrows
DELETE FROM user_identity_providers WHERE environment_id = $1;

-- name: ListEnvironmentGatewayUpstreams :many
SELECT * FROM gateway_upstreams WHERE environment_id = $1 ORDER BY name;
