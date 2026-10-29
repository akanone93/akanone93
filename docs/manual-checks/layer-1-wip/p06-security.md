<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Layer 1 — P6: security hardening

The 23 P6 rows still `WIP`. CI proves the verifiers with recorded vectors and the development
provider, which never rises above `AL0`. What only real phones, real Google and Apple evidence
and a person can prove is here ([device-trust.md](../../security/device-trust.md) §3.3).
Check numbers 600–699 belong to this file. Record every device result in
`docs/security/evidence/<date>-<device>.md` (model, OS, build, results, attestation objects
with identifiers removed), as [real-device-checklist.md](../../security/real-device-checklist.md) says.

## Resources this file adds

| ID | What | Made by |
|---|---|---|
| R-L1-IPHONE | An iPhone on iOS 17 or newer, in an Apple Developer team with App Attest enabled, Xcode on a Mac | You, once |
| R-L1-ATTEST | The stack of S-2 reachable from both phones over HTTPS, with `attestation.developmentProvider: false` for the test environment and an `attestation.apps` entry for Plux Bank and Plux Express (package, signing-certificate digest, Play Integrity keys, iOS App ID; `docs/guides/device-trust.md` §3, a copy of `deploy/compose/plux-server.yaml`). The fixtures `schema/testdata/documents/plux_bank` and `plux_express` are published to it (`plux publish --release`) and `test/refapi` runs beside it | L1-SEC-603 |
| R-L1-BANK | Release builds of Plux Bank and Plux Express pointed at R-L1-ATTEST (defines in `apps/starter/README.md`, `apps/plux_bank/README.md`): on R-L1-MID from a Play Console internal test track linked to a Google Cloud project with the Play Integrity API; on R-L1-IPHONE signed by the App Attest team | L1-SEC-603 |
| R-L1-DEVLIST | `curl -sS --cacert deploy/compose/.secrets/ca/ca.crt https://localhost:8080/plux.v1.DeviceService/ListDevices -H "Authorization: Bearer $PLUX_DEV_TOKEN" -H "X-Plux-Organization: $PLUX_DEV_ORGANIZATION" -H 'Content-Type: application/json' -d '{"appId":"…","environmentId":"…"}'` (body as in `docs/runbooks/device-revocation.md`; variables from `dev.env`, R-TOKEN, R-ORG); it shows each device's `assuranceLevel`, `keyStorage`, attestation provider and `revokedAt` | L1-SEC-603 |
| R-L1-ROOTED | A second Android phone, rooted with Magisk, with `frida-server` and LSPosed installed, USB debugging on | You, once |
| R-L1-PROXY | `mitmproxy` on your machine, its CA installed on both phones, the phones' Wi-Fi proxy pointed at it | L1-SEC-613 |

### L1-SEC-601 — The threat model covers every phase and is reviewed (SEC-000)

- **Needs:** nothing
- **Do:**
  1. Open `docs/security/threat-model.md`; list its `## P…` sections.
  2. For P6 to P11, check that every mitigation row names a requirement ID or a test.
- **Expect:** one section per phase from P6. Today P10 (dev sessions, Dev app, background sync)
  has no section: a finding.
- **Why:** the STRIDE model is reviewed at the end of every phase from P6 (SEC-000).
- **Moves to DONE when:** the P10 section exists and you record your review of P6–P11 (date and
  name in the file); then the row is kept current by each phase's exit.
- **Result:** ☐ pass ☐ fail — notes:

### L1-SEC-602 — Release builds carry no dev tooling (SEC-074)

- **Needs:** S-1 with the Android SDK (and a Mac with Xcode for iOS)
- **Do:** `make release-inspect PLATFORM=android`; on the Mac `make release-inspect PLATFORM=ios`.
- **Expect:** both end without an error: no devtools code, dev-only logging, source maps or debug files.
- **Why:** dev tooling in a shipped app is an attack surface (SEC-074, DEV-050).
- **Look at:** the same step in CI's `size-android` and `size-ios` jobs (`.github/workflows/ci.yml`).
- **Moves to DONE when:** both CI jobs ran `release-inspect` green on `main`; this is closed by
  CI, the local run is your look at it.
- **Result:** ☐ pass ☐ fail — notes:

### L1-SEC-603 — The DPoP key lives in secure hardware (SEC-001)

- **Needs:** R-L1-MID, R-L1-IPHONE; make R-L1-ATTEST, R-L1-BANK and R-L1-DEVLIST
- **Do:**
  1. Install Plux Bank on both phones and open it once.
  2. Run R-L1-DEVLIST for the Bank app's environment.
- **Expect:** two devices; the Android one with `keyStorage` `KEY_STORAGE_STRONGBOX` (or `TEE`
  where the phone has no StrongBox), the iPhone with `KEY_STORAGE_SECURE_ENCLAVE`; neither `SOFTWARE`.
- **Why:** the private key must never exist outside secure hardware (SEC-001).
- **Moves to DONE when:** this and L1-SEC-604 (which decides StrongBox against TEE by speed) pass and are in the evidence file.
- **Result:** ☐ pass ☐ fail — notes:

### L1-SEC-604 — DPoP proof creation within 15 ms on the phones (NFR-012)

- **Needs:** R-L1-MID, R-L1-IPHONE
- **Do:** in `apps/starter`, for each phone:
  `flutter test integration_test/dpop_bench_test.dart -d <device> --dart-define=PLUX_BENCH_ASSERT=1`
- **Expect:** the test passes; its JSON shows p95 ≤ 15 ms and the `key_storage` used.
- **Why:** every device request carries a proof; the budget is for the phone (NFR-012, SEC-021).
- **Moves to DONE when:** both rows of the device table in `docs/benchmarks/p6-security.md` are filled and within budget.
- **Result:** ☐ pass ☐ fail — notes:

### L1-SEC-605 — A real Key Attestation chain verifies (SEC-002)

- **Needs:** R-L1-MID with R-L1-BANK, R-L1-ROOTED, R-L1-DEVLIST
- **Do:**
  1. On R-L1-MID: R-L1-DEVLIST after L1-SEC-603.
  2. Install the same Bank build on R-L1-ROOTED (unlocked bootloader) and open it.
- **Expect:** the reference phone is at `AL2` or `AL3`; the rooted phone is refused or held
  below `AL2` (unverified boot or unlocked bootloader in the chain).
- **Why:** only a real chain to Google's roots proves the key is hardware-backed (SEC-002).
- **Look at:** the server log of the registration; keep the chain (identifiers removed) for the CI vectors.
- **Moves to DONE when:** both outcomes are in the evidence file.
- **Result:** ☐ pass ☐ fail — notes:

### L1-SEC-606 — A real Play Integrity verdict verifies (SEC-003)

- **Needs:** the run of L1-SEC-605
- **Do:** in the server log of the reference phone's registration and of its next token refresh,
  read the Play Integrity result; then install the Bank APK by `adb install` (not from Play) on R-L1-ROOTED.
- **Expect:** the reference phone: `PLAY_RECOGNIZED`, the device verdict the profile needs, the
  request hash matching; the side-loaded build on the rooted phone: not recognised, `AL0`.
- **Why:** the token binds the server's challenge and the device's keys (SEC-003, ADR-0012).
- **Moves to DONE when:** both outcomes and the recorded token (identifiers removed) are in the evidence file.
- **Result:** ☐ pass ☐ fail — notes:

### L1-SEC-607 — App Attest registration and assertions on the iPhone (SEC-004)

- **Needs:** R-L1-IPHONE with R-L1-BANK, R-L1-DEVLIST
- **Do:**
  1. R-L1-DEVLIST after L1-SEC-603: read the iPhone's level and provider.
  2. In Plux Bank, sign in and make two transfers (sensitive requests).
- **Expect:** the iPhone is `AL2` with the App Attest provider; both transfers succeed; the server
  log shows each assertion's counter higher than the last.
- **Why:** App Attest proves the genuine app on a genuine iPhone (SEC-004).
- **Moves to DONE when:** this and the recorded attestation object are in the evidence file.
- **Result:** ☐ pass ☐ fail — notes:

### L1-SEC-608 — Evidence maps to the right assurance level (SEC-007)

- **Needs:** the runs of L1-SEC-605 to L1-SEC-607
- **Do:**
  1. Compare the levels R-L1-DEVLIST shows with the table in `docs/guides/device-trust.md` §4.
  2. Turn on Apple's fraud-metric receipt (`attestation.apple.fraudReceipt`, ADR-0065, the App
     Store Connect key imported under `apple-appattest-receipt`); reopen Bank on the iPhone.
- **Expect:** each device has the level its evidence and profile give; with the receipt on, the
  iPhone reaches `AL3` when its risk metric is at or below `riskMetricMaxAL3`.
- **Why:** pages, functions and the gateway gate on the level (SEC-007).
- **Look at:** the `al` claim of the access token; the device's `attestation.riskMetric`.
- **Moves to DONE when:** the Android and iPhone levels, with and without the receipt, are in the evidence file.
- **Result:** ☐ pass ☐ fail — notes:

### L1-SEC-609 — Re-attestation and revocation (SEC-006)

- **Needs:** R-L1-MID with R-L1-BANK, R-L1-DEVLIST
- **Do:**
  1. Install a Bank build with a higher build number; open it.
  2. Revoke the phone: the call of R-L1-DEVLIST on `plux.v1.DeviceService/RevokeDevice` with
     `{"deviceId":"…","reason":"manual check"}`; use the app.
  3. Leave the app 24 hours; open it again.
- **Expect:** step 1 re-attests (a new `attestation.verifiedAt`); step 2 fails with `PLX-6006`,
  the app registers again as a new device; step 3 re-attests before the next refresh.
- **Why:** trust decays and an operator can cut a device off (SEC-006).
- **Look at:** the audit entry of the revocation (`ListAuditEntries`).
- **Moves to DONE when:** the three outcomes are in the evidence file.
- **Result:** ☐ pass ☐ fail — notes:

### L1-SEC-610 — Strong customer authentication signs a transfer (SEC-027)

- **Needs:** R-L1-MID and R-L1-IPHONE with R-L1-BANK, biometrics enrolled
- **Do:** in Plux Bank, sign in, New transfer, confirm the sheet with a fingerprint or Face ID;
  repeat and cancel the biometric prompt; then enrol a new fingerprint or face and try again.
- **Expect:** the first transfer succeeds; cancelling sends nothing; after re-enrolment the old
  key no longer signs (iOS `.biometryCurrentSet`), so the transfer is not confirmed.
- **Why:** one signature proves possession and inherence (SEC-027).
- **Look at:** the reference API's log of the signed payload (amount, currency, payee).
- **Moves to DONE when:** both platforms' outcomes are in the evidence file.
- **Result:** ☐ pass ☐ fail — notes:

### L1-SEC-611 — Confidential bundles open only on the registered phone (SEC-053)

- **Needs:** R-L1-MID, R-L1-IPHONE, R-L1-ATTEST
- **Do:**
  1. `plux security config set --env <bank env> --profile strict --expected-version <n> --set confidentialBundles=required`
     (keep the overrides `plux security config diff` shows); publish a Bank release.
  2. Open Bank on both phones; then on R-L1-ROOTED (below `AL2`).
- **Expect:** both phones show the new release; the rooted phone gets no content key and stays on its old release.
- **Why:** a sealed release is readable only on devices at the required level (SEC-053, ADR-0055).
- **Look at:** `docs/guides/confidential-bundles.md`; the release is delivered without a delta.
- **Moves to DONE when:** the three outcomes are in the evidence file.
- **Result:** ☐ pass ☐ fail — notes:

### L1-SEC-612 — Decrypting a confidential bundle within 10 ms per MiB (NFR-013)

- **Needs:** R-L1-MID, R-L1-IPHONE
- **Do:**
  1. `make bench-confidential` for the host upper bound (pure-Dart cipher).
  2. Run the same benchmark on each phone with the platform cipher
     (`packages/plux_flutter/lib/src/security/platform_aead.dart`). There is no device harness
     yet: ask Claude to wrap `test/security/confidential_bench_test.dart` as an integration test
     like `apps/starter/integration_test/dpop_bench_test.dart`.
- **Expect:** p95 ≤ 10 ms per MiB; encrypted local stores within 10% of the plain ones (L1-RUN-504 encrypted and plain).
- **Why:** sealing must not slow the first open of a release (NFR-013, SEC-053, SEC-073).
- **Moves to DONE when:** the phones' numbers are in `docs/benchmarks/p6-security.md` and within budget.
- **Result:** ☐ pass ☐ fail — notes:

### L1-SEC-613 — Pinning refuses an intercepting proxy (SEC-041)

- **Needs:** R-L1-MID and R-L1-IPHONE with R-L1-BANK; make R-L1-PROXY
- **Do:** with the phones' traffic through `mitmproxy`, open Bank and pull to refresh; open the
  dashboard (its SSE banner).
- **Expect:** no request reaches the server through the proxy; sync fails closed with `PLX-6020`
  and the app keeps its current release; removing the proxy brings sync back.
- **Why:** a user-installed CA must not let anyone read or change Plux traffic (SEC-041).
- **Look at:** the `pin_failure` event (`docs/security/privacy.md`); `mitmproxy` shows only failed handshakes.
- **Moves to DONE when:** both platforms' outcomes are in the evidence file.
- **Result:** ☐ pass ☐ fail — notes:

### L1-SEC-614 — Optional pins for a customer API domain (SEC-042)

- **Needs:** L1-SEC-613's set-up; a test app with a `direct` REST source to a domain you control
- **Do:** add `networkPins` for that domain to the app document (two SPKI pins, `schema/json/app.schema.json`),
  publish, open the page through `mitmproxy`; then remove the pins and repeat.
- **Expect:** with pins the call fails with `PLX-6020`; without them it goes through the proxy.
- **Why:** customers may pin their own APIs (SEC-042); WebSocket streams to customer domains are
  not pinned (threat model, P6 residual 8).
- **Moves to DONE when:** the outcome is in the evidence file and you accept residual 8 for this `SHOULD`.
- **Result:** ☐ pass ☐ fail — notes:

### L1-SEC-615 — RASP detects a hostile phone (SEC-070)

- **Needs:** R-L1-ROOTED, R-L1-MID, R-L1-IPHONE (a jailbroken one if you have it), R-L1-BANK
- **Do:** on R-L1-ROOTED open Bank: rooted; then with `frida-server` running; then with a debugger
  attached (`flutter attach` on a debug build). On R-L1-MID: install a re-signed APK
  (`apksigner` with another key); side-load with `adb install`; cast the screen; enable a
  third-party accessibility service; run an app that draws over Bank.
- **Expect:** each case is reported with its kind (`root`, `hooking`, `debugger`, `repackaging`,
  `untrusted_installer`, `screen_capture`, `accessibility`, `overlay`); the clean phone reports nothing.
- **Why:** the detections of `packages/plux_security/README.md` must fire on real devices (SEC-070).
- **Look at:** the `rasp_detection` events at the server.
- **Moves to DONE when:** each kind's outcome is in the evidence file.
- **Result:** ☐ pass ☐ fail — notes:

### L1-SEC-616 — A detection gets its response and lowers the level (SEC-071)

- **Needs:** the cases of L1-SEC-615, R-L1-DEVLIST
- **Do:** repeat two cases (root, screen capture) under `standard`, `strict` and `maximum`
  (`plux security config set … --profile …`); after each, run R-L1-DEVLIST.
- **Expect:** the responses of the table in `docs/guides/security.md` §2 (`report`, `warn`,
  `degrade`, `block`); Bank blocks root and hooking whatever the profile; the device's level is
  capped at the server (`AL1` for a modified environment) until a clean re-attestation.
- **Why:** a detection is a signal the server weighs, not only a screen (SEC-071).
- **Moves to DONE when:** the responses and levels are in the evidence file.
- **Result:** ☐ pass ☐ fail — notes:

### L1-SEC-617 — Nothing sensitive in plain storage (SEC-072)

- **Needs:** R-L1-ROOTED with Bank signed in
- **Do:** `adb shell su -c 'ls -R /data/data/<bank application id>'`; read `shared_prefs/*.xml` and
  any plain file; `grep` them for the access token's prefix and the user's name.
- **Expect:** no token, key, PIN or user data in plain text; only sealed records.
- **Why:** tokens and keys belong in Keystore/Keychain-protected storage (SEC-072).
- **Moves to DONE when:** this passes and the iOS equivalent (an app-container backup of the iPhone) is in the evidence file.
- **Result:** ☐ pass ☐ fail — notes:

### L1-SEC-618 — Secure pages block capture and blur in the app switcher (SEC-091)

- **Needs:** R-L1-MID and R-L1-IPHONE with R-L1-BANK
- **Do:** on the `login` and `transfer` pages take a screenshot, start a screen recording, open the app switcher.
- **Expect:** Android: the screenshot is refused or black, the recording black. iOS: the recording
  shows a covering view; the app switcher shows a blurred page on both.
- **Why:** secure pages must not leak through capture (SEC-091; iOS cannot block a screenshot, threat model).
- **Moves to DONE when:** both platforms' outcomes are in the evidence file.
- **Result:** ☐ pass ☐ fail — notes:

### L1-SEC-619 — Touches under an overlay are filtered (SEC-093)

- **Needs:** R-L1-MID with R-L1-BANK; an app with "Display over other apps" granted (a screen-filter app)
- **Do:** turn the overlay on, open `transfer`, tap Confirm.
- **Expect:** the tap is ignored while the overlay covers the page; an `overlay` detection is reported; with the overlay off it works.
- **Why:** tapjacking tricks a user into confirming (SEC-093).
- **Moves to DONE when:** the outcome is in the evidence file.
- **Result:** ☐ pass ☐ fail — notes:

### L1-SEC-620 — Gateway overhead and request verification (NFR-025)

- **Needs:** S-1
- **Do:** `cd backend && go test -run '^$' -bench 'BenchmarkGatewayOverhead|BenchmarkDeviceVerification' ./internal/gateway`
- **Expect:** `added-p95-ms` ≤ 5 and the verification p95 ≤ 1 ms.
- **Why:** the gateway sits on every device call (NFR-025, SEC-031, SEC-022).
- **Moves to DONE when:** the same budget holds at 1,000 requests per second per `api` replica.
  No harness runs that load through the gateway (`make load-test` drives sync, not the gateway):
  ask for one, or accept the benchmark as the evidence (your decision).
- **Result:** ☐ pass ☐ fail — notes:

### L1-SEC-621 — No personal data leaves the phone without consent (SEC-160)

- **Needs:** R-L1-MID with R-L1-BANK, R-L1-ATTEST
- **Do:** open Bank without granting analytics consent and use it for a few minutes; then read
  that device's rows of the `telemetry_events` table in the server's database (`psql`); compare
  with `docs/security/privacy.md` (processing activities and retention).
- **Expect:** only `necessary` events; a random device ID; no hardware identifier, locale or personal data.
- **Why:** data minimisation and pseudonymous IDs by design (SEC-160).
- **Moves to DONE when:** the MASVS-PRIVACY real-device column of `docs/security/masvs.md` is filled
  for a release (with L1-SEC-622); the privacy notice of a host app is the host's own lawyer's work.
- **Result:** ☐ pass ☐ fail — notes:

### L1-SEC-622 — MASTG results recorded for a release (SEC-190)

- **Needs:** the results of L1-SEC-603 to L1-SEC-621
- **Do:** for each control in `docs/security/masvs.md`, run its MASTG test cases on the emulator
  where possible and on the phones where not; fill the "MASTG results per release" table.
- **Expect:** every control `pass`, or `fail` with an issue, or `not applicable` with a reason.
- **Why:** MASVS L2 + R is the claim of the `maximum` profile (SEC-190).
- **Moves to DONE when:** the first release row of that table is filled, real-device column included.
- **Result:** ☐ pass ☐ fail — notes:

### L1-SEC-623 — The security test suite is complete (QA-008)

- **Needs:** R-STACK
- **Do:** `make zap-api`, then `make zap-studio`; read `build/zap`.
- **Expect:** both scans end without an alert outside `test/zap/api-rules.tsv` and `test/zap/studio-rules.tsv`.
- **Why:** QA-008 names ZAP scans, the DPoP and attestation negative suite, tampering and rollback,
  the authorisation matrix and MASTG.
- **Moves to DONE when:** the `zap-studio` CI job ran green on the P11 pull request and L1-SEC-622 is done.
- **Result:** ☐ pass ☐ fail — notes:
