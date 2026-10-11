// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Confidential bundles on the device (SEC-053, ADR-0055).
///
/// A confidential release arrives with every section of every bundle sealed
/// under a content key that is wrapped to this device's key-agreement key.
/// The order is the one `SEC-052` demands: the bundle hash the signed
/// manifest names is checked against the ciphertext, then every section hash,
/// and only then is anything decrypted — into memory, on a background
/// isolate, never to disk. Bundles stay encrypted at rest.
library;

import 'dart:convert';
import 'dart:io';
import 'dart:isolate';
import 'dart:typed_data';

import 'package:crypto/crypto.dart';
import 'package:cryptography/cryptography.dart'
    show Mac, MacAlgorithm, PaddingAlgorithm, SecretBox, SecretKey;
import 'package:cryptography/dart.dart' show DartAesCbc;
import 'package:plux_flutter/src/bundle/container.dart';
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/security/aead.dart';
import 'package:plux_flutter/src/security/device_keys.dart';
import 'package:plux_flutter/src/verify/bundle_verifier.dart';

/// The required feature of a confidential bundle (BND-008): a runtime that
/// lacks it refuses the release before it downloads anything.
const String confidentialFeature = 'security.confidential.v1';

/// The size of a sealed section's nonce.
const int _nonceLength = aeadNonceLength;

PluxException _unavailable(String message) =>
    PluxException(PluxErrorCode.confidentialKeyUnavailable, message);

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

/// The HKDF info a content key is bound to: the label, then the device,
/// app, environment and release sequence, each after a zero byte. It
/// matches the server's `bundle.KeyInfo`.
Uint8List contentKeyInfo(
  String deviceId,
  String app,
  String environment,
  int sequence,
) => Uint8List.fromList([
  ...utf8.encode('plux.confidential.v1'),
  for (final part in [deviceId, app, environment, '$sequence']) ...[
    0,
    ...utf8.encode(part),
  ],
]);

/// HKDF-SHA-256 (RFC 5869) of [secret], for one block of output.
Uint8List _hkdf32(List<int> secret, List<int> salt, List<int> info) {
  final prk = Hmac(sha256, salt).convert(secret).bytes;
  return Uint8List.fromList(Hmac(sha256, prk).convert([...info, 1]).bytes);
}

/// AES in the one-block ECB mode AES-KW needs: CBC with a zero IV and no
/// padding on a single block.
final class _NoPadding extends PaddingAlgorithm {
  const _NoPadding();

  @override
  int getBlockPadding(int blockLength, Uint8List bytes) => 0;

  @override
  int paddingLength(int blockLength, int dataLength) => 0;

  @override
  void setBlockPadding(int blockLength, Uint8List bytes, int start) {}
}

const _kwIv = [0xA6, 0xA6, 0xA6, 0xA6, 0xA6, 0xA6, 0xA6, 0xA6];

/// Unwraps [wrapped] under [kek] with AES-KW (RFC 3394); null when the
/// integrity check fails. It is asynchronous because the AES it needs is
/// `package:cryptography`'s CBC.
Future<Uint8List?> aesKeyUnwrap(Uint8List kek, Uint8List wrapped) async {
  if (wrapped.length < 24 || wrapped.length % 8 != 0) return null;
  final cipher = DartAesCbc(
    macAlgorithm: MacAlgorithm.empty,
    paddingAlgorithm: const _NoPadding(),
    secretKeyLength: kek.length,
  );
  final n = wrapped.length ~/ 8 - 1;
  final a = Uint8List.fromList(wrapped.sublist(0, 8));
  final r = Uint8List.fromList(wrapped.sublist(8));
  final zero = Uint8List(16);
  for (var j = 5; j >= 0; j--) {
    for (var i = n - 1; i >= 0; i--) {
      final t = n * j + i + 1;
      // A = A xor t, t big endian in the low bytes.
      for (var k = 0; k < 4; k++) {
        a[7 - k] ^= (t >> (8 * k)) & 0xff;
      }
      final block = Uint8List.fromList([...a, ...r.sublist(8 * i, 8 * i + 8)]);
      final plain = await cipher.decrypt(
        SecretBox(block, nonce: zero, mac: Mac.empty),
        secretKey: SecretKey(kek),
      );
      a.setAll(0, plain.sublist(0, 8));
      r.setAll(8 * i, plain.sublist(8, 16));
    }
  }
  var diff = 0;
  for (var i = 0; i < 8; i++) {
    diff |= a[i] ^ _kwIv[i];
  }
  return diff == 0 ? r : null;
}

/// Unwraps [envelope] with the device's key-agreement key under [alias]
/// (SEC-053): ECDH in the platform, HKDF-SHA-256 bound to the device and
/// the release [sequence], then AES-KW. Throws `PLX-6050` when the key is
/// missing, was wrapped for another device or release, or was altered.
Future<Uint8List> unwrapContentKey(
  WrappedContentKey envelope, {
  required DeviceKeys keys,
  required String alias,
  required int sequence,
}) async {
  final eph = envelope.ephemeralPublicKey;
  if (eph.length != 65 ||
      eph[0] != 4 ||
      envelope.wrappedKey.length != aeadKeyLength + 8) {
    throw _unavailable('the wrapped content key is malformed');
  }
  final Uint8List secret;
  final DeviceKey own;
  try {
    final loaded = await keys.load(alias, KeyPurpose.agreement);
    if (loaded == null) {
      throw _unavailable('the device has no key-agreement key');
    }
    own = loaded;
    secret = await keys.agree(
      alias,
      EcPublicKey(eph.sublist(1, 33), eph.sublist(33)),
    );
  } on PluxException catch (e) {
    if (e.code == PluxErrorCode.confidentialKeyUnavailable) rethrow;
    throw _unavailable('the key-agreement key cannot be used');
  } on ArgumentError {
    throw _unavailable('the wrapped content key names no valid point');
  }
  final salt = [...eph, 4, ...own.publicKey.x, ...own.publicKey.y];
  final kek = _hkdf32(
    secret,
    salt,
    contentKeyInfo(
      envelope.deviceId,
      envelope.app,
      envelope.environment,
      sequence,
    ),
  );
  final key = await aesKeyUnwrap(kek, envelope.wrappedKey);
  if (key == null) {
    throw _unavailable('the content key does not unwrap for this release');
  }
  return key;
}

/// The associated data of a sealed section: container kind, section kind
/// (little endian) and section ID. It matches the server's `sectionAAD`.
Uint8List _aad(int containerKind, Section s) {
  final out = Uint8List(4 + 16);
  ByteData.sublistView(out)
    ..setUint16(0, containerKind, Endian.little)
    ..setUint16(2, s.kind, Endian.little);
  out.setAll(4, s.id);
  return out;
}

/// Checks a confidential bundle as it is downloaded, before it is stored
/// and before any decryption (SEC-052): the container structure, the bundle
/// hash against the signed [expectedHash] — which covers the ciphertext —
/// the encrypted flag, and every section hash. The sections are not parsed
/// as FlatBuffers; they are sealed bytes until [decryptBundle] opens them.
BundleContainer verifyConfidentialBundle(
  Uint8List data,
  Uint8List expectedHash,
) {
  final b = BundleContainer.parse(data);
  final actual = bundleHash(data, b.sections.length);
  var same = actual.length == expectedHash.length;
  for (var i = 0; same && i < actual.length; i++) {
    same = actual[i] == expectedHash[i];
  }
  if (!same) {
    throw const PluxException(
      PluxErrorCode.bundleMalformed,
      'the bundle hash is not the signed one',
    );
  }
  if (b.flags & 1 == 0) {
    throw const PluxException(
      PluxErrorCode.bundleMalformed,
      'a confidential bundle is not marked encrypted',
    );
  }
  for (final s in b.sections) {
    checkSectionHash(s);
    if (s.data.length < aeadTagLength + _nonceLength) {
      throw const PluxException(
        PluxErrorCode.bundleMalformed,
        'a sealed section is shorter than its nonce and tag',
      );
    }
  }
  return b;
}

/// Decrypts a confidential bundle into the plain container the server
/// sealed it from. [sealed] is verified first with
/// [verifyConfidentialBundle] against [expectedHash] — nothing is decrypted
/// until the signed hash and every section hash match — and every section
/// must then open under [contentKey] (`PLX-6050` otherwise). The result
/// lives in memory only.
Uint8List decryptBundle(
  Uint8List sealed,
  Uint8List expectedHash,
  Uint8List contentKey, {
  Aead aead = const DartAead(),
}) {
  final b = verifyConfidentialBundle(sealed, expectedHash);
  final plain = <Section>[];
  for (final s in b.sections) {
    final nonce = Uint8List.sublistView(s.data, 0, _nonceLength);
    final body = Uint8List.sublistView(s.data, _nonceLength);
    final opened = aead.open(contentKey, nonce, body, _aad(b.kind, s));
    if (opened == null) {
      throw _unavailable('a section does not open under the content key');
    }
    plain.add(Section(s.kind, s.id, Uint8List(0), opened));
  }
  return encodeBundle(b.kind, plain);
}

/// Reads the sealed bundle at [path] and decrypts it on a background
/// isolate (L-7); the plain container comes back in memory.
Future<Uint8List> decryptBundleFile(
  String path,
  Uint8List expectedHash,
  Uint8List contentKey, {
  Aead aead = const DartAead(),
}) => Isolate.run(
  () => decryptBundle(
    File(path).readAsBytesSync(),
    expectedHash,
    contentKey,
    aead: aead,
  ),
);
