// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/src/security/aead.dart';

void main() {
  final key = Uint8List.fromList(List.generate(32, (i) => i));
  final nonce = Uint8List.fromList(List.generate(12, (i) => 100 + i));
  final aad = Uint8List.fromList([1, 2, 3]);
  const aead = DartAead();

  test(
    'seal and open round-trip; the tag follows the ciphertext [SEC-073]',
    () {
      final clear = Uint8List.fromList(List.generate(1000, (i) => i % 251));
      final sealed = aead.seal(key, nonce, clear, aad);
      expect(sealed, hasLength(clear.length + aeadTagLength));
      expect(aead.open(key, nonce, sealed, aad), clear);
    },
  );

  test(
    'a wrong key, nonce, aad or an altered byte opens to null [SEC-073]',
    () {
      final sealed = aead.seal(key, nonce, Uint8List.fromList([9, 9, 9]), aad);
      final otherKey = Uint8List.fromList(key)..[0] ^= 1;
      final otherNonce = Uint8List.fromList(nonce)..[0] ^= 1;
      final altered = Uint8List.fromList(sealed)..[0] ^= 1;
      expect(aead.open(otherKey, nonce, sealed, aad), isNull);
      expect(aead.open(key, otherNonce, sealed, aad), isNull);
      expect(aead.open(key, nonce, sealed, Uint8List(0)), isNull);
      expect(aead.open(key, nonce, altered, aad), isNull);
      expect(aead.open(key, nonce, Uint8List(3), aad), isNull);
    },
  );

  test('a key or nonce of the wrong length is refused', () {
    expect(
      () => aead.seal(Uint8List(16), nonce, Uint8List(0), aad),
      throwsArgumentError,
    );
    expect(
      () => aead.open(key, Uint8List(8), Uint8List(16), aad),
      throwsArgumentError,
    );
  });
}
