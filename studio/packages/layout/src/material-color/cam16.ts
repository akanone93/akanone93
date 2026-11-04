// SPDX-FileCopyrightText: 2021 Google LLC
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0 AND AGPL-3.0-only

// Ported from material_color_utilities 0.13.0 (lib/hct/viewing_conditions.dart, lib/hct/cam16.dart).

import { argbFromXyz, lerp, signum, whitePointD65, xyzFromArgb, yFromLstar } from "./color-utils.ts";

/** The environment a colour is seen in, which CAM16 needs to model how it appears. */
export interface ViewingConditions {
  readonly whitePoint: readonly number[];
  readonly adaptingLuminance: number;
  readonly backgroundLstar: number;
  readonly surround: number;
  readonly discountingIlluminant: boolean;
  readonly backgroundYTowhitePointY: number;
  readonly aw: number;
  readonly nbb: number;
  readonly ncb: number;
  readonly c: number;
  readonly nC: number;
  readonly rgbD: readonly number[];
  readonly fl: number;
  readonly fLRoot: number;
  readonly z: number;
}

/** `ViewingConditions.make` with its defaults: sRGB-like conditions, a mid-grey background, average surround. */
export function makeViewingConditions(
  options: {
    whitePoint?: readonly number[];
    adaptingLuminance?: number;
    backgroundLstar?: number;
    surround?: number;
    discountingIlluminant?: boolean;
  } = {},
): ViewingConditions {
  const whitePoint = options.whitePoint ?? whitePointD65();
  const given = options.adaptingLuminance ?? -1.0;
  const adaptingLuminance = given > 0.0 ? given : ((200.0 / Math.PI) * yFromLstar(50.0)) / 100.0;
  const backgroundLstar = Math.max(0.1, options.backgroundLstar ?? 50.0);
  const surround = options.surround ?? 2.0;
  const discountingIlluminant = options.discountingIlluminant ?? false;
  const [x0, x1, x2] = whitePoint as [number, number, number];
  const rW = x0 * 0.401288 + x1 * 0.650173 + x2 * -0.051461;
  const gW = x0 * -0.250268 + x1 * 1.204414 + x2 * 0.045854;
  const bW = x0 * -0.002079 + x1 * 0.048952 + x2 * 0.953127;
  const f = 0.8 + surround / 10.0;
  const c = f >= 0.9 ? lerp(0.59, 0.69, (f - 0.9) * 10.0) : lerp(0.525, 0.59, (f - 0.8) * 10.0);
  let d = discountingIlluminant ? 1.0 : f * (1.0 - (1.0 / 3.6) * Math.exp((-adaptingLuminance - 42.0) / 92.0));
  d = d > 1.0 ? 1.0 : d < 0.0 ? 0.0 : d;
  const nc = f;
  const rgbD = [d * (100.0 / rW) + 1.0 - d, d * (100.0 / gW) + 1.0 - d, d * (100.0 / bW) + 1.0 - d] as const;
  const k = 1.0 / (5.0 * adaptingLuminance + 1.0);
  const k4 = k * k * k * k;
  const k4F = 1.0 - k4;
  const fl = k4 * adaptingLuminance + 0.1 * k4F * k4F * (5.0 * adaptingLuminance) ** (1.0 / 3.0);
  const n = yFromLstar(backgroundLstar) / x1;
  const z = 1.48 + Math.sqrt(n);
  const nbb = 0.725 / n ** 0.2;
  const ncb = nbb;
  const factors = [
    ((fl * rgbD[0] * rW) / 100.0) ** 0.42,
    ((fl * rgbD[1] * gW) / 100.0) ** 0.42,
    ((fl * rgbD[2] * bW) / 100.0) ** 0.42,
  ] as const;
  const rgbA = factors.map((v) => (400.0 * v) / (v + 27.13)) as [number, number, number];
  const aw = ((40.0 * rgbA[0] + 20.0 * rgbA[1] + rgbA[2]) / 20.0) * nbb;
  return {
    whitePoint,
    adaptingLuminance,
    backgroundLstar,
    surround,
    discountingIlluminant,
    backgroundYTowhitePointY: n,
    aw,
    nbb,
    ncb,
    c,
    nC: nc,
    rgbD,
    fl,
    fLRoot: fl ** 0.25,
    z,
  };
}

/** The standard viewing conditions (`ViewingConditions.standard`, `ViewingConditions.sRgb`). */
export const STANDARD_VIEWING_CONDITIONS: ViewingConditions = makeViewingConditions();

/** A colour in CAM16: hue, chroma, lightness J and the CAM16-UCS coordinates. */
export class Cam16 {
  constructor(
    readonly hue: number,
    readonly chroma: number,
    readonly j: number,
    readonly q: number,
    readonly m: number,
    readonly s: number,
    readonly jstar: number,
    readonly astar: number,
    readonly bstar: number,
  ) {}

  distance(other: Cam16): number {
    const dJ = this.jstar - other.jstar;
    const dA = this.astar - other.astar;
    const dB = this.bstar - other.bstar;
    return 1.41 * Math.sqrt(dJ * dJ + dA * dA + dB * dB) ** 0.63;
  }

  static fromInt(argb: number): Cam16 {
    return Cam16.fromIntInViewingConditions(argb, STANDARD_VIEWING_CONDITIONS);
  }

  static fromIntInViewingConditions(argb: number, vc: ViewingConditions): Cam16 {
    const [x, y, z] = xyzFromArgb(argb) as [number, number, number];
    return Cam16.fromXyzInViewingConditions(x, y, z, vc);
  }

  static fromXyzInViewingConditions(x: number, y: number, z: number, vc: ViewingConditions): Cam16 {
    const rC = 0.401288 * x + 0.650173 * y - 0.051461 * z;
    const gC = -0.250268 * x + 1.204414 * y + 0.045854 * z;
    const bC = -0.002079 * x + 0.048952 * y + 0.953127 * z;
    const rD = (vc.rgbD[0] as number) * rC;
    const gD = (vc.rgbD[1] as number) * gC;
    const bD = (vc.rgbD[2] as number) * bC;
    const rAF = ((vc.fl * Math.abs(rD)) / 100.0) ** 0.42;
    const gAF = ((vc.fl * Math.abs(gD)) / 100.0) ** 0.42;
    const bAF = ((vc.fl * Math.abs(bD)) / 100.0) ** 0.42;
    const rA = (signum(rD) * 400.0 * rAF) / (rAF + 27.13);
    const gA = (signum(gD) * 400.0 * gAF) / (gAF + 27.13);
    const bA = (signum(bD) * 400.0 * bAF) / (bAF + 27.13);
    const a = (11.0 * rA + -12.0 * gA + bA) / 11.0;
    const b = (rA + gA - 2.0 * bA) / 9.0;
    const u = (20.0 * rA + 20.0 * gA + 21.0 * bA) / 20.0;
    const p2 = (40.0 * rA + 20.0 * gA + bA) / 20.0;
    const atanDegrees = (Math.atan2(b, a) * 180.0) / Math.PI;
    const hue = atanDegrees < 0 ? atanDegrees + 360.0 : atanDegrees >= 360 ? atanDegrees - 360 : atanDegrees;
    const hueRadians = (hue * Math.PI) / 180.0;
    const ac = p2 * vc.nbb;
    const J = 100.0 * (ac / vc.aw) ** (vc.c * vc.z);
    const Q = (4.0 / vc.c) * Math.sqrt(J / 100.0) * (vc.aw + 4.0) * vc.fLRoot;
    const huePrime = hue < 20.14 ? hue + 360 : hue;
    const eHue = (1.0 / 4.0) * (Math.cos((huePrime * Math.PI) / 180.0 + 2.0) + 3.8);
    const p1 = (50000.0 / 13.0) * eHue * vc.nC * vc.ncb;
    const t = (p1 * Math.sqrt(a * a + b * b)) / (u + 0.305);
    const alpha = t ** 0.9 * (1.64 - 0.29 ** vc.backgroundYTowhitePointY) ** 0.73;
    const C = alpha * Math.sqrt(J / 100.0);
    const M = C * vc.fLRoot;
    const s = 50.0 * Math.sqrt((alpha * vc.c) / (vc.aw + 4.0));
    const jstar = ((1.0 + 100.0 * 0.007) * J) / (1.0 + 0.007 * J);
    const mstar = Math.log(1.0 + 0.0228 * M) / 0.0228;
    return new Cam16(hue, C, J, Q, M, s, jstar, mstar * Math.cos(hueRadians), mstar * Math.sin(hueRadians));
  }

  static fromJch(j: number, c: number, h: number): Cam16 {
    return Cam16.fromJchInViewingConditions(j, c, h, STANDARD_VIEWING_CONDITIONS);
  }

  static fromJchInViewingConditions(J: number, C: number, h: number, vc: ViewingConditions): Cam16 {
    const Q = (4.0 / vc.c) * Math.sqrt(J / 100.0) * (vc.aw + 4.0) * vc.fLRoot;
    const M = C * vc.fLRoot;
    const alpha = C / Math.sqrt(J / 100.0);
    const s = 50.0 * Math.sqrt((alpha * vc.c) / (vc.aw + 4.0));
    const hueRadians = (h * Math.PI) / 180.0;
    const jstar = ((1.0 + 100.0 * 0.007) * J) / (1.0 + 0.007 * J);
    const mstar = (1.0 / 0.0228) * Math.log(1.0 + 0.0228 * M);
    return new Cam16(h, C, J, Q, M, s, jstar, mstar * Math.cos(hueRadians), mstar * Math.sin(hueRadians));
  }

  static fromUcs(jstar: number, astar: number, bstar: number): Cam16 {
    const m = Math.sqrt(astar * astar + bstar * bstar);
    const M = (Math.exp(m * 0.0228) - 1.0) / 0.0228;
    const c = M / STANDARD_VIEWING_CONDITIONS.fLRoot;
    let h = Math.atan2(bstar, astar) * (180.0 / Math.PI);
    if (h < 0.0) {
      h += 360.0;
    }
    const j = jstar / (1 - (jstar - 100) * 0.007);
    return Cam16.fromJchInViewingConditions(j, c, h, STANDARD_VIEWING_CONDITIONS);
  }

  toInt(): number {
    return this.viewed(STANDARD_VIEWING_CONDITIONS);
  }

  viewed(vc: ViewingConditions): number {
    const [x, y, z] = this.xyzInViewingConditions(vc) as [number, number, number];
    return argbFromXyz(x, y, z);
  }

  xyzInViewingConditions(vc: ViewingConditions): number[] {
    const alpha = this.chroma === 0.0 || this.j === 0.0 ? 0.0 : this.chroma / Math.sqrt(this.j / 100.0);
    const t = (alpha / (1.64 - 0.29 ** vc.backgroundYTowhitePointY) ** 0.73) ** (1.0 / 0.9);
    const hRad = (this.hue * Math.PI) / 180.0;
    const eHue = 0.25 * (Math.cos(hRad + 2.0) + 3.8);
    const ac = vc.aw * (this.j / 100.0) ** (1.0 / vc.c / vc.z);
    const p1 = eHue * (50000.0 / 13.0) * vc.nC * vc.ncb;
    const p2 = ac / vc.nbb;
    const hSin = Math.sin(hRad);
    const hCos = Math.cos(hRad);
    const gamma = (23.0 * (p2 + 0.305) * t) / (23.0 * p1 + 11 * t * hCos + 108.0 * t * hSin);
    const a = gamma * hCos;
    const b = gamma * hSin;
    const rA = (460.0 * p2 + 451.0 * a + 288.0 * b) / 1403.0;
    const gA = (460.0 * p2 - 891.0 * a - 261.0 * b) / 1403.0;
    const bA = (460.0 * p2 - 220.0 * a - 6300.0 * b) / 1403.0;
    const back = (v: number) =>
      signum(v) * (100.0 / vc.fl) * Math.max(0, (27.13 * Math.abs(v)) / (400.0 - Math.abs(v))) ** (1.0 / 0.42);
    const rF = back(rA) / (vc.rgbD[0] as number);
    const gF = back(gA) / (vc.rgbD[1] as number);
    const bF = back(bA) / (vc.rgbD[2] as number);
    return [
      1.86206786 * rF - 1.01125463 * gF + 0.14918677 * bF,
      0.38752654 * rF + 0.62144744 * gF - 0.00897398 * bF,
      -0.0158415 * rF - 0.03412294 * gF + 1.04996444 * bF,
    ];
  }
}
