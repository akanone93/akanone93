// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/src/l10n/l10n.dart';

PluxLocaleData _load(String id) => PluxLocaleData.fromJson(
  jsonDecode(File('../../schema/l10n/cldr/$id.json').readAsStringSync())
      as Map<String, Object?>,
);

PluxDecimal _d(String s) => PluxDecimal.parse(s);

Matcher _throws(L10nError code) =>
    throwsA(isA<L10nException>().having((e) => e.code, 'code', code));

void main() {
  late PluxLocaleIndex index;
  setUpAll(() {
    index = PluxLocaleIndex.fromJson(
      jsonDecode(File('../../schema/l10n/cldr/index.json').readAsStringSync())
          as Map<String, Object?>,
    );
  });

  test('money examples of I18N-011', () {
    expect(_load('de').formatMoney(_d('1234.56'), 'EUR'), '1.234,56 €');
    expect(_load('en').formatMoney(_d('1234.56'), 'USD'), r'$1,234.56');
    expect(_load('en').formatMoney(_d('1234.5'), 'JPY'), '¥1,234');
    expect(
      _load('en')
          .formatMoney(_d('-3'), 'USD', const MoneyOptions(accounting: true)),
      r'($3.00)',
    );
  });

  test('money errors', () {
    final en = _load('en');
    for (final bad in ['', 'us', 'usd', 'USDX']) {
      expect(
        () => en.formatMoney(_d('1'), bad),
        _throws(L10nError.invalidCurrency),
      );
    }
    expect(
      () => en.formatMoney(
        _d('1'),
        'USD',
        const MoneyOptions(fraction: FractionDigits(3, 2)),
      ),
      _throws(L10nError.invalidOption),
    );
    expect(
      () => en.formatMoney(
        _d('1'),
        'USD',
        const MoneyOptions(numberingSystem: 'arab'),
      ),
      _throws(L10nError.unsupported),
    );
    expect(en.currencyDigits('JPY'), 0);
    expect(en.currencyDigits('KWD'), 3);
    expect(en.currencyName('ZZZ'), 'ZZZ');
  });

  test('number formatting details', () {
    expect(_load('hi').formatNumber(_d('1234567.891')), '12,34,567.891');
    expect(_load('es').formatNumber(_d('1234')), '1234');
    expect(_load('es').formatNumber(_d('12345')), '12.345');
    final en = _load('en');
    const zero = NumberOptions(fraction: FractionDigits(0, 0));
    expect(en.formatNumber(_d('2.5'), zero), '2');
    expect(en.formatNumber(_d('3.5'), zero), '4');
    expect(
      en.formatNumber(
        _d('-0.001'),
        const NumberOptions(fraction: FractionDigits(0, 1)),
      ),
      '-0',
    );
    expect(
      en.formatNumber(_d('1234.5'), const NumberOptions(noGrouping: true)),
      '1234.5',
    );
    expect(
      _load('fa').formatNumber(
        _d('1234.5'),
        const NumberOptions(numberingSystem: 'default'),
      ),
      '۱٬۲۳۴٫۵',
    );
    expect(
      () => en.formatNumber(
        _d('1'),
        const NumberOptions(fraction: FractionDigits(0, 25)),
      ),
      _throws(L10nError.invalidOption),
    );
    expect(
      () => en.formatCompact(
        _d('1'),
        const NumberOptions(fraction: FractionDigits(0, 1)),
      ),
      _throws(L10nError.invalidOption),
    );
    expect(en.formatCompact(_d('999999')), '1M');
    expect(en.formatCompact(_d('-1500000')), '-1.5M');
    expect(_load('de').formatCompact(_d('1500000')), '1,5 Mio.');
  });

  test('decimals', () {
    for (final (input, want) in [
      ('0', '0'),
      ('-0', '-0'),
      ('+1.50', '1.50'),
      ('007', '7'),
      ('1.5e3', '1500'),
      ('1.5E-3', '0.0015'),
      ('.5', '0.5'),
    ]) {
      expect(_d(input).toString(), want, reason: input);
    }
    for (final bad in ['', '-', 'abc', '1.2.3', '1e', '1e9999', '--1', '٣']) {
      expect(() => _d(bad), _throws(L10nError.invalidDecimal), reason: bad);
    }
    expect(PluxDecimal.fromInt(-42).toString(), '-42');
    expect(PluxDecimal.zero.isZero, isTrue);
    expect(_d('9.995').roundFrac(2).toString(), '10.00');
    expect(_d('1250').roundFrac(-2).toString(), '1200');
    expect(_d('1.50'), isNot(equals(_d('1.5'))));
  });

  test('plural rule syntax and categories', () {
    for (final r in [
      'n = 1',
      'n != 0..2,5',
      'n % 10 = 2..4 and n % 100 != 12..14',
      'e = 0 and i != 0 and i % 1000000 = 0 and v = 0 or e != 0..5',
    ]) {
      PluralRule.parse(r);
    }
    for (final r in [
      '',
      'x = 1',
      'n',
      'n = ',
      'n % 0 = 1',
      'n = 5..2',
      'n = 1 and',
      'n ~ 1',
    ]) {
      expect(
        () => PluralRule.parse(r),
        _throws(L10nError.invalidData),
        reason: r,
      );
    }
    expect(
      () => PluralRules.compile({'several': 'n = 1'}),
      _throws(L10nError.invalidData),
    );
    expect(_load('ar').categories, PluralCategory.values);
    expect(_load('en').pluralCardinal(_d('1')), PluralCategory.one);
    expect(_load('en').pluralCardinal(_d('1.0')), PluralCategory.other);
    expect(_load('en').pluralOrdinal(_d('-22')), PluralCategory.two);
    expect(_load('fr').pluralCardinal(_d('1000000')), PluralCategory.many);
    expect(_load('ba').pluralCardinal(_d('1')), PluralCategory.other);
  });

  test('locale resolution', () {
    String r(String tag, List<String> supported, String fallback) =>
        resolveLocale(tag, supported, fallback, index: index);
    expect(r('de-AT', ['en', 'de'], 'en'), 'de');
    expect(r('de_at', ['en', 'de-AT'], 'en'), 'de-AT');
    expect(r('de', ['en-US', 'de-DE'], 'en-US'), 'de-DE');
    expect(r('fr-CA', ['en', 'de'], 'en'), 'en');
    expect(r('zh-TW', ['zh', 'zh-Hant'], 'zh'), 'zh-Hant');
    expect(r('zh-HK', ['zh', 'zh-Hant-HK'], 'zh'), 'zh-Hant-HK');
    expect(r('iw', ['he'], 'en'), 'he');
    expect(r('en-GB-u-ca-gregory', ['en'], 'de'), 'en');
    expect(r('xx', ['en'], 'de_de'), 'de-DE');
    expect(() => r('xx', ['en'], ''), _throws(L10nError.noLocale));
    for (final bad in [
      '',
      ' en',
      'e',
      'english',
      'en-',
      'und',
      'en-US-\$',
      '123',
    ]) {
      expect(
        () => r(bad, ['en'], 'en'),
        _throws(L10nError.invalidTag),
        reason: bad,
      );
    }
  });

  test('locale data', () {
    final de = _load('de');
    expect(de.id, 'de');
    expect(de.isRtl, isFalse);
    expect(_load('ar').isRtl, isTrue);
    expect(_load('am').territoryName('ET'), 'ኢትዮጵያ');
    expect(de.territoryName('ZZ'), isNull);
    expect(() => PluxLocaleData.fromJson({}), _throws(L10nError.invalidData));
    expect(
      () => PluxLocaleData.fromJson({
        'locale': 'x',
        'numbers': <String, Object?>{'latn': <String, Object?>{}},
        'calendars': <String, Object?>{'g': <String, Object?>{}},
      }),
      _throws(L10nError.invalidData),
    );
  });

  test('dates', () {
    final en = _load('en');
    final d = PluxCalendarDate.gregorian(2025, 10, 16, 15, 4, 5);
    expect(d.weekday, 4);
    expect(PluxCalendarDate.gregorian(0, 12, 31).era, 0);
    expect(PluxCalendarDate.gregorian(-399, 3, 1).weekday, 4);
    String f(PluxDateOptions o) => en.formatDate(d, 'gregorian', o);
    expect(
      f(const PluxDateOptions(dateStyle: 'full')),
      'Thursday, October 16, 2025',
    );
    expect(f(const PluxDateOptions(skeleton: 'GyMMMd')), 'Oct 16, 2025 AD');
    expect(f(const PluxDateOptions(skeleton: 'yyMd')), '10/16/25');
    expect(f(const PluxDateOptions(skeleton: 'Hm')), '15:04');
    expect(
      f(const PluxDateOptions(timeStyle: 'full', zoneName: 'UTC')),
      '3:04:05 PM UTC',
    );
    for (final o in [
      const PluxDateOptions(),
      const PluxDateOptions(dateStyle: 'full', skeleton: 'yMd'),
      const PluxDateOptions(dateStyle: 'tiny'),
      const PluxDateOptions(timeStyle: 'tiny'),
      const PluxDateOptions(skeleton: 'yQ'),
      const PluxDateOptions(skeleton: 'yMy'),
      const PluxDateOptions(skeleton: 'ymsd'),
      const PluxDateOptions(dateStyle: 'short', numberingSystem: 'arab'),
    ]) {
      expect(() => f(o), throwsA(isA<L10nException>()));
    }
    expect(() => en.formatDate(d, 'julian'), _throws(L10nError.unsupported));
    expect(
      () => en.formatDate(
        const PluxCalendarDate(era: 1, year: 1, month: 1, day: 0, weekday: 0),
        'gregorian',
        const PluxDateOptions(dateStyle: 'short'),
      ),
      _throws(L10nError.invalidOption),
    );
    expect(
      () => en.formatDate(
        const PluxCalendarDate(
          era: 1,
          year: 2000,
          month: 13,
          day: 1,
          weekday: 0,
        ),
        'gregorian',
        const PluxDateOptions(dateStyle: 'long'),
      ),
      _throws(L10nError.invalidOption),
    );
  });
}
