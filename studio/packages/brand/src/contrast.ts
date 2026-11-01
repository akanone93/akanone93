// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * WCAG 2.2 contrast calculations. The same formula backs the publish-time
 * contrast check of A11Y-003, so Studio can warn while a designer edits.
 */

import type { HexColor } from "./tokens.ts";

const hexPattern = /^#[0-9A-Fa-f]{6}$/;

/** Reports whether a string is a six-digit hexadecimal colour. */
export function isHexColor(value: string): value is HexColor {
  return hexPattern.test(value);
}

/** Converts one sRGB channel (0–255) to linear light, per WCAG 2.2. */
function linearChannel(channel: number): number {
  const c = channel / 255;
  return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
}

/**
 * Returns the relative luminance of a colour, from 0 (black) to 1 (white).
 * Throws a RangeError for anything but a six-digit hexadecimal colour.
 */
export function relativeLuminance(color: string): number {
  if (!isHexColor(color)) {
    throw new RangeError(`not a six-digit hex colour: ${color}`);
  }
  const r = Number.parseInt(color.slice(1, 3), 16);
  const g = Number.parseInt(color.slice(3, 5), 16);
  const b = Number.parseInt(color.slice(5, 7), 16);
  return 0.2126 * linearChannel(r) + 0.7152 * linearChannel(g) + 0.0722 * linearChannel(b);
}

/**
 * Returns the relative luminance of an opaque `oklch(L C H)` colour, as shadcn's theme writes them: OKLab to linear
 * sRGB (Björn Ottosson's matrices), each channel clipped to the sRGB gamut.
 */
export function oklchLuminance(color: string): number {
  const match = /^oklch\(([\d.]+) ([\d.]+) ([\d.]+)\)$/.exec(color);
  if (match === null) {
    throw new RangeError(`not an opaque oklch() colour: ${color}`);
  }
  const [l, c, h] = [Number(match[1]), Number(match[2]), (Number(match[3]) * Math.PI) / 180];
  const a = c * Math.cos(h);
  const b = c * Math.sin(h);
  const l_ = (l + 0.3963377774 * a + 0.2158037573 * b) ** 3;
  const m_ = (l - 0.1055613458 * a - 0.0638541728 * b) ** 3;
  const s_ = (l - 0.0894841775 * a - 1.291485548 * b) ** 3;
  const clip = (v: number) => Math.min(1, Math.max(0, v));
  const r = clip(4.0767416621 * l_ - 3.3077115913 * m_ + 0.2309699292 * s_);
  const g = clip(-1.2684380046 * l_ + 2.6097574011 * m_ - 0.3413193965 * s_);
  const bl = clip(-0.0041960863 * l_ - 0.7034186147 * m_ + 1.707614701 * s_);
  return 0.2126 * r + 0.7152 * g + 0.0722 * bl;
}

/** Returns the WCAG contrast ratio of two `oklch()` colours, from 1 to 21. */
export function oklchContrast(a: string, b: string): number {
  const la = oklchLuminance(a);
  const lb = oklchLuminance(b);
  const [light, dark] = la >= lb ? [la, lb] : [lb, la];
  return (light + 0.05) / (dark + 0.05);
}

/** Returns the WCAG contrast ratio of two colours, from 1 to 21. */
export function contrastRatio(a: string, b: string): number {
  const la = relativeLuminance(a);
  const lb = relativeLuminance(b);
  const [light, dark] = la >= lb ? [la, lb] : [lb, la];
  return (light + 0.05) / (dark + 0.05);
}

/** WCAG 2.2 level AA minimum contrast for normal-size text. */
export const minimumTextContrast = 4.5;

/** WCAG 2.2 level AA minimum contrast for large text and UI components. */
export const minimumComponentContrast = 3;
