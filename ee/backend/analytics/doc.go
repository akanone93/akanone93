// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

// Package analytics is the enterprise edition's analytics storage and
// exports (ANL-010, ANL-011, ADR-0080).
//
// # ClickHouse store
//
// Opener opens a ClickHouse implementation of the public analytics.Store
// seam in place of the core's PostgreSQL store. It is opened only when
// PLUX_ANALYTICS_CLICKHOUSE_URL names a server and the licence allows
// edition.FeatureClickHouse; otherwise Open returns no store and the core
// keeps PostgreSQL. The URL is a ClickHouse DSN such as
// clickhouse://user:password@host:9000/plux?secure=true and is never logged.
//
// Raw events live in a ReplacingMergeTree table partitioned by month of event
// time, so a retention run drops whole months (Compact) and removes the rows
// of one organisation with a synchronous mutation (Purge). The seam hands the
// store a cut-off per organisation, not a table-wide age, so there is no
// table TTL: a TTL could not keep one organisation's 13 months while another
// keeps 24. Rollups of 5 minutes, one hour and one day are rows of a second
// ReplacingMergeTree table, written by the same algorithm as the PostgreSQL
// store (raw events grouped in the database, folded upwards here), so the two
// stores return the same series for the same events. A row written again
// carries a higher version and replaces the older one; reads use FINAL, and
// raw reads drop duplicate (id, time) pairs, so the answers do not depend on
// when ClickHouse merges parts.
//
// Every query binds the organisation as a server-side query parameter
// ({org:UUID}); an identifier is also checked to be a canonical UUID before
// it is used, and no identifier is ever written into query text. The only
// text assembled at run time is the histogram bounds, which are compile-time
// constants of the public analytics package, and partition identifiers that
// have been parsed as year and month.
//
// # Switching from PostgreSQL
//
// PostgreSQL is the right store for most installations. Plan the move to
// ClickHouse at about ten million events a day or one million active devices,
// or earlier when dashboard queries over raw events compete with the
// transactional workload. Events already in PostgreSQL stay there: the
// switch starts a new history in ClickHouse. The operations guide carries
// the runbook.
//
// # Exports
//
// ParquetExporter writes raw events or rollups of an organisation as
// Snappy-compressed Parquet files to an ObjectStore the core passes in
// (edition.FeatureParquetExport). WebhookExporter posts batches of events to
// a configured URL, signed as Standard Webhooks, through the core's
// egress-registered HTTP client (edition.FeatureWebhookExport). Both refuse
// a write without their licence with PLX-8600.
package analytics
