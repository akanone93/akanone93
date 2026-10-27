-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Plux Test runs reported by `plux test` and CI (TST-001, TST-002, P11
-- S8.10). The table is new and nothing existing is altered, so a replica of
-- the previous version keeps working during a rolling upgrade (SRV-021).

-- One reported run. results holds the scenarios with their steps as JSON;
-- the counts are summed when the run is reported. report_key makes the
-- report idempotent per app: the same key returns the first run.
CREATE TABLE test_runs (
    id              uuid        PRIMARY KEY,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    app_id          uuid        NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    report_key      text        NOT NULL,
    source          text        NOT NULL DEFAULT '',
    label           text        NOT NULL DEFAULT '',
    passed          integer     NOT NULL,
    failed          integer     NOT NULL,
    skipped         integer     NOT NULL,
    duration_ms     bigint      NOT NULL,
    results         jsonb       NOT NULL,
    created_by      text        NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT test_runs_report_key UNIQUE (app_id, report_key)
);
CREATE INDEX test_runs_app ON test_runs (app_id, created_at DESC, id DESC);
SELECT plux_tenant_policy('test_runs');
SELECT plux_app_policy('test_runs');
