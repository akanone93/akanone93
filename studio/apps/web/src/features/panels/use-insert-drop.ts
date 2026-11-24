// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { WidgetDescriptor } from "@plux/schema";
import type { DragEvent } from "react";
import type { DropPlan, Refusal } from "../plugin-canvas/select/drop-rules.ts";
import type { EditFn } from "../plugin-canvas/select/edits.ts";
import type { GestureEnv } from "../plugin-canvas/select/gesture.ts";
import { dropMarks, noDrop, type OverlayStore } from "../plugin-canvas/select/overlay-store.ts";
import { itemIdOf, type NodeIndex, nodeIdOfHit } from "../plugin-canvas/select/tree.ts";
import type { CanvasSurface } from "../plugin-canvas/surface.ts";
import { ASSET_DRAG_TYPE } from "./assets-tab.tsx";
import type { StarterTexts } from "./catalogue.ts";
import { WIDGET_DRAG_TYPE } from "./insert-pane.tsx";
import { dropTarget, insertWidget, layoutRoots, type PanelModel, type Place } from "./model.ts";
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
  /** What a new widget starts with, in Studio's language. */
  readonly starter?: StarterTexts;
  /** Receives the id of a widget a drop inserted, to select it. */
  readonly onInserted?: (id: string) => void;
  /** The overlay that shows where a drop lands, or why it cannot (PC-20). */
  readonly overlay?: OverlayStore;
  /** The type of the widget being dragged from the Insert pane: a drag-over cannot read the dragged data. */
  readonly dragType?: () => string | null;
}

/** Where a drop target puts a new widget of `type`: at its index, in its slot, or at a Stack position. */
function placeOf(t: Extract<DropPlan, { ok: true }>["target"], type: string, parentType: string | undefined): Place {
  // A screen's first content goes in a safe area, padded, in a column (as Insert does), unless it is a layout itself.
  const screenContent =
    parentType === "Scaffold" && t.slot === "body" && t.wrap === undefined && !layoutRoots.has(type);
  return {
    parent: t.parent,
    index: t.index,
    slot: t.slot ?? null,
    ...(t.list === true ? { list: true } : {}),
    ...(t.wrap === undefined ? {} : { wrap: t.wrap }),
    ...(t.position === undefined ? {} : { at: t.position }),
    ...(screenContent ? { screenContent } : {}),
  };
}

/** The props of an `Image` dragged in from the Assets tab: the asset, covering its box (AST-002). */
const assetProps = (asset: string) =>
  asset === "" ? undefined : { source: { asset: { $asset: asset } }, fit: "cover" };

/** The image props of the widgets an asset can be dropped on. */
const IMAGE_PROPS: Readonly<Record<string, string>> = { Image: "source", CircleAvatar: "backgroundImage" };

/** The image or avatar under world point `w` an asset would replace the picture of, with its prop; or undefined. */
function imageUnder(a: Args, w: { x: number; y: number }): { id: string; prop: string; doc: string } | undefined {
  const s = a.surface();
  if (s === null || !a.canEdit) {
    return undefined;
  }
  const hits = s.hitTest?.((w.x - s.viewport.x) * s.viewport.zoom, (w.y - s.viewport.y) * s.viewport.zoom) ?? [];
  for (const hit of hits) {
    const id = nodeIdOfHit(hit);
    const prop = IMAGE_PROPS[a.index.type.get(id) ?? ""];
    const doc = a.screenOf.get(id);
    if (prop !== undefined && doc !== undefined) {
      return { id, prop, doc };
    }
  }
  return undefined;
}

/** Sets the picture of the image or avatar under world point `w` to `asset`; false when there is none. */
function replaceImage(a: Args, w: { x: number; y: number }, asset: string): boolean {
  const image = imageUnder(a, w);
  if (image === undefined) {
    return false;
  }
  a.edit(image.doc, setPropRecipe(image.id, "compact", image.prop, { asset: { $asset: asset } }), {
    group: `asset:${image.id}`,
  });
  return true;
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
  const isAsset = (e: DragEvent) => e.dataTransfer.types.includes(ASSET_DRAG_TYPE);
  const marks = (change: Parameters<OverlayStore["set"]>[0]) => a.overlay?.set(change);
  // The refusal where the drag last was over the canvas: a browser fires no drop where dropping is not allowed, so
  // the drag's end tells the person why nothing was dropped.
  let refused: { reason: Refusal; type: string; at: string | null; x: number; y: number } | null = null;
  return {
    /** The drag from the Insert pane ended: a refused drop says why where it was let go. */
    onDragEnd(): void {
      if (refused !== null) {
        marks({ ...noDrop, notice: refused });
      }
      refused = null;
    },
    onDragOver(e: DragEvent): void {
      const w = world(e);
      if (!mine(e) || w === null) {
        return;
      }
      e.preventDefault();
      // An image dragged in over an image or an avatar replaces its picture: that widget is the target (PC-20).
      const image = isAsset(e) ? imageUnder(a, w) : undefined;
      if (image !== undefined) {
        e.dataTransfer.dropEffect = "copy";
        a.surface()?.setIndicator?.(null);
        marks({ ...noDrop, drop: image.id, dropSlot: image.prop });
        return;
      }
      const type = isAsset(e) ? "Image" : (a.dragType?.() ?? "");
      const plan = dropTarget(env(), w.x, w.y, type);
      // Where the widget cannot go the cursor says so, and the reason shows on the widget under the pointer.
      e.dataTransfer.dropEffect = plan.ok ? "copy" : "none";
      refused = plan.ok ? null : { reason: plan.reason, type, at: plan.at, x: w.x, y: w.y };
      const shown = dropMarks(plan, type);
      a.surface()?.setIndicator?.(shown.indicator);
      marks(shown);
    },
    onDragLeave(e: DragEvent): void {
      a.surface()?.setIndicator?.(null);
      marks(noDrop);
      // Leaving the canvas for another part of the page is no drop; letting go leaves with nothing entered.
      if (e.relatedTarget !== null) {
        refused = null;
      }
    },
    onDrop(e: DragEvent): void {
      a.surface()?.setIndicator?.(null);
      marks(noDrop);
      refused = null;
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
      const plan = dropTarget(env(), w.x, w.y, type);
      if (!plan.ok) {
        marks({ notice: { reason: plan.reason, type, at: plan.at, x: w.x, y: w.y } });
        return;
      }
      const id = insertWidget(
        d,
        placeOf(plan.target, type, a.index.type.get(plan.target.parent)),
        a.edit,
        (id) => a.screenOf.get(id),
        a.canEdit,
        undefined,
        assetProps(asset),
        a.starter,
      );
      if (id !== null) {
        a.onInserted?.(id);
      }
    },
  };
}
