// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The server's side of a confidential release, in Dart, for tests that need
/// a bundle sealed or a key wrapped for a key the test only learns at
/// registration: the same format as `backend/internal/bundle`, which
/// `schema/testdata/confidential/vectors.json` ties to the Go code.
library;

import 'dart:typed_data';

import 'package:crypto/crypto.dart';
import 'package:cryptography/cryptography.dart'
    show MacAlgorithm, PaddingAlgorithm, SecretKey;
import 'package:cryptography/dart.dart' show DartAesCbc;
import 'package:plux_flutter/src/bundle/container.dart';
import 'package:plux_flutter/src/security/aead.dart';
import 'package:plux_flutter/src/security/confidential.dart';
import 'package:plux_flutter/src/security/device_keys.dart';
import 'package:plux_flutter/src/security/p256.dart';

/// A confidential release as a fake server serves it: the sealed bundles by
/// plugin key (empty for the app bundle) and the content key.
final class SealedRelease {
  /// Creates the release.
  const SealedRelease(this.bundles, this.contentKey);

  /// Seals [plain] bundles under [contentKey]; one nonce counter serves all.
  factory SealedRelease.of(Map<String, Uint8List> plain, Uint8List contentKey) {
    var next = 0;
    return SealedRelease({
      for (final MapEntry(:key, :value) in plain.entries)
        key: sealBundle(value, contentKey, next += 1000),
    }, contentKey);
  }

  /// The sealed bundles.
  final Map<String, Uint8List> bundles;

  /// The content key.
  final Uint8List contentKey;
}

/// Seals every section of [plain] with AES-256-GCM under [key]; nonces are a
/// counter from [nonceStart].
Uint8List sealBundle(Uint8List plain, Uint8List key, int nonceStart) {
  final b = BundleContainer.parse(plain);
  var counter = nonceStart;
  final sealed = <Section>[];
  for (final s in b.sections) {
    final nonce = Uint8List(12);
    ByteData.sublistView(nonce).setUint64(4, counter++);
    final aad = Uint8List(20);
    ByteData.sublistView(aad)
      ..setUint16(0, b.kind, Endian.little)
      ..setUint16(2, s.kind, Endian.little);
    aad.setAll(4, s.id);
    final body = const DartAead().seal(key, nonce, s.data, aad);
    sealed.add(
      Section(
        s.kind,
        s.id,
        Uint8List(0),
        Uint8List.fromList([...nonce, ...body]),
      ),
    );
  }
  final out = encodeBundle(b.kind, sealed);
  // Mark the container encrypted and recompute the header hash over it.
  out[8] |= 1;
  out.setAll(16, bundleHash(out, b.sections.length));
  return out;
}

class _NoPadding extends PaddingAlgorithm {
  const _NoPadding();

  @override
  int getBlockPadding(int blockLength, Uint8List bytes) => 0;

  @override
  int paddingLength(int blockLength, int dataLength) => 0;

  @override
  void setBlockPadding(int blockLength, Uint8List bytes, int start) {}
}

/// AES-KW (RFC 3394) of [key] under [kek].
Future<Uint8List> aesKeyWrap(Uint8List kek, Uint8List key) async {
  final cipher = DartAesCbc(
    macAlgorithm: MacAlgorithm.empty,
    paddingAlgorithm: const _NoPadding(),
    secretKeyLength: kek.length,
  );
  final n = key.length ~/ 8;
  final a = Uint8List.fromList(List.filled(8, 0xA6));
  final r = Uint8List.fromList(key);
  for (var j = 0; j < 6; j++) {
    for (var i = 0; i < n; i++) {
      final box = await cipher.encrypt(
        [...a, ...r.sublist(8 * i, 8 * i + 8)],
        secretKey: SecretKey(kek),
        nonce: Uint8List(16),
      );
      final out = box.cipherText;
      final t = n * j + i + 1;
      a.setAll(0, out.sublist(0, 8));
      for (var k = 0; k < 4; k++) {
        a[7 - k] ^= (t >> (8 * k)) & 0xff;
      }
      r.setAll(8 * i, out.sublist(8, 16));
    }
  }
  return Uint8List.fromList([...a, ...r]);
}

/// Wraps [contentKey] for the key-agreement key [device] the way the server
/// does: ECDH-ES with a fresh ephemeral key (here the scalar [ephemeral]),
/// HKDF-SHA-256 with the two public points as salt, then AES-KW.
Future<({Uint8List ephemeralPublicKey, Uint8List wrappedKey})>
wrapContentKeyFor(
  Uint8List contentKey,
  EcPublicKey device,
  Uint8List info, {
  int ephemeral = 12345,
}) async {
  final e = BigInt.from(ephemeral);
  final pub = p256Multiply(e, p256Base)!;
  final eph = Uint8List.fromList([
    4,
    ...p256Bytes32(pub.x),
    ...p256Bytes32(pub.y),
  ]);
  final shared = p256Multiply(
    e,
    P256Point(p256Int(device.x), p256Int(device.y)),
  )!;
  final salt = [...eph, 4, ...device.x, ...device.y];
  final prk = Hmac(sha256, salt).convert(p256Bytes32(shared.x)).bytes;
  final kek = Uint8List.fromList(Hmac(sha256, prk).convert([...info, 1]).bytes);
  return (
    ephemeralPublicKey: eph,
    wrappedKey: await aesKeyWrap(kek, contentKey),
  );
}

/// The HKDF info for a release, as the device derives it.
Uint8List infoFor(String deviceId, String app, String environment, int seq) =>
    contentKeyInfo(deviceId, app, environment, seq);
