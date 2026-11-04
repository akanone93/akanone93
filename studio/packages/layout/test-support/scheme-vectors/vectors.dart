// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Writes src/material-color/scheme-vectors.json: every colour role of SchemeTonalSpot for a set of seeds, light and dark,
// from material_color_utilities 0.13.0 (the version the pinned Flutter uses), which the TypeScript port is tested
// against. Run with the pinned Dart in a package that depends on material_color_utilities: 0.13.0:
//   dart run vectors.dart > ../../src/material-color/scheme-vectors.json

import 'dart:convert';

import 'package:material_color_utilities/material_color_utilities.dart';

void main() {
  const seeds = [
    0xFF6750A4,
    0xFF0B57D0,
    0xFF1E8E3E,
    0xFFD93025,
    0xFFF9AB00,
    0xFF000000,
    0xFFFFFFFF,
    0xFF7D5260,
    0xFF00BCD4,
    0xFF9C27B0,
    0xFF3F51B5,
    0xFF808000,
  ];
  final roles = <String, DynamicColor>{
    'primary': MaterialDynamicColors.primary,
    'onPrimary': MaterialDynamicColors.onPrimary,
    'primaryContainer': MaterialDynamicColors.primaryContainer,
    'onPrimaryContainer': MaterialDynamicColors.onPrimaryContainer,
    'primaryFixed': MaterialDynamicColors.primaryFixed,
    'primaryFixedDim': MaterialDynamicColors.primaryFixedDim,
    'onPrimaryFixed': MaterialDynamicColors.onPrimaryFixed,
    'onPrimaryFixedVariant': MaterialDynamicColors.onPrimaryFixedVariant,
    'secondary': MaterialDynamicColors.secondary,
    'onSecondary': MaterialDynamicColors.onSecondary,
    'secondaryContainer': MaterialDynamicColors.secondaryContainer,
    'onSecondaryContainer': MaterialDynamicColors.onSecondaryContainer,
    'secondaryFixed': MaterialDynamicColors.secondaryFixed,
    'secondaryFixedDim': MaterialDynamicColors.secondaryFixedDim,
    'onSecondaryFixed': MaterialDynamicColors.onSecondaryFixed,
    'onSecondaryFixedVariant': MaterialDynamicColors.onSecondaryFixedVariant,
    'tertiary': MaterialDynamicColors.tertiary,
    'onTertiary': MaterialDynamicColors.onTertiary,
    'tertiaryContainer': MaterialDynamicColors.tertiaryContainer,
    'onTertiaryContainer': MaterialDynamicColors.onTertiaryContainer,
    'tertiaryFixed': MaterialDynamicColors.tertiaryFixed,
    'tertiaryFixedDim': MaterialDynamicColors.tertiaryFixedDim,
    'onTertiaryFixed': MaterialDynamicColors.onTertiaryFixed,
    'onTertiaryFixedVariant': MaterialDynamicColors.onTertiaryFixedVariant,
    'error': MaterialDynamicColors.error,
    'onError': MaterialDynamicColors.onError,
    'errorContainer': MaterialDynamicColors.errorContainer,
    'onErrorContainer': MaterialDynamicColors.onErrorContainer,
    'surface': MaterialDynamicColors.surface,
    'onSurface': MaterialDynamicColors.onSurface,
    'surfaceDim': MaterialDynamicColors.surfaceDim,
    'surfaceBright': MaterialDynamicColors.surfaceBright,
    'surfaceContainerLowest': MaterialDynamicColors.surfaceContainerLowest,
    'surfaceContainerLow': MaterialDynamicColors.surfaceContainerLow,
    'surfaceContainer': MaterialDynamicColors.surfaceContainer,
    'surfaceContainerHigh': MaterialDynamicColors.surfaceContainerHigh,
    'surfaceContainerHighest': MaterialDynamicColors.surfaceContainerHighest,
    'onSurfaceVariant': MaterialDynamicColors.onSurfaceVariant,
    'outline': MaterialDynamicColors.outline,
    'outlineVariant': MaterialDynamicColors.outlineVariant,
    'shadow': MaterialDynamicColors.shadow,
    'scrim': MaterialDynamicColors.scrim,
    'inverseSurface': MaterialDynamicColors.inverseSurface,
    'onInverseSurface': MaterialDynamicColors.inverseOnSurface,
    'inversePrimary': MaterialDynamicColors.inversePrimary,
    'surfaceTint': MaterialDynamicColors.primary,
  };
  final out = <Map<String, Object>>[];
  for (final seed in seeds) {
    for (final dark in [false, true]) {
      final s = SchemeTonalSpot(
        sourceColorHct: Hct.fromInt(seed),
        isDark: dark,
        contrastLevel: 0.0,
      );
      out.add({
        'seed': '#${seed.toRadixString(16).padLeft(8, '0')}',
        'dark': dark,
        'roles': {
          for (final e in roles.entries)
            e.key: '#${e.value.getArgb(s).toRadixString(16).padLeft(8, '0')}',
        },
      });
    }
  }
  print(const JsonEncoder.withIndent('  ').convert(out));
}
