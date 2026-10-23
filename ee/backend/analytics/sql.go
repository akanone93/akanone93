// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package analytics

import (
	"strconv"
	"strings"

	ext "github.com/nightCode42/plux3/backend/ext/analytics"
)

// Every query of the store is here. Values reach them only as server-side
// parameters ({name:Type}); the one piece assembled at run time is the list
// of histogram bounds, which are constants of the public analytics package.
// The row limit is {row_limit:UInt64}: ClickHouse 25.8 rejects a parameter
// named limit.

// eventColumns are the columns of the events table in insert order.
func eventColumns() []string {
	return []string{
		"organization_id", "app_id", "environment_id", "device_id", "id", "name", "time", "received_at",
		"release_sequence", "plugin_key", "route", "fields", "device_class", "locale", "category",
	}
}

// rollupColumns are the columns of the rollups table in insert order.
func rollupColumns() []string {
	return []string{
		"organization_id", "app_id", "environment_id", "granularity", "event_name", "bucket", "release_sequence",
		"plugin_key", "page", "device_class", "locale", "events", "value_count", "value_sum", "value_min", "value_max",
		"histogram", "version",
	}
}

// rollupSelect lists the columns a rollup row is read with, in the order
// scanRollup expects.
const rollupSelect = `toString(app_id), toString(environment_id), granularity, event_name, bucket, release_sequence,
       plugin_key, page, device_class, locale, events, value_count, value_sum, value_min, value_max, histogram`

// eventSelect lists the columns an event is read with, in the order
// scanEvent expects.
const eventSelect = `toString(id), toString(organization_id), toString(app_id), toString(environment_id), toString(device_id),
       name, time, release_sequence, plugin_key, route, category, device_class, locale, fields`

const (
	// queryWatermark reads how far an organisation is rolled up.
	queryWatermark = `SELECT rolled_until FROM ` + watermarksTable + ` FINAL WHERE organization_id = {org:UUID}`

	// queryTouchedBuckets lists the five-minute buckets that events received
	// in (after, until] fall in.
	queryTouchedBuckets = `SELECT DISTINCT toDateTime64(toStartOfFiveMinutes(time), 6, 'UTC') AS bucket
  FROM ` + eventsTable + `
 WHERE organization_id = {org:UUID} AND received_at > {after:DateTime64(6)} AND received_at <= {until:DateTime64(6)}
 ORDER BY bucket`

	// queryRollupWindow reads the rollups of one granularity in a window.
	queryRollupWindow = `SELECT ` + rollupSelect + `
  FROM ` + rollupsTable + ` FINAL
 WHERE organization_id = {org:UUID} AND granularity = {granularity:String}
   AND bucket >= {from:DateTime64(6)} AND bucket < {to:DateTime64(6)}`

	// queryRollupSeries reads the rollups of one metric of one app.
	queryRollupSeries = `SELECT ` + rollupSelect + `
  FROM ` + rollupsTable + ` FINAL
 WHERE organization_id = {org:UUID} AND app_id = {app:UUID} AND granularity = {granularity:String}
   AND event_name = {metric:String}
   AND bucket >= {from:DateTime64(6)} AND bucket < {to:DateTime64(6)}`

	// seriesInEnvironment narrows queryRollupSeries to one environment.
	seriesInEnvironment = ` AND environment_id = {env:UUID}`

	// seriesOrder orders and bounds a series.
	seriesOrder = ` ORDER BY bucket LIMIT {row_limit:UInt64}`

	// queryEvents pages through the raw events of an organisation in time
	// order, once per (id, time), after a cursor.
	queryEvents = `SELECT ` + eventSelect + `
  FROM ` + eventsTable + `
 WHERE organization_id = {org:UUID} AND time >= {from:DateTime64(6)} AND time < {to:DateTime64(6)}
   AND (time, id) > ({cursor_time:DateTime64(6)}, {cursor_id:UUID})
 ORDER BY time, id
 LIMIT 1 BY id, time
 LIMIT {row_limit:UInt64}`

	// queryCountRaw counts the events an organisation holds before a time.
	queryCountRaw = `SELECT count() FROM (SELECT 1 FROM ` + eventsTable + `
 WHERE organization_id = {org:UUID} AND time < {before:DateTime64(6)} LIMIT 1 BY id, time)`

	// queryCountRollups counts the rollups an organisation holds before a time.
	queryCountRollups = `SELECT count() FROM ` + rollupsTable + ` FINAL
 WHERE organization_id = {org:UUID} AND bucket < {before:DateTime64(6)}`

	// deleteRaw deletes an organisation's events before a time and waits for
	// the mutation.
	deleteRaw = `ALTER TABLE ` + eventsTable + ` DELETE WHERE organization_id = {org:UUID} AND time < {before:DateTime64(6)}
SETTINGS mutations_sync = 2`

	// deleteRollups deletes an organisation's rollups before a time and waits
	// for the mutation.
	deleteRollups = `ALTER TABLE ` + rollupsTable + ` DELETE WHERE organization_id = {org:UUID} AND bucket < {before:DateTime64(6)}
SETTINGS mutations_sync = 2`

	// queryPartitions lists the active partitions of the events table.
	queryPartitions = `SELECT DISTINCT partition_id FROM system.parts
 WHERE database = currentDatabase() AND table = {table:String} AND active`
)

// aggregateRawQuery builds the query that groups the raw events of one
// organisation and window into five-minute buckets by dimension and
// histogram bucket, as the PostgreSQL store's AggregateRawEvents does.
//
// The duration of an event is its build_ms for render_perf and its
// duration_ms otherwise, when that field is a JSON number. The histogram
// bucket is the number of bounds at or below it, -1 without a duration.
func aggregateRawQuery() string {
	bounds := make([]string, len(ext.Bounds))
	for i, b := range ext.Bounds {
		bounds[i] = strconv.FormatFloat(b, 'f', -1, 64)
	}
	key := `if(name = 'render_perf', 'build_ms', 'duration_ms')`
	return `SELECT toDateTime64(toStartOfFiveMinutes(time), 6, 'UTC') AS bucket,
       toString(app_id), toString(environment_id), name, release_sequence, plugin_key, route, device_class, locale,
       if(isNull(value), -1, toInt32(arraySum(b -> toUInt8(assumeNotNull(value) >= b), [` + strings.Join(bounds, ", ") + `]))) AS hist_bucket,
       count() AS events, count(value) AS value_count,
       ifNull(sum(value), 0) AS value_sum, ifNull(min(value), 0) AS value_min, ifNull(max(value), 0) AS value_max
  FROM (
        SELECT time, app_id, environment_id, name, release_sequence, plugin_key, route, device_class, locale,
               if(JSONType(fields, ` + key + `) IN ('Int64', 'UInt64', 'Double'), JSONExtractFloat(fields, ` + key + `), NULL) AS value
          FROM ` + eventsTable + `
         WHERE organization_id = {org:UUID} AND time >= {from:DateTime64(6)} AND time < {to:DateTime64(6)}
         LIMIT 1 BY id, time
       )
 GROUP BY bucket, app_id, environment_id, name, release_sequence, plugin_key, route, device_class, locale, hist_bucket`
}
