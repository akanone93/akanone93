// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_flutter/src/control/control_snapshot.dart';
import 'package:plux_flutter/src/control/live_control.dart';
import 'package:plux_flutter/src/experiments/control_models.dart';

import '../support/harness.dart';

Uint8List bundle(String name) =>
    File('../../schema/testdata/bundles/$name').readAsBytesSync();

/// The control document of an experiment that shows [shown] to everyone.
ControlSnapshot allocation(String shown) => ControlSnapshot(
  version: 2,
  expiresAt: DateTime.utc(2030),
  experiments: [
    ControlExperiment(
      key: 'catalogueBanner',
      salt: 'catalogue-banner-1',
      variants: [
        for (final name in ['control', 'compact'])
          ControlVariant(
            name: name,
            weightBp: name == shown ? 10000 : 0,
            control: name == 'control',
          ),
      ],
    ),
  ],
);

/// The control document of the courier flag, set to [value].
ControlSnapshot courierFlag({required bool value}) => ControlSnapshot(
  version: 3,
  expiresAt: DateTime.utc(2030),
  flags: [
    ControlFlag(
      key: 'courierEnabled',
      type: FlagType.boolean,
      defaultValue: value,
    ),
  ],
);

/// A page the control document switches off.
ControlSnapshot killed(String pageId) => ControlSnapshot(
  version: 4,
  expiresAt: DateTime.utc(2030),
  switches: [ControlSwitch(SwitchKind.page, pageId)],
);

/// An engine that loads modules and never runs them: the scenarios do not
/// call a function, but the bundle of Plux Express carries one, so the
/// runtime needs an engine to accept it.
final class _Engine implements PluxFunctionEngine {
  @override
  String get name => 'fake';

  @override
  Future<Object> load({required String key, required Uint8List bytes}) async =>
      key;

  @override
  Future<PluxFunctionResult> invoke({
    required Object module,
    required int index,
    required Uint8List input,
    required PluxFunctionLimits limits,
    required PluxFunctionContext context,
    required PluxFunctionKv kv,
    PluxFunctionCancel? cancel,
  }) => throw UnimplementedError('no scenario runs a function');
}

Map<String, Object?> fieldsOf(Map<String, Object?> e) =>
    jsonDecode(utf8.decode(base64.decode(e['fields']! as String)))
        as Map<String, Object?>;

/// The P9 scenarios of the reference apps (DX-004), run on the bundles the
/// compiler writes from their fixtures (`schema/testdata/bundles/plux_*`).
void main() {
  late Harness h;
  late GlobalKey<NavigatorState> navigator;

  setUp(() async {
    h = await Harness.create();
    navigator = GlobalKey();
  });
  tearDown(() => h.close());

  Future<void> start(WidgetTester tester, String app, String plugin) async {
    await tester.runAsync(
      () => h.startFrom(
        bundle('plux_$app/$app.pxb'),
        {plugin: bundle('plux_$app/$plugin.pxb')},
        navigatorKey: navigator,
        consent: const PluxConsent(experiments: true),
        functionEngine: _Engine(),
      ),
    );
    await tester.runAsync(() => h.runtime.sync());
    await tester.pumpWidget(
      MaterialApp(
        navigatorKey: navigator,
        home: const PluxScope(child: Text('host')),
      ),
    );
    await settle(tester);
  }

  void applyControl(ControlSnapshot snapshot) =>
      h.runtime.control.value = LiveControl(snapshot: snapshot);

  Future<void> show(WidgetTester tester, String route) async {
    await tester.pumpWidget(
      MaterialApp(home: PluxScope(child: PluxView(route))),
    );
    await settle(tester);
  }

  Future<void> open(WidgetTester tester, String route) async {
    Plux.open<Object?>(navigator.currentContext!, route).ignore();
    await settle(tester);
    // The pages load data, so a spinner may never settle: pump bounded.
    for (var i = 0; i < 5; i++) {
      await tester.pump(const Duration(milliseconds: 300));
    }
  }

  Future<List<Map<String, Object?>>> exposures(WidgetTester tester) async {
    await tester.runAsync(h.runtime.flushTelemetry);
    return [
      for (final e in h.server.events)
        if (e['name'] == 'experiment_exposure') fieldsOf(e),
    ];
  }

  group('Plux Express', () {
    testWidgets('the catalogue banner shows the carousel to the control and '
        'one compact banner to the variant, and records one exposure '
        '[ABT-001] [ABT-002] [DX-004]', (tester) async {
      await start(tester, 'express', 'shop');
      applyControl(allocation('control'));
      await show(tester, 'catalogue');
      expect(find.text('Free delivery over 20 EUR'), findsOneWidget);
      expect(find.byType(PageView), findsOneWidget);
      expect(await exposures(tester), [
        {'experiment': 'catalogueBanner', 'variant': 'control'},
      ]);
    });

    testWidgets('the compact arm replaces the carousel [ABT-002] [DX-004]', (
      tester,
    ) async {
      await start(tester, 'express', 'shop');
      applyControl(allocation('compact'));
      await show(tester, 'catalogue');
      expect(find.text('Free delivery over 20 EUR'), findsOneWidget);
      expect(find.byType(PageView), findsNothing);
      expect(await exposures(tester), [
        {'experiment': 'catalogueBanner', 'variant': 'compact'},
      ]);
    });

    testWidgets('the courier route opens while the flag holds and sends the '
        'device to the catalogue once it is withdrawn, without a restart '
        '[NAV-009] [ABT-006] [DX-004]', (tester) async {
      await start(tester, 'express', 'shop');
      applyControl(courierFlag(value: true));
      await open(tester, 'courier');
      expect(find.text('Idle'), findsOneWidget);

      navigator.currentState!.pop();
      await tester.pump(const Duration(seconds: 1));
      applyControl(courierFlag(value: false));
      await open(tester, 'courier');
      expect(find.text('Idle'), findsNothing);
      expect(find.text('Catalogue'), findsWidgets);
    });
  });

  group('Plux Bank', () {
    testWidgets('switching the transfer page off shows the plugin\'s fallback '
        'page [REL-060] [RT-022] [DX-004]', (tester) async {
      await start(tester, 'bank', 'banking');
      await open(tester, 'transfer');
      expect(find.text('Transfers are paused'), findsNothing);

      navigator.currentState!.pop();
      await tester.pump(const Duration(seconds: 1));
      applyControl(killed('01d0c450-6c00-7000-8000-000000000212'));
      await open(tester, 'transfer');
      expect(find.text('Transfers are paused'), findsWidgets);
      expect(find.textContaining('Your money is safe'), findsOneWidget);
    });
  });
}
