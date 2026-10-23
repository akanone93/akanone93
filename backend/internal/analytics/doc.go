// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package analytics stores runtime events and their rollups, answers
// queries over the rollups, enforces retention and exports to OTLP
// (ANL-010, ANL-011, ADR-0080, maintainer decision Q17).
//
// # Storage
//
// Storage is behind Store, whose public definition is in backend/ext/analytics.
// The core implements it on PostgreSQL (Postgres): monthly range partitions
// of raw events, created ahead by the worker, and rollups at five minutes,
// one hour and one day by app, environment, release, plugin, page, device
// class, locale and event name. Counts are exact; p50, p90 and p99 are
// estimated from a fixed sixteen-bucket histogram that is added up when
// rollups are folded and when queries group, so a percentile never needs
// the raw events. A ClickHouse adapter in the enterprise edition implements
// the same Store.
//
// # Retention
//
// Raw events are kept 13 months, rollups indefinitely and the production
// audit log 10 years, each configurable per organisation (maintainer
// decision Q17). A change waits for the maintenance window, by default
// 02:00 to 04:00 in the installation's time zone, and is reported as PLX-8301;
// the administrator may apply it at once instead. Scheduled changes can be
// listed and cancelled, and every change is audited. The audit class is
// configuration and schedule only: nothing deletes audit entries here, so
// the hash chain stays verifiable (SEC-140).
//
// # OTLP export
//
// Service.ExportAll sends the five-minute rollups of an organisation as
// OTLP/HTTP JSON metrics (POST {endpoint}/v1/metrics) to the destination the
// organisation configured: plux.events, a monotonic delta sum, and
// plux.event.duration, an explicit-bucket delta histogram in milliseconds.
// Requests go through the SSRF-safe client of package httpx, over https only
// by default, and a destination that fails is PLX-8302 and is retried at
// the next round. The destination is the only outbound address this package
// uses; it belongs on the egress list of ADR-0082, which has no registry to
// record it in yet (milestone E8).
//
// Invariants: every call is scoped to one organisation; no rollup or export
// carries a device identifier or an event's fields; deleting data never
// touches the audit log.
package analytics
