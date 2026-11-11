<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Full USSD (P17, `pre-12-full-ussd`) — Decisions

> **Status:** Decision record, built up question by question in a preparation session with the
> maintainer (2026-11-02 to 2026-11-10), before the phase is written into the specification.
> Binding input for the spec change, the phase plan, its ADRs and every session of the phase.
> Nothing here is implemented, and no other document has been changed yet.

- **Purpose:** settle the scope and the technical questions of the USSD phase up front, so the
  spec change and the implementing sessions start from decisions instead of reopening them.
- **Starting point:** before this record the repository named the phase only in the schedule
  (`plans/audit-phase.md`, `plans/p11-handoff/next.md`, `manual-checks/`): after P12, before P16
  and the cross-phase audit, "not yet in the specification". No requirement, ADR or code
  mentions USSD.
- **Not yet applied:** the follow-ups listed with each decision, and the work list in §4, are
  made in the pull request that writes the phase into the specification, with the maintainer's
  approval where `AGENTS.md` §7 requires it.
- **Open questions:** §3 lists what this record does not settle yet. They are answered before
  the spec change.

---

## 1. The goal in one paragraph

One development serves two channels. A developer designs plugins, pages, actions, API calls and
messages once in Studio; the same signed release renders as native Flutter widgets on phones and
as USSD menus on any GSM handset. Every widget and action is classified for USSD: most need no
USSD work at all, some get an automatic USSD equivalent, decoration is dropped, and the few that
cannot work on a keypad are reported before publishing with a one-click fix. Banks and other
financial institutions run USSD under the `maximum` security profile through explicit, audited
exceptions with compensating controls.

---

## 2. Decisions

### D-1. Phase identity

- **Decision:** the phase is **P17 "Full USSD"**; the plan name is `pre-12-full-ussd` and the
  phase branch is **`feat/pre-12-full-ussd`**, cut from `claude/handoff-studio-fixes` (the latest
  branch on 2026-11-11, maintainer). It is scheduled **after P11 and before P12**; the execution order is kept
  in `docs/WORKLOG.md` and the plans, as for the cross-phase audit. Numbers P12–P16 do not
  change.
- **Reason:** renumbering P13–P16 would touch every phase reference in the spec, ADRs and plans
  for no functional gain.
- **Follow-ups:**
  - `tools/internal/spec/lint.go` accepts P0–P16 only (`phasePattern`) and its test expects
    "outside P0–P16"; extend both to P17 (a CI-gate change, approved with this decision).
  - `tools/internal/trace/report.go` orders phases numerically; the order is by number, not by
    schedule, which is acceptable for reports.
  - Spec §5: a P17 row (scope, exit criterion), the phase summary table, and a sentence that
    P17 runs before P12.
  - Requirement prefix: `USSD-` (a new area in the §4 conventions table, which `spec-lint`
    reads).
  - Update `plans/audit-phase.md`, `plans/p11-handoff/next.md`, `manual-checks/README.md` and
    `manual-checks/layer-3-confidence/README.md` to name P17 and its new place before P12.

### D-2. One source, two channels (the core model)

- **Decision:** USSD is a **channel of the same document model**: the same plugins, pages,
  actions, state, data sources and Functions serve Flutter and USSD. Nothing is authored twice.
  Example: a page with a title, two inputs, a submit action, a success message, an `apiCall`
  through the egress proxy and a result list becomes, on USSD, a title with one prompt per
  input, the message, the same API call through the same proxy, and the result as a numbered
  menu.
- **Rejected:** a separate USSD flow type (logic duplicated), automatic guessing from the
  Flutter layout (unpredictable).

### D-3. USSD classification of every widget and action

- **Decision:** every widget descriptor (`schema/widgets`) and action descriptor
  (`schema/actions`) declares a **USSD class**, generated into the registry like the other
  descriptor fields. Widget descriptors already carry `platforms` (`["android","ios"]` for all
  207); actions carry `category` and `effects`. Four classes:

  | Class | Behaviour | Examples (from the current catalogue) |
  |---|---|---|
  | **Native** | Runs as it is | `Text`, `RichText`, `TextField`, `TextFormField`, buttons as menu options, `ListTile`, `ListView`/`ForEach` as numbered lists, `Checkbox`/`Radio`/`Switch`/`DropdownMenu` as choice menus, `If`, `Match`, `FormScope`, `DataScope`; actions `apiCall`, `invokeFunction`, `setState`, `patchState`, `resetState`, `submitForm`, `validateForm`, `resetForm`, `navigate`, `pop`, `condition`, `switch`, `forEach`, `parallel`, `stop`, `callFlow`, `delay`, `readConfig`, `refreshData`, `trackEvent`, `setLocale`, `logout`, `emitEvent`, local DB and KV actions (server-side store, D-9) |
  | **Silent** | Dropped; children are kept | Layout (`Row`, `Column`, `Padding`, `Container`, `Center`, `Expanded`, `SizedBox`, slivers…), `Image`, `Icon`, `Divider`, all animated widgets and transitions, progress indicators, `SkeletonLoader`, `LottieView`, `RiveView`; actions `haptic`, `startAnimation`, `controlAnimation`, `setThemeMode`, `sync`, `unsubscribe` |
  | **Mapped** | Automatic USSD equivalent | `biometricAuth`/`BiometricButton` → PIN step (D-12); `showSnackbar`/`showToast`/`announce` → a line on the next screen; `openDialog`/`openBottomSheet` → their own screen; `TabBar`, `NavigationBar`, `BottomNavigationBar`, `Drawer`, `NavigationRail`, `switchTab` → a menu; `PhoneInput`, `AmountInput`, `IbanInput`, `OtpInput`, `SecureTextField`, `SecurePinPad` → validated (sensitive where tagged) text input; date pickers → typed input (`DDMMYYYY`); `CountryPicker`, `CurrencyPicker` → paged lists; `Slider`, `QuantityStepper`, `RatingInput` → number input with bounds; `ConsentCheckbox` → "1 Accept / 2 Decline"; `MoneyText`, `PriceTag`, `TransactionListItem`, `AccountCard`, `ProductCard`, `StatusTimeline`, `DataTable` → text lines; `TermsViewer`, `MarkdownView` → paged text or an SMS link; `openUrl`, `share`, `copyToClipboard` → an SMS with the content (D-22); `signTransaction` → USSD strong customer authentication (D-32) |
  | **Blocked** | Needs a developer decision before a USSD-enabled plugin can publish | `capturePhoto`, `pickImage`, `pickFile`, `scanCode`, `QrScanner`, `BarcodeScanner`, `KycCameraCapture`, `SignaturePad`, `FileUploader`, `getLocation`, `MapView`, `OrderTrackerMap`, `VideoPlayer`, `AudioPlayer`, `callNative`, `emitHostEvent`, `NativeSlot`, native routes, `subscribe` (live streams), `requestPermission`; `startPayment` until P13 classifies it |

- **Rules behind the classes:** decoration is dropped; information becomes text; inputs map to a
  keypad entry where an equivalent exists and are blocked where none does; security actions
  become PIN steps; logic and data run as normal; hardware and host-app features are blocked.
  Actions are never silently skipped unless their class is Silent (they have no effect a USSD
  user could miss).
- **Developer fixes for a Blocked item** (one report, computed per flow, so only reachable items
  count):
  - **Hide on USSD:** the element, branch or page is excluded; routes into it show a
    configurable "available in the app" screen, optionally with an SMS link to the app.
  - **USSD alternative:** an element or branch that runs only on USSD.
- **Overrides:** any element may carry optional USSD settings (label, text form, hide,
  alternative). They are part of the document, so they are signed with the release.
- **Precedent:** Microsoft Adaptive Cards element fallback (`"fallback": "drop"` or a fallback
  element) since 1.2; the "blocked" class is Plux's addition.
- **Follow-ups:** `widget-descriptor.schema.json` and `action.schema.json` gain the USSD class;
  every descriptor is classified; the coverage table and widget reference are regenerated
  (`make gen`); the document schema gains the per-element USSD settings.

### D-4. Unsupported display elements are skipped

- **Decision:** elements classed Silent are skipped without a warning; Blocked elements follow
  D-3.

### D-5. Screen splitting

- **Decision:**
  - One input per screen; every prompt shows the page title.
  - Long text is split across screens with `98 More` as the last line.
  - Lists become numbered menus; each screen holds what fits, with `98 More` and `0 Back`.
  - The character budget per screen comes from the **limits registry** (`LIM-001`), per gateway
    and encoding: 182 characters for GSM-7 text, about 80 for UCS-2 (Amharic and other
    non-Latin scripts). Never hard-coded.
  - A page may override labels such as "More" and "Back" in its USSD settings.

### D-6. Gateways and adapters

- **Decision:** one **gateway adapter interface**, a built-in **simulator**, and three adapters:
  **Ethio Telecom**, **Safaricom Ethiopia** and **Africa's Talking** (the most-used aggregator
  in Africa; no source lists Ethiopia, so it does not overlap the two Ethiopian adapters).
- **Research (2026-11-03):** neither Ethio Telecom nor Safaricom Ethiopia publishes a USSD
  gateway API. The Safaricom Ethiopia developer portal offers M-Pesa APIs; its "USSD Push" is a
  payment prompt, not interactive menus. Both give short codes and their gateway specification
  only under a commercial agreement. Africa's Talking covers Kenya, Uganda, Tanzania and Nigeria
  (confirmed by several sources) and possibly Rwanda, Ghana, Malawi, Zambia and South Africa.
- **All three adapters are implemented fully without a contract (maintainer 2026-11-11).** What
  a contract reveals is not a new technology but the operator's choice among a few known
  interfaces between its USSD gateway and an application server, plus the details: endpoint,
  credentials, IP ranges, certificates, field names, timeouts, character limits, push and SMS
  support. The operator's own core (MAP over SS7 to the handset) is never touched by third
  parties. The known interfaces:
  - **SMPP 3.4 / 5.0 with the USSD extensions:** `service_type` `USSD`, the `ussd_service_op`
    TLV for begin, continue, end and abort, session TLVs (`its_session_info`), `deliver_sm` and
    `submit_sm` or `data_sm` depending on the gateway, `bind_transceiver`, `enquire_link`.
  - **HTTP(S) callbacks:** the gateway POSTs each step to the application, which answers with the
    next screen and a continue or end flag, as JSON, XML or form fields (Africa's Talking: form
    POST, reply `CON …` or `END …`).
  - **SOAP / XML** variants of the same request-response dialogue.
- **So Plux builds:** a full **SMPP USSD client** and a **configurable HTTP/XML/JSON gateway
  adapter**, each with a contract test suite on the simulator; the **Ethio Telecom** and
  **Safaricom Ethiopia** adapters are complete, tested **profiles** on top of both (all
  operator-specific values are configuration); **Africa's Talking** is a complete adapter for
  its public API. When a contract arrives, the work is filling in the profile and running the
  acceptance tests of the onboarding document (D-27), not writing code. Their requirements are
  `DONE` once the code and its simulator tests pass; a live-network check is a manual check.
- **SMPP implementation:** D-31.
- **Failover (amended by the maintainer):** an operator routes a short code to **one** provider
  endpoint, so there is **one active provider per short code**. Redundancy comes from several
  `plux-ussd` and adapter instances standing in for each other. Switching provider is a
  configuration change, approved and audited, with an optional pre-configured standby provider.
  No automatic switching between providers.

### D-7. The USSD server component

- **Decision:** USSD runs in its **own isolated, separately scalable component**, `plux-ussd`,
  so gateway traffic and gateway faults cannot affect Studio, the API or device sync. It follows
  the layering rules: no signing keys, and only the database access it needs (a new rule L-8).
- **Form (Q-1, maintainer 2026-11-11):** a new **`ussd` role of `plux-server`** (ADR-0006,
  `SRV-001` gain the role), deployed and scaled on its own; single-node installations may run it
  in the same process. The name `plux-ussd` in this record means that role.

### D-8. Execution on the server

- **Decision:** `plux-ussd` executes the **same signed bundles** the devices run, with a **Go
  action executor** next to the existing Go PXL VM (`backend/internal/pxl`, which already shares
  conformance vectors with Dart, `PXL-007`). The conformance vectors are extended to actions, so
  both channels behave identically.
- **Rejected:** a headless Dart runtime on the server (a second server stack); interpreting the
  unsigned Studio documents (breaks verify-before-load, `SEC-052`).

### D-9. State and storage

- **Decision:**
  - **Live session state** (current page, inputs, state values) lives in **Valkey**, with expiry
    at the session timeout; sensitive values are encrypted before they are stored. Sessions
    survive a restart or a move to another instance.
  - **Device storage** (local DB, KV, saved state: a half-filled form, a cart, cached recent
    transactions, the chosen language) is backed on USSD by a **configurable user-data store**,
    so the same actions keep working. Accounts, usernames and PINs are never part of it; they
    stay in the institution's identity system (D-12).
  - **Two modes, chosen per app (organisation default, maintainer 2026-11-10):**
    - **Plux-hosted:** Plux keeps the data in its own store, scoped to what device storage would
      hold, with retention limits and data-subject requests (`dsr`).
  - **Isolation is per app, also on a shared short code:** the data belongs to the app the user
    is in, not to the short code, which only routes (D-10). Each app has its own encryption key
    and its own partition, and no app reads another app's data, even when several apps of one
    organisation share a code. The mode is set per app; an organisation can set it once for all
    apps on a shared code.
    - **Institution-hosted:** Plux holds none of it. Storage actions call a **storage delegate**
      the institution configures (an endpoint or a Plux Function implementing a small contract:
      get, put, delete and query per user and collection), through the egress proxy over mTLS.
  - **No silent default:** a plugin whose USSD flows use storage actions cannot be enabled on
    USSD until one mode is configured; the Blocked-items report (D-3) names the missing
    configuration and links to it. Switching mode is an audited configuration change.
  - **Cross-channel (D-21):** data is keyed by the **end-user identity** from the auth delegate
    (D-12), in whichever mode is configured, so a user continues on USSD what they began in the
    app and back. Data kept on the phone stays on the phone.
  - Live session state (above) is transient and always held by Plux, in either mode.
- **Spec §3:** the "hosting end-user data (a backend-as-a-service)" non-goal gains a written
  exception for the Plux-hosted mode, limited to the device-storage equivalent (Q-2, answered).

### D-10. Routing short codes to apps (dynamic)

- **Decision:**
  - Each organisation registers the short codes it leases.
  - A short code opens **one app**, or shows an **organisation-level menu of several apps** when
    a company runs more than one app on the same code.
  - Inside an app, **sub-codes deep-link** to a plugin or a page (e.g. `*878*2*1#`), always
    resolved through the app.
  - **A root code never opens a bare plugin**: the app supplies identity, language, settings and
    the shell.
  - The routing table is **server configuration**, not part of the signed release: changed at
    runtime without a redeploy, audited, and validated (no root code may point at a plugin).

### D-11. Releases, scope and language

- **Releases (10A):** one signed release serves both channels; rollouts, experiments and
  rollbacks apply to both; USSD has its own **pause switch** per app.
- **Scope (11A):** USSD is **opt-in per plugin**; an enabled plugin brings all its pages, with
  optional **per-page exclusion**.
- **Language (12A):** remembered per user, chosen at the first dial when the app has several
  languages, changeable from a menu entry; the UCS-2 screen budget applies automatically.

### D-12. Identity, PIN and sensitive actions

- **Identity (4A, refined by 17A):** the caller is identified by the phone number the operator
  passes. Sensitive actions (payments, personal data) require a PIN.
- **PIN verification (17A):** the **institution verifies the PIN**. Plux forwards it once, over
  mTLS with a signed request, to an **auth delegate** configured per app (an endpoint or a Plux
  Function). The institution returns a **short-lived token bound to a per-session key** held by
  `plux-ussd` in KMS or an HSM — the USSD equivalent of DPoP. **Plux never stores a PIN.**
  `HST-010` stays true (Plux implements no end-user login); its wording is extended to the
  server-side delegate. This supersedes 4A's "hashed PIN on the server".
- **Baseline controls (always on):**
  - The PIN is a sensitive field (`SCH-012`, `SEC-092`): in memory or encrypted session state
    only, never logged, forwarded at once.
  - Gateway calls are authenticated: mTLS where supported, signed callbacks, IP allowlist.
  - **SIM-swap check** before sensitive actions where the operator supports it (CAMARA SIM Swap
    is the emerging standard; no evidence yet that Ethio Telecom or Safaricom Ethiopia offer
    it), with a configurable block window after a SIM change.
  - Lockout and rate limits per number; the per-app risk limits of D-13.
- **Optional (per app):** a high-risk USSD action can require **approval in the user's
  registered phone app at AL2**.
- **Why:** USSD has no end-to-end encryption; the PIN is readable at the operator and the
  aggregator. The design assumes exposure and relies on the institution's own PIN
  verification, binding, SIM-swap checks and limits.

### D-13. Sensitive actions allowed with limits (13A)

- **Decision:** sensitive actions work on USSD only when the app opts in; the app sets risk
  limits (e.g. maximum amount, daily count); Plux enforces PIN lockout, SIM-swap checks where
  available, and a full audit trail.

### D-14. Assurance levels and the `maximum` profile (18, maintainer's rule)

- **Decision:** financial institutions must be able to run the highest security profile **and**
  USSD. USSD under `maximum` (and any route guard requiring an assurance level a USSD session
  does not reach) is an **explicit, audited exception** per app, optionally narrowed per route
  guard:
  - off by default;
  - enabling it requires approval with WebAuthn step-up (`SEC-103`) and produces a distinct
    warning (the rule for unsafe settings);
  - effective only with the compensating controls of D-12 and D-13.
- Without the exception, a guarded page shows the "available in the app" screen.

### D-15. Slow API calls (15, configurable)

- **Decision:** both behaviours, chosen by **configuration at app level** (at organisation level
  when several apps share a short code):
  - **SMS result:** within the time budget the result shows normally; otherwise the user sees
    "we will send the result by SMS" and the call completes in the background.
  - **Error:** after the budget, "try again later".
- **Time budget:** a registry limit, default about **5 s**. Operators drop a screen quickly:
  Airtel waits 15 s for the application, Safaricom ends a session after 20–30 s of inactivity,
  and whole sessions are capped at 180–300 s.
- **Industry practice:** transactions always end with an SMS receipt, and an interrupted
  critical flow sends an SMS.

### D-16. Edition (16A)

- **Decision:** open source: the channel, the simulator and all three adapters, unless an
  operator contract forbids publishing its specification (then that adapter is distributed per
  the contract).

### D-17. Studio (19A, full support in this phase)

- **Decision:** a USSD phone simulator beside the canvas; the Blocked-items report with one-click
  fixes; a USSD settings panel per element; screens for the routing table, short codes,
  adapters, USSD exceptions and enrolled USSD runtimes; the USSD home-menu editor (D-24); the
  SMS action on the canvas (D-25). Built with shadcn/ui as Studio ships it (`studio/AGENTS.md`,
  ADR-0103; corrected 2026-11-11, the earlier "Porcelain" wording predates ADR-0103).

### D-18. USSD push (20A, with deep caution)

- **Decision:** network-initiated USSD sessions, every sub-feature **off by default** and
  switched on separately:
  - confirmations of the user's own recent action (e.g. a payment they just started);
  - server-initiated prompts (e.g. a merchant payment request or an approval);
  - a PIN request inside a push, allowed only within N minutes of a user-initiated action
    (anti-phishing).
- **Shared controls:** consent per number, rate limits, quiet hours, the app's registered name on
  every push screen, approval to enable, full audit trail. Adapters declare whether they support
  push.

### D-19. Encrypted (confidential) bundles on USSD (22A)

- **Decision:** `plux-ussd` **enrols like a device**, so confidential plugins work on USSD. The
  required changes, none to be overlooked:
  1. A **"server runtime" identity type**, separate from devices: enrolled by an operator,
     approved with WebAuthn step-up, identified by mTLS and workload identity instead of
     attestation.
  2. Its key stays in **KMS or an HSM** through the `signing/` boundary (L-1); a new layering
     rule L-8 sets what `plux-ussd` may touch.
  3. The release pipeline **wraps each confidential bundle's content key** for enrolled
     runtimes, as it does for devices.
  4. It **syncs like a device**: control document, kill switches, rollouts, experiments;
     targeting by phone-number cohort.
  5. **Verify before load** (`SEC-052`): signature and hash checks in Go before anything is
     parsed.
  6. Decrypted bundles stay **in memory only**, never on disk (`SEC-053`).
  7. **Revocation, key rotation and per-app enablement** work like device revocation.
  8. Every key unwrap is **audited**.
  9. Studio and admin list enrolled USSD runtimes and their status.
  10. Tests: confidential round trip, revocation, kill switch, tampered bundle, key rotation.
- **Follow-up:** an ADR (key handling, `AGENTS.md` §7).

### D-20. Scale (23A)

- **Decision:** at least **1,000 key presses per second per `plux-ussd` instance** (`MUST`, the
  floor), aiming higher (`SHOULD` ≥ 2,000), with horizontal scaling; Plux's own time ≤ 150 ms
  p95, excluding the institution's and third-party APIs.
- **Verification:** a `plux-ussd` node is added to the §30.1 reference deployment; CI runs a
  scaled-down regression benchmark; the real number is measured on the reference deployment
  and the requirement stays `WIP` until then (as other performance targets).

### D-21. One user across both channels (21A)

- See D-9: server-side data follows the end-user identity across channels.

### D-22. Decided by the lead (maintainer may veto; accepted 2026-11-10)

- Functions placed on the device (`FN-055`) run server-side in `fnrunner` on USSD.
- A session stays on the release it started with.
- Release approvals show the USSD preview and its diff.
- Live debugging: USSD sessions from allowlisted test numbers stream to the Dev tools (P10).
- `openUrl`, `share` and `copyToClipboard` map to an SMS (D-3).
- The data-subject-request and retention rules apply to the server-side store (D-9).

### D-23. Text-channel architecture (26A)

- **Decision:** the classification, the executor and the session engine are **channel-neutral**
  ("text channel"); USSD is the first channel. SMS bots, WhatsApp or IVR can be added later
  without redesign. About 10–15 % more design work now.
- **Note:** the classification of D-3 is therefore stored per channel (USSD now), not as a
  USSD-only flag.

### D-24. The USSD home menu (24A, extended)

- **Decision:**
  - An app's USSD root menu is **generated from its enabled plugins**, and can be reordered and
    relabelled.
  - Pages can be marked **USSD-only** (e.g. a short menu with no app equivalent).
  - **Several apps on one short code:** the organisation-level menu (D-10) is generated from
    the apps on that code, editable in the same way, with the per-app settings of D-15 set at
    organisation level for that code.

### D-25. SMS as a general action (25A, extended)

- **Decision:** a `sendSms` action usable from **both channels**, reusing the adapters' SMS
  capability, with templates, consent, cost and abuse limits (registry limits, audit). It is a
  **prominent action on the Studio canvas**: its own place in the action palette and a quick
  add on success and failure branches.

### D-26. SMS sending

- **Decision:** SMS (receipts, D-15 results, D-3 links, `sendSms`) goes through the gateway
  adapters' SMS capability; each adapter declares it.

### D-27. Exit criterion (27A, no contract assumed)

- **Decision:** assume **no operator contract**. The phase is done when:
  - a sample bank flow runs end to end on the simulator and through the **Africa's Talking
    sandbox**, with the conformance suite green;
  - a **detailed technical onboarding document** for Ethio Telecom and Safaricom Ethiopia is
    part of the exit: what to request from the operator (gateway protocol and specification,
    sandbox, test numbers, IP ranges, certificates, timeouts, character limits, push and SMS
    support, SIM-swap data), how to fill in and test each adapter, the acceptance tests to run,
    and the steps that move the adapter requirements from `WIP` to `DONE`.

### D-28. Decided by the lead after the full re-analysis (2026-11-11; maintainer may veto)

The spec was swept section by section for USSD impact. Each item below becomes requirements.

1. **User context and consent:** the auth delegate (D-12) also returns the user context the host
   would set (`HST-001` `setUserContext`, PXL `user.*`). Consent (`SEC-161`) is asked at the
   first dial when the app requires it, stored per user, and respected by telemetry.
2. **API calls:** USSD sessions call data sources only through the Plux gateway
   (`SEC-030`–`SEC-032`), with the session-bound token; direct routes do not exist on USSD.
3. **Telemetry:** the role emits the same event types as the runtime (`ANL-001`), tagged with the
   channel; sensitive fields are stripped (`ANL-003`); the device battery and data budgets
   (`ANL-002`) do not apply.
4. **Rollouts and experiments:** the assignment unit is the end-user id, or a hash of the phone
   number before sign-in.
5. **Short-code ownership:** registering a short code needs proof (a verification dial through
   the adapter, or the provider's API confirming the route) and installation-admin approval, so
   no organisation can claim another's code (also the basis for P15).
6. **Usage metering:** sessions, screens, push prompts and SMS are counted per app and
   organisation, for cost visibility (the P15 metering interfaces).
7. **Concurrency:** user-data records carry a version and writes use optimistic concurrency, in
   both storage modes (part of the storage-delegate contract). One USSD session per number and
   app: a new dial ends the old one.
8. **Permissions:** the role catalogue (P9) gains permissions for the USSD channel, short codes,
   adapters, exceptions and push; enabling an exception or push needs approval with step-up.
9. **Deployment:** the Helm chart and Compose gain the `ussd` role, with a gateway ingress
   separate from the API ingress.
10. **Bundle format:** USSD settings are additive document fields; the Flutter runtime ignores
    them; `required_features` gains USSD features read only by the `ussd` role.
11. **Encoding:** GSM-7 or UCS-2 is chosen per screen from its content; no transliteration.
12. **Shared installations:** all organisations share the role, isolated per app (keys,
    partitions, rate limits).
13. **Later phases:** P12 (AI) generates and respects USSD classes and settings; P13 classifies
    its payment actions (Q-5); P15 isolates short codes per tenant; P16 decides whether partner
    mini-apps appear in a host's USSD menu. Each gains a requirement naming USSD when written.

### D-29. USSD-only apps (Q-8)

- **Decision:** an app may have **only the USSD channel**: no Flutter project is generated
  (`GEN-001` gains the condition). The same pages can gain the Flutter channel later.

### D-30. From the app to USSD without a data connection (Q-9)

- **Decision:** configurable per app. When the phone has no data connection, the Flutter app
  offers to continue the current flow over USSD by dialling its deep-link sub-code (D-10). Where
  the platform does not let apps dial USSD (iOS), the code is shown for the user to dial.

### D-31. SMPP implementation (Q-10)

- **Decision:** an **own SMPP client on the Go standard library**, covering only what USSD needs
  (bind, `enquire_link`, `deliver_sm`/`submit_sm`/`data_sm`, the USSD TLVs), like the project's
  own Valkey RESP client. No new dependency, no ADR for a library; fuzz and conformance tests.

### D-32. Transaction signing on USSD (Q-11)

- **Decision:** `signTransaction` maps to **USSD strong customer authentication**: possession of
  the SIM (the operator-passed number, with the SIM-swap check of D-12) plus the PIN verified by
  the institution. The amount and payee are shown on screen and sent with the PIN check
  (dynamic linking); the institution receives the server's verified assertion, marked as USSD,
  and applies its own policy (`SEC-032`). Available only under the D-14 exception. An ADR
  records it (security, `AGENTS.md` §7).

---

## 3. Questions found in the re-analysis

- **Q-1. Separate service or a role? — answered (maintainer 2026-11-11):** a `ussd` role of
  `plux-server` (D-7).
- **Q-2. End-user data on the server versus the §3 non-goal — answered (2026-11-10).** Both
  options, as configuration per app: Plux-hosted (a narrow, written §3 exception) or
  institution-hosted through a storage delegate; nothing is enabled until one is chosen. See
  D-9.
- **Q-3. Error codes — decided by the lead (2026-11-10).** Codes stay four digits
  (`^PLX-[0-9]{4}$`). USSD takes unused hundred-blocks inside the existing areas: text-channel
  rendering and sessions `PLX-4500`–`4699`; user-data store and storage delegate
  `PLX-5600`–`5699`; identity, PIN delegate, SIM swap, exceptions and push `PLX-6300`–`6399`;
  short codes, routing and adapters `PLX-3200`–`3299`; Studio and CLI `PLX-9400`–`9499`
  (all unused on 2026-11-10).
- **Q-4. Protocols — answered (maintainer 2026-11-11):** all three adapters are implemented
  fully without a contract, over the known gateway interfaces (D-6); the SMPP choice is D-31.
- **Q-5. Payments — withdrawn (maintainer 2026-11-11).** Payments are not special: a payment
  flow is an ordinary plugin with inputs and outputs, and its pages and actions follow the same
  classification as everything else (D-3). When P13 defines `startPayment` and its provider
  adapters, P13 classifies them like any action (e.g. a provider that pushes a prompt to the
  payer's phone is Native on USSD; card entry is Blocked). The security step that *is* special,
  transaction signing, is D-32.
- **Q-6. Data protection — decided by the lead (2026-11-10).** Ethiopia's Personal Data
  Protection Proclamation No. 1321/2024 (in force 24 July 2024; the Ethiopian Communications
  Authority enforces it) requires personal data collected in Ethiopia to be stored on servers in
  Ethiopia, allows cross-border transfer only under conditions, and needs prior authorisation for
  sensitive data. Consequences: §31 maps it (article numbers checked against the gazette text
  before citing); the deployment guide says that Ethiopian installations run `plux-ussd`, Valkey
  and the Plux-hosted user-data store in the country; an installation declares its data-residency
  region, and Studio warns when an adapter, storage delegate or auth delegate endpoint is
  declared outside it. Sources: <https://cipit.strathmore.edu/?p=4868>,
  <https://digitalpolicyalert.org/event/24922-implemented-personal-data-protection-proclamation-proclamation-no-13212024-including-data-localisation-requirements>,
  <https://www.techhiveadvisory.africa/insights/review-of-ethiopias-data-protection-act>.
- **Q-7. PIN visibility — decided by the lead (2026-11-10).** USSD cannot mask input; a PIN
  shows on the handset screen as it is typed. The spec states it as a known channel limit, the
  risk table (§34) lists it, and Studio's USSD preview shows a notice on every PIN step.

- **Q-8 to Q-11 — answered (maintainer 2026-11-11):** the recommendations, recorded as D-29 to
  D-32.

---

## 4. Spec work list (nothing to be forgotten)

Each item becomes requirements with IDs, priorities, statuses and verification.

| Area | Content | Decisions |
|---|---|---|
| §5 | P17 row, scope, exit criterion, summary table; schedule note | D-1, D-27 |
| New section "Text channels and USSD" | channel model, classification, rendering, screen splitting, routing, sessions, adapters, push, SMS | D-2–D-6, D-10, D-11, D-18, D-23–D-26 |
| §3 | the written exception to the end-user-data non-goal (Plux-hosted mode only) | D-9 |
| §14 (storage) | the two user-data store modes, the storage-delegate contract, the enable check | D-9 |
| §6 | component (`ussd` role), layering rule L-8 | D-7, Q-1, D-19 |
| §7 | per-element USSD settings, USSD-only pages, home menu | D-3, D-24 |
| §8 | USSD class per widget descriptor, coverage table | D-3 |
| §9–§10 | server runtime receives bundles, verify before load, confidential bundles, version pinning | D-8, D-19, D-22 |
| §11 | routing table, short codes, adapter configuration, APIs | D-6, D-10 |
| §13 | `HST-010` extended to the server-side auth delegate | D-12 |
| §14 | USSD class per action, Go action executor, server-side store, `sendSms` | D-3, D-8, D-9, D-25 |
| §15 | identity, PIN delegate, session-bound tokens, SIM swap, exceptions under `maximum`, push controls, gateway authentication, security settings | D-12–D-14, D-18 |
| §16 | Functions on USSD, device placement | D-22 |
| §17 | language choice, GSM-7/UCS-2 budgets | D-5, D-11 |
| §18 | rollouts, experiments, kill switch and USSD pause | D-11 |
| §19 | session funnels, drop-off, USSD metrics and logs | D-17 |
| §20 | simulator, live debugging of test numbers | D-22 |
| §21 | Studio screens of D-17 | D-17, D-24, D-25 |
| §22 | CLI and simulator | D-6 |
| §24 | payments on USSD (Q-5) | Q-5 |
| §28 | conformance vectors (actions, USSD screens), adapter contract tests | D-8 |
| §30 | reference deployment, throughput and latency targets, limits registry entries (screen budget, time budget, session length, menu depth, push and SMS limits) | D-5, D-15, D-20 |
| §27, §31 | Ethiopian data-residency guidance, residency region and warnings; compliance mapping | Q-6 |
| §34–§35 | editions, risks (operator contracts, PIN exposure), glossary | D-16, D-12 |
| Appendices | error codes (Q-3), action and widget tables, security settings | Q-3 |
| ADRs | text channels and USSD; server runtime and confidential bundles; PIN delegate and session-bound tokens; USSD under `maximum`; USSD push | — |
| Tooling | `spec-lint` P17; generated registry and coverage table | D-1, D-3 |

---

## 5. Sources (research, 2026-11-03 to 2026-11-10)

- Safaricom Ethiopia M-Pesa SDK: <https://github.com/Safaricom-Ethiopia-PLC/mpesa-go-sdk>
- Africa's Talking coverage: <https://docs.page/nexlabstudio/starhash-docs/providers/africas-talking>,
  <https://helloduty.com/blogs/countries-supported-by-africas-talking-ussd-api>
- Session timeouts: <https://help.africastalking.com/en/articles/1284071-what-is-the-duration-of-a-ussd-session-for-kenyan-telcos>,
  <https://hermankapange.com/blog/ussd-in-2026/>
- Menu design and SMS confirmation practice: <https://arkesel.com/ussd-menu-design-best-practices/>,
  <https://arkesel.com/ussd-financial-services-africa-mobile-money/>,
  <https://connectmedia.co.ke/ussd-menu-design-best-practices/>, <https://psi.atlassian.net/wiki/x/AQADC>
- Element fallback precedent: <https://adaptivecards.io/blog/2019/Announcing-1.2/>
- USSD and PIN security: <https://www.cise.ufl.edu/~butler/pubs/sec15a.pdf>,
  <https://www.simpalabs.com/blog/ussd-banking-security-penetration-testing/>,
  <https://github.com/iwtglobal/mobile-money-sim-swap-protection>,
  <https://arkesel.com/ussd-security-mobile-transaction-protection/>
- SIM swap and number verification APIs: <https://opengateway.telefonica.com/en/apis/sim-swap>
