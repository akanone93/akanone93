// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/material/theme_data.dart, typography.dart, button_style.dart
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/**
 * The Material 3 metrics layout depends on: the 2021 English-like text theme
 * (`Typography.englishLike2021`), `VisualDensity` and the tap-target sizes.
 * Colours and shapes are not ported; only what changes a box's size or place.
 */

import { BoxConstraints } from "../constraints.ts";
import { clampDouble } from "../geometry.ts";
import { TextLeadingDistribution } from "../../text/paragraph-engine.ts";
import type { TextStyle } from "../text-style.ts";

/** `kMinInteractiveDimension`. */
export const kMinInteractiveDimension = 48;
/** `kToolbarHeight`. */
export const kToolbarHeight = 56;
/** `kBottomNavigationBarHeight`. */
export const kBottomNavigationBarHeight = 56;
/** `kTextTabBarHeight`. */
export const kTextTabBarHeight = 48;

/** `MaterialTapTargetSize`. */
export enum MaterialTapTargetSize {
  padded = "padded",
  shrinkWrap = "shrinkWrap",
}

/** The roles of `TextTheme`. */
export type TextRole =
  | "displayLarge"
  | "displayMedium"
  | "displaySmall"
  | "headlineLarge"
  | "headlineMedium"
  | "headlineSmall"
  | "titleLarge"
  | "titleMedium"
  | "titleSmall"
  | "labelLarge"
  | "labelMedium"
  | "labelSmall"
  | "bodyLarge"
  | "bodyMedium"
  | "bodySmall";

type Metrics = readonly [fontSize: number, fontWeight: number, letterSpacing: number, height: number];

const englishLike2021: Readonly<Record<TextRole, Metrics>> = {
  displayLarge: [57, 400, -0.25, 1.12],
  displayMedium: [45, 400, 0, 1.16],
  displaySmall: [36, 400, 0, 1.22],
  headlineLarge: [32, 400, 0, 1.25],
  headlineMedium: [28, 400, 0, 1.29],
  headlineSmall: [24, 400, 0, 1.33],
  titleLarge: [22, 400, 0, 1.27],
  titleMedium: [16, 500, 0.15, 1.5],
  titleSmall: [14, 500, 0.1, 1.43],
  labelLarge: [14, 500, 0.1, 1.43],
  labelMedium: [12, 500, 0.5, 1.33],
  labelSmall: [11, 500, 0.5, 1.45],
  bodyLarge: [16, 400, 0.5, 1.5],
  bodyMedium: [14, 400, 0.25, 1.43],
  bodySmall: [12, 400, 0.4, 1.33],
};

/** The theme's text style for `role` in `fontFamily` (`ThemeData.textTheme`). */
export function m3TextStyle(role: TextRole, fontFamily?: string): TextStyle {
  const [fontSize, fontWeight, letterSpacing, height] = englishLike2021[role];
  return { fontFamily, fontSize, fontWeight, letterSpacing, height, leadingDistribution: TextLeadingDistribution.even };
}

/** `VisualDensity`: how compactly components lay out, in steps of 4 logical pixels. */
export class VisualDensity {
  static readonly standard = new VisualDensity(0, 0);
  static readonly comfortable = new VisualDensity(-1, -1);
  static readonly compact = new VisualDensity(-2, -2);

  constructor(
    readonly horizontal: number,
    readonly vertical: number,
  ) {}

  /** The density a scene names: `standard`, `comfortable` or `compact`. */
  static named(name: unknown): VisualDensity {
    switch (name) {
      case "comfortable":
        return VisualDensity.comfortable;
      case "compact":
        return VisualDensity.compact;
      default:
        return VisualDensity.standard;
    }
  }

  get baseSizeAdjustmentX(): number {
    return this.horizontal * 4;
  }

  get baseSizeAdjustmentY(): number {
    return this.vertical * 4;
  }

  /** `VisualDensity.effectiveConstraints`. */
  effectiveConstraints(c: BoxConstraints): BoxConstraints {
    return new BoxConstraints({
      minWidth: clampDouble(c.minWidth + this.baseSizeAdjustmentX, 0, c.maxWidth),
      maxWidth: c.maxWidth,
      minHeight: clampDouble(c.minHeight + this.baseSizeAdjustmentY, 0, c.maxHeight),
      maxHeight: c.maxHeight,
    });
  }
}

/** A directional inset as start, top, end, bottom. */
export type Steb = readonly [number, number, number, number];

/** `ButtonStyleButton.scaledPadding` for insets given as start/top/end/bottom. */
export function scaledPadding(g1: Steb, g2: Steb, g3: Steb, multiplier: number): Steb {
  const lerp = (a: Steb, b: Steb, t: number): Steb => [
    a[0] + (b[0] - a[0]) * t,
    a[1] + (b[1] - a[1]) * t,
    a[2] + (b[2] - a[2]) * t,
    a[3] + (b[3] - a[3]) * t,
  ];
  if (multiplier <= 1) {
    return g1;
  }
  if (multiplier < 2) {
    return lerp(g1, g2, multiplier - 1);
  }
  if (multiplier < 3) {
    return lerp(g2, g3, multiplier - 2);
  }
  return g3;
}
