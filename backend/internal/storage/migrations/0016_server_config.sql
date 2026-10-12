-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Server-held configuration (DAT-090–DAT-097, ADR-0061). Every change
-- here adds a table: nothing existing is altered (DEP-030).

-- The configuration epoch of an environment. epoch rises with every
-- change to any value, live_epoch with every change to a live value,
-- and strict_epoch with every change to a strict live value, so that a
-- gateway call that carries the live version it used can be refused
-- when a strict value has changed since (DAT-096). The row is locked by
-- each change, which serialises the changes of one environment.
CREATE TABLE config_environments (
    environment_id  uuid        PRIMARY KEY REFERENCES environments (id) ON DELETE CASCADE,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    app_id          uuid        NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    epoch           bigint      NOT NULL DEFAULT 0,
    live_epoch      bigint      NOT NULL DEFAULT 0,
    strict_epoch    bigint      NOT NULL DEFAULT 0,
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT config_environments_epochs CHECK (
        epoch >= 0 AND live_epoch BETWEEN 0 AND epoch AND strict_epoch BETWEEN 0 AND live_epoch)
);
SELECT plux_tenant_policy('config_environments');

-- A configuration value of an environment. A sensitive value is not
-- kept in value: it is sealed with the envelope encryption of the
-- environment secrets (SEC-106) and only the server-bound tier may be
-- sensitive; a strict value is a live one.
CREATE TABLE config_values (
    environment_id  uuid        NOT NULL REFERENCES environments (id) ON DELETE CASCADE,
    key             text        NOT NULL,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    app_id          uuid        NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    tier            text        NOT NULL,
    value_type      text        NOT NULL,
    sensitive       boolean     NOT NULL DEFAULT false,
    strict          boolean     NOT NULL DEFAULT false,
    value           text        NOT NULL DEFAULT '',
    ciphertext      bytea,
    wrapped_key     bytea,
    key_id          text,
    version         bigint      NOT NULL DEFAULT 1,
    updated_by      uuid        REFERENCES users (id) ON DELETE SET NULL,
    updated_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (environment_id, key),
    CONSTRAINT config_values_key CHECK (key ~ '^[A-Za-z][A-Za-z0-9_]{0,63}$'),
    CONSTRAINT config_values_tier CHECK (tier IN ('server-bound', 'live')),
    CONSTRAINT config_values_type CHECK (value_type IN ('string', 'integer', 'decimal', 'boolean')),
    CONSTRAINT config_values_sensitive CHECK (NOT sensitive OR tier = 'server-bound'),
    CONSTRAINT config_values_strict CHECK (NOT strict OR tier = 'live'),
    CONSTRAINT config_values_sealed CHECK (
        (sensitive AND value = '' AND ciphertext IS NOT NULL AND wrapped_key IS NOT NULL AND key_id IS NOT NULL)
        OR (NOT sensitive AND ciphertext IS NULL))
);
SELECT plux_tenant_policy('config_values');
