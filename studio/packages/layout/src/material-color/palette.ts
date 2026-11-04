// SPDX-FileCopyrightText: 2021 Google LLC
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0 AND AGPL-3.0-only

// Ported from material_color_utilities 0.13.0 (lib/palettes/tonal_palette.dart, lib/dislike/dislike_analyzer.dart,
// lib/contrast/contrast.dart).

import { clampDouble, dartRound, lstarFromY, yFromLstar } from "./color-utils.ts";
import { Hct } from "./hct.ts";

/** The tone of `hue` and `requestedChroma` closest to tone 50 that reaches the chroma (`KeyColor.create`). */
function keyColor(hue: number, requestedChroma: number): Hct {
  const cache = new Map<number, number>();
  const maxChroma = (tone: number): number => {
    let c = cache.get(tone);
    if (c === undefined) {
      c = Hct.from(hue, 200.0, tone).chroma;
      cache.set(tone, c);
    }
    return c;
  };
  const pivotTone = 50;
  const epsilon = 0.01;
  let lowerTone = 0;
  let upperTone = 100;
  while (lowerTone < upperTone) {
    const midTone = Math.floor((lowerTone + upperTone) / 2);
    const isAscending = maxChroma(midTone) < maxChroma(midTone + 1);
    const sufficientChroma = maxChroma(midTone) >= requestedChroma - epsilon;
    if (sufficientChroma) {
      if (Math.abs(lowerTone - pivotTone) < Math.abs(upperTone - pivotTone)) {
        upperTone = midTone;
      } else {
        if (lowerTone === midTone) {
          return Hct.from(hue, requestedChroma, lowerTone);
        }
        lowerTone = midTone;
      }
    } else if (isAscending) {
      lowerTone = midTone + 1;
    } else {
      upperTone = midTone;
    }
  }
  return Hct.from(hue, requestedChroma, lowerTone);
}

/** A hue and a chroma at every tone: the source of a scheme's roles. */
export class TonalPalette {
  private readonly cache = new Map<number, number>();
  readonly keyColor: Hct;

  private constructor(
    readonly hue: number,
    readonly chroma: number,
    key?: Hct,
  ) {
    this.keyColor = key ?? keyColor(hue, chroma);
  }

  static of(hue: number, chroma: number): TonalPalette {
    return new TonalPalette(hue, chroma);
  }

  static fromHct(hct: Hct): TonalPalette {
    return new TonalPalette(hct.hue, hct.chroma, hct);
  }

  /** The colour at integer `tone`. */
  get(tone: number): number {
    let argb = this.cache.get(tone);
    if (argb === undefined) {
      argb = Hct.from(this.hue, this.chroma, tone).toInt();
      this.cache.set(tone, argb);
    }
    return argb;
  }

  getHct(tone: number): Hct {
    const cached = this.cache.get(tone);
    return cached === undefined ? Hct.from(this.hue, this.chroma, tone) : Hct.fromInt(cached);
  }
}

/** Yellow-greens people dislike are lifted to tone 70 (`DislikeAnalyzer`). */
export function isDisliked(hct: Hct): boolean {
  const hue = dartRound(hct.hue);
  return hue >= 90.0 && hue <= 111.0 && dartRound(hct.chroma) > 16.0 && dartRound(hct.tone) < 65.0;
}

export function fixIfDisliked(hct: Hct): Hct {
  return isDisliked(hct) ? Hct.from(hct.hue, hct.chroma, 70.0) : hct;
}

function ratioOfYs(y1: number, y2: number): number {
  const lighter = y1 > y2 ? y1 : y2;
  const darker = lighter === y2 ? y1 : y2;
  return (lighter + 5.0) / (darker + 5.0);
}

/** The WCAG contrast ratio of two tones (`Contrast.ratioOfTones`). */
export function ratioOfTones(toneA: number, toneB: number): number {
  return ratioOfYs(yFromLstar(clampDouble(0.0, 100.0, toneA)), yFromLstar(clampDouble(0.0, 100.0, toneB)));
}

/** The tone lighter than `tone` reaching `ratio`, or -1 when none does. */
export function lighter(tone: number, ratio: number): number {
  if (tone < 0.0 || tone > 100.0) {
    return -1.0;
  }
  const darkY = yFromLstar(tone);
  const lightY = ratio * (darkY + 5.0) - 5.0;
  const real = ratioOfYs(lightY, darkY);
  if (real < ratio && Math.abs(real - ratio) > 0.04) {
    return -1;
  }
  const value = lstarFromY(lightY) + 0.4;
  return value < 0 || value > 100 ? -1 : value;
}

/** The tone darker than `tone` reaching `ratio`, or -1 when none does. */
export function darker(tone: number, ratio: number): number {
  if (tone < 0.0 || tone > 100.0) {
    return -1.0;
  }
  const lightY = yFromLstar(tone);
  const darkY = (lightY + 5.0) / ratio - 5.0;
  const real = ratioOfYs(lightY, darkY);
  if (real < ratio && Math.abs(real - ratio) > 0.04) {
    return -1;
  }
  const value = lstarFromY(darkY) - 0.4;
  return value < 0 || value > 100 ? -1 : value;
}

export function lighterUnsafe(tone: number, ratio: number): number {
  const safe = lighter(tone, ratio);
  return safe < 0.0 ? 100.0 : safe;
}

export function darkerUnsafe(tone: number, ratio: number): number {
  const safe = darker(tone, ratio);
  return safe < 0.0 ? 0.0 : safe;
}
