-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- SCIM 2.0 provisioning (GOV-004, ADR-0074): the bearer tokens an
-- organisation's identity provider presents, and the SCIM view of the
-- people and groups it provisioned. Every table here is new, so a replica
-- of the previous version keeps working during a rolling upgrade (SRV-021).

-- A SCIM bearer token. Only the hash is stored (SEC-101); the token is shown
-- once, when it is created. Rotating makes a new token and gives the old one
-- an expires_at, so the identity provider can be switched over without a
-- gap.
CREATE TABLE scim_tokens (
    id              uuid        PRIMARY KEY,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name            text        NOT NULL,
    prefix          text        NOT NULL,
    secret_hash     bytea       NOT NULL UNIQUE,
    created_by      uuid        REFERENCES users (id) ON DELETE SET NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    -- expires_at is NULL until the token is rotated away.
    expires_at      timestamptz,
    revoked_at      timestamptz,
    last_used_at    timestamptz
);
CREATE INDEX scim_tokens_org ON scim_tokens (organization_id);

-- A token is found by its hash before its organisation is known, so the
-- authentication scope may read it, as for access_tokens.
ALTER TABLE scim_tokens ENABLE ROW LEVEL SECURITY;
ALTER TABLE scim_tokens FORCE ROW LEVEL SECURITY;
CREATE POLICY plux_tenant ON scim_tokens
    USING (organization_id = plux_current_organization() OR plux_in_scope('authentication'))
    WITH CHECK (organization_id = plux_current_organization());

-- The people an organisation's SCIM client provisioned or adopted. A person
-- is one account of the installation; this row is the organisation's SCIM
-- view of it. created_account records that SCIM made the account, which is
-- what lets it set the account's display name; disabled_account records that
-- deprovisioning disabled the account, which is what lets reactivation enable
-- it again. version is the resource's ETag.
CREATE TABLE scim_users (
    organization_id  uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    user_id          uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    external_id      text        NOT NULL DEFAULT '',
    display_name     text        NOT NULL DEFAULT '',
    active           boolean     NOT NULL DEFAULT true,
    created_account  boolean     NOT NULL DEFAULT false,
    disabled_account boolean     NOT NULL DEFAULT false,
    -- deleted_at marks a person the client deleted: the record is kept, so
    -- that a rehire can be told from a stranger, and is invisible to SCIM.
    deleted_at       timestamptz,
    version          bigint      NOT NULL DEFAULT 1,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, user_id)
);
CREATE UNIQUE INDEX scim_users_external
    ON scim_users (organization_id, external_id) WHERE external_id <> '' AND deleted_at IS NULL;
SELECT plux_tenant_policy('scim_users');

-- The teams SCIM manages as groups. A team an administrator made by hand is
-- not here and is invisible to SCIM.
CREATE TABLE scim_groups (
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    team_id         uuid        PRIMARY KEY REFERENCES teams (id) ON DELETE CASCADE,
    external_id     text        NOT NULL DEFAULT '',
    version         bigint      NOT NULL DEFAULT 1,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX scim_groups_external ON scim_groups (organization_id, external_id) WHERE external_id <> '';
SELECT plux_tenant_policy('scim_groups');

-- A person reads their own grants and app access in every organisation, as
-- they read their own memberships (0002): deprovisioning in one organisation
-- needs to know whether they still belong to another, and only reads.
CREATE POLICY plux_own ON role_grants FOR SELECT USING (user_id = plux_current_user());
CREATE POLICY plux_own ON app_access FOR SELECT USING (user_id = plux_current_user());
