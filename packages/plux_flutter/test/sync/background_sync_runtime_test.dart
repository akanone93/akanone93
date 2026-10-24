// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_flutter/src/sync/background_sync.dart';

import '../support/harness.dart';
import 'fake_server.dart';

final class _Platform implements BackgroundSyncPlatform {
  _Platform({this.failure});

  final PluxException? failure;
  final List<BackgroundSyncRequest> scheduled = [];
  int cancelled = 0;

  @override
  Future<void> schedule(BackgroundSyncRequest request) async {
    if (failure != null) throw failure!;
    scheduled.add(request);
  }

  @override
  Future<void> cancel() async => cancelled++;
}

/// The runtime asks the platform for the background task only when the app
/// opted in (SYN-014).
void main() {
  final g = Harness.goldens;
  final demo = g.bundles['loan-calculator/demo.pxb']!;
  final loans = g.bundles['loan-calculator/loans.pxb']!;
  late Harness h;

  /// What the runtime reported about background sync; the start's own sync
  /// is not under test and may report its own.
  List<PluxErrorCode> background() => [
    for (final e in h.errors)
      if (e.code.id.startsWith('PLX-350')) e.code,
  ];

  setUp(() async => h = await Harness.create());
  tearDown(() => h.close());

  Future<void> start(
    WidgetTester tester,
    BackgroundSyncConfig config,
    _Platform platform, {
    bool platformHttp = true,
  }) async {
    await tester.runAsync(() async {
      await h.startFrom(
        demo,
        {'loans': loans},
        backgroundSync: config,
        backgroundPlatform: platform,
        platformHttp: platformHttp,
      );
      // The scheduling runs beside the start.
      await Future<void>.delayed(const Duration(milliseconds: 100));
    });
  }

  testWidgets('an app that did not opt in makes no platform call [SYN-014]', (
    tester,
  ) async {
    final platform = _Platform();
    await start(tester, const BackgroundSyncConfig(), platform);
    expect(platform.scheduled, isEmpty);
    expect(platform.cancelled, 0);
    expect(background(), isEmpty);
  });

  testWidgets('an app that opted in schedules the task with what the headless '
      'run needs [SYN-014]', (tester) async {
    final platform = _Platform();
    await start(
      tester,
      const BackgroundSyncConfig(
        enabled: true,
        interval: Duration(minutes: 30),
        requiresUnmeteredNetwork: true,
      ),
      platform,
    );
    final request = platform.scheduled.single;
    expect(request.config.intervalMinutes, 30);
    expect(request.config.requiresUnmeteredNetwork, isTrue);
    final d = BackgroundSyncDescriptor.fromJson(request.descriptor);
    expect(d.appId, FakePluxServer.app);
    expect(d.storeRoot, startsWith(h.dir));
    expect(d.keys.single.keyId, h.server.keyId);
    expect(request.descriptor, isNot(contains('token')));
    expect(background(), isEmpty);
  });

  testWidgets('a refusal of the platform is reported and does not stop the '
      'start [SYN-014]', (tester) async {
    final platform = _Platform(
      failure: const PluxException(
        PluxErrorCode.backgroundSyncNotConfigured,
        'not declared',
      ),
    );
    await start(tester, const BackgroundSyncConfig.enabled(), platform);
    expect(background(), [PluxErrorCode.backgroundSyncNotConfigured]);
  });

  testWidgets('a custom HTTP client cannot run in the background [SYN-014]', (
    tester,
  ) async {
    final platform = _Platform();
    await start(
      tester,
      const BackgroundSyncConfig.enabled(),
      platform,
      platformHttp: false,
    );
    expect(platform.scheduled, isEmpty);
    expect(background(), [PluxErrorCode.backgroundSyncFailed]);
  });
}
