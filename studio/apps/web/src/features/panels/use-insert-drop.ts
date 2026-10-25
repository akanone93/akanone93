// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { WidgetDescriptor } from "@plux/schema";
import type { DragEvent } from "react";
import type { EditFn } from "../plugin-canvas/select/edits.ts";
import type { GestureEnv } from "../plugin-canvas/select/gesture.ts";
import { itemIdOf, type NodeIndex } from "../plugin-canvas/select/tree.ts";
import type { CanvasSurface } from "../plugin-canvas/surface.ts";
import { WIDGET_DRAG_TYPE } from "./left-dock.tsx";
import { dropTarget, insertWidget, type PanelModel } from "./model.ts";

interface Args {
  readonly surface: () => CanvasSurface | null;
  readonly canvas: () => HTMLCanvasElement | null;
  readonly index: NodeIndex;
  readonly model: PanelModel;
  readonly edit: EditFn;
  readonly screenOf: ReadonlyMap<string, string>;
  readonly canEdit: boolean;
  readonly descriptors: readonly WidgetDescriptor[];
}

/** Drag-over and drop handlers for the canvas element: a dragged widget shows the S2.5 insertion indicator and lands at it. */
export function insertDropHandlers(a: Args) {
  const env = (): GestureEnv => ({
    index: a.index,
    edit: a.edit,
    docOf: (id) => a.screenOf.get(id),
    boxOf: (id) => {
      const screen = a.screenOf.get(id);
      return screen === undefined ? undefined : a.surface()?.boundsOf?.(itemIdOf(screen, id));
    },
    typeOf: (id) => a.index.type.get(id),
  });
  const world = (e: DragEvent) => {
    const s = a.surface();
    const box = a.canvas()?.getBoundingClientRect();
    return s === null || box === undefined
      ? null
      : {
          x: s.viewport.x + (e.clientX - box.left) / s.viewport.zoom,
          y: s.viewport.y + (e.clientY - box.top) / s.viewport.zoom,
        };
  };
  const mine = (e: DragEvent) => a.canEdit && e.dataTransfer.types.includes(WIDGET_DRAG_TYPE);
  return {
    onDragOver(e: DragEvent): void {
      const w = world(e);
      if (!mine(e) || w === null) {
        return;
      }
      e.preventDefault();
      const t = dropTarget(env(), w.x, w.y);
      a.surface()?.setGuides?.(t === null ? null : { lines: [], labels: [], indicator: t.indicator });
    },
    onDragLeave(): void {
      a.surface()?.setGuides?.(null);
    },
    onDrop(e: DragEvent): void {
      a.surface()?.setGuides?.(null);
      const w = world(e);
      const d = a.descriptors.find((x) => x.type === e.dataTransfer.getData(WIDGET_DRAG_TYPE));
      if (!mine(e) || w === null || d === undefined) {
        return;
      }
      e.preventDefault();
      const t = dropTarget(env(), w.x, w.y);
      if (t !== null) {
        insertWidget(
          d,
          { parent: t.parent, index: t.index, slot: null, ...(t.position === undefined ? {} : { at: t.position }) },
          a.edit,
          (id) => a.screenOf.get(id),
          a.canEdit,
        );
      }
    },
  };
}
