// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useCallback, useEffect, useRef, useState } from "react";
import type { CanvasSurface } from "../plugin-canvas/surface.ts";
import { ZOOM_MAX, ZOOM_MIN } from "./use-canvas-gestures.ts";

/**
 * The zoom of the canvas for its controls: read back from the surface after every change of view, at most once a
 * frame, and passed on to the scene (the arrows' level of detail). `zoomBy` zooms about the middle of the view within
 * the zoom range; `resetZoom` goes back to 100%.
 */
export function useZoom(
  surface: () => CanvasSurface | null,
  view: () => HTMLElement | null,
  zoomed: (zoom: number) => void,
) {
  const [zoom, setZoom] = useState(1);
  const frame = useRef(0);
  const viewChanged = useCallback(() => {
    if (frame.current !== 0) {
      return;
    }
    frame.current = requestAnimationFrame(() => {
      frame.current = 0;
      const z = surface()?.viewport.zoom ?? 1;
      setZoom(z);
      zoomed(z);
    });
  }, [surface, zoomed]);
  useEffect(() => () => cancelAnimationFrame(frame.current), []);
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
