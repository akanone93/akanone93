// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useCallback, useRef, useState } from "react";
import type { CanvasSurface } from "../plugin-canvas/surface.ts";

/** The heavy editors that open as full-screen overlays (STU-063); one at a time. */
export type OverlayId = "json" | "rust" | "graph" | "l10n" | "state" | "data" | "db" | "theme" | "history";

/** What the canvas looked like when an overlay opened: everything closing must put back. */
export interface CanvasSnapshot {
  readonly viewport: { readonly x: number; readonly y: number; readonly zoom: number };
  readonly scroll: { readonly left: number; readonly top: number };
  readonly focus: HTMLElement | null;
}

export interface OverlayHostOptions {
  readonly surface: () => CanvasSurface | null;
  readonly canvas: () => HTMLCanvasElement | null;
}

export interface OverlayHost {
  /** The overlay that is open, if any. */
  readonly open: OverlayId | null;
  show(id: OverlayId): void;
  hide(): void;
}

/**
 * Opens heavy editors over the canvas without touching it (STU-063): the canvas stays mounted, its worker is
 * paused (no frames, no idle work, no animation-frame callbacks) and what the user sees of it is recorded.
 * Closing restores viewport, scroll and focus exactly and resumes the worker; the selection lives in the
 * editor store, which an overlay does not write.
 */
export function useOverlayHost({ surface, canvas }: OverlayHostOptions): OverlayHost {
  const [open, setOpen] = useState<OverlayId | null>(null);
  const snapshot = useRef<CanvasSnapshot | null>(null);

  const show = useCallback(
    (id: OverlayId) => {
      if (snapshot.current === null) {
        const s = surface();
        const c = canvas();
        const active = document.activeElement;
        snapshot.current = {
          viewport: s === null ? { x: 0, y: 0, zoom: 1 } : { ...s.viewport },
          scroll: { left: c?.parentElement?.scrollLeft ?? 0, top: c?.parentElement?.scrollTop ?? 0 },
          focus: active instanceof HTMLElement && active !== document.body ? active : c,
        };
        s?.setPaused?.(true);
      }
      setOpen(id);
    },
    [surface, canvas],
  );

  const hide = useCallback(() => {
    const saved = snapshot.current;
    snapshot.current = null;
    setOpen(null);
    if (saved === null) {
      return;
    }
    const s = surface();
    const parent = canvas()?.parentElement;
    s?.setViewport(saved.viewport.x, saved.viewport.y, saved.viewport.zoom);
    s?.setPaused?.(false);
    if (parent) {
      parent.scrollLeft = saved.scroll.left;
      parent.scrollTop = saved.scroll.top;
    }
    // The dialog returns focus as it unmounts; this runs after it, so the element focused before wins.
    setTimeout(() => saved.focus?.focus({ preventScroll: true }), 0);
  }, [surface, canvas]);

  return { open, show, hide };
}
