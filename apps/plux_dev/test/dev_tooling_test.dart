// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_dev/dev.dart';
import 'package:plux_security/plux_security.dart';

/// The Dev app's tools [DEV-001] [DEV-040].
void main() {
  late GlobalKey<NavigatorState> key;
  setUp(() => key = GlobalKey<NavigatorState>());

  test('[DEV-040] wires the dev extensions into the session', () async {
    final tooling = DevTooling(rootKeys: const [], navigatorKey: key);
    expect(tooling.session.extensions, isNotEmpty);
    await tooling.dispose();
  });

  test(
    '[DEV-040] attachSecurity reports detections and simulates one',
    () async {
      final tooling = DevTooling(rootKeys: const [], navigatorKey: key);
      final security = PluxSecurity(
        mode: PluxSecurityMode.developmentReportOnly,
        environment: devEnvironment,
      );
      tooling.attachSecurity(security);
      expect(security.devEventSink, same(tooling.session.events));
      expect(
        tooling.session.extensions.containsKey('simulateSecurityDetection'),
        isTrue,
      );
      await tooling.dispose();
    },
  );

  testWidgets('[DEV-001] scan is null without a navigator', (tester) async {
    final tooling = DevTooling(
      rootKeys: const [],
      navigatorKey: key,
      scan: (_) async => 'never',
    );
    late BuildContext context;
    await tester.pumpWidget(
      Builder(
        builder: (c) {
          context = c;
          return const SizedBox();
        },
      ),
    );
    expect(await tooling.scan(context), isNull);
    await tooling.dispose();
  });

  testWidgets('[DEV-001] scan goes through the navigator', (tester) async {
    NavigatorState? seen;
    final tooling = DevTooling(
      rootKeys: const [],
      navigatorKey: key,
      scan: (navigator) async {
        seen = navigator;
        return 'code';
      },
    );
    late BuildContext context;
    await tester.pumpWidget(
      MaterialApp(
        navigatorKey: key,
        home: Builder(
          builder: (c) {
            context = c;
            return const SizedBox();
          },
        ),
      ),
    );
    expect(await tooling.scan(context), 'code');
    expect(seen, key.currentState);
    await tooling.dispose();
  });

  testWidgets('[DEV-001] wrap puts the app under the tools', (tester) async {
    final tooling = DevTooling(rootKeys: const [], navigatorKey: key);
    await tester.pumpWidget(
      MaterialApp(navigatorKey: key, home: tooling.wrap(const Text('app'))),
    );
    expect(find.text('app'), findsOneWidget);
    await tester.pumpWidget(const SizedBox());
    await tooling.dispose();
  });
}
