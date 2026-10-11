// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:math';
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/src/security/aead.dart';
import 'package:plux_flutter/src/security/record_cipher.dart';

String _hex(List<int> b) =>
    b.map((x) => x.toRadixString(16).padLeft(2, '0')).join();

void main() {
  const cipher = RecordCipher(DartAead());
  final key = Uint8List.fromList(List.generate(32, (i) => i));
  final nonce = Uint8List.fromList(List.generate(12, (i) => 100 + i));
  final clear = Uint8List.fromList(utf8.encode('hello plux'));

  test('a record is the version byte, the nonce, the ciphertext and the tag, '
      'as an independent AES-GCM computes it [SEC-073]', () {
    // Computed with Python's cryptography package: AES-256-GCM under the key
    // 0..31 and the nonce 100..111, additional data
    // len('cache') as two bytes || 'cache' || 'abc'.
    const expected =
        '016465666768696a6b6c6d6e6f207eb20a16c926f24b1a611ff1eded9556e09749'
        'b239290788f1';
    final record = cipher.seal(key, 'cache', 'abc', clear, nonce: nonce);
    expect(_hex(record), expected);
    expect(record[0], recordVersion);
    expect(record, hasLength(clear.length + recordOverhead));
    expect(cipher.open(key, 'cache', 'abc', record), clear);
  });

  test('the store and the record key are authenticated, and no pair of them '
      'reads as another [SEC-073]', () {
    final record = cipher.seal(key, 'cache', 'abc', clear);
    expect(cipher.open(key, 'cache', 'abc', record), clear);
    expect(cipher.open(key, 'outbox', 'abc', record), isNull);
    expect(cipher.open(key, 'cache', 'abd', record), isNull);
    final split = cipher.seal(key, 'ab', 'cd', clear);
    expect(cipher.open(key, 'abc', 'd', split), isNull);
    expect(cipher.open(key, 'a', 'bcd', split), isNull);
  });

  test('another key, an altered byte, an unknown version or a short record '
      'opens to null [SEC-073]', () {
    final record = cipher.seal(key, 's', 'k', clear);
    final other = Uint8List.fromList(key)..[0] ^= 1;
    expect(cipher.open(other, 's', 'k', record), isNull);
    for (final i in [0, 1, 13, record.length - 1]) {
      final altered = Uint8List.fromList(record)..[i] ^= 1;
      expect(cipher.open(key, 's', 'k', altered), isNull, reason: 'byte $i');
    }
    expect(cipher.open(key, 's', 'k', Uint8List(recordOverhead - 1)), isNull);
    expect(cipher.open(key, 's', 'k', Uint8List(0)), isNull);
    expect(RecordCipher.isRecord(record), isTrue);
    expect(
      RecordCipher.isRecord(Uint8List.fromList([2, ...record.skip(1)])),
      isFalse,
    );
  });

  test('every record gets its own random nonce [SEC-073]', () {
    final a = cipher.seal(key, 's', 'k', clear);
    final b = cipher.seal(key, 's', 'k', clear);
    expect(a.sublist(1, 13), isNot(b.sublist(1, 13)));
    expect(a, isNot(b));
    expect(
      RecordCipher.newNonce(Random(7)),
      RecordCipher.newNonce(Random(7)),
      reason: 'an injected generator is deterministic',
    );
  });

  test('an empty plaintext seals and opens [SEC-073]', () {
    final record = cipher.seal(key, 's', 'k', Uint8List(0));
    expect(record, hasLength(recordOverhead));
    expect(cipher.open(key, 's', 'k', record), isEmpty);
  });
}
