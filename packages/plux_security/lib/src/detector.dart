// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The detector interface and the platform detector behind it (SEC-070):
/// the checks themselves are Plux's own Kotlin and Swift, run off the UI
/// thread.
library;

import 'package:flutter/services.dart';
import 'package:plux_security/src/detection.dart';

/// What the app expects of itself, so that a scan can tell a repackaged
/// copy from the original (SEC-070).
final class PluxRaspExpectations {
  /// Creates the expectations.
  ///
  /// [signingCertSha256] are the SHA-256 digests of the app's signing
  /// certificates, in hex (colons and case do not matter); Android only.
  /// Without any, the repackaging check on Android is not run.
  /// [trustedInstallers] are the package names of the stores the app is
  /// distributed through; Android only, where a copy installed by anything
  /// else is an [PluxDetectionKind.untrustedInstaller]. [iosTeamId] and
  /// [iosBundleId] are the app's Apple team and bundle identifiers; iOS
  /// only, each checked when given.
  PluxRaspExpectations({
    Iterable<String> signingCertSha256 = const [],
    Iterable<String> trustedInstallers = const [
      'com.android.vending',
      'com.sec.android.app.samsungapps',
      'com.amazon.venezia',
    ],
    this.iosTeamId,
    this.iosBundleId,
  }) : signingCertSha256 = List.unmodifiable(
         signingCertSha256.map(normalizeDigest),
       ),
       trustedInstallers = List.unmodifiable(trustedInstallers);

  /// The expected signing certificate digests, lower-case hex.
  final List<String> signingCertSha256;

  /// The stores the app may be installed from.
  final List<String> trustedInstallers;

  /// The expected Apple team identifier.
  final String? iosTeamId;

  /// The expected iOS bundle identifier.
  final String? iosBundleId;

  /// A digest in lower-case hex without colons or spaces.
  static String normalizeDigest(String digest) =>
      digest.replaceAll(RegExp('[:\\s]'), '').toLowerCase();

  /// The arguments of the platform channel's `scan` call.
  Map<String, Object?> toArguments() => {
    'signingCertSha256': signingCertSha256,
    'trustedInstallers': trustedInstallers,
    'iosTeamId': iosTeamId,
    'iosBundleId': iosBundleId,
  };
}

/// Looks for hostile conditions on this device (SEC-070). A real detector
/// is [PlatformRaspDetector]; tests use `FakeRaspDetector` of
/// `package:plux_security/testing.dart`.
abstract interface class PluxRaspDetector {
  /// Runs every check and returns what it found; nothing found is an empty
  /// list. Must not block the UI thread.
  Future<List<PluxDetection>> scan(PluxRaspExpectations expectations);
}

/// The detector that asks the Kotlin and Swift checks of this package
/// through the `dev.plux/security` channel. The checks run on a background
/// thread of the platform; on a platform without them (desktop, web) a scan
/// finds nothing.
final class PlatformRaspDetector implements PluxRaspDetector {
  /// Creates the detector; [channel] is replaceable in tests.
  const PlatformRaspDetector({
    this._channel = const MethodChannel('dev.plux/security'),
  });

  final MethodChannel _channel;

  @override
  Future<List<PluxDetection>> scan(PluxRaspExpectations expectations) async {
    final List<Object?>? reply;
    try {
      reply = await _channel.invokeListMethod<Object?>(
        'scan',
        expectations.toArguments(),
      );
    } on MissingPluginException {
      return const [];
    }
    final found = <PluxDetection>[];
    for (final entry in reply ?? const <Object?>[]) {
      if (entry is! Map<Object?, Object?>) continue;
      final kind = PluxDetectionKind.fromWire(entry['kind']);
      if (kind == null) continue;
      final indicators = entry['indicators'];
      found.add(
        PluxDetection(kind, [
          if (indicators is List<Object?>) ...indicators.whereType<String>(),
        ]),
      );
    }
    return found;
  }
}
