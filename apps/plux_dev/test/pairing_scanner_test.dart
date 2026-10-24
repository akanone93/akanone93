// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_dev/dev.dart';

/// The pairing scanner page [DEV-001]. The camera view itself
/// ([cameraScanner]) needs a device; the page around it is tested with a
/// stand-in view.
void main() {
  final key = GlobalKey<NavigatorState>();

  Future<void> pump(WidgetTester tester) => tester.pumpWidget(
    MaterialApp(navigatorKey: key, home: const Text('home')),
  );

  testWidgets('[DEV-001] returns the text of the code read', (tester) async {
    await pump(tester);
    String? result;
    var done = false;
    final future =
        scanPairingCode(
          key.currentState!,
          scannerBuilder: (onValue, onGiveUp) => TextButton(
            onPressed: () => onValue('plux-dev://pair?x'),
            child: const Text('read'),
          ),
        ).then((v) {
          result = v;
          done = true;
        });
    await tester.pumpAndSettle();
    expect(find.text('Scan the pairing code'), findsOneWidget);
    await tester.tap(find.text('read'));
    await tester.pumpAndSettle();
    await future;
    expect(done, isTrue);
    expect(result, 'plux-dev://pair?x');
    expect(find.text('home'), findsOneWidget);
  });

  testWidgets('[DEV-001] gives null when the camera fails', (tester) async {
    await pump(tester);
    String? result = 'unset';
    final future = scanPairingCode(
      key.currentState!,
      scannerBuilder: (onValue, onGiveUp) =>
          TextButton(onPressed: onGiveUp, child: const Text('fail')),
    ).then((v) => result = v);
    await tester.pumpAndSettle();
    await tester.tap(find.text('fail'));
    await tester.pumpAndSettle();
    await future;
    expect(result, isNull);
  });

  testWidgets('[DEV-001] gives null when the user backs out', (tester) async {
    await pump(tester);
    String? result = 'unset';
    final future = scanPairingCode(
      key.currentState!,
      scannerBuilder: (_, _) => const SizedBox(),
    ).then((v) => result = v);
    await tester.pumpAndSettle();
    key.currentState!.pop();
    await tester.pumpAndSettle();
    await future;
    expect(result, isNull);
  });
}
