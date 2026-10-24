// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Level of detail by zoom (ADR-0013 M-12): below the thresholds links are
 * drawn as straight segments, text as bars, and frame shadows are dropped.
 * The levels are constants, so choosing one allocates nothing.
 */

/** Links become straight segments below this zoom. */
export const STRAIGHT_LINKS_BELOW = 0.5;
/** Text becomes bars below this zoom. */
export const TEXT_BARS_BELOW = 0.5;
/** Frame shadows are dropped below this zoom. */
export const SHADOWS_BELOW = 0.25;
/** Device-pixel ratio factor while the user interacts; refined to 1 when idle. */
export const INTERACTION_RESOLUTION = 0.5;

/** What to draw at a zoom level. */
export interface Lod {
  readonly straightLinks: boolean;
  readonly textBars: boolean;
  readonly shadows: boolean;
}

const full: Lod = Object.freeze({ straightLinks: false, textBars: false, shadows: true });
const reduced: Lod = Object.freeze({ straightLinks: true, textBars: true, shadows: true });
const minimal: Lod = Object.freeze({ straightLinks: true, textBars: true, shadows: false });

/** The level of detail for `zoom`. */
export function lodForZoom(zoom: number): Lod {
  if (zoom < SHADOWS_BELOW) {
    return minimal;
  }
  return zoom < Math.min(STRAIGHT_LINKS_BELOW, TEXT_BARS_BELOW) ? reduced : full;
}

/** The resolution factor: reduced while interacting, full when idle. */
export function resolutionFor(interacting: boolean): number {
  return interacting ? INTERACTION_RESOLUTION : 1;
}
