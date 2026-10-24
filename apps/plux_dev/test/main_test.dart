// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_dev/dev.dart';
import 'package:plux_dev/main.dart' as app;

/// The bootstrap of the Dev app [DEV-001].
void main() {
  testWidgets('[DEV-001] starts on the roots it was built with', (
    tester,
  ) async {
    app.main();
    await tester.pump();
    expect(find.byType(DevApp), findsOneWidget);
    // The test build is made without PLUX_DEV_ROOT_KEYS.
    expect(find.textContaining('No development roots configured'), findsOne);
  });
}
