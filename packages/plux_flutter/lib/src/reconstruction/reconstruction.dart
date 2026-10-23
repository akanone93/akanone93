// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The data of reconstruction mode (SEC-142, ADR-0082): a reconstruction
/// release as the server serves it, and the moment its signed manifest
/// describes once it has been verified.
library;

import 'dart:typed_data';
import 'dart:ui' show Locale;

import 'package:flutter/material.dart' show ThemeMode;
import 'package:plux_flutter/src/errors/plux_exception.dart';

/// One signature over a reconstruction manifest.
final class PluxReconstructionSignature {
  /// Creates a signature of [signature] bytes made by key [keyId].
  const PluxReconstructionSignature({
    required this.keyId,
    required this.algorithm,
    required this.signature,
  });

  /// The signing key's ID.
  final String keyId;

  /// The algorithm, `ed25519`.
  final String algorithm;

  /// The raw signature bytes.
  final Uint8List signature;
}

/// One bundle of a reconstruction release, exactly as it was shipped.
final class PluxReconstructionBundle {
  /// Creates a bundle of plugin [key] (empty for the app bundle).
  const PluxReconstructionBundle({required this.key, required this.bytes});

  /// The plugin key; empty for the app bundle.
  final String key;

  /// The bundle's bytes, not yet verified.
  final Uint8List bytes;
}

/// A reconstruction release as `AuditService/GetReconstruction` returns it
/// and the viewer host downloads it: nothing in it is trusted until the
/// runtime has verified it (SEC-052).
///
/// Pass it as `PluxConfig.reconstruction`, together with the root keys the
/// host pinned at build time as `PluxConfig.rootKeys`, to run the runtime in
/// reconstruction mode. The runtime walks [roots] from those keys to the
/// `targets` keys, as a sync does, and verifies the manifest under them.
final class PluxReconstruction {
  /// Creates a reconstruction release.
  const PluxReconstruction({
    required this.id,
    required this.manifest,
    required this.signatures,
    required this.bundles,
    required this.roots,
  });

  /// The release's identifier.
  final String id;

  /// The canonical JSON (RFC 8785) of the reconstruction manifest, the
  /// bytes the signatures cover.
  final Uint8List manifest;

  /// The signatures over [manifest].
  final List<PluxReconstructionSignature> signatures;

  /// The bundles the manifest names, in any order.
  final List<PluxReconstructionBundle> bundles;

  /// The signed root files after the pinned root, oldest first, as
  /// `ManifestService/GetRootKeys` returns them (SEC-051); empty while the
  /// pinned keys are the environment's current ones.
  final List<Uint8List> roots;
}

/// The moment a verified reconstruction manifest describes: what the
/// runtime shows, and with which presentation (SEC-142).
final class PluxReconstructionMoment {
  /// Creates a moment.
  const PluxReconstructionMoment({
    required this.id,
    required this.at,
    required this.deviceId,
    required this.releaseSequence,
    required this.releaseChannel,
    required this.locale,
    required this.deviceClass,
    this.theme,
    this.textScalePercent,
    this.route,
  });

  /// Reads the fields of a manifest whose signature has been verified;
  /// throws [PluxException] with `PLX-3001` when one is missing or of the
  /// wrong type.
  factory PluxReconstructionMoment.fromManifest(Map<String, Object?> m) {
    try {
      final subject = m['subject']! as Map<String, Object?>;
      final release = m['release']! as Map<String, Object?>;
      final presentation = m['presentation']! as Map<String, Object?>;
      return PluxReconstructionMoment(
        id: m['id']! as String,
        at: DateTime.parse(m['moment']! as String),
        deviceId: subject['deviceId']! as String,
        releaseSequence: release['sequence']! as int,
        releaseChannel: release['channel']! as String,
        locale: presentation['locale']! as String,
        deviceClass: presentation['deviceClass']! as String,
        theme: presentation['theme'] as String?,
        textScalePercent: presentation['textScalePercent'] as int?,
        route: m['route'] as String?,
      );
    } on Object {
      throw const PluxException(
        PluxErrorCode.manifestSignatureInvalid,
        'a field of the reconstruction manifest is missing or of the wrong '
        'type',
      );
    }
  }

  /// The reconstruction release's identifier.
  final String id;

  /// The reconstructed instant.
  final DateTime at;

  /// The device the moment was resolved on.
  final String deviceId;

  /// The sequence of the release the device ran.
  final int releaseSequence;

  /// The channel the device followed.
  final String releaseChannel;

  /// The BCP 47 tag of the locale the device used.
  final String locale;

  /// The device class the device reported, `phone` or `tablet`.
  final String deviceClass;

  /// The theme the device used, such as `dark`; null when not reported.
  final String? theme;

  /// The text scale in percent, 100 by default; null when not reported.
  final int? textScalePercent;

  /// The page the device last reported viewing; null when none was
  /// recorded. Its parameters are not recorded, so it opens without any.
  final String? route;

  /// The recorded locale, or null for a tag that cannot be read.
  Locale? get localeValue {
    final parts = locale.split(RegExp('[-_]'));
    if (parts.first.isEmpty) return null;
    return Locale.fromSubtags(
      languageCode: parts.first,
      countryCode: parts.skip(1).where((p) => p.length == 2).firstOrNull,
    );
  }

  /// The recorded theme: dark or light as reported, else the system's.
  ThemeMode get themeMode => switch (theme) {
    'dark' => ThemeMode.dark,
    'light' => ThemeMode.light,
    _ => ThemeMode.system,
  };

  /// The recorded text scale as a factor.
  double get textScale => (textScalePercent ?? 100) / 100;

  /// Whether the device was a tablet.
  bool get isTablet => deviceClass == 'tablet';
}
