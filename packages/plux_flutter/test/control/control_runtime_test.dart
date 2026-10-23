// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_flutter/src/control/control_poller.dart';
import 'package:plux_flutter/src/control/control_snapshot.dart';
import 'package:plux_flutter/src/control/fbs/control_controlfbs_generated.dart'
    as fbs;
import 'package:plux_flutter/src/core/active_release.dart';
import 'package:plux_flutter/src/core/runtime.dart';
import 'package:plux_flutter/src/render/page_renderer.dart';
import 'package:plux_flutter/src/state/providers.dart';
import 'package:plux_flutter/src/store/control_store.dart';
import 'package:plux_flutter/src/sync/sync_engine.dart';

import '../support/entering.dart';
import '../sync/fake_device.dart';
import '../sync/fake_server.dart';
import 'control_support.dart';

/// Renders a page as its route and the release it comes from.
final class _TestRenderer with AllowsEveryGuard implements PageRenderer {
  @override
  Widget build(
    BuildContext context,
    ActiveRelease release,
    PageRef page,
    Map<String, Object?> params, {
    bool routed = false,
    void Function(Object? result)? onPop,
    void Function(String event, Object? payload)? onEvent,
  }) => Text(
    'page ${page.route} of ${release.sequence}',
    textDirection: TextDirection.ltr,
  );
}

/// A periodic timer a test fires by hand.
final class _ManualTimer implements Timer {
  _ManualTimer(this.period, this._tick);

  final Duration period;
  final void Function() _tick;
  bool cancelled = false;

  void fire() => _tick();

  @override
  void cancel() => cancelled = true;

  @override
  bool get isActive => !cancelled;

  @override
  int get tick => 0;
}

void main() {
  final goldens = Goldens.load();
  final demo = goldens.bundles['loan-calculator/demo.pxb']!;
  final loans = goldens.bundles['loan-calculator/loans.pxb']!;

  late FakePluxServer server;
  late ControlFixture control;
  late String dir;
  late DateTime now;
  late List<_ManualTimer> timers;
  final errors = <PluxException>[];

  setUp(() async {
    server = await FakePluxServer.start();
    control = await ControlFixture.create();
    dir = Directory.systemTemp.createTempSync('plux_control_rt').path;
    now = DateTime.utc(2026, 10, 20, 12);
    timers = [];
    errors.clear();
  });
  tearDown(() async {
    await Plux.dispose();
    await server.close();
    Directory(dir).deleteSync(recursive: true);
  });

  String storePath() => '$dir/${FakePluxServer.app}/production/production';

  PluxConfig config({PluxUpdateScreenBuilder? updateScreen}) => PluxConfig(
    appId: FakePluxServer.app,
    endpoint: server.endpoint,
    rootKeys: [
      PluxPublicKey(
        keyId: server.keyId,
        algorithm: 'ed25519',
        role: 'targets',
        publicKey: server.publicKey,
      ),
      PluxPublicKey(
        keyId: control.keyId,
        algorithm: 'ed25519',
        role: 'control',
        publicKey: control.publicKey,
      ),
    ],
    storageDirectory: dir,
    httpClient: http.Client.new,
    onError: (e, _) => errors.add(e),
    fallbackBuilder: (_, e) =>
        Text('fallback ${e.code.id}', textDirection: TextDirection.ltr),
    updateScreen: updateScreen,
  );

  PluxRuntime rt() => Plux.container.read(pluxRuntimeProvider)!;

  Future<void> start({PluxUpdateScreenBuilder? updateScreen}) async {
    await Plux.initializeWith(
      config(updateScreen: updateScreen),
      RuntimeOverrides(
        credentials: MemoryCredentialStore.new,
        configSecrets: MemorySecretStore.new,
        deviceKeys: FakeDeviceKeys.new,
        attestation: FakeAttestation.new,
        clock: () => now,
        healthyAfter: const Duration(hours: 1),
        controlTimer: (period, tick) {
          final t = _ManualTimer(period, tick);
          timers.add(t);
          return t;
        },
      ),
    );
    rt().renderer = _TestRenderer();
  }

  /// Signs and serves document [version] of the test app's channel.
  Future<void> publish(
    int version, {
    List<fbs.KillSwitchObjectBuilder> switches = const [],
    fbs.MandatoryUpdateObjectBuilder? mandatory,
    fbs.ForcedRollbackObjectBuilder? rollback,
    ControlFixture? signer,
    void Function(Map<String, Object?>)? afterSigning,
  }) async {
    final expires = DateTime.utc(2100);
    server.controlEtag = '"c$version"';
    server.controlFile = await control.file(
      control.document(
        version: version,
        app: FakePluxServer.app,
        channel: 'production',
        expires: expires,
        switches: switches,
        mandatory: mandatory,
        rollback: rollback,
      ),
      version: version,
      app: FakePluxServer.app,
      channel: 'production',
      expires: expires,
      signer: signer,
      afterSigning: afterSigning,
    );
  }

  Future<void> check(WidgetTester tester) async {
    await tester.runAsync(() => rt().checkControl());
    await tester.pump();
  }

  /// Lets the sync isolate answer: its replies resume code in the test's
  /// fake-async zone, which runs only when the tester pumps. The isolate
  /// verifies a signed document in real time, which on a loaded machine
  /// took longer than three seconds, so it may take up to twenty.
  Future<void> settle(WidgetTester tester, bool Function() done) async {
    for (var i = 0; i < 2000 && !done(); i++) {
      await tester.runAsync(
        () => Future<void>.delayed(const Duration(milliseconds: 10)),
      );
      await tester.pump();
    }
    expect(done(), isTrue);
  }

  fbs.KillSwitchObjectBuilder off(fbs.SwitchScope scope, String target) =>
      killSwitch(scope, target);

  // The default update screen is made for an app: it needs the text
  // direction a MaterialApp gives.
  const view = Directionality(
    textDirection: TextDirection.ltr,
    child: PluxScope(child: PluxView('result')),
  );

  testWidgets('a plugin switch shows the fallback without a restart, and '
      'lifting it brings the page back [REL-060] [RT-022] [SYN-060]', (
    tester,
  ) async {
    await tester.runAsync(() async {
      server.release = FakeRelease(10, demo, {'loans': loans});
      await start();
    });
    await tester.pumpWidget(view);
    expect(find.text('page result of 10'), findsOneWidget);

    await publish(1, switches: [off(fbs.SwitchScope.Plugin, 'loans')]);
    await check(tester);
    expect(find.text('fallback PLX-4020'), findsOneWidget);
    expect(find.text('page result of 10'), findsNothing);

    await publish(2);
    await check(tester);
    expect(find.text('page result of 10'), findsOneWidget);
  });

  testWidgets('a page switch shows the fallback of that page only '
      '[REL-060] [RT-022]', (tester) async {
    await tester.runAsync(() async {
      server.release = FakeRelease(10, demo, {'loans': loans});
      await start();
    });
    await tester.pumpWidget(view);
    final page = rt().active.value!.page('result')!;

    await publish(1, switches: [off(fbs.SwitchScope.Page, page.id)]);
    await check(tester);
    expect(find.text('fallback PLX-4020'), findsOneWidget);

    await publish(2, switches: [off(fbs.SwitchScope.Page, 'not-this-page')]);
    await check(tester);
    expect(find.text('page result of 10'), findsOneWidget);
  });

  testWidgets('the other scopes reach the release the pages and runs ask '
      '[REL-060]', (tester) async {
    await tester.runAsync(() async {
      server.release = FakeRelease(10, demo, {'loans': loans});
      await start();
    });
    await publish(
      1,
      switches: [
        off(fbs.SwitchScope.ActionGraph, 'graph-1'),
        off(fbs.SwitchScope.DataSource, 'source-1'),
        off(fbs.SwitchScope.$Function, 'orders.total'),
      ],
    );
    await check(tester);
    final release = rt().active.value!;
    expect(release.graphDisabled('graph-1'), isTrue);
    expect(release.graphDisabled('graph-2'), isFalse);
    expect(release.dataSourceDisabled('source-1'), isTrue);
    expect(release.functionDisabled('orders.total'), isTrue);
    expect(release.functionDisabled('orders.count'), isFalse);
  });

  testWidgets('a document that was verified before keeps its switches in '
      'force at launch, also past its expiry; PLX-3101 is reported once '
      '[SYN-060]', (tester) async {
    await tester.runAsync(() async {
      server.release = FakeRelease(10, demo, {'loans': loans});
      await start();
      await Plux.dispose();
      ControlDocumentStore(storePath()).write(
        StoredControl(
          etag: '"c1"',
          snapshot: ControlSnapshot(
            version: 1,
            expiresAt: DateTime.utc(2020),
            switches: const [ControlSwitch(SwitchKind.plugin, 'loans')],
            mandatory: const MandatoryUpdate(
              releaseId: 'rel-99',
              releaseSequence: 99,
              messageKey: 'update.now',
            ),
          ),
        ),
      );
      await start();
    });
    await tester.pumpWidget(view);
    expect(find.text('fallback PLX-4020'), findsOneWidget);
    expect(
      find.byType(PluxDefaultUpdateScreen),
      findsNothing,
      reason: 'an expired document holds no page back, only its switches',
    );

    await check(tester);
    await check(tester);
    expect(
      errors.where((e) => e.code == PluxErrorCode.controlDocumentExpired),
      hasLength(1),
    );
    expect(find.text('fallback PLX-4020'), findsOneWidget);
  });

  testWidgets('a tampered document is refused with PLX-3100, once, and '
      'changes nothing [SYN-060] [SEC-052]', (tester) async {
    await tester.runAsync(() async {
      server.release = FakeRelease(10, demo, {'loans': loans});
      await start();
    });
    await tester.pumpWidget(view);
    await publish(
      1,
      switches: [off(fbs.SwitchScope.Plugin, 'loans')],
      afterSigning: (signed) => signed['version'] = 5,
    );
    await check(tester);
    await check(tester);
    expect(find.text('page result of 10'), findsOneWidget);
    expect(
      errors.where((e) => e.code == PluxErrorCode.controlSignatureInvalid),
      hasLength(1),
    );

    await publish(1, signer: await ControlFixture.create());
    await check(tester);
    expect(find.text('page result of 10'), findsOneWidget);
  });

  group('mandatory update', () {
    Future<void> release11(WidgetTester tester) async {
      server.release = FakeRelease(11, demo, {'loans': loans});
      await publish(
        1,
        mandatory: fbs.MandatoryUpdateObjectBuilder(
          releaseId: 'rel-11',
          releaseSequence: 11,
          messageKey: 'update.now',
        ),
      );
    }

    testWidgets('shows the update screen in place of the page, activates '
        'the release, and shows the page again [REL-070]', (tester) async {
      await tester.runAsync(() async {
        server.release = FakeRelease(10, demo, {'loans': loans});
        await start();
      });
      await tester.pumpWidget(view);
      expect(find.text('page result of 10'), findsOneWidget);

      await release11(tester);
      await tester.runAsync(() => rt().checkControl());
      await tester.pump();
      expect(find.byType(PluxDefaultUpdateScreen), findsOneWidget);
      expect(find.text('page result of 10'), findsNothing);
      expect(find.text('An update is required to continue'), findsOneWidget);
      expect(find.text('update.now'), findsOneWidget);
      expect(
        errors.where((e) => e.code == PluxErrorCode.mandatoryUpdatePending),
        hasLength(1),
      );

      await settle(tester, () => rt().active.value?.sequence == 11);
      await settle(
        tester,
        () => find.text('page result of 11').evaluate().isNotEmpty,
      );
      expect(find.byType(PluxDefaultUpdateScreen), findsNothing);
    });

    testWidgets('shows the host\'s update screen when it supplies one '
        '[REL-070]', (tester) async {
      await tester.runAsync(() async {
        server.release = FakeRelease(10, demo, {'loans': loans});
        await start(
          updateScreen: (_, key) =>
              Text('custom $key', textDirection: TextDirection.ltr),
        );
      });
      await tester.pumpWidget(view);
      await release11(tester);
      await tester.runAsync(() => rt().checkControl());
      await tester.pump();
      expect(find.text('custom update.now'), findsOneWidget);
      expect(find.byType(PluxDefaultUpdateScreen), findsNothing);
      await settle(tester, () => rt().active.value?.sequence == 11);
    });

    testWidgets('an update already satisfied holds nothing back [REL-070]', (
      tester,
    ) async {
      await tester.runAsync(() async {
        server.release = FakeRelease(10, demo, {'loans': loans});
        await start();
      });
      await tester.pumpWidget(view);
      await publish(
        1,
        mandatory: fbs.MandatoryUpdateObjectBuilder(
          releaseId: 'rel-10',
          releaseSequence: 10,
          messageKey: 'update.now',
        ),
      );
      await check(tester);
      expect(find.text('page result of 10'), findsOneWidget);
      expect(find.byType(PluxDefaultUpdateScreen), findsNothing);
    });
  });

  testWidgets('a forced rollback activates the retained release, reports '
      'PLX-3104 and emits control_applied [REL-060] [SYN-060]', (tester) async {
    await tester.runAsync(() async {
      server.release = FakeRelease(10, demo, {'loans': loans});
      await start();
      server.release = FakeRelease(11, demo, {'loans': loans});
      await rt().sync();
    });
    await settle(tester, () => rt().active.value?.sequence == 11);
    await tester.pumpWidget(view);
    expect(find.text('page result of 11'), findsOneWidget);

    await publish(
      3,
      switches: [off(fbs.SwitchScope.Plugin, 'cards')],
      rollback: fbs.ForcedRollbackObjectBuilder(
        releaseId: 'rel-10',
        releaseSequence: 10,
      ),
    );
    await check(tester);
    await settle(tester, () => rt().active.value?.sequence == 10);
    expect(find.text('page result of 10'), findsOneWidget);
    expect(
      errors.where((e) => e.code == PluxErrorCode.forcedRollbackApplied),
      hasLength(1),
    );

    await tester.runAsync(() => rt().flushTelemetry());
    final applied = server.events.singleWhere(
      (e) => e['name'] == 'control_applied',
    );
    final fields = jsonDecode(
      utf8.decode(base64.decode(applied['fields']! as String)),
    ) as Map<String, Object?>;
    expect(fields, {
      'version': 3,
      'switches': 1,
      'switches_changed': 1,
      'outcome': 'rolled_back',
      'rolled_back_to': 10,
    });
  });

  group('when it asks', () {
    Future<void> started(WidgetTester tester) async {
      await tester.runAsync(() async {
        server.release = FakeRelease(10, demo, {'loans': loans});
        await publish(1);
        await start();
      });
      await settle(tester, () => server.controlAsked.isNotEmpty);
    }

    testWidgets('a silent push with the Plux marker asks at once; anything '
        'else is ignored [SYN-060]', (tester) async {
      await started(tester);
      final before = server.controlAsked.length;
      final ignored = await tester.runAsync(
        () async => [
          await Plux.handleSilentPush({}),
          await Plux.handleSilentPush({'plux': 'other'}),
          await Plux.handleSilentPush({'plux': '{"route":"home"}'}),
          await Plux.handleSilentPush({'plux': 'control', 'app': 'another'}),
          await Plux.handleSilentPush({'aps': <String, Object?>{}}),
        ],
      );
      expect(ignored, everyElement(isFalse));
      expect(server.controlAsked, hasLength(before));

      await publish(2);
      final ran = await tester.runAsync(
        () => Plux.handleSilentPush({
          'plux': 'control',
          'app': FakePluxServer.app,
        }),
      );
      expect(ran, isTrue);
      expect(server.controlAsked, hasLength(before + 1));
      expect(server.controlAsked.last, '"c1"', reason: 'If-None-Match');
    });

    testWidgets('on launch and on resume, and every five minutes in the '
        'foreground only; a 304 changes nothing [SYN-060]', (tester) async {
      await started(tester);
      final timer = timers.single;
      expect(timer.period, const Duration(minutes: 5));
      expect(timer.isActive, isTrue);
      expect(server.controlAsked, ['']);

      timer.fire();
      await settle(tester, () => server.controlAsked.length == 2);
      expect(server.controlAsked.last, '"c1"');
      expect(rt().control.value.snapshot!.version, 1);
      await settle(tester, () => rt().controlCheckInFlight == null);

      rt().didChangeAppLifecycleState(AppLifecycleState.paused);
      expect(timer.cancelled, isTrue, reason: 'none in the background');

      await publish(2);
      rt().didChangeAppLifecycleState(AppLifecycleState.resumed);
      await settle(tester, () => server.controlAsked.length == 3);
      expect(timers, hasLength(2));
      expect(timers.last.isActive, isTrue);
      await settle(tester, () => rt().control.value.snapshot!.version == 2);
    });
  });

  group('the poller', () {
    test('joins a check already running, and never ticks after pause '
        '[SYN-060]', () async {
      final started = Completer<void>();
      var checks = 0;
      final timers = <_ManualTimer>[];
      final poller = ControlPoller(
        check: () async {
          checks++;
          await started.future;
        },
        timer: (period, tick) {
          final t = _ManualTimer(period, tick);
          timers.add(t);
          return t;
        },
      );
      poller.resume();
      poller.resume();
      expect(
        timers,
        hasLength(1),
        reason: 'one timer however often it resumes',
      );
      unawaited(poller.checkNow());
      expect(checks, 1, reason: 'a burst of triggers costs one request');
      started.complete();
      await poller.checkNow();
      poller.pause();
      expect(poller.ticking, isFalse);
      expect(timers.single.cancelled, isTrue);
      poller.close();
      await poller.checkNow();
      expect(checks, 1);
    });

    test('recognises the marker of the server\'s push [SYN-060]', () {
      expect(isControlPush({'plux': 'control'}, 'a'), isTrue);
      expect(isControlPush({'plux': 'control', 'app': 'a'}, 'a'), isTrue);
      expect(isControlPush({'plux': 'control', 'app': 'b'}, 'a'), isFalse);
      expect(isControlPush({'plux': 'x'}, 'a'), isFalse);
      expect(isControlPush({'plux': 1}, 'a'), isFalse);
      expect(isControlPush(const {}, 'a'), isFalse);
    });
  });
}
