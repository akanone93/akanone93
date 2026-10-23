-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Role grants (GOV-002, GOV-003, ADR-0073): a grant is (principal, role,
-- scope). The principal is a user or a team; the role is a predefined role
-- or the key of a custom role (0025); the scope is the organisation, one
-- app, or one environment of an app. The table is new and nothing existing
-- is altered, so a replica of the previous version keeps working during a
-- rolling upgrade (SRV-021).
--
-- Teams and their members already exist (0002): a team is a row of teams
-- and a team membership a row of memberships with a team_id. The roles
-- people hold today stay where they are and mean the same grants: a
-- direct membership's role is a grant on the organisation, and an
-- app_access row is a grant on one app. The resolver reads all three
-- sources, so no one gains or loses access when this migration runs.
CREATE TABLE role_grants (
    id              uuid        PRIMARY KEY,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    user_id         uuid        REFERENCES users (id) ON DELETE CASCADE,
    team_id         uuid        REFERENCES teams (id) ON DELETE CASCADE,
    role            text        NOT NULL,
    scope           text        NOT NULL,
    app_id          uuid        REFERENCES apps (id) ON DELETE CASCADE,
    environment_id  uuid        REFERENCES environments (id) ON DELETE CASCADE,
    created_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT role_grants_holder CHECK ((user_id IS NULL) <> (team_id IS NULL)),
    CONSTRAINT role_grants_role_form CHECK (role ~ '^[a-z][a-z0-9]*(-[a-z0-9]+)*$'),
    CONSTRAINT role_grants_scope CHECK (
        (scope = 'organization' AND app_id IS NULL AND environment_id IS NULL)
        OR (scope = 'app' AND app_id IS NOT NULL AND environment_id IS NULL)
        OR (scope = 'environment' AND app_id IS NOT NULL AND environment_id IS NOT NULL)
    )
);

-- A holder has a role at a scope once.
CREATE UNIQUE INDEX role_grants_once ON role_grants (
    organization_id,
    COALESCE(user_id, team_id),
    role,
    scope,
    COALESCE(app_id, '00000000-0000-0000-0000-000000000000'::uuid),
    COALESCE(environment_id, '00000000-0000-0000-0000-000000000000'::uuid)
);
CREATE INDEX role_grants_user ON role_grants (user_id) WHERE user_id IS NOT NULL;
CREATE INDEX role_grants_team ON role_grants (team_id) WHERE team_id IS NOT NULL;
CREATE INDEX role_grants_role ON role_grants (organization_id, role);
SELECT plux_tenant_policy('role_grants');
