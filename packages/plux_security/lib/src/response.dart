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
/// Root and hooking follow the `raspRootHookingResponse` security setting,
/// which the server distributes per profile (SEC-182); the app can make
/// them stricter in [responses] but never looser. The other kinds have no
/// setting yet: they follow [responses], else [otherwise].
final class PluxRaspPolicy {
  /// Creates a policy.
  const PluxRaspPolicy({
    this.responses = const {},
    this.otherwise = PluxRaspResponse.report,
  });

  /// The app's response per kind.
  final Map<PluxDetectionKind, PluxRaspResponse> responses;

  /// The response to a kind that neither the settings nor [responses] name.
  final PluxRaspResponse otherwise;

  /// The response to [kind] under [settings].
  PluxRaspResponse responseFor(
    PluxDetectionKind kind,
    SecuritySettings settings,
  ) {
    final own = responses[kind];
    return switch (kind) {
      PluxDetectionKind.root ||
      PluxDetectionKind.hooking => PluxRaspResponse.fromSetting(
        settings.choice(SecuritySetting.raspRootHookingResponse),
      ).stricter(own ?? PluxRaspResponse.report),
      _ => own ?? otherwise,
    };
  }
}
