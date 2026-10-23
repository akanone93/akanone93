# 0080. Telemetry ingestion and analytics storage: PostgreSQL rollups, ClickHouse, retention, exports

- **Status:** Accepted
- **Date:** 2026-10-18
- **Requirements:** `ANL-010`, `ANL-011`, `ANL-020`, `ANL-030`, `ANL-040`, `SEC-161`, `LIM-001`, `NFR-040`

## Context and problem

The server already accepts, validates and stores telemetry in bounded form (ADR-0034), without
rollups. P9 needs ingestion that survives bursts, queryable aggregates for dashboards, health
gates and experiments (ADR-0078, ADR-0079), alerting, export, and retention that regulated
customers set themselves. Most installations are small; a few will be large. The question: what
stores the events, how are they aggregated, and how do operators move up in scale?

## Decision drivers

- One system to run for most installations (ADR-0007: PostgreSQL as system of record).
- A path to higher volume without rewriting consumers.
- Retention is configurable and auditable; erasure is possible (ADR-0082).
- Dashboards run in Grafana (maintainer decision Q2); Studio surfaces wait for P11.
- Consent categories are honoured server-side as well.

## Considered options

1. PostgreSQL partitions and rollups in the core; ClickHouse behind the same interface in the enterprise edition.
2. ClickHouse only, as a required component.
3. A hosted analytics service.

## Decision

Chosen option: **1**.

- **Ingestion.** The endpoint batches events into a Valkey stream; workers write in bulk. Quotas
  come from the limit registry (`telemetry.eventsPerDevicePerHour`, `telemetry.batchBytes`;
  `PLX-8300` over quota). Consent categories are enforced server-side too.
- **Storage (`ANL-010`).** PostgreSQL: monthly partitions of raw events and rollups at 5 minutes,
  hourly and daily by app, release, plugin, page, device class and locale. A ClickHouse adapter
  (enterprise, `clickhouse-go/v2`) implements the same `analytics.Store` interface. The sizing
  guide recommends the switch at roughly 10 million events per day or 1 million active devices.
  How events migrate on switch-over is decided in the milestone.
- **Retention (maintainer decision Q17).** Defaults: raw events 13 months, rollups indefinitely,
  production audit 10 years. Every value is configurable per organisation and data class. A
  change applies in the off-peak maintenance window (default 02:00 to 04:00 in the installation's
  zone) or immediately when the administrator chooses. Scheduled changes are visible and
  cancellable (`PLX-8301`); every change is audited.
- **Exports (`ANL-011`).** OTLP in the core; signed webhooks and periodic Parquet files to
  object storage in the enterprise edition (`parquet-go/parquet-go`). A failing destination gives
  `PLX-8302`.
- **Dashboards (`ANL-020`).** Grafana over a Plux datasource API or directly over the rollups:
  organisation and app overview, page and plugin usage, funnels, crash-free sessions and error
  rates per release, performance by device class, sync health, function latency, experiments,
  `l10n_missing`.
- **Alerts (`ANL-030`).** Rules on any dashboard metric, evaluated by the worker and routed to
  ADR-0081 channels. Infrastructure alerts are Prometheus rules (ADR-0085).
- **Errors (`ANL-040`).** Fingerprint is error code plus JSON path plus top frames, symbolicated
  through the release's retained source maps; opening the node in Studio waits for P11.

## Consequences

- **Positive:** a single database for most installs; no new component is required; the store
  interface lets ClickHouse replace PostgreSQL without touching consumers.
- **Negative:** PostgreSQL write amplification at volume (partitioning, rollups and quotas
  mitigate it); two backends to test; ClickHouse needs a licence and operational knowledge.
- **Follow-up:** migrations 0039 to 0041; `dependencies.md` rows for `clickhouse-go/v2` and
  `parquet-go`; a switch-over runbook.

## Options in detail

### Option 1 — PostgreSQL first, ClickHouse as the enterprise store

Keeps the default install small and recoverable with one backup system (ADR-0083). Rollups make
the common queries cheap, so raw scans are rare.

### Option 2 — ClickHouse only

Best query performance, but a second stateful system for every install, with its own HA and
backup story, even for small ones. Rejected as the default.

### Option 3 — hosted service

Sends device data to a third party and fails air-gapped installs (`DEP-040`). Rejected.
