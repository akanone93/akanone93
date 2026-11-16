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

  test("a screen's first content goes in its body inside a SafeArea and a Column; a layout goes as it is", () => {
    const model = modelOf([{ id: "s", type: "Scaffold" } as Node]);
    expect(placeIn(model, "s", "ElevatedButton")).toEqual({ parent: "s", index: 0, slot: "body", screenContent: true });
    expect(placeIn(model, "s", "ListView")).toEqual({ parent: "s", index: 0, slot: "body" });
  });

  test("chrome slots take only their own widgets: an AppBar the app bar, a button never", () => {
    const body = { id: "sa", type: "SafeArea", slots: { child: { id: "col", type: "Column", children: [] } } };
    const model = modelOf([{ id: "s", type: "Scaffold", slots: { body } } as Node]);
    expect(placeIn(model, "s", "AppBar")).toEqual({ parent: "s", index: 0, slot: "appBar" });
    expect(placeIn(model, "s", "FloatingActionButton")?.slot).toBe("floatingActionButton");
    // With the body taken, a button goes on into the body's Column, at its end.
    expect(placeIn(model, "s", "ElevatedButton")).toEqual({ parent: "col", index: 0, slot: null });
  });

  test("with the body taken by a widget that holds nothing more, a button has no place on the screen", () => {
    const model = modelOf([{ id: "s", type: "Scaffold", slots: { body: { id: "t", type: "Text" } } } as Node]);
    expect(placeIn(model, "s", "ElevatedButton")).toBeNull();
  });

  test("a wrapper takes it as its child", () => {
    const model = modelOf([{ id: "p", type: "Padding" } as Node]);
    expect(placeIn(model, "p")?.slot).toBe("child");
  });
});
