// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { type DeviceChromeSpec, frameId, type ScreenStyle } from "@plux/canvas";
import { type ReactElement, useEffect, useState } from "react";
import type { PlacedScreen } from "../plugin-canvas/scene.ts";
import { screenDocuments } from "../plugin-canvas/scene.ts";
import type { CanvasSurface, SurfaceFactory } from "../plugin-canvas/surface.ts";

/** One cell to draw: the frame's size and chrome, and the device pixels its image is drawn at. */
export interface MatrixJob {
  readonly id: string;
  readonly width: number;
  readonly height: number;
  /** The width of the drawn image in device pixels: the cell's drawn width times the display's pixel ratio. */
  readonly pixels: number;
  /** The cell's device chrome (PC-11): each device in the matrix shows its own status bar and cutout. */
  readonly chrome?: DeviceChromeSpec;
}

/** `style` for one cell: the frame's width and height replace the device of `style`, under the same look. */
export function cellStyle(style: ScreenStyle, job: MatrixJob): ScreenStyle {
  const { chrome: _main, ...rest } = style;
  return {
    ...rest,
    deviceHeight: job.height,
    theme: { ...style.theme, width: job.width },
    ...(job.chrome === undefined ? {} : { chrome: job.chrome }),
  };
}

/** Lays `documents` out under `style` and draws the screen `frame` to a PNG `pixels` wide; null if it cannot be drawn. */
async function drawCell(
  surface: CanvasSurface,
  documents: ReturnType<typeof screenDocuments>,
  style: ScreenStyle,
  frame: string,
  pixels: number,
): Promise<Uint8Array | null> {
  await surface.layoutScreens(documents, style);
  return (await surface.thumbnail?.(frame, pixels, true)) ?? null;
}

/**
 * The matrix's images (STU-057): `screen` laid out at every cell's size by the canvas's own layout and renderer, under
 * the same look, each drawn to an image. One surface draws every cell in turn, so the matrix starts one worker and one
 * graphics context rather than one per device, and a browser's cap on live contexts never blanks a cell. The map grows
 * as cells finish; its object URLs are revoked when the screen, the look or the cells change.
 */
export function useMatrixImages(
  screen: PlacedScreen | undefined,
  style: ScreenStyle,
  jobs: readonly MatrixJob[],
  createSurface: SurfaceFactory,
): ReadonlyMap<string, string> {
  const [images, setImages] = useState<ReadonlyMap<string, string>>(new Map());
  // The jobs are rebuilt on every render; their content decides when the cells are drawn again.
  const key = JSON.stringify(jobs);
  useEffect(() => {
    const cells = JSON.parse(key) as MatrixJob[];
    setImages(new Map());
    if (screen === undefined) {
      return undefined;
    }
    const surface = createSurface(document.createElement("canvas"), {
      width: 1,
      height: 1,
      dpr: window.devicePixelRatio || 1,
      present: "direct",
    });
    const urls: string[] = [];
    let live = true;
    const documents = screenDocuments([{ ...screen, at: { x: 0, y: 0 } }]);
    const draw = async (): Promise<void> => {
      for (const job of cells) {
        const png = await drawCell(surface, documents, cellStyle(style, job), frameId(screen.id), job.pixels);
        if (!live) {
          return;
        }
        if (png !== null) {
          const url = URL.createObjectURL(new Blob([png as Uint8Array<ArrayBuffer>], { type: "image/png" }));
          urls.push(url);
          setImages((was) => new Map(was).set(job.id, url));
        }
      }
    };
    void draw();
    return () => {
      live = false;
      surface.dispose();
      for (const url of urls) {
        URL.revokeObjectURL(url);
      }
    };
  }, [screen, style, key, createSurface]);
  return images;
}

/**
 * One matrix cell's screen: its image once drawn, a shimmering placeholder of the same size until then. The cell names
 * the device, so the image itself is decorative.
 */
export function MatrixScreen({
  src,
  width,
  height,
}: {
  readonly src: string | undefined;
  readonly width: number;
  readonly height: number;
}): ReactElement {
  return src === undefined ? (
    <span className="device-matrix-pending" style={{ width, height }} aria-busy="true" />
  ) : (
    <img className="device-matrix-image" src={src} width={width} height={height} alt="" draggable={false} />
  );
}
