// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The builder of `DualCalendarDatePicker` (I18N-003, ADR-0067): a month grid
/// in one calendar with the day numbers of a second calendar in small text.
/// The ID comes from the generated registry (BND-011).
library;

import 'package:flutter/material.dart';
import 'package:plux_flutter/src/l10n/calendar/calendar.dart';
import 'package:plux_flutter/src/pxl/values.dart';
import 'package:plux_flutter/src/render/generated/render.g.dart';
import 'package:plux_flutter/src/render/node_context.dart';
import 'package:plux_flutter/src/schema/registry.g.dart';

final WidgetDescriptor _picker = widgetDescriptors.firstWhere(
  (d) => d.type == 'DualCalendarDatePicker',
);

/// The calendar widgets' builders, by permanent widget ID.
final Map<int, NodeBuilder> calendarBuilders = {
  _picker.id: buildDualCalendarDatePicker,
};

/// The JDN of 1970-01-01, day 0 of a PXL date.
const int _unixEpochJdn = 2440588;

/// The default bounds when `firstDate` or `lastDate` is absent: 1900-01-01
/// and 2100-12-31.
final int _defaultFirst = _jdnOf(1900, 1, 1);
final int _defaultLast = _jdnOf(2100, 12, 31);

int _jdnOf(int y, int m, int d) =>
    calendarSystem(gregorianId)!.toJdn(CalendarYmd(y, m, d));

/// The calendar system id of an enum member name: the enum spells
/// `islamic-umalqura` as `islamicUmalqura`, as member names are identifiers.
String _systemId(String? member) =>
    member == 'islamicUmalqura' ? islamicUmalquraId : member ?? gregorianId;

/// `DualCalendarDatePicker`: the month grid of `calendar` with the day
/// numbers of `secondaryCalendar` beside them.
Widget buildDualCalendarDatePicker(NodeContext c) {
  final p = _picker.props, e = _picker.events;
  PxlDate? date(String name) {
    final v = c.prop(p[name]!);
    return v is PxlDate ? v : null;
  }

  final primary = _systemId(c.decode(p['calendar']!, nameCalendarSystem));
  final secondary = c.decode(p['secondaryCalendar']!, nameCalendarSystem);
  final first = date('firstDate');
  final last = date('lastDate');
  return c.controlled<PxlDate?>(e['onChanged']!, date('value'), (
    value,
    onChanged,
  ) {
    return DualCalendarView(
      primary: calendarSystem(primary)!,
      secondary: secondary == null
          ? null
          : calendarSystem(_systemId(secondary)),
      value: value == null ? null : value.days + _unixEpochJdn,
      firstJdn: first == null ? _defaultFirst : first.days + _unixEpochJdn,
      lastJdn: last == null ? _defaultLast : last.days + _unixEpochJdn,
      onChanged: onChanged == null
          ? null
          : (jdn) => onChanged(PxlDate(jdn - _unixEpochJdn)),
    );
  });
}

// English month names as CLDR spells them. The runtime holds no locale data
// for the picker yet, so the names are English in every locale; weekday
// letters and the full Gregorian date come from MaterialLocalizations.
const Map<String, List<String>> _monthNames = {
  gregorianId: [
    'January',
    'February',
    'March',
    'April',
    'May',
    'June',
    'July',
    'August',
    'September',
    'October',
    'November',
    'December',
  ],
  ethiopicId: [
    'Meskerem',
    'Tekemt',
    'Hedar',
    'Tahsas',
    'Ter',
    'Yekatit',
    'Megabit',
    'Miazia',
    'Genbot',
    'Sene',
    'Hamle',
    'Nehasse',
    'Pagumen',
  ],
  islamicUmalquraId: [
    'Muharram',
    'Safar',
    'Rabiʻ I',
    'Rabiʻ II',
    'Jumada I',
    'Jumada II',
    'Rajab',
    'Shaʻban',
    'Ramadan',
    'Shawwal',
    'Dhuʻl-Qiʻdah',
    'Dhuʻl-Hijjah',
  ],
  persianId: [
    'Farvardin',
    'Ordibehesht',
    'Khordad',
    'Tir',
    'Mordad',
    'Shahrivar',
    'Mehr',
    'Aban',
    'Azar',
    'Dey',
    'Bahman',
    'Esfand',
  ],
};

const Map<String, String> _eraNames = {
  ethiopicId: 'AM|AA',
  islamicUmalquraId: 'AH|BH',
  persianId: 'AP|AP',
  gregorianId: 'BC|AD',
};

/// The month grid. [value], [firstJdn] and [lastJdn] are Julian day numbers.
class DualCalendarView extends StatefulWidget {
  /// Creates the picker.
  const DualCalendarView({
    super.key,
    required this.primary,
    required this.secondary,
    required this.value,
    required this.firstJdn,
    required this.lastJdn,
    required this.onChanged,
  });

  /// The calendar of the grid.
  final CalendarSystem primary;

  /// The calendar of the small day numbers, if any.
  final CalendarSystem? secondary;

  /// The selected day.
  final int? value;

  /// The earliest day that can be chosen.
  final int firstJdn;

  /// The latest day that can be chosen.
  final int lastJdn;

  /// Called with the day chosen, or null when the picker is read only.
  final ValueChanged<int>? onChanged;

  @override
  State<DualCalendarView> createState() => _DualCalendarViewState();
}

class _DualCalendarViewState extends State<DualCalendarView> {
  late int _year;
  late int _month;

  @override
  void initState() {
    super.initState();
    _show(widget.value ?? widget.firstJdn);
  }

  @override
  void didUpdateWidget(DualCalendarView old) {
    super.didUpdateWidget(old);
    if (old.primary != widget.primary) _show(widget.value ?? widget.firstJdn);
  }

  void _show(int jdn) {
    final d = widget.primary.fromJdn(jdn);
    _year = d.year;
    _month = d.month;
  }

  int _monthStart(int year, int month) =>
      widget.primary.toJdn(CalendarYmd(year, month, 1));

  int _monthEnd(int year, int month) =>
      _monthStart(year, month) + widget.primary.daysInMonth(year, month) - 1;

  bool get _canPrev => _monthStart(_year, _month) > widget.firstJdn;

  bool get _canNext => _monthEnd(_year, _month) < widget.lastJdn;

  void _step(int by) {
    setState(() {
      _show(_monthStart(_year, _month) + (by < 0 ? -1 : _daysHere()));
    });
  }

  int _daysHere() => widget.primary.daysInMonth(_year, _month);

  String _monthName(MaterialLocalizations l, CalendarSystem s, int m) =>
      _monthNames[s.id]![m - 1];

  /// The era abbreviation with a leading space; none for Gregorian years in
  /// the common era.
  String _era(CalendarSystem s, int year) {
    final e = s.eraOf(year).era;
    if (s.id == gregorianId && e == 1) return '';
    return ' ${_eraNames[s.id]!.split('|')[e]}';
  }

  String _dayLabel(MaterialLocalizations l, CalendarSystem s, int jdn) {
    final d = s.fromJdn(jdn);
    final e = s.eraOf(d.year);
    if (s.id == gregorianId) {
      final g = DateTime.utc(d.year, d.month, d.day);
      return l.formatFullDate(g);
    }
    return '${d.day} ${_monthName(l, s, d.month)} ${e.year}${_era(s, d.year)}';
  }

  @override
  Widget build(BuildContext context) {
    final l = MaterialLocalizations.of(context);
    final theme = Theme.of(context);
    final rtl = Directionality.of(context) == TextDirection.rtl;
    final s = widget.primary;
    final sec = widget.secondary;
    final start = _monthStart(_year, _month);
    final days = s.daysInMonth(_year, _month);
    final lead = (calendarWeekday(start) - l.firstDayOfWeekIndex + 7) % 7;
    final rows = (lead + days + 6) ~/ 7;
    final eraYear = s.eraOf(_year).year;
    final title = '${_monthName(l, s, _month)} $eraYear${_era(s, _year)}';
    final secondaryTitle = sec == null
        ? null
        : _secondaryTitle(l, sec, start, start + days - 1);
    return Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Row(
          children: [
            IconButton(
              tooltip: l.previousMonthTooltip,
              onPressed: _canPrev ? () => _step(-1) : null,
              icon: Icon(rtl ? Icons.chevron_right : Icons.chevron_left),
            ),
            Expanded(
              child: Semantics(
                header: true,
                liveRegion: true,
                child: Column(
                  children: [
                    Text(
                      title,
                      textAlign: TextAlign.center,
                      style: theme.textTheme.titleMedium,
                    ),
                    if (secondaryTitle != null)
                      Text(
                        secondaryTitle,
                        textAlign: TextAlign.center,
                        style: theme.textTheme.bodySmall,
                      ),
                  ],
                ),
              ),
            ),
            IconButton(
              tooltip: l.nextMonthTooltip,
              onPressed: _canNext ? () => _step(1) : null,
              icon: Icon(rtl ? Icons.chevron_left : Icons.chevron_right),
            ),
          ],
        ),
        Row(
          children: [
            for (var i = 0; i < 7; i++)
              Expanded(
                child: ExcludeSemantics(
                  child: Text(
                    l.narrowWeekdays[(l.firstDayOfWeekIndex + i) % 7],
                    textAlign: TextAlign.center,
                    style: theme.textTheme.labelMedium,
                  ),
                ),
              ),
          ],
        ),
        for (var r = 0; r < rows; r++)
          Row(
            children: [
              for (var i = 0; i < 7; i++)
                Expanded(
                  child: _cell(context, l, r * 7 + i - lead, start, days),
                ),
            ],
          ),
      ],
    );
  }

  /// The secondary calendar's span over the month, such as `Sha'ban –
  /// Ramadan 1445 AH`.
  String _secondaryTitle(
    MaterialLocalizations l,
    CalendarSystem sec,
    int from,
    int to,
  ) {
    final a = sec.fromJdn(from), b = sec.fromJdn(to);
    final ea = sec.eraOf(a.year), eb = sec.eraOf(b.year);
    String name(CalendarSystem s, int m) => _monthName(l, s, m);
    if (a.year == b.year && a.month == b.month) {
      return '${name(sec, a.month)} ${ea.year}${_era(sec, a.year)}';
    }
    if (a.year == b.year) {
      return '${name(sec, a.month)} – ${name(sec, b.month)} '
          '${ea.year}${_era(sec, a.year)}';
    }
    return '${name(sec, a.month)} ${ea.year} – ${name(sec, b.month)} '
        '${eb.year}${_era(sec, b.year)}';
  }

  Widget _cell(
    BuildContext context,
    MaterialLocalizations l,
    int index,
    int start,
    int days,
  ) {
    if (index < 0 || index >= days) return const SizedBox.shrink();
    final theme = Theme.of(context);
    final jdn = start + index;
    final selected = jdn == widget.value;
    final enabled =
        jdn >= widget.firstJdn &&
        jdn <= widget.lastJdn &&
        widget.onChanged != null;
    final sec = widget.secondary;
    final fg = selected
        ? theme.colorScheme.onPrimary
        : enabled
        ? theme.colorScheme.onSurface
        : theme.colorScheme.onSurface.withValues(alpha: 0.38);
    final label = sec == null
        ? _dayLabel(l, widget.primary, jdn)
        : '${_dayLabel(l, widget.primary, jdn)}, ${_dayLabel(l, sec, jdn)}';
    return Semantics(
      button: true,
      selected: selected,
      enabled: enabled,
      label: label,
      onTap: enabled ? () => widget.onChanged!(jdn) : null,
      excludeSemantics: true,
      child: InkWell(
        onTap: enabled ? () => widget.onChanged!(jdn) : null,
        customBorder: const CircleBorder(),
        child: ConstrainedBox(
          constraints: const BoxConstraints(minHeight: 48, minWidth: 48),
          child: DecoratedBox(
            decoration: BoxDecoration(
              shape: BoxShape.circle,
              color: selected ? theme.colorScheme.primary : null,
            ),
            child: Center(
              child: Column(
                mainAxisSize: MainAxisSize.min,
                children: [
                  Text(
                    '${widget.primary.fromJdn(jdn).day}',
                    style: theme.textTheme.bodyMedium?.copyWith(color: fg),
                  ),
                  if (sec != null)
                    Text(
                      '${sec.fromJdn(jdn).day}',
                      style: theme.textTheme.labelSmall?.copyWith(color: fg),
                    ),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}
