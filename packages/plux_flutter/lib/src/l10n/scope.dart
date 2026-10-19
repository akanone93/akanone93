// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// How locale data reaches widgets (I18N-011): [PluxLocaleScope] holds the
/// CLDR data of the active locale for the subtree below it. Widgets that
/// format money, numbers or territory names read it with
/// [PluxLocaleScope.dataOf] and fall back to a minimal English format when
/// no scope is present.
library;

import 'package:flutter/widgets.dart';
import 'package:plux_flutter/src/l10n/l10n.dart';

/// Provides the locale data of the active locale to its subtree.
final class PluxLocaleScope extends InheritedWidget {
  /// Creates the scope for the locale [tag] (such as `de-DE`) whose CLDR
  /// data is [data].
  const PluxLocaleScope({
    required this.data,
    required this.tag,
    required super.child,
    this.calendar,
    this.secondaryCalendar,
    super.key,
  });

  /// The CLDR data of the locale; null formats with the built-in fallback.
  final PluxLocaleData? data;

  /// The BCP 47 tag the locale was requested with; its region, when it has
  /// one, is the default region of a phone number.
  final String tag;

  /// The app's `localisation.calendar`, or null for the Gregorian one.
  final String? calendar;

  /// The app's `localisation.secondaryCalendar`, or null.
  final String? secondaryCalendar;

  /// The app's calendar in scope at [context], or null.
  static String? calendarOf(BuildContext context) =>
      context.dependOnInheritedWidgetOfExactType<PluxLocaleScope>()?.calendar;

  /// The app's secondary calendar in scope at [context], or null.
  static String? secondaryCalendarOf(BuildContext context) => context
      .dependOnInheritedWidgetOfExactType<PluxLocaleScope>()
      ?.secondaryCalendar;

  /// The locale data in scope at [context], or null when there is none.
  static PluxLocaleData? dataOf(BuildContext context) =>
      context.dependOnInheritedWidgetOfExactType<PluxLocaleScope>()?.data;

  /// The locale tag in scope at [context], or null when there is none.
  static String? tagOf(BuildContext context) =>
      context.dependOnInheritedWidgetOfExactType<PluxLocaleScope>()?.tag;

  @override
  bool updateShouldNotify(PluxLocaleScope oldWidget) =>
      oldWidget.data != data ||
      oldWidget.tag != tag ||
      oldWidget.calendar != calendar ||
      oldWidget.secondaryCalendar != secondaryCalendar;
}
