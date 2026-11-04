// SPDX-FileCopyrightText: 2021 Google LLC
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0 AND AGPL-3.0-only

// Ported from material_color_utilities 0.13.0 (lib/hct/hct.dart, lib/hct/src/hct_solver.dart).

import { Cam16, makeViewingConditions, STANDARD_VIEWING_CONDITIONS, type ViewingConditions } from "./cam16.ts";
import {
  argbFromLinrgb,
  argbFromLstar,
  lstarFromArgb,
  lstarFromY,
  matrixMultiply,
  sanitizeDegreesDouble,
  signum,
  yFromLstar,
} from "./color-utils.ts";
import {
  CRITICAL_PLANES,
  LINRGB_FROM_SCALED_DISCOUNT,
  SCALED_DISCOUNT_FROM_LINRGB,
  Y_FROM_LINRGB,
} from "./hct-tables.ts";

const [K_R, K_G, K_B] = Y_FROM_LINRGB as [number, number, number];

function sanitizeRadians(angle: number): number {
  return (angle + Math.PI * 8) % (Math.PI * 2);
}

function trueDelinearized(rgbComponent: number): number {
  const normalized = rgbComponent / 100.0;
  const d = normalized <= 0.0031308 ? normalized * 12.92 : 1.055 * normalized ** (1.0 / 2.4) - 0.055;
  return d * 255.0;
}

function chromaticAdaptation(component: number): number {
  const af = Math.abs(component) ** 0.42;
  return (signum(component) * 400.0 * af) / (af + 27.13);
}

function hueOf(linrgb: readonly number[]): number {
  const [s0, s1, s2] = matrixMultiply(linrgb, SCALED_DISCOUNT_FROM_LINRGB) as [number, number, number];
  const rA = chromaticAdaptation(s0);
  const gA = chromaticAdaptation(s1);
  const bA = chromaticAdaptation(s2);
  return Math.atan2((rA + gA - 2.0 * bA) / 9.0, (11.0 * rA + -12.0 * gA + bA) / 11.0);
}

function areInCyclicOrder(a: number, b: number, c: number): boolean {
  return sanitizeRadians(b - a) < sanitizeRadians(c - a);
}

function setCoordinate(
  source: readonly number[],
  coordinate: number,
  target: readonly number[],
  axis: number,
): number[] {
  const s = source[axis] as number;
  const t = (coordinate - s) / ((target[axis] as number) - s);
  return [0, 1, 2].map((i) => (source[i] as number) + ((target[i] as number) - (source[i] as number)) * t);
}

const isBounded = (x: number): boolean => x >= 0.0 && x <= 100.0;

function nthVertex(y: number, n: number): number[] {
  const coordA = n % 4 <= 1 ? 0.0 : 100.0;
  const coordB = n % 2 === 0 ? 0.0 : 100.0;
  if (n < 4) {
    const r = (y - coordA * K_G - coordB * K_B) / K_R;
    return isBounded(r) ? [r, coordA, coordB] : [-1.0, -1.0, -1.0];
  }
  if (n < 8) {
    const g = (y - coordB * K_R - coordA * K_B) / K_G;
    return isBounded(g) ? [coordB, g, coordA] : [-1.0, -1.0, -1.0];
  }
  const b = (y - coordA * K_R - coordB * K_G) / K_B;
  return isBounded(b) ? [coordA, coordB, b] : [-1.0, -1.0, -1.0];
}

function bisectToSegment(y: number, targetHue: number): [number[], number[]] {
  let left = [-1.0, -1.0, -1.0];
  let right = left;
  let leftHue = 0.0;
  let rightHue = 0.0;
  let initialized = false;
  let uncut = true;
  for (let n = 0; n < 12; n++) {
    const mid = nthVertex(y, n);
    if ((mid[0] as number) < 0) {
      continue;
    }
    const midHue = hueOf(mid);
    if (!initialized) {
      left = mid;
      right = mid;
      leftHue = midHue;
      rightHue = midHue;
      initialized = true;
      continue;
    }
    if (uncut || areInCyclicOrder(leftHue, midHue, rightHue)) {
      uncut = false;
      if (areInCyclicOrder(leftHue, targetHue, midHue)) {
        right = mid;
        rightHue = midHue;
      } else {
        left = mid;
        leftHue = midHue;
      }
    }
  }
  return [left, right];
}

function bisectToLimit(y: number, targetHue: number): number[] {
  let [left, right] = bisectToSegment(y, targetHue);
  let leftHue = hueOf(left);
  for (let axis = 0; axis < 3; axis++) {
    const l = left[axis] as number;
    const r = right[axis] as number;
    if (l === r) {
      continue;
    }
    let lPlane: number;
    let rPlane: number;
    if (l < r) {
      lPlane = Math.floor(trueDelinearized(l) - 0.5);
      rPlane = Math.ceil(trueDelinearized(r) - 0.5);
    } else {
      lPlane = Math.ceil(trueDelinearized(l) - 0.5);
      rPlane = Math.floor(trueDelinearized(r) - 0.5);
    }
    for (let i = 0; i < 8; i++) {
      if (Math.abs(rPlane - lPlane) <= 1) {
        break;
      }
      const mPlane = Math.floor((lPlane + rPlane) / 2.0);
      const mid = setCoordinate(left, CRITICAL_PLANES[mPlane] as number, right, axis);
      const midHue = hueOf(mid);
      if (areInCyclicOrder(leftHue, targetHue, midHue)) {
        right = mid;
        rPlane = mPlane;
      } else {
        left = mid;
        leftHue = midHue;
        lPlane = mPlane;
      }
    }
  }
  return [0, 1, 2].map((i) => ((left[i] as number) + (right[i] as number)) / 2);
}

function inverseChromaticAdaptation(adapted: number): number {
  const abs = Math.abs(adapted);
  const base = Math.max(0, (27.13 * abs) / (400.0 - abs));
  return signum(adapted) * base ** (1.0 / 0.42);
}

// biome-ignore lint/complexity/noExcessiveCognitiveComplexity: a line-for-line port of HctSolver._findResultByJ; splitting it would hide the correspondence
function findResultByJ(hueRadians: number, chroma: number, y: number): number {
  let j = Math.sqrt(y) * 11.0;
  const vc = STANDARD_VIEWING_CONDITIONS;
  const tInnerCoeff = 1 / (1.64 - 0.29 ** vc.backgroundYTowhitePointY) ** 0.73;
  const eHue = 0.25 * (Math.cos(hueRadians + 2.0) + 3.8);
  const p1 = eHue * (50000.0 / 13.0) * vc.nC * vc.ncb;
  const hSin = Math.sin(hueRadians);
  const hCos = Math.cos(hueRadians);
  for (let round = 0; round < 5; round++) {
    const jNormalized = j / 100.0;
    const alpha = chroma === 0.0 || j === 0.0 ? 0.0 : chroma / Math.sqrt(jNormalized);
    const t = (alpha * tInnerCoeff) ** (1.0 / 0.9);
    const ac = vc.aw * jNormalized ** (1.0 / vc.c / vc.z);
    const p2 = ac / vc.nbb;
    const gamma = (23.0 * (p2 + 0.305) * t) / (23.0 * p1 + 11 * t * hCos + 108.0 * t * hSin);
    const a = gamma * hCos;
    const b = gamma * hSin;
    const rA = (460.0 * p2 + 451.0 * a + 288.0 * b) / 1403.0;
    const gA = (460.0 * p2 - 891.0 * a - 261.0 * b) / 1403.0;
    const bA = (460.0 * p2 - 220.0 * a - 6300.0 * b) / 1403.0;
    const linrgb = matrixMultiply(
      [inverseChromaticAdaptation(rA), inverseChromaticAdaptation(gA), inverseChromaticAdaptation(bA)],
      LINRGB_FROM_SCALED_DISCOUNT,
    ) as [number, number, number];
    if (linrgb[0] < 0 || linrgb[1] < 0 || linrgb[2] < 0) {
      return 0;
    }
    const fnj = K_R * linrgb[0] + K_G * linrgb[1] + K_B * linrgb[2];
    if (fnj <= 0) {
      return 0;
    }
    if (round === 4 || Math.abs(fnj - y) < 0.002) {
      if (linrgb[0] > 100.01 || linrgb[1] > 100.01 || linrgb[2] > 100.01) {
        return 0;
      }
      return argbFromLinrgb(linrgb);
    }
    j = j - ((fnj - y) * j) / (2 * fnj);
  }
  return 0;
}

/** The sRGB colour closest to the hue, chroma and tone asked for (`HctSolver.solveToInt`). */
export function solveToInt(hueDegrees: number, chroma: number, lstar: number): number {
  if (chroma < 0.0001 || lstar < 0.0001 || lstar > 99.9999) {
    return argbFromLstar(lstar);
  }
  const hueRadians = (sanitizeDegreesDouble(hueDegrees) / 180) * Math.PI;
  const y = yFromLstar(lstar);
  const exact = findResultByJ(hueRadians, chroma, y);
  return exact !== 0 ? exact : argbFromLinrgb(bisectToLimit(y, hueRadians));
}

/** A colour by hue, chroma and tone (L*), the colour space Material's dynamic colour works in. */
export class Hct {
  private hueValue = 0;
  private chromaValue = 0;
  private toneValue = 0;
  private argb = 0;

  private constructor(argb: number) {
    this.set(argb);
  }

  static from(hue: number, chroma: number, tone: number): Hct {
    return new Hct(solveToInt(hue, chroma, tone));
  }

  static fromInt(argb: number): Hct {
    return new Hct(argb);
  }

  private set(argb: number): void {
    this.argb = argb;
    const cam = Cam16.fromInt(argb);
    this.hueValue = cam.hue;
    this.chromaValue = cam.chroma;
    this.toneValue = lstarFromArgb(argb);
  }

  toInt(): number {
    return this.argb;
  }

  get hue(): number {
    return this.hueValue;
  }

  set hue(value: number) {
    this.set(solveToInt(value, this.chromaValue, this.toneValue));
  }

  get chroma(): number {
    return this.chromaValue;
  }

  set chroma(value: number) {
    this.set(solveToInt(this.hueValue, value, this.toneValue));
  }

  get tone(): number {
    return this.toneValue;
  }

  set tone(value: number) {
    this.set(solveToInt(this.hueValue, this.chromaValue, value));
  }

  inViewingConditions(vc: ViewingConditions): Hct {
    const viewed = Cam16.fromInt(this.argb).xyzInViewingConditions(vc) as [number, number, number];
    const recast = Cam16.fromXyzInViewingConditions(viewed[0], viewed[1], viewed[2], makeViewingConditions());
    return Hct.from(recast.hue, recast.chroma, lstarFromY(viewed[1]));
  }
}
