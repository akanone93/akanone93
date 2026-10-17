// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Why a localisation call failed.
enum L10nError {
  /// A locale tag that is not well-formed BCP 47.
  invalidTag,

  /// A locale with no data and no fallback.
  noLocale,

  /// A string that is not a decimal number.
  invalidDecimal,

  /// Locale data that does not have the expected shape.
  invalidData,

  /// A request the engine does not implement: an unknown calendar, numbering
  /// system or style, or a pattern letter it lacks.
  unsupported,

  /// A currency that is not an ISO 4217 code.
  invalidCurrency,

  /// An option out of range.
  invalidOption,
}

/// The exception every localisation function throws, with a [code] to branch
/// on and a [message] for humans.
final class L10nException implements Exception {
  /// Creates the exception.
  const L10nException(this.code, this.message);

  /// What went wrong.
  final L10nError code;

  /// A description.
  final String message;

  @override
  String toString() => 'L10nException(${code.name}): $message';
}
