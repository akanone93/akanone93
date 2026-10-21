// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_viewer/viewer.dart';

/// The root keys the viewer is built with [SEC-142] [SEC-051].
void main() {
  final hex = '0a' * 32;

  test('reads a keys.json with every role it lists [SEC-142]', () {
    final keys = parseRootKeys(
      '[{"keyId":"r1","algorithm":"ed25519","role":"root","publicKey":"$hex"},'
      '{"keyId":"t1","algorithm":"ed25519","role":"targets",'
      '"publicKey":"$hex"}]',
    );
    expect(keys.map((k) => (k.keyId, k.role)), [
      ('r1', 'root'),
      ('t1', 'targets'),
    ]);
    expect(keys.first.publicKey, everyElement(0x0a));
  });

  test('holds no key when the build was given none [SEC-142]', () {
    expect(parseRootKeys(''), isEmpty);
    expect(parseRootKeys('  \n'), isEmpty);
    expect(builtInRootKeys(), isEmpty);
  });

  test('fails at start on keys it cannot read, instead of opening nothing '
      'quietly [SEC-142]', () {
    for (final bad in [
      'not json',
      '{"keyId":"r1"}',
      '[{"keyId":"r1"}]',
      '[{"keyId":"r1","algorithm":"ed25519","role":"root","publicKey":"zz"}]',
    ]) {
      expect(
        () => parseRootKeys(bad),
        throwsA(
          isA<FormatException>().having(
            (e) => e.message,
            'message',
            contains(rootKeysDefine),
          ),
        ),
        reason: bad,
      );
    }
  });
}
