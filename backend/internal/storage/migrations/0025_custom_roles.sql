-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Custom roles (GOV-002, ADR-0073): a role an organisation composes from
-- the permission catalogue. permissions holds catalogue names, bare or
-- limited to an environment as <permission>:<environment>; the server
-- checks them against the catalogue and against what the creator holds
-- (PLX-8101), because the catalogue lives in code. A custom role's key
-- never equals a predefined role's, so a grant's role is unambiguous.
-- The table is new and nothing existing is altered (SRV-021).
CREATE TABLE custom_roles (
    id              uuid        PRIMARY KEY,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    key             text        NOT NULL,
    name            text        NOT NULL,
    description     text        NOT NULL DEFAULT '',
    permissions     text[]      NOT NULL,
    created_by      uuid        REFERENCES users (id) ON DELETE SET NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, key),
    CONSTRAINT custom_roles_key_form CHECK (key ~ '^[a-z][a-z0-9]*(-[a-z0-9]+)*$' AND length(key) <= 64),
    CONSTRAINT custom_roles_not_predefined CHECK (key NOT IN (
        'owner', 'admin', 'release-manager', 'developer', 'designer', 'translator',
        'reviewer', 'compliance-officer', 'auditor', 'viewer'))
);
SELECT plux_tenant_policy('custom_roles');
