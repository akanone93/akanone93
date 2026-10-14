-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Logs and invocation records of Plux Functions (FN-032). Every change here
-- adds a table: nothing existing is altered (DEP-030). Every table carries
-- its organisation and is bound to it by row-level security (SRV-022). Rows
-- are kept for the telemetry retention and deleted by the maintenance sweep.

-- A line a server function logged. The runner bounds the lines of one
-- invocation by function.logBytes; invocation_id groups them.
CREATE TABLE function_logs (
    id              bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    app_id          uuid        NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    environment_id  uuid        NOT NULL REFERENCES environments (id) ON DELETE CASCADE,
    function        text        NOT NULL,
    version         integer     NOT NULL,
    invocation_id   uuid        NOT NULL,
    level           text        NOT NULL,
    message         text        NOT NULL,
    trace_id        text        NOT NULL DEFAULT '',
    logged_at       timestamptz NOT NULL,
    CONSTRAINT function_logs_version CHECK (version > 0),
    CONSTRAINT function_logs_level CHECK (level IN ('debug', 'info', 'warn', 'error'))
);
SELECT plux_tenant_policy('function_logs');
CREATE INDEX function_logs_lookup ON function_logs (environment_id, function, logged_at DESC, id DESC);
CREATE INDEX function_logs_expiry ON function_logs (organization_id, logged_at);

-- One row per run of a server function, or per replay of a stored result
-- (FN-021): the source of the per-function metrics the API reports. The
-- row holds no input, output, user or device.
CREATE TABLE function_invocations (
    id                bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id   uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    app_id            uuid        NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    environment_id    uuid        NOT NULL REFERENCES environments (id) ON DELETE CASCADE,
    function          text        NOT NULL,
    version           integer     NOT NULL,
    result            text        NOT NULL,
    duration_micros   bigint      NOT NULL,
    memory_peak_bytes bigint      NOT NULL DEFAULT 0,
    replayed          boolean     NOT NULL DEFAULT false,
    finished_at       timestamptz NOT NULL,
    CONSTRAINT function_invocations_version CHECK (version > 0),
    CONSTRAINT function_invocations_duration CHECK (duration_micros >= 0 AND memory_peak_bytes >= 0)
);
SELECT plux_tenant_policy('function_invocations');
CREATE INDEX function_invocations_lookup ON function_invocations (environment_id, function, finished_at);
CREATE INDEX function_invocations_expiry ON function_invocations (organization_id, finished_at);
