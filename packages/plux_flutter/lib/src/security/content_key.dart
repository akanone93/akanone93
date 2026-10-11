// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// What the release store keeps of a confidential release's key (SEC-053,
/// ADR-0055). It has no Flutter dependency, so the store's own tools can
/// read a record.
library;

import 'dart:convert';
import 'dart:typed_data';

/// The required feature of a confidential bundle (BND-008): a runtime that
/// lacks it refuses the release before it downloads anything.
const String confidentialFeature = 'security.confidential.v1';

/// A content key wrapped for this device, as the server delivered it, with
/// what its derivation is bound to (SEC-053). It is stored beside the
/// release it belongs to; it opens only with the device's key-agreement key.
final class WrappedContentKey {
  /// Creates the envelope.
  const WrappedContentKey({
    required this.ephemeralPublicKey,
    required this.wrappedKey,
    required this.deviceId,
    required this.app,
    required this.environment,
  });

  /// Reads the envelope [toJson] writes.
  factory WrappedContentKey.fromJson(Map<String, Object?> j) =>
      WrappedContentKey(
        ephemeralPublicKey: base64.decode(j['ephemeralPublicKey']! as String),
        wrappedKey: base64.decode(j['wrappedKey']! as String),
        deviceId: j['deviceId']! as String,
        app: j['app']! as String,
        environment: j['environment']! as String,
      );

  /// The server's one-time key: an uncompressed P-256 point, 65 bytes.
  final Uint8List ephemeralPublicKey;

  /// The AES-KW output, 40 bytes.
  final Uint8List wrappedKey;

  /// The device the key was wrapped for.
  final String deviceId;

  /// The app of the signed manifest.
  final String app;

  /// The environment of the signed manifest.
  final String environment;

  /// The envelope as JSON.
  Map<String, Object?> toJson() => {
    'ephemeralPublicKey': base64.encode(ephemeralPublicKey),
    'wrappedKey': base64.encode(wrappedKey),
    'deviceId': deviceId,
    'app': app,
    'environment': environment,
  };
}
