// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// What every Plux page is built under (ADR-0066 § Runtime, I18N-004,
/// I18N-005): the locale data, the runtime strings, the reading direction
/// and Plux's own Material, Cupertino and Widgets localizations. A new
/// [PluxLocaleState] rebuilds the subtree in place, so page state survives
/// a switch of locale.
library;

import 'package:flutter/cupertino.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:plux_flutter/src/l10n/flutter_localizations.dart';
import 'package:plux_flutter/src/l10n/locale_controller.dart';
import 'package:plux_flutter/src/l10n/runtime_strings.dart';
import 'package:plux_flutter/src/l10n/scope.dart';

/// Provides the locale of [state] to [child].
final class PluxLocaleHost extends StatelessWidget {
  /// Creates the host.
  const PluxLocaleHost({required this.state, required this.child, super.key});

  /// The effective locale.
  final PluxLocaleState state;

  /// The page.
  final Widget child;

  @override
  Widget build(BuildContext context) {
    PluxFlutterStrings strings(String group) => PluxFlutterStrings(
      map: state.flutter[group] ?? const {},
      data: state.data,
      nativeDigits: state.nativeDigits,
    );
    final delegates = <LocalizationsDelegate<Object?>>[
      _Delegate<MaterialLocalizations>(
        strings('material'),
        PluxMaterialLocalizations.new,
      ),
      _Delegate<CupertinoLocalizations>(
        strings('cupertino'),
        PluxCupertinoLocalizations.new,
      ),
      _Delegate<WidgetsLocalizations>(
        strings('widgets'),
        PluxWidgetsLocalizations.new,
      ),
    ];
    final direction = Directionality(
      textDirection: state.direction,
      child: child,
    );
    // Under a host's Localizations the host's other delegates stay in
    // force; Plux's three come first and win.
    final localized = Localizations.maybeLocaleOf(context) == null
        ? Localizations(
            locale: state.locale,
            delegates: delegates,
            child: direction,
          )
        : Localizations.override(
            context: context,
            locale: state.locale,
            delegates: delegates,
            child: direction,
          );
    return PluxLocaleScope(
      data: state.data,
      tag: state.tag,
      calendar: state.calendar,
      secondaryCalendar: state.secondaryCalendar,
      child: PluxRuntimeStringsScope(strings: state.strings, child: localized),
    );
  }
}

/// Hands out a localizations object made from [strings]; it reloads when
/// the strings change.
final class _Delegate<T> extends LocalizationsDelegate<T> {
  const _Delegate(this.strings, this.make);

  final PluxFlutterStrings strings;

  final T Function(PluxFlutterStrings strings) make;

  @override
  bool isSupported(Locale locale) => true;

  @override
  Future<T> load(Locale locale) => SynchronousFuture<T>(make(strings));

  @override
  bool shouldReload(covariant _Delegate<T> old) => old.strings != strings;
}
