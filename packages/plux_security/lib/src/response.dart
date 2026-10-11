// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The responses to a detection and the policy that picks one (SEC-071).
library;

import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_security/src/detection.dart';

/// What the app does about a detection (SEC-071), from the mildest to the
/// strictest. Every detection is reported whatever the response.
enum PluxRaspResponse {
  /// Only report it: telemetry and the server.
  report,

  /// Report it and show a message.
  warn,

  /// Report it and disable what needs `AL3`: pages, routes and data
  /// sources that ask for the highest assurance level are refused.
  degrade,

  /// Report it and show the blocking screen.
  block;

  /// The response a security setting names. A value this version does not
  /// know is [degrade]: restrictive, but it does not lock the user out.
  static PluxRaspResponse fromSetting(String value) => values.firstWhere(
    (r) => r.name == value,
    orElse: () => PluxRaspResponse.degrade,
  );

  /// The stricter of this response and [other].
  PluxRaspResponse stricter(PluxRaspResponse other) =>
      index >= other.index ? this : other;
}

/// Picks the response to each kind of detection (SEC-071).
///
/// Every kind follows its security setting (`raspRootHookingResponse` for
/// root and hooking, `rasp<Kind>Response` for the others), which the server
/// distributes per profile (SEC-182). The app can make a response stricter
/// in [responses], never looser.
final class PluxRaspPolicy {
  /// Creates a policy.
  const PluxRaspPolicy({this.responses = const {}});

  /// The app's own response per kind; the stricter of it and the setting
  /// applies.
  final Map<PluxDetectionKind, PluxRaspResponse> responses;

  /// The setting that names the response to [kind].
  static SecuritySetting settingFor(PluxDetectionKind kind) => switch (kind) {
    PluxDetectionKind.root ||
    PluxDetectionKind.hooking => SecuritySetting.raspRootHookingResponse,
    PluxDetectionKind.debugger => SecuritySetting.raspDebuggerResponse,
    PluxDetectionKind.emulator => SecuritySetting.raspEmulatorResponse,
    PluxDetectionKind.repackaging => SecuritySetting.raspRepackagingResponse,
    PluxDetectionKind.untrustedInstaller =>
      SecuritySetting.raspInstallerResponse,
    PluxDetectionKind.screenCapture =>
      SecuritySetting.raspScreenCaptureResponse,
    PluxDetectionKind.overlay => SecuritySetting.raspOverlayResponse,
    PluxDetectionKind.accessibility =>
      SecuritySetting.raspAccessibilityResponse,
  };

  /// The response to [kind] under [settings].
  PluxRaspResponse responseFor(
    PluxDetectionKind kind,
    SecuritySettings settings,
  ) =>
      PluxRaspResponse.fromSetting(settings.choice(settingFor(kind)))
          .stricter(responses[kind] ?? PluxRaspResponse.report);
}
