// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import type { ReactElement, ReactNode } from "react";
import { type DeviceFrame, matrixLayout, type Orientation } from "./frames.ts";
import { previewMessages } from "./preview.messages.ts";
import type { PreviewRender } from "./switches.ts";

/**
 * The device matrix (STU-057): one screen in several frames side by side. `renderScreen` draws the
 * screen at a frame's size; the preview switches apply to every cell alike.
 */
export function DeviceMatrix({
  frames,
  orientation,
  render,
  renderScreen,
}: {
  readonly frames: readonly DeviceFrame[];
  readonly orientation: Orientation;
  readonly render: PreviewRender;
  readonly renderScreen: (cell: { width: number; height: number; sizeClass: string }) => ReactNode;
}): ReactElement {
  const m = previewMessages(useLingui().i18n);
  return (
    <ul
      aria-label={m.matrix}
      dir={render.direction}
      data-theme={render.theme}
      data-platform={render.platform}
      className="flex items-start gap-6 overflow-auto"
    >
      {matrixLayout(frames, orientation).map((cell) => (
        <li
          key={cell.frame.id}
          aria-label={m.matrixCell(m.frameNames[cell.frame.id] ?? cell.frame.id, cell.width, cell.height)}
          data-size-class={cell.sizeClass}
          style={{ width: cell.width, height: cell.height, fontSize: `${render.textScale}em` }}
          className="shrink-0 overflow-hidden"
        >
          {renderScreen(cell)}
        </li>
      ))}
    </ul>
  );
}
