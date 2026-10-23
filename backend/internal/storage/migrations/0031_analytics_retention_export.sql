-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Analytics storage, part three: retention per organisation and data class,
-- scheduled retention changes, and the OTLP export (ANL-010, ANL-011,
-- ADR-0080, maintainer decision Q17). The tables are new and nothing
-- existing is altered.

-- The retention an organisation set. A data class with no row has the
-- installation default: raw events 396 days (13 months), rollups
-- indefinitely, production audit 3653 days (10 years). days is NULL for
-- "indefinitely". audit_production is recorded and shown but nothing
-- deletes audit entries yet: the chain must stay verifiable (SEC-140).
CREATE TABLE retention_settings (
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    data_class      text        NOT NULL,
    days            integer,
    changed_by      text        NOT NULL,
    changed_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, data_class),
    CONSTRAINT retention_settings_class CHECK (data_class IN ('raw_events', 'rollups', 'audit_production')),
    CONSTRAINT retention_settings_days CHECK (days IS NULL OR days >= 1)
);
SELECT plux_tenant_policy('retention_settings');

-- A change that waits for the maintenance window, or applied immediately,
-- kept as history. At most one is scheduled per class (PLX-8301).
CREATE TABLE retention_changes (
    id              uuid        PRIMARY KEY,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    data_class      text        NOT NULL,
    days            integer,
    state           text        NOT NULL,
    requested_by    text        NOT NULL,
    requested_at    timestamptz NOT NULL DEFAULT now(),
    apply_at        timestamptz NOT NULL,
    finished_at     timestamptz,
    CONSTRAINT retention_changes_class CHECK (data_class IN ('raw_events', 'rollups', 'audit_production')),
    CONSTRAINT retention_changes_state CHECK (state IN ('scheduled', 'applied', 'cancelled')),
    CONSTRAINT retention_changes_days CHECK (days IS NULL OR days >= 1)
);
CREATE UNIQUE INDEX retention_changes_scheduled ON retention_changes (organization_id, data_class) WHERE state = 'scheduled';
CREATE INDEX retention_changes_due ON retention_changes (apply_at) WHERE state = 'scheduled';
SELECT plux_tenant_policy('retention_changes');

-- The OTLP destination of an organisation. The export sends the rollups as
-- OTLP/HTTP JSON metrics. The destination is checked by the SSRF-safe client
-- on every send (SEC-105). exported_until is the end of the last bucket
-- sent, so a restart resumes where it stopped.
CREATE TABLE analytics_exports (
    organization_id uuid        PRIMARY KEY REFERENCES organizations (id) ON DELETE CASCADE,
    endpoint        text        NOT NULL,
    enabled         boolean     NOT NULL DEFAULT true,
    exported_until  timestamptz NOT NULL DEFAULT now(),
    last_error      text        NOT NULL DEFAULT '',
    last_attempt_at timestamptz,
    changed_by      text        NOT NULL,
    changed_at      timestamptz NOT NULL DEFAULT now()
);
SELECT plux_tenant_policy('analytics_exports');
