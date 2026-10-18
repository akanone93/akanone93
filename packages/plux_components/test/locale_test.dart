// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_components/plux_components.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_flutter/src/l10n/locale_controller.dart';

import '../../plux_flutter/test/support/harness.dart';
import '../../plux_flutter/test/support/locale_bundle.dart';

/// The runtime switching the locale of a rendered `money` gallery page
/// (I18N-004, ACT-020).
void main() {
  final g = Harness.goldens;
  late Harness h;
  setUp(() async {
    h = await Harness.create();
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(SystemChannels.platform, (_) async => null);
  });
  tearDown(() {
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(SystemChannels.platform, null);
    return h.close();
  });

  Future<void> switchTo(WidgetTester tester, Locale locale) async {
    await tester.runAsync(() async {
      Plux.setLocale(locale);
      await Plux.container.read(localeControllerProvider.notifier).settled();
    });
    await settle(tester);
  }

  testWidgets("a Layer 2 component's own words come from the runtime strings "
      'of the locale [I18N-004] [ACT-020]', (tester) async {
    final handle = tester.ensureSemantics();
    tester.view.physicalSize = const Size(800, 3000);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    await tester.runAsync(() async {
      final app = withLocales(
        g.bundles['widgets/widgets.pxb']!,
        const {},
        plux: {
          'de': {'price.was': 'statt'},
        },
        verbatim: {'de'},
      );
      await h.startFrom(
        app,
        {'gallery': g.bundles['widgets/gallery.pxb']!},
        widgetPackages: const [PluxComponents()],
      );
      await h.loadIconFonts();
    });
    await tester.pumpWidget(
      MaterialApp(
        debugShowCheckedModeBanner: false,
        home: PluxScope(child: PluxView('money')),
      ),
    );
    await settle(tester);
    await switchTo(tester, const Locale('de'));
    expect(
      find.bySemanticsLabel(RegExp('^19,99 Euro, statt 29,99 Euro')),
      findsOneWidget,
    );
    await switchTo(tester, const Locale('en'));
    expect(
      find.bySemanticsLabel(RegExp(r'19\.99.*, was .*29\.99')),
      findsOneWidget,
    );
    handle.dispose();
  });
}
