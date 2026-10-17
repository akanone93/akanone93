// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The accessibility and appearance actions (A11Y-006, THM-002):
/// `announce` has the screen reader speak a message without moving focus;
/// `setLocale` switches the locale of Plux pages like `Plux.setLocale`
/// (I18N-005);
/// `setThemeMode` changes the app's theme mode through the same
/// environment path as `Plux.setThemeMode`.
library;

import 'package:flutter/material.dart';
import 'package:flutter/semantics.dart';
import 'package:plux_flutter/src/actions/action_error.dart';
import 'package:plux_flutter/src/actions/handlers.dart';
import 'package:plux_flutter/src/core/plux.dart';
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/l10n/locale_controller.dart' show localeOfTag;

/// The handlers of the accessibility actions, by action name.
final Map<String, ActionHandler> a11yHandlers = {
  'announce': FunctionHandler(_announce),
  'setThemeMode': FunctionHandler(_setThemeMode),
  'setLocale': FunctionHandler(_setLocale),
};

Future<StepResult> _announce(StepContext c, Map<String, Object?> i) async {
  final message = i['message'];
  if (message is! String) {
    throw ActionError.validation('announce: message is not a string');
  }
  final assertiveness = switch (enumMember(
    'Assertiveness',
    i['assertiveness'],
  )) {
    'assertive' => Assertiveness.assertive,
    _ => Assertiveness.polite,
  };
  final context = c.device?.context?.call();
  if (context == null || !context.mounted) {
    throw const ActionError(
      ActionErrorKind.custom,
      PluxErrorCode.deviceUnavailable,
      'announce: there is no view to announce on',
    );
  }
  await SemanticsService.sendAnnouncement(
    View.of(context),
    message,
    Directionality.maybeOf(context) ?? TextDirection.ltr,
    assertiveness: assertiveness,
  );
  return const StepDone();
}

StepResult _setThemeMode(StepContext c, Map<String, Object?> i) {
  final mode = switch (enumMember('ThemeMode', i['mode'])) {
    'light' => ThemeMode.light,
    'dark' => ThemeMode.dark,
    'system' => ThemeMode.system,
    _ => throw ActionError.validation(
      'setThemeMode: mode is not system, light or dark',
    ),
  };
  Plux.setThemeMode(mode);
  return const StepDone();
}

StepResult _setLocale(StepContext c, Map<String, Object?> i) {
  final tag = i['locale'];
  if (tag != null && (tag is! String || tag.isEmpty)) {
    throw ActionError.validation('setLocale: locale is not a language tag');
  }
  Plux.setLocale(tag == null ? null : localeOfTag(tag as String));
  return const StepDone();
}
