// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { type ArrowCanvas, EXPAND_MS, HoverIntent, type IntentTimers, type SceneOp } from "@plux/canvas";

/** What the motion needs from the page: the clock, frames and the reduced-motion setting; real ones by default. */
export interface MotionHost {
  readonly now: () => number;
  readonly request: (step: () => void) => number;
  readonly cancel: (frame: number) => void;
  readonly reduced: () => boolean;
  /** The timers the hover intents wait with (#206, #240); the browser's when absent. */
  readonly timers?: IntentTimers;
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
  /** The pointer's hover waits for intent (#206): a sweep across arrows lights none of them. */
  private readonly intent: HoverIntent<string>;
  /** The boxes whose arrows are lit (#240): the screen being worked on, and the box the pointer rests on. */
  private selected: readonly string[] = [];
  private hovered: string | null = null;
  private readonly boxIntent: HoverIntent<string>;

  constructor(
    private readonly arrows: ArrowCanvas,
    private readonly patch: (ops: SceneOp[]) => void,
    private readonly host: MotionHost = browserHost,
  ) {
    this.intent = new HoverIntent<string>((id) => this.apply(id), host.timers);
    this.boxIntent = new HoverIntent<string>((id) => {
      this.hovered = id;
      this.relight();
    }, host.timers);
  }

  /** A clock that never runs backwards, also after a reduced-motion jump to the end of an animation. */
  now(): number {
    this.clock = Math.max(this.clock, this.host.now());
    return this.clock;
  }

  /** Expands bundle `id` at once (a click or keyboard focus), or collapses every bundle with null. */
  expand(id: string | null): void {
    this.intent.now(id);
  }

  /** The pointer rests over bundle `id` (null: over no arrow); it expands once the pointer has settled (#206). */
  hover(id: string | null): void {
    this.intent.hover(id);
  }

  /** Expands bundle `id`, or collapses every bundle with null; animated in the worker frame by frame. */
  private apply(id: string | null): void {
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

  /** Highlights the arrows of `boxes` — the screen being worked on — and dims the rest (#206); empty lights none. */
  highlight(boxes: readonly string[]): void {
    this.selected = boxes;
    this.relight();
  }

  /**
   * The pointer rests over box `id` — a screen or a target box — or over none (null): its arrows light up with the
   * highlighted ones once it has settled there, as on the app canvas (#240).
   */
  hoverBox(id: string | null): void {
    this.boxIntent.hover(id);
  }

  private relight(): void {
    const lit = this.hovered === null ? this.selected : [...this.selected, this.hovered];
    this.send(this.arrows.setHighlight(lit, this.now()));
  }

  /** Redraws the arrows for the zoom (level of detail, STU-048). */
  zoom(zoom: number): void {
    this.send(this.arrows.setZoom(zoom, this.now()));
  }

  /** Stops a running animation and drops a waiting hover. */
  dispose(): void {
    this.intent.dispose();
    this.boxIntent.dispose();
    this.host.cancel(this.frame);
  }

  private send(ops: SceneOp[]): void {
    if (ops.length > 0) {
      this.patch(ops);
    }
  }
}
