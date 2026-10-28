// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { defaultScreenTheme } from "@plux/layout";
import {
  type HostMessage,
  PROTOCOL_VERSION,
  readHostMessage,
  readWorkerMessage,
  type WorkerMessage,
} from "./protocol.ts";

const v = PROTOCOL_VERSION;

const hostMessages: HostMessage[] = [
  {
    v,
    type: "init",
    canvas: { width: 10, height: 10 },
    width: 10,
    height: 10,
    dpr: 2,
    options: {
      renderer: "auto",
      tileSize: 512,
      background: 0xffffffff,
      fonts: [new ArrayBuffer(4)],
      fontFamily: "Noto Sans",
      measureFrames: false,
    },
  },
  { v, type: "patch", ops: [{ op: "move", id: "a", dx: 1, dy: 2 }, { op: "remove", id: "b" }, { op: "clear" }] },
  { v, type: "viewport", change: { op: "zoom", factor: 1.5, x: 10, y: 20 } },
  { v, type: "interaction", phase: "start", kind: "drag", ids: ["a"] },
  { v, type: "resize", width: 100, height: 50, dpr: 1 },
  { v, type: "snapshot", requestId: 7 },
  {
    v,
    type: "screens",
    requestId: 4,
    screens: [
      {
        id: "home",
        title: "Home",
        x: 10,
        y: 20,
        root: { id: "c", type: "Column", children: [{ id: "t", type: "Text", props: { data: "Hi" } }] },
      },
      { id: "empty", title: "Empty", x: 500, y: 20, root: null },
    ],
    style: {
      deviceHeight: 844,
      theme: { ...defaultScreenTheme },
      frame: 0xffffffff,
      frameStroke: 1,
      box: 2,
      boxStroke: 3,
      placeholder: 4,
      text: 5,
      overflowFill: 6,
      overflowBox: 7,
      overflowText: 8,
      overflowStroke: 9,
    },
  },
  { v, type: "dispose" },
];

const workerMessages: WorkerMessage[] = [
  {
    v,
    type: "ready",
    renderer: {
      kind: "webgl",
      glRenderer: "SwiftShader",
      software: true,
      majorPerformanceCaveat: true,
      tiles: true,
    },
  },
  {
    v,
    type: "snapshot",
    requestId: 7,
    model: { version: 1, viewport: { x: 0, y: 0, zoom: 1, width: 10, height: 10 }, frames: [], nodes: [] },
  },
  {
    v,
    type: "screens",
    requestId: 4,
    ops: [
      { op: "clear" },
      { op: "add", item: { kind: "text", id: "home/t", x: 1, y: 2, w: 3, text: "Hi", size: 16, color: 5, z: 2 } },
    ],
    heights: [{ id: "home", contentHeight: 900 }],
  },
  { v, type: "error", message: "boom" },
];

describe("[STU-003] canvas worker protocol", () => {
  test("every host message survives structured clone and is read back", () => {
    for (const message of hostMessages) {
      const copy = structuredClone(message);
      expect(copy).toEqual(message);
      expect(readHostMessage(copy)).toEqual(message);
      if (message.type !== "snapshot" && message.type !== "screens") {
        expect(readWorkerMessage(copy)).toBeUndefined();
      }
    }
  });

  test("every worker message survives structured clone and is read back", () => {
    for (const message of workerMessages) {
      const copy = structuredClone(message);
      expect(readWorkerMessage(copy)).toEqual(message);
    }
  });

  test("[STU-121] a screensUpdate message (changed screens, removed ids) survives structured clone", () => {
    const full = hostMessages.find((m): m is Extract<HostMessage, { type: "screens" }> => m.type === "screens");
    if (full === undefined) {
      throw new Error("no screens message");
    }
    const update: HostMessage = {
      v,
      type: "screensUpdate",
      requestId: 5,
      screens: full.screens.slice(0, 1),
      removed: ["empty"],
      style: full.style,
    };
    const copy = structuredClone(update);
    expect(readHostMessage(copy)).toEqual(update);
    expect(readWorkerMessage({ ...copy })).toBeUndefined();
  });

  test("other versions, unknown types and non-objects are rejected", () => {
    expect(readHostMessage({ v: v + 1, type: "patch", ops: [] })).toBeUndefined();
    expect(readHostMessage({ v, type: "eval" })).toBeUndefined();
    expect(readHostMessage(null)).toBeUndefined();
    expect(readWorkerMessage("frame")).toBeUndefined();
  });
});
