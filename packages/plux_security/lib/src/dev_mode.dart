// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The report-only development mode of `plux_security` (DEV-001, ADR-0057
/// Revision 2026-10-22): what selects it, when it is refused, and how a
/// detection is named in the dev stream.
library;

import 'package:plux_security/src/detection.dart';

/// How `PluxSecurity` treats a detection.
enum PluxSecurityMode {
  /// Every detection follows the security settings: it lowers the assurance
  /// level and may degrade or block (SEC-071). The default, and the only
  /// mode a release build or a production environment ever runs.
  production,

  /// For the Plux Dev app (DEV-001): each detection is reported in the dev
  /// stream and recorded in the status, but nothing is reported to the
  /// server, no assurance ceiling is set and no response is applied. Only
  /// builds with `kPluxDevtools` and only environments other than
  /// `production` run it; anywhere else it is refused with a warning and
  /// the module runs as [production].
  developmentReportOnly,
}

/// The key of the environment that never runs the development mode.
const String productionEnvironment = 'production';

/// Why the development mode cannot run, or null when it can.
///
/// [devtools] is whether this build contains the dev channel
/// (`kPluxDevtools`) and [environment] the environment key of the
/// runtime's `PluxConfig`.
String? refuseDevelopmentMode(
  PluxSecurityMode mode, {
  required bool devtools,
  required String environment,
}) {
  if (mode != PluxSecurityMode.developmentReportOnly) return null;
  if (!devtools) {
    return 'development mode refused: this build has no dev channel; '
        'detections keep their production behaviour';
  }
  if (environment == productionEnvironment) {
    return 'development mode refused: the environment is production; '
        'detections keep their production behaviour';
  }
  return null;
}

/// The `DevSecurityDetectionKind` name of [kind], as proto3 JSON spells it.
String devKindName(PluxDetectionKind kind) =>
    'DEV_SECURITY_DETECTION_KIND_${kind.wireName.toUpperCase()}';

/// The detection kind a `DevSecurityDetectionKind` value names, given as
/// its name or its number; null for the unspecified or an unknown value.
PluxDetectionKind? kindFromDevValue(Object? value) {
  for (final k in PluxDetectionKind.values) {
    if (value == devKindName(k) || value == k.index + 1) return k;
  }
  return null;
}
