// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { ReactElement } from "react";
import { decodeCustom, resolveFrame } from "./frames.ts";
import { PreviewSwitcher } from "./preview-switcher.tsx";
import { type PreviewRender, previewRender } from "./switches.ts";
import { usePreviewSwitches } from "./use-preview-switches.ts";

/** The preview switches of an app: the render they imply for the canvas, and the toolbar that sets them (STU-058). */
export function usePreviewBar(app: string | undefined): { look: PreviewRender; bar: ReactElement } {
  const preview = usePreviewSwitches(app ?? "");
  const frame = decodeCustom(preview.state.deviceFrame) ?? resolveFrame(preview.state.deviceFrame);
  return {
    look: previewRender(preview.state, frame.platform),
    bar: (
      <div className="absolute start-3 top-3 z-10">
        <PreviewSwitcher
          state={preview.state}
          dataSources={[]}
          includePseudo={import.meta.env.DEV}
          onChange={(next) => void preview.set(next)}
        />
      </div>
    ),
  };
}
