// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Plux's localisation engine (ADR-0066): locale resolution, plural rules, and
/// number, percent, compact, money and date formatting over the CLDR locale
/// data of `schema/l10n/cldr`. It is the Dart twin of Go's
/// `backend/internal/l10n`.
library;

export 'package:plux_flutter/src/l10n/decimal.dart' show PluxDecimal;
export 'package:plux_flutter/src/l10n/errors.dart';
export 'package:plux_flutter/src/l10n/locale_data.dart';
export 'package:plux_flutter/src/l10n/plural.dart'
    show PluralCategory, PluralOperands, PluralRule, PluralRules;
export 'package:plux_flutter/src/l10n/resolve.dart';
