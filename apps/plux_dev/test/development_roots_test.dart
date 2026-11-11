// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_dev/dev.dart';
import 'package:plux_flutter/plux_flutter.dart' show DevSignInLink;

/// The development roots the Dev app pins, and only those [DEV-001]
/// [SEC-051] [SEC-056].
void main() {
  test('holds no key when the build was given none [DEV-001]', () {
    expect(developmentRootKeys(), isEmpty);
    expect(parseDevelopmentRootKeys('  '), isEmpty);
  });

  test('reads a keys.json, and names its own define when it is wrong '
      '[DEV-001]', () {
    final keys = parseDevelopmentRootKeys(
      '[{"keyId":"d1","algorithm":"ed25519","role":"root",'
      '"publicKey":"0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a"}]',
    );
    expect(keys.single.keyId, 'd1');
    expect(developmentRootKeysDefine, 'PLUX_DEV_ROOT_KEYS');
    expect(noDevelopmentRootsMessage, contains(developmentRootKeysDefine));
    expect(
      () => parseDevelopmentRootKeys('nope'),
      throwsA(
        isA<FormatException>().having(
          (e) => e.message,
          'message',
          contains(developmentRootKeysDefine),
        ),
      ),
    );
  });

  testWidgets('without roots the app says so and never offers to connect '
      '[DEV-001]', (tester) async {
    await tester.pumpWidget(const DevApp(rootKeys: [], phones: _NoPhone()));
    await tester.pump();
    expect(find.textContaining('No development roots configured'), findsOne);
    expect(find.byKey(const Key('scan')), findsNothing);
  });

  test('the app contains no production roots and imports none [DEV-001, '
      'SEC-056]', () {
    final sources = [
      for (final f in Directory('lib').listSync(recursive: true))
        if (f is File && f.path.endsWith('.dart')) f,
    ];
    expect(sources, isNotEmpty);
    for (final f in sources) {
      final text = f.readAsStringSync();
      expect(text, isNot(contains('plux_viewer')), reason: f.path);
      expect(text, isNot(contains('production_roots')), reason: f.path);
      expect(text, isNot(contains('PLUX_VIEWER_ROOT_KEYS')), reason: f.path);
    }
    expect(
      File('pubspec.yaml').readAsStringSync(),
      isNot(contains('plux_viewer:')),
    );
  });
}

/// A directory with no phone signed in.
final class _NoPhone implements PhoneDirectory {
  const _NoPhone();

  @override
  Future<SignedInPhone?> restore() async => null;

  @override
  Future<PhoneSignIn> claim(DevSignInLink link) =>
      throw UnimplementedError('no sign-in in this test');
}
