// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Guides } from "@plux/canvas";
import { useSyncExternalStore } from "react";
import type { Box } from "./geometry.ts";

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
  /** The box of a node being moved or resized, in world units, or null. */
  readonly ghost: Box | null;
  /** Changes whenever the view pans or zooms, so the overlay follows it. */
  readonly view: number;
}

export const idleOverlay: OverlayState = { hover: null, marquee: null, guides: null, drop: null, ghost: null, view: 0 };

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
