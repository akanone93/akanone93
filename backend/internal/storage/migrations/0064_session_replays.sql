-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Structural session replays of a device (ANL-050, ADR-0101): the screens,
-- layout snapshots, taps and non-sensitive state changes of one foreground
-- stretch, redacted again on the server. Kept for replay.retentionDays and
-- erased with the device (SEC-162). Expand-only: a new table.
CREATE TABLE session_replays (
    id               uuid        PRIMARY KEY,
    organization_id  uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    app_id           uuid        NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    environment_id   uuid        NOT NULL REFERENCES environments (id) ON DELETE CASCADE,
    device_id        uuid        NOT NULL REFERENCES devices (id) ON DELETE CASCADE,
    started_at       timestamptz NOT NULL,
    duration_ms      integer     NOT NULL CHECK (duration_ms >= 0),
    event_count      integer     NOT NULL CHECK (event_count >= 0),
    bytes            integer     NOT NULL CHECK (bytes >= 0),
    release_sequence bigint      NOT NULL DEFAULT 0,
    truncated        boolean     NOT NULL DEFAULT false,
    route            text        NOT NULL DEFAULT '',
    events           jsonb       NOT NULL,
    created_at       timestamptz NOT NULL,
    expires_at       timestamptz NOT NULL,
    CONSTRAINT session_replays_events CHECK (jsonb_typeof(events) = 'array')
);
CREATE INDEX session_replays_list ON session_replays (environment_id, started_at DESC, id DESC);
CREATE INDEX session_replays_device ON session_replays (device_id, created_at);
CREATE INDEX session_replays_expiry ON session_replays (organization_id, expires_at);
SELECT plux_tenant_policy('session_replays');
SELECT plux_app_policy('session_replays');
