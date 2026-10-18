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

## P8 — Localisation, accessibility, scanner and media

Run with release builds of the reference apps. The locale items use `en`, `de-DE`, `ar` and
`am-ET` (`I18N-002`).

| # | Requirements | Steps | Expected | Until it passes |
|---|---|---|---|---|
| L1 | `A11Y-001`, `A11Y-006`, `QA-011` | Android with TalkBack on, in `en` and in `ar`: walk Plux Bank and Plux Express from launch through sign-in, a transfer, the instalments page and a form with an error. Swipe through every screen in reading order; navigate by headings; trigger an error. | Every interactive element is announced with role, name, value and state; reading order follows `focusOrder`; headings are navigable; the error is announced once (`announce`, live region); nothing is announced as an unlabelled button; in `ar` the order runs right to left. | `A11Y-001`, `A11Y-006` stay `WIP` for the screen-reader behaviour |
| L2 | `A11Y-001`, `A11Y-006`, `QA-011` | The same walk on the iPhone with VoiceOver on, in `en` and `ar`, including the rotor for headings and containers. | As L1; groups read as one element where `group` is set; the announcement is spoken. | As L1 |
| L3 | `A11Y-007` | On both phones turn on, one at a time: bold text, reduce motion, high contrast (iOS: increase contrast; Android: high contrast text), and the screen reader. Reopen the apps. | The runtime follows each setting without restart; animations are replaced by non-moving alternatives under reduce motion; the `device` root of PXL reports the settings. | — (verified in widget tests; the device confirms) |
| L4 | `A11Y-004`, `A11Y-005`, `QA-011` | Set the system font size to the largest, and the display size to the largest; open the reference apps' main pages in `en` and `de-DE`. | Pages stay usable at 200% text; no text is cut off or hidden by a fixed-height container; targets stay at least 48 dp (Android) and 44 pt (iOS). | — |
| L5 | `QA-011` | Run the UiAutomator (Android) and XCUITest (iOS) accessibility-tree scripts of the reference apps on the real phones, not emulators or simulators. | The scripted passes agree with the CI results: names, roles, order and tap targets as in the tree the platform exposes. | `QA-011` device passes stay unverified until this passes |
| L6 | `I18N-002`, `I18N-004`, `I18N-005` | Change the system language in turn to `en`, `de-DE`, `ar` and `am-ET` with the reference apps open and closed; also change it inside the app (`setLocale`). | The app follows the host's locale; switching at runtime rebuilds the pages without a restart and without losing page state; `ar` mirrors layouts and system widgets; `am-ET` shows Ethiopic text in the pinned font without missing glyphs; numbers, money (`1.234,56 €` in `de-DE`) and dates follow the locale. | `I18N-004`, `I18N-005` on the device |
| L7 | `I18N-003` | Set the app's calendar to Ethiopian in `am-ET` and to Umm al-Qura in `ar`; open a date field and a date picker; compare with a known Ethiopian date (including the 13th month and the new year) and with the Umm al-Qura date of the same day from an official Saudi calendar. | The Ethiopian date and month names are right; the Umm al-Qura date agrees with the official table; pickers convert both ways. | — |
| L8 | `I18N-002`, `I18N-008` (D15) | A native speaker of Arabic and a native speaker of Amharic read every text of the reference apps in `ar` and `am-ET` on the device, including plurals, genders, dates and error messages. | Corrections are listed in the evidence file and made in the translations; plural forms read correctly for several counts; nothing is machine-translated nonsense. | The `ar` and `am-ET` translations are not marked `approved` until this passes |
| S1 | ADR-0051 (P6 plan B21) | On a Pixel with Google Play services: scan the set of codes of ADR-0051's measurement (QR at levels L, M and H, EAN-13, Code 128) in the eight conditions (clean, small, damaged, blurred, badly lit, low contrast, grainy, harsh gradient) with ML Kit, then with ZXing-C++ forced. Record hits and time per frame. | ML Kit's results are recorded beside the host measurement of ZXing-C++ (36/40, 0.7 ms per frame); the numbers go into ADR-0051's Measurement. | The ADR's "not yet measured" ML Kit line stays open |
| S2 | ADR-0051 | Build `plux_scanner` for `arm64-v8a`, `armeabi-v7a` and `x86_64` with the NDK; record the size of each library and of the release APK or app bundle's download size with and without the fallback. | The sizes per ABI go into ADR-0051's Measurement; the fallback adds about as much as the host library (711 KB, 586 KB stripped) predicts, per ABI. | The ADR's "per-ABI sizes" line stays open |
| S3 | ADR-0051 Revision (CameraX frame source) | On an Android device or image without Google Play services (a Huawei phone, or an AOSP image): open a scanner page. | The scanner works through the CameraX analysis stream and ZXing-C++ without Play services; no model download is attempted or fails silently; the preview is smooth and the UI isolate stays responsive. | The fallback frame source is verified on the host and by tests only |
| M1 | `SEC-041`, `AST-002` | With `plux_maps` showing a map on both phones: route the phone through an intercepting proxy whose certificate is installed on the device. | Tiles from a declared domain load through the pinned client only where the domain is pinned or the server's; an intercepting certificate makes tile loads fail (`PLX-6020` for pinned hosts) and the page shows its placeholder; tiles from an undeclared domain are refused. | — |
| M2 | `SEC-041`, `AST-002`, `pinnedMediaOnly` | Play a video and an audio file from a declared domain in two builds: `standard` (`pinnedMediaOnly` off) and `maximum` (on). Repeat with the intercepting proxy from M1. Try a file over `media.pinnedDownloadSize`. | Under `standard` the media streams (unpinned, as documented) and plays through the proxy. Under `maximum` the file is downloaded through the pinned client to a temporary file, playback starts after the download, the interception makes the download fail, an oversized file is refused, and the temporary file is gone after playback. | `pinnedMediaOnly` verified on the host only |

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
