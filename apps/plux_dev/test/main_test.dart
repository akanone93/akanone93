// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_dev/dev.dart';
import 'package:plux_dev/main.dart' as app;

/// The bootstrap of the Dev app [DEV-001].
void main() {
  testWidgets('[DEV-001] starts with no key of its own (ADR-0109)', (
    tester,
  ) async {
    // main asks the native side for the phone's name first: real async.
    await tester.runAsync(app.main);
    await tester.pump();
    expect(find.byType(DevApp), findsOneWidget);
    // Nothing waits for a key at build time: the phone's connection brings
    // the organisation's root, and each join its app's key.
    expect(find.textContaining('No development roots'), findsNothing);
  });
}
