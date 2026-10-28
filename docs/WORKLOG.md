# Work Log

Current focus, hand-off notes and open decisions. This file lets any contributor — or an AI agent starting a fresh session — pick up the work exactly where it stopped.

Per-requirement implementation status is **not** tracked here; it lives only in the `Status` column of [requirements.md](requirements.md) (spec §4.3).

---

## Current focus

| Item | Value |
|---|---|
| Phase | P11 — Plux Studio, **in progress on `feat/phase-11` (started 2026-10-24)**; P10 is merged to `main` ([#44](https://github.com/nightCode42/plux3/pull/44)) |
| Active branch | `feat/phase-11`, pushed; agent worktrees under `.claude/worktrees/` (branches `p11/<slice>`) |
| Active work | Hand-off to session 2: [plans/p11-handoff/README.md](plans/p11-handoff/README.md) (wave 7 at ~80 %) |
| Requirement IDs | P11: [plans/p11.md](plans/p11.md) §3, §5; statuses are set at the phase exit (S9.7) |

## Next up

**P11 (in progress):** [plans/p11.md](plans/p11.md) on `feat/phase-11`; agents per [sub-agents.md](engineering/sub-agents.md) with the model tiers of M-10; the latest hand-off note says what is merged and what runs.

**P9 CI (current):** continue from [plans/p9-handoff/ci-handoff.md](plans/p9-handoff/ci-handoff.md) (2026-10-23): the CI state, the method, what is done and what is left. The earlier P9 hand-off follows.

**P9:** continue from [plans/p9-handoff/README.md](plans/p9-handoff/README.md) (hand-off 2,
2026-10-21): §6 says how to start a session and run the agents, §5 lists the next agents in
order, §7 the lead's own work, §9 the decisions waiting for the maintainer. The plan is
[plans/p9.md](plans/p9.md). The P8 text below is history.

**P8 (current):** delivered on `feat/phase-8`; the exit gate and the maintainer's review remain
(latest hand-off note). [plans/p8.md](plans/p8.md) holds the decisions and the hand-offs to later
phases (§9).
The P7 text below is history.

**P7 (current):** follow [plans/p7.md](plans/p7.md) from §0. Functions are written in Rust
(plan §2.1 D1, measured in §2.3); the decisions of §2 are final. Milestones F0–F9 (§6), each
ending at the milestone exit gate (§11). The text below is the P4–P6 history of this section.

P4 is delivered as milestones R0–R11 ([plans/p4.md](plans/p4.md) §6), each ending at
the milestone exit gate below; R0 and the following milestones merge together on the
session branch (plan §2.1, A10).

**Milestone exit gate** (maintainer, 2026-09-29): a milestone ends only when (1) every
`make` gate passes locally with its exit code checked, not its output; (2) the end-to-end
test passes repeatedly (at least 40 runs, with the race detector) and every intermittent
failure is root-caused, never retried away; (3) a dispatched CI run is green in every job,
including the image checks on amd64 and arm64; (4) the milestone's open items are fixed or
put to the maintainer as decisions; (5) the hand-off note and the report list what was
verified and how.

1. **R0 — Fast CI** (done, reviewed): affected-only selection, Makefile fragments, the
   runtime benchmark in three parts, device builds during boot, caches (ADR-0043).
2. **R1 — Decisions and contracts** (done, reviewed; A16–A18 applied).
3. **R2 — Action engine core** and **R3 — Routing core** (done, reviewed; A19–A21 applied).
4. **R4 — guards, deep links, push, auth and user context**, **R5 — router adapters** and
   **R6 — native catalogue** (done; their shared exit gate passed 2026-10-02, A25–A29
   decided).
5. **R7 — mixed screens and the host API**, **R8 — codegen and init**, **R9 — `plux
   create`**, **R10 — add-to-app** and **R11 — close the phase** (done, with one exit gate
   at R11; the maintainer decided its open items, A34–A39).
6. **P5 — Actions, state, data, local database and animation** (delivered 2026-10-06, R0–R12,
   [plans/p5.md](plans/p5.md)): batch 1 (R0–R4), wave A (R5–R8), R9 (tooling), R10
   (reference apps), R11 (gates, benchmarks) and R12 (guides, references, threat model,
   runbook, statuses). Next: the maintainer's review of the pull request, the
   reference-device run (D3) and the open items of the first hand-off note.

### P4 approvals (maintainer, 2026-10-01)

The plan is approved, with the approvals recorded in its §2.1: R0's gate changes; no new
CI action (caches use `actions/cache`); Android 26 narrowed to `main`, the daily and manual
runs and Android-specific changes; `analyzer` widened to `plux_native_scan`; the §4.3
contract additions; coverage floors of new packages at the repository defaults. The
maintainer aims to handle as much as possible in this phase.

At R0's review the maintainer accepted its recommendations (plan §2.1 A8–A10): the iOS
device job runs on pull requests for the runtime, the starter and iOS-specific files, not
for server-only changes; plan §7.2 records what R0 did; `CI-002` is `DONE` with R0. R0
merges together with the following milestones, on the same branch.

At the start of R1 the maintainer settled five gaps in the plan's contracts (plan §2.1
A11–A15): the app document's `hostEvents` (`HST-013`'s declaration; P4 delivers the
plugin-to-host half); `NAV-010`'s custom timeline deferred to P5 and documented; an
optional `result` type on pages; a registry `GuardResult` with its enum `GuardDecision`;
PXL's built-in `user.authenticated`.

At R1's review (2026-10-02) the maintainer accepted its three recommendations (plan §2.1
A16–A18): `stop` moves to P4, so a guard graph returns its `GuardResult` with it
(ADR-0039 and ADR-0040 accepted); the R1 notes of plan §4.3 stand; `plux_flutter` becomes
0.2.0 with R2. R2 and R3 follow together.

At R2 and R3's exit gate (2026-10-02), with CI measuring the armeabi-v7a APK at 6.61 MiB,
the maintainer raised the Android size budgets (plan §2.1 A19): ≤ 10 MiB per APK and
≤ 4 MiB per App Bundle download, per ABI; the IPA stays ≤ 3 MiB (spec 1.2.1, ADR-0036
Revision, [size journey](benchmarks/size.md) round 2). The runtime is not trimmed to fit.

At R2 and R3's review (2026-10-02) the maintainer accepted their recommendations (plan
§2.1 A20, A21) and asked that documents always match what is implemented: time waiting
for the user in a dialog or sheet counts against no action bound, as implemented; the
specification's version header stays at 1.1.9 until R11 sets it to the latest revision
(R11 set it to 1.2.2).

During R4–R6 (2026-10-02) the maintainer decided (plan §2.1 A22, A23): router adapters
wrap the host's router, `PluxConfig(router: PluxGoRouter(myGoRouter))`, through a core
`PluxRouterAdapter` interface; a kill switch's declared fallback page that is guarded
gives way to the generic fallback; the app bundle carries the native catalogue's
declarations (A24).

At R4–R6's exit gate (2026-10-02) the maintainer decided (plan §2.1 A25–A29): starter
flavours per router are deferred to P10, each adapter keeping its `example/main.dart`; the
CI licence allowlist's new entries are confirmed; `PluxConfig.hostBuild` has no default
while the CLI defaults to the `pubspec.yaml` version; `plux.yaml` stays on the CLI's
standard-library parser; the three new packages stay unpublished for now.

During R7–R11 (2026-10-02) the maintainer decided A30–A33 (plan §2.1): `plux init` detects
a host app by its directory; `plux codegen` reads a local project in P4; component events
as built; the add-to-app hosts' dependencies. At P4's close (2026-10-03) the maintainer
decided R11's open items (plan §2.1 A34–A39): JUnit 4 accepted for the Kotlin host's tests
only ([ADR-0044](adr/0044-junit-4-for-the-android-host-tests.md)); a custom action named
like a built-in action is a compile error (`PLX-1124`); Dependabot for the Android host is
deferred; the iOS host project stays committed as before `pod install`; `plux create --git`
validates its remote and branch now; the size baseline moves to round 3.

### P3 scope decisions (maintainer, 2026-09-28, approving the P3 plan)

- **Scope boundary.** P3 renders; it does not act or navigate. Events are wired as no-ops reporting `PLX-4010` in debug builds; bindings read parameters, declared initial page state, translations and the theme; `Plux.open` pushes one page on the host navigator. `nativeRoutes`, `nativeSlots`, `nativeActions`, `events` and exposed-state writes arrive in P4/P5, so `HST-001` stays `WIP`. DPoP (P6) means the DPoP part of `ANL-002` stays open; root rotation (P6) keeps `SEC-051` `WIP`; `QA-010`'s "last three runtimes" starts with the first tag.
- **Dependencies approved:** `flutter_riverpod` (ADR-0008); `cryptography` for Ed25519 and `crypto` for SHA-256 (ADR-0029); vendored zstd C decoder built by a Dart build hook (ADR-0030); `http` with `cronet_http` and `cupertino_http` (ADR-0021); `vector_graphics` on the device and a Dart helper in the publish worker for `CMP-031` (option (a); ADR-0027 Revision); Starlight on Bun for the docs site (ADR-0033). The maintainer asked for the worker helper and its C dependency to be built optimised, with its size reported: see the R1 note below.
- **Decisions of 2026-09-29** (maintainer, on the open questions of R5): a publish waits for the app's pending assets (option 4; ADR-0027 § Publish); icon fonts by an in-house Go subsetter with Material Symbols and Cupertino icons embedded in the server (option (a); ADR-0032 § Icons); one server image for all roles (ADR-0027 Revision); layout and paint errors are left to Flutter's containment and the host's `FlutterError.onError` (option A; ADR-0031 Revision, `RT-020` reworded); `adaptive` is a prop on the eight widgets with `.adaptive` constructors (option A; ADR-0031 Revision, `WGT-011` reworded); the two P1 compiler bugs found in R5 are fixed; the legal review and CLA automation stay open. After the R6 hardening: `OfflineBanner.visible` true (the default) shows the banner only while the device is offline, false hides it; the check-added-large-files exception for the server-only Material Symbols font and `plux-svgc`'s transitive `xml` and `petitparser` are approved. Later the same day: computed state stays in P5 with every other state feature (option B), so the loan calculator's `calculator` page shows its fallback in P3 (noted in ADR-0031 § What P3 does not do); wazero's version-cache race is worked around in `internal/wasmrt`.
- **Device testing:** no paid service and no local emulators. Android emulator and iOS simulator tests and the benchmarks run only in GitHub Actions on free runners; cloud sessions never download or start emulators; reference-device `NFR` numbers are recorded later from manual runs by the maintainer, so those `NFR`s stay `WIP` until then.
- **Publishing:** `CI-005` through pub.dev automated publishing with GitHub OIDC (the maintainer enables it in the pub.dev admin); the docs site deploys to GitHub Pages.
- **AVIF:** the runtime asks for WebP variants and uses AVIF only where the device reports support (Android 12+, iOS 16+); if AVIF is never used after this, the AVIF encoder is revisited (P2 note).

### P3 closing decisions (maintainer, 2026-10-01)

- **Size (`RT-061`, `NFR-009`):** ≤ 3 MiB added to what a device downloads (App Bundle split per ABI, thinned IPA), ≤ 6.5 MiB per APK; all seven builds gated ([ADR-0036](adr/0036-size-budgets-per-build.md), [size journey](benchmarks/size.md)). Raised on 2026-10-02 to ≤ 4 MiB per App Bundle download and ≤ 10 MiB per APK, the IPA unchanged (P4 approvals above, A19). No reduction that adds work for developers; Cronet stays; optimization is parked for a later round.
- **Up-to-date check (`NFR-006`):** one manifest request, ≤ 1 KiB of bodies; the device sends a digest of its installed bundles ([ADR-0037](adr/0037-up-to-date-check-installed-digest.md)).
- **Flutter support (`RT-002`):** the latest stable and the previous one, from 3.47 on; nothing is downgraded to reach 3.44 ([ADR-0038](adr/0038-flutter-support-window.md)). When the next stable is released, CI gains a job on the previous stable.
- **Pub points (`RT-001`):** published packages declare caret ranges, except `flat_buffers`, pinned with `flatc` (ADR-0002); the policy check enforces it, the lockfile fixes what we build.
- **Cross-phase `MUST`s stay `WIP`:** each is marked `DONE` only when the phase that completes it does — `HST-001` (native routes, slots: P4; actions, events: P5), `RT-015` and `ANL-001` (action and data-source events: P5; RASP: P6; functions: P7; experiments: P9), `RT-021` (actions: P5), `ANL-002` (DPoP: P6), `SEC-051` (root rotation: P6), `WGT-012` (data sources: P5), `LIM-004` (outbox: P5), `RT-060` (optional packages, with their features), `DX-002` (action catalogue: P5; whitepaper and compliance pack: P6/P9; use-case guides: P4), `RT-016` (Studio canvas: P11), and the reference-device numbers (`RT-004`, `NFR-001`–`003`, `NFR-008`, `NFR-041`). Their P3 parts are implemented and tested. Each later phase's plan lists the ones it completes.
- **Deferred `SHOULD`s (spec §4.4):** `SYN-014`, opt-in background sync, is deferred to P10 with the other host tooling: it needs WorkManager and BGTaskScheduler native code, a second entry point and a new dependency or platform code. `WGT-021`, heavy Layer 2 components in optional packages, is met by construction while none exists and is checked in P5, when Lottie and Rive arrive.
- **Device farm (`QA-006`, stays `WIP`):** Firebase Test Lab on the no-cost Spark plan, in a later phase, not a P3 priority. Spark allows 10 virtual and 5 physical device runs a day (up to four devices each; reset at midnight Pacific). It needs a Google Cloud project and a service-account secret, an Android instrumentation wrapper for the flows (the iOS XCTest bundle of `ios/RunnerTests` runs there as it is), and a staging server the farm's devices can reach, since the CI runner's server is local. A nightly workflow then covers a low-end Android and API 24.
- **Device tests in CI:** Android 26 and 35, and one iOS job on the newest simulator under XCTest ([ADR-0035](adr/0035-device-tests-in-ci.md)).

### Cross-phase items P3 finishes (or finishes its part of)

`SEC-052` and `BND-006` (Dart verifier); `BND-008`, `BND-018` (runtime refuses unknown required features, tolerates unknown sections); `SEC-054`, `SCH-012` (runtime part); `QA-002` (activation half); `QA-004` (Dart manifest and bundle parsers fuzzed); `LIM-001`, `LIM-004` (runtime reads limits from the signed app bundle); `REL-080` (device features from telemetry); `CMP-031` (SVG to `vector_graphics`); `DEP-020` (Flutter hot reload in `make dev`).

### P2 scope decisions (maintainer, 2026-09-27; revised 2026-09-28)

- **Signing backends (2026-09-28)**: all of PKCS#11 (a separate helper process, so the server binary stays cgo-free for `CI-006`), AWS KMS, Google Cloud KMS and Azure Key Vault are supported, arriving in P6 with the key ceremonies of `SEC-121`; P2 ships Vault Transit and the development file backend, so `SEC-120` stays `WIP`.

- **Asset transcoding**: libwebp and libavif compiled to WebAssembly and run on `wazero` (no cgo, so the reproducible distroless build is kept); dotLottie written directly in Go. `CMP-031` (SVG → `vector_graphics`) is deferred to P3, where the Dart side that consumes it exists; `CMP-032` (a `SHOULD`) to P8, where the locales that decide the script ranges exist. Both stay `SPEC` with the reason recorded here.
- **Studio-side security**: P2 delivers the server half of `SEC-100` and `SEC-101` — built-in accounts with Argon2id, OpenID Connect sign-in, TOTP and WebAuthn second factors, sessions and CSRF tokens; the browser half (nonce CSP, Trusted Types) and the Studio service in the compose stack stay `WIP` until P11. At the phase review ADR-0026 proposed moving OIDC and WebAuthn to P9; the maintainer declined on 2026-09-28 and kept them in P2 (ADR-0026, Revision). SAML, provisioning and group mapping (`GOV-004`) and WebAuthn step-up (`SEC-103`) remain P9.
- **Cross-phase parts**: `REL-003` validates every reference kind that exists (function references arrive in P7, native routes in P4), `REL-080` computes compatibility from registrations until device telemetry arrives in P3, and `LIM-005` is readable through the API but shown in Studio only from P11. All three stay `WIP`.
- **ADR-0004 in P2**: the whole design is recorded now; P2 implements the `targets` role only — one Ed25519 key per environment behind the signing abstraction, signing bundle hashes and manifests. Roles, thresholds, rotation and confidential bundles arrive in P6, so `SEC-050`, `SEC-051`, `SEC-053` and `SEC-055` stay `SPEC`.

## Open decisions

| Decision | Owner | Notes |
|---|---|---|
| P11 design-gate rules off for the approved design | Maintainer (review later) | **2026-10-26: accepted for now, to be revisited.** The design gate (O-4, gating from S3) ignores three Impeccable rules by rule because they flag approved Porcelain tokens: `undersized-ui-text` (the 10px `type.tag`, U7), `dark-glow` (`shadow-primary`, U7), `pulsing-dot` (the live-device dot, U10). The detector has no value-scoped ignore on URL scans. Revisit if the tokens change (spec Appendix J) or the detector gains value ignores; config `studio/.impeccable/config.json`, reason in `studio/scripts/design-gate.ts`. |
| P9 open items | Maintainer | **2026-10-21: the recommendations of hand-off 2 §9 items 1–5 are accepted** (to apply: README §9); the limit-key defaults, the ADR-0066 rule and the `ee/LICENSE` legal review stay open. Listed with the lead's recommendations in [plans/p9-handoff/README.md](plans/p9-handoff/README.md) §9: SCIM deprovisioning and user-wide sessions (ADR-0074); the chart's `networkPolicy.externalHTTPS.enabled` default (`true` today; recommend `false`, a security default); CI jobs for IaC and the air-gapped kind install; `skopeo` and `registry:2` for `make airgap-kind` (`dependencies.md`); the smaller items the lead accepted; the P9 limit-key defaults; the ADR-0066 native-slot locale rule; legal review of the draft `ee/LICENSE`. |
| P8 open items | Maintainer | All planning and review decisions are taken (plan §2, D1–D17, A1–A3; 2026-10-17/18 decisions recorded in the hand-off notes and ADRs 0051, 0053, 0068–0070). Open: the device evidence of plan §3.2 (live TalkBack/VoiceOver, native-speaker review of `ar` and `am-ET`, device locale switching, ML Kit vs ZXing and per-ABI scanner sizes), listed in the real-device checklist's P8 section. |
| P7 real-device run | Maintainer | [real-device-checklist.md](security/real-device-checklist.md) P7 items: F1 the Plux Bank fee function's success path (SCA, App Attest/Play Integrity evidence, tier claim; CI only checks the refusal of a development device), F2 the Express loan timings that decide WAMR against `wasmi` (`FN-056`, M8), F3–F4 device confirmations; plus the P6 items carried in the same file. |
| Legal review of ADR-0022 and the CLA | Maintainer | Before accepting external contributions and before the first public release. |
| CLA signing automation | Maintainer | e.g. CLA Assistant with signatures stored in a separate branch; needed before external pull requests are merged (`GOV-034`). |
| Android hosts need `android.uniquePackageNames=false` | Maintainer (follow-up) | Needed on Android Gradle plugin 9 while Play Services Cronet ships `cronet-api` and `cronet-shared` with one namespace, `org.chromium.net`; documented in the host guide, set in the starter and size apps. Agreed: report it upstream to `cronet_http` (dart-lang/http) and drop the line when it is fixed. |
| Baseline format | Maintainer (informational) | `plux pull` will also write each bundle's signature and the asset files the release uses into the baseline (additive), so the runtime can verify baseline bundles (ADR-0029) and render assets offline (ADR-0027 Revision). |
| Dependabot for the Android add-to-app host | Maintainer (deferred, plan p4 §2.1 A36) | Dependabot does not watch `apps/add_to_app/android_host`'s Gradle files, so its androidx.test and Kotlin Gradle versions get no update pull requests; add the `gradle` ecosystem for that directory when the maintainer asks. |

## Hand-off notes

Newest first. Keep the latest ten; move older notes to [worklog-archive.md](worklog-archive.md) unchanged.

- 2026-10-28 (P11 S8.1 server part, branch `p11/s81-apps`) — **App metadata filters, bounded summaries, the `.plux` archive, shell jobs.** *Changed:* new `AppProjectService` (`proto/plux/v1/app_project.proto`: `ListAppSummaries`, `ExportApp`, `ImportApp`, `DuplicateApp`, `ListSampleApps`, `ImportSampleApp`, `GetShellStatus`, `SetShellSettings`, `GenerateProject`, `GetProjectJob`, `DownloadProject`); `ListAppsRequest` gains `status` and `mine`; packages `internal/archive` ([ADR-0102](adr/0102-plux-archive.md); deterministic stored ZIP, untrusted import, three embedded samples built from `schema/testdata/documents`), `internal/shellupdate` (GEN-004 comparison, queued generation job on the `export` queue); `tenancy.ListAppSummaries`; migration 0062 (`app_shell_settings`, `project_jobs`, `project_job_results`); limits `app.summariesPerCall`, `archive.bytes`, `archive.entries`, `archive.expandedBytes`, `project.bytes`, `project.jobsPerHour`; audit actions `app.exported`, `app.imported`, `app.duplicated`, `app.archived`, `shell.settings_set`, `project.generated`. *Already existed (not rebuilt):* app metadata and archive state (0057), `ListApps` tags/team/query/sort, plugin duplicate and archive, `ListPluginSummaries`. *Not done:* Git push of the generated project (the only Git client, `gitsync`, is bound to export settings and its SSRF proxy, so it is not reusable as is), the baseline release and the icon inside the generated project, a sweep that deletes old job results. *Next:* Studio client for the new RPCs (S8.1 web part). Samples are rewritten with `go test ./internal/archive -run Samples -update`.
- 2026-10-27 (P11 S7.4, branch `p11/s74-approvals`) — **Approvals area and the policy simulator.** *Changed:* `ApprovalService.SimulatePolicy` (additive; read-only; `policies.manage`; the draft stands in for the policy of its key, the policies in force that also match are listed; no write), `backend/internal/approval/simulate.go`, authz matrix entry, integration tests incl. a cross-organisation refusal; Studio `features/approvals/` (O7 inbox, O8 request with diff, reports, golden images, install base, changelog, decisions and audit sequence, D47 decide with comment and step-up, O9 history, O10 break-glass reviews with step-up, O11 policies, O12 editor with simulator), routes under `/o/:org/approvals`, en/de/ar. *Open:* audit sequences are shown as text until O27 (`/admin/audit`) exists; the inbox `?mine` filter compares the requester with the user id; statuses are set at the phase exit (S9.7).
- 2026-10-27 (P11 session 1 end) — **Hand-off package for a new session: [plans/p11-handoff/](plans/p11-handoff/README.md) on branch `handoff/p11-session2`.** *State:* `feat/phase-11` @ `daa1d1c8` pushed with waves 1–6 and most of wave 7 (S4.5, S5.1–S5.3, S5.5, S5.6, the S5.4 server bridge) merged; about 62 % of P11. *Open branches:* `p11/s55c-app-lock` (finished, `.po` conflicts only), `p11/s54b-rust-editor` and `p11/s46c-rest` (agents stopped, WIP saved, unreviewed). *Decided (maintainer):* model tiers — Haiku and Sonnet low by default outside the five Opus parts (M-10 revised, sub-agents.md); `function.deploy` gates the Rust tools; limit `lsp.messageBytes`; CSP `style-src-attr 'unsafe-inline'` for Monaco (ADR-0014 revision); design-gate rules off for approved tokens (to revisit); structural design-time rules (document-model §4.1). *Verified:* see the package's verification.md (studio-check 2,947, e2e 19, design gate, conformance, dart-analyze, gen/spec/go-lint, Go race tests). *Next:* the package's next.md.
- 2026-10-26 (P11 day 4) — **Wave 6 merged: S3.1–S3.5, S4.3, S4.4 and all of S4.5 on `feat/phase-11`.** *Merged:* S3.1 app canvas C1 on Plux Canvas (plugin boxes, badges, frames, native nodes, accessible box list, drag/snap/marquee, dagre via `@dagrejs/dagre` 3.1.1), migration 0057 app/plugin metadata with org-scoped team and icon checks, `ListPluginSummaries`; S3.2 bundled arrows (routing, lanes, lane menu, low-zoom), drag p95 16.67 ms and lift first frame 5 ms at 200 boxes / 1,000 links; S3.3 ghost screens and native nodes with thumbnails; S3.4 link creation (Actions tab, drag handle, trigger picker E7, keyboard path); S3.5 `ApplyChangeSet` (multi-document, multi-plugin, one undo step), where-used index, blocked deletes with fixes, type-to-confirm, rename warning, move screen, trash and deprecation, `TestRedirectLoops` (STU-077); S4.3 Material and S4.4 Cupertino composites; S4.5 all 58 Layer 2 components with goldens written by the runtime itself (`packages/plux_flutter/test/render/layout_goldens_test.dart`), the scanners checked as a box only. *Decided (maintainer):* new limits `studio.changeSetDocuments` 500 (max 5000) and `metadata.*` (description 2000, 20 tags of ≤ 40, icon id ≤ 200); first-party plugin packages as plux_flutter dev_dependencies for the golden writer, plux_scanner excluded (build-hook cycle), scanners checked as a box only. *Verified:* `make studio-check` (2,716 tests), `make studio-e2e` (16), `make studio-conformance` (1,916; goldens regenerated identical), `make dart-analyze`, `make dart-lock-check`, `make gen-check`, `make spec-lint`, `make go-lint`, `go test -race` on tenancy, document, api, compiler. *Running:* S3.2d drop settle and scene A gap (targets missed: gap 66.66 ms vs 50, drop settle 511 ms vs 300). *Open:* TermsViewer's 480 px design-time height stands in for 0.6 of the device height (the screen theme has no height); `calendar-tables.ts` is a test-pinned copy of `schema/l10n/calendars/*.json`, not generated; Material MenuAnchor and the NavigationRail screen builder; LottieView/RiveView lay out 0×0 before load (runtime behaviour); the canvas-click → lane dialog path has no browser test. *Next:* S3/S4 exit gates, then wave 7 (S5, S4.6).
- 2026-10-26 (P11 day 3) — **S0, S1 and S2 complete on `feat/phase-11`; wave 6 (S3, S4.3, S4.4) running.** *Merged:* canvas CPU raster on software GL (M-13 met: pan p95 16.67 ms, longest gap 16.67 ms, visible settle ~40 ms); worker layout with SkParagraph; S4.1 slivers, S4.2 boxes; S2.1 validator (WebAssembly, Problems tab); S2.2 edit pipeline (JSON Patch, conflicts, offline queue; `zustand` 5.0.15, `immer` 11.1.21); S2.3 locks (server takeover rule L-1, migration 0054); S2.4 undo (editor sessions, step ids, migration 0055); S2.5 selection (F5 decision: move/nudge/resize follow the parent); S2.6 panels, chunked uploads (0056, PLX-6032–6037), live canvas with damage-rect redraw (STU-121: 17.2 ms p95); S2.7 screens; `studio-dev` runs BFF + Vite.
  - *Security fixes (maintainer reviews):* sign-in cookie capture made request-scoped (race test); OIDC state secret required and shared across replicas (Helm Secret kept via `lookup`, Compose env, dev per-run secret); one Trusted Types policy `plux-worker` for Studio's own workers (canvas, validator, lock heartbeat); sonner styles shipped without runtime injection; CSP gate served by the BFF in e2e.
  - *Verified:* `make studio-check` (1272 tests), `make studio-e2e` (16), `make studio-conformance` (608), `make helm-lint`, `make gen-check`, `make go-lint`, `go test -race` on document/api/plxerr/storage, `make studio-bench-canvas`.
  - *Open:* Studio Docker image build not run here; sign-in page has no design pass; drag p95 had one run at 18 ms (median 11.8); statuses are set at the phase exit (S9.7).
- 2026-10-25 (P11, S2.6 right dock, branch `p11/s26c-properties`) — **Properties, Accessibility and Docs panels; property-to-canvas benchmark.** *Changed:* `studio/apps/web/src/features/panels/` (right dock, model, messages en/de/ar, tests), one `<RightDock>` element and a page validator hook on the plugin canvas page; `studio/packages/canvas/bench/property.ts` + `propertyToCanvas` scenario; `docs/benchmarks/studio.md`.
  - *Open:* STU-121 / NFR-032 is **not met** in software rendering: p95 101 ms (layout 54 ms) against 50 ms. Needs an incremental relayout of the edited screen in the canvas worker (not in S2.6). Docs tab shows descriptor text only: descriptors carry no links. The size class is chosen in the panel (one device frame exists); the canvas does not yet draw the edited, unsaved documents (screens still come from the server documents).
  - *Not run by this slice:* the left dock (parallel agent).
- 2026-10-25 (P11 day 2) — **Wave 3 merged; S0.6 shell merged; canvas settle within 300 ms (M-13).** *Merged on `feat/phase-11`:* S0.6 session bootstrap (`GetCurrentUser` returns `csrf_token` and `installation_admin`, CSRF-exempt and `no-store`; theme "contrast"), S0.9 asynchronous draft push coalesced per plugin (limit `devSession.draftCompileWorkers`), S1.3 progressive preemptible base re-render, S0.6 command registry/palette/shortcut sheet (`@leeoniya/ufuzzy` 1.0.19 pinned), notifications popover and inbox O14, status chips, help menu, dialogs D3–D8, `/unsupported`, the "Studio updated" banner from the BFF's `/version`, docs links from BFF config `docsUrl` (unset hides them).
  - *Lead fixes at review:* a failed limit read no longer pins the dispatcher to one worker; commands rebuilt on locale change, not every render; docs URL refused with credentials.
  - *Decided:* canvas settle ≤ 300 ms for the visible viewport without a GPU (maintainer, M-13; ADR-0013).
  - *Verified:* `make studio-check`, 561 Studio tests, `go test` of `internal/api`, `internal/preferences`, `internal/devsession` (with `-race` there), `make studio-i18n-check`, `make studio-bench-canvas` (pan p95 16.67 ms, settle visible 133/120 ms; longest gap 583/333 ms against 50 — not met).
  - *Running:* longest-gap fix (second base filled by budgeted jobs); S1.3 rest (hit-testing, selection overlay, context loss, accessible tree, fonts). *Next:* S1.4, S0/S1 exit gates.
  - *Open:* inbox `type` filter is client-side (`ListInboxRequest` has no type); call out the `identity.proto`/`preferences.proto` additions in the PR; CI runs only at the end (M-6).
- 2026-10-24 (P11 day 1) — **P11 started; S0.0, S0.1, S0.8, most of S0.5, S1.1 (box, flex, stack, wrap, text) and S1.2 merged on `feat/phase-11`.** *How:* lead plus short-lived agents in worktrees (sub-agents.md), lowest model per brief (M-10: Opus only for the layout port and conformance, canvas core, arrows, editing core, dev sessions across replicas), token rules and a tool-call cap in every brief; the lead reviewed every diff and fixed what review found (same-origin cookies in `@plux/api`, the generated theme CSS committed, the events stream ending with the session, ported Flutter files under `src/flutter/` with their BSD annotation, a migration renumbered to 0052). *Merged:* spec 1.10.0, ADR-0013/0014/0094–0100 and four revisions, the handbook moves; limit keys and contract fields; the Studio workspace (Vite, Hono BFF skeleton, size check, lint rules and import boundaries, Tailwind from tokens, happy-dom, Playwright with axe, design gate report-only); resumable shared logs in `internal/cache`; session idle on user activity only; PreferencesService; inbox params, grouping and retention; the Studio events stream; the TS permission catalogue; `@plux/api`; `@plux/layout` (box, flex, stack, wrap, text through Flutter's own CanvasKit) with 133 Flutter goldens matching (worst text delta 0 px); the canvas prototype and the no-GPU measurements. *Decided (maintainer):* canvas go (M-11); the canvas must perform without a GPU (M-12, ADR-0013 Revision); dark buttons use dark text on the dark-theme violet (Material 3 tonal practice) and the high-contrast theme meets WCAG AAA with `prefers-contrast` and `forced-colors` (in progress on `p11/s02-tokens`). *Verified:* `make studio-check` (356 tests), the Flutter goldens, the touched Go packages with `-race`, `make reuse-lint`, `make proto-lint`. *Running:* S0.2 tokens fix, S0.9a dev-session state on the shared logs. *Open:* `leadingDistribution.even` baseline unverified against Flutter; the CI studio job needs `flutter precache --web` (S0.7); the scene A no-GPU budget (16.7 ms p95) not yet met — S1.3. *Next:* S0.2 components, S0.3 i18n, S0.4 BFF, S1.3 canvas core, then wave 4.
- 2026-10-24 (night) — **P11 plan accepted: every decision taken; implementation not started.** *Decided:* the maintainer approved the lead's recommendation for O-1–O-16 and O-18 (M-9; O-17, O-19, O-20 were decided earlier, M-6–M-8). *Applied in [plans/p11.md](plans/p11.md):* status Accepted; §2.3 rows carry the decision and the step that carries it out; §4.1 lists the approved spec wording changes for 1.10.0 with their ADRs (`STU-001`, `STU-002`, `STU-006`, `STU-010`, `STU-024`/`055`/`060`, `STU-115`, §21 and §3 tables, `GOV-002`/`003`, `CI-006`, `QA-007`, `library.*` permissions, Appendix J, F, H, K); §4.5 the document corrections (A5.9 versioning, routing without `/setup` and with A45–A47 = 102 routes, F14, F16, F19, U4, U19); `ANL-050` in scope with ADR-0101, and `SEC-161`, `SCH-012` completed by it (§3.3); legal reviews as release conditions in ADR-0013/0014; the render worker image in S7.1; coverage floors and the design-gate detector in S0.1; the supported-server range in S0.4; read-only keys and pins in S8.7. The specification, ADRs and handbook are **not** edited yet: that is step S0.0. *Verified:* every P11 ID mapped, every O-/L-/M-/step reference resolves and every table row has its columns (scripts); `make reuse-lint`, `make docs-site`, pre-commit. *Next:* when the maintainer says to start, S0.0 and S0.8 (plan §0).
- 2026-10-24 (evening) — **P11 plan: maintainer decisions O-17, O-19, O-20 recorded; branch pushed.** *Decided:* CI runs once, fully, at the end of the phase (M-6); every Studio command maps to the server permission of its RPC, a missing one is added with its server check in the same change and listed for approval (M-7, procedure in [plans/p11.md](plans/p11.md) §6.2); dev sessions keep no state only in process memory (M-8). *Added:* step S0.9 (events and device frames on the shared stream layer, resumable on any `api` replica; Studio sessions follow the stored draft; CLI overlays as a PostgreSQL patch log; two-replica tests and benchmark; ADR-0087 Revision); S0.5 builds the shared stream layer (resumable reads on `internal/cache` streams, woken by the existing `cache.Notifier`) and the TypeScript permission catalogue; S0.6's command registry types each command's permission. *Correction to the earlier answer on O-20:* the draft copy goes away for Studio sessions only; CLI sessions keep an overlay of local files, moved to the shared patch log. *Verified:* every P11 ID mapped and every O-/L-/M-/step reference resolves (script), `make reuse-lint`, `make docs-site`, pre-commit. *Open:* §2.3 O-1–O-16, O-18. *Next:* the remaining answers, then S0.0 and S0.8.
- 2026-10-24 (later) — **P11 plan verified against the code and documents; `feat/phase-11` rebuilt on `feat/phase-10-v2`.** *State:* P10's PR #43 was closed unmerged and replaced by [#44](https://github.com/nightCode42/plux3/pull/44) (`feat/phase-10-v2`: the same tree plus `docs/engineering/sub-agents.md`); `feat/phase-11` was re-cut from it and the plan commit cherry-picked (the old local branch kept as `backup/feat-phase-11-on-p10v1`; nothing pushed — the push to the session branch was refused). *Checked:* every Connect service mounted in `internal/server/services.go` against each Studio area, the cookie/CSRF/organisation headers (`internal/api/authn.go`), the dev-session draft and relay, registries, pins and the handbook. *Added to [plans/p11.md](plans/p11.md):* §12 verification record; contracts §4.3 #13–#25 (app/plugin metadata and summaries, atomic change sets, lock last-edit and takeover rule, dev session following the stored draft, rollout advance, policy simulator, notification parameters, funnels and org overview, installation admin, licence upload, the undefined `.plux` archive, snapshot session tags); lead decisions L-12–L-15 (importer and migration preview in WebAssembly; the BFF hosts the OIDC callback and SAML assertion consumer — the server has no `/auth/*`; Studio never sends an edit twice; briefs and tiers per sub-agents.md); conflicts O-12–O-20 for the maintainer (`STU-010` radii, Appendix J colours, `STU-115` keys and pins vs the offline ceremony, routing `/setup` vs ADR-0026, versioning vs `workflow.md`, per-slice gates vs sub-agents.md, inbox text vs F16, browser-only permissions, the dev relay on several `api` replicas). *Verified:* every P11 ID mapped (script), every O-/L-/contract reference resolves, `make reuse-lint`, `make docs-site`, pre-commit on the plan. *Next:* the maintainer's answers to §2.3 (O-1–O-20), then S0.0 and S0.8.
