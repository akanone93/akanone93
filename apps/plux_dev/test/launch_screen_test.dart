// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:go_router/go_router.dart';
import 'package:plux_dev/src/run_screen.dart';
import 'package:plux_flutter/plux_flutter.dart';

Widget _launch(ValueNotifier<PluxException?> refused) => MaterialApp.router(
  routerConfig: GoRouter(
    routes: [
      GoRoute(
        path: '/',
        builder: (context, _) => DevLaunchScreen(
          appName: 'Apply Register2',
          entryRoute: ValueNotifier<String?>(null),
          refused: refused,
          onLeave: () {},
        ),
      ),
    ],
  ),
);

void main() {
  // The phone refused the dev bundle (it needed pxl.format.v1, which the
  // runtime did not declare) and the screen said only "no entry screen yet"
  // after 20 seconds (build log #247, #257).
  testWidgets(
    'a refused dev bundle is said at once, with its reason [DEV-001, BND-008]',
    (tester) async {
      final refused = ValueNotifier<PluxException?>(null);
      await tester.pumpWidget(_launch(refused));
      expect(find.text('Opening Apply Register2…'), findsOneWidget);

      refused.value = PluxException(
        PluxErrorCode.unsupportedRequiredFeature,
        'the dev bundle requires pxl.format.v1',
      );
      await tester.pump();
      expect(find.text('Apply Register2 could not open'), findsOneWidget);
      expect(
        find.textContaining('the dev bundle requires pxl.format.v1'),
        findsOneWidget,
      );
      expect(find.textContaining('Update the Dev app'), findsOneWidget);
      expect(find.text('Opening Apply Register2…'), findsNothing);

      refused.value = null;
      await tester.pump();
      expect(find.text('Opening Apply Register2…'), findsOneWidget);
      await tester.pump(entryWait);
    },
  );
}
