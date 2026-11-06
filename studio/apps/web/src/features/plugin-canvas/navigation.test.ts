// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { deriveGhosts, paramsLine } from "./ghosts.ts";
import {
  declaredParams,
  flowDepths,
  flowLayout,
  linkParams,
  missingParams,
  paramText,
  paramValue,
  setLinkParamRecipe,
  setTransitionRecipe,
  transitionOf,
} from "./navigation.ts";

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

  test("arrange by flow: a column per step from the entry, unreached screens last", () => {
    const edges = new Map([
      ["home", ["list", "settings"]],
      ["list", ["detail"]],
    ]);
    const ids = ["home", "list", "detail", "settings", "orphan"];
    expect(Object.fromEntries(flowDepths(ids, "home", edges))).toEqual({
      home: 0,
      list: 1,
      settings: 1,
      detail: 2,
      orphan: 3,
    });
    const at = flowLayout(ids, "home", edges, () => ({ w: 400, h: 800 }), 100);
    expect(at.get("home")).toEqual({ x: 0, y: 0 });
    expect(at.get("list")).toEqual({ x: 500, y: 0 });
    expect(at.get("settings")).toEqual({ x: 500, y: 900 });
    expect(at.get("detail")).toEqual({ x: 1000, y: 0 });
    expect(at.get("orphan")).toEqual({ x: 1500, y: 0 });
  });
});
