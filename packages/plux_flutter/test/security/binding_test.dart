// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/src/security/attestation.dart';

/// The vectors the server's `attest/binding` package reproduces too.
const _vectors = '../../schema/testdata/attest/binding.json';

Uint8List _unhex(String s) => Uint8List.fromList([
  for (var i = 0; i < s.length; i += 2)
    int.parse(s.substring(i, i + 2), radix: 16),
]);

String _hex(List<int> b) =>
    b.map((e) => e.toRadixString(16).padLeft(2, '0')).join();

void main() {
  final file =
      jsonDecode(File(_vectors).readAsStringSync()) as Map<String, Object?>;
  final vectors = (file['vectors']! as List).cast<Map<String, Object?>>();

  test('the shared vectors are present', () => expect(vectors, isNotEmpty));

  for (final v in vectors) {
    test('binding vector "${v['name']}" [SEC-004]', () {
      final hash = bindingHash(
        _unhex(v['challengeHex']! as String),
        v['jkt']! as String,
        v['kaJkt']! as String,
        v['scaJkt']! as String,
      );
      expect(_hex(hash), v['hashHex']);
      expect(base64Url.encode(hash).replaceAll('=', ''), v['requestHash']);
    });
  }

  test('every thumbprint changes the binding [SEC-004]', () {
    final c = Uint8List.fromList([1, 2, 3]);
    final base = bindingHash(c, 'd', 'k', 's');
    for (final other in [
      bindingHash(c, 'd2', 'k', 's'),
      bindingHash(c, 'd', 'k2', 's'),
      bindingHash(c, 'd', 'k', 's2'),
      bindingHash(c, 'd', 'k', ''),
      bindingHash(c, 'dk', '', 's'),
    ]) {
      expect(other, isNot(base));
    }
  });
}
