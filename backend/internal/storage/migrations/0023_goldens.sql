-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Golden (visual regression) tests (TST-003, ADR-0070): the runs `plux
-- golden run --upload` records, their cases and the approved baselines. The
-- images are in object storage under the plugin, by SHA-256; the rows hold
-- the hashes. The tables are new and nothing existing is altered, so a
-- replica of the previous version keeps working during a rolling upgrade
-- (SRV-021).

-- One run of the golden matrix for one bundle. plugin_key is "" for the app
-- bundle. bundle_sha256 is the bundle under test and app_sha256 the app
-- bundle it was rendered with; the compiler's determinism (CMP-002) makes
-- them the hashes a publish produces. A run that was not rendered on the
-- canonical renderer is recorded but never satisfies the publish gate.
CREATE TABLE golden_runs (
    id              uuid        PRIMARY KEY,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    app_id          uuid        NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    plugin_key      text        NOT NULL,
    bundle_sha256   bytea       NOT NULL,
    app_sha256      bytea       NOT NULL,
    canonical       boolean     NOT NULL,
    renderer        text        NOT NULL DEFAULT '',
    created_by      text        NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX golden_runs_bundle ON golden_runs (app_id, plugin_key, bundle_sha256, app_sha256, created_at DESC);
SELECT plux_tenant_policy('golden_runs');

-- One rendered case of a run. image_bytes is 0 when the client sent no image
-- because the case matched the baseline it had been given.
CREATE TABLE golden_cases (
    run_id          uuid             NOT NULL REFERENCES golden_runs (id) ON DELETE CASCADE,
    case_id         text             NOT NULL,
    organization_id uuid             NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    frame           text             NOT NULL,
    locale          text             NOT NULL,
    theme           text             NOT NULL,
    text_scale      double precision NOT NULL,
    image_sha256    bytea            NOT NULL,
    image_bytes     bigint           NOT NULL DEFAULT 0,
    PRIMARY KEY (run_id, case_id)
);
SELECT plux_tenant_policy('golden_cases');

-- The approved image of a case of a plugin (goldens.approve).
CREATE TABLE golden_baselines (
    app_id          uuid        NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    plugin_key      text        NOT NULL,
    case_id         text        NOT NULL,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    image_sha256    bytea       NOT NULL,
    image_bytes     bigint      NOT NULL,
    approved_by     text        NOT NULL,
    approved_at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (app_id, plugin_key, case_id)
);
SELECT plux_tenant_policy('golden_baselines');
