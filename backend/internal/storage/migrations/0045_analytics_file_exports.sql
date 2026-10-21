-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Settings of the enterprise analytics exports (ANL-011): periodic Parquet
-- files to object storage and signed webhooks of events. One row per
-- organisation, next to its OTLP destination in analytics_exports; the
-- OTLP row has a mandatory endpoint, which these exports do not have.
--
-- exported_until is the end of the last window delivered to every enabled
-- destination, so a failed window is delivered again from its start.
-- The webhook's signing secret is stored sealed, bound to the organisation
-- (SEC-092); the hint is its last few characters.
CREATE TABLE analytics_file_exports (
    organization_id         uuid        PRIMARY KEY REFERENCES organizations (id) ON DELETE CASCADE,
    parquet_enabled         boolean     NOT NULL DEFAULT false,
    parquet_events          boolean     NOT NULL DEFAULT true,
    parquet_rollups         boolean     NOT NULL DEFAULT true,
    parquet_granularity     text        NOT NULL DEFAULT '1h',
    webhook_enabled         boolean     NOT NULL DEFAULT false,
    webhook_url             text        NOT NULL DEFAULT '',
    secret_ciphertext       bytea       NOT NULL DEFAULT '',
    secret_wrapped_key      bytea       NOT NULL DEFAULT '',
    secret_key_id           text        NOT NULL DEFAULT '',
    secret_hint             text        NOT NULL DEFAULT '',
    exported_until          timestamptz NOT NULL DEFAULT now(),
    last_error              text        NOT NULL DEFAULT '',
    last_attempt_at         timestamptz,
    changed_by              text        NOT NULL,
    changed_at              timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT analytics_file_exports_granularity CHECK (parquet_granularity IN ('5m', '1h', '1d'))
);
SELECT plux_tenant_policy('analytics_file_exports');
