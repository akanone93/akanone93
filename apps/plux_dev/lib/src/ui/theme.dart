// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The Plux Dev app's look: white and quiet, generous spacing, rounded
/// corners, cards raised only as far as grouping needs, and large touch
/// targets (at least 56 logical pixels).
library;

import 'package:flutter/material.dart';

/// The app's colours.
abstract final class DevColors {
  /// Text and icons.
  static const Color ink = Color(0xFF111322);

  /// Secondary text.
  static const Color muted = Color(0xFF6B7085);

  /// Cards and sheets.
  static const Color surface = Color(0xFFFFFFFF);

  /// The page behind the cards.
  static const Color canvas = Color(0xFFF6F7FB);

  /// Hairlines.
  static const Color line = Color(0xFFE8E9F0);

  /// Actions and focus.
  static const Color primary = Color(0xFF5B3DF5);

  /// Live.
  static const Color live = Color(0xFF12B76A);

  /// Connecting again.
  static const Color warn = Color(0xFFF59E0B);

  /// Failure.
  static const Color danger = Color(0xFFE5484D);
}

/// The app's spacing and radii.
abstract final class DevSizes {
  /// The page's padding.
  static const double page = 24;

  /// A card's corner radius.
  static const double card = 24;

  /// A button's corner radius.
  static const double button = 16;

  /// The smallest touch target.
  static const double touch = 56;
}

/// The soft shadow of a raised card.
const List<BoxShadow> devCardShadow = [
  BoxShadow(color: Color(0x0A111322), blurRadius: 2, offset: Offset(0, 1)),
  BoxShadow(color: Color(0x0F111322), blurRadius: 24, offset: Offset(0, 8)),
];

/// The app's theme.
ThemeData devTheme() {
  final scheme = ColorScheme.fromSeed(
    seedColor: DevColors.primary,
    primary: DevColors.primary,
    surface: DevColors.surface,
    error: DevColors.danger,
  );
  const buttonShape = RoundedRectangleBorder(
    borderRadius: BorderRadius.all(Radius.circular(DevSizes.button)),
  );
  const buttonText = TextStyle(fontSize: 16, fontWeight: FontWeight.w600);
  final base = ThemeData(useMaterial3: true, colorScheme: scheme);
  return base.copyWith(
    scaffoldBackgroundColor: DevColors.canvas,
    textTheme: base.textTheme
        .apply(bodyColor: DevColors.ink, displayColor: DevColors.ink)
        .copyWith(
          headlineMedium: const TextStyle(
            fontSize: 30,
            fontWeight: FontWeight.w700,
            letterSpacing: -0.6,
            color: DevColors.ink,
          ),
          titleLarge: const TextStyle(
            fontSize: 20,
            fontWeight: FontWeight.w600,
            letterSpacing: -0.2,
            color: DevColors.ink,
          ),
          bodyLarge: const TextStyle(
            fontSize: 16,
            height: 1.45,
            color: DevColors.ink,
          ),
          bodyMedium: const TextStyle(
            fontSize: 14,
            height: 1.45,
            color: DevColors.muted,
          ),
        ),
    appBarTheme: const AppBarTheme(
      backgroundColor: Colors.transparent,
      surfaceTintColor: Colors.transparent,
      elevation: 0,
      foregroundColor: DevColors.ink,
    ),
    filledButtonTheme: FilledButtonThemeData(
      style: FilledButton.styleFrom(
        minimumSize: const Size.fromHeight(DevSizes.touch),
        shape: buttonShape,
        textStyle: buttonText,
      ),
    ),
    outlinedButtonTheme: OutlinedButtonThemeData(
      style: OutlinedButton.styleFrom(
        minimumSize: const Size.fromHeight(DevSizes.touch),
        shape: buttonShape,
        side: const BorderSide(color: DevColors.line),
        foregroundColor: DevColors.ink,
        textStyle: buttonText,
      ),
    ),
    textButtonTheme: TextButtonThemeData(
      style: TextButton.styleFrom(
        minimumSize: const Size(DevSizes.touch, 48),
        shape: buttonShape,
        textStyle: buttonText,
      ),
    ),
    inputDecorationTheme: const InputDecorationTheme(
      filled: true,
      fillColor: DevColors.surface,
      border: OutlineInputBorder(
        borderRadius: BorderRadius.all(Radius.circular(DevSizes.button)),
        borderSide: BorderSide(color: DevColors.line),
      ),
      enabledBorder: OutlineInputBorder(
        borderRadius: BorderRadius.all(Radius.circular(DevSizes.button)),
        borderSide: BorderSide(color: DevColors.line),
      ),
    ),
    bottomSheetTheme: const BottomSheetThemeData(
      backgroundColor: DevColors.surface,
      surfaceTintColor: Colors.transparent,
      showDragHandle: true,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(28)),
      ),
    ),
    snackBarTheme: const SnackBarThemeData(
      behavior: SnackBarBehavior.floating,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.all(Radius.circular(DevSizes.button)),
      ),
    ),
    dialogTheme: const DialogThemeData(
      backgroundColor: DevColors.surface,
      surfaceTintColor: Colors.transparent,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.all(Radius.circular(DevSizes.card)),
      ),
    ),
  );
}

/// A white card with rounded corners and, when [raised], a soft shadow.
final class DevCard extends StatelessWidget {
  /// Creates the card.
  const DevCard({
    super.key,
    required this.child,
    this.raised = true,
    this.padding = const EdgeInsets.all(20),
    this.onTap,
  });

  /// The content.
  final Widget child;

  /// Whether the card casts its soft shadow.
  final bool raised;

  /// The space inside.
  final EdgeInsets padding;

  /// Runs on a tap; the whole card is the target.
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) => DecoratedBox(
    decoration: BoxDecoration(
      color: DevColors.surface,
      borderRadius: BorderRadius.circular(DevSizes.card),
      border: Border.all(color: DevColors.line),
      boxShadow: raised ? devCardShadow : null,
    ),
    child: Material(
      type: MaterialType.transparency,
      child: InkWell(
        borderRadius: BorderRadius.circular(DevSizes.card),
        onTap: onTap,
        child: Padding(padding: padding, child: child),
      ),
    ),
  );
}
