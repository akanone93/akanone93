// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
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
      measuredMs: { webgl: 3, raster: 4 },
    },
  },
  {
    v,
    type: "snapshot",
    requestId: 7,
    model: { version: 1, viewport: { x: 0, y: 0, zoom: 1, width: 10, height: 10 }, frames: [], nodes: [] },
  },
  { v, type: "error", message: "boom" },
];

describe("STU-003 canvas worker protocol", () => {
  test("every host message survives structured clone and is read back", () => {
    for (const message of hostMessages) {
      const copy = structuredClone(message);
      expect(copy).toEqual(message);
      expect(readHostMessage(copy)).toEqual(message);
      if (message.type !== "snapshot") {
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

  test("other versions, unknown types and non-objects are rejected", () => {
    expect(readHostMessage({ v: v + 1, type: "patch", ops: [] })).toBeUndefined();
    expect(readHostMessage({ v, type: "eval" })).toBeUndefined();
    expect(readHostMessage(null)).toBeUndefined();
    expect(readWorkerMessage("frame")).toBeUndefined();
  });
});
