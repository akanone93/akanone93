// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

part of 'locale_data.dart';

/// A date and time of day in some calendar, already converted by the caller:
/// the engine formats the fields it is given and does no calendar arithmetic.
/// [era] indexes the calendar's era names (Gregorian: 0 is BC, 1 is AD),
/// [year] is the year within the era, [month] starts at 1 (the Ethiopic
/// calendar has 13), [weekday] is 0 for Sunday to 6 for Saturday, and [hour]
/// is 0 to 23.
final class PluxCalendarDate {
  /// Creates a date from its fields.
  const PluxCalendarDate({
    required this.era,
    required this.year,
    required this.month,
    required this.day,
    required this.weekday,
    this.hour = 0,
    this.minute = 0,
    this.second = 0,
  });

  /// The date and time of day of a proleptic Gregorian civil date, with its
  /// weekday. Years before 1 are BC (year 0 is 1 BC).
  factory PluxCalendarDate.gregorian(
    int year,
    int month,
    int day, [
    int hour = 0,
    int minute = 0,
    int second = 0,
  ]) {
    // Days from the civil epoch (Howard Hinnant's algorithm); 1970-01-01 was
    // a Thursday.
    final y = month <= 2 ? year - 1 : year;
    final era = y >= 0 ? y ~/ 400 : (y - 399) ~/ 400;
    final yoe = y - era * 400;
    final mp = (month + 9) % 12;
    final doy = (153 * mp + 2) ~/ 5 + day - 1;
    final doe = yoe * 365 + yoe ~/ 4 - yoe ~/ 100 + doy;
    final days = era * 146097 + doe - 719468;
    return PluxCalendarDate(
      era: year < 1 ? 0 : 1,
      year: year < 1 ? 1 - year : year,
      month: month,
      day: day,
      weekday: ((days + 4) % 7 + 7) % 7,
      hour: hour,
      minute: minute,
      second: second,
    );
  }

  /// The era index.
  final int era;

  /// The year within the era.
  final int year;

  /// The month, from 1.
  final int month;

  /// The day of the month, from 1.
  final int day;

  /// The weekday, 0 for Sunday.
  final int weekday;

  /// The hour, 0 to 23.
  final int hour;

  /// The minute, 0 to 59.
  final int minute;

  /// The second, 0 to 59.
  final int second;

  @override
  String toString() =>
      'PluxCalendarDate(era $era, $year-$month-$day, weekday $weekday, '
      '$hour:$minute:$second)';
}

/// Selects the format of [PluxLocaleData.formatDate]: [dateStyle] and
/// [timeStyle] (`full`, `long`, `medium`, `short`), or else a CLDR [skeleton]
/// such as `yMMMd` or `yMMMEdhm`.
final class PluxDateOptions {
  /// Creates the options.
  const PluxDateOptions({
    this.dateStyle = '',
    this.timeStyle = '',
    this.skeleton = '',
    this.numberingSystem = '',
    this.zoneName = '',
  });

  /// The date style, or empty.
  final String dateStyle;

  /// The time style, or empty.
  final String timeStyle;

  /// A CLDR skeleton, used when no style is given.
  final String skeleton;

  /// Empty or `latn` for Latin digits, or `default` or the locale's own
  /// system id for native digits.
  final String numberingSystem;

  /// What time zone pattern letters (z, v, V, O, Z, x, X) write; the engine
  /// knows no zones.
  final String zoneName;
}

const _styles = ['full', 'long', 'medium', 'short'];

/// One calendar's names and formats.
final class _Calendar {
  _Calendar._(this.json, this.avail);

  factory _Calendar.compile(Map<String, Object?> json) {
    for (final k in _styles) {
      if (_string(_map(json, 'dateFormats'), k).isEmpty ||
          _string(_map(json, 'timeFormats'), k).isEmpty ||
          _string(_map(json, 'dateTimeFormats'), k).isEmpty) {
        throw L10nException(L10nError.invalidData, 'format $k is missing');
      }
    }
    final months = _map(_map(json, 'months'), 'format');
    final days = _map(_map(json, 'days'), 'format');
    final eras = _map(json, 'eras');
    if ((eras['abbreviated'] as List?)?.isNotEmpty != true ||
        (months['wide'] as List?)?.isNotEmpty != true ||
        (days['wide'] as List?)?.length != 7) {
      throw const L10nException(L10nError.invalidData, 'names are missing');
    }
    final avail = <_AvailableFormat>[
      for (final e in _map(json, 'availableFormats').entries)
        _AvailableFormat(e.key, _Skeleton.parse(e.key), e.value! as String),
    ]..sort((a, b) => a.key.compareTo(b.key));
    return _Calendar._(json, avail);
  }

  final Map<String, Object?> json;
  final List<_AvailableFormat> avail;

  Map<String, Object?> get _dateFormats => _map(json, 'dateFormats');
  Map<String, Object?> get _timeFormats => _map(json, 'timeFormats');
  Map<String, Object?> get _dateTimeFormats => _map(json, 'dateTimeFormats');
  Map<String, Object?> get _atTime => _map(json, 'dateTimeAtTime');

  /// Returns the date-time pattern the options select.
  String pattern(PluxDateOptions opts) {
    if (opts.dateStyle.isNotEmpty || opts.timeStyle.isNotEmpty) {
      if (opts.skeleton.isNotEmpty) {
        throw const L10nException(
          L10nError.invalidOption,
          'both styles and a skeleton',
        );
      }
      var date = '';
      var time = '';
      if (opts.dateStyle.isNotEmpty) {
        date = _string(_dateFormats, opts.dateStyle);
        if (date.isEmpty) {
          throw L10nException(
            L10nError.unsupported,
            'date style "${opts.dateStyle}"',
          );
        }
      }
      if (opts.timeStyle.isNotEmpty) {
        time = _string(_timeFormats, opts.timeStyle);
        if (time.isEmpty) {
          throw L10nException(
            L10nError.unsupported,
            'time style "${opts.timeStyle}"',
          );
        }
      }
      if (date.isEmpty || time.isEmpty) return date + time;
      var glue = _string(_atTime, opts.dateStyle);
      if (glue.isEmpty) glue = _string(_dateTimeFormats, opts.dateStyle);
      return _fillGlue(glue, date, time);
    }
    if (opts.skeleton.isEmpty) {
      throw const L10nException(
        L10nError.invalidOption,
        'neither a style nor a skeleton',
      );
    }
    final want = _Skeleton.parse(opts.skeleton);
    final (dateSk, timeSk) = want.split();
    if (timeSk.isEmpty) return _match(dateSk);
    if (dateSk.isEmpty) return _match(timeSk);
    final date = _match(dateSk);
    final time = _match(timeSk);
    var style = 'short';
    final m = dateSk.count('M');
    if (m >= 4) {
      style = dateSk.has('E') ? 'full' : 'long';
    } else if (m == 3) {
      style = 'medium';
    }
    var glue = _string(_atTime, style);
    if (glue.isEmpty) glue = _string(_dateTimeFormats, style);
    return _fillGlue(glue, date, time);
  }

  /// Finds the pattern for a skeleton of date fields or of time fields: the
  /// available format whose fields are the same, preferring the closest
  /// widths, with its field widths adjusted to the request the way CLDR's
  /// DateTimePatternGenerator does.
  String _match(_Skeleton want) {
    var best = -1;
    var bestDist = 0;
    for (var i = 0; i < avail.length; i++) {
      final dist = _distance(want, avail[i].fields);
      if (dist == null) continue;
      if (best < 0 || dist < bestDist) {
        best = i;
        bestDist = dist;
      }
    }
    if (best < 0) {
      throw const L10nException(
        L10nError.unsupported,
        'no format for the requested fields',
      );
    }
    final a = avail[best];
    final toks = _tokenize(a.pattern);
    for (final t in toks) {
      if (t.letter.isEmpty) continue;
      final key = _fieldOf(t.letter);
      if (key == null) continue;
      final req = want.fields[key];
      if (req == null) continue;
      t.n = _adjustedWidth(key, t, req, a.fields.fields[key]!);
      if (key == 'h') t.letter = req.letter; // h for K, as ICU does
    }
    return _untokenize(toks);
  }
}

String _fillGlue(String glue, String date, String time) =>
    glue.replaceAll('{0}', time).replaceAll('{1}', date);

final class _AvailableFormat {
  const _AvailableFormat(this.key, this.fields, this.pattern);

  final String key;
  final _Skeleton fields;
  final String pattern;
}

/// A field of a skeleton: the letter as requested and its repeat count.
final class _SkelField {
  const _SkelField(this.letter, this.count);

  final String letter;
  final int count;
}

/// A parsed CLDR skeleton: the field of each normalised key. L reads as M, c
/// as E, Y as y, and the hour letters h K H k are one field; a (the day
/// period) is implied by the hour and not kept.
final class _Skeleton {
  const _Skeleton(this.fields);

  factory _Skeleton.parse(String s) {
    final out = <String, _SkelField>{};
    var i = 0;
    while (i < s.length) {
      var j = i;
      while (j < s.length && s[j] == s[i]) {
        j++;
      }
      final letter = s[i];
      final n = j - i;
      i = j;
      final String key;
      var kept = letter;
      switch (letter) {
        case 'G' || 'y' || 'M' || 'E' || 'd' || 'm' || 's':
          key = letter;
        case 'L':
          key = 'M';
          kept = 'M';
        case 'c':
          key = 'E';
          kept = 'E';
        case 'Y':
          key = 'y';
          kept = 'y';
        case 'h' || 'K' || 'H' || 'k':
          key = 'h';
        case 'a':
          continue;
        default:
          throw L10nException(
            L10nError.unsupported,
            'skeleton field "$letter" in "$s"',
          );
      }
      if (out.containsKey(key)) {
        throw L10nException(
          L10nError.invalidOption,
          'skeleton "$s" repeats a field',
        );
      }
      out[key] = _SkelField(kept, n);
    }
    if (out.isEmpty) {
      throw const L10nException(L10nError.invalidOption, 'empty skeleton');
    }
    return _Skeleton(out);
  }

  final Map<String, _SkelField> fields;

  bool get isEmpty => fields.isEmpty;

  bool has(String k) => fields.containsKey(k);

  int count(String k) => fields[k]?.count ?? 0;

  /// Separates the date fields from the time fields.
  (_Skeleton, _Skeleton) split() {
    final date = <String, _SkelField>{};
    final time = <String, _SkelField>{};
    fields.forEach((k, f) {
      if (k == 'h' || k == 'm' || k == 's') {
        time[k] = f;
      } else {
        date[k] = f;
      }
    });
    return (_Skeleton(date), _Skeleton(time));
  }
}

/// Maps a pattern letter to its skeleton field.
String? _fieldOf(String letter) => switch (letter) {
  'G' || 'y' || 'M' || 'E' || 'd' || 'm' || 's' => letter,
  'L' => 'M',
  'c' => 'E',
  'Y' => 'y',
  'h' || 'H' || 'K' || 'k' => 'h',
  _ => null,
};

bool _is24Hour(String letter) => letter == 'H' || letter == 'k';

/// Compares a request with an available format's skeleton; null when their
/// fields differ.
int? _distance(_Skeleton want, _Skeleton have) {
  if (want.fields.length != have.fields.length) return null;
  var dist = 0;
  for (final e in want.fields.entries) {
    final h = have.fields[e.key];
    if (h == null) return null;
    final w = e.value;
    if (e.key == 'h' && _is24Hour(w.letter) != _is24Hour(h.letter)) {
      return null;
    }
    var d = (w.count - h.count).abs();
    if (e.key == 'M' && (w.count <= 2) != (h.count <= 2)) d += 100;
    dist += d;
  }
  return dist;
}

/// The field width the matched pattern gets (ICU's adjustFieldTypes with the
/// hour-length matching ECMA-402 asks for): minutes and seconds keep the
/// pattern's width; so does any field whose skeleton already had the
/// requested width, or whose pattern and skeleton disagree on numeric against
/// text. Otherwise the requested width wins.
int _adjustedWidth(String key, _Token t, _SkelField req, _SkelField have) {
  if (key == 'm' || key == 's') return t.n;
  if (have.count == req.count) return t.n;
  if (_isNumeric(key, t.n) != _isNumeric(key, have.count)) return t.n;
  return req.count;
}

bool _isNumeric(String key, int n) => switch (key) {
  'y' || 'd' || 'h' || 'm' || 's' => true,
  'M' => n <= 2,
  _ => false,
};

/// A pattern field (letter and repeat count) or literal text.
final class _Token {
  _Token.field(this.letter, this.n) : text = '';
  _Token.text(this.text) : letter = '', n = 0;

  String letter;
  int n;
  final String text;
}

bool _isAsciiLetter(int c) =>
    (c >= 0x61 && c <= 0x7a) || (c >= 0x41 && c <= 0x5a);

/// Splits a CLDR date pattern into fields and literals; quotes protect text
/// and `''` is an apostrophe.
List<_Token> _tokenize(String p) {
  final out = <_Token>[];
  final text = StringBuffer();
  void flush() {
    if (text.isNotEmpty) {
      out.add(_Token.text(text.toString()));
      text.clear();
    }
  }

  var i = 0;
  while (i < p.length) {
    final c = p.codeUnitAt(i);
    if (c == 0x27) {
      if (i + 1 < p.length && p.codeUnitAt(i + 1) == 0x27) {
        text.write("'");
        i += 2;
        continue;
      }
      var j = i + 1;
      while (j < p.length) {
        if (p.codeUnitAt(j) == 0x27) {
          if (j + 1 < p.length && p.codeUnitAt(j + 1) == 0x27) {
            text.write("'");
            j += 2;
            continue;
          }
          break;
        }
        text.write(p[j]);
        j++;
      }
      if (j == p.length) {
        throw L10nException(
          L10nError.invalidData,
          'unterminated quote in "$p"',
        );
      }
      i = j + 1;
    } else if (_isAsciiLetter(c)) {
      flush();
      var j = i;
      while (j < p.length && p.codeUnitAt(j) == c) {
        j++;
      }
      out.add(_Token.field(p[i], j - i));
      i = j;
    } else {
      text.write(p[i]);
      i++;
    }
  }
  flush();
  return out;
}

/// Writes tokens back as a pattern, quoting literal text that holds letters.
String _untokenize(List<_Token> toks) {
  final b = StringBuffer();
  for (final t in toks) {
    if (t.letter.isNotEmpty) {
      b.write(t.letter * t.n);
      continue;
    }
    var text = t.text.replaceAll("'", "''");
    if (RegExp('[a-zA-Z]').hasMatch(t.text)) text = "'$text'";
    b.write(text);
  }
  return b.toString();
}

/// Names of a width, falling back to wider widths when a calendar carries no
/// narrow names.
String? _name(Map<String, Object?> byWidth, String width, int i) {
  for (final w in [width, 'abbreviated', 'wide']) {
    final names = byWidth[w] as List<Object?>?;
    if (names != null && i >= 0 && i < names.length) return names[i]! as String;
  }
  return null;
}

/// Maps a count of pattern letters to a name width (E, M, G, a).
String _widthOf(int n) => n == 4
    ? 'wide'
    : n >= 5
    ? 'narrow'
    : 'abbreviated';

/// The date formatting method of [PluxLocaleData].
extension PluxDateFormatting on PluxLocaleData {
  /// Formats [d] in a calendar (`gregorian`, `ethiopic`, `islamic-umalqura`
  /// or `persian`) from the locale's date and time styles or skeleton
  /// (I18N-001). Pattern letters G y M L d E c a h H K k m s are implemented;
  /// Y reads as y, B as a, and the zone letters write
  /// [PluxDateOptions.zoneName].
  String formatDate(
    PluxCalendarDate d,
    String calendar, [
    PluxDateOptions opts = const PluxDateOptions(),
  ]) {
    final cal = _calendars[calendar];
    if (cal == null) {
      throw L10nException(
        L10nError.unsupported,
        'calendar "$calendar" for locale $id',
      );
    }
    final (sysId, _) = _system(opts.numberingSystem);
    final toks = _tokenize(cal.pattern(opts));
    if (d.day < 1 ||
        d.day > 31 ||
        d.weekday < 0 ||
        d.weekday > 6 ||
        d.hour < 0 ||
        d.hour > 23 ||
        d.minute < 0 ||
        d.minute > 59 ||
        d.second < 0 ||
        d.second > 59 ||
        d.year < 0) {
      throw L10nException(L10nError.invalidOption, 'date $d');
    }
    final b = StringBuffer();
    for (final t in toks) {
      b.write(t.letter.isEmpty ? t.text : _dateField(cal, d, t, sysId, opts));
    }
    return b.toString();
  }

  String _num(int v, int width, String sys) {
    var s = v.toString();
    if (s.length < width) s = '0' * (width - s.length) + s;
    return _mapDigits(s, sys);
  }

  String _dateField(
    _Calendar cal,
    PluxCalendarDate d,
    _Token t,
    String sys,
    PluxDateOptions opts,
  ) {
    final json = cal.json;
    String bad() => throw L10nException(
      L10nError.invalidOption,
      'date $d has no value for ${t.letter * t.n}',
    );
    switch (t.letter) {
      case 'G':
        return _name(_map(json, 'eras'), _widthOf(t.n), d.era) ?? bad();
      case 'y' || 'Y':
        return t.n == 2 ? _num(d.year % 100, 2, sys) : _num(d.year, t.n, sys);
      case 'M' || 'L':
        if (t.n <= 2) return _num(d.month, t.n, sys);
        final ctx = t.letter == 'L' ? 'standAlone' : 'format';
        return _name(
              _map(_map(json, 'months'), ctx),
              _widthOf(t.n),
              d.month - 1,
            ) ??
            bad();
      case 'd':
        return _num(d.day, t.n, sys);
      case 'E' || 'c':
        if (t.letter == 'c' && t.n < 3) {
          throw const L10nException(
            L10nError.unsupported,
            'numeric local weekday',
          );
        }
        final ctx = t.letter == 'c' ? 'standAlone' : 'format';
        final w = t.n == 6 ? 'short' : _widthOf(t.n);
        return _name(_map(_map(json, 'days'), ctx), w, d.weekday) ?? bad();
      case 'a' || 'B':
        final key = d.hour >= 12 ? 'pm' : 'am';
        final periods = _map(_map(json, 'dayPeriods'), 'format');
        final s = _string(_map(periods, _widthOf(t.n)), key);
        if (s.isNotEmpty) return s;
        final abbr = _string(_map(periods, 'abbreviated'), key);
        return abbr.isEmpty ? bad() : abbr;
      case 'h':
        final h = d.hour % 12;
        return _num(h == 0 ? 12 : h, t.n, sys);
      case 'H':
        return _num(d.hour, t.n, sys);
      case 'K':
        return _num(d.hour % 12, t.n, sys);
      case 'k':
        return _num(d.hour == 0 ? 24 : d.hour, t.n, sys);
      case 'm':
        return _num(d.minute, t.n, sys);
      case 's':
        return _num(d.second, t.n, sys);
      case 'z' || 'v' || 'V' || 'O' || 'Z' || 'x' || 'X':
        return opts.zoneName;
    }
    throw L10nException(L10nError.unsupported, 'pattern letter "${t.letter}"');
  }
}
