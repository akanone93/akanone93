-- name: CreateRestriction :one
INSERT INTO restriction_policies (id, organization_id, subject_kind, team_id, user_id, patterns,
                                  app_id, environment, reason, created_by)
VALUES ($1, $2, $3, sqlc.narg(team_id)::uuid, sqlc.narg(user_id)::uuid, $4,
        sqlc.narg(app_id)::uuid, sqlc.narg(environment)::text, $5, sqlc.narg(created_by)::uuid)
RETURNING *;

-- name: GetRestriction :one
SELECT * FROM restriction_policies WHERE id = $1;

-- name: DeleteRestriction :execrows
DELETE FROM restriction_policies WHERE id = $1;

-- name: ListRestrictions :many
SELECT * FROM restriction_policies
 WHERE organization_id = $1
   AND id > sqlc.arg(after_id)::uuid
   AND (sqlc.narg(subject_id)::uuid IS NULL OR subject_id = sqlc.narg(subject_id)::uuid)
   AND (sqlc.narg(app_id)::uuid IS NULL OR app_id = sqlc.narg(app_id)::uuid)
 ORDER BY id
 LIMIT sqlc.arg(page_size);

-- name: CountRestrictions :one
SELECT count(*) FROM restriction_policies WHERE organization_id = $1;

-- name: CountRestrictionsOfSubject :one
-- The restrictions one subject has: the organisation's own, when subject_id
-- is NULL, or a team's or a person's.
SELECT count(*) FROM restriction_policies
 WHERE organization_id = $1
   AND subject_id IS NOT DISTINCT FROM sqlc.narg(subject_id)::uuid;

-- name: ListRestrictionsForPerson :many
-- The restrictions that apply to a person, in identifier order: the
-- organisation's, those of the teams the person belongs to, and their own.
-- With no person (a workload identity) only the organisation's apply.
SELECT * FROM restriction_policies r
 WHERE r.organization_id = $1
   AND (r.subject_kind = 'organization'
        OR (sqlc.narg(user_id)::uuid IS NOT NULL
            AND (r.user_id = sqlc.narg(user_id)::uuid
                 OR r.team_id IN (SELECT m.team_id FROM memberships m
                                   WHERE m.organization_id = $1 AND m.user_id = sqlc.narg(user_id)::uuid
                                     AND m.team_id IS NOT NULL))))
 ORDER BY r.id;

-- name: ListOwnerUserIDs :many
-- The people who are owners of the organisation, as CountOwners counts them.
SELECT m.user_id FROM memberships m
 WHERE m.organization_id = $1 AND m.team_id IS NULL AND m.role = 'owner'
UNION
SELECT g.user_id FROM role_grants g
 WHERE g.organization_id = $1 AND g.scope = 'organization' AND g.role = 'owner' AND g.user_id IS NOT NULL
UNION
SELECT m.user_id FROM role_grants g
  JOIN memberships m ON m.organization_id = g.organization_id AND m.team_id = g.team_id
 WHERE g.organization_id = $1 AND g.scope = 'organization' AND g.role = 'owner'
ORDER BY 1;

-- name: GetPermissionsVersion :one
SELECT permissions_version FROM organizations WHERE id = $1;
