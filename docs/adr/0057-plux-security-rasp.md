# 0057. `plux_security`: runtime application self-protection

- **Status:** Accepted (maintainer, 2026-10-07, P6 S0; P6 plan §2.1 B18)
- **Date:** 2026-10-07
- **Requirements:** `SEC-070`, `SEC-071`, `SEC-007`, `SEC-161`

## Context and problem

MASVS-RESILIENCE asks the app to detect hostile environments. `SEC-070` lists the
detections; `SEC-071` the responses and their effect on the assurance level.

## Decision drivers

- Plux's own code, no third-party RASP SDK (B18); optional package, no cost when absent.
- Detections are signals, not proofs: the server lowers assurance; the device never raises it.
- Nothing personal is reported.

## Considered options

1. **An optional package `plux_security` with native detectors on Android and iOS, a
   policy engine in Dart, and a `rasp_detection` telemetry event.**
2. A commercial RASP SDK.
3. Detections inside `plux_flutter`.

## Decision

Chosen option: **1**.

- **Detections:** root/jailbreak (files, properties, writable system paths, `su`,
  sandbox escape), hooking frameworks (Frida ports, named pipes and memory maps; Xposed,
  LSPosed, Substrate classes and libraries), debugger (`TracerPid`, `P_TRACED`), emulator
  and simulator, repackaging (signing certificate digest vs `PluxConfig`), installer
  source, screen mirroring and recording, overlays, accessibility services with suspicious
  capabilities. Each detector runs off the UI isolate, at start and on a timer.
- **Responses** per detection kind from the security configuration (`report`, `warn`,
  `degrade`, `block`; `raspRootHookingResponse` sets root and hooking): `degrade` disables
  pages and functions that require `AL3`; `block` shows the host's blocking screen.
- **Reporting:** the `rasp_detection` event (`necessary` category, kind and severity only)
  goes to the server through a DPoP-protected call; the server lowers the device's level
  at once (`PLX-6100` when a request is then refused).
- `plux_flutter` exposes a detector interface; without `plux_security` nothing is
  detected and `AL3` is never reached.

## Consequences

- **Positive:** auditable, owned code; no SDK licence; size only when used.
- **Negative:** detection is an arms race; signatures need maintenance.
- **Follow-up:** S9; real-device evidence in S11 (B3).

## Options in detail

### Option 2: a commercial SDK

Licence cost, closed code inside a security-critical path, and an extra vendor.

### Option 3: inside the runtime

Every app would pay the size and permissions, including apps that do not need it.

## Revisions (2026-10-11)

Found while building S9; the decision above stands, these change how it is carried out.

- **The detector interface lives in `plux_security`.** `PluxRaspDetector` (with
  `PlatformRaspDetector` and a `FakeRaspDetector` for tests) belongs to the package that
  has the detectors. `plux_flutter` exposes the seam instead: `PluxConfig.securityModules`
  takes `PluxSecurityModule`s, and an attached module gets a `PluxSecurityHost` through
  which it reports `rasp_detection`, caps the assurance level (`degrade` caps at `AL2`,
  `block` at `AL0`) and asks for re-attestation. Without a module nothing is detected.
- **Responses and the scan interval are settings (B5).** Every kind has a response
  setting: `raspRootHookingResponse` for root and hooking, and `raspDebuggerResponse`,
  `raspEmulatorResponse`, `raspRepackagingResponse`, `raspInstallerResponse`,
  `raspScreenCaptureResponse`, `raspOverlayResponse` and `raspAccessibilityResponse`;
  `raspScanInterval` sets the scan interval. All travel to the device. The app's
  `PluxRaspPolicy` and `PluxSecurity(scanInterval:)` may only tighten them.
- **The server lowers the level.** No new RPC: the device sends `rasp_detection` over the
  DPoP-bound `IngestEvents`, and the server records, in the same call and before the event
  is stored, a detection of a modified environment (root, hooking, debugger, repackaging,
  untrusted installer, emulator) on the device row (`rasp_detected_at`, `rasp_kinds`) and
  caps its level at `AL1`. The access token issued next carries the cap. A clean
  `ReattestDevice` recomputes the level from its evidence and clears the record. Screen
  capture, overlays and accessibility services follow the moment and lower nothing on the
  server. Tokens already issued keep their level until they expire (at most 15 minutes);
  the runtime caps its own level at once.

## Revision (2026-10-22, P10: development mode)

Decided by the maintainer (P10 plan D-8; ADR-0091). In the Plux Dev app, in development
environments only, `plux_security` runs in a **report-only development mode**: each detection
(root, hooking, debugger, emulator, repackaging, installer, screen capture, overlay,
accessibility) is reported in the dev stream but does not lower the assurance level and does
not trigger `degrade` or `block`. A dev command simulates a detection of a chosen kind so plugin
authors can test how secure pages and SCA steps react. The mode exists only in debug and Dev-app
builds and only for development environments; behaviour in production, and every response setting
above, is unchanged.
