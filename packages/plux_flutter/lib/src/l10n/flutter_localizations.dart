// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Flutter's own widget strings from data (ADR-0066 § Flutter's widget
/// strings, I18N-004): Plux's `MaterialLocalizations`,
/// `CupertinoLocalizations` and `WidgetsLocalizations` read the `flutter`
/// map the bundle carries for the active locale (Flutter's ARB keys to
/// strings) and format dates and numbers with the L1 engine. Digits are
/// Latin unless the app opts a locale into its native numbering system.
/// A key the map lacks reads Flutter's English text; the controller has
/// already resolved the map along the CLDR parent chain.
library;

import 'package:flutter/cupertino.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:plux_flutter/src/l10n/l10n.dart';

/// The locale's strings and formatter the three localizations share.
@immutable
final class PluxFlutterStrings {
  /// Creates the strings of a locale: [map] holds Flutter's ARB keys,
  /// [data] the CLDR data to format with (null: Flutter's English
  /// formatting), and [nativeDigits] says the app uses the locale's own
  /// numbering system.
  const PluxFlutterStrings({
    this.map = const {},
    this.data,
    this.nativeDigits = false,
  });

  /// Flutter's ARB keys to the locale's strings.
  final Map<String, String> map;

  /// The CLDR data of the locale.
  final PluxLocaleData? data;

  /// Whether digits are the locale's native ones
  /// (`localisation.numberingSystem`).
  final bool nativeDigits;

  /// The string of [key], or [fallback].
  String string(String key, String fallback) => map[key] ?? fallback;

  /// The string of [key] with each `$name` or `{name}` of [args] replaced,
  /// or [fallback] when the map has no such key.
  String format(String key, String fallback, Map<String, String> args) {
    final pattern = map[key];
    if (pattern == null) return fallback;
    var s = pattern;
    for (final e in args.entries) {
      s = s.replaceAll('\$${e.key}', e.value).replaceAll('{${e.key}}', e.value);
    }
    return s;
  }

  /// [d] formatted with the CLDR [skeleton], or null without data.
  String? date(DateTime d, String skeleton) {
    final data = this.data;
    if (data == null) return null;
    try {
      return data.formatDate(
        PluxCalendarDate.gregorian(
          d.year,
          d.month,
          d.day,
          d.hour,
          d.minute,
          d.second,
        ),
        'gregorian',
        PluxDateOptions(
          skeleton: skeleton,
          numberingSystem: nativeDigits ? 'default' : '',
        ),
      );
    } on L10nException {
      return null;
    }
  }

  /// [n] formatted with the locale's grouping and digits, or null.
  String? decimal(int n) {
    final data = this.data;
    if (data == null) return null;
    try {
      return data.formatNumber(
        PluxDecimal.parse('$n'),
        NumberOptions(numberingSystem: nativeDigits ? 'default' : ''),
      );
    } on L10nException {
      return null;
    }
  }

  @override
  bool operator ==(Object other) =>
      other is PluxFlutterStrings &&
      mapEquals(other.map, map) &&
      other.data == data &&
      other.nativeDigits == nativeDigits;

  @override
  int get hashCode => Object.hash(map.length, data, nativeDigits);
}

/// `MaterialLocalizations` over a locale's strings. It extends Flutter's
/// English defaults, so a key the map lacks stays English.
class PluxMaterialLocalizations extends DefaultMaterialLocalizations {
  /// Creates the localizations.
  const PluxMaterialLocalizations(this.strings);

  /// The locale's strings.
  final PluxFlutterStrings strings;

  String _s(String key, String fallback) => strings.string(key, fallback);

  @override
  String formatYear(DateTime date) =>
      strings.date(date, 'y') ?? super.formatYear(date);

  @override
  String formatCompactDate(DateTime date) =>
      strings.date(date, 'yMd') ?? super.formatCompactDate(date);

  @override
  String formatShortDate(DateTime date) =>
      strings.date(date, 'yMMMd') ?? super.formatShortDate(date);

  @override
  String formatMediumDate(DateTime date) =>
      strings.date(date, 'MMMEd') ?? super.formatMediumDate(date);

  @override
  String formatFullDate(DateTime date) =>
      strings.date(date, 'yMMMMEEEEd') ?? super.formatFullDate(date);

  @override
  String formatMonthYear(DateTime date) =>
      strings.date(date, 'yMMMM') ?? super.formatMonthYear(date);

  @override
  String formatShortMonthDay(DateTime date) =>
      strings.date(date, 'MMMd') ?? super.formatShortMonthDay(date);

  @override
  String formatDecimal(int number) =>
      strings.decimal(number) ?? super.formatDecimal(number);

  @override
  List<String> get narrowWeekdays {
    if (strings.data == null) return super.narrowWeekdays;
    // 2024-01-07 was a Sunday.
    final days = [
      for (var i = 0; i < 7; i++)
        strings.date(DateTime(2024, 1, 7 + i), 'EEEEE') ?? '',
    ];
    return days.contains('') ? super.narrowWeekdays : days;
  }

  @override
  int get firstDayOfWeekIndex =>
      int.tryParse(strings.map['firstDayOfWeekIndex'] ?? '') ??
      super.firstDayOfWeekIndex;

  @override
  TimeOfDayFormat timeOfDayFormat({bool alwaysUse24HourFormat = false}) {
    final f = strings.map['timeOfDayFormat'];
    if (f == null) {
      return super.timeOfDayFormat(
        alwaysUse24HourFormat: alwaysUse24HourFormat,
      );
    }
    if (alwaysUse24HourFormat) return _twentyFour(f);
    return switch (f) {
      'h:mm a' => TimeOfDayFormat.h_colon_mm_space_a,
      'a h:mm' => TimeOfDayFormat.a_space_h_colon_mm,
      _ => _twentyFour(f),
    };
  }

  static TimeOfDayFormat _twentyFour(String f) => switch (f) {
    'H:mm' || 'h:mm a' || 'a h:mm' || 'H.mm' => TimeOfDayFormat.H_colon_mm,
    'HH.mm' => TimeOfDayFormat.HH_dot_mm,
    "HH 'h' mm" => TimeOfDayFormat.frenchCanadian,
    _ => TimeOfDayFormat.HH_colon_mm,
  };

  @override
  String aboutListTileTitle(String applicationName) => strings.format(
    'aboutListTileTitle',
    super.aboutListTileTitle(applicationName),
    {'applicationName': applicationName},
  );

  @override
  String dateRangeStartDateSemanticLabel(String formattedDate) =>
      strings.format(
        'dateRangeStartDateSemanticLabel',
        super.dateRangeStartDateSemanticLabel(formattedDate),
        {'fullDate': formattedDate},
      );

  @override
  String dateRangeEndDateSemanticLabel(String formattedDate) => strings.format(
    'dateRangeEndDateSemanticLabel',
    super.dateRangeEndDateSemanticLabel(formattedDate),
    {'fullDate': formattedDate},
  );

  @override
  String scrimOnTapHint(String modalRouteContentName) => strings.format(
    'scrimOnTapHint',
    super.scrimOnTapHint(modalRouteContentName),
    {'modalRouteContentName': modalRouteContentName},
  );

  @override
  String tabLabel({required int tabIndex, required int tabCount}) =>
      strings.format(
        'tabLabel',
        super.tabLabel(tabIndex: tabIndex, tabCount: tabCount),
        {'tabIndex': '$tabIndex', 'tabCount': '$tabCount'},
      );

  @override
  String get openAppDrawerTooltip =>
      _s('openAppDrawerTooltip', super.openAppDrawerTooltip);

  @override
  String get backButtonTooltip =>
      _s('backButtonTooltip', super.backButtonTooltip);

  @override
  String get clearButtonTooltip =>
      _s('clearButtonTooltip', super.clearButtonTooltip);

  @override
  String get closeButtonTooltip =>
      _s('closeButtonTooltip', super.closeButtonTooltip);

  @override
  String get deleteButtonTooltip =>
      _s('deleteButtonTooltip', super.deleteButtonTooltip);

  @override
  String get moreButtonTooltip =>
      _s('moreButtonTooltip', super.moreButtonTooltip);

  @override
  String get nextMonthTooltip => _s('nextMonthTooltip', super.nextMonthTooltip);

  @override
  String get previousMonthTooltip =>
      _s('previousMonthTooltip', super.previousMonthTooltip);

  @override
  String get firstPageTooltip => _s('firstPageTooltip', super.firstPageTooltip);

  @override
  String get lastPageTooltip => _s('lastPageTooltip', super.lastPageTooltip);

  @override
  String get nextPageTooltip => _s('nextPageTooltip', super.nextPageTooltip);

  @override
  String get previousPageTooltip =>
      _s('previousPageTooltip', super.previousPageTooltip);

  @override
  String get showMenuTooltip => _s('showMenuTooltip', super.showMenuTooltip);

  @override
  String get licensesPageTitle =>
      _s('licensesPageTitle', super.licensesPageTitle);

  @override
  String get rowsPerPageTitle => _s('rowsPerPageTitle', super.rowsPerPageTitle);

  @override
  String get cancelButtonLabel =>
      _s('cancelButtonLabel', super.cancelButtonLabel);

  @override
  String get closeButtonLabel => _s('closeButtonLabel', super.closeButtonLabel);

  @override
  String get continueButtonLabel =>
      _s('continueButtonLabel', super.continueButtonLabel);

  @override
  String get copyButtonLabel => _s('copyButtonLabel', super.copyButtonLabel);

  @override
  String get cutButtonLabel => _s('cutButtonLabel', super.cutButtonLabel);

  @override
  String get scanTextButtonLabel =>
      _s('scanTextButtonLabel', super.scanTextButtonLabel);

  @override
  String get okButtonLabel => _s('okButtonLabel', super.okButtonLabel);

  @override
  String get pasteButtonLabel => _s('pasteButtonLabel', super.pasteButtonLabel);

  @override
  String get selectAllButtonLabel =>
      _s('selectAllButtonLabel', super.selectAllButtonLabel);

  @override
  String get lookUpButtonLabel =>
      _s('lookUpButtonLabel', super.lookUpButtonLabel);

  @override
  String get searchWebButtonLabel =>
      _s('searchWebButtonLabel', super.searchWebButtonLabel);

  @override
  String get shareButtonLabel => _s('shareButtonLabel', super.shareButtonLabel);

  @override
  String get viewLicensesButtonLabel =>
      _s('viewLicensesButtonLabel', super.viewLicensesButtonLabel);

  @override
  String get anteMeridiemAbbreviation =>
      _s('anteMeridiemAbbreviation', super.anteMeridiemAbbreviation);

  @override
  String get postMeridiemAbbreviation =>
      _s('postMeridiemAbbreviation', super.postMeridiemAbbreviation);

  @override
  String get timePickerHourModeAnnouncement => _s(
    'timePickerHourModeAnnouncement',
    super.timePickerHourModeAnnouncement,
  );

  @override
  String get timePickerMinuteModeAnnouncement => _s(
    'timePickerMinuteModeAnnouncement',
    super.timePickerMinuteModeAnnouncement,
  );

  @override
  String get modalBarrierDismissLabel =>
      _s('modalBarrierDismissLabel', super.modalBarrierDismissLabel);

  @override
  String get menuDismissLabel => _s('menuDismissLabel', super.menuDismissLabel);

  @override
  String get drawerLabel => _s('drawerLabel', super.drawerLabel);

  @override
  String get popupMenuLabel => _s('popupMenuLabel', super.popupMenuLabel);

  @override
  String get menuBarMenuLabel => _s('menuBarMenuLabel', super.menuBarMenuLabel);

  @override
  String get dialogLabel => _s('dialogLabel', super.dialogLabel);

  @override
  String get alertDialogLabel => _s('alertDialogLabel', super.alertDialogLabel);

  @override
  String get searchFieldLabel => _s('searchFieldLabel', super.searchFieldLabel);

  @override
  String get currentDateLabel => _s('currentDateLabel', super.currentDateLabel);

  @override
  String get selectedDateLabel =>
      _s('selectedDateLabel', super.selectedDateLabel);

  @override
  String get scrimLabel => _s('scrimLabel', super.scrimLabel);

  @override
  String get bottomSheetLabel => _s('bottomSheetLabel', super.bottomSheetLabel);

  @override
  String get dateSeparator => _s('dateSeparator', super.dateSeparator);

  @override
  String get dateHelpText => _s('dateHelpText', super.dateHelpText);

  @override
  String get selectYearSemanticsLabel =>
      _s('selectYearSemanticsLabel', super.selectYearSemanticsLabel);

  @override
  String get unspecifiedDate => _s('unspecifiedDate', super.unspecifiedDate);

  @override
  String get unspecifiedDateRange =>
      _s('unspecifiedDateRange', super.unspecifiedDateRange);

  @override
  String get dateInputLabel => _s('dateInputLabel', super.dateInputLabel);

  @override
  String get dateRangeStartLabel =>
      _s('dateRangeStartLabel', super.dateRangeStartLabel);

  @override
  String get dateRangeEndLabel =>
      _s('dateRangeEndLabel', super.dateRangeEndLabel);

  @override
  String get invalidDateFormatLabel =>
      _s('invalidDateFormatLabel', super.invalidDateFormatLabel);

  @override
  String get invalidDateRangeLabel =>
      _s('invalidDateRangeLabel', super.invalidDateRangeLabel);

  @override
  String get dateOutOfRangeLabel =>
      _s('dateOutOfRangeLabel', super.dateOutOfRangeLabel);

  @override
  String get saveButtonLabel => _s('saveButtonLabel', super.saveButtonLabel);

  @override
  String get datePickerHelpText =>
      _s('datePickerHelpText', super.datePickerHelpText);

  @override
  String get dateRangePickerHelpText =>
      _s('dateRangePickerHelpText', super.dateRangePickerHelpText);

  @override
  String get calendarModeButtonLabel =>
      _s('calendarModeButtonLabel', super.calendarModeButtonLabel);

  @override
  String get inputDateModeButtonLabel =>
      _s('inputDateModeButtonLabel', super.inputDateModeButtonLabel);

  @override
  String get timePickerDialHelpText =>
      _s('timePickerDialHelpText', super.timePickerDialHelpText);

  @override
  String get timePickerInputHelpText =>
      _s('timePickerInputHelpText', super.timePickerInputHelpText);

  @override
  String get timePickerHourLabel =>
      _s('timePickerHourLabel', super.timePickerHourLabel);

  @override
  String get timePickerMinuteLabel =>
      _s('timePickerMinuteLabel', super.timePickerMinuteLabel);

  @override
  String get invalidTimeLabel => _s('invalidTimeLabel', super.invalidTimeLabel);

  @override
  String get dialModeButtonLabel =>
      _s('dialModeButtonLabel', super.dialModeButtonLabel);

  @override
  String get inputTimeModeButtonLabel =>
      _s('inputTimeModeButtonLabel', super.inputTimeModeButtonLabel);

  @override
  String get signedInLabel => _s('signedInLabel', super.signedInLabel);

  @override
  String get hideAccountsLabel =>
      _s('hideAccountsLabel', super.hideAccountsLabel);

  @override
  String get showAccountsLabel =>
      _s('showAccountsLabel', super.showAccountsLabel);

  @override
  String get reorderItemToStart =>
      _s('reorderItemToStart', super.reorderItemToStart);

  @override
  String get reorderItemToEnd => _s('reorderItemToEnd', super.reorderItemToEnd);

  @override
  String get reorderItemUp => _s('reorderItemUp', super.reorderItemUp);

  @override
  String get reorderItemDown => _s('reorderItemDown', super.reorderItemDown);

  @override
  String get reorderItemLeft => _s('reorderItemLeft', super.reorderItemLeft);

  @override
  String get reorderItemRight => _s('reorderItemRight', super.reorderItemRight);

  @override
  String get refreshIndicatorSemanticLabel =>
      _s('refreshIndicatorSemanticLabel', super.refreshIndicatorSemanticLabel);
}

/// `CupertinoLocalizations` over a locale's strings.
class PluxCupertinoLocalizations extends DefaultCupertinoLocalizations {
  /// Creates the localizations.
  const PluxCupertinoLocalizations(this.strings);

  /// The locale's strings.
  final PluxFlutterStrings strings;

  String _s(String key, String fallback) => strings.string(key, fallback);

  @override
  String datePickerMonth(int monthIndex) =>
      strings.date(DateTime(2024, monthIndex), 'LLLL') ??
      super.datePickerMonth(monthIndex);

  @override
  String datePickerStandaloneMonth(int monthIndex) =>
      strings.date(DateTime(2024, monthIndex), 'LLLL') ??
      super.datePickerStandaloneMonth(monthIndex);

  @override
  String datePickerMediumDate(DateTime date) =>
      strings.date(date, 'MMMEd') ?? super.datePickerMediumDate(date);

  @override
  String datePickerYear(int yearIndex) =>
      strings.decimal(yearIndex) ?? super.datePickerYear(yearIndex);

  @override
  String get anteMeridiemAbbreviation =>
      _s('anteMeridiemAbbreviation', super.anteMeridiemAbbreviation);

  @override
  String get postMeridiemAbbreviation =>
      _s('postMeridiemAbbreviation', super.postMeridiemAbbreviation);

  @override
  String get todayLabel => _s('todayLabel', super.todayLabel);

  @override
  String get alertDialogLabel => _s('alertDialogLabel', super.alertDialogLabel);

  @override
  String get cutButtonLabel => _s('cutButtonLabel', super.cutButtonLabel);

  @override
  String get copyButtonLabel => _s('copyButtonLabel', super.copyButtonLabel);

  @override
  String get pasteButtonLabel => _s('pasteButtonLabel', super.pasteButtonLabel);

  @override
  String get clearButtonLabel => _s('clearButtonLabel', super.clearButtonLabel);

  @override
  String get noSpellCheckReplacementsLabel =>
      _s('noSpellCheckReplacementsLabel', super.noSpellCheckReplacementsLabel);

  @override
  String get selectAllButtonLabel =>
      _s('selectAllButtonLabel', super.selectAllButtonLabel);

  @override
  String get lookUpButtonLabel =>
      _s('lookUpButtonLabel', super.lookUpButtonLabel);

  @override
  String get searchWebButtonLabel =>
      _s('searchWebButtonLabel', super.searchWebButtonLabel);

  @override
  String get shareButtonLabel => _s('shareButtonLabel', super.shareButtonLabel);

  @override
  String get searchTextFieldPlaceholderLabel => _s(
    'searchTextFieldPlaceholderLabel',
    super.searchTextFieldPlaceholderLabel,
  );

  @override
  String get modalBarrierDismissLabel =>
      _s('modalBarrierDismissLabel', super.modalBarrierDismissLabel);

  @override
  String get menuDismissLabel => _s('menuDismissLabel', super.menuDismissLabel);

  @override
  String get cancelButtonLabel =>
      _s('cancelButtonLabel', super.cancelButtonLabel);

  @override
  String get backButtonLabel => _s('backButtonLabel', super.backButtonLabel);
}

/// `WidgetsLocalizations` over a locale's strings; the text direction is the
/// locale's.
class PluxWidgetsLocalizations extends DefaultWidgetsLocalizations {
  /// Creates the localizations.
  const PluxWidgetsLocalizations(this.strings);

  /// The locale's strings.
  final PluxFlutterStrings strings;

  String _s(String key, String fallback) => strings.string(key, fallback);

  @override
  TextDirection get textDirection =>
      (strings.data?.isRtl ?? false) ? TextDirection.rtl : TextDirection.ltr;

  @override
  String get reorderItemToStart =>
      _s('reorderItemToStart', super.reorderItemToStart);

  @override
  String get reorderItemToEnd => _s('reorderItemToEnd', super.reorderItemToEnd);

  @override
  String get reorderItemUp => _s('reorderItemUp', super.reorderItemUp);

  @override
  String get reorderItemDown => _s('reorderItemDown', super.reorderItemDown);

  @override
  String get reorderItemLeft => _s('reorderItemLeft', super.reorderItemLeft);

  @override
  String get reorderItemRight => _s('reorderItemRight', super.reorderItemRight);

  @override
  String get searchResultsFound =>
      _s('searchResultsFound', super.searchResultsFound);

  @override
  String get noResultsFound => _s('noResultsFound', super.noResultsFound);

  @override
  String get copyButtonLabel => _s('copyButtonLabel', super.copyButtonLabel);

  @override
  String get cutButtonLabel => _s('cutButtonLabel', super.cutButtonLabel);

  @override
  String get pasteButtonLabel => _s('pasteButtonLabel', super.pasteButtonLabel);

  @override
  String get selectAllButtonLabel =>
      _s('selectAllButtonLabel', super.selectAllButtonLabel);

  @override
  String get lookUpButtonLabel =>
      _s('lookUpButtonLabel', super.lookUpButtonLabel);

  @override
  String get searchWebButtonLabel =>
      _s('searchWebButtonLabel', super.searchWebButtonLabel);

  @override
  String get shareButtonLabel => _s('shareButtonLabel', super.shareButtonLabel);

  @override
  String get radioButtonUnselectedLabel =>
      _s('radioButtonUnselectedLabel', super.radioButtonUnselectedLabel);
}
