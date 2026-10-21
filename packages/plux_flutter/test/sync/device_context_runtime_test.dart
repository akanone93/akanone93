// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter/material.dart' show ThemeMode;
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_flutter/src/sync/device_targeting.dart';

import '../support/harness.dart';
import 'fake_server.dart';

/// The runtime hands the sync isolate what it knows of the device and its
/// user, and reports the release it runs with what the user saw it in
/// (REL-051, SEC-142), against the fake server.
void main() {
  final g = Harness.goldens;
  final demo = g.bundles['loan-calculator/demo.pxb']!;
  final loans = g.bundles['loan-calculator/loans.pxb']!;
  late Harness h;

  setUp(() async => h = await Harness.create());
  tearDown(() => h.close());

  /// Waits, outside the fake-async zone, until [done] holds.
  Future<void> until(WidgetTester tester, bool Function() done) async {
    for (var i = 0; i < 200 && !done(); i++) {
      await tester.runAsync(
        () => Future<void>.delayed(const Duration(milliseconds: 10)),
      );
    }
    expect(done(), isTrue, reason: 'it did not happen in time');
  }

  testWidgets('a sync tells the server the device class, locale and region '
      'and only hashes of the user [REL-051]', (tester) async {
    await tester.runAsync(() async {
      await h.startFrom(demo, {'loans': loans});
      h.server.release = FakeRelease(10, demo, {'loans': loans});
      // A sync already running is joined, so the first one settles the
      // launch's and the second carries the user.
      await h.runtime.sync();
      Plux.setUserContext(
        const PluxUser(id: 'user-9', attributes: {'tier': 'gold'}),
      );
      await h.runtime.sync();
    });
    final sent = h.server.deviceTargetingSeen.last! as Map<String, Object?>;
    expect(sent['os'], isNotEmpty);
    expect(sent['runtimeVersion'], PluxRuntimeInfo.version);
    expect(sent['deviceClass'], anyOf('phone', 'tablet'));
    final locale = sent['locale']! as String;
    expect(locale, isNotEmpty);
    expect(
      sent['region'],
      locale.split('-').last,
      reason: 'the locale setting',
    );
    expect(sent['userHash'], hashUserId('user-9'));
    expect(sent['userAttributes'], {'tier': hashUserAttribute('tier', 'gold')});
    expect(sent.toString(), isNot(contains('user-9')));
    expect(sent.toString(), isNot(contains('gold')));
  });

  testWidgets('a sync of a device with no user context sends no user '
      '[REL-051]', (tester) async {
    await tester.runAsync(() async {
      await h.startFrom(demo, {'loans': loans});
      h.server.release = FakeRelease(10, demo, {'loans': loans});
      await h.runtime.sync();
    });
    final sent = h.server.deviceTargetingSeen.last! as Map<String, Object?>;
    expect(sent.containsKey('userHash'), isFalse);
    expect(sent.containsKey('userAttributes'), isFalse);
  });

  testWidgets('the release the device runs is reported with the theme, the '
      'text scale and the pseudonymous user ID [SEC-142]', (tester) async {
    tester.platformDispatcher.textScaleFactorTestValue = 1.5;
    addTearDown(tester.platformDispatcher.clearAllTestValues);
    await tester.runAsync(() async {
      await h.startFrom(demo, {'loans': loans});
      h.server.release = FakeRelease(10, demo, {'loans': loans});
      await h.runtime.sync();
    });
    await until(tester, () => h.server.installedReports.isNotEmpty);
    expect(h.server.installedReports.single, {
      'deviceId': 'dev_1',
      'releaseSequence': '10',
      'theme': 'light',
      'textScalePercent': 150,
    });

    await tester.runAsync(() async {
      Plux.setThemeMode(ThemeMode.dark);
      Plux.setUserContext(
        const PluxUser(id: 'user-9', attributes: {'tier': 'gold'}),
      );
      h.server.release = FakeRelease(11, demo, {'loans': loans});
      await h.runtime.sync();
    });
    await until(tester, () => h.server.installedReports.length == 2);
    expect(h.server.installedReports.last, {
      'deviceId': 'dev_1',
      'releaseSequence': '11',
      'theme': 'dark',
      'textScalePercent': 150,
      'userRef': 'user-9',
    });
    expect(
      h.server.installedReports.toString(),
      isNot(contains('gold')),
      reason: 'no user attribute is part of the report',
    );
  });
}
