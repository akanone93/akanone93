// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Hover intent for the canvases' arrows (#206, STU-044). A pointer sweeping across a canvas crosses many arrows; if
 * each one lit up the moment it was touched, the canvas would flicker between them. An arrow is therefore shown as
 * hovered only after the pointer has rested on it for `enterMs`, and stays shown for `leaveMs` after the pointer
 * leaves, so a short gap between a lane and its title does not drop it. A click or keyboard focus (`now`) acts at once.
 */

/** How long the pointer rests on an arrow before it lights up. */
export const HOVER_ENTER_MS = 160;
/** How long a lit arrow stays lit after the pointer has left it. */
export const HOVER_LEAVE_MS = 120;

/** The timers the intent waits with; the browser's by default, a fake clock in tests. */
export interface IntentTimers {
  readonly set: (run: () => void, ms: number) => number;
  readonly clear: (handle: number) => void;
}

const browserTimers: IntentTimers = {
  set: (run, ms) => setTimeout(run, ms) as unknown as number,
  clear: (handle) => clearTimeout(handle),
};

/** Applies a hovered value (an arrow's bundle id, or null for none) after the pointer has settled on it. */
export class HoverIntent<T> {
  private shown: T | null = null;
  private pending: { readonly value: T | null; readonly handle: number } | null = null;

  constructor(
    private readonly apply: (value: T | null) => void,
    private readonly timers: IntentTimers = browserTimers,
    private readonly enterMs = HOVER_ENTER_MS,
    private readonly leaveMs = HOVER_LEAVE_MS,
  ) {}

  /** The pointer is over `value` (null: over no arrow); applied once it has stayed there long enough. */
  hover(value: T | null): void {
    if (this.pending !== null && this.pending.value === value) {
      return;
    }
    this.cancel();
    if (value === this.shown) {
      return;
    }
    const handle = this.timers.set(
      () => {
        this.pending = null;
        this.show(value);
      },
      value === null ? this.leaveMs : this.enterMs,
    );
    this.pending = { value, handle };
  }

  /** A click or keyboard focus: `value` is applied at once, and any waiting hover is dropped. */
  now(value: T | null): void {
    this.cancel();
    this.show(value);
  }

  /** Drops any waiting hover; what is shown stays. */
  dispose(): void {
    this.cancel();
  }

  private show(value: T | null): void {
    if (value !== this.shown) {
      this.shown = value;
      this.apply(value);
    }
  }

  private cancel(): void {
    if (this.pending !== null) {
      this.timers.clear(this.pending.handle);
      this.pending = null;
    }
  }
}
