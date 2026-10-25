// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Progressive base re-rendering (STU-045, STU-120, ADR-0013 M-12).
 *
 * On software GL the browser's single GPU thread both runs the canvas and
 * composites the page, so one large re-render of the base stalls every
 * animation for as long as it takes. The base is therefore re-rendered as a
 * queue of tile-sized jobs, each flushed on its own, run in slices of a fixed
 * budget per animation frame: what the view shows first, then the margin the
 * pan heads into, then the rest. A new viewport replans the queue; the old
 * base stays on screen, realigned, until each tile is replaced.
 */

import type { Rect } from "./geometry.ts";

/** Time per animation frame for base jobs; a tile render that overruns it blits in the next slice, in ms. */
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
export function orderJobs<T>(items: readonly T[], rectOf: (item: T) => Rect, view: Rect, direction: Direction): T[] {
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

/** The environment of a `BaseJobQueue`. */
export interface JobQueueDeps {
  now(): number;
  /** Schedules the next slice, once per animation frame. */
  schedule(callback: () => void): void;
}

/**
 * Runs one job that may be preempted: returns false when nothing is left to
 * do, true otherwise; it must stop by `deadline` where it can.
 */
export type Job = (deadline: number) => boolean;

/** Called after each slice with its duration in ms and whether work remains. */
export type SliceDone = (ms: number, more: boolean) => void;

/**
 * Runs a job queue in slices of at most `budgetMs` per animation frame (M-12).
 * Each slice runs at least one job, then as many as fit in the budget; a job
 * whose start would pass the budget waits for the next frame. `start` (re)arms
 * the queue with a new job function — the caller replans its queue around the
 * new viewport first — and `cancel` drops it; at most one slice is scheduled at
 * a time, and a slice scheduled for a cancelled or replaced run does nothing.
 */
export class BaseJobQueue {
  private job: Job | null = null;
  private done: SliceDone | null = null;
  private scheduled = false;
  private generation = 0;

  constructor(
    private readonly deps: JobQueueDeps,
    private readonly budgetMs: number = JOB_BUDGET_MS,
  ) {}

  /** Whether a run is armed. */
  get active(): boolean {
    return this.job !== null;
  }

  /** Arms the queue with `job`, replacing any earlier run, and schedules a slice. */
  start(job: Job, done: SliceDone): void {
    this.generation++;
    this.job = job;
    this.done = done;
    this.request();
  }

  /** Drops the current run; a slice already scheduled runs nothing. */
  cancel(): void {
    this.generation++;
    this.job = null;
    this.done = null;
  }

  private request(): void {
    if (this.scheduled) {
      return;
    }
    this.scheduled = true;
    this.deps.schedule(() => this.slice());
  }

  private slice(): void {
    this.scheduled = false;
    const job = this.job;
    const done = this.done;
    if (job === null || done === null) {
      return;
    }
    const generation = this.generation;
    const start = this.deps.now();
    const deadline = start + this.budgetMs;
    let more = true;
    do {
      more = job(deadline);
    } while (more && generation === this.generation && this.deps.now() < deadline);
    if (generation !== this.generation) {
      // The job replaced or cancelled the run; the new run has scheduled its own slice.
      return;
    }
    if (!more) {
      this.job = null;
      this.done = null;
    }
    done(this.deps.now() - start, more);
    if (more && generation === this.generation) {
      this.request();
    }
  }
}
