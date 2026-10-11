// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// A [PluxSecurityHost] that records what a module asks of the runtime.
library;

import 'package:flutter/foundation.dart';
import 'package:plux_flutter/plux_flutter.dart';

/// One call of `reportDetection`.
typedef Reported = ({
  String kind,
  String severity,
  String response,
  List<String> indicators,
});

/// The runtime's side of a security module, in memory.
final class FakeHost implements PluxSecurityHost {
  /// Creates a host under the standard profile's built-in settings.
  FakeHost([SecuritySettings? settings])
    : settings = ValueNotifier(settings ?? SecuritySettings.builtIn);

  @override
  final ValueNotifier<SecuritySettings> settings;

  @override
  final ValueNotifier<int> assurance = ValueNotifier(3);

  /// The detections reported, in order.
  final List<Reported> reported = [];

  /// The ceilings set, in order.
  final List<int> ceilings = [];

  /// How often re-attestation was requested.
  var reattestations = 0;

  /// The events in the order they happened.
  final List<String> log = [];

  @override
  void reportDetection({
    required String kind,
    required String severity,
    required String response,
    List<String> indicators = const [],
  }) {
    reported.add((
      kind: kind,
      severity: severity,
      response: response,
      indicators: indicators,
    ));
    log.add('report:$kind');
  }

  @override
  void limitAssurance(int ceiling) {
    ceilings.add(ceiling);
    if (assurance.value > ceiling) assurance.value = ceiling;
    log.add('limit:$ceiling');
  }

  @override
  Future<void> requestReattestation() async {
    reattestations++;
    log.add('reattest');
  }
}

/// The settings of a device document that sets `raspRootHookingResponse`.
SecuritySettings settingsWithRaspResponse(String response) =>
    SecuritySettings.fromDocument({
      'profile': 'standard',
      'overrides': {'raspRootHookingResponse': response},
    });
