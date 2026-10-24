# Work Log

Current focus, hand-off notes and open decisions. This file lets any contributor — or an AI agent starting a fresh session — pick up the work exactly where it stopped.

Per-requirement implementation status is **not** tracked here; it lives only in the `Status` column of [requirements.md](requirements.md) (spec §4.3).

---

## Current focus

| Item | Value |
|---|---|
| Phase | P10 — Dev app and live debugging, **delivered on `feat/phase-10` (2026-10-23)**; P9 is merged to `main` ([#42](https://github.com/nightCode42/plux3/pull/42)) |
| Active branch | `feat/phase-10`, pushed |
| Active work | Merge `main` into `feat/phase-10`, a dispatched CI run (release inspection, device jobs), then the maintainer's review and the pull request |
| Requirement IDs | P10: [plans/p10.md](plans/p10.md) §3; `DEV-001`, `DEV-020`, `DEV-050`, `SEC-074`, `SYN-014`, `HST-034`, `DX-001` stay `WIP` (latest hand-off note) |

## Next up

**P11 (plan accepted, every decision taken):** [plans/p11.md](plans/p11.md) on `feat/phase-11` (pushed, on `feat/phase-10-v2`): start with S0.0 and S0.8 (§0) when the maintainer says to start. P10 below stays current until its pull request ([#44](https://github.com/nightCode42/plux3/pull/44)) merges.

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

- 2026-10-24 (night) — **P11 plan accepted: every decision taken; implementation not started.** *Decided:* the maintainer approved the lead's recommendation for O-1–O-16 and O-18 (M-9; O-17, O-19, O-20 were decided earlier, M-6–M-8). *Applied in [plans/p11.md](plans/p11.md):* status Accepted; §2.3 rows carry the decision and the step that carries it out; §4.1 lists the approved spec wording changes for 1.10.0 with their ADRs (`STU-001`, `STU-002`, `STU-006`, `STU-010`, `STU-024`/`055`/`060`, `STU-115`, §21 and §3 tables, `GOV-002`/`003`, `CI-006`, `QA-007`, `library.*` permissions, Appendix J, F, H, K); §4.5 the document corrections (A5.9 versioning, routing without `/setup` and with A45–A47 = 102 routes, F14, F16, F19, U4, U19); `ANL-050` in scope with ADR-0101, and `SEC-161`, `SCH-012` completed by it (§3.3); legal reviews as release conditions in ADR-0013/0014; the render worker image in S7.1; coverage floors and the design-gate detector in S0.1; the supported-server range in S0.4; read-only keys and pins in S8.7. The specification, ADRs and handbook are **not** edited yet: that is step S0.0. *Verified:* every P11 ID mapped, every O-/L-/M-/step reference resolves and every table row has its columns (scripts); `make reuse-lint`, `make docs-site`, pre-commit. *Next:* when the maintainer says to start, S0.0 and S0.8 (plan §0).
- 2026-10-24 (evening) — **P11 plan: maintainer decisions O-17, O-19, O-20 recorded; branch pushed.** *Decided:* CI runs once, fully, at the end of the phase (M-6); every Studio command maps to the server permission of its RPC, a missing one is added with its server check in the same change and listed for approval (M-7, procedure in [plans/p11.md](plans/p11.md) §6.2); dev sessions keep no state only in process memory (M-8). *Added:* step S0.9 (events and device frames on the shared stream layer, resumable on any `api` replica; Studio sessions follow the stored draft; CLI overlays as a PostgreSQL patch log; two-replica tests and benchmark; ADR-0087 Revision); S0.5 builds the shared stream layer (resumable reads on `internal/cache` streams, woken by the existing `cache.Notifier`) and the TypeScript permission catalogue; S0.6's command registry types each command's permission. *Correction to the earlier answer on O-20:* the draft copy goes away for Studio sessions only; CLI sessions keep an overlay of local files, moved to the shared patch log. *Verified:* every P11 ID mapped and every O-/L-/M-/step reference resolves (script), `make reuse-lint`, `make docs-site`, pre-commit. *Open:* §2.3 O-1–O-16, O-18. *Next:* the remaining answers, then S0.0 and S0.8.
- 2026-10-24 (later) — **P11 plan verified against the code and documents; `feat/phase-11` rebuilt on `feat/phase-10-v2`.** *State:* P10's PR #43 was closed unmerged and replaced by [#44](https://github.com/nightCode42/plux3/pull/44) (`feat/phase-10-v2`: the same tree plus `docs/engineering/sub-agents.md`); `feat/phase-11` was re-cut from it and the plan commit cherry-picked (the old local branch kept as `backup/feat-phase-11-on-p10v1`; nothing pushed — the push to the session branch was refused). *Checked:* every Connect service mounted in `internal/server/services.go` against each Studio area, the cookie/CSRF/organisation headers (`internal/api/authn.go`), the dev-session draft and relay, registries, pins and the handbook. *Added to [plans/p11.md](plans/p11.md):* §12 verification record; contracts §4.3 #13–#25 (app/plugin metadata and summaries, atomic change sets, lock last-edit and takeover rule, dev session following the stored draft, rollout advance, policy simulator, notification parameters, funnels and org overview, installation admin, licence upload, the undefined `.plux` archive, snapshot session tags); lead decisions L-12–L-15 (importer and migration preview in WebAssembly; the BFF hosts the OIDC callback and SAML assertion consumer — the server has no `/auth/*`; Studio never sends an edit twice; briefs and tiers per sub-agents.md); conflicts O-12–O-20 for the maintainer (`STU-010` radii, Appendix J colours, `STU-115` keys and pins vs the offline ceremony, routing `/setup` vs ADR-0026, versioning vs `workflow.md`, per-slice gates vs sub-agents.md, inbox text vs F16, browser-only permissions, the dev relay on several `api` replicas). *Verified:* every P11 ID mapped (script), every O-/L-/contract reference resolves, `make reuse-lint`, `make docs-site`, pre-commit on the plan. *Next:* the maintainer's answers to §2.3 (O-1–O-20), then S0.0 and S0.8.
- 2026-10-24 — **P11 planned: [plans/p11.md](plans/p11.md) proposed on `feat/phase-11`.** *How:* `feat/phase-11` cut locally from `feat/phase-10` (`a2b09de6`, not pushed); `main` is merged in once P10 ([#43](https://github.com/nightCode42/plux3/pull/43)) is merged. The plan binds the six P11 preparation documents into one programme: the 97 active P11 rows and the earlier `WIP` rows waiting on Studio, each mapped to a step (§3.3, §5; a script confirmed every P11 ID is mapped); F23's ten slices split into agent-sized steps with waves (§6); registry allocations (§2.4: ADRs 0013, 0014, 0094–0101, `PLX-9400`–`9499`, migrations from 0051). *Maintainer decisions (2026-10-24):* Arabic UI fonts Noto Sans Arabic and Noto Naskh Arabic; a QR library for pairing (lead chose `uqr`); legal reviews kept open; the reference laptop later. *Lead decisions (delegated):* takeover after 5 min without edits or a lapsed lease; Studio localisation ADR is 0094 (0066 is taken). *Found:* decisions file C6 is stale — every Appendix C widget has a descriptor and a builder (149 Layer 1, 58 Layer 2; `catalogue -phase P16 -report` lists only the P13 payment components); P9 already set session timeouts (30 min idle, 12 h absolute) where F19 proposed 2 h idle; P9 already delivers search, the notification inbox, restrictions and explain access (F14 fallback not needed); `QA-007` names Lighthouse CI, which decisions A5.8 replaced unrecorded; the routing file has no Plux Test or replay pages. *Verified:* `make reuse-lint`, `make docs-site`, pre-commit hooks on the plan. *Open (maintainer):* plan §2.3 O-1–O-11. *Next:* the maintainer's answers, then S0.0 and S0.8.
- 2026-10-23 — **P10 delivered on `feat/phase-10` (D0–D12), cut from `feat/phase-9` before P9 merged (D-11).** *How:* lead plus short-lived sub-agents in worktrees from pre-digested briefs (AGENTS §6); the lead reviewed every diff, fixed integration defects and ran the gates.
  - *Changed:* spec 1.9.0 and ADRs 0087–0093 (revisions of 0044, 0057, 0075, 0082, 0087); `devsession.proto`/`devevents.proto`, registries, migrations 0049–0050; `internal/compiler/incremental`; `internal/devsession` (sessions, pairing by link or short code, socket tickets, WebSocket relay with permessage-deflate, PushPatch, commands, CLI stream, trace export, function-log merge, server-compiled PXL console); runtime `devtools_api` (channel, dev overlay and hot apply, commands, event sources, inspect, overlays, screenshots, network simulation); `plux_devtools` 0.2.0 pairing; `plux_host_kit`; `apps/plux_dev`; `plux_security` 0.2.0 report-only development mode; `plux dev` CLI; background sync with a process-wide sync lock; `plux create --router`; chart ingress for `/dev/socket`; CI jobs `bench-devloop`, `bench-devapply`, `devloop-session`, `quickstart-dry-run`; quick-start guide.
  - *Maintainer decisions (2026-10-23):* WorkManager approved; no QR library (link and short code); limit defaults confirmed; JUnit 4 for the runtime's JVM tests (ADR-0044 Revision); `go_router`/`auto_route` in generated starter apps; `DEV-020` keeps its "to Studio" wording, so it stays `WIP` until P11.
  - *Lead decisions:* device frames are proto3 JSON; `RenewSocketTicket` for reconnects; the dev socket compresses (ADR-0087 Revision: 619 ms p95 uncompressed, 231 ms compressed); the edit-latency test runs under `make go-budgets`.
  - *Verified:* `make trace` (0 violations), `make spec-lint`, `make bench-devloop` (231 ms p95 against 350), `make bench-devapply` (≤ 49 ms p95 against 150), `make quickstart-dry-run` (15 s), `make devloop-session`, the touched Go and Flutter suites, the pre-push hook; `make check` result in the report.
  - *Not verified here (no SDKs):* Gradle and Xcode builds, the Kotlin and Swift tests, `make release-inspect` on real release builds, the emulator variant of the dev session; hence `DEV-001`, `DEV-050`, `SEC-074`, `SYN-014` stay `WIP` until the dispatched CI run.
  - *Open:* merge `main` (P9 squash-merged as #42) into `feat/phase-10` — needs the maintainer's go-ahead; the dev-session hub is per process (the CLI and the device must reach the same api replica); `HST-034`/`DX-001` wait for the recorded usability sessions; the PXL console types every root as a string map.
- 2026-10-23 — **P9 CI: PR #42 open as a draft; every job green but the audit walkthrough; hand-off for a new session.** *How:* root cause from each CI log before any fix, smallest fix validated locally, no sub-agents; manual walkthrough-only runs with step 3 reduced to its setup (`walkthrough-skip`). Full record: [plans/p9-handoff/ci-handoff.md](plans/p9-handoff/ci-handoff.md).
  - *Fixed (17 commits, ci-handoff §5):* two four-eyes bypasses through access tokens (approval requester, break-glass review: GOV-021, GOV-024); promotion changes against the channel's release; OpenBao Transit signer and a real bootstrap in the walkthrough; CLI import locks; the rollout health gate (release attribution, stage length, token renewal, closing-rollout race); test-schema drops; two iOS lost-tap races; step 4 freezes production (GOV-025).
  - *Maintainer decisions:* OpenBao dev signer; real bootstrap; three licences allowed; the freeze is shown on production; kill-switch propagation within 60 s (`NFR-024`) is a manual device check (walkthrough document, *Manual checks*); partial walkthrough runs; break-glass records the invoking person (`invoked_by_user`, migration 0048).
  - *Verified:* full PR run 39892738554 green except the walkthrough; partial run 39908253174 passed steps 1, 2 and 4; the touched packages' tests and lint; the pre-push `make go-test`.
  - *Not verified:* walkthrough steps 5–9 (step 5 fails: PLX-8033, the mandatory release is newer than the channel's, ci-handoff §4.1); a green full PR run on `25ca2ad4` (two PR runs were cancelled by hand); the iOS tap fixes compile only in CI.
  - *Open:* ci-handoff §4.4 (capability-set gate requester attribution, the gate's first rollup bucket, the manual device check, older open items).
- 2026-10-22 (session 3) — **P9 delivered on `feat/phase-9`; every slice merged; `make check` green but `go-vuln`.** *How:* README §6 of [plans/p9-handoff](plans/p9-handoff/README.md), agents by model per §5; rules 13 and the no-reformat line added to the agent prefix, the direct-and-short-lived rule to `AGENTS.md` §6.
  - *Merged this session:* E2a (+ ABT-007 gate), E2b, E5i, E5j, E5k, E5l, E5m (runtime 0.6.0), E8f, E8g (viewer pins root keys), E9e, E11a1, E11a2, E11b, E11c, E1e-a, E1e-b (restriction policies, `GOV-006`/`GOV-007`, spec 1.8.0, ADR-0086), `origin/main`, the CI jobs (restore drill, air-gapped, IaC, nightly fleet and control propagation), the E12 gate fixes (lint, proto names, tidy, coverage floors, a Dart race).
  - *Lead fixes at review:* propagation metric queried once per device (cached); viewer trusted server keys (now pinned roots); approval stores missing from the personal-data map; Admin could restrict the organisation and lift their own restriction (Owner only now, audited); generated files reformatted by agents (restored with `make gen`); migration collisions renumbered (0044–0048); flaky waits root-caused.
  - *Statuses:* 52 of 59 P9 rows `DONE`; `GOV-020`, `ANL-040` (Studio, P11), `DEP-004`, `DEP-010` (kind runs never ran here), `NFR-040` (no check) `WIP`.
  - *Verified:* `make -k check` (every gate but `go-vuln`), `make trace` 0 violations, `make spec-lint`, `make go-test` in the pre-push hook.
  - *Not verified:* `go-vuln`, a dispatched CI run, `make e2e-starter`, the 40 race runs, any kind/Docker run (restore drill, air-gapped, walkthrough), OpenTofu init.
  - *Open:* the walkthrough needs a WebAuthn step-up driver and an enterprise image with a licence before CI can run it (no job yet); renamed P9 proto messages listed in the PR body; hand-off 2 §9 items 6–7 (limit defaults incl. the restriction limits, ADR-0066 locale rule, `ee/LICENSE` review); obsolete branches `wip/p9-*`, `wip/p9-handoff`, `claude/tender-mendel-b4bh9u`.
- 2026-10-22 — **P11 Studio design chosen; P11 preparation documents brought to `main`.** *How:* five directions for the plugin canvas (Ledger, Atelier, Porcelain, Signal, Halo) were mocked up as private design pages; the maintainer chose Porcelain with Atelier's typefaces and screen titles and Signal's compact, information-rich presentation, then accepted the combined design. *Changed (documents only, no code):* [plans/p11-studio-ui-language.md](plans/p11-studio-ui-language.md) rewritten (Porcelain: three glass capsules, pill tabs, Instrument Sans/Serif + DM Mono, Phosphor duotone icons, measured contrast, information patterns U10, how the design is kept U18); [plans/p11-studio-reference.html](plans/p11-studio-reference.html) (new, the approved canvas as a self-contained page); [plans/p11-studio-design-skill.md](plans/p11-studio-design-skill.md) (new, the draft of `.claude/skills/plux-studio-design/`); the decisions file (A1.5, A3, A4.14, A4.20, A5.4, rule book §4.3, §4.6, §8.3, §13.7, §14, C1, C3; Part B links now resolve from `docs/plans/`); the functional decisions (F23 design gate, Z4); SPDX headers on the four P11 files. The P11 files reach `main` with this pull request (merged from `feat/studio-preparation`). *Verified:* `make reuse-lint`, `make docs-site`, the pre-commit hooks on the changed files; relative links and anchors of the changed Markdown checked with a script (lychee runs in CI). *Open (maintainer):* the Arabic pairing for Instrument (U7); allowlist entries and ADR-0014 for the fonts and Phosphor; the design gate's detector as a CI gate (F23, Z4); Phosphor is stable rather than active (core 2.1.1, March 2024) — inlined paths accepted in A4.20. *Next:* P8 review and P9 as planned; these files move to `docs/engineering/` when P11 starts.
- 2026-10-21 — **P9 hand-off 2: twenty agent slices merged on `feat/phase-9`; two stopped, nine briefed; package for a new session.** *How:* the agent playbook ([plans/p9-handoff/agents/AGENT-PLAYBOOK.md](plans/p9-handoff/agents/AGENT-PLAYBOOK.md)): middle-sized Sonnet/Haiku agents in worktrees from self-contained briefs, five or six at a time; the lead reviewed every diff, fixed what review found and merged. *State:* `feat/phase-9` holds E0, E1, E3, E4/E5 core, E6, E7, E8 core, E9 (except load and sizing) and E10 (README §3, with every lead fix); E2a (approvals) and E9e (load, sizing, `bench-control`) stopped on a usage limit, saved on `wip/p9-e2a-approvals` and `wip/p9-e9e-load`; nothing is running. Migrations 0024–0043 (next free 0044). *Package:* [plans/p9-handoff/README.md](plans/p9-handoff/README.md) (state, branches, done, in progress, next agents, how to run, traps, lead work, verification, decisions, follow-ups, progress), `tasks.md`, `agents/prefix.md` (shared agent rules), `agents/definitions/` (the `coder`, `coder-high`, `coder-light` agent types), `agents/briefs/{done,in-progress,next}/`; the first hand-off is kept as `README-2026-10-18.md` and `briefs-2026-10-18.md`, its consumed patches removed.
  - *Verified at each merge:* `go build ./...` in `backend` (with and without `GOWORK=off`) and `ee/backend`; `go vet` of touched packages; `go test -count=1` of the merged and neighbouring packages against PostgreSQL; `dart analyze lib test`; the pre-push `make go-test` on every push.
  - *Not verified:* `make check`, `-race` everywhere, the full Flutter suite (the experiments/control/bundle/pxl/render suites timed out under load after the E5-runtime merge), `make e2e-starter`, kind/drill/air-gap/walkthrough runs (no Docker here), OpenTofu validation (registry blocked), CI on `feat/phase-9`.
  - *Next:* README §5 rows 1–5 together (E2a and E9e continuations, E5i, E5j, E8f), then E2b, then E11b, E11c, E11a; the lead merges `origin/main` (#37, #39, #40), adds the CI jobs, runs E12. *Open:* README §9 (Open decisions above).
- 2026-10-18 (second session) — **Root cause of the iOS bank shard's repeated failures.** *Found:* run 39149009669 (`796ddbd`) failed only in Device end-to-end (iOS, bank): `AccessibilityTreeTests.enterPin` waited for the PIN pad to read "6 of 6 digits entered", which it never does: `SecurePinPadView._press` clears the pad before `onCompleted` (SEC-090, "the pad clears itself first"), so a full PIN reads "0 of 6". The check came in `c0479a3` as a guess at run 39143719161's "Wrong username, PIN or code", whose cause was the doubled first key (`796ddbd`); that run reached the server's 401, and the form requires the PIN, so the pad's taps were counted. The pass then deleted and re-entered the PIN twice and failed after 8 min with a message that read like a lost tap. Why it repeated: the passes are written where neither Xcode nor an Android SDK runs, each guess was checked only by a ~30 min CI run, and no widget test pinned the pad's count after a full PIN. *Changed:* the iOS pass checks the count after each digit ("N of 6", then "0 of 6" after the last; a missed or doubled tap fails at its position, no retry); `secure_widgets_test.dart` pins the count through a full entry and the cleared state; testing.md §2 *Device passes* (maintainer-approved): a pass expects only states the widget's own test pins, and names it; a stray comment left by `796ddbd` removed. *Verified:* the widget test passes; `dart format`, `flutter analyze`, `make spec-lint trace catalogue-check reuse-lint hygiene dart-lock-check dart-fmt-check dart-analyze widgets-api-check dart-cover`, `actionlint` and `zizmor --offline` pass (zizmor's online audits need the GitHub API, refused here). *Not verified:* the Swift (no Xcode here); a dispatched CI run on `feat/phase-8` checks it (the maintainer reports the result). *Next:* that run; then the maintainer's review and the P8 pull request. *Then:* run 39153556766 (`509bbbf`) passed the PIN pad and failed one step later, a new cause the PIN failure had hidden (`continueAfterFailure = false` stops at the first failure, so each run showed one step): the pass sent Return after the one-time code, but `OtpInputView` ends the autofill context when the code is complete, and on iOS the engine's `triggerAutofillSave` resigns the field's first responder (Flutter 3.47.5, `FlutterTextInputPlugin.mm`), so no field had keyboard focus. The pass sends no Return after the code; the widget test pins `TextInput.finishAutofillContext` on completion (fails without the call). The remaining steps were read against their widgets and the Android twin, which passes the same pages. A manual CI run can now ask for one job and flow (`only`, `shard`; ADR-0043 Revision 2026-10-18, maintainer's request), and CI OK fails in such a run. *Verified:* the widget test, `dart format`, `flutter analyze`, `tools` tests, `actionlint`, `zizmor --offline`, `make spec-lint trace reuse-lint hygiene dart-fmt-check dart-analyze`, the changes step's script run locally for nine input cases; `dart-cover` not re-run (a test-only change). *Next:* the narrowed run of `e2e-ios`/`bank`, then a full run.
