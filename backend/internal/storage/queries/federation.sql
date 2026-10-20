-- name: CreateIdentityProvider :one
INSERT INTO identity_providers (id, organization_id, key, kind, name, enabled, jit_provisioning, config, secret, created_by)
VALUES (sqlc.arg(id), sqlc.narg(organization_id)::uuid, sqlc.arg(key), sqlc.arg(kind), sqlc.arg(name),
        sqlc.arg(enabled), sqlc.arg(jit_provisioning), sqlc.arg(config), sqlc.narg(secret)::bytea,
        sqlc.narg(created_by)::uuid)
RETURNING *;

-- name: GetIdentityProvider :one
SELECT * FROM identity_providers WHERE id = $1;

-- name: GetIdentityProviderByKey :one
SELECT * FROM identity_providers
 WHERE organization_id IS NOT DISTINCT FROM sqlc.narg(organization_id)::uuid AND key = sqlc.arg(key)::text;

-- name: ListIdentityProviders :many
-- The providers of one organisation, or of the installation when the
-- organisation is NULL.
SELECT * FROM identity_providers
 WHERE organization_id IS NOT DISTINCT FROM sqlc.narg(organization_id)::uuid
   AND key > sqlc.arg(after_key)::text
 ORDER BY key
 LIMIT sqlc.arg(page_size);

-- name: ListSignInProviders :many
-- What a person may choose on the sign-in page: the installation's enabled
-- providers and those of the named organisation.
SELECT id, organization_id, key, kind, name FROM identity_providers
 WHERE enabled AND (organization_id IS NULL OR organization_id = sqlc.narg(organization_id)::uuid)
 ORDER BY organization_id NULLS FIRST, key
 LIMIT sqlc.arg(page_size);

-- name: UpdateIdentityProvider :one
UPDATE identity_providers
   SET name = sqlc.arg(name), enabled = sqlc.arg(enabled), jit_provisioning = sqlc.arg(jit_provisioning),
       config = sqlc.arg(config), secret = COALESCE(sqlc.narg(secret)::bytea, secret), updated_at = now()
 WHERE id = sqlc.arg(id)
RETURNING *;

-- name: SetIdentityProviderMapping :one
UPDATE identity_providers SET mapping = $2, updated_at = now() WHERE id = $1 RETURNING *;

-- name: DeleteIdentityProvider :execrows
DELETE FROM identity_providers WHERE id = $1;

-- name: CountOtherEnabledInstallationProviders :one
-- The installation-wide providers, other than one, that people can sign in
-- with.
SELECT count(*) FROM identity_providers
 WHERE organization_id IS NULL AND enabled AND id <> sqlc.arg(excluded)::uuid;

-- name: CountEnabledInstallationProviders :one
SELECT count(*) FROM identity_providers WHERE organization_id IS NULL AND enabled;

-- name: CreateProviderOIDCLogin :exec
INSERT INTO oidc_logins (state_hash, nonce, verifier, expires_at, provider_id)
VALUES (sqlc.arg(state_hash), sqlc.arg(nonce), sqlc.arg(verifier), sqlc.arg(expires_at), sqlc.narg(provider_id)::uuid);

-- name: CreateSAMLLogin :exec
INSERT INTO saml_logins (state_hash, provider_id, request_id, expires_at)
VALUES ($1, $2, $3, $4);

-- name: TakeSAMLLogin :one
DELETE FROM saml_logins WHERE state_hash = $1 RETURNING *;

-- name: DeleteExpiredSAMLLogins :execrows
DELETE FROM saml_logins WHERE expires_at < now();

-- name: GetOrganizationByKey :one
SELECT * FROM organizations WHERE key = $1;

-- name: GetAppByKey :one
SELECT * FROM apps WHERE key = $1 AND deleted_at IS NULL;

-- name: GetTeamByKey :one
SELECT * FROM teams WHERE key = $1;

-- name: RecordFederatedSignIn :one
INSERT INTO federated_sign_ins (id, user_id, provider_id, issuer, subject, sid, acr, amr, session_id)
VALUES (sqlc.arg(id), sqlc.arg(user_id), sqlc.narg(provider_id)::uuid, sqlc.arg(issuer), sqlc.arg(subject),
        sqlc.arg(sid), sqlc.arg(acr), sqlc.arg(amr), sqlc.narg(session_id)::uuid)
RETURNING *;

-- name: SetFederatedSignInSession :exec
UPDATE federated_sign_ins SET session_id = $2 WHERE id = $1;

-- name: DeleteOldFederatedSignIns :exec
DELETE FROM federated_sign_ins WHERE user_id = $1 AND signed_in_at < $2;

-- name: ListFederatedSignInsFor :many
-- The sign-ins a back-channel logout names: by the provider's session
-- identifier when it sent one, by subject otherwise.
SELECT * FROM federated_sign_ins
 WHERE issuer = sqlc.arg(issuer)
   AND ((sqlc.arg(sid)::text <> '' AND sid = sqlc.arg(sid)::text)
        OR (sqlc.arg(sid)::text = '' AND subject = sqlc.arg(subject)::text));

-- name: ListFederatedAssignmentsOfUser :many
-- Everything a provider's mapping gave a person, in every organisation,
-- with the grant or team membership it stands for.
SELECT a.id, a.organization_id, a.role_grant_id, a.membership_id,
       g.role AS grant_role, g.scope AS grant_scope, g.app_id AS grant_app_id,
       g.environment_id AS grant_environment_id, m.team_id AS team_id
  FROM federated_assignments a
  LEFT JOIN role_grants g ON g.id = a.role_grant_id
  LEFT JOIN memberships m ON m.id = a.membership_id
 WHERE a.provider_id = $1 AND a.user_id = $2;

-- name: ListFederatedAssignmentsOfProvider :many
SELECT a.id, a.organization_id, a.role_grant_id, a.membership_id
  FROM federated_assignments a
 WHERE a.provider_id = $1;

-- name: CreateFederatedGrant :one
-- A grant the mapping asks for; an identical one that exists already is
-- left alone and not owned by the provider.
INSERT INTO role_grants (id, organization_id, user_id, role, scope, app_id, environment_id)
VALUES (sqlc.arg(id), sqlc.arg(organization_id), sqlc.arg(user_id), sqlc.arg(role), sqlc.arg(scope),
        sqlc.narg(app_id)::uuid, sqlc.narg(environment_id)::uuid)
ON CONFLICT DO NOTHING
RETURNING *;

-- name: CreateFederatedTeamMembership :one
INSERT INTO memberships (id, organization_id, user_id, team_id, role)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT DO NOTHING
RETURNING *;

-- name: CreateFederatedAssignment :exec
INSERT INTO federated_assignments (id, organization_id, provider_id, user_id, role_grant_id, membership_id)
VALUES (sqlc.arg(id), sqlc.arg(organization_id), sqlc.arg(provider_id), sqlc.arg(user_id),
        sqlc.narg(role_grant_id)::uuid, sqlc.narg(membership_id)::uuid);

-- name: DeleteMembershipByID :execrows
DELETE FROM memberships WHERE id = $1;

-- name: DeleteFederatedAssignment :exec
DELETE FROM federated_assignments WHERE id = $1;

-- name: DisableUser :execrows
UPDATE users SET disabled_at = now() WHERE id = $1 AND disabled_at IS NULL;

-- name: EnableUser :execrows
UPDATE users SET disabled_at = NULL WHERE id = $1 AND disabled_at IS NOT NULL;

-- name: RevokeAllUserSessions :execrows
UPDATE sessions SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL;

-- name: RevokeSessionsByID :execrows
UPDATE sessions SET revoked_at = now()
 WHERE user_id = sqlc.arg(user_id) AND id = ANY(sqlc.arg(ids)::uuid[]) AND revoked_at IS NULL;

-- name: RevokeUserTokens :execrows
-- Every personal access token a person holds, in every organisation.
UPDATE access_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL;

-- name: RevokeUserTokensIn :execrows
UPDATE access_tokens SET revoked_at = now()
 WHERE organization_id = $1 AND user_id = $2 AND revoked_at IS NULL;

-- name: CreateScimToken :one
INSERT INTO scim_tokens (id, organization_id, name, prefix, secret_hash, created_by)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetScimTokenByHash :one
SELECT * FROM scim_tokens
 WHERE secret_hash = $1 AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > now());

-- name: GetScimToken :one
SELECT * FROM scim_tokens WHERE id = $1;

-- name: TouchScimToken :exec
UPDATE scim_tokens SET last_used_at = now()
 WHERE id = $1 AND (last_used_at IS NULL OR last_used_at < now() - interval '1 minute');

-- name: ListScimTokens :many
SELECT * FROM scim_tokens
 WHERE organization_id = $1 AND (created_at, id) > (sqlc.arg(after_time)::timestamptz, sqlc.arg(after_id)::uuid)
 ORDER BY created_at, id
 LIMIT sqlc.arg(page_size);

-- name: ExpireScimToken :one
UPDATE scim_tokens SET expires_at = sqlc.arg(expires_at)
 WHERE id = sqlc.arg(id) AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > sqlc.arg(expires_at))
RETURNING *;

-- name: RevokeScimToken :execrows
UPDATE scim_tokens SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL;

-- name: CreateScimUser :one
INSERT INTO scim_users (organization_id, user_id, external_id, display_name, active, created_account)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetScimUserForUpdate :one
-- The record including a deleted one, so that a rehire revives it.
SELECT * FROM scim_users WHERE organization_id = $1 AND user_id = $2 FOR UPDATE;

-- name: UpdateScimUser :one
-- Sets the record; a record that was deleted is revived.
UPDATE scim_users
   SET external_id = $3, display_name = $4, active = $5, disabled_account = $6, deleted_at = NULL,
       version = version + 1, updated_at = now()
 WHERE organization_id = $1 AND user_id = $2
RETURNING *;

-- name: DeleteScimUser :execrows
UPDATE scim_users
   SET deleted_at = now(), active = false, version = version + 1, updated_at = now()
 WHERE organization_id = $1 AND user_id = $2 AND deleted_at IS NULL;

-- name: ListScimUsers :many
-- A page of the organisation's SCIM users joined to their accounts, by
-- optional equality on userName, externalId or displayName; total is the
-- number that match.
SELECT s.organization_id, s.user_id, s.external_id, s.display_name, s.active, s.created_account,
       s.disabled_account, s.version, s.created_at, s.updated_at,
       u.email, u.display_name AS account_name,
       count(*) OVER ()::bigint AS total
  FROM scim_users s
  JOIN users u ON u.id = s.user_id
 WHERE s.organization_id = sqlc.arg(organization_id) AND s.deleted_at IS NULL
   AND (sqlc.arg(attribute)::text = ''
        OR (sqlc.arg(attribute)::text = 'userName' AND lower(u.email) = lower(sqlc.arg(value)::text))
        OR (sqlc.arg(attribute)::text = 'externalId' AND s.external_id = sqlc.arg(value)::text)
        OR (sqlc.arg(attribute)::text = 'displayName' AND s.display_name = sqlc.arg(value)::text))
 ORDER BY s.created_at, s.user_id
 LIMIT sqlc.arg(page_size) OFFSET sqlc.arg(page_offset);

-- name: GetScimUserWithAccount :one
SELECT s.organization_id, s.user_id, s.external_id, s.display_name, s.active, s.created_account,
       s.disabled_account, s.deleted_at, s.version, s.created_at, s.updated_at,
       u.email, u.display_name AS account_name
  FROM scim_users s
  JOIN users u ON u.id = s.user_id
 WHERE s.organization_id = $1 AND s.user_id = $2 AND s.deleted_at IS NULL;

-- name: ListScimGroupRefsOfUser :many
SELECT t.id, t.name
  FROM memberships m
  JOIN scim_groups g ON g.team_id = m.team_id
  JOIN teams t ON t.id = g.team_id
 WHERE m.organization_id = $1 AND m.user_id = $2
 ORDER BY t.name, t.id;

-- name: CountOrganizationAccess :one
-- How many places a person still has access in: memberships, grants and app
-- access, in every organisation.
SELECT (SELECT count(*) FROM memberships m WHERE m.user_id = sqlc.arg(user_id)::uuid)
     + (SELECT count(*) FROM role_grants g WHERE g.user_id = sqlc.arg(user_id)::uuid)
     + (SELECT count(*) FROM app_access a WHERE a.user_id = sqlc.arg(user_id)::uuid) AS n;

-- name: DeleteMembershipsOfUserIn :execrows
-- Removes a person's direct access to one organisation: memberships (team
-- memberships too), grants to them and app access. Team grants stay with
-- the team.
DELETE FROM memberships WHERE organization_id = $1 AND user_id = $2;

-- name: DeleteGrantsOfUserIn :execrows
DELETE FROM role_grants WHERE organization_id = $1 AND user_id = $2;

-- name: DeleteAppAccessOfUserIn :execrows
DELETE FROM app_access WHERE organization_id = $1 AND user_id = $2;

-- name: CreateScimGroup :one
INSERT INTO scim_groups (organization_id, team_id, external_id)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetScimGroup :one
SELECT g.*, t.name AS display_name, t.created_at AS team_created_at
  FROM scim_groups g
  JOIN teams t ON t.id = g.team_id
 WHERE g.organization_id = $1 AND g.team_id = $2;

-- name: GetScimGroupForUpdate :one
SELECT * FROM scim_groups WHERE organization_id = $1 AND team_id = $2 FOR UPDATE;

-- name: UpdateScimGroup :one
UPDATE scim_groups SET external_id = $3, version = version + 1, updated_at = now()
 WHERE organization_id = $1 AND team_id = $2
RETURNING *;

-- name: ListScimGroups :many
SELECT g.organization_id, g.team_id, g.external_id, g.version, g.created_at, g.updated_at,
       t.name AS display_name, count(*) OVER ()::bigint AS total
  FROM scim_groups g
  JOIN teams t ON t.id = g.team_id
 WHERE g.organization_id = sqlc.arg(organization_id)
   AND (sqlc.arg(attribute)::text = ''
        OR (sqlc.arg(attribute)::text = 'externalId' AND g.external_id = sqlc.arg(value)::text)
        OR (sqlc.arg(attribute)::text = 'displayName' AND t.name = sqlc.arg(value)::text))
 ORDER BY g.created_at, g.team_id
 LIMIT sqlc.arg(page_size) OFFSET sqlc.arg(page_offset);

-- name: ListTeamMembersDetailed :many
SELECT m.user_id, u.email, u.display_name
  FROM memberships m
  JOIN users u ON u.id = m.user_id
 WHERE m.organization_id = $1 AND m.team_id = $2
 ORDER BY m.user_id;

-- name: DeleteScimGroup :execrows
DELETE FROM scim_groups WHERE organization_id = $1 AND team_id = $2;

-- name: UserHasAccessIn :one
-- Whether a person belongs to an organisation in any way: a membership, a
-- grant, app access or a SCIM record.
SELECT EXISTS (SELECT 1 FROM memberships m WHERE m.organization_id = sqlc.arg(organization_id)::uuid AND m.user_id = sqlc.arg(user_id)::uuid)
    OR EXISTS (SELECT 1 FROM role_grants g WHERE g.organization_id = sqlc.arg(organization_id)::uuid AND g.user_id = sqlc.arg(user_id)::uuid)
    OR EXISTS (SELECT 1 FROM app_access a WHERE a.organization_id = sqlc.arg(organization_id)::uuid AND a.user_id = sqlc.arg(user_id)::uuid)
    OR EXISTS (SELECT 1 FROM scim_users s WHERE s.organization_id = sqlc.arg(organization_id)::uuid AND s.user_id = sqlc.arg(user_id)::uuid) AS has_access;

-- name: SetUserEmail :execrows
UPDATE users SET email = $2 WHERE id = $1;

-- name: SetUserDisplayName :exec
UPDATE users SET display_name = $2 WHERE id = $1;

-- name: CountScimUsers :one
SELECT count(*) FROM scim_users s JOIN users u ON u.id = s.user_id
 WHERE s.organization_id = sqlc.arg(organization_id) AND s.deleted_at IS NULL
   AND (sqlc.arg(attribute)::text = ''
        OR (sqlc.arg(attribute)::text = 'userName' AND lower(u.email) = lower(sqlc.arg(value)::text))
        OR (sqlc.arg(attribute)::text = 'externalId' AND s.external_id = sqlc.arg(value)::text)
        OR (sqlc.arg(attribute)::text = 'displayName' AND s.display_name = sqlc.arg(value)::text));

-- name: CountScimGroups :one
SELECT count(*) FROM scim_groups g JOIN teams t ON t.id = g.team_id
 WHERE g.organization_id = sqlc.arg(organization_id)
   AND (sqlc.arg(attribute)::text = ''
        OR (sqlc.arg(attribute)::text = 'externalId' AND g.external_id = sqlc.arg(value)::text)
        OR (sqlc.arg(attribute)::text = 'displayName' AND t.name = sqlc.arg(value)::text));
