// SPDX-FileCopyrightText: 2021 Google LLC
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0 AND AGPL-3.0-only

// Ported from material_color_utilities 0.13.0 (lib/dynamiccolor/dynamic_color.dart, dynamic_scheme.dart,
// material_dynamic_colors.dart, src/contrast_curve.dart, src/tone_delta_pair.dart, lib/scheme/scheme_tonal_spot.dart)
// and from Flutter 3.47.5's `ColorScheme.fromSeed` (packages/flutter/lib/src/material/color_scheme.dart). Only the
// tonal spot variant is ported: it is the one `ColorScheme.fromSeed` uses by default and the one the runtime asks for.

import { clampDouble, dartRound, lerp, sanitizeDegreesDouble } from "./color-utils.ts";
import { Hct } from "./hct.ts";
import { darker, darkerUnsafe, lighter, lighterUnsafe, ratioOfTones, TonalPalette } from "./palette.ts";

/** A tonal spot scheme: the palettes of a source colour, in light or dark, at a contrast level. */
export interface DynamicScheme {
  readonly sourceColorHct: Hct;
  readonly isDark: boolean;
  readonly contrastLevel: number;
  readonly primaryPalette: TonalPalette;
  readonly secondaryPalette: TonalPalette;
  readonly tertiaryPalette: TonalPalette;
  readonly neutralPalette: TonalPalette;
  readonly neutralVariantPalette: TonalPalette;
  readonly errorPalette: TonalPalette;
}

/** `SchemeTonalSpot`. */
export function schemeTonalSpot(sourceColorHct: Hct, isDark: boolean, contrastLevel = 0.0): DynamicScheme {
  const hue = sourceColorHct.hue;
  return {
    sourceColorHct,
    isDark,
    contrastLevel,
    primaryPalette: TonalPalette.of(hue, 36.0),
    secondaryPalette: TonalPalette.of(hue, 16.0),
    tertiaryPalette: TonalPalette.of(sanitizeDegreesDouble(hue + 60.0), 24.0),
    neutralPalette: TonalPalette.of(hue, 6.0),
    neutralVariantPalette: TonalPalette.of(hue, 8.0),
    errorPalette: TonalPalette.of(25.0, 84.0),
  };
}

/** How much contrast a colour needs against its background, by contrast level (`ContrastCurve`). */
class ContrastCurve {
  constructor(
    readonly low: number,
    readonly normal: number,
    readonly medium: number,
    readonly high: number,
  ) {}

  get(level: number): number {
    if (level <= -1.0) {
      return this.low;
    }
    if (level < 0.0) {
      return lerp(this.low, this.normal, level + 1);
    }
    if (level < 0.5) {
      return lerp(this.normal, this.medium, level / 0.5);
    }
    if (level < 1.0) {
      return lerp(this.medium, this.high, (level - 0.5) / 0.5);
    }
    return this.high;
  }
}

type Polarity = "darker" | "lighter" | "nearer" | "farther";

interface ToneDeltaPair {
  readonly roleA: DynamicColor;
  readonly roleB: DynamicColor;
  readonly delta: number;
  readonly polarity: Polarity;
  readonly stayTogether: boolean;
}

interface DynamicColorSpec {
  readonly name: string;
  readonly palette: (s: DynamicScheme) => TonalPalette;
  readonly tone: (s: DynamicScheme) => number;
  readonly isBackground?: boolean;
  readonly background?: (s: DynamicScheme) => DynamicColor;
  readonly secondBackground?: (s: DynamicScheme) => DynamicColor;
  readonly contrastCurve?: ContrastCurve;
  readonly toneDeltaPair?: (s: DynamicScheme) => ToneDeltaPair;
}

const tonePrefersLightForeground = (tone: number): boolean => dartRound(tone) < 60;

/** The tone that reaches `ratio` against `bgTone`, on the side the background prefers (`DynamicColor.foregroundTone`). */
export function foregroundTone(bgTone: number, ratio: number): number {
  const lighterTone = lighterUnsafe(bgTone, ratio);
  const darkerTone = darkerUnsafe(bgTone, ratio);
  const lighterRatio = ratioOfTones(lighterTone, bgTone);
  const darkerRatio = ratioOfTones(darkerTone, bgTone);
  if (tonePrefersLightForeground(bgTone)) {
    const negligible = Math.abs(lighterRatio - darkerRatio) < 0.1 && lighterRatio < ratio && darkerRatio < ratio;
    return lighterRatio >= ratio || lighterRatio >= darkerRatio || negligible ? lighterTone : darkerTone;
  }
  return darkerRatio >= ratio || darkerRatio >= lighterRatio ? darkerTone : lighterTone;
}

/** A colour role: a palette and a tone, adjusted for contrast against its backgrounds (`DynamicColor`). */
class DynamicColor {
  private readonly cache = new Map<DynamicScheme, Hct>();

  constructor(readonly spec: DynamicColorSpec) {}

  get name(): string {
    return this.spec.name;
  }

  tone(s: DynamicScheme): number {
    return this.spec.tone(s);
  }

  get contrastCurve(): ContrastCurve | undefined {
    return this.spec.contrastCurve;
  }

  getArgb(s: DynamicScheme): number {
    return this.getHct(s).toInt();
  }

  getHct(s: DynamicScheme): Hct {
    const cached = this.cache.get(s);
    if (cached !== undefined) {
      return cached;
    }
    const answer = this.spec.palette(s).getHct(this.getTone(s));
    if (this.cache.size > 4) {
      this.cache.clear();
    }
    this.cache.set(s, answer);
    return answer;
  }

  getTone(s: DynamicScheme): number {
    return this.spec.toneDeltaPair === undefined ? this.singleTone(s) : this.pairTone(s, this.spec.toneDeltaPair(s));
  }

  // biome-ignore lint/complexity/noExcessiveCognitiveComplexity: a line-for-line port of DynamicColor.getTone's tone-delta-pair branch
  private pairTone(s: DynamicScheme, pair: ToneDeltaPair): number {
    const decreasingContrast = s.contrastLevel < 0;
    const bgTone = (this.spec.background as (s: DynamicScheme) => DynamicColor)(s).getTone(s);
    const aIsNearer =
      pair.polarity === "nearer" ||
      (pair.polarity === "lighter" && !s.isDark) ||
      (pair.polarity === "darker" && s.isDark);
    const nearer = aIsNearer ? pair.roleA : pair.roleB;
    const farther = aIsNearer ? pair.roleB : pair.roleA;
    const amNearer = this.name === nearer.name;
    const expansionDir = s.isDark ? 1 : -1;
    const nContrast = (nearer.contrastCurve as ContrastCurve).get(s.contrastLevel);
    const fContrast = (farther.contrastCurve as ContrastCurve).get(s.contrastLevel);
    const nInitial = nearer.tone(s);
    let nTone = ratioOfTones(bgTone, nInitial) >= nContrast ? nInitial : foregroundTone(bgTone, nContrast);
    const fInitial = farther.tone(s);
    let fTone = ratioOfTones(bgTone, fInitial) >= fContrast ? fInitial : foregroundTone(bgTone, fContrast);
    if (decreasingContrast) {
      nTone = foregroundTone(bgTone, nContrast);
      fTone = foregroundTone(bgTone, fContrast);
    }
    const { delta } = pair;
    if ((fTone - nTone) * expansionDir < delta) {
      fTone = clampDouble(0, 100, nTone + delta * expansionDir);
      if ((fTone - nTone) * expansionDir < delta) {
        nTone = clampDouble(0, 100, fTone - delta * expansionDir);
      }
    }
    const avoidMidTones = () => {
      if (expansionDir > 0) {
        nTone = 60;
        fTone = Math.max(fTone, nTone + delta * expansionDir);
      } else {
        nTone = 49;
        fTone = Math.min(fTone, nTone + delta * expansionDir);
      }
    };
    if (nTone >= 50 && nTone < 60) {
      avoidMidTones();
    } else if (fTone >= 50 && fTone < 60) {
      if (pair.stayTogether) {
        avoidMidTones();
      } else {
        fTone = expansionDir > 0 ? 60 : 49;
      }
    }
    return amNearer ? nTone : fTone;
  }

  // biome-ignore lint/complexity/noExcessiveCognitiveComplexity: a line-for-line port of DynamicColor.getTone's single-colour branch
  private singleTone(s: DynamicScheme): number {
    let answer = this.spec.tone(s);
    if (this.spec.background === undefined) {
      return answer;
    }
    const bgTone = this.spec.background(s).getTone(s);
    const desired = (this.spec.contrastCurve as ContrastCurve).get(s.contrastLevel);
    if (ratioOfTones(bgTone, answer) < desired) {
      answer = foregroundTone(bgTone, desired);
    }
    if (s.contrastLevel < 0) {
      answer = foregroundTone(bgTone, desired);
    }
    if (this.spec.isBackground === true && answer >= 50 && answer < 60) {
      answer = ratioOfTones(49, bgTone) >= desired ? 49 : 60;
    }
    if (this.spec.secondBackground === undefined) {
      return answer;
    }
    const bgTone2 = this.spec.secondBackground(s).getTone(s);
    const upper = Math.max(bgTone, bgTone2);
    const lower = Math.min(bgTone, bgTone2);
    if (ratioOfTones(upper, answer) >= desired && ratioOfTones(lower, answer) >= desired) {
      return answer;
    }
    const lightOption = lighter(upper, desired);
    const darkOption = darker(lower, desired);
    const available = [lightOption, darkOption].filter((t) => t !== -1);
    if (tonePrefersLightForeground(bgTone) || tonePrefersLightForeground(bgTone2)) {
      return lightOption < 0 ? 100 : lightOption;
    }
    if (available.length === 1) {
      return available[0] as number;
    }
    return darkOption < 0 ? 0 : darkOption;
  }
}

const curve = (low: number, normal: number, medium: number, high: number) =>
  new ContrastCurve(low, normal, medium, high);

/** The roles of `MaterialDynamicColors` that `ColorScheme.fromSeed` reads, for the tonal spot variant. */
function makeRoles() {
  const color = (spec: DynamicColorSpec) => new DynamicColor(spec);
  const neutral = (s: DynamicScheme) => s.neutralPalette;
  const neutralVariant = (s: DynamicScheme) => s.neutralVariantPalette;
  const primaryP = (s: DynamicScheme) => s.primaryPalette;
  const secondaryP = (s: DynamicScheme) => s.secondaryPalette;
  const tertiaryP = (s: DynamicScheme) => s.tertiaryPalette;
  const errorP = (s: DynamicScheme) => s.errorPalette;
  const r = {} as Record<string, DynamicColor>;
  const highestSurface = (s: DynamicScheme) =>
    s.isDark ? (r["surfaceBright"] as DynamicColor) : (r["surfaceDim"] as DynamicColor);
  const role = (name: string) => () => r[name] as DynamicColor;
  const pair = (a: string, b: string, polarity: Polarity, stayTogether: boolean) => () => ({
    roleA: r[a] as DynamicColor,
    roleB: r[b] as DynamicColor,
    delta: 10,
    polarity,
    stayTogether,
  });
  const surfaceTone = (dark: number | ContrastCurve, light: number | ContrastCurve) => (s: DynamicScheme) => {
    const v = s.isDark ? dark : light;
    return typeof v === "number" ? v : v.get(s.contrastLevel);
  };
  const background = (name: string, palette: (s: DynamicScheme) => TonalPalette, tone: (s: DynamicScheme) => number) =>
    color({ name, palette, tone, isBackground: true });
  r["background"] = background("background", neutral, (s) => (s.isDark ? 6 : 98));
  r["onBackground"] = color({
    name: "on_background",
    palette: neutral,
    tone: (s) => (s.isDark ? 90 : 10),
    background: role("background"),
    contrastCurve: curve(3, 3, 4.5, 7),
  });
  r["surface"] = background("surface", neutral, (s) => (s.isDark ? 6 : 98));
  r["surfaceDim"] = background("surface_dim", neutral, surfaceTone(6, curve(87, 87, 80, 75)));
  r["surfaceBright"] = background("surface_bright", neutral, surfaceTone(curve(24, 24, 29, 34), 98));
  r["surfaceContainerLowest"] = background("surface_container_lowest", neutral, surfaceTone(curve(4, 4, 2, 0), 100));
  r["surfaceContainerLow"] = background(
    "surface_container_low",
    neutral,
    surfaceTone(curve(10, 10, 11, 12), curve(96, 96, 96, 95)),
  );
  r["surfaceContainer"] = background(
    "surface_container",
    neutral,
    surfaceTone(curve(12, 12, 16, 20), curve(94, 94, 92, 90)),
  );
  r["surfaceContainerHigh"] = background(
    "surface_container_high",
    neutral,
    surfaceTone(curve(17, 17, 21, 25), curve(92, 92, 88, 85)),
  );
  r["surfaceContainerHighest"] = background(
    "surface_container_highest",
    neutral,
    surfaceTone(curve(22, 22, 26, 30), curve(90, 90, 84, 80)),
  );
  r["onSurface"] = color({
    name: "on_surface",
    palette: neutral,
    tone: (s) => (s.isDark ? 90 : 10),
    background: highestSurface,
    contrastCurve: curve(4.5, 7, 11, 21),
  });
  r["surfaceVariant"] = background("surface_variant", neutralVariant, (s) => (s.isDark ? 30 : 90));
  r["onSurfaceVariant"] = color({
    name: "on_surface_variant",
    palette: neutralVariant,
    tone: (s) => (s.isDark ? 80 : 30),
    background: highestSurface,
    contrastCurve: curve(3, 4.5, 7, 11),
  });
  r["inverseSurface"] = color({ name: "inverse_surface", palette: neutral, tone: (s) => (s.isDark ? 90 : 20) });
  r["inverseOnSurface"] = color({
    name: "inverse_on_surface",
    palette: neutral,
    tone: (s) => (s.isDark ? 20 : 95),
    background: role("inverseSurface"),
    contrastCurve: curve(4.5, 7, 11, 21),
  });
  r["outline"] = color({
    name: "outline",
    palette: neutralVariant,
    tone: (s) => (s.isDark ? 60 : 50),
    background: highestSurface,
    contrastCurve: curve(1.5, 3, 4.5, 7),
  });
  r["outlineVariant"] = color({
    name: "outline_variant",
    palette: neutralVariant,
    tone: (s) => (s.isDark ? 30 : 80),
    background: highestSurface,
    contrastCurve: curve(1, 1, 3, 4.5),
  });
  r["shadow"] = color({ name: "shadow", palette: neutral, tone: () => 0 });
  r["scrim"] = color({ name: "scrim", palette: neutral, tone: () => 0 });
  const accent = (key: string, name: string, palette: (s: DynamicScheme) => TonalPalette, container: string) => {
    r[key] = color({
      name,
      palette,
      tone: (s) => (s.isDark ? 80 : 40),
      isBackground: true,
      background: highestSurface,
      contrastCurve: curve(3, 4.5, 7, 7),
      toneDeltaPair: pair(container, key, "nearer", false),
    });
    r[`on${key[0]?.toUpperCase()}${key.slice(1)}`] = color({
      name: `on_${name}`,
      palette,
      tone: (s) => (s.isDark ? 20 : 100),
      background: role(key),
      contrastCurve: curve(4.5, 7, 11, 21),
    });
    r[container] = color({
      name: `${name}_container`,
      palette,
      tone: (s) => (s.isDark ? 30 : 90),
      isBackground: true,
      background: highestSurface,
      contrastCurve: curve(1, 1, 3, 4.5),
      toneDeltaPair: pair(container, key, "nearer", false),
    });
    r[`on${container[0]?.toUpperCase()}${container.slice(1)}`] = color({
      name: `on_${name}_container`,
      palette,
      tone: (s) => (s.isDark ? 90 : 30),
      background: role(container),
      contrastCurve: curve(3, 4.5, 7, 11),
    });
  };
  accent("primary", "primary", primaryP, "primaryContainer");
  accent("secondary", "secondary", secondaryP, "secondaryContainer");
  accent("tertiary", "tertiary", tertiaryP, "tertiaryContainer");
  accent("error", "error", errorP, "errorContainer");
  r["inversePrimary"] = color({
    name: "inverse_primary",
    palette: primaryP,
    tone: (s) => (s.isDark ? 40 : 80),
    background: role("inverseSurface"),
    contrastCurve: curve(3, 4.5, 7, 7),
  });
  const fixed = (key: string, name: string, palette: (s: DynamicScheme) => TonalPalette) => {
    const dim = `${key}FixedDim`;
    const fix = `${key}Fixed`;
    r[fix] = color({
      name: `${name}_fixed`,
      palette,
      tone: () => 90.0,
      isBackground: true,
      background: highestSurface,
      contrastCurve: curve(1, 1, 3, 4.5),
      toneDeltaPair: pair(fix, dim, "lighter", true),
    });
    r[dim] = color({
      name: `${name}_fixed_dim`,
      palette,
      tone: () => 80.0,
      isBackground: true,
      background: highestSurface,
      contrastCurve: curve(1, 1, 3, 4.5),
      toneDeltaPair: pair(fix, dim, "lighter", true),
    });
    const cap = `${key[0]?.toUpperCase()}${key.slice(1)}`;
    r[`on${cap}Fixed`] = color({
      name: `on_${name}_fixed`,
      palette,
      tone: () => 10.0,
      background: role(dim),
      secondBackground: role(fix),
      contrastCurve: curve(4.5, 7, 11, 21),
    });
    r[`on${cap}FixedVariant`] = color({
      name: `on_${name}_fixed_variant`,
      palette,
      tone: () => 30.0,
      background: role(dim),
      secondBackground: role(fix),
      contrastCurve: curve(3, 4.5, 7, 11),
    });
  };
  fixed("primary", "primary", primaryP);
  fixed("secondary", "secondary", secondaryP);
  fixed("tertiary", "tertiary", tertiaryP);
  return r;
}

const ROLES = makeRoles();

/** The colour roles of a Material 3 colour scheme (Flutter's `ColorScheme`), as unsigned ARGB integers. */
export const colorRoleNames = [
  "primary",
  "onPrimary",
  "primaryContainer",
  "onPrimaryContainer",
  "primaryFixed",
  "primaryFixedDim",
  "onPrimaryFixed",
  "onPrimaryFixedVariant",
  "secondary",
  "onSecondary",
  "secondaryContainer",
  "onSecondaryContainer",
  "secondaryFixed",
  "secondaryFixedDim",
  "onSecondaryFixed",
  "onSecondaryFixedVariant",
  "tertiary",
  "onTertiary",
  "tertiaryContainer",
  "onTertiaryContainer",
  "tertiaryFixed",
  "tertiaryFixedDim",
  "onTertiaryFixed",
  "onTertiaryFixedVariant",
  "error",
  "onError",
  "errorContainer",
  "onErrorContainer",
  "surface",
  "onSurface",
  "surfaceDim",
  "surfaceBright",
  "surfaceContainerLowest",
  "surfaceContainerLow",
  "surfaceContainer",
  "surfaceContainerHigh",
  "surfaceContainerHighest",
  "onSurfaceVariant",
  "outline",
  "outlineVariant",
  "shadow",
  "scrim",
  "inverseSurface",
  "onInverseSurface",
  "inversePrimary",
  "surfaceTint",
] as const;

export type ColorRole = (typeof colorRoleNames)[number];
export type ColorScheme = Readonly<Record<ColorRole, number>> & { readonly brightness: "light" | "dark" };

/** Flutter's default seed, the one the runtime uses when the theme names no primary colour (`_baselineSeed`). */
export const BASELINE_SEED = 0xff6750a4;

/** `ColorScheme.fromSeed(seedColor: seed, brightness: …)` with the tonal spot variant at standard contrast. */
export function colorSchemeFromSeed(seed: number, dark: boolean): ColorScheme {
  const scheme = schemeTonalSpot(Hct.fromInt(seed >>> 0), dark, 0.0);
  const out = { brightness: dark ? "dark" : "light" } as Record<string, number | string>;
  for (const name of colorRoleNames) {
    const source = name === "onInverseSurface" ? "inverseOnSurface" : name === "surfaceTint" ? "primary" : name;
    out[name] = (ROLES[source] as DynamicColor).getArgb(scheme);
  }
  return out as unknown as ColorScheme;
}

/** `scheme` with the roles in `overrides` replaced (the runtime's `withColorRoles`). */
export function withColorRoles(scheme: ColorScheme, overrides: Partial<Record<ColorRole, number>>): ColorScheme {
  return { ...scheme, ...overrides };
}
