-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- The secure gateway, user identity and strong customer authentication
-- (SEC-026–SEC-032, ADR-0056). Every change here adds a table: nothing
-- existing is altered (DEP-030).

-- An API the gateway may reach for the devices of one environment
-- (SEC-031). Credentials are not stored here: auth holds the names of
-- environment secrets, which are sealed at rest (SEC-106), and the
-- non-secret settings of the authentication.
CREATE TABLE gateway_upstreams (
    id                      uuid        PRIMARY KEY,
    organization_id         uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    app_id                  uuid        NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    environment_id          uuid        NOT NULL REFERENCES environments (id) ON DELETE CASCADE,
    name                    text        NOT NULL,
    base_url                text        NOT NULL,
    auth_kind               text        NOT NULL DEFAULT 'none',
    auth                    jsonb       NOT NULL DEFAULT '{}',
    allow_private_addresses boolean     NOT NULL DEFAULT false,
    timeout_ms              integer     NOT NULL DEFAULT 0,
    allowed_methods         text[]      NOT NULL,
    requires_assurance      text        NOT NULL DEFAULT 'AL0',
    operations              jsonb       NOT NULL DEFAULT '[]',
    audience                text        NOT NULL DEFAULT '',
    -- version rises with every change, so a replica that cached the
    -- upstream's connections knows when to build them again.
    version                 bigint      NOT NULL DEFAULT 1,
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT gateway_upstreams_name CHECK (name ~ '^[a-z][a-zA-Z0-9]{0,63}$'),
    CONSTRAINT gateway_upstreams_auth_kind CHECK (auth_kind IN ('none', 'mtls', 'oauth_client_credentials', 'api_key')),
    CONSTRAINT gateway_upstreams_assurance CHECK (requires_assurance IN ('AL0', 'AL1', 'AL2', 'AL3')),
    CONSTRAINT gateway_upstreams_timeout CHECK (timeout_ms >= 0),
    CONSTRAINT gateway_upstreams_methods CHECK (cardinality(allowed_methods) >= 1),
    CONSTRAINT gateway_upstreams_operations CHECK (jsonb_typeof(operations) = 'array'),
    UNIQUE (environment_id, name)
);
CREATE INDEX gateway_upstreams_app ON gateway_upstreams (app_id, environment_id, name);
SELECT plux_tenant_policy('gateway_upstreams');

-- The public key of a device's strong customer authentication key, after
-- the server verified its attestation (SEC-027). One key per device;
-- enrolling again replaces it, as a change of enrolled biometrics
-- invalidates the old one.
CREATE TABLE device_sca_keys (
    device_id            uuid        PRIMARY KEY REFERENCES devices (id) ON DELETE CASCADE,
    organization_id      uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    public_key           bytea       NOT NULL,
    jkt                  text        NOT NULL,
    key_storage          text        NOT NULL,
    attestation_provider text        NOT NULL,
    enrolled_at          timestamptz NOT NULL DEFAULT now()
);
SELECT plux_tenant_policy('device_sca_keys');

-- The customer identity provider whose user tokens an environment's
-- devices exchange (SEC-026): at most one per environment.
CREATE TABLE user_identity_providers (
    environment_id  uuid        PRIMARY KEY REFERENCES environments (id) ON DELETE CASCADE,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    app_id          uuid        NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    issuer          text        NOT NULL,
    jwks_url        text        NOT NULL,
    audiences       text[]      NOT NULL,
    forward_claims  text[]      NOT NULL DEFAULT '{}',
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT user_identity_providers_audiences CHECK (cardinality(audiences) >= 1)
);
SELECT plux_tenant_policy('user_identity_providers');
