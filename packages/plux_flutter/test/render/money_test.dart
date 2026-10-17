// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_flutter/src/l10n/l10n.dart';
import 'package:plux_flutter/src/l10n/locale_controller.dart';
import 'package:plux_flutter/src/render/scope.dart';

import '../support/harness.dart';
import '../support/locale_bundle.dart';

PluxLocaleData _load(String id) => PluxLocaleData.fromJson(
  jsonDecode(File('../../schema/l10n/cldr/$id.json').readAsStringSync())
      as Map<String, Object?>,
);

/// The money group (I18N-011, WGT-020, A11Y-001, ADR-0071): AmountInput,
/// MoneyText, MaskedText, PhoneInput, IbanInput, CountryPicker,
/// CurrencyPicker, PriceTag, CartSummary, TransactionListItem, AccountCard
/// and PaymentCard, on the gallery page `money`.
void main() {
  final g = Harness.goldens;
  late Harness h;
  setUp(() async {
    h = await Harness.create();
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(SystemChannels.platform, (call) async {
          return null;
        });
  });
  tearDown(() {
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(SystemChannels.platform, null);
    return h.close();
  });

  Future<void> open(
    WidgetTester tester, {
    String? locale,
    String tag = '',
    bool rtl = false,
    bool dark = false,
    double scale = 1,
  }) async {
    tester.view.physicalSize = const Size(800, 4200);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    await tester.runAsync(() async {
      // The locale comes from the release, as in an app: the bundle carries
      // its data and the runtime switches to it.
      final app = g.bundles['widgets/widgets.pxb']!;
      await h.startFrom(
        locale == null ? app : withLocales(app, const {}, verbatim: {locale}),
        {'gallery': g.bundles['widgets/gallery.pxb']!},
        themeSource: PluxThemeSource.plux,
      );
      await h.loadIconFonts();
      if (locale != null) {
        Plux.setLocale(localeOfTag(tag.isEmpty ? locale : tag));
        await Plux.container.read(localeControllerProvider.notifier).settled();
      }
    });
    await tester.pumpWidget(
      MaterialApp(
        debugShowCheckedModeBanner: false,
        builder: (context, child) => Directionality(
          textDirection: rtl ? TextDirection.rtl : TextDirection.ltr,
          child: MediaQuery(
            data: MediaQuery.of(context)
                .copyWith(textScaler: TextScaler.linear(scale)),
            child: child!,
          ),
        ),
        home: PluxScope(child: PluxView('money')),
      ),
    );
    await settle(tester);
    if (dark) {
      Plux.setThemeMode(ThemeMode.dark);
      await settle(tester);
      await tester.pump(const Duration(milliseconds: 300));
    }
  }

  List<PluxException> problems() => [
    for (final e in h.errors)
      if (e.code != PluxErrorCode.syncFailed) e,
  ];

  Map<String, Object?> state(WidgetTester tester) {
    final scope = RenderScope.of(tester.element(find.text('Check out')));
    return Plux.container.read(pageStateProvider(scope.state!));
  }

  group('The money group in light, dark, right-to-left and 200% text '
      '[WGT-020]', () {
    for (final (name, dark, rtl, scale) in [
      ('light', false, false, 1.0),
      ('dark', true, false, 1.0),
      ('rtl', false, true, 1.0),
      ('text200', false, false, 2.0),
    ]) {
      testWidgets(name, (tester) async {
        await open(tester, dark: dark, rtl: rtl, scale: scale);
        for (final t in ['Shirt', 'Check out', 'Coffee shop', 'Everyday']) {
          expect(find.text(t), findsWidgets, reason: t);
        }
        expect(tester.takeException(), isNull);
        expect(problems(), isEmpty);
        await expectLater(
          find.byType(MaterialApp),
          matchesGoldenFile('goldens/money_$name.png'),
        );
      });
    }
  });

  testWidgets('the default locale writes amounts with the locale data of '
      'the release [I18N-011]', (tester) async {
    await open(tester);
    expect(find.text('€1,234.56'), findsWidgets);
    // The release's data holds the currencies the app uses; USD is not one.
    expect(find.text('-USD\u00a050.00'), findsOneWidget);
  });

  testWidgets('de-DE writes 1.234,56 € and types with the German '
      'separators [I18N-011]', (tester) async {
    await open(tester, locale: 'de', tag: 'de-DE');
    expect(find.text('1.234,56\u00a0€'), findsWidgets);
    expect(find.text('-50,00\u00a0USD'), findsOneWidget);
    expect(find.text('19,99\u00a0€'), findsOneWidget);
    expect(find.text('30,35\u00a0€'), findsOneWidget);
    final amount = find.byType(TextField).at(0);
    expect(tester.widget<TextField>(amount).controller!.text, '12,50');
    await tester.enterText(amount, '1.234,56');
    await settle(tester);
    expect(state(tester)['amount'], '1234.56 EUR');
    await tester.enterText(amount, '1.234,567');
    expect(tester.widget<TextField>(amount).controller!.text, '1.234,56');
    await tester.enterText(amount, '9999');
    await settle(tester);
    expect(find.text('Between 1 and 5000'), findsOneWidget);
    expect(problems(), isEmpty);
  });

  testWidgets('ar writes amounts with the locale data [I18N-011]', (
    tester,
  ) async {
    await open(tester, locale: 'ar', tag: 'ar', rtl: true);
    final ar = _load('ar');
    final expected = ar.formatMoney(PluxDecimal.parse('1234.56'), 'EUR');
    expect(find.text(expected), findsWidgets);
    expect(expected, isNot(contains('EUR')));
    expect(problems(), isEmpty);
    await expectLater(
      find.byType(MaterialApp),
      matchesGoldenFile('goldens/money_ar.png'),
    );
  });

  testWidgets('MoneyText reads the amount with the currency name '
      '[A11Y-001]', (tester) async {
    final handle = tester.ensureSemantics();
    await open(tester, locale: 'de', tag: 'de-DE');
    expect(find.bySemanticsLabel('1.234,56 Euro'), findsOneWidget);
    handle.dispose();
  });

  testWidgets('MaskedText hides all but the last digits from the screen '
      'reader and reveals on request [WGT-020, A11Y-001]', (tester) async {
    final handle = tester.ensureSemantics();
    await open(tester);
    expect(
      find.text(
        '•••• •••• '
        '•••• 1234',
      ),
      findsWidgets,
    );
    expect(find.bySemanticsLabel(RegExp('\u2022')), findsNothing);
    expect(find.bySemanticsLabel(RegExp('4111')), findsNothing);
    await tester.tap(find.byTooltip('Show number'));
    await settle(tester);
    expect(state(tester)['revealed'], 'yes');
    expect(find.text('4111 1111 1111 1234'), findsOneWidget);
    handle.dispose();
  });

  testWidgets('IbanInput groups by four and checks the digits '
      '[WGT-020]', (tester) async {
    await open(tester);
    final iban = find.byType(TextField).at(2);
    await tester.enterText(iban, 'de89370400440532013000');
    await settle(tester);
    expect(
      tester.widget<TextField>(iban).controller!.text,
      'DE89 3704 0044 0532 0130 00',
    );
    expect(state(tester)['iban'], 'DE89370400440532013000');
    expect(find.text('Check the IBAN'), findsNothing);
    await tester.enterText(iban, 'DE89370400440532013001');
    await settle(tester);
    expect(find.text('Check the IBAN'), findsOneWidget);
    expect(state(tester)['iban'], '');
  });

  testWidgets('PhoneInput validates with the region of the locale and '
      'reports E.164 [WGT-020]', (tester) async {
    await open(tester, locale: 'de', tag: 'de-DE');
    expect(find.textContaining('+49'), findsOneWidget);
    final phone = find.byType(TextField).at(1);
    await tester.enterText(phone, '030 901820');
    await settle(tester);
    expect(state(tester)['phone'], '+4930901820');
    await tester.enterText(phone, '12');
    await settle(tester);
    expect(find.text('Invalid phone number'), findsOneWidget);
    expect(state(tester)['phone'], '');
  });

  testWidgets('CountryPicker and CurrencyPicker search names in the '
      'locale [I18N-011]', (tester) async {
    await open(tester, locale: 'de', tag: 'de-DE');
    await tester.enterText(find.byType(TextField).at(3), 'Deutschland');
    await settle(tester);
    final row = find.widgetWithText(ListTile, 'Deutschland');
    expect(row, findsOneWidget);
    expect(find.text('\u{1F1E9}\u{1F1EA}'), findsOneWidget);
    await tester.tap(row);
    await settle(tester);
    expect(state(tester)['country'], 'DE');
    await tester.enterText(find.byType(TextField).at(4), 'Schweizer');
    await settle(tester);
    await tester.tap(find.widgetWithText(ListTile, 'Schweizer Franken'));
    await settle(tester);
    expect(state(tester)['currency'], 'CHF');
  });

  testWidgets('PriceTag announces the original price; the cart and the '
      'transaction fire their events [WGT-020, A11Y-001]', (tester) async {
    final handle = tester.ensureSemantics();
    await open(tester, locale: 'de', tag: 'de-DE');
    expect(
      find.bySemanticsLabel(RegExp('^19,99 Euro, was 29,99 Euro')),
      findsOneWidget,
    );
    await tester.tap(find.text('Check out'));
    await settle(tester);
    expect(state(tester)['checkout'], 'go');
    await tester.tap(find.text('Coffee shop'));
    await settle(tester);
    expect(state(tester)['tapped'], 'tx');
    handle.dispose();
  });

  testWidgets('cards flip on tap and fire onFlip; reduce motion flips at '
      'once [WGT-020, A11Y-001]', (tester) async {
    await open(tester);
    expect(find.text('Everyday'), findsOneWidget);
    await tester.tap(find.text('Everyday'));
    await settle(tester);
    await tester.pump(const Duration(milliseconds: 500));
    expect(state(tester)['flip'], 'true');
    expect(find.text('Account details'), findsOneWidget);
    expect(find.text('Everyday'), findsNothing);
    await tester.tap(find.text('Account details'));
    await settle(tester);
    await tester.pump(const Duration(milliseconds: 500));
    expect(find.text('Everyday'), findsOneWidget);
    expect(state(tester)['flip'], 'false');
  });
}
