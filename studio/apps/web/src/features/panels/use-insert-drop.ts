// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { WidgetDescriptor } from "@plux/schema";
import type { DragEvent } from "react";
import type { EditFn } from "../plugin-canvas/select/edits.ts";
import type { GestureEnv } from "../plugin-canvas/select/gesture.ts";
import { itemIdOf, type NodeIndex, nodeIdOfHit } from "../plugin-canvas/select/tree.ts";
import type { CanvasSurface } from "../plugin-canvas/surface.ts";
import { ASSET_DRAG_TYPE } from "./assets-tab.tsx";
import { WIDGET_DRAG_TYPE } from "./insert-pane.tsx";
import { dropTarget, insertWidget, type PanelModel } from "./model.ts";
import { setPropRecipe } from "./properties-model.ts";

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

/** Where a drop target puts a new widget: at its index, in its slot, or at a Stack position. */
function placeOf(t: NonNullable<ReturnType<typeof dropTarget>>) {
  return {
    parent: t.parent,
    index: t.index,
    slot: t.slot ?? null,
    ...(t.position === undefined ? {} : { at: t.position }),
  };
}

/** The props of an `Image` dragged in from the Assets tab: the asset, covering its box (AST-002). */
const assetProps = (asset: string) =>
  asset === "" ? undefined : { source: { asset: { $asset: asset } }, fit: "cover" };

/** The image props of the widgets an asset can be dropped on. */
const IMAGE_PROPS: Readonly<Record<string, string>> = { Image: "source", CircleAvatar: "backgroundImage" };

/** Sets the picture of the image or avatar under world point `w` to `asset`; false when there is none. */
function replaceImage(a: Args, w: { x: number; y: number }, asset: string): boolean {
  const s = a.surface();
  const box = a.canvas()?.getBoundingClientRect();
  if (s === null || box === undefined || !a.canEdit) {
    return false;
  }
  const hits = s.hitTest?.((w.x - s.viewport.x) * s.viewport.zoom, (w.y - s.viewport.y) * s.viewport.zoom) ?? [];
  for (const hit of hits) {
    const id = nodeIdOfHit(hit);
    const prop = IMAGE_PROPS[a.index.type.get(id) ?? ""];
    const doc = a.screenOf.get(id);
    if (prop !== undefined && doc !== undefined) {
      a.edit(doc, setPropRecipe(id, "compact", prop, { asset: { $asset: asset } }), { group: `asset:${id}` });
      return true;
    }
  }
  return false;
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
  const mine = (e: DragEvent) =>
    a.canEdit && (e.dataTransfer.types.includes(WIDGET_DRAG_TYPE) || e.dataTransfer.types.includes(ASSET_DRAG_TYPE));
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
      // An image dragged from the Assets tab lands as an `Image` of that asset (AST-002).
      const asset = e.dataTransfer.getData(ASSET_DRAG_TYPE);
      const type = asset === "" ? e.dataTransfer.getData(WIDGET_DRAG_TYPE) : "Image";
      const d = a.descriptors.find((x) => x.type === type);
      if (!mine(e) || w === null || d === undefined) {
        return;
      }
      e.preventDefault();
      // An asset dropped on an image or an avatar replaces its picture (PC-20).
      if (asset !== "" && replaceImage(a, w, asset)) {
        return;
      }
      const t = dropTarget(env(), w.x, w.y);
      if (t !== null) {
        insertWidget(d, placeOf(t), a.edit, (id) => a.screenOf.get(id), a.canEdit, undefined, assetProps(asset));
      }
    },
  };
}
