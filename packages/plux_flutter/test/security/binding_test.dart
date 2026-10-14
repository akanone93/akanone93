// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/src/security/attestation.dart';

/// The vectors the server's `attest/binding` package reproduces too.
const _vectors = '../../schema/testdata/attest/binding.json';
const _invocationVectors =
    '../../schema/testdata/attest/invocation_binding.json';

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

  final invocation = jsonDecode(
    File(_invocationVectors).readAsStringSync(),
  ) as Map<String, Object?>;
  for (final v
      in (invocation['vectors']! as List).cast<Map<String, Object?>>()) {
    test('invocation binding vector "${v['name']}" [FN-020] [SEC-004]', () {
      final hash = invocationBinding(
        function: v['function']! as String,
        version: v['version']! as int,
        input: Uint8List.fromList(utf8.encode(v['input']! as String)),
        jti: v['jti']! as String,
      );
      expect(_hex(hash), v['hashHex']);
      expect(base64Url.encode(hash).replaceAll('=', ''), v['requestHash']);
    });
  }

  test('every input of the invocation binding changes it [FN-020]', () {
    Uint8List bind({
      String function = 'a.b',
      int version = 1,
      String input = 'x',
      String jti = 'j',
    }) => invocationBinding(
      function: function,
      version: version,
      input: Uint8List.fromList(utf8.encode(input)),
      jti: jti,
    );
    final base = _hex(bind());
    for (final other in [
      bind(function: 'a.c'),
      bind(version: 2),
      bind(input: 'y'),
      bind(jti: 'k'),
    ]) {
      expect(_hex(other), isNot(base));
    }
  });
}
