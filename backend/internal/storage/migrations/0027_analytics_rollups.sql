-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Analytics storage, part two: rollups (ANL-010, ADR-0080). One row is one
-- event name in one time bucket of one combination of dimensions: how many
-- events, and for events that carry a duration, their sum, minimum, maximum
-- and a fixed-bucket histogram from which p50, p90 and p99 are estimated.
-- Rollups are kept indefinitely unless the organisation sets otherwise
-- (Q17); hourly and daily rows are folded from the five-minute rows, so
-- they survive the deletion of the raw events.

-- plux_event_value is the duration in milliseconds an event carries, or
-- NULL: render_perf has build_ms, the rest duration_ms (Appendix G.2).
CREATE FUNCTION plux_event_value(name text, fields jsonb) RETURNS double precision
    LANGUAGE sql IMMUTABLE PARALLEL SAFE
    AS $$
        SELECT CASE
            WHEN jsonb_typeof(fields -> CASE WHEN name = 'render_perf' THEN 'build_ms' ELSE 'duration_ms' END) = 'number'
            THEN (fields ->> CASE WHEN name = 'render_perf' THEN 'build_ms' ELSE 'duration_ms' END)::double precision
        END
    $$;

-- plux_value_bucket is the histogram bucket of a value, 0 to 15: the number
-- of bounds at or below it. The bounds are 1, 2, 5, 10, 20, 50, 100, 200,
-- 500, 1000, 2000, 5000, 10000, 30000 and 60000 ms, so bucket 0 is below
-- 1 ms and bucket 15 is 60000 ms and above.
CREATE FUNCTION plux_value_bucket(v double precision) RETURNS integer
    LANGUAGE sql IMMUTABLE PARALLEL SAFE
    AS $$
        SELECT width_bucket(v, ARRAY[1, 2, 5, 10, 20, 50, 100, 200, 500, 1000, 2000, 5000, 10000, 30000, 60000]::double precision[])
    $$;

CREATE TABLE analytics_rollups (
    organization_id  uuid             NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    app_id           uuid             NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    environment_id   uuid             NOT NULL REFERENCES environments (id) ON DELETE CASCADE,
    granularity      text             NOT NULL,
    event_name       text             NOT NULL,
    bucket           timestamptz      NOT NULL,
    release_sequence bigint           NOT NULL,
    plugin_key       text             NOT NULL,
    page             text             NOT NULL,
    device_class     text             NOT NULL,
    locale           text             NOT NULL,
    events           bigint           NOT NULL,
    value_count      bigint           NOT NULL,
    value_sum        double precision NOT NULL,
    value_min        double precision NOT NULL,
    value_max        double precision NOT NULL,
    histogram        bigint[]         NOT NULL,
    PRIMARY KEY (app_id, environment_id, granularity, event_name, bucket, release_sequence, plugin_key, page, device_class, locale),
    CONSTRAINT analytics_rollups_granularity CHECK (granularity IN ('5m', '1h', '1d')),
    CONSTRAINT analytics_rollups_histogram CHECK (cardinality(histogram) = 16)
);
CREATE INDEX analytics_rollups_org ON analytics_rollups (organization_id, granularity, bucket);
SELECT plux_tenant_policy('analytics_rollups');

-- How far the rollup job has read, per organisation: the newest received_at
-- whose events are folded in. Rollups are recomputed for the buckets that
-- later events touch, so a late event corrects its bucket.
CREATE TABLE analytics_watermarks (
    organization_id uuid        PRIMARY KEY REFERENCES organizations (id) ON DELETE CASCADE,
    rolled_until    timestamptz NOT NULL
);
SELECT plux_tenant_policy('analytics_watermarks');
