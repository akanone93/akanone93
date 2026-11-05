// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { beforeAll, describe, expect, test } from "bun:test";
import { defaultScreenTheme, type WidgetNode } from "@plux/layout";
import type { CanvasKit } from "canvaskit-wasm";
import { fontBytes, loadCanvasKit } from "../test-support/canvaskit.ts";
import { CanvasHost, type WorkerLike } from "./host.ts";
import type { CanvasOptions, HostMessage, ItemSpec, ScreenStyle } from "./protocol.ts";
import { moveSpec } from "./scene.ts";
import { ScreenLayouts } from "./screens.ts";
import { canvasParagraphEngine } from "./worker.ts";

let ck: CanvasKit;
beforeAll(async () => {
  ck = await loadCanvasKit();
});

const style: ScreenStyle = {
  deviceHeight: 844,
  theme: { ...defaultScreenTheme, fontFamilies: ["Noto Sans"] },
  frame: 0xffffffff,
  frameStroke: 0x11,
  box: 0x22,
  boxStroke: 0x33,
  placeholder: 0x44,
  text: 0x55,
  overflowFill: 0x66,
  overflowBox: 0x77,
  overflowText: 0x88,
  overflowStroke: 0x99,
};
const root: WidgetNode = {
  id: "col",
  type: "Column",
  children: [
    { id: "a", type: "Text", props: { data: "One" } },
    { id: "b", type: "Text", props: { data: "Two" } },
  ],
};

describe("[STU-050] a screen dragged on the canvas is moved, not laid out again", () => {
  test("[STU-050] moving an item moves the world points it paints by: its clips and its gradient", () => {
    const rect: ItemSpec = {
      kind: "rect",
      id: "r",
      x: 10,
      y: 20,
      w: 30,
      h: 40,
      radius: 0,
      z: 1,
      clips: [{ x: 0, y: 0, w: 100, h: 100 }],
      gradient: { kind: "linear", colors: [1, 2], from: [10, 20], to: [40, 60], radius: 0 },
    };
    const moved = moveSpec(rect, 5, -5);
    expect(
      moved.kind === "rect" && [moved.x, moved.y, moved.clips?.[0], moved.gradient?.from, moved.gradient?.to],
    ).toEqual([15, 15, { x: 5, y: -5, w: 100, h: 100 }, [15, 15], [45, 55]]);
    const text: ItemSpec = {
      kind: "text",
      id: "t",
      x: 0,
      y: 0,
      w: 10,
      text: "T",
      size: 12,
      color: 1,
      z: 1,
      clips: [{ x: 1, y: 1, w: 2, h: 2 }],
    };
    const movedText = moveSpec(text, 1, 1);
    expect(movedText.kind === "text" && movedText.clips?.[0]).toEqual({ x: 2, y: 2, w: 2, h: 2 });
    const { clips: _clips, gradient: _gradient, ...bare } = rect;
    const plain = moveSpec(bare, 1, 1);
    expect(plain.kind === "rect" && [plain.clips, plain.gradient]).toEqual([undefined, undefined]);
  });

  test("[STU-050] the worker translates a moved screen's items with move ops and lays nothing out", () => {
    const engine = canvasParagraphEngine(ck, { fonts: [fontBytes()], fontFamily: "Noto Sans" } as never);
    const layouts = new ScreenLayouts();
    const first = layouts.replaceAll(
      [
        { id: "s", title: "S", x: 0, y: 0, root },
        { id: "t", title: "T", x: 500, y: 0, root },
      ],
      style,
      engine,
    );
    const ownOf = (id: string) => id === "screen:s" || id === "title:s" || id.startsWith("s/");
    const items = first.ops.flatMap((op) => (op.op === "add" && ownOf(op.item.id) ? [op.item.id] : []));
    const before = layouts.laidOut;
    const { ops, heights } = layouts.update([], [], style, engine, [{ id: "s", dx: 40, dy: -10 }]);
    expect(layouts.laidOut).toBe(before);
    expect(heights).toEqual([]);
    expect(ops.every((op) => op.op === "move" && op.dx === 40 && op.dy === -10)).toBe(true);
    expect(ops.map((op) => (op.op === "move" ? op.id : "")).sort()).toEqual([...items].sort());
    // A later edit of the moved screen diffs against its moved items: replacements, not a clear and adds.
    const edited = layouts.update([{ id: "s", title: "S", x: 40, y: -10, root }], [], style, engine);
    expect(edited.ops).toEqual([]);
    expect(layouts.update([], [], style, engine, [{ id: "gone", dx: 1, dy: 1 }]).ops).toEqual([]);
    engine.dispose();
  });

  test("[STU-050] the host sends only a move for a screen whose tree and title are unchanged", () => {
    const sent: HostMessage[] = [];
    const worker: WorkerLike = {
      onmessage: null,
      postMessage: (message) => sent.push(message as HostMessage),
      terminate: () => undefined,
    };
    const options: CanvasOptions = {
      renderer: "raster",
      tileSize: 256,
      background: 0xffffffff,
      fonts: [],
      fontFamily: "Noto Sans",
      measureFrames: false,
    };
    const element = { width: 0, height: 0, transferControlToOffscreen: () => ({ width: 0, height: 0 }) };
    const host = new CanvasHost(element, { width: 300, height: 200, dpr: 1 }, options, {
      createWorker: () => worker,
      mainThread: () => {
        throw new Error("unused");
      },
    });
    const other: WidgetNode = { id: "x", type: "Text", props: { data: "X" } };
    void host.layoutScreens(
      [
        { id: "s", title: "S", x: 0, y: 0, root },
        { id: "t", title: "T", x: 500, y: 0, root: other },
      ],
      style,
    );
    void host.layoutScreens(
      [
        { id: "s", title: "S", x: 30, y: 20, root },
        { id: "t", title: "T", x: 500, y: 0, root: { ...other } },
      ],
      style,
    );
    const update = sent.findLast((m) => m.type === "screensUpdate");
    expect(update?.type === "screensUpdate" && update.moved).toEqual([{ id: "s", dx: 30, dy: 20 }]);
    expect(update?.type === "screensUpdate" && update.screens.map((s) => s.id)).toEqual(["t"]);
    host.dispose();
  });
});
