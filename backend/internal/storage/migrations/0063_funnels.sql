-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Funnel definitions of an app (ANL-020, STU-114, P11 plan section 4.3 #21).
-- A funnel is an ordered list of event names; its result counts each step's
-- events over a time range from the rollups. Expand-only: a new table.
CREATE TABLE funnels (
    id              uuid        PRIMARY KEY,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    app_id          uuid        NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    name            text        NOT NULL,
    steps           jsonb       NOT NULL,
    environment_id  uuid        REFERENCES environments (id) ON DELETE CASCADE,
    revision        bigint      NOT NULL DEFAULT 1,
    created_by      text        NOT NULL,
    created_at      timestamptz NOT NULL,
    updated_by      text        NOT NULL,
    updated_at      timestamptz NOT NULL,
    CONSTRAINT funnels_steps CHECK (jsonb_typeof(steps) = 'array' AND jsonb_array_length(steps) >= 2)
);
CREATE UNIQUE INDEX funnels_name ON funnels (app_id, name);
CREATE INDEX funnels_org ON funnels (organization_id);
SELECT plux_tenant_policy('funnels');
SELECT plux_app_policy('funnels');
