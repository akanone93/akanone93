-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Unfinished chunked uploads (P11 S2.6, SRV-060, plan §4.3 #2). Expand-only:
-- one new table. A row holds the bytes received so far, which only grow at
-- the offset received; it is deleted at finish and ignored after expires_at
-- (asset.uploadSessionTtl), when the next begin sweeps it.
CREATE TABLE upload_sessions (
    id              uuid        PRIMARY KEY,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    user_id         uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    kind            text        NOT NULL DEFAULT 'asset',
    app_id          uuid        NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    plugin_id       text        NOT NULL DEFAULT '',
    editor_session  text        NOT NULL DEFAULT '',
    name            text        NOT NULL,
    size            bigint      NOT NULL,
    sha256          bytea       NOT NULL,
    received        bigint      NOT NULL DEFAULT 0,
    data            bytea       NOT NULL DEFAULT ''::bytea,
    created_at      timestamptz NOT NULL DEFAULT now(),
    expires_at      timestamptz NOT NULL,
    CONSTRAINT upload_sessions_kind CHECK (kind IN ('asset')),
    CONSTRAINT upload_sessions_progress CHECK (received >= 0 AND received <= size)
);
CREATE INDEX upload_sessions_expiry ON upload_sessions (expires_at);

ALTER TABLE upload_sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE upload_sessions FORCE ROW LEVEL SECURITY;
CREATE POLICY plux_tenant ON upload_sessions
    USING (organization_id = plux_current_organization())
    WITH CHECK (organization_id = plux_current_organization());

SELECT plux_app_policy('upload_sessions');
