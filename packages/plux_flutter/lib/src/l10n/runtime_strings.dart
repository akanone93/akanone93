// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The runtime's own user-facing strings (ADR-0066 § Runtime, ACT-020,
/// I18N-004): the fallback panel, the not-found page and the labels the
/// Layer 2 components show when a document gives none. English is built in;
/// other locales come from the `plux.`-prefixed keys of the locale's
/// Flutter-style string map, so the compiler can include them later.
library;

import 'package:flutter/widgets.dart';

/// The built-in English strings by key. Every key is in the `plux.`
/// namespace so it can sit beside Flutter's own ARB keys in one map.
const Map<String, String> englishRuntimeStrings = {
  'plux.fallback.unavailable': 'Content unavailable',
  'plux.action.network': 'Check your connection and try again.',
  'plux.action.timeout': 'This is taking too long. Please try again.',
  'plux.action.permission': 'Permission is needed to do this.',
  'plux.action.error': 'Something went wrong. Please try again.',
  'plux.router.notFound': 'Page not found',
  'plux.wizard.back': 'Back',
  'plux.wizard.next': 'Next',
  'plux.wizard.submit': 'Submit',
  'plux.pager.done': 'Done',
  'plux.amount.outOfRange': 'Amount out of range',
  'plux.map.zoomIn': 'Zoom in',
  'plux.map.zoomOut': 'Zoom out',
  'plux.masked.reveal': 'Reveal',
  'plux.picker.search': 'Search',
  'plux.phone.invalid': 'Invalid phone number',
  'plux.iban.invalid': 'Invalid IBAN',
  'plux.cart.subtotal': 'Subtotal',
  'plux.cart.tax': 'Tax',
  'plux.cart.total': 'Total',
  'plux.cart.checkout': 'Checkout',
  'plux.product.addToCart': 'Add to cart',
  'plux.stepper.decrease': 'Decrease',
  'plux.stepper.increase': 'Increase',
  'plux.uploader.choose': 'Choose file',
  'plux.uploader.upload': 'Upload',
  'plux.rating.of': 'of',
  'plux.price.was': 'was',
  'plux.timeline.done': 'done',
  'plux.timeline.current': 'current step',
  'plux.timeline.pending': 'pending',
  'plux.timeline.failed': 'failed',
};

/// The runtime strings of one locale: its own table over the English one.
@immutable
final class RuntimeStrings {
  /// Creates strings from a locale's [table]; keys it lacks read English.
  const RuntimeStrings([this.table = const {}]);

  /// English only.
  static const RuntimeStrings english = RuntimeStrings();

  /// The locale's `plux.` entries.
  final Map<String, String> table;

  /// The string of [key]; its English text when the locale has none, and
  /// the key's last segment never, so a key is not shown to a user: an
  /// unknown key reads as the empty string.
  String operator [](String key) =>
      table[key] ?? englishRuntimeStrings[key] ?? '';

  /// The strings in scope at [context]; English when there is no scope.
  static RuntimeStrings of(BuildContext context) =>
      context
          .dependOnInheritedWidgetOfExactType<PluxRuntimeStringsScope>()
          ?.strings ??
      english;
}

/// Provides [RuntimeStrings] to its subtree.
final class PluxRuntimeStringsScope extends InheritedWidget {
  /// Creates the scope.
  const PluxRuntimeStringsScope({
    required this.strings,
    required super.child,
    super.key,
  });

  /// The strings of the active locale.
  final RuntimeStrings strings;

  @override
  bool updateShouldNotify(PluxRuntimeStringsScope oldWidget) =>
      oldWidget.strings.table != strings.table;
}
