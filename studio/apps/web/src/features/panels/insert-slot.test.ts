// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import type { Node } from "../plugin-canvas/plugin-model.ts";
import { modelOf, placeIn } from "./model.ts";

describe("inserting into a widget with slots [STU-061]", () => {
  test("an empty Scaffold takes the new widget in its body, not its app bar", () => {
    const model = modelOf([{ id: "s", type: "Scaffold" } as Node]);
    expect(placeIn(model, "s")).toEqual({ parent: "s", index: 0, slot: "body" });
  });

  test("with the body taken, the next empty slot in the descriptor's order", () => {
    const model = modelOf([{ id: "s", type: "Scaffold", slots: { body: { id: "b", type: "Center" } } } as Node]);
    expect(placeIn(model, "s")).toEqual({ parent: "s", index: 0, slot: "appBar" });
  });

  test("a wrapper takes it as its child", () => {
    const model = modelOf([{ id: "p", type: "Padding" } as Node]);
    expect(placeIn(model, "p")?.slot).toBe("child");
  });
});
