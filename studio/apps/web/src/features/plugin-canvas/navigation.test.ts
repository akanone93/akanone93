// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { deriveGhosts, paramsLine } from "./ghosts.ts";
import {
  declaredParams,
  linkParams,
  missingParams,
  paramText,
  paramValue,
  setLinkParamRecipe,
  setTransitionRecipe,
  transitionOf,
} from "./navigation.ts";
import { arrangeScreens, placeScreens, SCREEN_GAP, SCREEN_ROW_GAP } from "./positions.ts";

const page = {
  params: [
    { name: "orderId", type: "string", required: true },
    { name: "tab", type: "int", required: true, default: 0 },
    { name: "highlight", type: "bool" },
  ],
};
const root = {
  id: "list",
  type: "ListTile",
  events: { onTap: { steps: [{ id: "navigate", action: "navigate", input: { route: "order" } }] } },
};
const link = { event: "onTap", step: 0, route: "order" };

describe("PC-28 navigation [NAV-001] [NAV-010]", () => {
  test("a link's parameters are typed as the page declares them, and the required ones it misses are named", () => {
    const declared = declaredParams(page);
    expect(declared.map((d) => d.name)).toEqual(["orderId", "tab", "highlight"]);
    expect(missingParams(declared, {})).toEqual(["orderId"]);
    expect(paramValue("int", "3")).toBe(3);
    expect(paramValue("bool", "true")).toBe(true);
    expect(paramValue("string", '"A-1"')).toBe("A-1");
    expect(paramValue("string", "item.id")).toEqual({ $expr: "item.id" });
    expect(paramValue("string", " ")).toBeUndefined();
    const tree = structuredClone(root);
    setLinkParamRecipe("list", link, "orderId", { $expr: "item.id" })(tree);
    setLinkParamRecipe("list", link, "tab", 2)(tree);
    expect(linkParams(tree, "list", link)).toEqual({ orderId: { $expr: "item.id" }, tab: 2 });
    expect(paramText(linkParams(tree, "list", link)["orderId"])).toBe("item.id");
    setLinkParamRecipe("list", link, "orderId", undefined)(tree);
    setLinkParamRecipe("list", link, "tab", undefined)(tree);
    expect(tree.events.onTap.steps[0]?.input).toEqual({ route: "order" });
  });

  test("a lane's title shows the parameters its link passes", () => {
    expect(paramsLine({ orderId: { $expr: "item.id" }, tab: 2, q: "x" })).toBe('orderId: item.id, tab: 2, q: "x"');
    const graph = deriveGhosts({
      plugin: "p",
      screens: [
        {
          id: "a",
          route: "list",
          title: "Orders",
          targets: ["order"],
          targetParams: [{ orderId: { $expr: "item.id" } }],
        },
        { id: "b", route: "order", title: "Order", targets: [] },
      ],
      appScreens: [],
      nativeRoutes: [],
    });
    expect(graph.edges[0]?.title).toBe("Orders → Order (orderId: item.id)");
  });

  test("a page's transition is read and written; platform clears it", () => {
    const p: Record<string, unknown> = {};
    expect(transitionOf(p)).toBe("platform");
    setTransitionRecipe("slideUp")(p);
    expect(transitionOf(p)).toBe("slideUp");
    setTransitionRecipe("platform")(p);
    expect(p).toEqual({});
  });

  test("[STU-050] arrange by flow: from the entry along the links, unreached screens apart, nothing overlaps (#241)", () => {
    const edges = new Map([
      ["home", ["list", "settings"]],
      ["list", ["detail"]],
    ]);
    const ids = ["home", "list", "detail", "settings", "orphan"];
    const size = () => ({ w: 400, h: 800 });
    const at = arrangeScreens(ids, "home", edges, size);
    const p = (id: string) => at.get(id) ?? { x: Number.NaN, y: Number.NaN };
    // Each step of the flow lies after the one it comes from, along the flow.
    const after = (a: string, b: string) => p(b).x > p(a).x || p(b).y > p(a).y;
    expect(after("home", "list")).toBe(true);
    expect(after("home", "settings")).toBe(true);
    expect(after("list", "detail")).toBe(true);
    // No two screens closer than the gaps.
    for (const a of ids) {
      for (const b of ids) {
        const apart =
          a === b ||
          p(a).x + 400 + SCREEN_GAP <= p(b).x ||
          p(b).x + 400 + SCREEN_GAP <= p(a).x ||
          p(a).y + 800 + SCREEN_ROW_GAP <= p(b).y ||
          p(b).y + 800 + SCREEN_ROW_GAP <= p(a).y;
        expect(apart).toBe(true);
      }
    }
    // The same screens and links always give the same places.
    expect(arrangeScreens(ids, "home", edges, size)).toEqual(at);
  });

  test("[STU-050] screens without a saved place are arranged to the right of the saved ones", () => {
    const saved = new Map([["home", { x: 100, y: 50 }]]);
    const at = placeScreens(["home", "list"], "home", new Map([["home", ["list"]]]), () => ({ w: 400, h: 800 }), saved);
    expect(at.get("home")).toEqual({ x: 100, y: 50 });
    expect(at.get("list")).toEqual({ x: 100 + 400 + SCREEN_GAP, y: 50 });
  });
});
