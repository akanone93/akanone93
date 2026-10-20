// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_flutter/src/control/control_snapshot.dart';
import 'package:plux_flutter/src/control/live_control.dart';
import 'package:plux_flutter/src/experiments/control_models.dart';
import 'package:plux_flutter/src/experiments/targeting.dart';

import '../support/harness.dart';

ControlSnapshot flagOn({required bool value, TargetingRule? rule}) =>
    ControlSnapshot(
      version: value ? 2 : 3,
      expiresAt: DateTime.utc(2030),
      flags: [
        ControlFlag(
          key: 'betaEnabled',
          cases: [FlagCase(rule: rule, value: value)],
        ),
      ],
    );

/// The feature-flag guard of the routing conformance project: the `beta`
/// route opens only while `flags.betaEnabled` holds, and the flag's value
/// comes from the control document in force (NAV-009, ABT-006).
void main() {
  final g = Harness.goldens;
  late Harness h;
  late GlobalKey<NavigatorState> navigator;

  setUp(() async {
    h = await Harness.create();
    navigator = GlobalKey();
  });
  tearDown(() => h.close());

  Future<void> start(WidgetTester tester) async {
    await tester.runAsync(
      () => h.startFrom(g.bundles['routing/routing.pxb']!, {
        'nav': g.bundles['routing/nav.pxb']!,
      }, navigatorKey: navigator),
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

  Future<void> open(WidgetTester tester) async {
    Plux.open<Object?>(navigator.currentContext!, 'beta').ignore();
    await settle(tester);
    await tester.pumpAndSettle();
  }

  testWidgets('a flag guard follows the control document: a targeted flag '
      'opens the route, and withdrawing it closes it again without a '
      'restart [NAV-009] [ABT-006]', (tester) async {
    await start(tester);
    h.runtime.control.value = LiveControl(
      snapshot: flagOn(
        value: true,
        rule: const TargetingCondition('os', TargetingOperator.inList, [
          'android',
          'ios',
        ]),
      ),
    );
    await open(tester);
    expect(find.text('Beta'), findsWidgets);
    expect(Plux.flag<bool>('betaEnabled'), isTrue);

    navigator.currentState!.pop();
    await tester.pumpAndSettle();
    h.runtime.control.value = LiveControl(snapshot: flagOn(value: false));
    await open(tester);
    expect(find.text('fallback PLX-4102'), findsOneWidget);
    expect(Plux.flag<bool>('betaEnabled'), isFalse);
  });

  testWidgets('a flag whose rule does not match this device keeps the app '
      'default, so the guard refuses [NAV-009] [ABT-006]', (tester) async {
    await start(tester);
    h.runtime.control.value = LiveControl(
      snapshot: flagOn(
        value: true,
        rule: const TargetingCondition('channel', TargetingOperator.eq, 'nope'),
      ),
    );
    await open(tester);
    expect(find.text('Beta'), findsNothing);
    expect(find.text('fallback PLX-4102'), findsOneWidget);
  });
}
