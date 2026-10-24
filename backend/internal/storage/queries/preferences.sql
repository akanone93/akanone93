-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- name: GetUserPreferences :one
SELECT preferences FROM user_preferences WHERE organization_id = $1 AND user_id = $2;

-- name: UpsertUserPreferences :exec
INSERT INTO user_preferences (organization_id, user_id, preferences, updated_at)
VALUES ($1, $2, $3, now())
ON CONFLICT (organization_id, user_id) DO UPDATE SET preferences = EXCLUDED.preferences, updated_at = now();

-- The facts the onboarding steps derive from (STU-015), within the
-- organisation of the transaction.
-- name: OnboardingFacts :one
SELECT
    EXISTS (SELECT 1 FROM apps a WHERE a.organization_id = $1 AND a.deleted_at IS NULL)::boolean AS has_app,
    EXISTS (SELECT 1 FROM documents d WHERE d.organization_id = $1)::boolean AS has_document,
    EXISTS (SELECT 1 FROM dev_session_devices v WHERE v.organization_id = $1)::boolean AS has_device,
    EXISTS (SELECT 1 FROM releases r WHERE r.organization_id = $1)::boolean AS has_release,
    ((SELECT count(DISTINCT m.user_id) FROM memberships m WHERE m.organization_id = $1) > 1)::boolean AS has_members;
