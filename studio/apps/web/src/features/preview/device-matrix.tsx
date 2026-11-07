// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import type { ReactElement, ReactNode } from "react";
import { type DeviceFrame, frameChrome, type MatrixCell, matrixLayout, type Orientation } from "./frames.ts";
import { previewMessages } from "./preview.messages.ts";
import "./device-matrix.css";
import type { PreviewRender } from "./switches.ts";

/** The scale the matrix draws every frame at: the tallest fits `room` CSS pixels, never above 1. */
export function matrixScale(cells: readonly { readonly height: number }[], room: number): number {
  const tallest = Math.max(1, ...cells.map((c) => c.height));
  return Math.min(1, room / tallest);
}

/** The height in CSS pixels the tallest frame of the matrix is drawn at. */
export const MATRIX_ROOM = 560;

/** The bezel around a drawn display, in CSS pixels at full size: thin, as on current phones. */
const BEZEL = 10;

/**
 * The device matrix (STU-057): one screen in several frames side by side, every frame at one scale so their sizes
 * compare. `renderScreen` draws the screen of a cell at its drawn size; the preview switches apply to every cell alike.
 * Each display sits in a thin bezel with the device's own corner radius, and is named with its size and size class.
 */
export function DeviceMatrix({
  frames,
  orientation,
  render,
  renderScreen,
  room = MATRIX_ROOM,
}: {
  readonly frames: readonly DeviceFrame[];
  readonly orientation: Orientation;
  readonly render: PreviewRender;
  readonly renderScreen: (cell: MatrixCell, drawn: { readonly width: number; readonly height: number }) => ReactNode;
  /** The height the tallest frame is drawn at; every frame shares its scale, so their sizes compare. */
  readonly room?: number;
}): ReactElement {
  const m = previewMessages(useLingui().i18n);
  const cells = matrixLayout(frames, orientation);
  const scale = matrixScale(cells, room);
  return (
    <ul
      aria-label={m.matrix}
      dir={render.direction}
      data-preview-theme={render.theme}
      data-platform={render.platform}
      className="device-matrix"
    >
      {cells.map((cell) => (
        <li
          key={cell.frame.id}
          aria-label={m.matrixCell(m.frameNames[cell.frame.id] ?? cell.frame.id, cell.width, cell.height)}
          data-size-class={cell.sizeClass}
          className="device-matrix-cell"
          style={{ fontSize: `${render.textScale}em` }}
        >
          <span
            className="device-matrix-frame"
            data-kind={cell.frame.kind}
            style={{
              padding: BEZEL * scale,
              borderRadius: (frameChrome(cell.frame).radius + BEZEL) * scale,
            }}
          >
            <span
              className="device-matrix-screen"
              style={{
                width: cell.width * scale,
                height: cell.height * scale,
                borderRadius: frameChrome(cell.frame).radius * scale,
              }}
            >
              {renderScreen(cell, { width: cell.width * scale, height: cell.height * scale })}
            </span>
          </span>
          <span className="device-matrix-name">{m.frameNames[cell.frame.id] ?? cell.frame.id}</span>
          <span className="device-matrix-meta">
            {m.matrixSize(cell.width, cell.height)}
            <span className="device-matrix-class" data-size-class={cell.sizeClass}>
              {m.sizeClass(cell.sizeClass)}
            </span>
          </span>
        </li>
      ))}
    </ul>
  );
}
