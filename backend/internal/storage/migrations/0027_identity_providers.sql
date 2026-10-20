-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Identity federation (GOV-004, ADR-0074): several OpenID Connect and SAML
-- providers per installation and per organisation, a group mapping document
-- per provider, and the records that let a sign-in be traced, mapped and
-- ended by the provider. Every table here is new and the one column added
-- to an existing table is nullable, so a replica of the previous version
-- keeps working during a rolling upgrade (SRV-021).

-- An identity provider. organization_id is NULL for a provider of the whole
-- installation and set for a provider only that organisation's people sign
-- in with. config holds what is not secret (issuer, client identifier,
-- redirect URL, the SAML provider's descriptor); secret holds what is
-- sealed with envelope encryption bound to the provider's identifier
-- (SEC-106): an OpenID client secret, or a SAML service provider's private
-- key. mapping is the group mapping document (GOV-004): rules from a group
-- the provider asserts to the roles and teams its members hold.
CREATE TABLE identity_providers (
    id               uuid        PRIMARY KEY,
    organization_id  uuid        REFERENCES organizations (id) ON DELETE CASCADE,
    key              text        NOT NULL,
    kind             text        NOT NULL,
    name             text        NOT NULL,
    enabled          boolean     NOT NULL DEFAULT true,
    -- jit_provisioning creates an account for a person the provider vouches
    -- for and no account exists for. It is off unless an administrator turns
    -- it on, and a person created this way holds only what the mapping gives.
    jit_provisioning boolean     NOT NULL DEFAULT false,
    config           jsonb       NOT NULL,
    secret           bytea,
    mapping          jsonb       NOT NULL DEFAULT '{"rules": []}'::jsonb,
    created_by       uuid        REFERENCES users (id) ON DELETE SET NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT identity_providers_key_form CHECK (key ~ '^[a-z0-9]([a-z0-9-]{0,62}[a-z0-9])?$'),
    CONSTRAINT identity_providers_kind CHECK (kind IN ('oidc', 'saml'))
);

-- A key names one provider of an organisation, or of the installation.
CREATE UNIQUE INDEX identity_providers_key
    ON identity_providers (COALESCE(organization_id, '00000000-0000-0000-0000-000000000000'::uuid), key);

-- Row-level security: an organisation sees its own providers, and code that
-- declared the installation scope (the sign-in, never a caller's behalf)
-- sees them all, because a person signs in before their organisation is
-- known.
ALTER TABLE identity_providers ENABLE ROW LEVEL SECURITY;
ALTER TABLE identity_providers FORCE ROW LEVEL SECURITY;
CREATE POLICY plux_tenant ON identity_providers
    USING (organization_id = plux_current_organization() OR plux_in_scope('installation'))
    WITH CHECK (organization_id = plux_current_organization() OR plux_in_scope('installation'));

-- Which provider a sign-in in progress belongs to; NULL for the provider in
-- the configuration file.
ALTER TABLE oidc_logins ADD COLUMN provider_id uuid REFERENCES identity_providers (id) ON DELETE CASCADE;

-- A SAML sign-in in progress: the hash of the relay state and the
-- identifier of the AuthnRequest the response must answer.
CREATE TABLE saml_logins (
    state_hash  bytea       PRIMARY KEY,
    provider_id uuid        NOT NULL REFERENCES identity_providers (id) ON DELETE CASCADE,
    request_id  text        NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    expires_at  timestamptz NOT NULL
);

-- Each sign-in through a provider: whom, with which issuer and subject, the
-- provider's session identifier (sid) and the authentication context and
-- methods it reported (acr, amr). A back-channel logout names an issuer and
-- a sid or a subject, and ends the sessions these rows lead to. session_id is
-- empty when the sign-in is waiting for a second factor.
CREATE TABLE federated_sign_ins (
    id           uuid        PRIMARY KEY,
    user_id      uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    provider_id  uuid        REFERENCES identity_providers (id) ON DELETE SET NULL,
    issuer       text        NOT NULL,
    subject      text        NOT NULL,
    sid          text        NOT NULL DEFAULT '',
    acr          text        NOT NULL DEFAULT '',
    amr          text[]      NOT NULL DEFAULT '{}',
    session_id   uuid        REFERENCES sessions (id) ON DELETE SET NULL,
    signed_in_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX federated_sign_ins_user ON federated_sign_ins (user_id, signed_in_at);
CREATE INDEX federated_sign_ins_sid ON federated_sign_ins (issuer, sid) WHERE sid <> '';
CREATE INDEX federated_sign_ins_subject ON federated_sign_ins (issuer, subject);

-- The grants and team memberships a provider's mapping gave a person. They
-- are ordinary role_grants and memberships rows; this table remembers which
-- of them the provider owns, so the next sign-in can remove what the person
-- no longer qualifies for without touching anything an administrator
-- granted by hand. Deleting the grant or the membership deletes the row.
CREATE TABLE federated_assignments (
    id              uuid        PRIMARY KEY,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    provider_id     uuid        NOT NULL REFERENCES identity_providers (id) ON DELETE CASCADE,
    user_id         uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role_grant_id   uuid        REFERENCES role_grants (id) ON DELETE CASCADE,
    membership_id   uuid        REFERENCES memberships (id) ON DELETE CASCADE,
    created_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT federated_assignments_one CHECK ((role_grant_id IS NULL) <> (membership_id IS NULL))
);
CREATE INDEX federated_assignments_holder ON federated_assignments (provider_id, user_id);

-- Tenant data, with the installation scope admitted: a sign-in finds every
-- organisation in which the provider has given a person something before it
-- visits each of them in turn.
ALTER TABLE federated_assignments ENABLE ROW LEVEL SECURITY;
ALTER TABLE federated_assignments FORCE ROW LEVEL SECURITY;
CREATE POLICY plux_tenant ON federated_assignments
    USING (organization_id = plux_current_organization() OR plux_in_scope('installation'))
    WITH CHECK (organization_id = plux_current_organization());
