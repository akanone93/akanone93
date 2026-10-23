-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Analytics storage, part one: raw events get a table partitioned by
-- month (ANL-010, ADR-0080). Retention then drops a month at a time, and a
-- query for a time range reads only its months.
--
-- Expand only: the flat telemetry_events table of 0007 stays as it is, for a
-- replica of the previous version that still writes to it, and a later
-- migration drops it. The partitioned table is new and is named
-- analytics_events; existing rows are copied once. Events a previous-version
-- replica writes to the flat table during a rolling upgrade are not copied.

-- What the newest session_start says about the device, so every later event
-- of the device is rolled up by device class and locale (ANL-010). Locale
-- arrives only with analytics consent and is '' otherwise.
ALTER TABLE devices ADD COLUMN device_class text NOT NULL DEFAULT '';
ALTER TABLE devices ADD COLUMN locale text NOT NULL DEFAULT '';

-- plux_events_partition makes the partition of the month that contains
-- `month`, if it does not exist. Bounds are UTC, so a partition is the same
-- wherever the server runs. It returns the partition's name.
CREATE OR REPLACE FUNCTION plux_events_partition(month date) RETURNS text
    LANGUAGE plpgsql
    AS $$
    DECLARE
        first_day date := date_trunc('month', month)::date;
        name text := 'analytics_events_' || to_char(first_day, 'YYYYMM');
    BEGIN
        -- Two replicas making the same partition would collide in the catalogue.
        PERFORM pg_advisory_xact_lock(hashtext('plux_events_partition'));
        IF to_regclass(name) IS NULL THEN
            EXECUTE format(
                'CREATE TABLE %I PARTITION OF analytics_events FOR VALUES FROM (%L) TO (%L)',
                name,
                (first_day::timestamp AT TIME ZONE 'UTC'),
                ((first_day + interval '1 month')::timestamp AT TIME ZONE 'UTC'));
            -- A partition holds tenant rows, so it gets the same row-level
            -- security as its parent (SRV-022).
            PERFORM plux_tenant_policy(name::regclass);
        END IF;
        RETURN name;
    END;
    $$;

COMMENT ON FUNCTION plux_events_partition(date) IS
    'Creates the monthly partition of analytics_events holding the given date (ANL-010).';

-- category is the consent category of the event name (SEC-161); device_class
-- and locale are the device's at the time the event was written.
CREATE TABLE analytics_events (
    id               uuid        NOT NULL,
    organization_id  uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    app_id           uuid        NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    environment_id   uuid        NOT NULL REFERENCES environments (id) ON DELETE CASCADE,
    device_id        uuid        NOT NULL REFERENCES devices (id) ON DELETE CASCADE,
    name             text        NOT NULL,
    time             timestamptz NOT NULL,
    release_sequence bigint      NOT NULL DEFAULT 0,
    plugin_key       text        NOT NULL DEFAULT '',
    route            text        NOT NULL DEFAULT '',
    fields           jsonb       NOT NULL DEFAULT '{}',
    received_at      timestamptz NOT NULL DEFAULT now(),
    device_class     text        NOT NULL DEFAULT '',
    locale           text        NOT NULL DEFAULT '',
    category         text        NOT NULL DEFAULT 'necessary',
    PRIMARY KEY (id, time),
    CONSTRAINT analytics_events_category CHECK (category IN ('necessary', 'analytics', 'experiments'))
) PARTITION BY RANGE (time);
CREATE INDEX analytics_events_app ON analytics_events (app_id, environment_id, time DESC, id DESC);
CREATE INDEX analytics_events_received ON analytics_events (received_at);

-- Partitions from the oldest copied event to three months ahead; the worker
-- keeps creating them ahead of time.
DO $$
DECLARE
    month date := date_trunc('month', coalesce((SELECT min(time) FROM telemetry_events), now()))::date;
    last  date := (date_trunc('month', now()) + interval '3 months')::date;
BEGIN
    WHILE month <= last LOOP
        PERFORM plux_events_partition(month);
        month := (month + interval '1 month')::date;
    END LOOP;
END
$$;

INSERT INTO analytics_events (id, organization_id, app_id, environment_id, device_id, name, time,
                              release_sequence, plugin_key, route, fields, received_at)
SELECT id, organization_id, app_id, environment_id, device_id, name, time,
       release_sequence, plugin_key, route, fields, received_at
  FROM telemetry_events;

SELECT plux_tenant_policy('analytics_events');
