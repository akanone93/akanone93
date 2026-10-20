-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Alert rules on rollup metrics (ANL-030, ADR-0080) and grouped errors
-- (ANL-040, CMP-041). Expand-only: new tables, nothing existing changes.

-- An alert rule: an aggregation of one event name's rollups over a window,
-- compared with a threshold. The worker evaluates it every minute.
-- filters maps a rollup dimension to the value a series must have, such as
-- {"release": "42", "page": "checkout"}; environment_id narrows by
-- environment. revision counts changes.
CREATE TABLE alert_rules (
    id              uuid             PRIMARY KEY,
    organization_id uuid             NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    app_id          uuid             NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    name            text             NOT NULL,
    metric          text             NOT NULL,
    environment_id  uuid             REFERENCES environments (id) ON DELETE CASCADE,
    filters         jsonb            NOT NULL DEFAULT '{}',
    aggregation     text             NOT NULL,
    window_seconds  integer          NOT NULL,
    comparison      text             NOT NULL,
    threshold       double precision NOT NULL,
    for_seconds     integer          NOT NULL DEFAULT 0,
    severity        text             NOT NULL,
    enabled         boolean          NOT NULL DEFAULT true,
    revision        bigint           NOT NULL DEFAULT 1,
    created_by      text             NOT NULL,
    created_at      timestamptz      NOT NULL,
    updated_by      text             NOT NULL,
    updated_at      timestamptz      NOT NULL,
    CONSTRAINT alert_rules_aggregation CHECK (aggregation IN ('count', 'sum', 'avg', 'min', 'max', 'p50', 'p90', 'p99')),
    CONSTRAINT alert_rules_comparison CHECK (comparison IN ('>', '>=', '<', '<=')),
    CONSTRAINT alert_rules_severity CHECK (severity IN ('info', 'warning', 'critical')),
    CONSTRAINT alert_rules_window CHECK (window_seconds > 0 AND for_seconds >= 0)
);
CREATE UNIQUE INDEX alert_rules_name ON alert_rules (app_id, name);
CREATE INDEX alert_rules_org ON alert_rules (organization_id) WHERE enabled;
SELECT plux_tenant_policy('alert_rules');

-- The state of a rule: ok, pending (the condition holds, but not yet for
-- the for-duration), firing, or resolved (it fired and the condition
-- stopped). evaluated_at is the evaluation window last applied: an
-- evaluation at or before it is a repeat and changes nothing, which is
-- what keeps two evaluators from firing twice.
CREATE TABLE alert_states (
    rule_id         uuid             PRIMARY KEY REFERENCES alert_rules (id) ON DELETE CASCADE,
    organization_id uuid             NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    state           text             NOT NULL,
    since           timestamptz      NOT NULL,
    evaluated_at    timestamptz      NOT NULL,
    value           double precision,
    fired_at        timestamptz,
    resolved_at     timestamptz,
    CONSTRAINT alert_states_state CHECK (state IN ('ok', 'pending', 'firing', 'resolved'))
);
CREATE INDEX alert_states_org ON alert_states (organization_id, state);
SELECT plux_tenant_policy('alert_states');

-- A group of errors with one fingerprint in one release of an app. The
-- fingerprint is computed at ingestion; json_path and source_file are filled
-- in lazily, the first time the group is read, from the release's retained
-- source map (symbolicated_at says it was tried). node_path is where the
-- runtime reported the error; Studio (P11) opens the node by it.
CREATE TABLE error_groups (
    id               uuid        PRIMARY KEY,
    organization_id  uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    app_id           uuid        NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    release_sequence bigint      NOT NULL,
    fingerprint      text        NOT NULL,
    code             text        NOT NULL,
    plugin_key       text        NOT NULL,
    route            text        NOT NULL,
    node_path        text        NOT NULL,
    frames           text        NOT NULL,
    fatal            boolean     NOT NULL,
    json_path        text        NOT NULL DEFAULT '',
    source_file      text        NOT NULL DEFAULT '',
    symbolicated_at  timestamptz,
    first_seen       timestamptz NOT NULL,
    last_seen        timestamptz NOT NULL,
    events           bigint      NOT NULL DEFAULT 0,
    devices          bigint      NOT NULL DEFAULT 0,
    UNIQUE (app_id, release_sequence, fingerprint)
);
CREATE INDEX error_groups_seen ON error_groups (app_id, last_seen DESC);
SELECT plux_tenant_policy('error_groups');

-- The devices a group has seen, so the affected devices are counted once
-- each. The identifier never leaves the server.
CREATE TABLE error_group_devices (
    group_id        uuid NOT NULL REFERENCES error_groups (id) ON DELETE CASCADE,
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    device_id       uuid NOT NULL,
    PRIMARY KEY (group_id, device_id)
);
SELECT plux_tenant_policy('error_group_devices');

-- Events per group and UTC day: the trend.
CREATE TABLE error_group_days (
    group_id        uuid   NOT NULL REFERENCES error_groups (id) ON DELETE CASCADE,
    organization_id uuid   NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    day             date   NOT NULL,
    events          bigint NOT NULL,
    PRIMARY KEY (group_id, day)
);
SELECT plux_tenant_policy('error_group_days');

-- The newest events of a group, kept so a developer can see what one looked
-- like. The primary key is also what makes recording idempotent: an event
-- delivered twice is counted once. fields holds the event's flat scalars,
-- never a payload (Appendix G.2).
CREATE TABLE error_samples (
    group_id        uuid        NOT NULL REFERENCES error_groups (id) ON DELETE CASCADE,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    event_id        uuid        NOT NULL,
    occurred_at     timestamptz NOT NULL,
    fields          jsonb       NOT NULL,
    PRIMARY KEY (group_id, event_id)
);
CREATE INDEX error_samples_recent ON error_samples (group_id, occurred_at DESC);
SELECT plux_tenant_policy('error_samples');
