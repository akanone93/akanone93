// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { ScreenStyle } from "@plux/canvas";
import { type ReactElement, useEffect, useRef } from "react";
import type { PlacedScreen } from "../plugin-canvas/scene.ts";
import { screenDocuments } from "../plugin-canvas/scene.ts";
import type { SurfaceFactory } from "../plugin-canvas/surface.ts";

/**
 * One matrix cell (STU-057): the screen drawn at the frame's size by the same worker layout and surface as the
 * canvas, under the same look. The frame's width and height replace the device of `style`.
 */
export function MatrixScreen({
  screen,
  style,
  width,
  height,
  createSurface,
}: {
  readonly screen: PlacedScreen | undefined;
  readonly style: ScreenStyle;
  readonly width: number;
  readonly height: number;
  readonly createSurface: SurfaceFactory;
}): ReactElement {
  const ref = useRef<HTMLCanvasElement>(null);
  useEffect(() => {
    const canvas = ref.current;
    if (canvas === null || screen === undefined) {
      return undefined;
    }
    const surface = createSurface(canvas, { width, height, dpr: window.devicePixelRatio || 1 });
    const framed: ScreenStyle = { ...style, deviceHeight: height, theme: { ...style.theme, width } };
    void surface.layoutScreens(screenDocuments([{ ...screen, at: { x: 0, y: 0 } }]), framed);
    return () => surface.dispose();
  }, [screen, style, width, height, createSurface]);
  return <canvas ref={ref} width={width} height={height} style={{ width, height }} />;
}
