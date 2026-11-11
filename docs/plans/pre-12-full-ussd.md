<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Phase 17 plan — Full USSD (`pre-12-full-ussd`)

| Item | Value |
|---|---|
| Phase | P17 — Full USSD; scheduled after P11 and **before P12**, so it is part of milestone M3 — Plux 1.0 |
| Status | Proposed 2026-11-11; every decision taken in [pre-12-full-ussd-decisions.md](pre-12-full-ussd-decisions.md) (D-1 to D-32); §2.3 lists what the maintainer approves with this plan |
| Branch | `feat/pre-12-full-ussd`, cut from `claude/handoff-studio-fixes` (maintainer, 2026-11-11) |
| Specification | 1.14.0 → 1.15.0 in milestone U0 (§4.1) |
| Previous plans | [p11.md](p11.md); decision record [pre-12-full-ussd-decisions.md](pre-12-full-ussd-decisions.md) |

P17 lets one development serve two channels. A developer designs plugins, pages, actions, API
calls and messages once in Studio; the same signed release renders as native Flutter widgets on
phones and as USSD menus on any GSM handset. Every widget and action is classified per text
channel (native, silent, mapped, blocked); most pages need no USSD work, and the few elements a
keypad cannot express are reported before publishing with a one-click fix. A new `ussd` role of
`plux-server` executes the signed bundles in Go, behind three gateway adapters (Ethio Telecom,
Safaricom Ethiopia, Africa's Talking) and a simulator. Banks run it under the `maximum` profile
through audited exceptions with compensating controls: the institution verifies the PIN, tokens
are bound to the runtime, SIM swaps are checked where the operator allows, and transaction
signing becomes USSD strong customer authentication. The design is channel-neutral, so SMS bots,
WhatsApp or IVR can follow without a redesign.

---

## 0. For the session that starts P17

1. Read `AGENTS.md`, `docs/WORKLOG.md`, the decision record and this plan; run `git status` and
   `git branch --show-current` (expected: `feat/pre-12-full-ussd`).
2. Read the component `AGENTS.md` of every area a milestone touches (`backend/`, `packages/`,
   `studio/`, `tools/`), and the handbook pages of the routing table (AGENTS §4). Studio work
   follows `studio/AGENTS.md` and ADR-0103 (shadcn/ui as it ships); load the shadcn skill before
   changing the interface.
3. Start with U0 (§6). It lands the specification, the ADRs, the contracts and the registry
   allocations of §2.4. Only U1 (descriptor classification, mechanical) may run beside it.
4. Work as in P10 and P11: short-lived agents in worktrees from self-contained briefs (AGENTS §6,
   [sub-agents.md](../engineering/sub-agents.md)), the smallest model that does the job, the lead
   reviews every diff, runs `make gen` and owns the gates and CI. Agents never edit generated
   files by hand.
5. Stop and ask before anything in AGENTS §7 that this plan and the decision record do not
   settle. Security-relevant milestones (U7, U8, U9, U11) are reviewed by the lead line by line.

---

## 1. Goal and exit criteria

**Goal (spec §5.1 row written in U0):** a text-channel layer with USSD as its first channel:
per-element classification, server-side execution of signed bundles, USSD rendering, sessions,
routing of short codes, three gateway adapters and a simulator, institution-verified PIN,
session-bound tokens, SIM-swap checks, audited exceptions under `maximum`, USSD strong customer
authentication, USSD push, SMS and `sendSms`, a configurable user-data store, confidential
bundles for server runtimes, full Studio support, CLI and live debugging.

| # | Exit criterion | How P17 proves it |
|---|---|---|
| X1 | One design serves both channels | The Plux Bank reference app gains a transfer flow (two inputs, success message, `apiCall` through the gateway, result list, PIN-confirmed transfer). The same signed release runs on the Flutter runtime and through the `ussd` role. `test/ussd/` scripts the USSD session on the simulator in CI (every step's screen compared to goldens) and the Flutter side in the existing widget and emulator jobs. |
| X2 | Every element is classified and every gap reported | `make gen` fails when a widget or action descriptor lacks a text-channel class; the coverage table and Appendix C/D list the classes; a golden corpus of documents proves the compiler's Blocked report (reachability, hide, alternative). |
| X3 | Server and device execute identically | The action conformance vectors (`schema/testdata/actions/`, new) pass on the Dart engine and the Go executor, as the PXL vectors do (`PXL-007`); the text-render vectors (`schema/testdata/ussd/`) pass on the Go renderer. |
| X4 | Banking-grade security on USSD | Threat-model delta and ASVS evidence (§7); automated tests for every control of §7.2: PIN never stored or logged (log, trace, Valkey and database scrapes), PIN envelope, lockout, SIM-swap policy, exceptions and their approval, USSD SCA with dynamic linking, push anti-phishing window, confidential bundles to a runtime, revocation and key rotation, gateway authentication and MSISDN trust. |
| X5 | Three adapters complete without a contract | The adapter contract suite passes for the HTTP gateway adapter, the SMPP client (against the SMPP test server, U11), Africa's Talking and the Ethio Telecom and Safaricom Ethiopia profiles (each over SMPP and HTTP). A nightly job runs the Africa's Talking sandbox when its credential is configured (manual check otherwise). The operator onboarding document is published (D-27). |
| X6 | Throughput and latency | `make bench-ussd`: the CI-scaled benchmark guards regressions; the reference measurement (≥ 1,000 key presses per second per instance, Plux time ≤ 150 ms p95) is run on the §30.1 reference deployment and recorded in `docs/benchmarks/ussd.md`; the `NFR` rows stay `WIP` until it is run (§3.2). |
| X7 | Studio delivers "one design, both channels" visibly | The USSD simulator beside the canvas, the Blocked report with one-click fixes, the per-element channel panel, the home-menu editor, and the screens for short codes, adapters, exceptions, push, runtimes and data residency; Studio unit, e2e and design-gate checks pass. |
| X8 | Operable | Helm chart and Compose run the `ussd` role (kind e2e with the simulator); runbooks for every new failure mode (§10); user documentation; the reference app updated (DoD-7 to DoD-9). |

---

## 2. Decisions

### 2.1 Maintainer decisions

All maintainer decisions are in [pre-12-full-ussd-decisions.md](pre-12-full-ussd-decisions.md)
and are not repeated here. Summary for orientation:

| Topic | Decision |
|---|---|
| Phase | P17 "Full USSD", before P12 (D-1) |
| Model | One source, per-element text-channel classes; unsupported display elements skipped; developer fixes for blocked items (D-2 to D-5) |
| Server | A `ussd` role of `plux-server`; Go executor of the signed bundles; Valkey sessions (D-7, D-8, D-9, Q-1) |
| Gateways | Three adapters fully implemented without a contract (SMPP and HTTP profiles), own SMPP client, simulator, one active provider per short code (D-6, D-31) |
| Routing | Dynamic short codes, organisation menus for shared codes, sub-code deep links, never a bare plugin (D-10, D-24) |
| Identity and security | Institution-verified PIN, session-bound tokens, SIM-swap checks, risk limits, audited exceptions under `maximum`, USSD SCA, optional in-app approval (D-12 to D-14, D-32) |
| Data | User-data store per app, Plux-hosted or institution-hosted, no silent default (D-9) |
| Channels | Text-channel architecture; USSD-only apps; app-to-USSD handoff without data (D-23, D-29, D-30) |
| Messaging | USSD push with every sub-feature off by default; SMS through the adapters; `sendSms` on both channels, prominent on the canvas (D-18, D-25, D-26) |
| Product | Full Studio support; open source; ≥ 1,000 key presses per second per instance; exit without an operator contract plus an onboarding document (D-16, D-17, D-20, D-27) |

### 2.2 Technical decisions of the lead (this plan; the maintainer may veto at review)

| # | Topic | Decision | Why |
|---|---|---|---|
| L-1 | Requirement areas | Two areas: **`CHN`** (text channels, channel-neutral: classification, executor, sessions, user data, `sendSms`) and **`USSD`** (USSD-specific: role, gateways, adapters, short codes, rendering limits, identity on USSD, push). Studio, CLI, deployment and target rows use their existing areas (`STU`, `CLI`, `DEP`, `NFR`, `LIM`). Refines D-1's single `USSD-` prefix. | D-23 makes the core channel-neutral; naming it `USSD-` would mislabel it when the second channel arrives. |
| L-2 | Spec placement | A new **§36 "Text Channels and USSD"** after §35, and a §5.1 row; no section is renumbered. | Renumbering §24–§35 would break every `§` reference in the spec, ADRs and code comments. |
| L-3 | Where classes live | Each widget and action descriptor gains `channels: { "ussd": { "class": …, "mapping": … } }`. A generator writes the class tables into the compiler, the Go executor and Studio; nothing is hand-copied. The existing `platforms` field (`android`, `ios`) is unchanged. | One registry, `CI-003` (generated code only through `make gen`). |
| L-4 | Document fields | Optional, additive fields: `channels.ussd` on app, plugin, page and node (enable, exclude, hide, alternative, label, text form, home-menu entries, USSD-only pages). `schemaVersion` bumps with an automatic, tested no-op forward migration (`SCH-000`). | Additive by habit before 0.0.1 (AGENTS §5). |
| L-5 | Feature flags in the manifest | USSD-only features go into a new manifest list `channel_features`, read only by the `ussd` role. Device runtimes never read it, so a USSD feature never makes a phone refuse a release (`BND-008`). | Keeps device compatibility independent of USSD evolution. |
| L-6 | Executor shape | The Go executor (`internal/textrt`) reads the release's FlatBuffers sections with the generated accessors in `internal/bundle/fbs`, runs PXL with `internal/pxl`, and implements the action engine, state scopes, forms, validators and navigation stack in Go. It never renders Flutter widgets: it produces a **channel-neutral screen model** (title, text blocks, one input, a choice list, an end flag) that a channel renderer turns into text. | The server has no renderer today (`internal/reconstruct` doc); the screen model is what makes D-23 true. |
| L-7 | Parity | Action semantics are pinned by **action conformance vectors** run by both engines, exactly as PXL is pinned (`PXL-007`). Every action the Go executor implements has vectors; a Dart change that breaks a vector fails the Dart job. | Two implementations must not drift; vectors are cheaper and stricter than prose. |
| L-8 | Layering rule L-8 | The `ussd` role: holds **no signing capability** (keys only through `signing/`, L-3); calls customer APIs **only through the `gateway` role** over internal mutual TLS (it never holds upstream credentials); uses a **restricted database role** granted only its own tables (short codes, routes, menus, adapters, exceptions, push consents, user data, metering) plus audit inserts and configuration reads; fetches releases **like a device** from the `api` role (never from the release tables). | Smallest blast radius at the internet- and operator-facing edge. |
| L-9 | Gateway caller | The gateway gains a second caller kind beside `device.Identity`: a **server-runtime caller** (runtime ID, organisation, app, environment, channel, session ID, end-user token, verified factors). Every gateway check (upstream registry, operation allowlist, capability, rate, assurance, SCA) runs unchanged; the `Plux-Assertion` names the channel and the factors. | Reuses the audited path (`SEC-030`–`SEC-032`, ADR-0056) instead of a parallel one. |
| L-10 | Session-bound tokens | D-12 asked for a per-session key in KMS or an HSM. Generating one HSM key per session cannot reach 1,000 sessions per second. Instead each **runtime holds one sender-constraint key** in the signing backend; the institution's token is bound to that key's thumbprint (`cnf.jkt`, RFC 7800/RFC 9449) **and** to the session ID (`sid` claim). Every call carries a proof signed through `signing/` that names the session; a token presented for another session, another runtime or after expiry is refused. **Needs the maintainer's approval (§2.3).** | Same guarantee (a stolen token is useless without the runtime's key and the live session), at HSM-friendly key counts. |
| L-11 | PIN envelope | The PIN never travels in clear beyond the `ussd` process: it is sealed to the institution's registered public key with ECDH-ES (P-256) + HKDF-SHA-256 + AES-256-GCM (the primitive family of ADR-0055, standard library only, no JOSE dependency), bound to the session ID and a nonce, inside an mTLS call. Plux holds no key that can open it. | A TLS-terminating proxy or a compromised log pipeline on either side still sees no PIN. |
| L-12 | Runtime key-agreement keys | Server runtimes unwrap confidential content keys through `signing/`. The PKCS#11 helper gains an ECDH-derive operation (`CKM_ECDH1_DERIVE`); the Vault backend keeps the runtime's key-agreement key enveloped under a Transit key and decrypts it only inside `signing/`; the file backend is for development. A new security setting `runtimeKeyBackend` refuses the file backend under `strict` and `maximum` and requires PKCS#11 under `maximum`. **Needs the maintainer's approval (§2.3).** | Vault Transit has no ECDH; the setting makes the stronger backend the rule where it matters. |
| L-13 | Assurance on USSD | A USSD session has no device, so its device assurance is `AL0`. A route guard or gateway operation that needs `AL1`+ refuses it, unless an **exception** (D-14) lists the `ussd` channel and the factors it accepts (`sim` = operator-passed number with the SIM-swap policy satisfied, `pin` = institution-verified PIN, `inapp` = approval in the registered app at `AL2`). The factors travel in the assertion; nothing silently upgrades a USSD session to an AL. | Explicit, auditable, and the upstream sees exactly what was verified. |
| L-14 | SIM swap when unsupported | A security setting `ussdSimSwapUnknown` decides what happens when the adapter cannot answer: `allow` (standard), `limits` (strict: sensitive actions only within reduced risk limits), `deny` (maximum: sensitive actions refused unless the exception explicitly accepts `unknown`). | Ethiopian operators may not offer SIM-swap data yet (decision record D-12). |
| L-15 | MSISDN trust | The caller's number is trusted only from an authenticated gateway connection (mTLS or SMPP bind credentials, plus the adapter's IP allowlist, plus signed callbacks where the provider signs). It is validated with `internal/pxl/phone` and normalised to E.164. Telemetry and metering carry a keyed hash (per app), never the number. | The number is the possession factor; it must not be spoofable from the internet. |
| L-16 | Idempotency | Every gateway request carries a session ID and a step sequence; a duplicate or retried delivery returns the stored screen and never re-runs actions. Mutating calls get an idempotency key derived from (session, step, node). | Operators and aggregators retry; money must not move twice. |
| L-17 | Valkey outage | Sessions fail closed: a new dial gets a short "service unavailable" end screen; no in-memory fallback for session state (unlike the DPoP replay cache, `SEC-023`). | Session state carries partially entered transactions; a split-brain fallback could replay them. |
| L-18 | Encryption of session state and user data | AES-256-GCM with per-app data keys enveloped under KMS (`SEC-106`); keys cached in memory with a bounded lifetime; sensitive inputs (`sensitive` fields, PIN) never written to Valkey at all, only held for the one request that forwards them. | Defence in depth; sensitive values have no reason to persist. |
| L-19 | `sendSms` on the Flutter channel | The device calls a new RPC (`MessagingService.SendSms`, DPoP-bound) that executes on the server through the app's SMS adapter. The recipient defaults to the **caller's own verified number**; other recipients need the capability flag `sms.anyRecipient` approved for the app and a profile setting; templates are declared in the plugin and compiled (PXL parameters, no free text from the device). | Prevents an SMS relay for spam or phishing (deny by default, `SEC-080`). |
| L-20 | Spec placement of payment behaviour | None in P17 (Q-5 withdrawn): P13 classifies its actions like any other. `signTransaction` (P6) is mapped in P17 (D-32). | Maintainer, 2026-11-11. |
| L-21 | Simulator licence | The simulator, the adapter interface, the render vectors and the CLI commands are Apache-2.0 paths (shared with the CLI, like the compiler and PXL); the `ussd` role and adapters wired into the server are AGPL-3.0-only server paths; `REUSE.toml` gains the entries. | ADR-0022 per-path licensing. |
| L-22 | Studio design | Studio work follows `studio/AGENTS.md` and ADR-0103 (shadcn/ui as it ships); the decision record's D-17 is corrected accordingly (2026-11-11). | ADR-0103 superseded the Porcelain design on 2026-10-30. |

### 2.3 Approvals (AGENTS §7)

| Item | Status |
|---|---|
| Phase P17, spec 1.15.0, `spec-lint` accepting P17 (CI gate) | Approved (D-1) |
| New server role `ussd` (`SRV-001`, ADR-0006 revision) | Approved (Q-1) |
| PIN delegate, session-bound tokens, SIM-swap policy, exceptions under `maximum`, USSD SCA, push controls (security defaults and controls) | Approved in principle (D-12 to D-14, D-18, D-32); **L-10, L-12, L-13, L-14 and the security-setting defaults of §2.4 need the maintainer's approval with this plan** |
| Server-runtime identity and confidential bundles for runtimes (key handling) | Approved (D-19); the PKCS#11 helper protocol change of L-12 needs approval with this plan |
| §3 non-goal exception for the Plux-hosted user-data mode | Approved (D-9) |
| `HST-010` extended to the server-side auth delegate; `GEN-001` conditional for USSD-only apps; `SEC-027`/`SEC-032` gain the USSD mapping | Approved (D-12, D-29, D-32); each `MUST` change gets its ADR (§4.2) |
| Dependencies | **None.** SMPP is an own implementation on the standard library (D-31); Africa's Talking is plain HTTP; the PIN envelope uses the standard library (L-11) |
| New CI jobs (`bench-ussd`, the `test/ussd` session suite, the nightly Africa's Talking sandbox job when a credential exists) | Needs approval with this plan |
| Coverage floors (§8) | Needs approval with this plan |
| Limit defaults and security-setting defaults (§2.4) | Needs approval with this plan |

### 2.4 Registry allocations for parallel work (lead, 2026-11-11)

| Registry | Allocation |
|---|---|
| Spec | 1.14.0 → 1.15.0 (U0) |
| ADRs | 0104 text channels and the USSD channel (model, classes, role; revises ADR-0006 and ADR-0010); 0105 server-side execution of signed bundles and action conformance; 0106 server runtimes: identity, enrolment, keys and confidential bundles (revises ADR-0055, ADR-0060); 0107 identity on USSD: auth delegate, PIN envelope, session-bound tokens, SIM swap, USSD SCA (revises ADR-0056; extends `HST-010`); 0108 USSD under the `maximum` profile: audited exceptions and factors; 0109 the user-data store: Plux-hosted and institution-hosted modes (§3 exception); 0110 gateway adapters, the SMPP client, profiles and provider switching; 0111 USSD push, SMS and `sendSms`; 0112 short codes, routing, menus and the app-to-USSD handoff |
| Requirement IDs | `CHN-001`–`CHN-040`, `USSD-001`–`USSD-060` (§3.1); `STU-135`–`STU-160`; `CLI-009`–`CLI-014`; `DEP-061`–`DEP-066`; `NFR-050`–`NFR-054`; `LIM` rows as needed |
| Error codes (four digits, Q-3; all blocks unused on 2026-11-11) | `PLX-2400`–`2499` channel validation (compiler); `PLX-3200`–`3299` short codes, routing, adapters, gateway sessions; `PLX-4500`–`4699` text-channel execution, rendering and sessions; `PLX-5600`–`5699` user-data store and storage delegate; `PLX-6300`–`6399` identity, PIN, SIM swap, exceptions, SCA, push, runtimes; `PLX-9400`–`9499` Studio and CLI. First codes in §5.13. |
| Migrations | From `0068` (next free on this branch): 0068 short codes, ownership proofs, routes and menus; 0069 adapters, provider configurations and standby; 0070 server runtimes, runtime keys and enrolment; 0071 auth delegates, storage delegates and institution keys; 0072 channel exceptions and approvals; 0073 user-data store (Plux-hosted); 0074 push consents and quiet hours; 0075 SMS templates and message metadata; 0076 usage metering counters |
| Permissions | `ussd.view`; `ussd.manage` (channel enablement, menus, settings); `ussd.shortcodes.manage`; `ussd.adapters.manage`; `ussd.exceptions.request` (approval through the approval engine, step-up); `ussd.push.manage` (approval, step-up); `ussd.runtimes.manage` (installation admin, step-up); `ussd.simulate`; `ussd.debug` (test-number streams); `sms.templates.manage` |
| Capabilities (plugin, `SEC-080`) | `sms` (send to the caller's own number); `sms.anyRecipient` (approval); `ussd.push` |
| Limit keys (defaults confirmed at U0 review) | `ussd.screenChars.gsm7` (182), `ussd.screenChars.ucs2` (80), `ussd.menuItemsPerScreen` (8), `ussd.inputChars` (64), `ussd.apiBudgetMs` (5000), `ussd.responseBudgetMs` (2000, Plux's own share per key press), `ussd.sessionSeconds` (180), `ussd.screensPerSession` (60), `ussd.stepsPerKeypress` (500), `ussd.sessionStateBytes` (16 KiB), `ussd.releasesCached` (3), `ussd.sessionsPerNumberPerHour` (30), `ussd.homeMenuItems` (9), `ussd.subCodeDepth` (4), `ussd.shortCodesPerOrganization` (20), `ussd.pushPerNumberPerDay` (5), `sms.perNumberPerDay` (10), `sms.perAppPerMinute` (600), `sms.templateChars` (459), `sms.segmentsPerMessage` (3), `userData.bytesPerUser` (256 KiB), `userData.recordsPerCollection` (1000), `userData.retentionDays` (365), `userData.delegateTimeoutMs` (2000) |
| Security settings (`schema/security/settings.json`, values standard / strict / maximum) | `ussdChannel` (allowed / allowed / exception-only); `ussdPinMaxAttempts` (5 / 3 / 3); `ussdPinLockoutMinutes` (30 / 60 / 1440); `ussdSessionTokenSeconds` (300 / 180 / 120); `ussdSimSwapBlockHours` (24 / 48 / 72); `ussdSimSwapUnknown` (allow / limits / deny); `ussdPushPinWindowSeconds` (120 / 60 / 0 = no PIN in push); `ussdInAppApproval` (off / optional / optional); `runtimeKeyBackend` (any / no-file / pkcs11); `smsAnyRecipient` (approval / approval / off); `confidentialMinAssurance` unchanged, runtimes eligible only when enrolled and not revoked |
| Proto | `proto/plux/v1/ussd.proto` (`UssdService`: short codes, ownership proof, routes, menus, adapters and provider switch, exceptions, push settings, runtimes, residency, simulator `SimulateStart`/`SimulateStep`, debug stream for test numbers); `proto/plux/v1/messaging.proto` (`MessagingService.SendSms`, templates); internal `runtimepb` (runtime sync and key delivery over mTLS); `pkcs11pb` gains `Derive` |
| Packages | `backend/internal/textrt` (executor), `textrt/screen` (screen model), `textchan` (channel-neutral session engine), `ussd` (role wiring), `ussd/render`, `ussd/routing`, `ussd/adapter` (+ `sim`, `httpgw`, `africastalking`, `smpp`, `profiles`), `ussd/identity`, `ussd/push`, `userdata`, `messaging`, `runtimes`; `schema/channels/`, `schema/testdata/actions/`, `schema/testdata/ussd/`, `schema/ussd/profiles/`; `test/ussd/`; Studio `features/ussd/`, `features/simulator/`; `packages/plux_flutter` `src/channels/` (handoff), `src/actions/` (`sendSms`) |

---

## 3. Requirements

The rows below are the content of the spec change (U0). U0 writes each as a normative row with
its exact wording; the summaries here fix scope and verification. "Verified by" names the test
or check that will carry the ID (`make trace`).

### 3.1 New requirements (target `DONE` at the exit unless §3.2 says otherwise)

**Channel model and classification (`CHN`)**

| ID | Pri. | Requirement (summary) | Verified by | Dec. |
|---|---|---|---|---|
| `CHN-001` | MUST | One set of documents and one signed release serve the Flutter channel and every enabled text channel; nothing is authored twice. | X1 session suite | D-2 |
| `CHN-002` | MUST | Every widget and action descriptor declares a class per text channel: native, silent, mapped or blocked; `make gen` fails when one is missing. | generator test, `gen-check` | D-3 |
| `CHN-003` | MUST | Each mapped element has a specified, tested channel equivalent (Appendix C/D columns). | render vectors | D-3 |
| `CHN-004` | MUST | Silent elements are dropped and their children kept; silent actions complete as no-ops and appear in traces. | executor tests | D-3, D-4 |
| `CHN-005` | MUST | The compiler reports every blocked element reachable on an enabled channel, per flow; publishing a channel-enabled plugin fails until each is hidden or has an alternative. | compiler corpus | D-3 |
| `CHN-006` | MUST | Elements may carry channel settings (label, text form, hide, alternative); they are document fields, compiled and signed. | schema and compiler tests | D-3 |
| `CHN-007` | MUST | Text channels are opt-in per plugin, with per-page exclusion and channel-only pages. | compiler tests | D-11, D-24 |
| `CHN-008` | MUST | Routes into hidden, excluded or guard-refused content show a configurable "available in the app" screen, optionally with an SMS link. | session suite | D-3, D-14 |
| `CHN-009` | MUST | The server executes the signed release with a Go executor whose behaviour equals the device engine's, pinned by shared action conformance vectors. | vectors on both engines | D-8 |
| `CHN-010` | MUST | The executor verifies every bundle before parsing anything beyond the container header (`SEC-052`) and refuses releases whose `channel_features` it does not support. | bundle tests | D-8, D-19 |
| `CHN-011` | MUST | The action bounds of `ACT-005` and the retry policies of `ACT-006` apply on the server; runs are cancelled when their session ends. | executor tests | D-8 |
| `CHN-012` | MUST | Session state lives in Valkey, encrypted per app, expiring with the session, surviving a restart or a move to another instance; sessions fail closed when Valkey is unavailable. | session tests, chaos test | D-9, L-17 |
| `CHN-013` | MUST | A session stays on the release it started with. | session tests | D-22 |
| `CHN-014` | MUST | Duplicate or retried gateway deliveries never re-run actions; mutating calls carry idempotency keys. | session tests | L-16 |
| `CHN-015` | MUST | Device-storage actions (local DB, KV, saved state) work on text channels through the user-data store; a plugin that uses them cannot be enabled on a text channel until the app has a store mode. | compiler and session tests | D-9 |
| `CHN-016` | MUST | Plux-hosted mode: data scoped to the device-storage equivalent, isolated and encrypted per app, retention limits, data-subject requests, versioned writes. | `userdata` tests | D-9 |
| `CHN-017` | MUST | Institution-hosted mode: storage actions call the configured storage delegate (endpoint or Function) over mTLS through egress with the documented contract (get, put, delete, query per user and collection, versions); Plux keeps none of the data. | contract suite | D-9 |
| `CHN-018` | MUST | User data is keyed by the end-user identity from the auth delegate, so it follows the user across channels. | session suite | D-21 |
| `CHN-019` | MUST | Data sources are reached only through the gateway; direct routes are refused on text channels. | gateway tests | D-28.2 |
| `CHN-020` | MUST | Per app (organisation for a shared code): a call that exceeds the time budget either continues in the background with the result by SMS, or ends with an error; the budget comes from the registry. | session tests | D-15 |
| `CHN-021` | MUST | The auth delegate supplies the user context (`user.*`); consent is asked at the first contact when the app requires it, stored per user and respected by telemetry (`SEC-161`). | session tests | D-28.1 |
| `CHN-022` | MUST | Text channels emit the runtime's telemetry events tagged with the channel; sensitive fields are stripped; numbers appear only as a keyed hash. | telemetry tests | D-28.3, L-15 |
| `CHN-023` | MUST | Rollouts and experiments assign text-channel users by end-user ID, or by the keyed number hash before sign-in. | rollout tests | D-28.4 |
| `CHN-024` | MUST | Kill switches apply to text channels; each app has a channel pause switch. | control tests | D-11 |
| `CHN-025` | MUST | Functions placed on the device run in `fnrunner` for text-channel sessions. | function tests | D-22 |
| `CHN-026` | MUST | Messages, numbers, dates and calendars are formatted on the server with the same rules as the runtime (`internal/l10n`), pinned by vectors. | l10n vectors | D-11 |
| `CHN-027` | MUST | `sendSms`: an action on every channel, through the app's SMS adapter, with declared templates, consent, the `sms` capability, limits and audit; recipients other than the caller need `sms.anyRecipient`. | action tests | D-25, L-19 |
| `CHN-028` | MUST | The session engine, executor and classification are channel-neutral: a second, test-only text channel runs the conformance and session suites without changes to them. | test channel | D-23 |
| `CHN-029` | MUST | Release approvals show each enabled channel's preview and its diff. | approval tests | D-22 |
| `CHN-030` | MUST | Sessions, screens, push prompts and SMS are metered per app and organisation. | metering tests | D-28.6 |
| `CHN-031` | MUST | Concurrent writes to user data use optimistic concurrency in both modes; one text session per user and app (a new contact ends the old one). | `userdata` tests | D-28.7 |
| `CHN-032` | MUST | Text-channel features in the manifest are a separate list that device runtimes never read. | bundle tests | L-5 |
| `CHN-033` | SHOULD | The executor keeps the last `ussd.releasesCached` releases decoded in memory; confidential ones in memory only. | executor tests | D-19 |

**USSD (`USSD`)**

| ID | Pri. | Requirement (summary) | Verified by | Dec. |
|---|---|---|---|---|
| `USSD-001` | MUST | USSD runs in the `ussd` role of `plux-server`, deployable and scalable on its own, under layering rule L-8. | config tests, layering test | Q-1, L-8 |
| `USSD-002` | MUST | One gateway adapter interface: begin, continue, end, abort, push, SMS, delivery reports and SIM-swap queries, each declared as a capability. | contract suite | D-6 |
| `USSD-003` | MUST | A simulator implements the interface with configurable operator profiles (limits, timeouts, encodings, push, SMS), used by tests, the CLI and Studio. | contract suite | D-6 |
| `USSD-004` | MUST | A configurable HTTP gateway adapter: JSON, XML or form requests and replies, field mapping, continue/end markers, mTLS, HMAC request signatures, IP allowlist. | contract suite | D-6 |
| `USSD-005` | MUST | An SMPP 3.4/5.0 USSD client on the Go standard library: `bind_transceiver`, `enquire_link`, `deliver_sm`/`submit_sm`/`data_sm`, `ussd_service_op`, `its_session_info`, windowing, reconnect, TLS where offered; the PDU decoder is fuzzed. | SMPP tests, fuzz | D-31 |
| `USSD-006` | MUST | An Africa's Talking adapter for USSD and SMS over its public API. | contract suite | D-6 |
| `USSD-007` | MUST | An Ethio Telecom profile over SMPP and HTTP, complete and tested on the simulator. | contract suite | D-6 |
| `USSD-008` | MUST | A Safaricom Ethiopia profile over SMPP and HTTP, complete and tested on the simulator. | contract suite | D-6 |
| `USSD-009` | MUST | The operator onboarding document: what to request, how to fill in and test each profile, the acceptance tests, and the steps to production. | docs check | D-27 |
| `USSD-010` | MUST | One active provider per short code; a standby provider may be configured; switching is a configuration change approved and audited; adapter instances stand in for each other. | routing tests | D-6 |
| `USSD-011` | MUST | Gateway traffic is accepted only from authenticated connections (mTLS or SMPP credentials, IP allowlist, provider signatures); the caller's number is trusted only from them and normalised to E.164. | gateway auth tests | L-15 |
| `USSD-012` | MUST | Organisations register their short codes; registration needs an ownership proof and installation-admin approval. | routing tests | D-28.5 |
| `USSD-013` | MUST | A short code opens one app or an organisation menu of apps; sub-codes deep-link to plugins or pages through the app; no root code opens a bare plugin; routes are runtime configuration, validated and audited. | routing tests | D-10 |
| `USSD-014` | MUST | An app's USSD home menu is generated from its enabled plugins and editable; a shared code's organisation menu likewise. | routing tests | D-24 |
| `USSD-015` | MUST | Rendering: the page title on every screen, one input per screen, numbered choices, `98 More` and `0 Back` (labels configurable), long text split across screens. | render vectors | D-5 |
| `USSD-016` | MUST | Screen budgets come from the registry per gateway and encoding; GSM-7 or UCS-2 is chosen per screen; no screen exceeds its budget; no transliteration. | render vectors, property test | D-5, D-28.11 |
| `USSD-017` | MUST | Typed inputs (phone, amount, IBAN, date, bounded numbers, choices) validate on the server and re-prompt with an error line. | render vectors | D-3 |
| `USSD-018` | MUST | The language is chosen at the first dial when the app has several, remembered per user and changeable from the menu. | session tests | D-11 |
| `USSD-019` | MUST | Each response stays within `ussd.responseBudgetMs` of Plux time; an interrupted critical flow sends an SMS. | session tests, bench | D-15 |
| `USSD-020` | MUST | The caller is identified by the operator-passed number; each app configures an auth delegate (endpoint or Plux Function) that verifies the PIN and returns the end-user token and user context; Plux never stores a PIN. | identity tests | D-12 |
| `USSD-021` | MUST | The PIN is sealed to the institution's key (L-11) and sent over mTLS; it is a sensitive value: never logged, traced, persisted or written to Valkey. | identity tests, scrape tests | D-12, L-11 |
| `USSD-022` | MUST | End-user tokens on USSD are short-lived and bound to the runtime key and the session; proofs are signed through `signing/`. | identity tests | D-12, L-10 |
| `USSD-023` | MUST | PIN attempts are limited and locked out per number and app (`ussdPinMaxAttempts`, `ussdPinLockoutMinutes`), with rate limits per number and code. | identity tests | D-12 |
| `USSD-024` | MUST | Before a sensitive action the adapter's SIM-swap capability is queried; a swap within `ussdSimSwapBlockHours` refuses it; an unknown answer follows `ussdSimSwapUnknown`. | identity tests | D-12, L-14 |
| `USSD-025` | MUST | Sensitive actions work on USSD only when the app opts in, within its risk limits (amount, daily count), with a full audit trail. | identity tests | D-13 |
| `USSD-026` | MUST | Under `maximum`, and for guards above the session's level, USSD needs an exception per app (optionally per guard): off by default, approved with WebAuthn step-up, a distinct warning, effective only with its declared factors (L-13). | exception tests | D-14 |
| `USSD-027` | MUST | `signTransaction` on USSD is USSD strong customer authentication: amount and payee shown and bound into the PIN verification (dynamic linking), the institution's approval, the assertion marked USSD. | SCA tests | D-32 |
| `USSD-028` | SHOULD | A high-risk USSD action can require approval in the user's registered app at `AL2`. | identity tests | D-12 |
| `USSD-029` | MUST | USSD push: confirmations, server-initiated prompts and PIN-in-push each off by default and enabled separately with approval; consent per number, rate limits, quiet hours, the app's registered name on every push screen, PIN only within `ussdPushPinWindowSeconds` of a user-initiated action; audited. | push tests | D-18 |
| `USSD-030` | MUST | Receipts, slow-call results and links go by SMS through the adapter; delivery reports are recorded where the provider sends them. | messaging tests | D-26 |
| `USSD-031` | MUST | Server runtimes are an identity type separate from devices: enrolled by an installation admin with step-up, identified by mTLS and workload identity, revocable. | runtime tests | D-19 |
| `USSD-032` | MUST | Runtime keys live in the signing backend; `runtimeKeyBackend` refuses the file backend under `strict` and `maximum` and requires PKCS#11 under `maximum`. | signing tests | L-12 |
| `USSD-033` | MUST | Confidential releases reach enrolled runtimes with content keys wrapped to the runtime key; decrypted only in memory; every unwrap audited; revocation and rotation honoured. | runtime tests | D-19 |
| `USSD-034` | MUST | Runtimes sync like devices: control document, kill switches, rollouts and experiments. | sync tests | D-19 |
| `USSD-035` | MUST | Apps may have only the USSD channel; no Flutter project is generated for them. | generator tests | D-29 |
| `USSD-036` | MUST | Per app, the Flutter runtime offers to continue a flow over USSD when there is no data connection (dial on Android; show the code on iOS). | runtime tests | D-30 |
| `USSD-037` | MUST | An installation declares its data-residency region; Studio and the CLI warn when an adapter, storage or auth delegate endpoint is declared outside it. | config tests | Q-6 |
| `USSD-038` | MUST | Sessions from allowlisted test numbers stream to the dev tools (`DEV-020`) with the same redaction. | dev tests | D-22 |
| `USSD-039` | MUST | Studio's USSD preview states that USSD cannot mask input on every PIN step. | Studio tests | Q-7 |
| `USSD-040` | MUST | Abuse controls: sessions per number per hour, per-code rate limits, connection limits per adapter, and back-pressure that ends new sessions politely under overload. | load tests | D-28 |

`USSD-041` to `USSD-060` stay free for rows U0 splits out of the summaries above.

**Other areas**

| ID | Pri. | Requirement (summary) | Dec. |
|---|---|---|---|
| `STU-135`–`STU-150` | MUST | Studio: USSD simulator beside the canvas with operator profiles; Blocked report with one-click fixes; per-element channel panel; channel enablement and exclusions; home-menu and organisation-menu editors; short codes and ownership proof; adapters and provider switch; exceptions with approval; push settings; runtimes; data residency; user-data store mode; auth and storage delegates; `sendSms` on the canvas (own palette entry, quick add on success and failure branches); USSD PIN notice; release approval channel diff. | D-17, D-24, D-25 |
| `CLI-009`–`CLI-014` | MUST | `plux ussd simulate` (interactive and scripted), `plux ussd codes`, `plux ussd adapters test` (contract suite against a live or sandbox provider), `plux ussd runtimes`, `plux ussd debug` (test-number stream), `plux sms templates`. | D-22 |
| `DEP-061`–`DEP-066` | MUST | Helm and Compose run the `ussd` role with its own ingress and SMPP egress; network policies; the restricted database role; runtime enrolment in the chart; residency documentation; runbooks. | D-28.9, Q-6 |
| `NFR-050` | MUST | ≥ 1,000 key presses per second per `ussd` instance on the reference node. | D-20 |
| `NFR-051` | SHOULD | ≥ 2,000 key presses per second per instance. | D-20 |
| `NFR-052` | MUST | Plux time per key press ≤ 150 ms p95, excluding institution and third-party APIs. | D-20 |
| `NFR-053` | MUST | Executor overhead per action step ≤ 20 µs p95 on the reference node (the server counterpart of `ACT-008`). | D-8 |
| `NFR-054` | MUST | A session survives an instance loss with no lost or repeated step (measured in the chaos test). | L-16, L-17 |

### 3.2 Stays `WIP` at the P17 exit

- `NFR-050`, `NFR-051`, `NFR-052`: until measured on the §30.1 reference deployment (the CI run is
  a scaled regression guard only).
- Live-network verification of `USSD-007` and `USSD-008` is a manual check (layer 1); the rows
  are `DONE` on code plus the simulator contract suite (decision record D-6).

### 3.3 Existing requirements P17 changes

| ID / section | Change | ADR |
|---|---|---|
| `SRV-001` | Adds the role `ussd`. | 0104 |
| `HST-010` | Plux still implements no end-user login; on text channels the app's server-side auth delegate supplies the token. | 0107 |
| `HST-001` | Notes that text channels take user context and consent from the auth delegate. | 0107 |
| `GEN-001` | Applies to apps with the Flutter channel; USSD-only apps generate no project. | 0112 |
| `SEC-007` | Notes that text-channel sessions are `AL0` on the device dimension, with factors (L-13). | 0108 |
| `SEC-027`, `SEC-032` | Gain the USSD SCA mapping. | 0107 |
| `SEC-053` | Confidential content keys may be wrapped to enrolled server runtimes. | 0106 |
| `SEC-080` | Lists the capabilities `sms`, `sms.anyRecipient`, `ussd.push`. | 0111 |
| `SEC-161` | Consent on text channels (`CHN-021`). | 0107 |
| `ANL-001`, `ANL-002` | Channel tag; device budgets do not apply to the role. | 0104 |
| `WGT` descriptor rows, `ACT` catalogue | The text-channel class per element (Appendix C, D). | 0104 |
| §3.2 non-goals | Written exception for the Plux-hosted user-data mode. | 0109 |
| §5 | P17 row, milestone M3 lists P17, the flowchart shows P11 → P17 → P12, and the phase count. | — |
| §6 | The `ussd` role in the component diagram; layering rule L-8. | 0104 |
| §30.1 | A `ussd` node (2 vCPU, 4 GiB) in the reference deployment. | — |
| §31 | Ethiopia's Personal Data Protection Proclamation No. 1321/2024 (articles checked against the gazette before citing). | — |
| §34 | Risks: operator contracts, PIN exposure on the channel, SIM swap. | — |
| Appendices F, H, K | Error codes, security settings, counts. | — |

---

## 4. Specification, ADRs, contracts and dependencies (milestone U0)

### 4.1 Specification (1.14.0 → 1.15.0)

- `tools/internal/spec/lint.go` and its test accept P17 (`phasePattern`); the conventions table
  gains the areas `CHN` and `USSD` (which `spec-lint` reads).
- §5 row, milestone table and flowchart; §36 with the rows of §3.1; the changes of §3.3;
  Appendix C and D gain the text-channel class column (generated); Appendix F, H, K.
- `make spec-lint` after every edit; the revision history entry names the decision record.

### 4.2 ADRs

0104–0112 (§2.4), each `Accepted` before the milestone that implements it merges. Revisions:
ADR-0006 (role), ADR-0010 (descriptor channel classes), ADR-0055 (runtimes as recipients),
ADR-0056 (server-runtime caller, USSD SCA), ADR-0060 (PKCS#11 `Derive`).

### 4.3 Contracts

- Registry schemas: `widget-descriptor.schema.json` and `action.schema.json` gain `channels`;
  every descriptor is classified (U1); `make gen` regenerates the coverage table, the widget
  reference and the class tables.
- Document schema: the optional `channels` fields of L-4; `schemaVersion` bump with its
  migration and test.
- Bundle: the manifest's `channel_features` list (L-5); the FlatBuffers tables for channel
  settings, additive; `make gen`.
- `schema/channels/ussd.json`: the channel definition (encodings, control labels, defaults);
  `schema/ussd/profiles/*.json`: gateway profiles (Ethio Telecom, Safaricom Ethiopia, Africa's
  Talking, simulator) with a JSON Schema.
- Protobuf (§2.4) with authorisation-matrix entries for every RPC.
- Errors, limits, security settings, permissions and capabilities registries (§2.4).
- Conformance vectors: `schema/testdata/actions/` (new), `schema/testdata/ussd/` (render and
  session vectors).

### 4.4 Dependencies

None (§2.3). Every new capability uses the Go standard library, the existing packages
(`internal/pxl`, `internal/l10n`, `internal/bundle`, `internal/gateway`, `internal/signing`,
`internal/cache`), and Studio's existing stack.

---

## 5. Architecture

### 5.0 Starting point (surveyed 2026-11-11)

- `plux-server` has the roles `api`, `worker`, `gateway`, `fnrunner`, `egress` and `builder`
  (`internal/config/config.go`); `NoDataAccess()` already models roles without database access.
- `internal/bundle` verifies containers and holds the confidential-bundle primitives (AES-GCM
  sections, ECDH-ES key wrap, `KeyInfo` bound to device, app, environment and sequence); Go
  accessors for every bundle table exist in `internal/bundle/fbs`.
- `internal/pxl` is the PXL compiler and VM, sharing vectors with Dart (`PXL-007`);
  `internal/pxl/phone` validates numbers by region; `internal/l10n` formats messages, numbers, dates and
  calendars.
- `internal/gateway` is the secure gateway: upstream registry, operation allowlist, user-token
  exchange (RFC 8693), assertions, SCA challenges with dynamic linking; it is keyed on
  `device.Identity`.
- `internal/signing` has the file, Vault Transit and PKCS#11-helper backends (`Sign`,
  `PublicKey`), and envelope encryption for secrets (`SEC-106`).
- `internal/cache` is the project's own Valkey (RESP) client; `internal/dsr` serves data-subject
  requests; `internal/telemetry` ingests events.
- **Missing:** any server-side execution of pages or actions ("the server has no renderer",
  `internal/reconstruct`), action conformance vectors, any USSD, SMPP or SMS code, any channel
  concept in descriptors, documents or Studio.

### 5.1 Classification and document fields (U1, U2) — ADR-0104

- U1 classifies all 207 widget descriptors and 58 action descriptors per the decision record's
  D-3 table (`unsubscribe` is Silent, since `subscribe` is Blocked; `startPayment`, a P13 action,
  is Blocked until P13 classifies it); each mapped entry names its mapping (`menu`, `choice`, `input:<type>`, `text`,
  `paged-text`, `screen`, `line-next-screen`, `pin-step`, `sms-link`, `sms-content`, `sca`), each
  blocked entry its reason. A generator test fails when a new descriptor has no class.
- Classification is reviewed by the lead entry by entry; disputed entries are Blocked (safe
  default).

### 5.2 Compiler (U2) — ADR-0104

- Channel validation is part of the pure compiler (L-5): per enabled channel it walks the
  reachable flow graph from the channel's entry points (home menu, deep links) through
  navigation, action branches and component instances, and reports each reachable blocked
  element with its node path and the two fixes (`PLX-2400`–`2419`).
- It validates channel settings (labels within budget at the smallest encoding, alternatives
  type-checked like the original, hidden elements not required by validators), USSD-only pages,
  the home menu, store-mode prerequisites (`CHN-015`), template declarations for `sendSms`, and
  exceptions referenced by guards.
- Output stays byte-identical for the same input (`CMP-002`); channel data are additive
  sections.

### 5.3 The `ussd` role (U6) — ADR-0104

- `internal/config`: `RoleUSSD`, its section (listeners for HTTP gateways, SMPP outbound binds,
  internal mTLS to `api` and `gateway`, Valkey, the restricted database DSN), validation with
  secure defaults (no plaintext listener outside development, mTLS required).
- Layering (L-8) is enforced by tests: the role's dependency graph may not import
  `signing` key material packages, the gateway's upstream credential store, or release-write
  packages; the database role's grants are tested in the integration suite.
- Graceful shutdown drains sessions to Valkey; health and readiness reflect adapter binds.

### 5.4 Server runtimes and confidential bundles (U7) — ADR-0106

- `internal/runtimes`: identity (ID, installation, allowed organisations/apps, mTLS
  certificate fingerprint, workload identity), enrolment (installation admin, step-up,
  audited), revocation, key rotation.
- Keys: a signing key (proofs, L-10) and a key-agreement key (confidential content), both in the
  signing backend (L-12); `pkcs11pb` gains `Derive`; the Vault backend's enveloped key-agreement
  key is decrypted only inside `signing/`.
- Sync: an internal `runtimepb` service on the `api` role mirrors the device manifest and bundle
  calls (control document, rollouts, experiments, eligibility), authenticated by mTLS plus a
  proof from the runtime key. Content keys are wrapped with `bundle.WrapContentKey` using a
  runtime-specific `KeyInfo` (runtime ID instead of device ID), so a key wrapped for a device can
  never open for a runtime or the reverse.
- In the role: verify (`SEC-052`), decrypt in memory (never disk, `SEC-053`), cache decoded
  releases (`CHN-033`), drop keys on revocation within the control propagation target.

### 5.5 The Go executor (U3) — ADR-0105

- `internal/textrt`: loads a verified release; resolves the app, plugin, page and component
  tables from `bundle/fbs`; instantiates state scopes (app, plugin, page, component) with the
  same typing and defaults as the Dart runtime; evaluates bindings with `internal/pxl`; runs
  action graphs with the `ACT-001`–`ACT-006` semantics (typed branches, concurrency policies
  where meaningful on a single-input channel, bounds, retries, cancellation); maintains the
  navigation stack and route guards; runs forms and validators.
- Actions by class: native ones implemented; silent ones no-ops with a trace; mapped ones emit
  their screen-model step (PIN step, SCA step, message line, SMS); blocked ones are unreachable
  after compilation and refused defensively at runtime (`PLX-4510`).
- Data actions call the gateway role through the server-runtime caller (L-9); device-storage
  actions call `internal/userdata` (§5.9); `invokeFunction` calls `fnrunner` as the server
  does today; device-placed functions run in `fnrunner` (`CHN-025`).
- Output: the screen model (`textrt/screen`), never Flutter widgets.
- Conformance: `schema/testdata/actions/` vectors (inputs, initial state, mocked data-source
  responses, expected state, branches, traces, errors) run by a Go runner and a new Dart runner
  in `packages/plux_flutter/test/actions/`.

### 5.6 USSD rendering (U4) — ADR-0104

- `internal/ussd/render`: screen model → screens: title, body, one input or a numbered choice
  list, control lines; splitting long text and lists by the budget (`USSD-015`, `USSD-016`);
  encoding chosen per screen (GSM-7 when every character is in the GSM-7 default alphabet and
  its extension table, else UCS-2); budgets from the registry, lowered by the adapter profile.
- Input mapping and validation per input type (`USSD-017`), re-prompt lines, localised control
  labels, the language menu (`USSD-018`).
- Property test: no generated screen exceeds its budget for any locale and content; render
  vectors in `schema/testdata/ussd/`.

### 5.7 The session engine (U5) — ADR-0104

- `internal/textchan`: a channel-neutral state machine (begin, step, end, abort, timeout) keyed by
  (channel, gateway session ID); state encrypted per app (L-18) in Valkey with a TTL of
  `ussd.sessionSeconds`; release pinning (`CHN-013`); idempotent steps (L-16); one session per
  user and app (`CHN-031`); fail closed on Valkey loss (L-17).
- Time budgets: Plux's own budget per key press and the API budget; on expiry, the app's
  slow-call policy (`CHN-020`): background completion with an SMS result (the run continues
  detached under `ACT-004` rules) or an error screen.
- The test-only second channel (`CHN-028`) lives in `textchan/testchan`.

### 5.8 Gateway adapters (U10, U11) — ADR-0110

- `internal/ussd/adapter`: the interface (§3.1 `USSD-002`) with capability flags; adapters are
  stateless and horizontally redundant; provider switching per short code is configuration with
  approval (`USSD-010`).
- `sim`: an in-process and networked simulator with profiles; Studio and the CLI use it.
- `httpgw`: configurable mapping (JSON, XML, form), continue/end markers, auth (mTLS, HMAC, IP
  allowlist), body limits, strict parsing (no XML external entities, bounded depth).
- `africastalking`: the public USSD callback (form POST, `CON`/`END` replies) and SMS API; the
  API key through `SEC-106` secrets.
- `smpp`: an own client (D-31): PDU encoder and decoder for the needed PDUs and TLVs, sequence
  numbers and windowing, `enquire_link`, reconnect with back-off, bind credentials as secrets,
  TLS where offered; the decoder is fuzzed and bounded (PDU length limits). A test SMPP server
  (`smpp/smpptest`) plays the operator side in the contract suite.
- `profiles`: Ethio Telecom and Safaricom Ethiopia over SMPP and HTTP, every operator value
  configurable, validated by `schema/ussd/profiles` (D-6).

### 5.9 Routing, short codes and menus (U6) — ADR-0112

- Tables (migration 0068): short codes per organisation with ownership proof (a verification
  dial through the adapter, or the provider API confirming the route) and installation-admin
  approval; routes (code → app or organisation menu; sub-codes → plugin or page through the app);
  menus (generated, edited overrides).
- A route resolver cached in the role, invalidated through the control channel; validation that
  no root code opens a bare plugin (`USSD-013`).

### 5.10 Identity, PIN, SIM swap, exceptions and SCA (U8) — ADR-0107, ADR-0108

- `internal/ussd/identity`: auth delegate registration (endpoint with mTLS, or a Plux Function),
  the institution's PIN-envelope public key (rotation supported), the PIN envelope (L-11), token
  validation and binding (L-10), user context and consent (`CHN-021`).
- Lockout and rate limits per number and app in Valkey with durable audit; SIM-swap query through
  the adapter capability with the `ussdSimSwap*` settings; risk limits per app (`USSD-025`).
- Exceptions: stored per app and guard (migration 0072), requested with
  `ussd.exceptions.request`, approved through the approval engine with step-up, listing accepted
  factors (L-13), shown with a distinct warning in Studio and in the audit.
- SCA (`USSD-027`): the SCA challenge of `internal/gateway` gains a USSD path: the summary
  (amount, payee) is rendered, the PIN is sealed together with the summary hash, the institution
  verifies both and returns a signed approval, the gateway verifies it and issues the assertion
  with method `ussd-pin`.
- Optional in-app approval (`USSD-028`): a push to the registered `AL2` device through the
  existing push channel, with the same summary; the USSD session waits within its budget.

### 5.11 The user-data store (U9) — ADR-0109

- `internal/userdata`: one interface, two implementations. Plux-hosted (migration 0073):
  partitioned by app, per-app data keys (L-18), versioned records, retention sweeps, DSR export
  and erasure through `internal/dsr`. Institution-hosted: the storage-delegate client over mTLS
  through `egress`, with timeouts (`userData.delegateTimeoutMs`), versions and typed errors
  (`PLX-5600`–`5699`).
- The store mode is per app with an organisation default; switching is audited and never
  migrates data silently (an explicit export/import tool is out of scope; noted in §11).

### 5.12 Push, SMS and `sendSms` (U12) — ADR-0111

- `internal/ussd/push`: push requests from actions and server events, every sub-feature gated
  (D-18), consent and quiet hours (migration 0074), rate limits, the registered app name, the
  PIN window, audit.
- `internal/messaging`: SMS through the app's adapter, templates (migration 0075), segments and
  encodings, delivery reports, limits, audit with metadata only (no message bodies stored beyond
  delivery).
- `sendSms`: a new action descriptor (`category: messaging`), native on both channels; on the
  Flutter channel it calls `MessagingService.SendSms` with DPoP (L-19).

### 5.13 Errors (first allocations)

| Code | Meaning |
|---|---|
| `PLX-2400` | Blocked element reachable on an enabled channel |
| `PLX-2401` | Channel alternative does not type-check |
| `PLX-2402` | Channel label exceeds the smallest screen budget |
| `PLX-2403` | Plugin uses device storage but the app has no user-data store mode |
| `PLX-2404` | USSD-only page or home-menu entry is invalid |
| `PLX-2405` | `sendSms` template undeclared or invalid |
| `PLX-3200` | Short code not registered or not approved |
| `PLX-3201` | Route points at a bare plugin |
| `PLX-3202` | Gateway connection not authenticated |
| `PLX-3203` | Adapter capability missing (push, SMS, SIM swap) |
| `PLX-3204` | No active provider for the short code |
| `PLX-4500` | Session expired or unknown |
| `PLX-4501` | Session state unavailable (fail closed) |
| `PLX-4502` | Release not available to this runtime |
| `PLX-4510` | Blocked element reached at runtime |
| `PLX-4511` | Response budget exceeded |
| `PLX-5600` | Storage delegate unavailable or timed out |
| `PLX-5601` | User-data version conflict |
| `PLX-5602` | User-data quota exceeded |
| `PLX-6300` | PIN verification failed |
| `PLX-6301` | PIN locked out |
| `PLX-6302` | SIM swapped within the block window |
| `PLX-6303` | SIM-swap status unknown and policy denies |
| `PLX-6304` | Channel not allowed by the security profile and no exception |
| `PLX-6305` | Exception does not accept the session's factors |
| `PLX-6306` | Session-bound token invalid, expired or bound elsewhere |
| `PLX-6307` | USSD SCA approval invalid or not linked to the summary |
| `PLX-6308` | Push not allowed (sub-feature off, consent, quiet hours, rate) |
| `PLX-6309` | Server runtime not enrolled or revoked |
| `PLX-6310` | Risk limit exceeded |
| `PLX-9400` | Simulator profile invalid |
| `PLX-9401` | Short-code ownership proof failed |

### 5.14 Studio (U14) — ADR-0104, ADR-0112

- `features/simulator/`: a phone-shaped USSD simulator beside the canvas (operator profile,
  encoding, budget meter per screen, PIN notice, test-number debug stream), driven by
  `SimulateStart`/`SimulateStep`.
- `features/ussd/`: the Blocked report (Problems tab integration, one-click "Hide on USSD" and
  "Add USSD alternative"), the per-element channel panel in the right dock, channel enablement and
  exclusions, home-menu and organisation-menu editors, short codes and ownership proof, adapters
  and provider switch, exceptions with approval and step-up, push settings, runtimes (installation
  admin), data residency, store mode and delegates.
- Canvas: `sendSms` with its own palette entry and quick add on success and failure branches.
- Release approval: the channel preview and diff (`CHN-029`).
- shadcn/ui per ADR-0103; messages in every Studio locale; accessibility as for the rest of
  Studio; unit, e2e and design-gate checks.

### 5.15 CLI, dev tools, telemetry and metering (U15)

- CLI commands of §3.1 (`CLI-009`–`CLI-014`), sharing the simulator.
- Dev tools: test-number sessions emit dev events with the P10 redaction (`USSD-038`).
- Telemetry with the channel tag and keyed number hash; funnels and drop-off per screen in the
  analytics area; metering counters (migration 0076).

### 5.16 Flutter runtime (U13)

- Ignores channel fields (they are additive) and `channel_features` (L-5); a test proves a
  release with USSD settings renders identically.
- The app-to-USSD handoff (`USSD-036`): connectivity check, prompt, Android dial intent of the
  deep-link sub-code, iOS display; per-app setting.
- The `sendSms` handler calling `MessagingService.SendSms`.

### 5.17 Deployment and operations (U16)

- Helm: a `ussd` deployment, its own ingress for HTTP gateways, egress rules for SMPP binds,
  the restricted database role, runtime enrolment, network policies; Compose: the same for local
  runs; kind e2e with the simulator (`DEP-061`–`DEP-066`).
- Residency guidance for Ethiopian installations (role, Valkey and the Plux-hosted store in the
  country) and the residency warnings (`USSD-037`).

---

## 6. Milestones

### 6.1 Milestones and dependencies

| # | Milestone | Closes | Depends on | Size | Model |
|---|---|---|---|---|---|
| U0 | Spec 1.15.0, ADRs 0104–0112, contracts, registries, `spec-lint` P17 | prerequisite | decisions | L | lead |
| U1 | Descriptor and document classification, schema fields, generators | `CHN-002`, `CHN-003` (tables), `CHN-006`, `CHN-032` | U0 (schemas) | M | Haiku for entries, Sonnet low for generators; lead reviews every class |
| U2 | Compiler channel validation and reports | `CHN-005`, `CHN-007`, `CHN-015` (compile part), `PLX-24xx` | U1 | M | Sonnet |
| U3 | Go executor and action conformance vectors (Go and Dart runners) | `CHN-004`, `CHN-009`, `CHN-010`, `CHN-011`, `CHN-025`, `CHN-033`, `NFR-053` | U1 | XL | Sonnet medium; split into executor core, actions by category, vectors |
| U4 | USSD rendering, encodings, inputs, language | `USSD-015`–`USSD-018`, `CHN-026` | U3 (screen model) | M | Sonnet |
| U5 | Session engine, Valkey, idempotency, budgets, slow-call policy, test channel | `CHN-012`–`CHN-014`, `CHN-020`, `CHN-028`, `CHN-031`, `USSD-019` | U3 | L | Sonnet medium |
| U6 | `ussd` role, L-8, routing, short codes, menus | `USSD-001`, `USSD-012`–`USSD-014`, `USSD-035` | U0 | M | Sonnet; migrations Opus |
| U7 | Server runtimes, keys, sync, confidential bundles, PKCS#11 `Derive` | `USSD-031`–`USSD-034`, `SEC-053` change | U0, U6 | L | Opus (cryptography) |
| U8 | Identity: auth delegate, PIN envelope, bound tokens, lockout, SIM swap, risk limits, exceptions, SCA, in-app approval, gateway caller | `USSD-020`–`USSD-028`, `CHN-019`, `CHN-021` | U5, U6, U7 | XL | Opus (security) |
| U9 | User-data store, both modes, DSR | `CHN-015`–`CHN-018`, `CHN-031` | U5 | L | Sonnet high; migration Opus |
| U10 | Adapter interface, simulator, HTTP adapter, Africa's Talking | `USSD-002`–`USSD-004`, `USSD-006`, `USSD-011` | U5 | L | Sonnet |
| U11 | SMPP client and test server, Ethio Telecom and Safaricom Ethiopia profiles, contract suite | `USSD-005`, `USSD-007`, `USSD-008`, `USSD-010` | U10 | L | Sonnet medium; lead reviews the decoder |
| U12 | Push, SMS, `sendSms` (server and RPC) | `USSD-029`, `USSD-030`, `CHN-027` | U8, U10 | M | Sonnet high |
| U13 | Flutter runtime: field tolerance, handoff, `sendSms` handler | `USSD-036`, device part of `CHN-027` | U12 (RPC) | S | Sonnet |
| U14 | Studio (six slices: simulator; report and fixes; channel panel and enablement; menus and codes; adapters, exceptions, push, runtimes, residency, store; canvas `sendSms` and approval diff) | `STU-135`–`STU-150`, `USSD-039`, `CHN-029` | U2, U5, U6, U8, U10, U12 | XL | Sonnet per slice |
| U15 | CLI, dev streams, telemetry, analytics funnels, metering | `CLI-009`–`CLI-014`, `USSD-038`, `CHN-022`–`CHN-024`, `CHN-030` | U5, U10 | M | Sonnet |
| U16 | Deployment, benchmarks, chaos test, runbooks, docs, onboarding document, reference app, threat model, gates, statuses | `DEP-061`–`DEP-066`, `NFR-050`–`NFR-054`, `USSD-009`, `USSD-037`, `USSD-040`, X1–X8 | all | L | lead; docs Haiku |

### 6.2 Waves

1. U0 and U1.
2. U2, U3, U6.
3. U4, U5, U7.
4. U8, U9, U10.
5. U11, U12, U15.
6. U13, U14.
7. U16.

A wave starts once the previous wave's code is merged on `feat/pre-12-full-ussd`; the full gate
runs once per wave and in U16 (as in P11). Briefs are written by the lead from §5 and §2.4,
self-contained and middle-sized.

---

## 7. Security design

### 7.1 Threat-model delta (STRIDE, added to `docs/security/threat-model.md` in U16)

| Threat | Where | Mitigation |
|---|---|---|
| Spoofed caller number | Gateway ingress | Authenticated gateways only (L-15, `USSD-011`); number trusted only from them |
| SIM swap takeover | Possession factor | SIM-swap query and block window; policy when unknown (L-14); lockout; in-app approval option |
| PIN capture | Operator, aggregator, Plux edge | PIN exposure on the channel accepted and documented (Q-7); Plux side: memory-only, sealed to the institution (L-11), never logged or stored (`USSD-021`) |
| PIN guessing | Identity | Lockout and rate limits per number and app (`USSD-023`) |
| Token theft | Between role and gateway | Tokens bound to the runtime key and the session (L-10); short lifetimes |
| Replay or duplicate delivery | Gateway retries | Idempotent steps and idempotency keys (L-16) |
| Phishing by push | Push | Sub-features off by default, registered app name, PIN-in-push window, consent, quiet hours, rate limits, approval (`USSD-029`) |
| SMS relay abuse | `sendSms` | Own-number default, capability with approval, templates only, limits (L-19) |
| Code hijack | Short codes | Ownership proof and installation-admin approval (`USSD-012`) |
| Tampered or rolled-back bundle | Runtime | Verify before load, TUF metadata as devices, anti-rollback (`CHN-010`) |
| Content-key leakage | Runtime | Keys in the signing backend, runtime-specific `KeyInfo`, memory-only decryption, revocation (`USSD-033`) |
| Compromised `ussd` role | Edge | L-8: no signing, no upstream credentials, restricted database role, mTLS everywhere |
| Malformed SMPP or XML input | Adapters | Bounded, fuzzed decoders; no XML external entities; body limits |
| Denial of service | Gateway ingress | Abuse controls and back-pressure (`USSD-040`) |
| Session-state disclosure | Valkey | Per-app encryption, no sensitive inputs persisted (L-18), fail closed (L-17) |
| Data residency breach | Deployment | Residency region and warnings (`USSD-037`), guidance |
| Over-broad exceptions | Governance | Approval with step-up, per-guard scope, declared factors, distinct warning, audit (`USSD-026`) |

### 7.2 Security tests (all automated, in U7–U12 and U16)

PIN absent from logs, traces, metrics, Valkey dumps, database rows and dev streams (scrape tests
with a canary PIN); envelope round trip and tamper refusal; lockout and unlock; SIM-swap
allow/limit/deny; exception refusal without factors and without approval; USSD SCA with a
modified amount refused; token replay from another session or runtime refused; push PIN outside
the window refused; `sendSms` to another number refused without the capability; runtime revoked
mid-session loses keys and bundles; duplicate SMPP and HTTP deliveries do not re-run actions;
gateway requests from unauthenticated sources refused; fuzzing of SMPP PDUs, HTTP/XML bodies and
the render splitter; ASVS checks for the new endpoints recorded in `docs/security/asvs-server.md`.

---

## 8. CI, coverage and benchmarks

- New path-selected jobs in `ci/affected.json`: `ussd` (Go tests of the new packages with
  `-race`, the contract suite, the session suite `test/ussd`), `bench-ussd` (nightly and on
  changes to `textrt`, `textchan`, `ussd`), the nightly Africa's Talking sandbox job (skipped
  without its credential), the Dart action-vector runner in the Dart job, Studio jobs as today.
- Coverage floors (`coverage.json`): 85 % for `textrt`, `textchan`, `ussd/identity`,
  `ussd/adapter/smpp`, `runtimes`, `userdata` (security-relevant or engine), 80 % for the other
  new Go packages, the package defaults for Dart and Studio (`QA-001`).
- Benchmarks: `make bench-ussd` drives the simulator at a fixed rate through a full flow (dial,
  inputs, gateway call to a stub upstream, list, PIN, end) and reports key presses per second and
  Plux time p50/p95/p99; the CI run guards regressions against a committed baseline; the
  reference run (X6) is recorded in `docs/benchmarks/ussd.md` with method and hardware.
- Chaos test (`NFR-054`): kill an instance mid-session; the next step on another instance
  continues without a lost or repeated step.

## 9. Testing strategy (spec §28, `testing.md`)

- Every requirement named in at least one test (`make trace`).
- Conformance: action vectors on both engines; render and session vectors on Go; l10n vectors.
- Property tests: screen budgets never exceeded; idempotency under random duplicate deliveries;
  executor equals Dart on generated action graphs (differential test through the vectors).
- Contract suite: one suite every adapter must pass (simulator, HTTP, SMPP via the test server,
  Africa's Talking via a recorded fixture server, both Ethiopian profiles).
- Integration: the role with Postgres, Valkey, `api`, `gateway` and `fnrunner` in the Compose
  stack; the kind e2e with the simulator.
- Security: §7.2. Determinism: compile output with channel data byte-identical (`CMP-002`).
- Studio: unit, e2e (simulator flow, Blocked report fixes, exception approval with step-up),
  design gate.

## 10. Documentation, reference app and operations (DoD-7 to DoD-9)

- User docs: the channel model and classes, building for USSD, the simulator, short codes,
  adapters, store modes and delegates (with the delegate contracts), exceptions and their
  factors, push, `sendSms`, residency.
- The **operator onboarding document** (`docs/guides/ussd-operator-onboarding.md`, D-27): what to
  request from Ethio Telecom and Safaricom Ethiopia (interface and specification, sandbox, test
  numbers, IP ranges, certificates, bind credentials, timeouts, character limits, push, SMS and
  SIM-swap support, delivery reports), how to fill in each profile, the acceptance tests
  (`plux ussd adapters test`), security review items, and the steps that turn a profile live.
- Runbooks (`docs/runbooks/`): gateway or provider outage and provider switch, SMPP bind
  failures, Valkey outage (fail closed), HSM or Vault outage (runtime keys), runtime revocation,
  SIM-swap feed outage, push abuse, SMS cost spikes.
- Reference app: Plux Bank gains the X1 transfer flow and a USSD channel on the simulator.

## 11. Hand-offs to later phases

- P12 (AI): generate and respect channel classes and settings; propose USSD alternatives.
- P13 (Payments): classify `startPayment` and provider adapters per channel (push-based providers
  native, card entry blocked).
- P14 (Collaboration): channel settings in co-editing and comments.
- P15 (Multi-tenant): short codes per tenant, tenant keys for user data and session state.
- P16 (Super-app): whether partner mini-apps appear in a host's USSD menu.
- Later: SMS bots, WhatsApp, IVR as text channels; an export/import tool between store modes.

## 12. Risks

| Risk | Mitigation |
|---|---|
| Executor drift from the Dart engine | Shared vectors (L-7) and the differential test; vectors required for every implemented action |
| Executor scope (58 actions, state, forms, navigation) larger than estimated | Classification first (U1) shows the exact native and mapped set; U3 is split by action category; blocked actions need no implementation |
| Operator specifications differ from the assumed interfaces | Profiles over two interfaces, all values configurable; the onboarding document; a third interface would be one adapter, not a redesign |
| 1,000 key presses per second missed | Plux budget per key press measured from U5; release cache, Valkey pipelining, no per-session HSM work (L-10); horizontal scaling |
| HSM throughput for proofs | Proof signing per gateway call through `signing/`; cache-free but batched where the backend allows; measured in `bench-ussd` |
| PIN exposure on the channel | Inherent; documented (Q-7), compensated by limits, SIM-swap checks, exceptions with factors, optional in-app approval |
| No SIM-swap data from Ethiopian operators | `ussdSimSwapUnknown` policy (L-14) |
| Banks need `maximum` and USSD together | Exceptions with factors (D-14, L-13) |
| Studio scope | Six slices over two waves; shadcn components only |
| Data residency | Residency region, warnings, guidance (Q-6) |

## 13. Process

- The P10/P11 playbook: agents in worktrees from self-contained briefs, the smallest model that
  does the job (Haiku for mechanical work, Sonnet low by default, Opus for security, cryptography
  and migrations), the lead reviews every diff, merges, runs `make gen` and the gates, and owns
  CI. Agents never reformat generated files.
- The lead runs `make check`, `make trace`, the CI run and the benchmarks at U16, sets the
  statuses in the spec, updates `docs/WORKLOG.md`, and writes the pull request.
