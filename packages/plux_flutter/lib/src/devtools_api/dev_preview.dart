// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The preview a dev session puts on the device (`DEV-011`): the locale,
/// theme mode and text scale a screen was opened with. Only builds with
/// `kPluxDevtools` read it.
library;

import 'package:flutter/material.dart';

/// The overrides of the preview; a null field leaves the device's setting.
@immutable
final class DevPreview {
  /// Creates the overrides.
  const DevPreview({this.locale, this.themeMode, this.textScale});

  /// The locale, pseudo-locales included (`I18N-010`).
  final Locale? locale;

  /// The theme mode.
  final ThemeMode? themeMode;

  /// The text scale factor.
  final double? textScale;

  /// This preview with the given fields replaced.
  DevPreview copyWith({
    Locale? locale,
    ThemeMode? themeMode,
    double? textScale,
  }) => DevPreview(
    locale: locale ?? this.locale,
    themeMode: themeMode ?? this.themeMode,
    textScale: textScale ?? this.textScale,
  );

  @override
  bool operator ==(Object other) =>
      other is DevPreview &&
      other.locale == locale &&
      other.themeMode == themeMode &&
      other.textScale == textScale;

  @override
  int get hashCode => Object.hash(locale, themeMode, textScale);
}

/// The preview overrides of one runtime, which its root widget listens to.
final class DevPreviewOverrides extends ValueNotifier<DevPreview> {
  /// Creates overrides that change nothing.
  DevPreviewOverrides() : super(const DevPreview());
}

/// Applies the text scale of [overrides] to [child] (`A11Y-005`).
final class DevPreviewScope extends StatelessWidget {
  /// Creates the scope.
  const DevPreviewScope({
    super.key,
    required this.overrides,
    required this.child,
  });

  /// The overrides to follow.
  final DevPreviewOverrides overrides;

  /// The subtree.
  final Widget child;

  @override
  Widget build(BuildContext context) => ValueListenableBuilder<DevPreview>(
    valueListenable: overrides,
    child: child,
    builder: (context, preview, child) {
      final scale = preview.textScale;
      if (scale == null) return child!;
      return MediaQuery(
        data: MediaQuery.of(context)
            .copyWith(textScaler: TextScaler.linear(scale)),
        child: child!,
      );
    },
  );
}
