// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_dev/dev.dart';

/// What the Dev app trusts: no key of its own, development keys only, and
/// no production root [DEV-001] [DEV-003] [SEC-056].
void main() {
  test("is built with no key: each app's comes with its join [DEV-003, "
      'ADR-0109]', () {
    // No build setting carries a key any more; the phone pins its
    // organisation's development root when it is connected.
    for (final f in [
      for (final f in Directory('lib').listSync(recursive: true))
        if (f is File && f.path.endsWith('.dart')) f,
    ]) {
      expect(
        f.readAsStringSync(),
        isNot(contains('PLUX_DEV_ROOT_KEYS')),
        reason: f.path,
      );
    }
    expect(devEnvironment, 'development');
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
