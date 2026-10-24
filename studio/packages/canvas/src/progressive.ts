// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Progressive base re-rendering (STU-045, STU-120, ADR-0013 M-12).
 *
 * On software GL the browser's single GPU thread both runs the canvas and
 * composites the page, so one large re-render of the base stalls every
 * animation for as long as it takes. The base is therefore re-rendered as a
 * queue of tile-sized jobs, each flushed on its own and followed by a yield to
 * the event loop: what the view shows first, then the margin the pan heads
 * into, then the rest. A new gesture cancels the queue; the old base stays on
 * screen until each tile is replaced.
 */

import type { Rect } from "./geometry.ts";

/** Time a job may take before it yields; a tile render that overruns it blits in the next job, in ms. */
export const JOB_BUDGET_MS = 8;

/** A direction of travel, in world units; zero when unknown. */
export interface Direction {
  readonly x: number;
  readonly y: number;
}

/** The priority band of a job: 0 in the view, 1 in the margin ahead of the pan, 2 the rest. */
export function jobBand(rect: Rect, view: Rect, direction: Direction): number {
  if (rect.l < view.r && rect.r > view.l && rect.t < view.b && rect.b > view.t) {
    return 0;
  }
  const dx = (rect.l + rect.r) / 2 - (view.l + view.r) / 2;
  const dy = (rect.t + rect.b) / 2 - (view.t + view.b) / 2;
  return dx * direction.x + dy * direction.y > 0 ? 1 : 2;
}

/**
 * Orders `items` for rendering by band (see `jobBand`), and within a band by
 * distance from the centre of `view`; the order is stable for equal keys.
 */
export function orderJobs<T>(
  items: readonly T[],
  rectOf: (item: T) => Rect,
  view: Rect,
  direction: Direction,
): T[] {
  const cx = (view.l + view.r) / 2;
  const cy = (view.t + view.b) / 2;
  const keyed = items.map((item, index) => {
    const rect = rectOf(item);
    const dx = (rect.l + rect.r) / 2 - cx;
    const dy = (rect.t + rect.b) / 2 - cy;
    return { item, index, band: jobBand(rect, view, direction), distance: dx * dx + dy * dy };
  });
  keyed.sort((a, b) => a.band - b.band || a.distance - b.distance || a.index - b.index);
  return keyed.map((entry) => entry.item);
}

/** The parts of a worker scope used to yield between jobs. */
export interface JobScope {
  setTimeout(callback: () => void, ms: number): unknown;
  readonly scheduler?: unknown;
}

interface PostTask {
  postTask(callback: () => void, options: { priority: "background" }): Promise<unknown>;
}

function hasPostTask(scheduler: unknown): scheduler is PostTask {
  return (
    typeof scheduler === "object" &&
    scheduler !== null &&
    typeof (scheduler as { postTask?: unknown }).postTask === "function"
  );
}

/**
 * Schedules the next job as a new task: `scheduler.postTask` at background
 * priority where the scope has it (a feature test, as workers differ by
 * browser), otherwise `setTimeout(0)`.
 */
export function jobScheduler(scope: JobScope): (callback: () => void) => void {
  const scheduler = scope.scheduler;
  if (hasPostTask(scheduler)) {
    return (callback) => {
      scheduler.postTask(callback, { priority: "background" }).catch(() => undefined);
    };
  }
  return (callback) => {
    scope.setTimeout(callback, 0);
  };
}
