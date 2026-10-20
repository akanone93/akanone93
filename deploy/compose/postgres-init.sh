#!/bin/sh
# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: Apache-2.0

# Runs once, when the postgres volume is first created: makes the role
# plux-server connects as. It owns the database's schema but is neither a
# superuser nor BYPASSRLS, so row-level security binds it (SRV-022); the
# server refuses to start as a role that it does not bind.
#
# It also makes the Grafana analytics reader (ANL-020). That role gets no
# table privileges here: deploy/observability/analytics-reader.sql grants
# SELECT on the rollups once the server has applied its migrations.
set -eu
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" \
  -v app_password="$PLUX_APP_PASSWORD" -v reader_password="$PLUX_ANALYTICS_READER_PASSWORD" <<'SQL'
CREATE ROLE plux LOGIN NOSUPERUSER NOBYPASSRLS NOCREATEROLE NOCREATEDB PASSWORD :'app_password';
GRANT CONNECT, CREATE, TEMPORARY ON DATABASE plux TO plux;
ALTER SCHEMA public OWNER TO plux;
CREATE ROLE plux_analytics_reader LOGIN NOSUPERUSER NOBYPASSRLS NOCREATEROLE NOCREATEDB PASSWORD :'reader_password';
GRANT CONNECT ON DATABASE plux TO plux_analytics_reader;
GRANT USAGE ON SCHEMA public TO plux_analytics_reader;
SQL
