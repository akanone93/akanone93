// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { widget } from "@plux/schema";
import type { DragEvent } from "react";
import { indexNodes } from "../plugin-canvas/select/tree.ts";
import type { CanvasSurface } from "../plugin-canvas/surface.ts";
import { ASSET_DRAG_TYPE } from "./assets-tab.tsx";
import { modelOf } from "./model.ts";
import { insertDropHandlers } from "./use-insert-drop.ts";

describe("PC-20 an asset dropped on an image [AST-002]", () => {
  test("replaces the picture of the image under the pointer instead of adding one", () => {
    const root = {
      id: "col",
      type: "Column",
      children: [{ id: "img", type: "Image", props: { source: { asset: { $asset: "old" } } } }],
    };
    const edits: string[] = [];
    const surface = {
      viewport: { x: 0, y: 0, zoom: 1 },
      hitTest: () => ["s1/img#paint0", "screen:s1"],
      boundsOf: () => undefined,
    } as unknown as CanvasSurface;
    const handlers = insertDropHandlers({
      surface: () => surface,
      canvas: () => ({ getBoundingClientRect: () => ({ left: 0, top: 0 }) }) as HTMLCanvasElement,
      index: indexNodes([root] as never),
      model: modelOf([root] as never),
      edit: (doc, recipe) => {
        recipe(root);
        edits.push(doc);
      },
      screenOf: new Map([
        ["col", "s1"],
        ["img", "s1"],
      ]),
      canEdit: true,
      descriptors: [widget("Image")].flatMap((d) => (d === undefined ? [] : [d])),
    });
    handlers.onDrop({
      clientX: 10,
      clientY: 10,
      preventDefault: () => undefined,
      dataTransfer: {
        types: [ASSET_DRAG_TYPE],
        getData: (t: string) => (t === ASSET_DRAG_TYPE ? "logo" : ""),
      },
    } as unknown as DragEvent);
    expect(root.children).toHaveLength(1);
    expect(root.children[0]?.props.source).toEqual({ asset: { $asset: "logo" } });
    expect(edits).toEqual(["s1"]);
  });
});
