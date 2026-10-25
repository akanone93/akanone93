// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// STU-121 / NFR-032: a property change reaches the canvas in 50 ms at p95. The change is one edited
// document going to the canvas worker, laid out with SkParagraph, applied to the scene and drawn:
// the time from handing the edited screens over to the frame that shows them.

import type { FrameStats, SceneOp, ScreenDocument, ScreenHeight, ScreenStyle } from "../src/protocol.ts";

/** What the measurement needs of the canvas host. */
export interface PropertyHost {
  layoutScreens(screens: readonly ScreenDocument[], style: ScreenStyle): Promise<readonly ScreenHeight[]>;
  patch(ops: readonly SceneOp[]): void;
  nextFrame(): Promise<FrameStats>;
}

export interface PropertyOptions {
  readonly warmup: number;
  readonly runs: number;
  /** A clock in milliseconds; `performance.now` in the browser. */
  readonly now: () => number;
}

export interface PropertySummary {
  readonly runs: number;
  readonly p50: number;
  readonly p95: number;
  readonly max: number;
  /** p95 of the part up to the worker's layout answer; the rest is the frame. */
  readonly layoutP95: number;
  /** p95 of the worker's own time drawing the frame (`FrameStats.ms`). */
  readonly frameP95: number;
  /** p95 of the frame's flush (`FrameStats.flushMs`); the rest of `frameP95` is drawing tiles. */
  readonly flushP95: number;
  readonly budgetMs: number;
}

/** The p95 budget of STU-121. */
export const PROPERTY_BUDGET_MS = 50;

function percentile(sorted: readonly number[], p: number): number {
  return sorted[Math.min(sorted.length - 1, Math.floor((sorted.length - 1) * p))] ?? 0;
}

const round = (v: number): number => Math.round(v * 100) / 100;

/**
 * Applies `edit(i)` to the screens `warmup + runs` times and times each from the hand-over to the first frame
 * that follows the worker's answer; an empty patch forces that frame, so the time is an upper bound.
 */
export async function measurePropertyChange(
  host: PropertyHost,
  initial: readonly ScreenDocument[],
  style: ScreenStyle,
  edit: (screens: readonly ScreenDocument[], i: number) => readonly ScreenDocument[],
  options: PropertyOptions,
): Promise<PropertySummary> {
  await host.layoutScreens(initial, style);
  let screens = initial;
  const ms: number[] = [];
  const layout: number[] = [];
  const frame: number[] = [];
  const flush: number[] = [];
  for (let i = 0; i < options.warmup + options.runs; i++) {
    screens = edit(screens, i);
    const start = options.now();
    await host.layoutScreens(screens, style);
    const laidOut = options.now();
    host.patch([]);
    const stats = await host.nextFrame();
    if (i >= options.warmup) {
      ms.push(options.now() - start);
      layout.push(laidOut - start);
      frame.push(stats.ms);
      flush.push(stats.flushMs);
    }
  }
  ms.sort((a, b) => a - b);
  layout.sort((a, b) => a - b);
  for (const list of [frame, flush]) {
    list.sort((a, b) => a - b);
  }
  return {
    runs: ms.length,
    p50: round(percentile(ms, 0.5)),
    p95: round(percentile(ms, 0.95)),
    max: round(ms.at(-1) ?? 0),
    layoutP95: round(percentile(layout, 0.95)),
    frameP95: round(percentile(frame, 0.95)),
    flushP95: round(percentile(flush, 0.95)),
    budgetMs: PROPERTY_BUDGET_MS,
  };
}
