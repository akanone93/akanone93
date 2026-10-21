<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Sizing guide (`DEP-051`, `NFR-023`, `NFR-040`)

How much to run for a given number of active devices, apps and publishes, how the numbers were
obtained, and when to move telemetry analytics from PostgreSQL to ClickHouse. Read it with the
[on-premises reference architecture](on-premises.md) and the chart's `values.yaml`.

## How to read this guide

Every figure is one of three kinds, and the table that holds it says which:

- **Measured**: taken by a committed benchmark, with the commit and machine in
  [docs/benchmarks/](../benchmarks/README.md). Only these may be quoted as results.
- **Derived**: computed from measured figures and stated assumptions by the formulas below. A
  derived figure is a starting point; check it against your own traffic and the metrics in
  [Verify your own numbers](#verify-your-own-numbers).
- **Pending**: not yet measured. The scaled load run (`make load-test`) is produced by CI, and the
  full run on maintainer hardware (decision Q20) fills the table in
  [Full run](#full-run-q20-pending). Until then the tiers below are derived estimates, not
  guarantees.

## What one device costs

The load generator ([load.md](../benchmarks/load.md)) models a device in the foreground with the
runtime's default pacing:

| Request | Interval | Share of requests | Measured per-replica ceiling |
|---|---|---|---|
| Manifest check (`GetManifest`, `304`-style answer) | 10 min | 8 % | 5,000 req/s at p99 28 ms (`NFR-020`, [p2-backend.md](../benchmarks/p2-backend.md)) |
| Control document (`GetSignedControl`, ETag) | 5 min | 15 % | Pending (`make load-test`) |
| Telemetry batch (`IngestEvents`, 3 to 4 events) | 1 min | 77 % | Pending (`make load-test`) |
| Function call (`FunctionService/Invoke`), only when the app uses functions | 5 min | adds 0.0033 req/s | Overhead measured by `bench-fn` (`NFR-022`); capacity pending |

Shares are of the default mix without function calls: 1/600 + 1/300 + 1/60 = **0.0217 requests per
second per foreground device** (0.0250 with functions). Telemetry dominates because a batch is
sent every minute, so the cost of the control and manifest endpoints that `NFR-020` and `NFR-040`
speak of is the smaller part of the load.

Two assumptions turn *active* devices into requests; both are yours to replace with what your
analytics show:

| Assumption | Default used here | Why it matters |
|---|---|---|
| Share of active devices in the foreground at the same time (concurrency ratio) | 5 % | Scales every request rate |
| Foreground minutes per active device and day | 15 | Scales events per day |
| Peak factor over the average (app starts cluster) | 5 | Sizing is for the peak, not the mean |

## Requests per second by tier (derived)

`requests/s = active devices × concurrency ratio × 0.0217`, then × the peak factor.

| Active devices | Foreground at once | Average req/s | Peak req/s (× 5) | Telemetry events/day |
|---|---|---|---|---|
| 10,000 | 500 | 11 | 54 | 0.5 M |
| 100,000 | 5,000 | 108 | 542 | 4.8 M |
| 1,000,000 | 50,000 | 1,085 | 5,425 | 48 M |

Events per day: 15 foreground minutes × 3 events a minute, plus about three `session_start`
events, is 48 events per device and day. A device may send at most 600 events an hour
(`telemetry.eventsPerDevicePerHour` in `schema/limits.json`); that bound is the worst case for a
misbehaving client, not a sizing input.

## Roles and replicas (derived)

Every role runs at least two replicas in the chart. The reference deployment of spec §30.1 is the
configuration `NFR-023` is stated for (1,000,000 active devices, 100 apps, behind a CDN); the
smaller tiers keep its shape at lower resource requests.

| Component | Up to 10 k devices | Up to 100 k devices | Up to 1 M devices (reference) |
|---|---|---|---|
| `api` | 2 × (1 vCPU, 2 GiB) | 2 × (2 vCPU, 4 GiB) | 3 × (2 vCPU, 4 GiB) |
| `worker` | 2 × (1 vCPU, 2 GiB) | 2 × (2 vCPU, 4 GiB) | 2 × (4 vCPU, 8 GiB) |
| `fnrunner` and `egress` | 2 × (1 vCPU, 2 GiB), if the app uses functions | 2 × (2 vCPU, 4 GiB) | 2 × (4 vCPU, 8 GiB) |
| PostgreSQL (CloudNativePG, 3 instances, synchronous) | 2 vCPU, 8 GiB, 50 GiB | 4 vCPU, 16 GiB, 500 GiB | 4 vCPU, 16 GiB, storage per [PostgreSQL](#postgresql) |
| Valkey (3 pods with Sentinel) | 0.5 vCPU, 1 GiB | 1 vCPU, 2 GiB | 2 vCPU, 4 GiB |
| Object storage | 50 GiB | 500 GiB | per [Object storage](#object-storage) |
| CDN in front of bundle downloads | optional | recommended | required (`NFR-023`) |

Basis. The `api` count follows the measured 5,000 req/s of one 2 vCPU replica for the manifest
path: the peak of the largest tier, 5,425 req/s, is carried by three replicas with room for one
to fail (`NFR-040` needs the service to survive a replica loss), assuming the telemetry path is
no more expensive per request than the manifest path. **That assumption is not measured** and is
the first thing `make load-test` and the Q20 run settle; if telemetry ingestion costs more, add
`api` replicas in proportion. The `worker` count follows publishing (below), not request load.

Scale `api` on CPU and on `plux_rpc_duration_seconds` p99; scale `worker` on
`plux_jobs_queue_depth`.

### Publishing

A publish compiles, checks, signs and stores a release and computes deltas from the previous ten
versions. Measured: a 50-page plugin takes 317 to 395 ms (`NFR-021`, 15 s target), so one worker
core completes roughly 9,000 such publishes an hour. Publish frequency therefore rarely
constrains `worker` size: the worker's other work (function builds, analytics rollups, exports) and
availability do. Two things grow with publishes:

- **Fan-out.** After a publish every active device downloads the delta once. Download volume
  is `active devices × delta size`: the measured single-text-change delta of the loan calculator is
  635 B (`NFR-005`, ≤ 2 KiB), so 1 M devices pull about 0.6 GB; a 50 KiB page change pulls about
  50 GB. Put bundles behind a CDN so that this traffic does not reach `api`.
- **Storage.** See below.

## PostgreSQL

PostgreSQL holds the control plane (organisations, apps, releases, devices, audit) and, in the
community edition, telemetry events and rollups (`ANL-010`). Size connections as
`replicas × maxConnections` (chart default 20 per replica). Sizes of the devices and releases
tables, bytes per raw telemetry event, and rollup growth are **pending** the load run; take them
from `pg_total_relation_size` after a `make load-test` at your scale and extrapolate with the
events-per-day figure above. Raw events are kept 13 months by default (decision Q17), so storage
grows with `events per day × 395 days × bytes per event` until retention starts deleting; shorten
retention or move to ClickHouse before that dominates.

## Valkey

Valkey carries the rate-limit counters, the DPoP replay cache and the telemetry ingestion stream.
Its memory does not grow with history: it is proportional to the devices seen in the replay and
rate-limit windows and to the events queued between ingestion and the worker's bulk write. A 3-pod Sentinel set with 1 to 4 GiB covers every tier here.
Its availability gates `NFR-040`: run it with Sentinel and TLS as the chart does.

## Object storage

Releases are stored once per version as a bundle plus deltas from the previous ten versions:

```
bytes ≈ Σ over apps and retained versions ( bundle size + Σ over ≤ 10 predecessors ( delta size ) )
```

Bundle size is bounded by the limits registry (default 20 MiB per plugin bundle, 100 MiB per app
release); typical bundles are far smaller (the loan calculator at 50 pages is 61 KB, a text-only
delta 635 B). With 100 apps, a 5 MiB release and 50 retained versions the bound is
`100 × 50 × 5 MiB × (1 + deltas as a fraction of the bundle)`, about 25 GiB for bundles plus the
deltas, which a changed-page-only publish keeps to a fraction of that. The chart's bundled SeaweedFS is for development and
small sites; use an external S3 for production.

## ClickHouse switch-over point

Telemetry analytics run in PostgreSQL partitions and rollups until the event volume makes that the
bottleneck. [ADR-0080](../adr/0080-telemetry-and-analytics-storage.md) recommends moving to the
ClickHouse store of the enterprise edition at roughly **10 million events a day or 1 million
active devices**, whichever comes first.

With the assumptions above (48 events per device and day) the event threshold is reached at about
**210,000 active devices**; the device threshold governs only apps that send fewer than ten
events per device and day. Count your own stored events per day rather than devices. Signs it is
time: raw-event partitions dominate the database size, the worker's bulk writes fall behind
(`plux_jobs_queue_depth` grows through the day) or analytics queries in Studio exceed their time
budget. The control plane stays in PostgreSQL; how events migrate on switch-over is decided in
the ClickHouse milestone (ADR-0080).

## Availability (`NFR-040`)

99.95 % a month for the manifest and control endpoints is about 22 minutes of unavailability. The
reference deployment meets it by design, not by size alone: three `api` replicas across zones,
CloudNativePG with synchronous replication, Valkey with Sentinel, a CDN that keeps serving
bundles, and devices that run their cached release when the server is unreachable (`G5`). The
measurement is the availability SLO of [ADR-0085](../adr/0085-slos-dashboards-and-alert-rules.md)
on the installation's own probes. Control changes reach online devices within 60 s p95 only with
silent push; see [NFR-024](../benchmarks/load.md#nfr-024-control-propagation).

## Verify your own numbers

1. Run `make load-test` on a staging stack of the tier's size, with `LOAD_DEVICES` and
   `LOAD_SPEEDUP` chosen so the aggregate rate reaches the tier's peak requests per second
   (`devices × 0.0217 × speedup`). A run is valid when no operation has errors, and p99 for the
   manifest check stays within 50 ms.
2. Watch PostgreSQL, Valkey and object-storage utilisation in Grafana during the run. Saturation
   of any of them means you measured that component.
3. Run `make bench-control` to check propagation on your push configuration.
4. In production, track `plux_rpc_duration_seconds`, `plux_jobs_queue_depth` and
   `plux_control_propagation_seconds` against the SLOs, and re-size when the 30-day trend crosses
   70 % of a replica's measured ceiling.

## Full run (Q20): pending

The maintainer's full run on production-class hardware replaces the derived rows of the tier
tables. Until it is committed the rows below stay empty; nothing in this guide is a measured
result for the 1 M tier.

| Tier | Hardware | Devices simulated | Peak req/s sustained | Manifest p99 | Telemetry p99 | Control p99 | Errors | PostgreSQL size at end | Date and commit |
|---|---|---|---|---|---|---|---|---|---|
| 10 k | pending | | | | | | | | |
| 100 k | pending | | | | | | | | |
| 1 M | pending | | | | | | | | |
