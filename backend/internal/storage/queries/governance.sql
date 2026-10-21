-- name: CreateCustomRole :one
INSERT INTO custom_roles (id, organization_id, key, name, description, permissions, created_by)
VALUES ($1, $2, $3, $4, $5, $6, sqlc.narg(created_by)::uuid)
RETURNING *;

-- name: GetCustomRole :one
SELECT * FROM custom_roles WHERE organization_id = $1 AND key = $2;

-- name: ListCustomRoles :many
SELECT * FROM custom_roles
 WHERE organization_id = $1 AND key > sqlc.arg(after_key)::text
 ORDER BY key
 LIMIT sqlc.arg(page_size);

-- name: ListAllCustomRoles :many
-- Every custom role of the organisation, for the resolver.
SELECT * FROM custom_roles WHERE organization_id = $1 ORDER BY key;

-- name: UpdateCustomRole :one
UPDATE custom_roles
   SET name = $3, description = $4, permissions = $5, updated_at = now()
 WHERE organization_id = $1 AND key = $2
RETURNING *;

-- name: DeleteCustomRole :execrows
DELETE FROM custom_roles WHERE organization_id = $1 AND key = $2;

-- name: CountGrantsOfRole :one
-- How many grants and app-access rows use a role, so that a role in use is
-- not deleted from under them.
SELECT (SELECT count(*) FROM role_grants g WHERE g.organization_id = $1 AND g.role = $2)
     + (SELECT count(*) FROM app_access a WHERE a.organization_id = $1 AND a.role = $2) AS n;

-- name: CreateRoleGrant :one
INSERT INTO role_grants (id, organization_id, user_id, team_id, role, scope, app_id, environment_id)
VALUES ($1, $2, sqlc.narg(user_id)::uuid, sqlc.narg(team_id)::uuid, $3, $4,
        sqlc.narg(app_id)::uuid, sqlc.narg(environment_id)::uuid)
RETURNING *;

-- name: GetRoleGrant :one
SELECT * FROM role_grants WHERE id = $1;

-- name: UpdateRoleGrant :one
UPDATE role_grants SET role = $2 WHERE id = $1 RETURNING *;

-- name: DeleteRoleGrant :execrows
DELETE FROM role_grants WHERE id = $1;

-- name: ListRoleGrants :many
SELECT * FROM role_grants
 WHERE organization_id = $1
   AND id > sqlc.arg(after_id)::uuid
   AND (sqlc.narg(user_id)::uuid IS NULL OR user_id = sqlc.narg(user_id)::uuid)
   AND (sqlc.narg(team_id)::uuid IS NULL OR team_id = sqlc.narg(team_id)::uuid)
   AND (sqlc.narg(app_id)::uuid IS NULL OR app_id = sqlc.narg(app_id)::uuid)
 ORDER BY id
 LIMIT sqlc.arg(page_size);

-- name: ListRoleGrantsForUser :many
-- The grants a user holds directly or through a team, with the key of the
-- environment a grant is limited to (GOV-003).
SELECT g.id, g.role, g.scope, g.app_id, e.key AS environment_key
  FROM role_grants g
  LEFT JOIN environments e ON e.id = g.environment_id
 WHERE g.organization_id = $1
   AND (g.user_id = sqlc.arg(user_id)::uuid
        OR g.team_id IN (SELECT m.team_id FROM memberships m
                          WHERE m.organization_id = $1 AND m.user_id = sqlc.arg(user_id)::uuid
                            AND m.team_id IS NOT NULL));
