// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { type ArrowCanvas, EXPAND_MS, type SceneOp } from "@plux/canvas";

/** What the motion needs from the page: the clock, frames and the reduced-motion setting; real ones by default. */
export interface MotionHost {
  readonly now: () => number;
  readonly request: (step: () => void) => number;
  readonly cancel: (frame: number) => void;
  readonly reduced: () => boolean;
}

const browserHost: MotionHost = {
  now: () => (typeof performance === "undefined" ? 0 : performance.now()),
  request: (step) => requestAnimationFrame(step),
  cancel: (frame) => cancelAnimationFrame(frame),
  reduced: () =>
    typeof window !== "undefined" && window.matchMedia?.("(prefers-reduced-motion: reduce)").matches === true,
};

/**
 * The live side of a canvas's arrows: a bundle expands into its lanes on hover or click and collapses again
 * (STU-044, 150–200 ms, at once under reduced motion), and the arrows of boxes shown at a dragged place re-route
 * before the drop (STU-045). Every change is sent to the worker as scene patches; nothing is drawn here.
 */
export class ArrowMotion {
  private focus: string | null = null;
  private frame = 0;
  private clock = 0;

  constructor(
    private readonly arrows: ArrowCanvas,
    private readonly patch: (ops: SceneOp[]) => void,
    private readonly host: MotionHost = browserHost,
  ) {}

  /** A clock that never runs backwards, also after a reduced-motion jump to the end of an animation. */
  now(): number {
    this.clock = Math.max(this.clock, this.host.now());
    return this.clock;
  }

  /** Expands bundle `id`, or collapses every bundle with null; animated in the worker frame by frame. */
  expand(id: string | null): void {
    if (this.focus === id) {
      return;
    }
    this.focus = id;
    const t = this.now();
    const ops = this.arrows.setFocus(id, t);
    if (this.host.reduced()) {
      this.clock = t + EXPAND_MS;
      ops.push(...this.arrows.frame(this.clock));
    }
    this.send(ops);
    this.host.cancel(this.frame);
    const step = () => {
      const at = this.now();
      this.send(this.arrows.frame(at));
      if (this.arrows.animating(at)) {
        this.frame = this.host.request(step);
      }
    };
    // After a reduced-motion jump the clock is past the animation's end, so no frame loop starts.
    if (this.arrows.animating(this.clock)) {
      this.frame = this.host.request(step);
    }
  }

  /** Re-routes the arrows of the boxes at their shown places, by box id. */
  follow(moved: ReadonlyMap<string, { readonly x: number; readonly y: number }>): void {
    const t = this.now();
    this.send([...moved].flatMap(([id, at]) => this.arrows.moveBox(id, at.x, at.y, t)));
  }

  /** Redraws the arrows for the zoom (level of detail, STU-048). */
  zoom(zoom: number): void {
    this.send(this.arrows.setZoom(zoom, this.now()));
  }

  /** Stops a running animation. */
  dispose(): void {
    this.host.cancel(this.frame);
  }

  private send(ops: SceneOp[]): void {
    if (ops.length > 0) {
      this.patch(ops);
    }
  }
}
