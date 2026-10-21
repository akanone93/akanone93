<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Analytics Guide

What the runtime reports about your app in use, how long it is kept, how it leaves the
installation, and how it becomes dashboards, alerts and grouped errors. The requirements are
`ANL-010`, `ANL-011`, `ANL-020`, `ANL-030`, `ANL-040`, `SEC-140` and
[ADR-0080](../adr/0080-telemetry-and-analytics-storage.md).

## 1. The idea in one page

A device sends **events** when something happens in the app: a session starts or ends, an action
runs, a function fails, an error is reported, a custom event is recorded. The server stores them
and keeps **rollups**: counts and percentiles per app, environment, release, plugin, page, device
class, locale and event name, at five minutes, one hour and one day. Dashboards and alerts read
the rollups, not the raw events.

Two rules hold throughout:

- **No device identifier and no event field** reaches a rollup or an export (the analytics
  invariant). Analytics answers "how many" and "how slow", not "which person".
- **Consent is honoured on the server too.** The categories a person allowed are enforced by the
  server as well as by the device.

## 2. Ingestion

Devices post batches to the telemetry endpoint. The endpoint writes the batch to a Valkey stream,
and workers bulk-write it to storage.

Quotas come from the limits registry ([limits reference](../reference/limits.md)):

- `telemetry.eventsPerDevicePerHour` and `telemetry.eventsPerRequest` bound a device's events;
- `telemetry.batchBytes` and `telemetry.bufferBytes` bound a batch and the device's buffer.

A device over its quota is refused with `PLX-8300`.

## 3. Storage

- **Raw events** are kept in monthly partitions, created ahead of time by the worker.
- **Rollups** are kept at five minutes, one hour and one day.
- **Counts are exact.** p50, p90 and p99 are estimated from a fixed sixteen-bucket histogram that
  adds up when rollups are folded and when queries group. A percentile never needs the raw events.

PostgreSQL is the default store. The enterprise edition adds a ClickHouse store with the same
interface, for higher volumes. [The sizing guide](../operations/sizing.md) gives the point at which
the switch is worth making: roughly 10 million events a day, or 1 million active devices,
whichever comes first.

## 4. Retention

The defaults are set by the maintainers (decision Q17), and each is configurable per organisation
and per data class:

| Data | Default |
|---|---|
| Raw events | 13 months |
| Rollups | indefinitely |
| Production audit log | 10 years |

A change to retention waits for the **off-peak maintenance window**, by default 02:00 to 04:00 in
the installation's time zone, and is reported as `PLX-8301` until then. An administrator may apply
it at once instead. A scheduled change is listed and can be cancelled before it runs
(`AnalyticsService.ListRetentionChanges`, `CancelRetentionChange`); every change is audited.

The audit log is never deleted by a retention change. Nothing in analytics deletes audit entries,
so the hash chain stays verifiable (`SEC-140`).

## 5. Exports

### OTLP metrics (core)

The five-minute rollups of an organisation can be sent to an OpenTelemetry collector as OTLP/HTTP
JSON metrics, to the destination the organisation configures. The metrics are:

- `plux.events`, a monotonic delta sum;
- `plux.event.duration`, a delta histogram in milliseconds.

The request goes through the SSRF-safe client over HTTPS only, by default. A destination that
fails is reported as `PLX-8302` and retried at the next round. Set the export with
`AnalyticsService.SetExport` and read it with `GetExport`.

### Files (enterprise)

Periodic Parquet files, written to object storage, are an enterprise export. Each file is bounded
by `telemetry.exportFileRows`. Configure them with `AnalyticsService.SetFileExport` and read them
with `GetFileExport`.

Every outbound destination is on the [egress list](../operations/egress.md).

## 6. Dashboards

Dashboards run in Grafana (maintainer decision Q2). The analytics dashboards are provisioned from
`deploy/observability/dashboards`:

- `plux-analytics-app`, `plux-analytics-usage`, `plux-analytics-quality`,
  `plux-analytics-performance`, `plux-analytics-sync-functions`,
  `plux-analytics-localisation`, `plux-analytics-experiments` and `plux-analytics-organization`.

Grafana reads through a dedicated analytics login role with no bypass of row-level security. Its
table privileges are in `deploy/observability/analytics-reader.sql`. Dashboards inside Studio come
later.

The infrastructure dashboards (server overview, SLO burn, PostgreSQL, Valkey, object storage) are
in the same directory. The SLO objectives are in the [operations guide](operations.md).

## 7. Alerts

Product alerts are rules you write about your own app's events, for example a rise in function
errors. A rule has a condition and a for-duration, and moves through four states:

- **ok**, while the condition does not hold;
- **pending**, while it holds for less than the for-duration;
- **firing**, once it has held that long;
- **resolved**, when a firing rule's condition stops holding. A resolved rule that breaches again
  goes through pending, like an ok one.

Manage rules with `AnalyticsService.CreateAlertRule`, `UpdateAlertRule`, `DeleteAlertRule` and
`ListAlertRules`; `ListAlertStates` shows where each one stands. A firing alert is announced
through the notification channels as the `alert fired` event ([notifications guide](notifications.md)).

Infrastructure alerts (database replication lag, backup age, queue depth, certificate expiry) are
different: they go through Alertmanager, from the Prometheus rules in `deploy/observability/rules`.
The [alert runbooks](../runbooks/alerts/README.md) cover each alert.

## 8. Error grouping

Errors from devices are grouped, so you see one problem once rather than once per device.

An error's **fingerprint** is its error code, the node path where the runtime reported it (the
JSON path in the document), and the top frames of its stack. The same error on two devices has the
same fingerprint; a different code, node or frame has another. The number of frames is part of the
algorithm, not a tunable limit, because changing it would regroup every error.

Groups are kept per release, and the fingerprint is computed when the event arrives. The readable
location is resolved when a group is read, through the release's source map, which maps each
compiled node to its document pointer.

Read groups with `AnalyticsService.ListErrorGroups` and `GetErrorGroup`.

## 9. Privacy checklist

- Confirm the consent categories your app asks for match what you need.
- Choose the retention you can defend, and record why in your own documentation.
- Check the export destination is one you control. It is on the egress list, and it is the only
  outbound address analytics uses.
- Erasure of a person's data (`DataSubjectService.EraseSubject`) reaches their events and their
  contributions to rollups. The [personal data map](../operations/personal-data.md) lists what
  each store holds.

## 10. Errors

| Code | Meaning |
|---|---|
| `PLX-8300` | A device is over its telemetry quota. |
| `PLX-8301` | A retention change is scheduled for the maintenance window. |
| `PLX-8302` | An export destination failed; it is retried at the next round. |
