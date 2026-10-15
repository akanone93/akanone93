<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Real-Device Checklist

Layer 3 of [device-trust.md](device-trust.md#33-layer-3-the-maintainers-real-device-check-once-per-phase-and-before-a-release):
what only real devices prove. The maintainer runs it on one mid-tier Android phone (a Pixel 6a
or newer, with Google Play) and one iPhone (iOS 17 or newer), once per phase and before a
release, with release builds of the reference apps from a Play Console internal test track and
an Apple Developer team with App Attest enabled. Each item names its requirements, the steps,
the expected result and what stays `WIP` until it passes. Results go to
`docs/security/evidence/<date>-<device>.md`: device model, OS version, build, results, and the
recorded attestation objects (identifiers removed) that feed the CI verifier vectors.

CI cannot run these: its emulators and simulators register through the development
attestation provider, which never rises above `AL0` and makes no platform evidence
(`SEC-008`).

## P7 — Plux Functions

| # | Requirements | Steps | Expected | Until it passes |
|---|---|---|---|---|
| F1 | `FN-020`, `SEC-003`, `SEC-004`, `SEC-026`, `SEC-027` | Plux Bank, signed in as a `standard` user: New transfer, amount 12.50, Show fee; confirm the transaction sheet with the device key. Repeat with the user's tier set to `premium` at the reference backend. | The sheet names `bank.transferFee` and the input; after Confirm the page shows `Fee: 0.50 EUR` (`premium`: `Fee: 0.00 EUR`); the server's audit shows the invocation with App Attest (iOS) or Play Integrity (Android) evidence bound to it, and the user's tier claim. Cancel shows `Fee unavailable: declined` and no invocation. CI checks only that a development device is refused (`Fee unavailable: refused`); this is the success path (P7 plan §2.6 M12). | `FN-020`'s device evidence path stays verified by recorded vectors only |
| F2 | `FN-050`, `FN-051`, `FN-053`, `FN-056` | Plux Express in flight mode: Instalments, Calculate on this device, 3 and 6 months, then 999.99. Record the call time of the loan function on the device (100 runs, p50 and p95) with the release build's performance overlay. | The schedule appears offline; `PRINCIPAL_TOO_SMALL` for 999.99; a typical calculation within 20 ms p95. The numbers decide WAMR against `wasmi` (ADR-0011, Revision 2026-10-14; M8). | `FN-056` stays `WIP` |
| F3 | `FN-055` | Under the `maximum` profile, with `deviceFunctions` off, open Instalments. | The step fails with `PLX-7014` and the page shows its fallback; turning `deviceFunctions` on (D6 permission) brings the calculation back after the next sync. | — (verified on the host; the device confirms) |
| F4 | `FN-053` | A function that loops for ever, published to a test plugin, on the device. | The step fails after the device timeout (500 ms by default) and the UI stays responsive; the app's memory returns to its baseline. | — (verified on the host; the device confirms) |

## Carried from P6

These were recorded in the P6 hand-off notes and stay `WIP` until this check passes.

| Requirements | What the device proves |
|---|---|
| `SEC-002`, `SEC-003`, `SEC-004` | A real Key Attestation chain, Play Integrity verdict and App Attest attestation register the device at `AL2` or `AL3`; a rooted or emulated device does not. |
| `SEC-007` | Apple's fraud-metric receipt lifts an iPhone to `AL3` once the Apple key is imported (off by default). |
| `SEC-027`, `SEC-053` | The SCA key and the key-agreement key live in StrongBox or the Secure Enclave and require the user's authentication; confidential bundles decrypt only on the registered device. |
| `SEC-041` | Certificate and WebSocket pinning refuse an intercepting proxy's certificate. |
| `SEC-070`–`SEC-072`, `SEC-074` | The RASP detections (root, hooking, debugger, repackaging) fire on a prepared device and take the configured response. |
| `SEC-090`, `SEC-091`, `SEC-093` | Screenshot and screen-recording blocking, overlay filtering and SMS User Consent work on the device. |
| `SEC-160`, `SEC-190`, `QA-008` | The MASTG results of [masvs.md](masvs.md) for this release. |
| `NFR-012`, `NFR-013`, `NFR-025` | The device-side timings (DPoP proof, registration, start-up with security on) within their budgets. |
