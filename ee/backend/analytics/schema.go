// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package analytics

import (
	"context"
	"fmt"
)

// The tables of the store. They have the names of the PostgreSQL tables.
const (
	eventsTable     = "analytics_events"
	rollupsTable    = "analytics_rollups"
	watermarksTable = "analytics_watermarks"
)

// schemaStatements are the idempotent statements that create the tables.
//
// Events are keyed by (organisation, time, id): writing an event again
// replaces the row with an identical one. Rollups are keyed by the series and
// carry a version, so a recomputed bucket replaces the earlier one.
func schemaStatements() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS ` + eventsTable + ` (
    organization_id  UUID,
    app_id           UUID,
    environment_id   UUID,
    device_id        UUID,
    id               UUID,
    name             LowCardinality(String),
    time             DateTime64(6, 'UTC'),
    received_at      DateTime64(6, 'UTC'),
    release_sequence Int64,
    plugin_key       String,
    route            String,
    fields           String,
    device_class     LowCardinality(String),
    locale           LowCardinality(String),
    category         LowCardinality(String)
) ENGINE = ReplacingMergeTree
PARTITION BY toYYYYMM(time)
ORDER BY (organization_id, time, id)`,
		`CREATE TABLE IF NOT EXISTS ` + rollupsTable + ` (
    organization_id  UUID,
    app_id           UUID,
    environment_id   UUID,
    granularity      LowCardinality(String),
    event_name       LowCardinality(String),
    bucket           DateTime64(6, 'UTC'),
    release_sequence Int64,
    plugin_key       String,
    page             String,
    device_class     String,
    locale           String,
    events           Int64,
    value_count      Int64,
    value_sum        Float64,
    value_min        Float64,
    value_max        Float64,
    histogram        Array(Int64),
    version          UInt64
) ENGINE = ReplacingMergeTree(version)
PARTITION BY toYear(bucket)
ORDER BY (organization_id, app_id, granularity, event_name, bucket, environment_id, release_sequence, plugin_key, page, device_class, locale)`,
		`CREATE TABLE IF NOT EXISTS ` + watermarksTable + ` (
    organization_id UUID,
    rolled_until    DateTime64(6, 'UTC'),
    version         UInt64
) ENGINE = ReplacingMergeTree(version)
ORDER BY organization_id`,
	}
}

// migrate creates the tables that do not exist.
func (s *Store) migrate(ctx context.Context) error {
	for _, stmt := range schemaStatements() {
		if err := s.db.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("analytics: create the tables: %w", err)
		}
	}
	return nil
}
