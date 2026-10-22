<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Phase 10 analysis — Dev app and live debugging

Status: analysis for the maintainer, written 2026-10-22 on `feat/phase-9`. It is not the plan:
the plan (`docs/plans/p10.md`) follows the maintainer's answers to section 4. Sources: spec
§5.1 (P10 row), §20, §6.4, §6.7, §1.4, the P10 rows and the earlier-phase rows that name P10,
ADR-0031, 0035, 0042, 0045, 0048, 0069, 0075, 0082, and the code named in section 2.

## 1. Scope

**Goal (spec §5.1, P10, verbatim):** "The ten-minute quick start (`HST-034`, `DX-001`); Plux
Dev app; QR pairing; live draft push with incremental compile; streaming of logs, action
traces (process flow), state, network, function calls, sync and performance; on-device
inspect mode; mocks and network simulation; `plux_devtools` for developers' own host apps
(debug builds only)."

**Exit criteria (verbatim):** "Edit-to-device latency target met; a Studio-less developer can
debug a flow using the CLI viewer; devtools proven absent from release builds."

Appendix K counts 20 P10 rows: 18 `MUST`, 2 `SHOULD`. All 20 are `SPEC` today.

| ID | Pri | Summary |
|---|---|---|
| `CMP-051` | MUST | Incremental compilation recompiles only changed pages and their dependants. |
| `HST-034` | MUST | `flutter create` to a published page on a device in at most 10 minutes (recorded sessions). |
| `DX-001` | MUST | The same bound, seen from the quick-start guide. |
| `FN-036` | MUST | Device function invocations appear in dev-session traces. |
| `DEV-001` | MUST | Dev app: Flutter host on `plux_flutter` and `plux_devtools`, Android and iOS, every first-party optional package. |
| `DEV-002` | MUST | QR pairing: token at most 2 min, single use, signed, bound to Studio user, app and development environment; several devices per session. |
| `DEV-003` | MUST | Dev sessions only for non-production environments; dev bundles signed with development keys that production runtimes reject. |
| `DEV-010` | MUST | Draft edits reach paired devices within 500 ms p95 of Studio autosave, incrementally compiled, hot-applied with state kept where the state schema is compatible. |
| `DEV-011` | MUST | Studio opens any screen on a chosen device with parameters, locale, theme, text scale and data-source mock state. |
| `DEV-020` | MUST | Devices stream logs, action traces as a process flow, state changes, network calls, function calls (server logs merged by trace ID), sync events, frame timings, errors with node paths. |
| `DEV-021` | MUST | The stream is filterable, searchable, pausable, exportable (trace file) and survives flaky networks with bounded buffering. |
| `DEV-030` | MUST | Inspect mode: tap a widget on the device to select the node in Studio; select in Studio to highlight on the device. |
| `DEV-031` | MUST | Edit state values, switch mocks and their states, simulate network (offline, 2G, 3G, latency, loss), fake the clock, trigger lifecycle events, from Studio. |
| `DEV-032` | SHOULD | PXL console against the live state of a device. |
| `DEV-033` | SHOULD | Device screenshots capturable into Studio. |
| `DEV-034` | MUST | Performance and accessibility overlays (frame chart, rebuild counters, touch targets, contrast) toggleable on the device. |
| `DEV-040` | MUST | A developer's own host app pairs in debug builds with `plux_devtools`, with the Dev app's capabilities. |
| `DEV-041` | MUST | `plux dev logs` streams the same data in a terminal. |
| `DEV-050` | MUST | `plux_devtools` excluded from release builds by compile-time constants and tree shaking; CI inspects a release build (`SEC-074`). |
| `NFR-030` | MUST | Edit to paired device at most 500 ms p95 (`DEV-010`). |

Earlier-phase rows that P10 completes or advances (their Status stays as the row says until
P10 lands):

- `ACT-030` (`WIP`): traces stream to paired Studio sessions; ADR-0045 keeps it `WIP` until `DEV-020`.
- `DAT-080` (`WIP`): mocks "for Studio, the Dev app and tests"; ADR-0048 keeps it `WIP` for the Dev app and Studio.
- `SEC-074` (`WIP`): the release-build inspection is `DEV-050`.
- `A11Y-005` (`WIP`): "the Dev app and Studio provide a text-scale preview".
- `I18N-007`, `I18N-010` (`WIP`): their Studio and Dev-app parts (P8 plan D1); `I18N-010` needs a pseudo-locale preview on a device.
- `ANL-040` (`WIP`): Studio link; not P10 (P11).
- `WGT-031`, `STU-066` (P11): consumers of `DEV-033` and `DEV-020`.
- `SYN-014` (`SHOULD`, `SPEC`): deferred to P10 "with the other host tooling" (WORKLOG, P3 closing decisions).
- Router-adapter flavours in `plux_go_router` and `plux_auto_route` examples: deferred to P10 (WORKLOG, P4).
- `SEC-021` already lists "dev session" among the DPoP-bound requests; `PLX-9200` "dev session expired" is reserved in Appendix F.

## 2. Current state per requirement

Nothing of the dev session exists on the server, in the proto or in the CLI. The device side
has the diagnostics seam and an overlay. Verified by grep on 2026-10-22:
`grep -ril "devsession" proto backend schema packages` finds no source file; there is no
`backend/internal/devsession`, no `plux dev` or `plux watch` command, no `apps/plux_dev`.

| ID | State | Evidence |
|---|---|---|
| `CMP-051` | **Absent.** The compiler is one-shot over a whole unit. Development mode exists (`Mode` Release/Development, source map embedded in dev bundles, `CMP-041` `DONE`). Nothing caches per-page results or tracks dependants. | `backend/internal/compiler/compiler.go` (`Mode`, `Options`), `unit.go`, `refgraph.go` (reference graph: the likely dependants index), `resolve.go`. `backend/internal/schema/load.go:244` has a single-node incremental *validation* only. `plux build --dev` in `backend/cmd/plux/compile.go:129`. |
| `HST-034`, `DX-001` | **Absent** (no guide timed, no sessions). P4 delivered the parts: `plux init`, `codegen`, `create`, `starter` app, `make dev`, `dev-starter`, `dev-app` (DEP-020). | `apps/starter`, `mk/stack.mk:42-65`, `backend/cmd/plux/cmd_init_host.go`, `cmd_create.go`; ADR-0042. |
| `FN-036` | **Absent.** Server-side function logs and traces linked to the device request exist (`FN-032` `DONE`); the device-side function host exists; nothing emits a function-call trace event. | `packages/plux_flutter/lib/src/functions/`, `backend/internal/fnhost`, `fnrunner`. |
| `DEV-001` | **Partial.** `apps/plux_viewer` is a host (sign-in by device grant, link receiver, pinned root keys, reconstruction opener) that "P10 grows into the Dev app". `apps/README.md` still lists a separate `plux_dev/`. No QR scanning in the viewer; `packages/plux_scanner` (QR scanner widget over ML Kit/Vision) exists but is `publish_to: none` and is a plugin-facing package. No first-party package list is wired into a host. | `apps/plux_viewer/README.md`, `apps/README.md`, `packages/plux_scanner/pubspec.yaml`. |
| `DEV-002` | **Absent.** The ingredients exist: a `development` signing token class with its own key (`TokenDevelopment`, `token-development.p256`, PKCS#11 and Vault variants), DPoP on every device request (`SEC-021` `DONE`), device authorisation grant. No pairing token, no pairing RPC. | `backend/internal/signing/token.go`, `token_file.go`, `token_vault.go`; `backend/internal/dpop`. |
| `DEV-003` | **Partial.** Key separation and "production runtime rejects development-signed bundles" is `SEC-056` `DONE`. Nothing restricts session creation to non-production environments (no session exists). The environment model exists (`internal/release`, `internal/server`). | `backend/internal/signing/*`, `SEC-056` row. |
| `DEV-010` | **Absent.** No push channel, no relay, no hot-apply. Runtime state is scoped Riverpod providers (`src/state`), so preservation is feasible, but nothing computes "state schema compatible" between two compiled units. | `packages/plux_flutter/lib/src/state/`, `compiler/state.go`, `typecheck.go`. |
| `DEV-011` | **Absent** (server relay, device command, parameters/locale/theme/text-scale/mock override). The navigation, locale and theme layers take explicit inputs, so an override seam is plausible. | `src/navigation`, `src/l10n`, `src/theme` (via render), `src/data/mocks.dart`. |
| `DEV-020` | **Partial, device in-process only.** `PluxDiagnostics` exposes `log` (problems), `release`, `syncStatus`, `actionTraces` as `ValueListenable`s; traces are a bounded ring buffer with redaction (`ACT-030/031`). Frame timings are read in `plux_view.dart:483` for telemetry only. No state-change, network-call, function-call or per-node-path error stream; no serialisation; no transport; no server-log merge. | `packages/plux_flutter/lib/src/devtools_api/diagnostics.dart`, `actions/trace.dart`, `core/plux_view.dart`, `telemetry/recorder.dart`. |
| `DEV-021` | **Absent.** Bounded buffering exists for telemetry and the outbox (`ANL-002` `WIP`), not for a dev stream. | — |
| `DEV-030` | **Partial (compiler half).** Dev bundles carry the node-index to JSON-path source map (`CMP-041`). The renderer has no hit-test to node mapping and no highlight. ADR-0031 §"Layout and paint errors" already says P10 devtools attribute Flutter errors to Plux nodes. | `compiler/encode.go`, `bundle/container.dart` (source map section), `render/plux_node.dart`, `render/renderer.dart`. |
| `DEV-031` | **Partial.** `DataMocks` and states (`loading`, `empty`, `error`, `success`) exist (`DAT-080`); `ActionClock` is an injectable clock; `internal/netsim` simulates a network path (bandwidth, RTT, loss; `Slow3G` profile; HTTP proxy, TCP forwarder) but only as a server-side benchmark proxy. No on-device network simulation, offline switch, state editing, clock faking at runtime or lifecycle triggering. | `packages/plux_flutter/lib/src/data/mocks.dart`, `actions/clock.dart`, `backend/internal/netsim/{netsim,proxy,forward}.go` (no `doc.go`; package doc in `netsim.go`). |
| `DEV-032` | **Partial.** The PXL interpreter is in the runtime (`src/pxl`); no REPL entry point against live state. | `packages/plux_flutter/lib/src/pxl/`. |
| `DEV-033` | **Absent.** | — |
| `DEV-034` | **Absent** (overlays). The compiler already computes contrast and touch-target checks (`a11y*.go`, `A11Y-*`), which the overlay can reuse as thresholds. | `compiler/a11y.go`, `a11y_contrast.go`. |
| `DEV-040` | **Partial.** `packages/plux_devtools` 0.1.0 is a debug overlay (sync status and button, active release and plugins, reported problems), 315 lines, renders nothing in release. It uses only public diagnostics. Pairing, streaming and commands are absent. | `packages/plux_devtools/lib/src/devtools_overlay.dart`, `README.md`. |
| `DEV-041` | **Absent.** The CLI has `build --dev` and `mock` (an OpenAPI mock server, `TST-004`, unrelated to dev sessions); no `dev` command. | `backend/cmd/plux/main.go:132` command table, `cmd_import.go:140`. |
| `DEV-050` | **Largely present, to be extended.** `make release-inspect PLATFORM=android\|ios` builds the starter and Plux Bank in release and fails on devtools code, dev-only logging, source maps, debug files; CI runs it ("Verifies: SEC-074, DEV-050", `.github/workflows/ci.yml` around lines 1819 and 1913). `SEC-074` stays `WIP` because the inspection covers `plux_devtools` as it is today (an overlay), not a transport; the Dev app and the new hooks widen what must be absent. | `mk/bench.mk:89-93`, `.github/workflows/ci.yml:1743-1913`. |
| `NFR-030` | **Absent** (no benchmark). `netsim`, `loadgen` and the `bench-*` targets are the models. | `mk/bench.mk`, `backend/internal/netsim`. |

Spec layout already reserves the homes: `backend/internal/devsession/` ("pairing, live relay
between Studio and devices", §6.3 line ~490), `devtools_api/` in the runtime (§6.4 line ~527),
the `api` module's "dev-session relay" (§6.1), and `PLX-9200`. The Dev-session sequence is
§20 (create session, QR, pair with DPoP and dev attestation, document patch, compile changed
pages on the worker, push sections over WebSocket, hot-apply, stream back).

## 3. Gaps and work items (candidate milestones)

Naming follows P9 (E0…E12) with prefix `D`. Sizes: S under a day of agent work, M a few days,
L a week or more of several agents.

| # | Milestone | Closes | Touches | Needs | Size |
|---|---|---|---|---|---|
| D0 | Spec 1.9.0, ADRs, contracts, dependencies, limits | prerequisite for all | `docs/`, `proto/`, `schema/limits.json`, `dependencies.md` | decisions 4.1–4.6 | M |
| D1 | Incremental compiler | `CMP-051` | Go `compiler` | none (parallel with D0) | M |
| D2 | Dev session service: pairing, relay, push | `DEV-002`, `DEV-003`, server half of `DEV-010`, `DEV-011` | Go `devsession`, `api`, `signing` seam, `storage` | D0, D1 | L |
| D3 | Runtime dev channel and hot apply | runtime half of `DEV-010`, `DEV-011`, `DEV-031` (state, mocks, clock, lifecycle) | Dart `plux_flutter` `devtools_api` | D0 | L |
| D4 | Device streams | `DEV-020`, `DEV-021`, `FN-036`, `ACT-030` | Dart (`devtools_api`), Go (`fnhost`/`api` merge by trace ID) | D2, D3 | L |
| D5 | Inspect, overlays, console, screenshots, network simulation | `DEV-030`, `DEV-034`, `DEV-032`, `DEV-033`, network part of `DEV-031` | Dart `plux_devtools`, runtime hooks | D3, D4 | L |
| D6 | `plux_devtools` pairing UI and own-host pairing | `DEV-040` | Dart `plux_devtools`, `starter` | D3 | M |
| D7 | Dev app | `DEV-001` | `apps/plux_viewer` → Dev app, QR scan, package set | D6 | M |
| D8 | CLI: `plux dev logs`, trace export, `dev watch` | `DEV-041`, CLI half of `DEV-021` | Go `cmd/plux` | D2, D4 | M |
| D9 | Release-absence gate and `NFR-030` benchmark | `DEV-050`, `NFR-030`, `SEC-074` | Makefile, CI, Dart, Go | D3–D7 | M |
| D10 | Quick start and usability sessions | `HST-034`, `DX-001` | docs, `starter`, `make` targets | D7, D8 | M |
| D11 | P10 carry-overs (optional) | `SYN-014`, router flavours | Dart native code | none | M |
| D12 | Gates, statuses, close-out | all | repo | all | S |

Notes per milestone:

- **D0.** Spec rows to touch: §20 (transport wording, decision 4.1), Appendix F (`PLX-9200` family: pairing token expired, reused, environment not allowed, session ended), Appendix K counts, `NFR-030` method. New proto `plux/v1/devsession.proto` (create session, pair, patch, command, list devices, end), registered in the authorisation matrix with new permissions (`devsession.create`, `devsession.pair`; deny by default) in `permissions.json`. Limits (registry, never hard-coded): token TTL (120 s ceiling from `DEV-002`), devices per session, session lifetime, stream buffer entries and bytes per device, event size, commands per minute. ADRs: dev-session transport and trust (4.1, 4.2), dev-key bundle acceptance on a device, stream event schema.
- **D1.** Per-page content hash cache keyed by page source plus the hashes of everything it references (use the reference graph in `refgraph.go`), invalidating dependants (widgets, components, themes, translations, data sources, state schema, navigation). Output must stay byte-identical to a full compile (`CMP-002`): the test is "incremental equals full" over a generated edit sequence (property/fuzz test), plus a unit test per dependency edge. The compiler stays a pure library (L-5): the cache is an injected value, owned by the caller (worker or CLI). Also exports the state-schema compatibility verdict for hot-apply (D3).
- **D2.** Package `internal/devsession` (spec §6.3), handlers thin (L-1). Pairing token: signed with the development token key through the `signing` seam (L-3), single use (consumed in a transaction), at most 120 s, bound to user, app and environment; device pairs with DPoP and, where available, dev attestation (`SEC-021`). Refuses production environments (`DEV-003`). Compile happens on the worker or `builder`-style isolation: L-4 means the compile step gets no signing capability, so signing the dev bundle sections is a separate call into `signing` with the development class. Relay: authenticated, bounded per-device queues, backpressure, fan-out to several devices. Audit every state change. Step-up for creating a session when the organisation requires it (ADR-0075 follow-up; decision 4.5).
- **D3.** A device-side client in `devtools_api` (not in the UI isolate: L-6; network, verification and patching run in a background isolate). It verifies dev bundles before load (`SEC-052`) against pinned development keys, applies sections to the live release without touching the production store (a session overlay, discarded on end), and re-renders keeping scoped state when the compiler's compatibility verdict allows, else resets the page and says so. Commands: open screen with parameters/locale/theme/text scale/mock state, set state value, switch mock and mock state, fake clock, lifecycle event.
- **D4.** A typed event schema (log, trace, state diff, network, function, sync, frame, error with node path) with redaction at the source (`ACT-031`, `SCH-012`: sensitive values never leave the device, even in development), bounded ring buffers with drop counters, resumable by sequence number, a pause that keeps buffering to the bound. Server-side function logs merged by trace ID at the relay (the `FN-032` trace link already exists). `ACT-030` becomes `DONE` here.
- **D5.** Inspect: hit-test that maps a render object to the node index and, with the source map, to the JSON path; highlight overlay. Network simulation on device: a debug-only wrapper at the runtime's HTTP client seam applying offline, latency, bandwidth and loss profiles; reuse the profile definitions of `netsim` (move the shared `Profile` values into a registry or schema file so Go and Dart agree, rather than copying numbers). Frame chart and rebuild counters from `FrameTiming`; touch-target and contrast highlights reuse the compiler thresholds. PXL console and screenshots are `SHOULD` and can be last.
- **D6/D7.** `plux_devtools` gains the pairing client and a commands/stream adapter over the D3 API; the Dev app is a host that pins development root keys and installs every first-party optional package (`plux_db_drift`, `plux_lottie`, `plux_rive`, `plux_maps`, `plux_maps_google`, `plux_charts`, `plux_components`, `plux_markdown`, `plux_media`, `plux_location`, `plux_player`, `plux_scanner`, `plux_svgc`, `plux_security`, router adapters as applicable) and QR scanning.
- **D8.** `plux dev logs [--app --env --device --filter --json --since]`, `plux dev trace export <file>`; the trace file format is shared with `DEV-021` and the Studio export. The viewer connects as a developer client of the same relay with a `devsession.observe` permission.
- **D9.** Extend `release-inspect` to the Dev-app-free hosts with the new devtools symbols (transport, event schema, hit-test, simulator) and add an iOS leg; add a benchmark `bench-devloop` measuring autosave (patch RPC accepted) to device-applied callback with the P95 over at least 200 edits, over a netsim "good Wi-Fi" profile.
- **D10.** Written against D7 and D8; the maintainer tests by hand first (ADR-0042); the recorded sessions are a human activity that no agent can run.

Dependencies: D1 ∥ D0; D2 and D3 after D0 (D2 also D1); D4 after D2+D3; D5, D6 after D3 (D5 also D4); D7 after D6; D8 after D2+D4; D9 after D3–D7; D10 after D7+D8; D12 last. Critical path: D0, D2/D3, D4, D5, D9. Suggested waves: (D0, D1), (D2, D3), (D4, D6, D8), (D5, D7), (D9, D10, D11), D12.

## 4. Conflicts and decisions for the maintainer

**4.1 Transport of the live channel (WebSocket versus what the stack has).**
- *Conflict:* §20 names WebSocket for the push. The server has no WebSocket library; `dependencies.md` has none, and the runtime's pubspec has `http`, `cronet_http`, `cupertino_http` only. The ConnectRPC streams in use are server-streaming over HTTP/2 (`ExportDraft`). The `api` also sits behind ingress and the gateway that verify DPoP per request (`SEC-021`); a long-lived socket authenticates once at upgrade.
- *A:* WebSocket on the server through a new allowlisted dependency (ADR first; `coder/websocket`, licence to confirm), `dart:io` `WebSocket` on the device (SDK, no new Dart dependency). Matches the spec; per-message DPoP replaced by a session-bound token (needs an ADR line).
- *B:* ConnectRPC bidirectional-over-HTTP/2 or server streaming plus unary commands; no new Go dependency but the Dart side needs a streaming client over `package:http`, which Cronet/URLSession wrappers do not give bidirectionally.
- *C:* Server-Sent Events down, unary POST up. Smallest, DPoP per request, reuses the SSE client from the data layer; weaker for high-rate device-to-server streams.
- *Recommend A.* It is what the spec says, the cost is one reviewed dependency; B and C change §20. Needs maintainer approval (AGENTS §7, dependency).

**4.2 How a device accepts a dev bundle without weakening verify-before-load.**
- *Conflict:* `SEC-052` requires signature and hash verification of metadata, manifest and bundles before anything is loaded; a draft is not a release (no TUF metadata chain, no sequence number, `SEC-055` anti-rollback). `DEV-003` says dev bundles are signed with development keys; `SEC-056` says production rejects them.
- *A:* The relay emits a mini release (manifest plus sections) signed with the development targets key; the Dev app and debug host builds pin a development root (`PLUX_DEV_ROOT_KEYS`, like the viewer's `PLUX_VIEWER_ROOT_KEYS`); the session overlay never writes the production store and uses a separate sequence space. Production builds do not compile the code path in (`DEV-050`).
- *B:* Allow unsigned dev sections over the authenticated socket. Rejected: contradicts `SEC-052`.
- *Recommend A.* Needs an ADR; touches signing and key handling, so AGENTS §7 requires approval.

**4.3 Studio halves of the requirements versus P11.**
- *Conflict:* `DEV-010`, `DEV-011`, `DEV-020`, `DEV-021`, `DEV-030`, `DEV-031`, `DEV-032`, `DEV-033` are written from Studio's point of view ("from Studio autosave", "Studio MUST be able to…"); Studio is P11 (`STU-066` bottom drawer). The P10 exit criterion needs only the CLI viewer. P9 precedent: `ANL-040` stayed `WIP` for its Studio half.
- *A:* P10 delivers device, server relay, CLI and a Go/Dart test client playing Studio; rows with a Studio-only clause stay `WIP` until P11, with the split recorded in the plan (as P9 did).
- *B:* Re-tag the Studio clauses to P11 by splitting rows (`DEV-010a/b` style), as `FN-035`/`FN-036` and `STU-xx` were split before; P10 rows can then be `DONE`. Requires spec change and, for `MUST`, an ADR.
- *Recommend B for `DEV-011`, `DEV-030`, `DEV-031`, `DEV-032`, `DEV-033`* (the Studio interaction is the Studio feature; the device capability is the P10 row), and A for `DEV-010`/`DEV-021`, which are properties of the channel. Trade-off: B is more spec work now, but makes `make trace` honest.

**4.4 `NFR-030` measured without Studio.**
- *Conflict:* "from Studio autosave" cannot be timed before Studio exists, and a device farm is unavailable (ADR-0035: free emulators/simulators only, not representative of radio latency).
- *A:* Define the measured interval as "patch RPC received at the server to section applied and rendered on the device runtime", over a netsim good-Wi-Fi profile, with a Dart VM client harness; add the autosave debounce and Studio render budget in P11.
- *B:* Wait for P11. Delays an exit criterion.
- *Recommend A,* with the method written into the `NFR-030` row (spec edit, maintainer decides). Server-side budget suggestion: incremental compile at most 100 ms p95 for a one-page edit of a 50-page plugin, sign and enqueue at most 20 ms, leaving at least 300 ms for network and device.

**4.5 Dev-session creation policy.**
- ADR-0075 left "step-up for dev sessions in P10". Options: (A) no step-up (non-production only, token short-lived); (B) step-up when the organisation's policy requires it for environment access. *Recommend B* (policy-driven, default off in the community edition): GOV-032 forbids edition-gating a security capability, so it is core. Also decide the roles: new `devsession.create` for developer and above, `devsession.observe` for CLI viewers.

**4.6 Dev app: grow `plux_viewer` or add `plux_dev`.**
- `apps/plux_viewer/README.md` and `docs/plans/p9.md:761` say the viewer grows into the Dev app; `apps/README.md` lists `plux_dev/`. They disagree.
- *A:* Rename `plux_viewer` to `plux_dev` and keep reconstruction viewing as a mode (one signed-in host, one set of pinned roots). *B:* Two apps sharing a small package. *Recommend A* (the sign-in, link handling and root-key pinning are the same code); update `apps/README.md`, the viewer README, the ADR-0082 reference and release/CI paths in the same PR. The viewer pins production roots, the Dev app development roots: the two must not share a binary's trust set. If that is judged to matter, choose B. *This is a real trade-off for the maintainer.*

**4.7 Smaller points**
- `FN-036` also needs the server-log merge: relay-side join by trace ID against `fnrunner` logs; confirm the relay may read function logs for the session's own app only.
- `DEV-001` says "every first-party optional package": `plux_scanner`, `plux_security` and the native packages are `publish_to: none` or have native code; confirm which are in the Dev app, and whether `plux_security` (RASP) runs in the Dev app (it would block debugger and emulator use; ADR-0057 to be read in the plan).
- `SYN-014` (background sync, native code) and the router-adapter flavours are deferred to P10 in the WORKLOG but are not P10-tagged rows: include (D11) or defer to P11/P12? *Recommend defer `SYN-014` again* (`SHOULD`, needs WorkManager/BGTaskScheduler and a dependency) and do the router flavours only if the quick start needs them.
- Appendix K says 18 `MUST` + 2 `SHOULD`; the row list above has 18 and 2 (`DEV-032`, `DEV-033`) — consistent.

## 5. Risks

- **Latency (`DEV-010`/`NFR-030`, 500 ms p95).** Budget pieces: autosave debounce (Studio, P11), RPC, incremental compile, dev signing, push, device-side verification and patching (off the UI isolate, L-6), re-render. The full-compile cost today for large units is the unknown: `compiler/bench_test.go` and `budgets_test.go` are the starting point and need an incremental variant. Hot-apply with state preservation can be slower than a restart if the state-schema diff is conservative. A first measurement belongs in D1, before the service is built.
- **Security.** (1) Debug features must be absent from release (`DEV-050`): compile-time constants (`kDebugMode`/`bool.fromEnvironment`) with tree shaking must remove the transport, command handler, simulator and hit-test; verify in the symbol inspection per platform and add canary strings. `dart:io` `WebSocket` and the isolate code must be behind the same guard. (2) A device that can be driven remotely (set state, fake clock, run PXL) is a remote-control surface: pair only in debug or Dev-app builds, pinned dev keys, session-bound, audited, rate-limited, ended on token expiry. (3) The pairing token is a bearer secret in a QR: single use, at most 120 s, bound to user/app/environment, never logged (`SEC-092`); rotate and hash at rest. (4) Streams carry data: redaction at the source, sensitive fields never leave the device even in development (`ACT-031`), bodies and headers redacted by default with an explicit opt-in. (5) Dev bundles and production runtimes: negative tests that production rejects dev-signed sections (`SEC-056`) and that the dev root is not compiled into release builds. (6) Several devices per session: the relay must authorise per device, not per session. (7) DoS: bounded queues and a registry-limited rate per device. (8) SSRF/egress: the relay talks only to devices and the worker, not to arbitrary URLs.
- **Platforms.** iOS: camera permission and QR scanning in the Dev app, local-network permission when pairing over LAN, background suspension drops the socket (resume by sequence number). Android: cleartext only in debug flavours if LAN is allowed; `RT-002` floors (API 24, iOS 15) with the emulator limits recorded in ADR-0035 (API 26+ in CI). Screenshots (`DEV-033`) need `RepaintBoundary` capture; platform views (native slots, maps) do not capture, which is the `WGT-031` case Studio wants.
- **Frame-time overhead.** Streaming must not disturb the measurements it reports: event encoding on a background isolate, sampling when paused; state-diff computation bounded by the limits registry.
- **Determinism.** Incremental output must equal full output byte for byte (`CMP-002`); the cache must key on content hashes, never wall clock or map order.
- **Usability sessions (`HST-034`).** They are human; the milestone can prepare the guide and a timing harness (scripted dry run with `make`), but the status can only move after the recorded sessions. Risk: ten minutes includes a toolchain install on a cold machine; the guide must state what is excluded.
- **CI.** New jobs: `release-inspect` on iOS (needs macOS runner, already used by e2e); a `bench-devloop` job (Linux, Dart VM harness, netsim); device e2e of pairing on emulator/simulator (free runners only, ADR-0035); path rules in `ci/` for `apps/plux_dev` and `devsession`; coverage floors for new packages (not lowered; AGENTS §7). Flutter-tool flakiness on iOS (ADR-0035) applies to any simulator test of the Dev app.
- **Dependencies.** WebSocket library on the server (4.1); QR scanning in the Dev app (`plux_scanner` is not published); any Dart package needs the allowlist or an ADR.
- **Scale of the Dart side.** Roughly half of the work is Dart in `plux_flutter` and `plux_devtools`, a package the P9 hand-off treats as released at 0.6.0: new public API in `devtools_api` must be additive and debug-gated; plan a version bump for `plux_flutter` and `plux_devtools`.

## 6. Carry-overs from P9

From `docs/WORKLOG.md` (latest note, 2026-10-22 session 3) and `docs/plans/p9-handoff/README.md` §9–§10:

- **Viewer host grows into the Dev app** (ADR-0082; `docs/plans/p9.md:579,761`; `apps/plux_viewer`): see 4.6. The viewer pins production root keys at build time (`PLUX_VIEWER_ROOT_KEYS`, lead fix at P9 review); the Dev app needs its own development-root pinning, not the server's keys. "Reconstruction and experiment previews" are named in p9.md:761 as Dev-app features: reconstruction mode exists, experiment preview does not.
- **Step-up for dev sessions** (ADR-0075 Follow-up): decision 4.5.
- **Statuses P10 can move:** `ACT-030`, `DAT-080`, `SEC-074`, `A11Y-005`, `I18N-007`, `I18N-010` (their P10 halves). `ANL-040`, `WGT-031` stay with P11.
- **Open from the P9 hand-off that touch P10 areas:**
  - §9 item 7: the ten limit defaults, the ADR-0066 native-slot locale rule, the legal review of `ee/LICENSE`. Not P10 work, but the native-slot locale rule shows in the Dev app's locale preview (`DEV-011`).
  - The walkthrough job needs a WebAuthn driver (`tools/internal/webauthndriver`) and an enterprise image; a dev-session walkthrough should reuse it for step-up.
  - `DEP-004`/`DEP-010` kind runs never ran in agent sessions: the Dev app's server dependencies (relay behind ingress, long-lived sockets through the chart's NetworkPolicy and ingress timeouts) are not covered by any chart test; add idle-timeout and ingress annotations for the relay in the Helm chart and check the NetworkPolicy allows device egress to the relay.
  - §10 follow-ups (ClickHouse write gate, airgap verify key, reconstruction retention, rollout attributes, experiment promotion atomicity, chart KMS values): not P10.
- **Process lessons to apply (hand-off §6):** agents reformatted generated files (use `make gen` only); migration numbers collide (next free is 0044 on `feat/phase-9`; D2 needs its table for sessions and pairing tokens); keep briefs direct and short (AGENTS §6); the tool-sandbox note: agent sessions cannot run Docker/kind or OpenTofu, so D9/D10 CI jobs are unverified until a dispatched CI run.
- **Not verified at P9 close** (so P10 starts on an unproven base): `make e2e-starter`, `go-vuln`, a dispatched CI run on `feat/phase-9`, the 40 race runs. The release inspection (`DEV-050`'s existing gate) runs in the same device jobs; confirm it is green before extending it.
