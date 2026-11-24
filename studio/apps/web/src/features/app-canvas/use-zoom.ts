// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useCallback, useEffect, useRef, useState } from "react";
import type { CanvasSurface } from "../plugin-canvas/surface.ts";
import { ZOOM_MAX, ZOOM_MIN } from "./use-canvas-gestures.ts";

/** How long the view must rest before the page takes its zoom and `moved` hears of it (#214). */
const VIEW_SETTLE_MS = 200;

/**
 * The zoom of the canvas: passed on to the scene (the arrows' level of detail) at most once a frame, and taken by the
 * page — with `moved` hearing of it, as the canvas keeps the view it was left at — once the view rests: a page update
 * once a frame through a pinch re-rendered the whole canvas page at each step (#214); the zoom controls follow the
 * surface themselves (`FollowedZoomControls`). `zoomBy` zooms about the middle of the view within the zoom range;
 * `resetZoom` goes back to 100%.
 */
export function useZoom(
  surface: () => CanvasSurface | null,
  view: () => HTMLElement | null,
  zoomed: (zoom: number) => void,
  moved?: () => void,
) {
  const [zoom, setZoom] = useState(1);
  const frame = useRef(0);
  const settle = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const viewChanged = useCallback(() => {
    clearTimeout(settle.current);
    settle.current = setTimeout(() => {
      setZoom(surface()?.viewport.zoom ?? 1);
      moved?.();
    }, VIEW_SETTLE_MS);
    if (frame.current !== 0) {
      return;
    }
    frame.current = requestAnimationFrame(() => {
      frame.current = 0;
      zoomed(surface()?.viewport.zoom ?? 1);
    });
  }, [surface, zoomed, moved]);
  useEffect(
    () => () => {
      cancelAnimationFrame(frame.current);
      clearTimeout(settle.current);
    },
    [],
  );
  const zoomBy = useCallback(
    (factor: number) => {
      const target = surface();
      const element = view();
      if (target === null || element === null) {
        return;
      }
      const z = target.viewport.zoom;
      const next = Math.min(ZOOM_MAX, Math.max(ZOOM_MIN, z * factor));
      target.zoomAbout(next / z, element.clientWidth / 2, element.clientHeight / 2);
      viewChanged();
    },
    [surface, view, viewChanged],
  );
  const resetZoom = useCallback(() => zoomBy(1 / (surface()?.viewport.zoom ?? 1)), [surface, zoomBy]);
  return { zoom, setZoom, viewChanged, zoomBy, resetZoom };
}
