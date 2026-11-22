// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Icon } from "@plux/ui";
import { type CSSProperties, type ReactNode, useEffect, useState } from "react";
import { appCanvasMessages } from "../app-canvas/app-canvas.messages.ts";
import { type PlacedGhost, targetKey } from "./ghosts.ts";
import { type OverlayStore, useOverlayState } from "./select/overlay-store.ts";
import type { CanvasSurface } from "./surface.ts";

/** Room a target's name takes beyond its box, in CSS pixels: targets stand in a column, with space to their end. */
const LABEL_ROOM = 160;

/**
 * The names of the boxes of the screens and routes the plugin links to (#228), above each box as a screen's title is:
 * one size at every zoom, so a target can be told at a glance where the canvas's own text would be too small to read.
 * Labels only — the box itself takes the pointer (a drag moves it, a click opens it).
 */
export function TargetLabels(props: {
  readonly store: OverlayStore;
  readonly surface: () => CanvasSurface | null;
  readonly targets: readonly PlacedGhost[];
  /** Where a target being dragged is shown now, and the subscription to its moves: its label follows it. */
  readonly shownAt?: (key: string) => { readonly x: number; readonly y: number } | undefined;
  readonly onMoved?: (listener: () => void) => () => void;
}): ReactNode {
  useOverlayState(props.store);
  const [, setMoves] = useState(0);
  const { onMoved } = props;
  useEffect(() => onMoved?.(() => setMoves((n) => n + 1)), [onMoved]);
  const m = appCanvasMessages(useLingui().i18n);
  const surface = props.surface();
  if (surface === null || props.targets.length === 0) {
    return null;
  }
  const { x, y, zoom } = surface.viewport;
  return (
    <div className="pc-headers" aria-hidden="true">
      <div className="pc-headers-plane" style={{ transform: `translate(${-x * zoom}px, ${-y * zoom}px)` }}>
        {props.targets.map((t) => {
          const at = props.shownAt?.(targetKey(t)) ?? t;
          return (
            <div
              key={t.id}
              className="pc-header"
              data-target={t.kind}
              style={
                { "--x": at.x * zoom, "--y": at.y * zoom, "--w": t.w * zoom, "--room": LABEL_ROOM } as CSSProperties
              }
            >
              <span className="pc-header-kind">
                <Icon name={t.kind === "native" ? "device-mobile" : "arrow-up-right"} size="pill" />
              </span>
              <span className="pc-header-name">{t.title}</span>
              {t.kind === "native" && <span className="pc-header-entry">{m.native}</span>}
            </div>
          );
        })}
      </div>
    </div>
  );
}
