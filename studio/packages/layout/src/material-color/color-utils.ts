// SPDX-FileCopyrightText: 2021 Google LLC
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0 AND AGPL-3.0-only

// Ported from material_color_utilities 0.13.0 (lib/utils/color_utils.dart, lib/utils/math_utils.dart), the version
// the pinned Flutter uses (ADR-0013). Colours are ARGB integers held as unsigned 32-bit numbers.

const SRGB_TO_XYZ: readonly (readonly number[])[] = [
  [0.41233895, 0.35762064, 0.18051042],
  [0.2126, 0.7152, 0.0722],
  [0.01932141, 0.11916382, 0.95034478],
];

const XYZ_TO_SRGB: readonly (readonly number[])[] = [
  [3.2413774792388685, -1.5376652402851851, -0.49885366846268053],
  [-0.9691452513005321, 1.8758853451067872, 0.04156585616912061],
  [0.05562093689691305, -0.20395524564742123, 1.0571799111220335],
];

const WHITE_POINT_D65: readonly number[] = [95.047, 100.0, 108.883];

/** Dart's `round()`: half away from zero (JavaScript's `Math.round` rounds half towards positive infinity). */
export function dartRound(x: number): number {
  return x < 0 ? -Math.round(-x) : Math.round(x);
}

export function signum(x: number): number {
  return x < 0 ? -1 : x === 0 ? 0 : 1;
}

export function lerp(start: number, stop: number, amount: number): number {
  return (1.0 - amount) * start + amount * stop;
}

export function clampInt(min: number, max: number, input: number): number {
  return input < min ? min : input > max ? max : input;
}

export function clampDouble(min: number, max: number, input: number): number {
  return input < min ? min : input > max ? max : input;
}

export function sanitizeDegreesInt(degrees: number): number {
  const d = degrees % 360;
  return d < 0 ? d + 360 : d;
}

export function sanitizeDegreesDouble(degrees: number): number {
  const d = degrees % 360.0;
  return d < 0 ? d + 360.0 : d;
}

export function rotationDirection(from: number, to: number): number {
  return sanitizeDegreesDouble(to - from) <= 180.0 ? 1.0 : -1.0;
}

export function differenceDegrees(a: number, b: number): number {
  return 180.0 - Math.abs(Math.abs(a - b) - 180.0);
}

export function matrixMultiply(row: readonly number[], matrix: readonly (readonly number[])[]): number[] {
  const m0 = matrix[0] as readonly number[];
  const m1 = matrix[1] as readonly number[];
  const m2 = matrix[2] as readonly number[];
  const [r0, r1, r2] = row as [number, number, number];
  return [
    r0 * (m0[0] as number) + r1 * (m0[1] as number) + r2 * (m0[2] as number),
    r0 * (m1[0] as number) + r1 * (m1[1] as number) + r2 * (m1[2] as number),
    r0 * (m2[0] as number) + r1 * (m2[1] as number) + r2 * (m2[2] as number),
  ];
}

export function argbFromRgb(red: number, green: number, blue: number): number {
  return ((255 << 24) | ((red & 255) << 16) | ((green & 255) << 8) | (blue & 255)) >>> 0;
}

export function alphaFromArgb(argb: number): number {
  return (argb >>> 24) & 255;
}

export function redFromArgb(argb: number): number {
  return (argb >>> 16) & 255;
}

export function greenFromArgb(argb: number): number {
  return (argb >>> 8) & 255;
}

export function blueFromArgb(argb: number): number {
  return argb & 255;
}

export function linearized(rgbComponent: number): number {
  const normalized = rgbComponent / 255.0;
  return normalized <= 0.040449936 ? (normalized / 12.92) * 100.0 : ((normalized + 0.055) / 1.055) ** 2.4 * 100.0;
}

export function delinearized(rgbComponent: number): number {
  const normalized = rgbComponent / 100.0;
  const d = normalized <= 0.0031308 ? normalized * 12.92 : 1.055 * normalized ** (1.0 / 2.4) - 0.055;
  return clampInt(0, 255, dartRound(d * 255.0));
}

export function argbFromLinrgb(linrgb: readonly number[]): number {
  return argbFromRgb(
    delinearized(linrgb[0] as number),
    delinearized(linrgb[1] as number),
    delinearized(linrgb[2] as number),
  );
}

export function argbFromXyz(x: number, y: number, z: number): number {
  const m = XYZ_TO_SRGB;
  const row = (i: number) => {
    const r = m[i] as readonly number[];
    return (r[0] as number) * x + (r[1] as number) * y + (r[2] as number) * z;
  };
  return argbFromRgb(delinearized(row(0)), delinearized(row(1)), delinearized(row(2)));
}

export function xyzFromArgb(argb: number): number[] {
  return matrixMultiply(
    [linearized(redFromArgb(argb)), linearized(greenFromArgb(argb)), linearized(blueFromArgb(argb))],
    SRGB_TO_XYZ,
  );
}

function labF(t: number): number {
  const e = 216.0 / 24389.0;
  const kappa = 24389.0 / 27.0;
  return t > e ? t ** (1.0 / 3.0) : (kappa * t + 16) / 116;
}

function labInvf(ft: number): number {
  const e = 216.0 / 24389.0;
  const kappa = 24389.0 / 27.0;
  const ft3 = ft * ft * ft;
  return ft3 > e ? ft3 : (116 * ft - 16) / kappa;
}

export function yFromLstar(lstar: number): number {
  return 100.0 * labInvf((lstar + 16.0) / 116.0);
}

export function lstarFromY(y: number): number {
  return labF(y / 100.0) * 116.0 - 16.0;
}

export function argbFromLstar(lstar: number): number {
  const component = delinearized(yFromLstar(lstar));
  return argbFromRgb(component, component, component);
}

export function lstarFromArgb(argb: number): number {
  return 116.0 * labF((xyzFromArgb(argb)[1] as number) / 100.0) - 16.0;
}

export function whitePointD65(): readonly number[] {
  return WHITE_POINT_D65;
}
