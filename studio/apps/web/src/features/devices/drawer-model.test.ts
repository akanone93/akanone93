// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { create } from "@bufbuild/protobuf";
import { DevEventSchema, DevLogLevel } from "@plux/api";
import { emptyBuffer, matches, pause, pending, push, resume, tabOf, visible } from "./drawer-model.ts";

const ev = (sequence: bigint, deviceId: string, level: DevLogLevel, message: string) =>
  create(DevEventSchema, { sequence, deviceId, payload: { case: "log", value: { level, source: "app", message } } });

describe("S6.4 drawer model (STU-066)", () => {
  test("routes events to their tab", () => {
    expect(tabOf(ev(1n, "d", DevLogLevel.INFO, "x"))).toBe("logs");
    expect(tabOf(create(DevEventSchema, { payload: { case: "networkCall", value: {} } }))).toBe("network");
    expect(tabOf(create(DevEventSchema, {}))).toBeUndefined();
  });

  test("filters by device, level and search", () => {
    const e = ev(1n, "d1", DevLogLevel.WARNING, "Slow frame");
    expect(matches(e, { deviceId: "d2", level: "all", search: "" })).toBe(false);
    expect(matches(e, { deviceId: "d1", level: "error", search: "" })).toBe(false);
    expect(matches(e, { deviceId: "d1", level: "warning", search: "SLOW" })).toBe(true);
  });

  test("a paused buffer keeps its view while events arrive and the window drops the oldest", () => {
    let b = push(emptyBuffer, ev(1n, "d", DevLogLevel.INFO, "a"));
    b = pause(b);
    b = push(b, ev(2n, "d", DevLogLevel.INFO, "b"));
    expect(visible(b).length).toBe(1);
    expect(pending(b)).toBe(1);
    expect(visible(resume(b)).length).toBe(2);
    const capped = push(
      push(emptyBuffer, ev(1n, "d", DevLogLevel.INFO, "a"), 1),
      ev(2n, "d", DevLogLevel.INFO, "b"),
      1,
    );
    expect(capped.events.map((x) => x.sequence)).toEqual([2n]);
  });
});
