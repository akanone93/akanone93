// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_components/plux_components.dart';
import 'package:plux_flutter/plux_flutter.dart';

import '../../plux_flutter/test/support/harness.dart';

/// The Layer 1 widgets this package builds render on the gallery pages that
/// show them, with the package registered and without it (RT-060). The
/// pages' behaviour is tested in `plux_flutter` (`scrolling_test.dart`,
/// `navigation_test.dart`, `cupertino_more_test.dart`,
/// `animation_test.dart`), which registers the package in its harness.
void main() {
  const pages = ['animation', 'cupertino-more', 'navigation', 'scrolling'];
  final g = Harness.goldens;
  late Harness h;
  setUp(() async => h = await Harness.create());
  tearDown(() => h.close());

  Future<void> open(
    WidgetTester tester,
    String route,
    List<PluxWidgetPackage> packages,
  ) async {
    tester.view.physicalSize = const Size(800, 3000);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    await tester.runAsync(() async {
      await h.startFrom(
        g.bundles['widgets/widgets.pxb']!,
        {'gallery': g.bundles['widgets/gallery.pxb']!},
        themeSource: PluxThemeSource.plux,
        widgetPackages: packages,
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
  }

  for (final route in pages) {
    // Verifies: RT-060.
    testWidgets('gallery page $route renders with the package [RT-060]', (
      tester,
    ) async {
      await open(tester, route, const [PluxComponents()]);
      expect([
        for (final e in h.errors)
          if (e.code != PluxErrorCode.syncFailed &&
              e.code != PluxErrorCode.dataSourceUnavailable &&
              !e.message.contains('an image failed to load'))
            e,
      ], isEmpty);
      expect(find.textContaining('fallback PLX'), findsNothing);
      expect(find.byType(Scaffold), findsWidgets);
    });

    // Verifies: RT-060.
    testWidgets('gallery page $route reports the missing package [RT-060]', (
      tester,
    ) async {
      await open(tester, route, const []);
      expect(
        h.errors.where((e) => e.code == PluxErrorCode.widgetPackageMissing),
        isNotEmpty,
      );
    });
  }
}
