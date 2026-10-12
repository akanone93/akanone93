// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The seam between the runtime and an optional security module such as
/// `plux_security` (SEC-070, SEC-071, ADR-0057): the module detects and
/// decides; the runtime owns the telemetry, the assurance level and the
/// device's standing with the server. Without a module nothing is detected.
library;

import 'package:flutter/foundation.dart' show ValueListenable;
import 'package:plux_flutter/src/security/security_config.dart';
import 'package:plux_flutter/src/telemetry/events.dart';

/// A security module the host registers in `PluxConfig.securityModules`,
/// such as `PluxSecurity` of `plux_security` (RT-060).
abstract interface class PluxSecurityModule {
  /// The module's package name, such as `plux_security`.
  String get name;

  /// Starts the module once the runtime is up. [host] stays valid until
  /// [detach].
  void attach(PluxSecurityHost host);

  /// Stops the module when the runtime is disposed.
  void detach();
}

/// What the runtime offers an attached [PluxSecurityModule]. Everything
/// here is safe to call at any time; after the runtime is disposed the
/// calls do nothing.
abstract interface class PluxSecurityHost {
  /// The security settings in force (SEC-182); they change when a sync
  /// applies a new configuration.
  ValueListenable<SecuritySettings> get settings;

  /// The device's assurance level, 0 to 3, as the server named it and any
  /// [limitAssurance] left it (SEC-007).
  ValueListenable<int> get assurance;

  /// Reports a detection as the `necessary` `rasp_detection` event
  /// (SEC-071, ADR-0057) and sends it at once; the server lowers the
  /// device's level. [kind], [severity] and [response] are the module's
  /// own words; each of [indicators] is a short code naming the check that
  /// tripped, never a path, an identifier or a value from the device
  /// (SEC-092).
  void reportDetection({
    required String kind,
    required String severity,
    required String response,
    List<String> indicators = const [],
  });

  /// Caps this device's assurance level at [ceiling] (0 to 3) for as long
  /// as the runtime runs, so that pages, routes, data sources and syncs
  /// that ask for more are refused with `PLX-6002` (the `degrade`
  /// response, SEC-071). A later token never lifts the level above the
  /// cap; 3 removes the cap, and the level rises again only when the
  /// server's next token names it.
  void limitAssurance(int ceiling);

  /// Offers the server fresh evidence for the device's key now, after a
  /// detection (SEC-006, SEC-025). Completes without error when the
  /// device is not registered or the exchange fails; the next sync tries
  /// again as the server asks.
  Future<void> requestReattestation();
}

/// The runtime's [PluxSecurityHost], wired to the runtime by callbacks.
final class RuntimeSecurityHost implements PluxSecurityHost {
  /// Creates the host.
  RuntimeSecurityHost({
    required this.settings,
    required this.assurance,
    required this._record,
    required this._limit,
    required this._reattest,
  });

  @override
  final ValueListenable<SecuritySettings> settings;

  @override
  final ValueListenable<int> assurance;

  final void Function(String name, Map<String, Object?> fields) _record;
  final void Function(int ceiling) _limit;
  final Future<void> Function() _reattest;

  @override
  void reportDetection({
    required String kind,
    required String severity,
    required String response,
    List<String> indicators = const [],
  }) => _record('rasp_detection', {
    'kind': kind,
    'severity': severity,
    'response': response,
    if (indicators.isNotEmpty) 'indicators': clipText(indicators.join(',')),
  });

  @override
  void limitAssurance(int ceiling) => _limit(ceiling.clamp(0, 3));

  @override
  Future<void> requestReattestation() => _reattest();
}
