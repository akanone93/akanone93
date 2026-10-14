<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# P7 — Plux Functions

Results of Phase 7's function benchmarks, with their method (DoD-4, `QA-007`). The targets are
set for the reference deployment (server) and the mid-tier reference device, which CI and this
repository's cloud container do not have. Every number below comes from that container (Intel
Xeon @ 2.80 GHz, 4 cores, Linux x86-64; 2026-10-14), which other jobs were using at the same
time (load average 3 to 8 on 4 cores). The numbers say how the code behaves on a busy shared
host, not whether the reference deployment or a phone meets the target.

| Requirement | Target | Command |
|---|---|---|
| `NFR-022`, `FN-023` | Server invocation overhead: warm ≤ 1 ms p99; first after deploy ≤ 50 ms p99 | `make bench-fn` |
| `FN-056` | A typical calculation in ≤ 20 ms p95 on the mid-tier reference device | not run: waits for the reference device |
| `FN-050`, ADR-0011 | WAMR against `wasmi` on size and per-call time | measured below, by hand |

## Server invocation overhead (`NFR-022`)

**Method.** `make bench-fn` runs `TestFunctionInvocationOverhead_NFR_022`
(`backend/internal/fnrunner/bench_test.go`) with `PLUX_BENCH_FN=1`; without it the test skips.
It builds a real `Runner` against the api role's `FunctionHostService` double over mutual TLS
and invokes the conformance probe's echo entry (`schema/testdata/functions/probe.wasm`, 75 KB,
entry 0), so the guest does no work of its own and the time is the runner's: concurrency slot,
request checks, cache lookup, a fresh instance with its linear memory, the call, result decoding,
metrics. The limits are the registry defaults (64 MiB, 2 s). `wazero` 1.12.0, compiler mode, Go
1.27.1. The transport in front of the runner (ConnectRPC from the api role) is part of no
number. Times are wall clock around `Runner.invoke`; the pinned connection to the api role is
established before the first sample.

| Measurement | Samples | What it times |
|---|---:|---|
| `warm` | 2,000 after 200 warm-up | A call on a compiled, cached module, one at a time. |
| `firstAfterDeploy` | 100 | A new runner with an empty cache: fetch from the api role, verify (`FN-009`), compile, then the call. Gated. |
| `firstAfterPrepare` | 100 | A new runner after `Prepare` compiled the module (the deploy path of `FN-010`): the first call. Not gated. |
| `firstWithCompilationCache` | 100 | A new runner whose `CacheDir` already holds the compiled code (a restart or a second replica). Not gated. |

The test prints the four as JSON (p50, p99 and maximum, in microseconds) and fails when `warm`
p99 exceeds 1 ms or `firstAfterDeploy` p99 exceeds 50 ms. The CI sync benchmark job runs it
after `bench-sync`.

Three consecutive runs with the default garbage collector:

| Measurement | p50 (µs) | p99 (µs) | Budget p99 |
|---|---|---|---|
| `warm` | 232 to 432 | 6,395 to 9,972 | 1,000 |
| `firstAfterDeploy` | 56,099 to 73,464 | 81,822 to 116,367 | 50,000 |
| `firstAfterPrepare` | 406 to 448 | 5,884 to 10,829 | none |
| `firstWithCompilationCache` | 8,718 to 12,740 | 17,147 to 31,718 | none |

**Verdict: both budgets are missed on this host.** Three observations bear on what to do about
it before the reference deployment is measured.

- **Warm p50 is within budget, the tail is the collector.** Every call allocates the guest's
  initial linear memory (1.1 MiB, 17 pages, a Rust module's default stack) and discards it, so
  with a small live heap the collector runs every few calls. A diagnostic run with `GOGC=off`
  gave warm p50 63 µs and p99 556 µs, under the budget even on the busy host; `GOGC=400` gave
  254 µs and 2,692 µs. A production server's larger live heap lowers the collector's
  frequency per allocation, so the benchmark probably overstates the tail, but only the
  reference deployment shows by how much. Reusing the instance memory across calls would
  remove the allocation, at the price of zeroing it by hand to keep `FN-010`.
- **First after deploy is the compile.** 50 to 70 ms of the median is `wazero` compiling the
  75 KB probe (the profile puts 55% of the run in `CompileModule`, mostly register allocation).
  Plan §2.3 measured 50 ms for an 82 KB Rust module, so the budget sits at the compile time
  of a small module, with no margin for a larger one. After `Prepare` has compiled it, or with a
  populated compilation cache, the first call costs 0.4 ms and 9 to 13 ms. Which of the three
  `FN-023` means by "first invocation after deploy" is the maintainer's reading to confirm; the
  test gates the strictest.
- **Noise.** The p99 of 100 first-call samples is the slowest sample, and the host was shared.
  The reference deployment's number replaces these.

## WAMR 2.4.3 against `wasmi` on the host (D2)

**Method.** The same module, `packages/plux_functions/test/fixtures/loan_module.wasm`
(153,307 bytes; built from `sdk/fn/examples/loan` by `loan-module`), called through its
ABI on a fresh instance each time (instantiate, `_initialize`, `plux_alloc`, `plux_invoke`, read
the result), the host imports answered as the device host does for a pure function (`context`
returns a four-field object, the rest unavailable). 200 warm-up calls, then 2,000 timed calls;
five alternating runs of 1,000 each give the ranges below. The load of the module is timed once
per run.

- **WAMR side:** the library built exactly as `packages/plux_functions/hook/build.dart` builds
  it (same sources, defines and flags, `-O3`, clang 18.1.3), driven through the engine's own
  `pf_module_load` and `pf_invoke` from a C program. That includes the engine's limits
  (2 s, 32 MiB), the WASI shim and the watchdog bookkeeping.
- **`wasmi` side:** `wasmi` 2.0.0 (the latest stable), Rust 1.97.0, `opt-level = 3`, LTO,
  `codegen-units = 1`, `panic = "abort"`, default features, the default engine configuration.
  A minimal harness with the same ABI calls and no limits layer, so its size is a lower bound for
  an equivalent engine and its time omits that layer's cost. The code is a measurement in the
  scratch directory, not in the repository (decision D10).
- Emulators are not available in this environment, so no Android or iOS number was taken; the
  host is x86-64 and the ratios may differ on arm64.

| | WAMR 2.4.3 fast interpreter | `wasmi` 2.0.0 |
|---|---:|---:|
| Native library, stripped (`strip --strip-unneeded`) | 200,328 B | 2,335,200 B (2,418,816 B at `opt-level = "s"`) |
| Native library, gzip -9 | 87,005 B | 850,977 B |
| Load of the 153 KB module | 6.4 to 11.7 ms | 1.2 to 1.8 ms |
| 3-month schedule: call p50 / p95 | 241 to 392 µs / 482 to 556 µs | 89 to 98 µs / 127 to 161 µs |
| 360-month schedule: call p50 / p95 | 12.6 to 14.6 ms / 20.2 to 24.4 ms | 2.8 to 2.9 ms / 4.3 to 4.7 ms |

The 360-month input is `{"principal":"250000","annualRate":"0.05","months":360}` and produces
34,835 bytes of output; the 3-month one is the example's first stored case. WAMR's fixed cost per
call (about 0.2 ms) shows in the 3-month row. The library sizes compare the two engines with
their thin layers, not the engines alone.

**Reading.** `wasmi` is about 4.5 times faster on a compute-heavy call and 3 to 4 times on a
small one, and about 12 times larger on disk. WAMR here uses software bounds checks (the
build's `WASM_DISABLE_HW_BOUND_CHECK`, chosen so the host process installs no signal handlers),
which costs it speed that a hardware-checked build would recover. On the 360-month call WAMR's
p95 of 20 to 24 ms is at the `FN-056` target of 20 ms for the device, on a host faster than a
mid-tier phone; the ≤ 10,000-operation calculation `FN-056` names is much lighter than that
call. These are host numbers; `FN-056` is settled by the device run below.

## Device numbers

`FN-056` (and the device side of `FN-053`) are measured on the mid-tier reference device and
wait for it (plan p7 C5). The requirements stay `WIP` until then. Nothing above substitutes
for that run.
