-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- name: InsertApprovalPolicy :one
INSERT INTO approval_policies (id, organization_id, app_id, key, version, document, document_hash, deleted,
                               created_by_kind, created_by_id, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: LockApprovalPolicies :exec
-- Serialises the writers of one app's policies, so that two numbers the
-- same version.
SELECT pg_advisory_xact_lock(hashtextextended('approval-policies:' || sqlc.arg(app_id)::text, 0));

-- name: GetLatestApprovalPolicy :one
SELECT * FROM approval_policies WHERE app_id = $1 AND key = $2 ORDER BY version DESC LIMIT 1;

-- name: ListLatestApprovalPolicies :many
-- The policy in force for each key of an app, deleted ones included.
SELECT DISTINCT ON (key) * FROM approval_policies WHERE app_id = $1 ORDER BY key, version DESC;

-- name: ListApprovalPolicyVersions :many
SELECT * FROM approval_policies WHERE app_id = $1 AND key = $2 ORDER BY version DESC LIMIT sqlc.arg(page_size);

-- name: InsertApprovalRequest :one
INSERT INTO approval_requests (id, organization_id, app_id, environment_id, environment_key, production, resource, action,
                               subject, content_hash, summary, risk, payload, policy_keys, approvals_required, author_id,
                               requested_by_kind, requested_by_id, requested_by, created_at, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
ON CONFLICT DO NOTHING
RETURNING *;

-- name: SupersedeApprovalRequests :execrows
-- Retires the live requests of a subject that name other content: their
-- approvals no longer count (GOV-022).
UPDATE approval_requests SET status = 'superseded'
 WHERE organization_id = $1 AND app_id = $2 AND resource = $3 AND action = sqlc.arg(action)::text AND subject = $4
   AND content_hash <> sqlc.arg(content_hash)::text AND status IN ('open', 'approved');

-- name: GetApprovalRequest :one
SELECT * FROM approval_requests WHERE id = $1;

-- name: GetApprovalRequestForUpdate :one
SELECT * FROM approval_requests WHERE id = $1 FOR UPDATE;

-- name: FindLiveApprovalRequest :one
SELECT * FROM approval_requests
 WHERE organization_id = $1 AND app_id = $2 AND resource = $3 AND action = sqlc.arg(action)::text AND subject = $4
   AND content_hash = $5 AND status IN ('open', 'approved');

-- name: LatestApprovalRequestForSubject :one
-- The newest request of a subject whatever it names, to tell a changed
-- artifact from one never asked about.
SELECT * FROM approval_requests
 WHERE organization_id = $1 AND app_id = $2 AND resource = $3 AND action = sqlc.arg(action)::text AND subject = $4
 ORDER BY created_at DESC, id DESC LIMIT 1;

-- name: ListApprovalRequests :many
SELECT * FROM approval_requests
 WHERE app_id = $1
   AND (sqlc.arg(status)::text = '' OR status = sqlc.arg(status)::text)
   AND (created_at, id) < (sqlc.arg(before_time)::timestamptz, sqlc.arg(before_id)::uuid)
 ORDER BY created_at DESC, id DESC
 LIMIT sqlc.arg(page_size);

-- name: SetApprovalRequestStatus :exec
UPDATE approval_requests SET status = $2 WHERE id = $1;

-- name: UseApprovalRequest :execrows
-- Spends a request once: a second change on the same approval finds nothing
-- to update.
UPDATE approval_requests SET status = 'used', used_at = $2, used_by = $3
 WHERE id = $1 AND status IN ('open', 'approved');

-- name: InsertApprovalDecision :exec
INSERT INTO approval_decisions (id, request_id, organization_id, decider_id, decider, roles, teams, decision, comment,
                                content_hash, decided_at, audit_sequence, payload)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13);

-- name: ListApprovalDecisions :many
SELECT * FROM approval_decisions WHERE request_id = $1 ORDER BY decided_at, id;

-- name: ListUnsignedApprovalDecisions :many
SELECT * FROM approval_decisions WHERE signature IS NULL ORDER BY decided_at, id LIMIT sqlc.arg(page_size);

-- name: SignApprovalDecision :execrows
UPDATE approval_decisions SET key_id = $2, signature = $3 WHERE id = $1 AND signature IS NULL;

-- name: ListApproverRoles :many
-- The role keys a user holds in an organisation, directly, on an app or
-- in a grant, personally or through a team: what a policy's roles match.
SELECT m.role FROM memberships m
 WHERE m.organization_id = $1 AND m.user_id = sqlc.arg(user_id)::uuid AND m.team_id IS NULL
UNION
SELECT a.role FROM app_access a
 WHERE a.organization_id = $1
   AND (a.user_id = sqlc.arg(user_id)::uuid
        OR a.team_id IN (SELECT t.team_id FROM memberships t
                          WHERE t.organization_id = $1 AND t.user_id = sqlc.arg(user_id)::uuid AND t.team_id IS NOT NULL))
UNION
SELECT g.role FROM role_grants g
 WHERE g.organization_id = $1
   AND (g.user_id = sqlc.arg(user_id)::uuid
        OR g.team_id IN (SELECT t.team_id FROM memberships t
                          WHERE t.organization_id = $1 AND t.user_id = sqlc.arg(user_id)::uuid AND t.team_id IS NOT NULL));

-- name: ListApproverTeams :many
SELECT t.team_id FROM memberships t
 WHERE t.organization_id = $1 AND t.user_id = $2 AND t.team_id IS NOT NULL
 ORDER BY t.team_id;

-- name: ApprovalInstallBase :many
-- The devices of an environment by the release sequence they hold and
-- their host build, for the review of a change (GOV-023).
SELECT installed_sequence, host_build, count(*)::bigint AS devices
  FROM devices WHERE environment_id = $1
 GROUP BY installed_sequence, host_build
 ORDER BY installed_sequence DESC, host_build;

-- name: ListApprovalAppEnvironments :many
SELECT id FROM environments WHERE app_id = $1;

-- name: ApprovalChannels :many
SELECT key, release_sequence FROM channels WHERE environment_id = $1 ORDER BY key;
