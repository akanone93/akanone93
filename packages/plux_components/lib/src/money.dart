// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The money and locale Layer 2 components of P5 (WGT-020, I18N-011,
/// ADR-0071): `AmountInput`, `MoneyText`, `MaskedText`, `PhoneInput`,
/// `IbanInput`, `CountryPicker`, `CurrencyPicker`, `PriceTag`,
/// `CartSummary`, `TransactionListItem`, `AccountCard` and `PaymentCard`.
/// Amounts, country and currency names are written with the locale data of
/// the nearest `PluxLocaleScope`, or with a minimal English format when
/// there is none; every component follows the text direction and scale and
/// names its role to screen readers (A11Y-001).
library;

import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:plux_components/src/ids.dart';
import 'package:plux_flutter/plux_flutter.dart';

final _amount = componentIds['AmountInput']!;
final _moneyText = componentIds['MoneyText']!;
final _masked = componentIds['MaskedText']!;
final _phone = componentIds['PhoneInput']!;
final _iban = componentIds['IbanInput']!;
final _country = componentIds['CountryPicker']!;
final _currency = componentIds['CurrencyPicker']!;
final _price = componentIds['PriceTag']!;
final _cart = componentIds['CartSummary']!;
final _transaction = componentIds['TransactionListItem']!;
final _account = componentIds['AccountCard']!;
final _payment = componentIds['PaymentCard']!;

/// The builders of the money group, by permanent widget ID.
final Map<int, NodeBuilder> moneyBuilders = {
  _amount.id: buildAmountInput,
  _moneyText.id: buildMoneyText,
  _masked.id: buildMaskedText,
  _phone.id: buildPhoneInput,
  _iban.id: buildIbanInput,
  _country.id: buildCountryPicker,
  _currency.id: buildCurrencyPicker,
  _price.id: buildPriceTag,
  _cart.id: buildCartSummary,
  _transaction.id: buildTransactionListItem,
  _account.id: buildAccountCard,
  _payment.id: buildPaymentCard,
};

// ── Decoding ────────────────────────────────────────────────────────────────

Money? _asMoney(Decoding d, Object? v) => v is Money ? v : null;

PxlDateTime? _asDateTime(Decoding d, Object? v) => v is PxlDateTime ? v : null;

// ── Formatting ──────────────────────────────────────────────────────────────

const _zeroDigits = {'JPY', 'KRW', 'VND', 'CLP', 'ISK', 'UGX', 'XAF', 'XOF'};
const _threeDigits = {'BHD', 'KWD', 'OMR', 'JOD', 'TND', 'IQD', 'LYD'};

/// The minor-unit digits of [currency] in [data], or in the built-in table.
int _digits(PluxLocaleData? data, String currency) =>
    data?.currencyDigits(currency) ??
    (_zeroDigits.contains(currency)
        ? 0
        : _threeDigits.contains(currency)
        ? 3
        : 2);

PluxDecimal _px(Decimal d) => PluxDecimal.parse(d.toString());

/// [a] rounded half to even to [digits] fraction digits, as unsigned
/// digits with grouping by three and a point: the built-in English format.
String _enNumber(Decimal a, int digits) {
  var u = a.unscaled.abs();
  if (a.scale > digits) {
    final f = BigInt.from(10).pow(a.scale - digits);
    final q = u ~/ f;
    final twice = (u % f) * BigInt.two;
    u = twice > f || (twice == f && q.isOdd) ? q + BigInt.one : q;
  } else if (a.scale < digits) {
    u *= BigInt.from(10).pow(digits - a.scale);
  }
  final t = u.toString().padLeft(digits + 1, '0');
  final ip = t.substring(0, t.length - digits);
  final b = StringBuffer();
  for (var i = 0; i < ip.length; i++) {
    if (i > 0 && (ip.length - i) % 3 == 0) b.write(',');
    b.write(ip[i]);
  }
  return digits > 0 ? '$b.${t.substring(t.length - digits)}' : '$b';
}

CurrencyDisplay _currencyDisplay(String name) => switch (name) {
  'narrowSymbol' => CurrencyDisplay.narrow,
  'code' => CurrencyDisplay.code,
  _ => CurrencyDisplay.symbol,
};

/// [m] written for the locale in scope: `1.234,56 €` in German. [display]
/// is a `MoneyDisplay` and [sign] a `SignDisplay` member name.
String _money(
  BuildContext context,
  Money m, {
  String display = 'symbol',
  String sign = 'auto',
}) {
  final data = PluxLocaleScope.dataOf(context);
  final neg = m.amount.sign < 0;
  final shown = sign == 'never' && neg ? Money(-m.amount, m.currency) : m;
  String body;
  try {
    body = data != null
        ? data.formatMoney(
            _px(shown.amount),
            shown.currency,
            MoneyOptions(display: _currencyDisplay(display)),
          )
        : _fallbackMoney(shown);
  } on L10nException {
    body = _fallbackMoney(shown);
  }
  final plus =
      !neg && (sign == 'always' || (sign == 'exceptZero' && !m.amount.isZero));
  return plus ? '+$body' : body;
}

String _fallbackMoney(Money m) =>
    '${m.amount.sign < 0 ? '-' : ''}${m.currency} '
    '${_enNumber(m.amount, _digits(null, m.currency))}';

/// [m] as a screen reader says it: the number and the currency's name.
String _spoken(BuildContext context, Money m) {
  final data = PluxLocaleScope.dataOf(context);
  final digits = _digits(data, m.currency);
  if (data == null) {
    return '${m.amount.sign < 0 ? '-' : ''}${_enNumber(m.amount, digits)} '
        '${m.currency}';
  }
  try {
    return '${data.formatNumber(_px(m.amount), NumberOptions(fraction: FractionDigits(digits, digits)))} '
        '${data.currencyName(m.currency)}';
  } on L10nException {
    return '${m.amount} ${m.currency}';
  }
}

/// The decimal and grouping separators of the locale in scope.
(String, String) _separators(PluxLocaleData? data) {
  if (data == null) return ('.', ',');
  final s = data.formatNumber(
    PluxDecimal.parse('1234567.5'),
    const NumberOptions(fraction: FractionDigits(1, 1)),
  );
  final dec = s.length >= 2 ? s[s.length - 2] : '.';
  final one = s.indexOf('1');
  final group = one >= 0 && one + 1 < s.length && s[one + 1] != '2'
      ? s[one + 1]
      : ',';
  return (dec, group);
}

/// [d] as typed in the locale: grouped, with as many fraction digits as it
/// has up to the currency's.
String _typed(PluxLocaleData? data, Decimal d, int digits) {
  final scale = math.min(d.scale, digits);
  if (data == null) {
    final t = _enNumber(d, scale);
    return d.sign < 0 ? '-$t' : t;
  }
  return data.formatNumber(
    _px(d),
    NumberOptions(fraction: FractionDigits(scale, scale)),
  );
}

String _date(BuildContext context, PxlDateTime t) {
  final (days, _) = t.local;
  final data = PluxLocaleScope.dataOf(context);
  if (data == null) return PxlDate(days).toString();
  final (y, m, d) = civilFromDays(days);
  return data.formatDate(
    PluxCalendarDate.gregorian(y, m, d),
    'gregorian',
    const PluxDateOptions(dateStyle: 'medium'),
  );
}

String _flag(String code) => code.length == 2
    ? String.fromCharCodes([
        for (final c in code.toUpperCase().codeUnits) 0x1F1E6 + c - 0x41,
      ])
    : '';

/// The trailing run of digits of [s], which is all a masked number says.
String _ending(String s) => RegExp(r'[0-9]+$').stringMatch(s) ?? '';

// ── AmountInput ─────────────────────────────────────────────────────────────

/// `AmountInput`: a text field for an amount in `currency`. It accepts the
/// locale's group and decimal separators, at most the currency's minor
/// units, and fires `onChanged(money)` for every valid amount in
/// `min..max`; an amount outside it shows `rangeMessage`, announced.
Widget buildAmountInput(NodeContext c) {
  final currency =
      c.decode(_amount.p('currency'), asString) ??
      c.missing('AmountInput.currency');
  return _AmountInput(
    label: c.decode(_amount.p('label'), asString),
    currency: currency,
    value: c.decode(_amount.p('value'), _asMoney),
    min: c.decode(_amount.p('min'), _asMoney),
    max: c.decode(_amount.p('max'), _asMoney),
    message:
        c.decode(_amount.p('rangeMessage'), asString) ??
        RuntimeStrings.of(c.context)['plux.amount.outOfRange'],
    onChanged: c.handles(_amount.e('onChanged'))
        ? (m) => c.fire(_amount.e('onChanged'), m)
        : null,
  );
}

final class _AmountInput extends StatefulWidget {
  const _AmountInput({
    required this.label,
    required this.currency,
    required this.value,
    required this.min,
    required this.max,
    required this.message,
    required this.onChanged,
  });

  final String? label;
  final String currency;
  final Money? value;
  final Money? min;
  final Money? max;
  final String message;
  final ValueChanged<Money>? onChanged;

  @override
  State<_AmountInput> createState() => _AmountInputState();
}

final class _AmountInputState extends State<_AmountInput> {
  late final TextEditingController _controller = TextEditingController();
  bool _seeded = false;
  String? _error;

  PluxLocaleData? get _data => PluxLocaleScope.dataOf(context);

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    if (!_seeded) {
      _seeded = true;
      _show(widget.value);
    }
  }

  @override
  void didUpdateWidget(_AmountInput old) {
    super.didUpdateWidget(old);
    if (old.value != widget.value && widget.value != _parse(_controller.text)) {
      _show(widget.value);
    }
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  void _show(Money? m) {
    _controller.text = m == null
        ? ''
        : _typed(_data, m.amount, _digits(_data, widget.currency));
  }

  Money? _parse(String text) {
    final (dec, group) = _separators(_data);
    var t = text.replaceAll(RegExp(r'\s'), '').replaceAll(group, '');
    t = t.replaceAll(dec, '.');
    if (t.endsWith('.')) t = t.substring(0, t.length - 1);
    final d = Decimal.tryParse(t);
    return d == null ? null : Money(d, widget.currency);
  }

  void _changed(String text) {
    final m = _parse(text);
    String? error;
    if (m != null) {
      final lo = widget.min;
      final hi = widget.max;
      if ((lo != null && m.amount.compareTo(lo.amount) < 0) ||
          (hi != null && m.amount.compareTo(hi.amount) > 0)) {
        error = widget.message;
      } else {
        widget.onChanged?.call(m);
      }
    }
    if (error != _error) setState(() => _error = error);
  }

  @override
  Widget build(BuildContext context) {
    final (dec, group) = _separators(_data);
    final digits = _digits(_data, widget.currency);
    final signed = widget.min == null || widget.min!.amount.sign < 0;
    return TextField(
      controller: _controller,
      keyboardType: TextInputType.numberWithOptions(
        decimal: digits > 0,
        signed: signed,
      ),
      inputFormatters: [_AmountFormatter(dec, group, digits, signed)],
      textDirection: TextDirection.ltr,
      onChanged: _changed,
      decoration: InputDecoration(
        labelText: widget.label,
        prefixText: '${widget.currency} ',
        error: _error == null
            ? null
            : Semantics(liveRegion: true, child: Text(_error!)),
      ),
    );
  }
}

/// Keeps what is typed to digits, the locale's separators and a leading
/// minus, with one decimal separator and at most the currency's digits
/// after it.
final class _AmountFormatter extends TextInputFormatter {
  const _AmountFormatter(this.decimal, this.group, this.digits, this.signed);

  final String decimal;
  final String group;
  final int digits;
  final bool signed;

  @override
  TextEditingValue formatEditUpdate(
    TextEditingValue oldValue,
    TextEditingValue newValue,
  ) {
    final t = newValue.text;
    for (var i = 0; i < t.length; i++) {
      final ch = t[i];
      final ok =
          (ch.codeUnitAt(0) >= 0x30 && ch.codeUnitAt(0) <= 0x39) ||
          ch == decimal ||
          ch == group ||
          ch.trim().isEmpty ||
          (ch == '-' && i == 0 && signed);
      if (!ok) return oldValue;
    }
    final at = t.indexOf(decimal);
    if (at >= 0) {
      if (digits == 0 || t.indexOf(decimal, at + 1) >= 0) return oldValue;
      if (t.length - at - decimal.length > digits) return oldValue;
    }
    return newValue;
  }
}

// ── MoneyText ───────────────────────────────────────────────────────────────

/// `MoneyText`: an amount written for the locale and currency; screen
/// readers hear the number and the currency's name.
Widget buildMoneyText(NodeContext c) {
  final value =
      c.decode(_moneyText.p('value'), _asMoney) ?? c.missing('MoneyText.value');
  final context = c.context;
  return Semantics(
    label: _spoken(context, value),
    excludeSemantics: true,
    child: Text(
      _money(
        context,
        value,
        display:
            c.decode(_moneyText.p('display'), nameMoneyDisplay) ?? 'symbol',
        sign: c.decode(_moneyText.p('signDisplay'), nameSignDisplay) ?? 'auto',
      ),
      style: c.decode(_moneyText.p('style'), decodeTextStyle),
    ),
  );
}

// ── MaskedText ──────────────────────────────────────────────────────────────

/// `MaskedText`: [value] hidden but for its last `visibleDigits`.
Widget buildMaskedText(NodeContext c) => _MaskedText(
  value:
      c.decode(_masked.p('value'), asString) ?? c.missing('MaskedText.value'),
  mask: c.decode(_masked.p('mask'), nameMaskKind) ?? 'card',
  visible: math.max(0, c.decode(_masked.p('visibleDigits'), asInt) ?? 4),
  revealable: c.decode(_masked.p('revealable'), asBool) ?? false,
  revealLabel:
      c.decode(_masked.p('revealLabel'), asString) ??
      RuntimeStrings.of(c.context)['plux.masked.reveal'],
  onReveal: c.handles(_masked.e('onReveal'))
      ? () => c.fire(_masked.e('onReveal'))
      : null,
);

/// [value] with all but the last [visible] digits (all characters for
/// `custom`) replaced by bullets.
String _mask(String value, String kind, int visible) {
  final plain = kind == 'card' ? value.replaceAll(' ', '') : value;
  bool isDigit(int u) => u >= 0x30 && u <= 0x39;
  final hideable = [
    for (var i = 0; i < plain.length; i++)
      if (kind == 'custom' || isDigit(plain.codeUnitAt(i))) i,
  ];
  final keep = hideable.length > visible
      ? hideable.sublist(hideable.length - visible).toSet()
      : hideable.toSet();
  final hide = hideable.toSet().difference(keep);
  final out = StringBuffer();
  for (var i = 0; i < plain.length; i++) {
    if (kind == 'card' && i > 0 && i % 4 == 0) out.write(' ');
    out.write(hide.contains(i) ? '•' : plain[i]);
  }
  return out.toString();
}

final class _MaskedText extends StatefulWidget {
  const _MaskedText({
    required this.value,
    required this.mask,
    required this.visible,
    required this.revealable,
    required this.revealLabel,
    required this.onReveal,
  });

  final String value;
  final String mask;
  final int visible;
  final bool revealable;
  final String revealLabel;
  final VoidCallback? onReveal;

  @override
  State<_MaskedText> createState() => _MaskedTextState();
}

final class _MaskedTextState extends State<_MaskedText> {
  bool _revealed = false;

  @override
  void didUpdateWidget(_MaskedText old) {
    super.didUpdateWidget(old);
    if (old.value != widget.value) _revealed = false;
  }

  @override
  Widget build(BuildContext context) {
    final w = widget;
    final shown = _revealed
        ? (w.mask == 'card' ? _mask(w.value, 'card', w.value.length) : w.value)
        : _mask(w.value, w.mask, w.visible);
    final label = _revealed ? w.value : _ending(shown);
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Flexible(
          child: Semantics(
            label: label,
            excludeSemantics: true,
            child: Text(shown, textDirection: TextDirection.ltr),
          ),
        ),
        if (w.revealable)
          IconButton(
            tooltip: w.revealLabel,
            icon: Icon(_revealed ? Icons.visibility_off : Icons.visibility),
            onPressed: () {
              setState(() => _revealed = !_revealed);
              if (_revealed) w.onReveal?.call();
            },
          ),
      ],
    );
  }
}

// ── Pickers ─────────────────────────────────────────────────────────────────

@immutable
final class _Entry {
  const _Entry(this.code, this.name, this.lead);

  final String code;
  final String name;
  final String lead;
}

const _notCountries = {'EU', 'UN', 'EZ', 'QO', 'XA', 'XB', 'ZZ'};

List<_Entry> _countries(PluxLocaleData? data) {
  final codes = data == null
      ? phoneRegions()
      : [
          for (final c in data.territoryCodes)
            if (RegExp(r'^[A-Z]{2}$').hasMatch(c) && !_notCountries.contains(c))
              c,
        ];
  return [
    for (final c in codes) _Entry(c, data?.territoryName(c) ?? c, _flag(c)),
  ]..sort((a, b) => a.name.compareTo(b.name));
}

const _commonCurrencies = ['CHF', 'EUR', 'GBP', 'JPY', 'USD'];

List<_Entry> _currencies(PluxLocaleData? data, List<String>? only) {
  final codes =
      only ??
      (data == null
          ? _commonCurrencies
          : [
              for (final c in data.currencyCodes)
                if (RegExp(r'^[A-Z]{3}$').hasMatch(c)) c,
            ]);
  return [for (final c in codes) _Entry(c, data?.currencyName(c) ?? c, c)]
    ..sort((a, b) => a.name.compareTo(b.name));
}

/// A search field above a list of [entries]; choosing one calls [onPick].
final class _PickerList extends StatefulWidget {
  const _PickerList({
    required this.entries,
    required this.selected,
    required this.hint,
    required this.onPick,
    this.height = 360,
  });

  final List<_Entry> entries;
  final String? selected;
  final String hint;
  final ValueChanged<String> onPick;
  final double height;

  @override
  State<_PickerList> createState() => _PickerListState();
}

final class _PickerListState extends State<_PickerList> {
  String _query = '';

  @override
  Widget build(BuildContext context) {
    final q = _query.trim().toLowerCase();
    final shown = [
      for (final e in widget.entries)
        if (q.isEmpty ||
            e.name.toLowerCase().contains(q) ||
            e.code.toLowerCase().contains(q))
          e,
    ];
    return SizedBox(
      height: widget.height,
      child: Column(
        children: [
          Padding(
            padding: const EdgeInsets.all(8),
            child: TextField(
              onChanged: (v) => setState(() => _query = v),
              decoration: InputDecoration(
                prefixIcon: const Icon(Icons.search),
                hintText: widget.hint,
              ),
            ),
          ),
          Expanded(
            child: ListView.builder(
              itemCount: shown.length,
              itemBuilder: (context, i) {
                final e = shown[i];
                final on = e.code == widget.selected;
                return Semantics(
                  selected: on,
                  child: ListTile(
                    selected: on,
                    leading: Text(e.lead),
                    title: Text(e.name),
                    subtitle: Text(e.code),
                    trailing: on ? const Icon(Icons.check) : null,
                    onTap: () => widget.onPick(e.code),
                  ),
                );
              },
            ),
          ),
        ],
      ),
    );
  }
}

/// `CountryPicker`: a searchable list of territories named in the locale,
/// with their flags; fires `onSelected(code)`.
Widget buildCountryPicker(NodeContext c) => _Picker(
  build: (context) => _countries(PluxLocaleScope.dataOf(context)),
  selected: c.decode(_country.p('value'), asString),
  hint:
      c.decode(_country.p('searchHint'), asString) ??
      RuntimeStrings.of(c.context)['plux.picker.search'],
  onSelected: (code) => c.fire(_country.e('onSelected'), code),
);

/// `CurrencyPicker`: a searchable list of currencies named in the locale;
/// fires `onSelected(code)`.
Widget buildCurrencyPicker(NodeContext c) {
  final only = c.decode(_currency.p('currencies'), asStrings);
  return _Picker(
    build: (context) => _currencies(PluxLocaleScope.dataOf(context), only),
    selected: c.decode(_currency.p('value'), asString),
    hint:
        c.decode(_currency.p('searchHint'), asString) ??
        RuntimeStrings.of(c.context)['plux.picker.search'],
    onSelected: (code) => c.fire(_currency.e('onSelected'), code),
  );
}

final class _Picker extends StatefulWidget {
  const _Picker({
    required this.build,
    required this.selected,
    required this.hint,
    required this.onSelected,
  });

  final List<_Entry> Function(BuildContext context) build;
  final String? selected;
  final String hint;
  final ValueChanged<String> onSelected;

  @override
  State<_Picker> createState() => _PickerState();
}

final class _PickerState extends State<_Picker> {
  String? _selected;

  @override
  void initState() {
    super.initState();
    _selected = widget.selected;
  }

  @override
  void didUpdateWidget(_Picker old) {
    super.didUpdateWidget(old);
    if (old.selected != widget.selected) _selected = widget.selected;
  }

  @override
  Widget build(BuildContext context) => Semantics(
    container: true,
    explicitChildNodes: true,
    child: _PickerList(
      entries: widget.build(context),
      selected: _selected,
      hint: widget.hint,
      onPick: (code) {
        setState(() => _selected = code);
        widget.onSelected(code);
      },
    ),
  );
}

// ── PhoneInput ──────────────────────────────────────────────────────────────

/// `PhoneInput`: a phone number field with the flag and calling code of its
/// region as a country picker. The region starts as `region` or the
/// locale's; the number is checked with the `pxl.phone.v1` table and
/// `onChanged` fires with the E.164 form, or an empty string when it is not
/// valid.
Widget buildPhoneInput(NodeContext c) => _PhoneInput(
  label: c.decode(_phone.p('label'), asString),
  value: c.decode(_phone.p('value'), asString),
  region: c.decode(_phone.p('region'), asString),
  message:
      c.decode(_phone.p('invalidMessage'), asString) ??
      RuntimeStrings.of(c.context)['plux.phone.invalid'],
  onChanged: c.handles(_phone.e('onChanged'))
      ? (v) => c.fire(_phone.e('onChanged'), v)
      : null,
);

/// The region of a locale tag such as `de-DE`, or null.
String? _regionOf(String? tag) {
  if (tag == null) return null;
  for (final p in tag.split(RegExp('[-_]')).skip(1)) {
    if (p.length == 2 && p == p.toUpperCase()) return p;
  }
  return null;
}

/// The E.164 number [text] denotes in [region], or null when it is not a
/// valid number.
String? _e164(String text, String? region) {
  final t = text.trim();
  final digits = t.replaceAll(RegExp('[^0-9]'), '');
  if (digits.isEmpty) return null;
  final code = region == null ? null : phoneCallingCode(region);
  final candidates = t.startsWith('+')
      ? ['+$digits']
      : [
          if (code != null && digits.startsWith('0'))
            '+$code${digits.substring(1)}',
          if (code != null) '+$code$digits',
        ];
  for (final n in candidates) {
    if (isValidPhone(n, region ?? '')) return n;
  }
  return null;
}

final class _PhoneInput extends StatefulWidget {
  const _PhoneInput({
    required this.label,
    required this.value,
    required this.region,
    required this.message,
    required this.onChanged,
  });

  final String? label;
  final String? value;
  final String? region;
  final String message;
  final ValueChanged<String>? onChanged;

  @override
  State<_PhoneInput> createState() => _PhoneInputState();
}

final class _PhoneInputState extends State<_PhoneInput> {
  late final TextEditingController _controller = TextEditingController(
    text: widget.value ?? '',
  );
  String? _region;
  String _last = '';

  String? _defaultRegion(BuildContext context) {
    final r =
        widget.region?.toUpperCase() ??
        _regionOf(PluxLocaleScope.tagOf(context)) ??
        Localizations.maybeLocaleOf(context)?.countryCode;
    return r != null && isPhoneRegion(r) ? r.toUpperCase() : null;
  }

  @override
  void didUpdateWidget(_PhoneInput old) {
    super.didUpdateWidget(old);
    if (old.value != widget.value &&
        (widget.value ?? '') != _e164(_controller.text, _region)) {
      _controller.text = widget.value ?? '';
    }
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  void _emit(String text, String? region) {
    final v = _e164(text, region) ?? '';
    if (v != _last) {
      _last = v;
      widget.onChanged?.call(v);
    }
  }

  Future<void> _pick(BuildContext context, String? region) async {
    final data = PluxLocaleScope.dataOf(context);
    final code = await showModalBottomSheet<String>(
      context: context,
      isScrollControlled: true,
      builder: (sheet) => SafeArea(
        child: _PickerList(
          entries: _countries(data),
          selected: region,
          hint: 'Search',
          height: MediaQuery.sizeOf(sheet).height * 0.6,
          onPick: (c) => Navigator.of(sheet).pop(c),
        ),
      ),
    );
    if (code == null || !mounted) return;
    setState(() => _region = code);
    _emit(_controller.text, code);
  }

  @override
  Widget build(BuildContext context) {
    final region = _region ?? _defaultRegion(context);
    final code = region == null ? null : phoneCallingCode(region);
    final text = _controller.text;
    final invalid = text.trim().isNotEmpty && _e164(text, region) == null;
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Semantics(
          button: true,
          label: region == null ? null : '$region +$code',
          excludeSemantics: region != null,
          child: TextButton(
            onPressed: () => _pick(context, region),
            child: Text(
              region == null ? '+' : '${_flag(region)} +$code',
              textDirection: TextDirection.ltr,
            ),
          ),
        ),
        Expanded(
          child: TextField(
            controller: _controller,
            keyboardType: TextInputType.phone,
            textDirection: TextDirection.ltr,
            inputFormatters: [
              FilteringTextInputFormatter.allow(RegExp(r'[0-9+\-\s().]')),
            ],
            onChanged: (v) {
              setState(() {});
              _emit(v, region);
            },
            decoration: InputDecoration(
              labelText: widget.label,
              error: invalid
                  ? Semantics(liveRegion: true, child: Text(widget.message))
                  : null,
            ),
          ),
        ),
      ],
    );
  }
}

// ── IbanInput ───────────────────────────────────────────────────────────────

/// `IbanInput`: an IBAN field that groups what is typed by four and checks
/// the mod-97 digits; `onChanged` fires with the IBAN without spaces, or an
/// empty string while it is not valid.
Widget buildIbanInput(NodeContext c) => _IbanInput(
  label: c.decode(_iban.p('label'), asString),
  value: c.decode(_iban.p('value'), asString),
  message:
      c.decode(_iban.p('invalidMessage'), asString) ??
      RuntimeStrings.of(c.context)['plux.iban.invalid'],
  onChanged: c.handles(_iban.e('onChanged'))
      ? (v) => c.fire(_iban.e('onChanged'), v)
      : null,
);

String _group4(String compact) {
  final b = StringBuffer();
  for (var i = 0; i < compact.length; i++) {
    if (i > 0 && i % 4 == 0) b.write(' ');
    b.write(compact[i]);
  }
  return b.toString();
}

/// Upper-cases what is typed, drops all but letters and digits, and groups
/// it by four.
final class _IbanFormatter extends TextInputFormatter {
  const _IbanFormatter();

  @override
  TextEditingValue formatEditUpdate(
    TextEditingValue oldValue,
    TextEditingValue newValue,
  ) {
    var compact = newValue.text.toUpperCase().replaceAll(
      RegExp('[^A-Z0-9]'),
      '',
    );
    if (compact.length > 34) compact = compact.substring(0, 34);
    final t = _group4(compact);
    return TextEditingValue(
      text: t,
      selection: TextSelection.collapsed(offset: t.length),
    );
  }
}

final class _IbanInput extends StatefulWidget {
  const _IbanInput({
    required this.label,
    required this.value,
    required this.message,
    required this.onChanged,
  });

  final String? label;
  final String? value;
  final String message;
  final ValueChanged<String>? onChanged;

  @override
  State<_IbanInput> createState() => _IbanInputState();
}

final class _IbanInputState extends State<_IbanInput> {
  late final TextEditingController _controller = TextEditingController(
    text: _group4((widget.value ?? '').replaceAll(' ', '').toUpperCase()),
  );
  String _last = '';

  String get _compact => _controller.text.replaceAll(' ', '');

  @override
  void didUpdateWidget(_IbanInput old) {
    super.didUpdateWidget(old);
    final v = (widget.value ?? '').replaceAll(' ', '').toUpperCase();
    if (old.value != widget.value && v != _compact) {
      _controller.text = _group4(v);
    }
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final compact = _compact;
    final invalid = compact.length >= 15 && !isIbanNumber(compact);
    return TextField(
      controller: _controller,
      textDirection: TextDirection.ltr,
      textCapitalization: TextCapitalization.characters,
      inputFormatters: const [_IbanFormatter()],
      onChanged: (_) {
        setState(() {});
        final v = isIbanNumber(_compact) ? _compact : '';
        if (v != _last) {
          _last = v;
          widget.onChanged?.call(v);
        }
      },
      decoration: InputDecoration(
        labelText: widget.label,
        error: invalid
            ? Semantics(liveRegion: true, child: Text(widget.message))
            : null,
      ),
    );
  }
}

// ── PriceTag ────────────────────────────────────────────────────────────────

/// `PriceTag`: `price`, with `originalPrice` struck through; screen readers
/// hear the price and then "was" and the original price.
Widget buildPriceTag(NodeContext c) {
  final price =
      c.decode(_price.p('price'), _asMoney) ?? c.missing('PriceTag.price');
  final original = c.decode(_price.p('originalPrice'), _asMoney);
  final was =
      c.decode(_price.p('wasLabel'), asString) ??
      RuntimeStrings.of(c.context)['plux.price.was'];
  final context = c.context;
  final style = Theme.of(context).textTheme;
  return Semantics(
    label:
        '${_spoken(context, price)}'
        '${original == null ? '' : ', $was ${_spoken(context, original)}'}',
    excludeSemantics: true,
    child: Wrap(
      crossAxisAlignment: WrapCrossAlignment.end,
      spacing: 8,
      children: [
        Text(_money(context, price), style: style.titleLarge),
        if (original != null)
          Text(
            _money(context, original),
            style: style.bodyMedium?.copyWith(
              decoration: TextDecoration.lineThrough,
              color: Theme.of(context).colorScheme.onSurfaceVariant,
            ),
          ),
      ],
    ),
  );
}

// ── CartSummary ─────────────────────────────────────────────────────────────

/// `CartSummary`: the lines, subtotal, tax and total of a cart, then the
/// `checkout` slot or a button that fires `onCheckout`.
Widget buildCartSummary(NodeContext c) {
  final lines =
      c.decode(
        _cart.p('lines'),
        (d, v) => asList<(String, Money)>(d, v, (d, x) {
          final f = Fields.of(d, x);
          if (f == null) return null;
          final label = asString(d, f.get(1, 'label'));
          final amount = _asMoney(d, f.get(2, 'amount'));
          return label == null || amount == null ? null : (label, amount);
        }),
      ) ??
      c.missing('CartSummary.lines');
  final subtotal =
      c.decode(_cart.p('subtotal'), _asMoney) ??
      c.missing('CartSummary.subtotal');
  final total =
      c.decode(_cart.p('total'), _asMoney) ?? c.missing('CartSummary.total');
  final tax = c.decode(_cart.p('tax'), _asMoney);
  final labels = (
    c.decode(_cart.p('subtotalLabel'), asString) ??
        RuntimeStrings.of(c.context)['plux.cart.subtotal'],
    c.decode(_cart.p('taxLabel'), asString) ??
        RuntimeStrings.of(c.context)['plux.cart.tax'],
    c.decode(_cart.p('totalLabel'), asString) ??
        RuntimeStrings.of(c.context)['plux.cart.total'],
  );
  final checkout =
      c.slot(_cart.s('checkout')) ??
      (c.handles(_cart.e('onCheckout'))
          ? FilledButton(
              onPressed: () => c.fire(_cart.e('onCheckout')),
              child: Text(
                c.decode(_cart.p('checkoutLabel'), asString) ??
                    RuntimeStrings.of(c.context)['plux.cart.checkout'],
              ),
            )
          : null);
  final context = c.context;
  Widget row(String label, Money m, {TextStyle? style}) => MergeSemantics(
    child: Padding(
      padding: const EdgeInsets.symmetric(vertical: 4),
      child: Row(
        children: [
          Expanded(child: Text(label, style: style)),
          Semantics(
            label: _spoken(context, m),
            excludeSemantics: true,
            child: Text(_money(context, m), style: style),
          ),
        ],
      ),
    ),
  );
  final bold = Theme.of(context).textTheme.titleMedium;
  return Semantics(
    container: true,
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      mainAxisSize: MainAxisSize.min,
      children: [
        for (final (label, amount) in lines) row(label, amount),
        const Divider(),
        row(labels.$1, subtotal),
        if (tax != null) row(labels.$2, tax),
        row(labels.$3, total, style: bold),
        if (checkout != null)
          Padding(padding: const EdgeInsets.only(top: 8), child: checkout),
      ],
    ),
  );
}

// ── TransactionListItem ─────────────────────────────────────────────────────

/// `TransactionListItem`: a row with the title, subtitle, date and status,
/// and the signed amount in the theme's tertiary colour for a credit and its
/// error colour for a debit.
Widget buildTransactionListItem(NodeContext c) {
  final title =
      c.decode(_transaction.p('title'), asString) ??
      c.missing('TransactionListItem.title');
  final amount =
      c.decode(_transaction.p('amount'), _asMoney) ??
      c.missing('TransactionListItem.amount');
  final subtitle = c.decode(_transaction.p('subtitle'), asString);
  final status =
      c.decode(_transaction.p('status'), nameTransactionStatus) ?? 'completed';
  final statusText =
      c.decode(_transaction.p('statusLabel'), asString) ?? status;
  final when = c.decode(_transaction.p('date'), _asDateTime);
  final context = c.context;
  final theme = Theme.of(context);
  final scheme = theme.colorScheme;
  final credit = amount.amount.sign > 0;
  final tone = credit ? scheme.tertiary : scheme.error;
  final failed = status == 'failed' || status == 'reversed';
  final tap = c.handles(_transaction.e('onTap'))
      ? () => c.fire(_transaction.e('onTap'))
      : null;
  final date = when == null ? null : _date(context, when);
  final money = _money(context, amount, sign: 'always');
  final details = [?subtitle, ?date].join(' · ');
  return Semantics(
    container: true,
    button: tap != null,
    onTap: tap,
    label: [
      title,
      ?subtitle,
      _spoken(context, amount),
      statusText,
      ?date,
    ].join(', '),
    excludeSemantics: true,
    child: InkWell(
      onTap: tap,
      child: ConstrainedBox(
        constraints: const BoxConstraints(minHeight: 56),
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
          child: Row(
            children: [
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(title, style: theme.textTheme.bodyLarge),
                    if (details.isNotEmpty)
                      Text(details, style: theme.textTheme.bodySmall),
                  ],
                ),
              ),
              Column(
                crossAxisAlignment: CrossAxisAlignment.end,
                children: [
                  Text(
                    money,
                    textDirection: TextDirection.ltr,
                    style: theme.textTheme.bodyLarge?.copyWith(
                      color: tone,
                      decoration: failed ? TextDecoration.lineThrough : null,
                    ),
                  ),
                  Text(
                    statusText,
                    style: theme.textTheme.bodySmall?.copyWith(
                      color: failed ? scheme.error : null,
                    ),
                  ),
                ],
              ),
            ],
          ),
        ),
      ),
    ),
  );
}

// ── Cards ───────────────────────────────────────────────────────────────────

/// `AccountCard`: the account's name, masked number and balance; tapping
/// flips it and fires `onFlip(flipped)`.
Widget buildAccountCard(NodeContext c) {
  final name =
      c.decode(_account.p('name'), asString) ?? c.missing('AccountCard.name');
  final number =
      c.decode(_account.p('maskedNumber'), asString) ??
      c.missing('AccountCard.maskedNumber');
  final balance =
      c.decode(_account.p('balance'), _asMoney) ??
      c.missing('AccountCard.balance');
  final back = c.decode(_account.p('backLabel'), asString);
  final context = c.context;
  final style = Theme.of(context).textTheme;
  return _FlipCard(
    flipped: c.decode(_account.p('flipped'), asBool) ?? false,
    label: [name, _ending(number), _spoken(context, balance)].join(', '),
    onFlip: (v) => c.fire(_account.e('onFlip'), v),
    front: (context, on) => [
      Text(name, style: style.titleMedium?.copyWith(color: on)),
      const SizedBox(height: 16),
      Text(
        number,
        textDirection: TextDirection.ltr,
        style: style.titleLarge?.copyWith(color: on),
      ),
      const SizedBox(height: 16),
      Text(
        _money(context, balance),
        style: style.headlineSmall?.copyWith(color: on),
      ),
    ],
    back: (context, on) => [
      Text(back ?? name, style: style.titleMedium?.copyWith(color: on)),
    ],
  );
}

/// `PaymentCard`: brand, masked number, expiry and holder; tapping flips it
/// and fires `onFlip(flipped)`.
Widget buildPaymentCard(NodeContext c) {
  final brand = c.decode(_payment.p('brand'), nameCardBrand) ?? 'other';
  final number =
      c.decode(_payment.p('maskedNumber'), asString) ??
      c.missing('PaymentCard.maskedNumber');
  final expiry =
      c.decode(_payment.p('expiry'), asString) ??
      c.missing('PaymentCard.expiry');
  final holder =
      c.decode(_payment.p('holder'), asString) ??
      c.missing('PaymentCard.holder');
  final back = c.decode(_payment.p('backLabel'), asString);
  final style = Theme.of(c.context).textTheme;
  return _FlipCard(
    flipped: c.decode(_payment.p('flipped'), asBool) ?? false,
    label: [brand, _ending(number), expiry, holder].join(', '),
    onFlip: (v) => c.fire(_payment.e('onFlip'), v),
    front: (context, on) => [
      Text(
        brand.toUpperCase(),
        style: style.titleMedium?.copyWith(
          color: on,
          fontWeight: FontWeight.bold,
        ),
      ),
      const SizedBox(height: 16),
      Text(
        number,
        textDirection: TextDirection.ltr,
        style: style.titleLarge?.copyWith(color: on),
      ),
      const SizedBox(height: 16),
      Row(
        children: [
          Expanded(
            child: Text(holder, style: style.bodyMedium?.copyWith(color: on)),
          ),
          Text(
            expiry,
            textDirection: TextDirection.ltr,
            style: style.bodyMedium?.copyWith(color: on),
          ),
        ],
      ),
    ],
    back: (context, on) => [
      Container(height: 40, color: Colors.black87),
      if (back != null) ...[
        const SizedBox(height: 16),
        Text(back, style: style.bodyMedium?.copyWith(color: on)),
      ],
    ],
  );
}

/// A card with a front and a back that turns about its vertical axis; with
/// animations disabled (reduce motion) it swaps at once.
final class _FlipCard extends StatefulWidget {
  const _FlipCard({
    required this.flipped,
    required this.label,
    required this.onFlip,
    required this.front,
    required this.back,
  });

  final bool flipped;
  final String label;
  final ValueChanged<bool> onFlip;
  final List<Widget> Function(BuildContext context, Color on) front;
  final List<Widget> Function(BuildContext context, Color on) back;

  @override
  State<_FlipCard> createState() => _FlipCardState();
}

final class _FlipCardState extends State<_FlipCard> {
  late bool _flipped = widget.flipped;

  @override
  void didUpdateWidget(_FlipCard old) {
    super.didUpdateWidget(old);
    if (old.flipped != widget.flipped) _flipped = widget.flipped;
  }

  void _toggle() {
    setState(() => _flipped = !_flipped);
    widget.onFlip(_flipped);
  }

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final duration = MediaQuery.disableAnimationsOf(context)
        ? Duration.zero
        : const Duration(milliseconds: 400);
    return Semantics(
      container: true,
      button: true,
      label: widget.label,
      onTap: _toggle,
      excludeSemantics: true,
      child: TweenAnimationBuilder<double>(
        tween: Tween(end: _flipped ? math.pi : 0),
        duration: duration,
        builder: (context, angle, _) {
          final back = angle > math.pi / 2;
          final children = back
              ? widget.back(context, scheme.onPrimary)
              : widget.front(context, scheme.onPrimary);
          Widget face = Container(
            constraints: const BoxConstraints(minHeight: 180, maxWidth: 360),
            padding: const EdgeInsets.all(20),
            decoration: BoxDecoration(
              borderRadius: BorderRadius.circular(16),
              gradient: LinearGradient(
                colors: [scheme.primary, scheme.secondary],
              ),
            ),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              mainAxisSize: MainAxisSize.min,
              children: children,
            ),
          );
          if (back) {
            face = Transform(
              alignment: Alignment.center,
              transform: Matrix4.rotationY(math.pi),
              child: face,
            );
          }
          return GestureDetector(
            onTap: _toggle,
            child: Transform(
              alignment: Alignment.center,
              transform: Matrix4.identity()
                ..setEntry(3, 2, 0.001)
                ..rotateY(angle),
              child: face,
            ),
          );
        },
      ),
    );
  }
}
