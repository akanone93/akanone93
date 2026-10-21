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
import 'package:plux_flutter/src/render/exposure_probe.dart';

import '../support/harness.dart';

Uint8List bundle(String name) =>
    File('../../schema/testdata/bundles/experiments/$name').readAsBytesSync();

ControlExperiment experiment(
  String key,
  Map<String, Map<String, Object?>> variants, {
  required String shown,
}) => ControlExperiment(
  key: key,
  salt: '$key-1',
  variants: [
    for (final MapEntry(key: name, value: props) in variants.entries)
      ControlVariant(name: name, weight: name == shown ? 100 : 0, props: props),
  ],
);

ControlSnapshot control(List<ControlExperiment> experiments) => ControlSnapshot(
  version: 2,
  expiresAt: DateTime.utc(2030),
  experiments: experiments,
);

Map<String, Object?> fieldsOf(Map<String, Object?> e) =>
    jsonDecode(utf8.decode(base64.decode(e['fields']! as String)))
        as Map<String, Object?>;

/// The `experiments` conformance project (schema/testdata/documents/
/// experiments): a page with a `Variant` node and prop experiments, as the
/// Go compiler wrote it.
void main() {
  late Harness h;
  setUp(() async => h = await Harness.create());
  tearDown(() => h.close());

  Future<void> start(WidgetTester tester, {required bool consent}) async {
    await tester.runAsync(
      () => h.startFrom(bundle('experiments.pxb'), {
        'shop': bundle('shop.pxb'),
      }, consent: PluxConsent(experiments: consent)),
    );
  }

  Future<void> show(WidgetTester tester) async {
    await tester.pumpWidget(
      const MaterialApp(home: PluxScope(child: PluxView('home'))),
    );
    await settle(tester);
  }

  void applyControl(ControlSnapshot? snapshot) =>
      h.runtime.control.value = LiveControl(snapshot: snapshot);

  Future<List<Map<String, Object?>>> exposures(WidgetTester tester) async {
    await tester.runAsync(h.runtime.flushTelemetry);
    return [
      for (final e in h.server.events)
        if (e['name'] == 'experiment_exposure') fieldsOf(e),
    ];
  }

  testWidgets('a Variant node builds the arm of the assigned variant and '
      'records one exposure; prop experiments read experiment.<key>.<prop> '
      '[ABT-002] [ABT-001]', (tester) async {
    await start(tester, consent: true);
    applyControl(
      control([
        experiment('heroLayout', {'control': {}, 'wide': {}}, shown: 'wide'),
        experiment('ctaLabel', {
          'control': {'label': 'Buy'},
          'bold': {'label': 'Buy now'},
        }, shown: 'bold'),
      ]),
    );
    await show(tester);
    expect(find.text('Welcome back, we missed you'), findsOneWidget);
    expect(find.text('Welcome'), findsNothing);
    expect(find.text('Buy now'), findsOneWidget);
    expect(find.text('bold'), findsOneWidget);
    expect(await exposures(tester), [
      {'experiment': 'heroLayout', 'variant': 'wide'},
    ]);
    // Rebuilding the page does not expose again.
    applyControl(h.runtime.control.value.snapshot);
    await show(tester);
    await tester.pump();
    expect(await exposures(tester), hasLength(1));
  });

  testWidgets('assignment alone records no exposure: until the page builds '
      'the arm, nothing is sent [ABT-002]', (tester) async {
    await start(tester, consent: true);
    applyControl(
      control([
        experiment('heroLayout', {'control': {}, 'wide': {}}, shown: 'wide'),
      ]),
    );
    expect(h.runtime.experiments.variantOf('heroLayout'), 'wide');
    expect(h.runtime.experiments.root(), containsPair('heroLayout', 'wide'));
    expect(await exposures(tester), isEmpty);
    await show(tester);
    expect(await exposures(tester), hasLength(1));
  });

  testWidgets('a device the control document does not name gets the control '
      'arm and no exposure [ABT-002]', (tester) async {
    await start(tester, consent: true);
    // Only the prop experiment runs; the Variant's experiment is absent.
    applyControl(
      control([
        experiment('ctaLabel', {
          'control': {'label': 'Buy'},
          'bold': {'label': 'Buy now'},
        }, shown: 'bold'),
      ]),
    );
    await show(tester);
    expect(find.text('Welcome'), findsOneWidget);
    expect(find.text('Welcome back, we missed you'), findsNothing);
    expect(await exposures(tester), isEmpty);
  });

  testWidgets('with no control document a prop experiment reads the control '
      'value the compiler embedded in the app bundle [ABT-002]', (
    tester,
  ) async {
    await start(tester, consent: true);
    applyControl(null);
    await show(tester);
    expect(find.text('Buy'), findsOneWidget);
    expect(find.text('Buy now'), findsNothing);
    expect(find.text('Welcome'), findsOneWidget);
    expect(await exposures(tester), isEmpty);
  });

  testWidgets('without the experiments consent everyone gets the control '
      'and nothing is recorded [ABT-002] [SEC-161]', (tester) async {
    await start(tester, consent: false);
    applyControl(
      control([
        experiment('heroLayout', {'control': {}, 'wide': {}}, shown: 'wide'),
        experiment('ctaLabel', {
          'control': {'label': 'Buy'},
          'bold': {'label': 'Buy now'},
        }, shown: 'bold'),
      ]),
    );
    await show(tester);
    expect(find.text('Welcome'), findsOneWidget);
    expect(find.text('Buy'), findsOneWidget);
    expect(find.text('other'), findsOneWidget);
    expect(await exposures(tester), isEmpty);
  });

  testWidgets('a Variant inside an unvisited branch records nothing '
      '[ABT-002]', (tester) async {
    final shown = <String>[];
    // A list builds only the rows near the viewport.
    await tester.pumpWidget(
      MaterialApp(
        home: SizedBox(
          height: 200,
          child: ListView.builder(
            itemCount: 50,
            itemExtent: 100,
            itemBuilder: (_, i) => ExposureProbe(
              onShown: () => shown.add('row $i'),
              child: Text('row $i'),
            ),
          ),
        ),
      ),
    );
    expect(shown, contains('row 0'));
    expect(shown.length, lessThan(15));
    expect(shown, isNot(contains('row 40')));
    // Scrolling to it mounts it, and only then is it shown.
    await tester.drag(find.byType(ListView), const Offset(0, -4000));
    await tester.pump();
    expect(shown, contains('row 40'));
    // Rebuilding a mounted row does not call the probe again.
    final count = shown.length;
    await tester.pump();
    expect(shown, hasLength(count));
  });
}
