// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { pointerOf } from "../panels/properties-model.ts";
import { applyConditions } from "./conditions.ts";
import type { Node } from "./plugin-model.ts";
import { findNode, wrapRecipe } from "./select/edits.ts";
import { indexNodes } from "./select/tree.ts";
import { variantArms, variantOf } from "./variants.ts";

const text = (id: string, data: string) => ({ id, type: "Text", props: { data } });

/** A page whose column holds a Variant: a text as the control and a column of two texts as variant `wide`. */
function page() {
  return {
    id: "p",
    root: {
      id: "root",
      type: "Column",
      children: [
        {
          id: "v",
          type: "Variant",
          experiment: "heroLayout",
          control: text("c", "Welcome"),
          variants: {
            wide: { id: "w", type: "Column", children: [text("w1", "Welcome back"), text("w2", "We missed you")] },
            "a/b": text("ab", "Hello"),
          },
        },
      ],
    },
  };
}

describe("[STU-103] Variant nodes on the canvas (build log row 304)", () => {
  test("a Variant's arms are the control, then each variant by name, with their pointers", () => {
    const arms = variantArms(page().root.children[0]);
    expect(arms.map((a) => [a.name, a.pointer, a.node["id"]])).toEqual([
      [null, "/control", "c"],
      ["a/b", "/variants/a~1b", "ab"],
      ["wide", "/variants/wide", "w"],
    ]);
    expect(variantArms(text("t", "x"))).toEqual([]);
  });

  test("Layers indexes every arm under its Variant, each named by its variant", () => {
    const index = indexNodes([page().root as unknown as Node]);
    expect(index.children.get("v")).toEqual(["c", "ab", "w"]);
    expect(index.parent.get("w1")).toBe("w");
    expect([index.slot.get("c"), index.slot.get("w")]).toEqual(["control", "wide"]);
  });

  test("a node inside an arm is found and edited in place; an arm's own root is never moved or wrapped", () => {
    const doc = page();
    expect(pointerOf(doc, "w2")).toBe("/root/children/0/variants/wide/children/1");
    expect(pointerOf(doc, "c")).toBe("/root/children/0/control");
    expect(findNode(doc.root, "w2")?.parent?.["id"]).toBe("w");
    wrapRecipe("w2", "box", "Column")(doc.root);
    expect(pointerOf(doc, "w2")).toBe("/root/children/0/variants/wide/children/1/children/0");
    const before = JSON.stringify(doc);
    wrapRecipe("c", "box2", "Column")(doc.root);
    expect(JSON.stringify(doc)).toBe(before);
  });

  test("the canvas draws the control, or the arm picked in the inspector", () => {
    const drawnTexts = (picks: ReadonlyMap<string, string>) => {
      const root = applyConditions(page().root as unknown as Node, new Map(), {}, undefined, picks) as unknown as {
        children: { slots?: { child?: { id: string } } }[];
      };
      return root.children[0]?.slots?.child?.id;
    };
    expect(drawnTexts(new Map())).toBe("c");
    expect(drawnTexts(new Map([["v", "arm:wide"]]))).toBe("w");
    expect(drawnTexts(new Map([["v", "arm:missing"]]))).toBe("c");
  });

  test("wrapping a node makes it the control and copies it with fresh IDs for each variant", () => {
    let n = 0;
    const fresh = () => `n${++n}`;
    const node = { id: "card", type: "Card", slots: { child: text("t", "Hi") } };
    const v = variantOf(node, "heroLayout", ["wide", "tall"], fresh);
    expect(v).toMatchObject({ type: "Variant", experiment: "heroLayout", control: node });
    const arms = variantArms(v);
    expect(arms.map((a) => a.name)).toEqual([null, "tall", "wide"]);
    const ids = JSON.stringify(v).match(/"id":"[^"]+"/g) ?? [];
    expect(new Set(ids).size).toBe(ids.length);
    expect(arms[2]?.node).toMatchObject({ type: "Card", slots: { child: { type: "Text", props: { data: "Hi" } } } });
  });
});
