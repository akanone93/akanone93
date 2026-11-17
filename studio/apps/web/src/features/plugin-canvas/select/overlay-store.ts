// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Guides } from "@plux/canvas";
import { useSyncExternalStore } from "react";
import type { Refusal } from "./drop-rules.ts";
import type { Box } from "./geometry.ts";

/** A drop the rules refuse (PC-20): the widget dragged, the widget the refusal is about (null off the screens), why. */
export interface RefusedDrop {
  readonly reason: Refusal;
  readonly type: string;
  readonly at: string | null;
}

/**
 * What the canvas shows over the screens while the pointer works (STU-067): the node under the pointer, the marquee,
 * snap guides and distance labels, the insertion indicator, the container a drop lands in and the box being dragged
 * or resized. The page's DOM overlay draws it; the worker never re-renders for it.
 */
export interface OverlayState {
  /** The node a click would select, or null. */
  readonly hover: string | null;
  /** The marquee in canvas pixels, or null. */
  readonly marquee: Box | null;
  /** Snap guides, distance labels and the insertion indicator, in world units, or null. */
  readonly guides: Guides | null;
  /** The container a drop would land in, or null. */
  readonly drop: string | null;
  /** The slot of `drop` it would land in, named on its tag; null for its children. */
  readonly dropSlot: string | null;
  /** The drop the rules refuse where the pointer is, while dragging, or null. */
  readonly refused: RefusedDrop | null;
  /** A refused drop just let go, at world point (x, y): its reason shows there for a moment. */
  readonly notice: (RefusedDrop & { readonly x: number; readonly y: number }) | null;
  /** The box of a node being moved or resized, in world units, or null. */
  readonly ghost: Box | null;
  /** Changes whenever the view pans or zooms, so the overlay follows it. */
  readonly view: number;
}

export const idleOverlay: OverlayState = {
  hover: null,
  marquee: null,
  guides: null,
  drop: null,
  dropSlot: null,
  refused: null,
  notice: null,
  ghost: null,
  view: 0,
};

/** The overlay change that shows a drop plan while dragging: the target, its slot and indicator, or the refusal. */
export function dropMarks(
  plan:
    | {
        readonly ok: true;
        readonly target: { readonly parent: string; readonly slot?: string; readonly indicator: Box };
      }
    | (Omit<RefusedDrop, "type"> & { readonly ok: false }),
  type: string,
): Pick<OverlayState, "drop" | "dropSlot" | "refused" | "guides"> {
  return plan.ok
    ? {
        drop: plan.target.parent,
        dropSlot: plan.target.slot ?? null,
        refused: null,
        guides: { lines: [], labels: [], indicator: plan.target.indicator },
      }
    : { drop: null, dropSlot: null, refused: { reason: plan.reason, type, at: plan.at }, guides: null };
}

/** The overlay change that clears every drop mark. */
export const noDrop: Pick<OverlayState, "drop" | "dropSlot" | "refused" | "guides"> = {
  drop: null,
  dropSlot: null,
  refused: null,
  guides: null,
};

export interface OverlayStore {
  get(): OverlayState;
  set(change: Partial<Omit<OverlayState, "view">>): void;
  /** The view moved: the overlay redraws at the next render. */
  viewChanged(): void;
  subscribe(listener: () => void): () => void;
}

const same = (a: OverlayState, change: Partial<OverlayState>): boolean =>
  (Object.keys(change) as (keyof OverlayState)[]).every((k) => a[k] === change[k]);

/** A small external store: pointer moves update it without re-rendering the page, only the overlay. */
export function createOverlayStore(): OverlayStore {
  let state = idleOverlay;
  const listeners = new Set<() => void>();
  const emit = () => {
    for (const l of [...listeners]) {
      l();
    }
  };
  return {
    get: () => state,
    set(change) {
      if (!same(state, change)) {
        state = { ...state, ...change };
        emit();
      }
    },
    viewChanged() {
      state = { ...state, view: state.view + 1 };
      emit();
    },
    subscribe(listener) {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
  };
}

/** The overlay state, re-rendering the caller on each change. */
export function useOverlayState(store: OverlayStore): OverlayState {
  return useSyncExternalStore(store.subscribe, store.get, store.get);
}
