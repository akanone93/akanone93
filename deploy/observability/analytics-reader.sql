-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: Apache-2.0

-- The table privileges of the Grafana analytics reader (ANL-020). The reader
-- is a login role without BYPASSRLS, created by deploy/compose/postgres-init.sh
-- or, on Kubernetes, by the operator (the chart's observability.dashboards.analytics
-- values name it). It may read the rollups and nothing else: never the raw
-- events (analytics_events) or any table that holds personal data.
--
-- Row-level security still applies (plux_tenant_policy, SRV-022): every
-- dashboard query binds plux.organization_id to the organisation the
-- Organisation ID variable names, so the reader sees one organisation's rollups.
--
-- Run once after the server has applied migration 0030 (the rollups table
-- must exist), as the database owner:
--   psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f deploy/observability/analytics-reader.sql

GRANT SELECT ON analytics_rollups TO plux_analytics_reader;
