// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import type { DragEvent } from "react";
import { createOverlayStore } from "../plugin-canvas/select/overlay-store.ts";
import { indexNodes } from "../plugin-canvas/select/tree.ts";
import type { CanvasSurface } from "../plugin-canvas/surface.ts";
import { catalogue } from "./catalogue.ts";
import { WIDGET_DRAG_TYPE } from "./insert-pane.tsx";
import { modelOf } from "./model.ts";
import { insertDropHandlers } from "./use-insert-drop.ts";

const root = {
  id: "s",
  type: "Scaffold",
  slots: {
    body: { id: "wrap", type: "Wrap", children: [{ id: "w1", type: "Text", props: { data: "A" } }] },
  },
};
const boxes: Record<string, { l: number; t: number; r: number; b: number }> = {
  s: { l: 0, t: 0, r: 400, b: 800 },
  wrap: { l: 0, t: 0, r: 400, b: 800 },
  w1: { l: 0, t: 0, r: 40, b: 20 },
};

function rig(type: string) {
  const overlay = createOverlayStore();
  const edits: string[] = [];
  const surface = {
    viewport: { x: 0, y: 0, zoom: 1 },
    boundsOf: (item: string) => boxes[item.split("/")[1] ?? ""],
    setIndicator: () => undefined,
  } as unknown as CanvasSurface;
  const handlers = insertDropHandlers({
    surface: () => surface,
    canvas: () => ({ getBoundingClientRect: () => ({ left: 0, top: 0 }) }) as HTMLCanvasElement,
    index: indexNodes([root] as never),
    model: modelOf([root] as never),
    edit: (doc) => {
      edits.push(doc);
    },
    screenOf: new Map([
      ["s", "p"],
      ["wrap", "p"],
      ["w1", "p"],
    ]),
    canEdit: true,
    descriptors: catalogue(),
    overlay,
    dragType: () => type,
  });
  const event = (x: number, y: number) => {
    const dataTransfer = {
      types: [WIDGET_DRAG_TYPE],
      getData: (t: string) => (t === WIDGET_DRAG_TYPE ? type : ""),
      dropEffect: "",
    };
    return { clientX: x, clientY: y, preventDefault: () => undefined, dataTransfer, relatedTarget: null };
  };
  return { overlay, edits, handlers, event };
}

describe("[STU-067] a widget dragged from the Insert pane where it cannot go (PC-20)", () => {
  test("shows why while over it, shows the not-allowed cursor, and says so once let go", () => {
    const r = rig("Expanded");
    const over = r.event(200, 400);
    r.handlers.onDragOver(over as unknown as DragEvent);
    expect(over.dataTransfer.dropEffect).toBe("none");
    expect(r.overlay.get().refused).toEqual({ reason: "needsFlex", type: "Expanded", at: "wrap" });
    expect(r.overlay.get().drop).toBeNull();
    // Letting go where dropping is not allowed: the browser leaves the canvas, then ends the drag.
    r.handlers.onDragLeave(over as unknown as DragEvent);
    r.handlers.onDragEnd();
    expect(r.overlay.get().notice).toMatchObject({ reason: "needsFlex", type: "Expanded", x: 200, y: 400 });
    expect(r.edits).toEqual([]);
  });

  test("a drop where it can go names its target, and nothing is left to say after", () => {
    const r = rig("Text");
    const over = r.event(200, 400);
    r.handlers.onDragOver(over as unknown as DragEvent);
    expect(over.dataTransfer.dropEffect).toBe("copy");
    expect(r.overlay.get()).toMatchObject({ drop: "wrap", dropSlot: null, refused: null });
    r.handlers.onDrop(over as unknown as DragEvent);
    r.handlers.onDragEnd();
    expect(r.edits).toEqual(["p"]);
    expect(r.overlay.get().notice).toBeNull();
    expect(r.overlay.get().drop).toBeNull();
  });

  test("leaving the canvas for another part of the page is no refusal", () => {
    const r = rig("Expanded");
    const over = r.event(200, 400);
    r.handlers.onDragOver(over as unknown as DragEvent);
    r.handlers.onDragLeave({ ...over, relatedTarget: {} } as unknown as DragEvent);
    r.handlers.onDragEnd();
    expect(r.overlay.get().notice).toBeNull();
  });
});
