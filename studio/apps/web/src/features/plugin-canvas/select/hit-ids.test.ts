// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { beforeAll, describe, expect, test } from "bun:test";
import { CanvasWorkerCore, PROTOCOL_VERSION, type WorkerMessage } from "@plux/canvas";
import { fontBytes, loadCanvasKit, TestEnvironment } from "../../../../../../packages/canvas/test-support/canvaskit.ts";
import type { Node } from "../plugin-model.ts";
import { screenDocuments, screenStyle } from "../scene.ts";
import { indexNodes, nodeIdOfHit } from "./tree.ts";

let ck: Awaited<ReturnType<typeof loadCanvasKit>>;
beforeAll(async () => {
  ck = await loadCanvasKit();
});

const root: Node = {
  id: "n-root",
  type: "Column",
  children: [
    { id: "t1", type: "Text", props: { data: "One" } },
    { id: "pad", type: "Padding", slots: { child: { id: "t2", type: "Text", props: { data: "Two" } } } },
    { id: "box", type: "SizedBox", props: { width: 40, height: 20 } as { data?: unknown } },
  ],
};

describe("[STU-067] hit IDs of the real worker layout", () => {
  test("[STU-067] every placed item maps back to a document node ID", async () => {
    const env = new TestEnvironment(ck);
    const posted: WorkerMessage[] = [];
    const core = new CanvasWorkerCore((m) => posted.push(m), env.deps());
    await core.receive({
      v: PROTOCOL_VERSION,
      type: "init",
      canvas: { width: 0, height: 0 },
      width: 400,
      height: 300,
      dpr: 1,
      options: {
        renderer: "raster",
        tileSize: 256,
        background: 0xffffffff,
        fonts: [fontBytes()],
        fontFamilies: ["Noto Sans"],
        fontFamily: "Noto Sans",
        measureFrames: false,
      },
    });
    const style = { ...screenStyle, theme: { ...screenStyle.theme, fontFamilies: ["Noto Sans"] } };
    await core.receive({
      v: PROTOCOL_VERSION,
      type: "screens",
      requestId: 1,
      style,
      screens: screenDocuments([{ id: "home", title: "Home", at: { x: 0, y: 0 }, root }]),
    });
    const reply = posted.find((m): m is Extract<WorkerMessage, { type: "screens" }> => m.type === "screens");
    const ids = (reply?.ops ?? []).flatMap((op) => (op.op === "add" ? [op.item.id] : []));
    const items = ids.filter((id) => !id.startsWith("screen:") && !id.startsWith("overflow:"));
    const nodes = indexNodes([root]).parent;
    expect(items.length).toBeGreaterThan(0);
    for (const id of items) {
      expect(nodes.has(nodeIdOfHit(id))).toBe(true);
    }
    await core.receive({ v: PROTOCOL_VERSION, type: "dispose" });
  });
});
