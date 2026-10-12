<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# plux_security

Runtime application self-protection (RASP) for apps that host [Plux](https://nightcode42.github.io/plux3/)
pages with [`plux_flutter`](https://pub.dev/packages/plux_flutter) (`SEC-070`, `SEC-071`, ADR-0057).
Plux's own Kotlin and Swift checks, no third-party SDK.

## What it does, and does not do

RASP **raises the cost** of attacking your app. It does **not** stop a determined attacker on a
device they control: someone who owns the device can hide root, rename or relocate Frida, clear
debugger flags, patch the checks out of the binary or fake their answers. Every check here is
a signal for the server to weigh, not proof. The protection that matters is on the server:
a detection lowers the device's assurance level there, so requests that need a high level are
refused whatever the app on the device claims. Treat this package as one layer, never as the
only one.

## Detections

| Kind | Android | iOS |
|---|---|---|
| `root` | `su` binaries, test-keys firmware, Magisk traces, writable system partitions | Jailbreak files, a write outside the sandbox, package-manager URL schemes |
| `hooking` | Frida port, files, libraries and threads; Xposed and LSPosed classes and libraries | Substrate-like and Frida dylibs, `DYLD_INSERT_LIBRARIES` |
| `debugger` | Debugger attached, `TracerPid` | `P_TRACED` |
| `emulator` | Build properties, QEMU device files | Simulator |
| `repackaging` | Signing certificate SHA-256 differs from the expected one | Bundle or team identifier differs |
| `untrusted_installer` | Installed by something other than a trusted store | not applicable |
| `screen_capture` | A cast or mirroring display (a recording of the device's own display is not visible on every Android version) | `UIScreen.isCaptured` |
| `overlay` | A touch that arrived while another window covered the app | not applicable |
| `accessibility` | An enabled non-system service that can read the window and perform gestures | not applicable |

Checks run off the UI thread, at start and every `raspScanInterval` seconds (60 by default). A
hostile environment (root, hooking, debugger, emulator, repackaging, installer) stays in force until
the app restarts, because what it did cannot be undone by its going away. Screen capture,
overlays and accessibility services follow the current state.

## Usage

```dart
final security = PluxSecurity(
  expectations: PluxRaspExpectations(
    signingCertSha256: [releaseCertSha256], // Android: from `apksigner verify --print-certs`
    iosTeamId: 'ABCDE12345',
    iosBundleId: 'com.example.app',
  ),
);
await Plux.initialize(PluxConfig(
  appId: 'acme-mobile',
  endpoint: Uri.parse('https://plux.acme.example'),
  securityModules: [security],
));
runApp(MaterialApp(
  builder: (context, child) => PluxSecurityGate(security: security, child: child!),
  home: const HomeScreen(),
));
```

Without `signingCertSha256` the Android repackaging check does not run; without `iosTeamId` and
`iosBundleId` the iOS one does not. The default trusted installers are Google Play, Samsung
Galaxy Store and Amazon Appstore; list the stores you ship through in `trustedInstallers`.
On iOS, list the URL schemes `cydia`, `sileo` and `zbra` under `LSApplicationQueriesSchemes`
in `Info.plist` so that the jailbreak check can ask whether they are handled.

Debug builds and emulators trip several checks by design; set `PluxSecurity` only for release builds,
or choose `report` there.

## Responses

Each detection has one response (`SEC-071`), from mild to strict:

| Response | Effect |
|---|---|
| `report` | Reported only |
| `warn` | Reported; `PluxSecurityGate` shows a message |
| `degrade` | Reported; the assurance level is capped at `AL2`, so pages, routes and data sources that need `AL3` are refused with `PLX-6002` |
| `block` | Reported; `PluxSecurityGate` replaces the app with the blocking screen and the level is capped at `AL0` |

Every kind follows a security setting that the server distributes per profile (`SEC-182`):
`raspRootHookingResponse` for root and hooking, and `raspDebuggerResponse`, `raspEmulatorResponse`,
`raspRepackagingResponse`, `raspInstallerResponse`, `raspScreenCaptureResponse`,
`raspOverlayResponse` and `raspAccessibilityResponse` for the rest; `raspScanInterval` sets how
often the device is scanned. `PluxRaspPolicy.responses` and `PluxSecurity(scanInterval:)` can make
the response stricter or the scan more frequent, never looser. Give `PluxSecurityGate` your own
`blockedBuilder` and `warningBuilder`; the defaults are plain and English.

## Reporting

Every detection is recorded as the `necessary` telemetry event `rasp_detection` with its
`kind`, `severity`, `response` and the codes of the checks that tripped, and sent at once over the
DPoP-protected telemetry call. It carries no path, identifier or other personal data. A detection
that suggests a modified environment also lowers the device's level at the server to `AL1`
until it attests again, and makes the device offer fresh attestation evidence (`SEC-006`).

## Testing your app

`package:plux_security/testing.dart` has `FakeRaspDetector`: pass it as `PluxSecurity(detector: …)`
to see how your app reacts to a rooted or hooked device without one.

## Licence

Apache-2.0.
