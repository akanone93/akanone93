// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/material/shadows.dart
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/** A box shadow: an offset, a blur radius and a spread, in logical pixels; the colour is its alpha over black. */
export interface ElevationShadow {
  readonly dx: number;
  readonly dy: number;
  readonly blur: number;
  readonly spread: number;
  /** The shadow colour's alpha, 0–255, over black (key umbra, key penumbra, ambient). */
  readonly alpha: number;
}

const UMBRA = 0x33;
const PENUMBRA = 0x24;
const AMBIENT = 0x1f;

const s = (dy: number, blur: number, spread: number, alpha: number): ElevationShadow => ({
  dx: 0,
  dy,
  blur,
  spread,
  alpha,
});

/** `kElevationToShadow`: Material's shadows for the elevations it defines, each an umbra, a penumbra and an ambient. */
export const kElevationToShadow: ReadonlyMap<number, readonly ElevationShadow[]> = new Map([
  [0, []],
  [1, [s(2, 1, -1, UMBRA), s(1, 1, 0, PENUMBRA), s(1, 3, 0, AMBIENT)]],
  [2, [s(3, 1, -2, UMBRA), s(2, 2, 0, PENUMBRA), s(1, 5, 0, AMBIENT)]],
  [3, [s(3, 3, -2, UMBRA), s(3, 4, 0, PENUMBRA), s(1, 8, 0, AMBIENT)]],
  [4, [s(2, 4, -1, UMBRA), s(4, 5, 0, PENUMBRA), s(1, 10, 0, AMBIENT)]],
  [6, [s(3, 5, -1, UMBRA), s(6, 10, 0, PENUMBRA), s(1, 18, 0, AMBIENT)]],
  [8, [s(5, 5, -3, UMBRA), s(8, 10, 1, PENUMBRA), s(3, 14, 2, AMBIENT)]],
  [9, [s(5, 6, -3, UMBRA), s(9, 12, 1, PENUMBRA), s(3, 16, 2, AMBIENT)]],
  [12, [s(7, 8, -4, UMBRA), s(12, 17, 2, PENUMBRA), s(5, 22, 4, AMBIENT)]],
  [16, [s(8, 10, -5, UMBRA), s(16, 24, 2, PENUMBRA), s(6, 30, 5, AMBIENT)]],
  [24, [s(11, 15, -7, UMBRA), s(24, 38, 3, PENUMBRA), s(9, 46, 8, AMBIENT)]],
]);

/** The shadows of `elevation`: those of the nearest elevation Material defines at or below it. */
export function shadowsForElevation(elevation: number): readonly ElevationShadow[] {
  let best: readonly ElevationShadow[] = [];
  for (const [e, list] of kElevationToShadow) {
    if (e <= elevation) {
      best = list;
    }
  }
  return best;
}
