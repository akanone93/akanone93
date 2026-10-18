// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_components/plux_components.dart';
import 'package:plux_flutter/plux_flutter.dart';

import '../../plux_flutter/test/support/a11y.dart';
import '../../plux_flutter/test/support/harness.dart';

/// Every gallery page of this package's components meets Flutter's
/// accessibility guidelines in light and dark (QA-011, A11Y-004); the
/// runtime's own pages are tested in `plux_flutter`.
void main() {
  const pages = [
    'animation',
    'components-a',
    'components-b',
    'cupertino-more',
    'money',
  ];
  final g = Harness.goldens;
  late Harness h;
  setUp(() async => h = await Harness.create());
  tearDown(() => h.close());

  for (final route in pages) {
    for (final dark in [false, true]) {
      testWidgets('gallery page $route meets the guidelines in '
          '${dark ? 'dark' : 'light'} [QA-011] [A11Y-004]', (tester) async {
        tester.view.physicalSize = const Size(800, 1400);
        tester.view.devicePixelRatio = 1;
        addTearDown(tester.view.reset);
        await tester.runAsync(() async {
          await h.startFrom(
            g.bundles['widgets/widgets.pxb']!,
            {'gallery': g.bundles['widgets/gallery.pxb']!},
            themeSource: PluxThemeSource.plux,
            widgetPackages: const [PluxComponents()],
          );
          await h.loadIconFonts();
        });
        await tester.pumpWidget(
          MaterialApp(
            debugShowCheckedModeBanner: false,
            home: PluxScope(child: PluxView(route)),
          ),
        );
        await settle(tester);
        if (dark) {
          Plux.setThemeMode(ThemeMode.dark);
          await settle(tester);
          for (var i = 0; i < 3; i++) {
            await tester.pump(const Duration(milliseconds: 300));
          }
        }
        // The sync that finds no server release, the pages' uploader and
        // static data source (kinds the test runtime does not load) and the
        // images no test server serves are expected reports, not findings.
        expect([
          for (final e in h.errors)
            if (e.code != PluxErrorCode.syncFailed &&
                e.code != PluxErrorCode.devicePackageMissing &&
                e.code != PluxErrorCode.dataSourceUnavailable &&
                !e.message.contains('an image failed to load'))
              e,
        ], isEmpty);
        expect(find.textContaining('fallback PLX'), findsNothing);
        expect(find.byType(PluxView), findsOneWidget);
        expect(find.byType(Scaffold), findsWidgets);
        await expectMeetsA11yGuidelines(tester);
      });
    }
  }
}
