-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Dev sessions and pairing (DEV-002, DEV-003, ADR-0087). Expand-only: three
-- new tables.
--
-- No credential is stored: a pairing token and a socket ticket are kept as
-- the SHA-256 of their text, which is enough to recognise one presented and
-- useless to anyone who reads the table.
--
-- A device presents its socket ticket at the WebSocket upgrade before its
-- organisation is known, so the 'devsocket' scope lets dev_session_devices
-- be read by ticket; the transaction is then rescoped to the organisation of
-- the row it found, as the 'hook' scope does for function_hooks.

-- dev_sessions holds the dev sessions of an organisation. A session lives
-- until ended_at is set, expires_at passes, or it sees no traffic for the
-- limit devSession.idleMinutes after last_activity_at (PLX-9200).
CREATE TABLE dev_sessions (
    id               uuid        PRIMARY KEY,
    organization_id  uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    app_id           uuid        NOT NULL,
    environment_id   uuid        NOT NULL,
    environment_key  text        NOT NULL,
    created_by_user  text        NOT NULL,
    created_by_kind  text        NOT NULL,
    created_by_id    text        NOT NULL,
    created_by       text        NOT NULL,
    created_at       timestamptz NOT NULL,
    expires_at       timestamptz NOT NULL,
    last_activity_at timestamptz NOT NULL,
    ended_at         timestamptz,
    CONSTRAINT dev_sessions_lifetime CHECK (created_at < expires_at)
);
CREATE INDEX dev_sessions_live ON dev_sessions (organization_id, expires_at) WHERE ended_at IS NULL;
SELECT plux_tenant_policy('dev_sessions');
SELECT plux_app_policy('dev_sessions');

-- dev_pairing_tokens holds the pairing tokens issued for a session, by
-- hash. A token is consumed once (consumed_at), and never after expires_at
-- (DEV-002).
CREATE TABLE dev_pairing_tokens (
    token_hash       bytea       PRIMARY KEY,
    organization_id  uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    session_id       uuid        NOT NULL REFERENCES dev_sessions (id) ON DELETE CASCADE,
    expires_at       timestamptz NOT NULL,
    consumed_at      timestamptz,
    CONSTRAINT dev_pairing_tokens_hash CHECK (length(token_hash) = 32)
);
CREATE INDEX dev_pairing_tokens_session ON dev_pairing_tokens (session_id);
SELECT plux_tenant_policy('dev_pairing_tokens');

-- dev_session_devices holds the devices paired with a session, each bound to
-- the thumbprint of the DPoP key it paired with (SEC-021). ticket_hash is
-- the socket ticket the device presents once at the WebSocket upgrade; it is
-- cleared when used.
CREATE TABLE dev_session_devices (
    id                uuid        PRIMARY KEY,
    organization_id   uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    session_id        uuid        NOT NULL REFERENCES dev_sessions (id) ON DELETE CASCADE,
    device_id         text        NOT NULL,
    dpop_thumbprint   text        NOT NULL,
    name              text        NOT NULL DEFAULT '',
    platform          text        NOT NULL DEFAULT '',
    runtime_version   text        NOT NULL DEFAULT '',
    paired_at         timestamptz NOT NULL,
    revoked_at        timestamptz,
    ticket_hash       bytea       UNIQUE,
    ticket_expires_at timestamptz,
    CONSTRAINT dev_session_devices_ticket CHECK ((ticket_hash IS NULL) = (ticket_expires_at IS NULL)),
    CONSTRAINT dev_session_devices_ticket_hash CHECK (ticket_hash IS NULL OR length(ticket_hash) = 32)
);
CREATE INDEX dev_session_devices_session ON dev_session_devices (session_id, paired_at, id);
SELECT plux_tenant_policy('dev_session_devices');
CREATE POLICY plux_devsocket ON dev_session_devices FOR SELECT USING (plux_in_scope('devsocket'));
