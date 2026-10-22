# P9 Load and Control-Propagation Benchmarks

Measurements for `DEP-051`, `NFR-023` (scale), `NFR-040` (availability) and `NFR-024` (control
propagation), with the method needed to repeat them. The sizing guide that uses them is
[docs/operations/sizing.md](../operations/sizing.md).

## Status of the numbers

| Measurement | Status |
|---|---|
| Control propagation, polling and with silent push (`make bench-control`) | Measured, below |
| Scaled load run against the Compose stack (`make load-test`) | **Pending.** The machine of this first run has no Docker daemon, so the command is documented and the nightly `Load` workflow produces the numbers |
| Full run on maintainer hardware (Q20) | **Pending**; fills the top rows of the sizing guide |

No number in this file is copied from another source or estimated: a row is either measured on the
machine named next to it or marked pending.

## The load generator

`plux-load` (`backend/cmd/plux-load`, package `backend/internal/loadgen`) runs N synthetic
devices against a Plux server. Each device does what the runtime does:

1. registers with development evidence (the server needs `attestation.developmentProvider`, so the
   environment must not be a production one) and obtains a DPoP-bound token, which it renews
   three quarters into its life (the `token` operation; tokens last minutes, `SEC-020`),
2. at start: checks the manifest, reports `session_start` for the release it names, fetches the
   signed control document; every event carries the release the device runs,
3. then keeps to its pacing, each interval jittered by ±20 %:

| Operation | Interval at real pacing | RPC |
|---|---|---|
| Manifest check (`If-None-Match`) | 10 min | `ManifestService/GetManifest` |
| Control document (`If-None-Match`) | 5 min | `ManifestService/GetSignedControl` |
| Telemetry batch (two screens, one action) | 1 min | `TelemetryService/IngestEvents` |
| Function call (only with `-function`) | 5 min | `FunctionService/Invoke` |

When a device sees a control document newer than the one it holds it sends `control_applied` at
once; that event is what the server's `plux_control_propagation_seconds` histogram measures.
Nothing a device sends comes from a person or a real device: identifiers are random, event fields
are constants.

`-speedup K` divides every interval by K, so a few hundred devices can stand in for K times as
many in the same time. The report gives each operation's rate at the wall clock (`ratePerSecond`)
and at real pacing (`realRatePerSecond`, the first divided by K), request counts, error counts by
Connect code, and p50, p95, p99 and maximum latency in milliseconds (histogram buckets 4 % wide).
The process exits 1 when more than `-max-error-ratio` (0.1 %) of the requests fail.

```bash
make load-test                                  # 1,000 devices, 5 min, 10x pacing, Compose stack
make load-test LOAD_DEVICES=5000 LOAD_DURATION=10m LOAD_SPEEDUP=20
```

The target starts the stack with `deploy/compose/compose.load.yaml` (which trusts Docker's bridge
as a proxy so that each device names its own address in `X-Forwarded-For`, keeping the per-address
and per-device rate limits as in the field), seeds it on first run and writes the JSON report to
`deploy/compose/.secrets/load-report.json`. `LOAD_OUT` changes the path.

### Extrapolation method

A scaled run measures the server's response at an arrival rate, not at a device count. Convert in
both directions with the real-pacing mix of one foreground device:

```
requests per second per foreground device
    = 1/600 (manifest) + 1/300 (control) + 1/60 (telemetry) [+ 1/300 (function)]
    = 0.0217 /s without functions, 0.0250 /s with them

foreground devices served at a measured aggregate rate R = R / 0.0217
```

The result is then divided by the share of active devices that are in the foreground at the same
time (the concurrency ratio). Scaling up is valid while these hold, and the report is read against
them:

- the scaled run's error ratio is zero and p99 stays within the target of the operation
  (`NFR-020`: 50 ms for the manifest check);
- PostgreSQL, Valkey and object-storage utilisation are read from the stack's Grafana during the
  run: a run that saturated one of them measures that component, not the server;
- per-replica rates are measured with the replica pinned to the cores the reference replica has
  (2 vCPU), as in [p2-backend.md](p2-backend.md), and scale linearly with replicas only up to the
  database's limit, which is why the sizing guide sizes PostgreSQL separately.

## NFR-024: control propagation

**Target:** a control-document change reaches ≥ 95 % of online devices within 60 s.

**Workload.** A fleet of synthetic devices that have applied the current control document; the
test then changes it through the CLI (`plux control kill` / `unkill` of a plugin switch) and each
device's `control_applied` event for the new version is timed from the moment the command was
issued. Two modes, each on a fresh fleet:

- **poll:** devices fetch the control document every 15 s (the interval is shortened from the
  production 5 min so that the run is short; the result scales with the interval);
- **push:** silent push is enabled; the server sends one message per registered push token to a
  fake FCM endpoint, which wakes the device's control check as the host app does on a silent push
  (`SYN-060`). The fake stands in for FCM's network and delivery time only.

The server records every device's time in `plux_control_propagation_seconds{app}` (reports later
than ten minutes are not counted: that device was offline). The test asserts that the histogram
holds a sample per device and mode, that the push p95 is under 60 s and below the polling p95, and
that the polling p95 is within one and a half poll intervals plus 5 s.

```bash
make test-db                                    # then export what it prints
make bench-control                              # PLUX_BENCH_DEVICES=20 PLUX_BENCH_POLL=15s
PLUX_BENCH_CONTROL_OUT=control.json make bench-control   # also writes the JSON
```

**Machine.** The development container of [p2-backend.md](p2-backend.md) (4 vCPU, 16 GiB RAM,
Linux 6.18, Go 1.27.1, PostgreSQL 16), server and generator in one process, measured on
2026-10-21 at commit `ad9b4d7`.

| Mode | Devices | Poll interval | p50 | p95 | max | Pushes sent |
|---|---|---|---|---|---|---|
| Polling only | 20 | 15 s | 7.4 s | 14.3 s | 15.0 s | 0 |
| Silent push (fake FCM) | 20 | 15 s | 1.6 s | 1.6 s | 1.6 s | 20 |

**Reading.** With polling a change arrives after half the interval on average and one interval at
most, so at the production interval of 5 min the p95 is about 4.75 min: polling alone does not
meet `NFR-024`; silent push does, with 58 s to spare in this setup. The push figure is the
server's own time (change, queueing, delivery to the fake) plus one control fetch; real FCM and
APNs add their delivery latency, which the provider does not guarantee and which the nightly run
on the emulator fleet (plan X3) and `plux_control_propagation_seconds` in production show. Devices
without a push token (push disabled, token not granted) fall back to polling: the SLO of
[ADR-0085](../adr/0085-slos-dashboards-and-alert-rules.md) is therefore stated for online devices
with silent push.

## NFR-023 / DEP-051: scaled load

Pending (see Status). When the nightly `Load` workflow or a maintainer runs `make load-test`, the
report's per-operation rates, latencies and error counts go into this table, with the commit, the
machine and the stack's resource limits.

| Date | Commit | Devices | Speed-up | Duration | Operation | Real-pacing rate | p50 | p95 | p99 | Errors |
|---|---|---|---|---|---|---|---|---|---|---|
| pending | | | | | | | | | | |
