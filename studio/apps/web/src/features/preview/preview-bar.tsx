// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import type { ScreenStyle } from "@plux/canvas";
import { Button } from "@plux/ui";
import { type ReactElement, useEffect, useRef, useState } from "react";
import { chromeSpec } from "../plugin-canvas/look.ts";
import type { PlacedScreen } from "../plugin-canvas/scene.ts";
import type { SurfaceFactory } from "../plugin-canvas/surface.ts";
import { DeviceMatrix, MATRIX_ROOM, matrixScale } from "./device-matrix.tsx";
import { useFrameView } from "./frame-view.ts";
import { decodeCustom, frameCatalogue, matrixLayout, type Orientation, resolveFrame } from "./frames.ts";
import { type MatrixJob, MatrixScreen, useMatrixImages } from "./matrix-screen.tsx";
import { previewMessages } from "./preview.messages.ts";
import { PreviewSwitcher } from "./preview-switcher.tsx";
import { type PreviewRender, previewDevice, previewRender } from "./switches.ts";
import { usePreviewSwitches } from "./use-preview-switches.ts";

/** The toolbar of the preview switches with the toggle between the single frame and the device matrix (STU-057). */
export function PreviewBarView({
  switcher,
  matrix,
  onMatrix,
}: {
  readonly switcher: ReactElement;
  readonly matrix: boolean;
  readonly onMatrix: (on: boolean) => void;
}): ReactElement {
  const m = previewMessages(useLingui().i18n);
  return (
    <div className="flex flex-col gap-3">
      {switcher}
      <Button variant="secondary" aria-pressed={matrix} onClick={() => onMatrix(!matrix)}>
        {m.matrix}
      </Button>
    </div>
  );
}

/**
 * The preview switches of an app: the render they imply for the canvas, the toolbar that sets them
 * (STU-058) and whether the device matrix shows in place of the single frame.
 */
export function usePreviewBar(
  app: string | undefined,
  dataSources: readonly string[],
  brands: readonly string[] = [],
  sourceNames: Readonly<Record<string, string>> = {},
  locales: readonly string[] = [],
): {
  look: PreviewRender;
  bar: ReactElement;
  switcher: ReactElement;
  matrix: boolean;
  closeMatrix: () => void;
} {
  const preview = usePreviewSwitches(app ?? "");
  const [matrix, setMatrix] = useState(false);
  const [view, setView] = useFrameView();
  const frame = decodeCustom(preview.state.deviceFrame) ?? resolveFrame(preview.state.deviceFrame);
  // The render keeps its identity while the switches are unchanged, so the canvas lays out again only on a change.
  const cache = useRef<{ key: string; look: PreviewRender } | null>(null);
  const key = JSON.stringify([preview.state, frame, view]);
  if (cache.current?.key !== key) {
    const device = previewDevice(frame, view.orientation, view.chrome);
    cache.current = { key, look: previewRender(preview.state, frame.platform, device) };
  }
  return {
    look: cache.current.look,
    matrix,
    closeMatrix: () => setMatrix(false),
    switcher: (
      <PreviewSwitcher
        part="main"
        view={view}
        onView={setView}
        state={preview.state}
        dataSources={dataSources}
        includePseudo={import.meta.env.DEV}
        locales={locales}
        onChange={(next) => void preview.set(next)}
      />
    ),
    bar: (
      <PreviewBarView
        matrix={matrix}
        onMatrix={setMatrix}
        switcher={
          <PreviewSwitcher
            part="extra"
            state={preview.state}
            dataSources={dataSources}
            sourceNames={sourceNames}
            brands={brands}
            includePseudo={import.meta.env.DEV}
            onChange={(next) => void preview.set(next)}
          />
        }
      />
    ),
  };
}

/**
 * The device matrix over the canvas, in place of the single frame (STU-057): the screen being worked on in every device
 * frame at one scale, each named with its size and size class, under a heading that says what it is for; Escape or
 * the button closes it.
 */
export function MatrixOverlay({
  show,
  render,
  screen,
  style,
  createSurface,
  onClose,
}: {
  readonly show: boolean;
  readonly render: PreviewRender;
  readonly screen: PlacedScreen | undefined;
  readonly style: ScreenStyle;
  readonly createSurface: SurfaceFactory;
  readonly onClose: () => void;
}): ReactElement | null {
  const m = previewMessages(useLingui().i18n);
  useEffect(() => {
    if (!show) {
      return undefined;
    }
    const close = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        onClose();
      }
    };
    window.addEventListener("keydown", close);
    return () => window.removeEventListener("keydown", close);
  }, [show, onClose]);
  // The matrix follows the preview's orientation and chrome; every cell is drawn at its size on screen, in device pixels.
  const orientation: Orientation = render.device.landscape ? "landscape" : "portrait";
  const cells = matrixLayout(frameCatalogue, orientation);
  const scale = matrixScale(cells, MATRIX_ROOM);
  const dpr = typeof window === "undefined" ? 1 : window.devicePixelRatio || 1;
  const jobs: MatrixJob[] = show
    ? cells.map((cell) => ({
        id: cell.frame.id,
        width: cell.width,
        height: cell.height,
        pixels: Math.ceil(cell.width * scale * dpr),
        chrome: chromeSpec(previewDevice(cell.frame, orientation, render.device.chrome), cell.frame.platform),
      }))
    : [];
  const images = useMatrixImages(show ? screen : undefined, style, jobs, createSurface);
  return !show ? null : (
    <section className="device-matrix-overlay" aria-label={m.matrix}>
      <header className="device-matrix-head">
        <div className="flex min-w-0 flex-col gap-1">
          <h2 className="device-matrix-title">{m.matrixTitle(screen?.title ?? "")}</h2>
          <p className="device-matrix-help">{m.matrixHelp}</p>
        </div>
        <Button variant="secondary" onClick={onClose}>
          {m.matrixClose}
        </Button>
      </header>
      <DeviceMatrix
        frames={frameCatalogue}
        orientation={orientation}
        render={render}
        renderScreen={(cell, drawn) => (
          <MatrixScreen src={images.get(cell.frame.id)} width={drawn.width} height={drawn.height} />
        )}
      />
    </section>
  );
}
