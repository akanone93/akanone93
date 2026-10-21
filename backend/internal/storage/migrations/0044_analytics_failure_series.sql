-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Failure series for the rollout health gates (REL-053, ANL-010). The
-- rollups have no result dimension, so the failures of actions and
-- functions are rolled up as series of their own, next to the events they
-- come from: `action_error` counts the `action_run` events whose result is
-- `failed` (a cancelled run is not a failure), and `function_error` counts
-- the `function_call` events whose status is not `ok`, or is a number of
-- 400 or more. Devices cannot send these names (the ingestion refuses
-- unknown events), so a series is never counted twice.
--
-- Additive: rows already rolled up keep their figures, and the series
-- appear for every bucket that is rolled up after this migration.

-- plux_event_series is the rollup series an event adds to: its own name,
-- then the failure series it counts towards, if any.
CREATE FUNCTION plux_event_series(name text, fields jsonb) RETURNS text[]
    LANGUAGE sql IMMUTABLE PARALLEL SAFE
    AS $$
        SELECT CASE
            WHEN name = 'action_run' AND fields ->> 'result' = 'failed'
                THEN ARRAY[name, 'action_error']
            WHEN name = 'function_call' AND (
                    (jsonb_typeof(fields -> 'status') = 'string' AND fields ->> 'status' <> 'ok')
                 OR (jsonb_typeof(fields -> 'status') = 'number' AND (fields ->> 'status')::double precision >= 400))
                THEN ARRAY[name, 'function_error']
            ELSE ARRAY[name]
        END
    $$;
