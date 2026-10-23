// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Runtime application self-protection for Plux apps (SEC-070, SEC-071,
/// ADR-0057).
///
/// Add `PluxSecurity()` to `PluxConfig.securityModules` and wrap the app in
/// a `PluxSecurityGate`. Detection raises the cost of an attack; it does
/// not stop a determined attacker on a device they control (see the
/// README). For the jailbreak check on iOS the host lists the URL schemes
/// `cydia`, `sileo` and `zbra` in `LSApplicationQueriesSchemes`.
library;

export 'src/detection.dart' show PluxDetection, PluxDetectionKind, PluxSeverity;
export 'src/detector.dart'
    show PlatformRaspDetector, PluxRaspDetector, PluxRaspExpectations;
export 'src/dev_handler.dart' show pluxSecuritySimulateDetectionHandler;
export 'src/dev_mode.dart' show PluxSecurityMode;
export 'src/gate.dart'
    show PluxBlockedBuilder, PluxSecurityGate, PluxWarningBuilder;
export 'src/response.dart' show PluxRaspPolicy, PluxRaspResponse;
export 'src/security.dart' show PluxSecurity, PluxSecurityStatus;
