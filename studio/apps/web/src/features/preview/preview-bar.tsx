// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button } from "@plux/ui";
import { type ReactElement, useRef, useState } from "react";
import { DeviceMatrix } from "./device-matrix.tsx";
import { decodeCustom, frameCatalogue, resolveFrame } from "./frames.ts";
import { previewMessages } from "./preview.messages.ts";
import { PreviewSwitcher } from "./preview-switcher.tsx";
import { type PreviewRender, previewRender } from "./switches.ts";
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
    <div className="absolute start-3 top-3 z-10 flex flex-wrap items-end gap-3">
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
): { look: PreviewRender; bar: ReactElement; matrix: boolean } {
  const preview = usePreviewSwitches(app ?? "");
  const [matrix, setMatrix] = useState(false);
  const frame = decodeCustom(preview.state.deviceFrame) ?? resolveFrame(preview.state.deviceFrame);
  // The render keeps its identity while the switches are unchanged, so the canvas lays out again only on a change.
  const cache = useRef<{ key: string; look: PreviewRender } | null>(null);
  const key = JSON.stringify([preview.state, frame.platform]);
  if (cache.current?.key !== key) {
    cache.current = { key, look: previewRender(preview.state, frame.platform) };
  }
  return {
    look: cache.current.look,
    matrix,
    bar: (
      <PreviewBarView
        matrix={matrix}
        onMatrix={setMatrix}
        switcher={
          <PreviewSwitcher
            state={preview.state}
            dataSources={dataSources}
            includePseudo={import.meta.env.DEV}
            onChange={(next) => void preview.set(next)}
          />
        }
      />
    ),
  };
}

/** The device matrix over the canvas, in place of the single frame (STU-057). */
export function MatrixOverlay({
  show,
  render,
}: {
  readonly show: boolean;
  readonly render: PreviewRender;
}): ReactElement | null {
  return !show ? null : (
    <div className="absolute inset-0 z-5 overflow-auto bg-raised p-6 pt-20">
      <DeviceMatrix
        frames={frameCatalogue}
        orientation="portrait"
        render={render}
        renderScreen={(cell) => <span className="p-2">{cell.sizeClass}</span>}
      />
    </div>
  );
}
