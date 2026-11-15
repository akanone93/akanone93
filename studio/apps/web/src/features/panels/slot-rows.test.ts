// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import type { Node } from "../plugin-canvas/plugin-model.ts";
import type { PlacedScreen } from "../plugin-canvas/scene.ts";
import { indexNodes } from "../plugin-canvas/select/tree.ts";
import { modelOf, moveIntoSlot } from "./model.ts";
import { slotOfRow, slotRowId, treeNodes } from "./selection-model.ts";

const labels = { empty: "empty", unknown: "Not a slot of this widget" };

const scaffold = (slots: Record<string, unknown>): Node => ({ id: "root", type: "Scaffold", slots }) as unknown as Node;

function tree(root: Node) {
  const screens = [{ id: "home", title: "Home", root }] as unknown as PlacedScreen[];
  const index = indexNodes([root]);
  return { nodes: treeNodes(screens, index, "Plugin", undefined, undefined, undefined, labels), index };
}

describe("[PC-20] Layers names the slot each child sits in", () => {
  test("a Scaffold's slots are rows between it and their children; empty slots say so", () => {
    const { nodes } = tree(scaffold({ body: { id: "c", type: "Center" } }));
    const root = nodes["root"];
    expect(root?.children).toContain(slotRowId("root", "body"));
    expect(root?.children).toContain(slotRowId("root", "appBar"));
    expect(nodes[slotRowId("root", "body")]).toMatchObject({ name: "body", children: ["c"] });
    expect(nodes[slotRowId("root", "body")]?.preview).toBeUndefined();
    expect(nodes[slotRowId("root", "appBar")]).toMatchObject({ name: "appBar", children: [], preview: "empty" });
  });

  test("a child in a slot the widget does not have is shown there, marked", () => {
    const { nodes } = tree(scaffold({ header: { id: "c", type: "Center" } }));
    expect(nodes[slotRowId("root", "header")]).toMatchObject({ children: ["c"], badge: labels.unknown });
  });

  test("a slot row's id says its widget and slot", () => {
    expect(slotOfRow(slotRowId("a-b:c", "floatingActionButton"))).toEqual({
      parent: "a-b:c",
      slot: "floatingActionButton",
    });
    expect(slotOfRow("plain")).toBeNull();
  });
});

describe("[PC-20] a layer moves into a slot", () => {
  const page = (): { kind: string; root: { id: string; type: string; slots: Record<string, unknown> } } => ({
    kind: "page",
    root: { id: "root", type: "Scaffold", slots: { appBar: { id: "c", type: "Center" } } },
  });
  const run = (doc: ReturnType<typeof page>, dragged: string, slot: string) => {
    const root = doc.root as unknown as Node;
    const index = indexNodes([root]);
    const model = modelOf([root]);
    const out: unknown = structuredClone(doc);
    const moved = moveIntoSlot(
      index,
      dragged,
      "root",
      slot,
      // The canvas's edit hands a recipe the page's root, as the plugin canvas does.
      (_path, recipe) => recipe((out as { root: unknown }).root),
      () => "pages/home.page.json",
      true,
      model,
    );
    return { moved, out: out as ReturnType<typeof page> };
  };

  test("from the wrong slot into an empty one", () => {
    const { moved, out } = run(page(), "c", "body");
    expect(moved).toBe(true);
    expect(out.root.slots).toEqual({ body: { id: "c", type: "Center" } });
  });

  test("never into a filled single slot, a slot the widget lacks, or itself", () => {
    const filled = page();
    (filled.root.slots as Record<string, unknown>)["body"] = { id: "b", type: "Text" };
    expect(run(filled, "c", "body").moved).toBe(false);
    expect(run(page(), "c", "header").moved).toBe(false);
    expect(run(page(), "root", "body").moved).toBe(false);
  });
});
