// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { expandLists, MAX_DRAWN_ITEMS, sourceOfItems, templateIdOf } from "./list-preview.ts";
import { emptyScope } from "./mock-bindings.ts";
import type { Node } from "./plugin-model.ts";
import { nodeIdOfHit } from "./select/tree.ts";

type Tree = {
  children?: Tree[];
  slots?: Record<string, Tree>;
  props?: Record<string, unknown>;
  id?: string;
  type?: string;
};
const tile = {
  id: "tile",
  type: "ListTile",
  slots: {
    title: { id: "t", type: "Text", props: { data: { $expr: "item.name" } } },
    subtitle: { id: "s", type: "Text", props: { data: { $expr: "index" } } },
  },
};
const list = (props: object, slots: object = {}): Node =>
  ({ id: "list", type: "ListView", props, slots: { item: tile, ...slots } }) as unknown as Node;
const mocks = new Map<string, unknown>([["orders", [{ name: "Ada" }, { name: "Grace" }, { name: "Linus" }]]]);
const draw = (root: Node, states = {}, picked = new Map()) =>
  expandLists(root, mocks, states, emptyScope, picked) as unknown as Tree;

describe("PC-24 lists from data [WGT-012] [DAT-011]", () => {
  test("the item template is repeated per item with item and index bound; copies map back to the template", () => {
    const drawn = draw(list({ items: { $expr: "data.orders.value" } }));
    expect(drawn.slots?.["item"]).toBeUndefined();
    expect(drawn.children?.map((c) => c.id)).toEqual(["tile", "tile~item1", "tile~item2"]);
    expect(drawn.children?.[1]?.slots?.["title"]?.props?.["data"]).toBe("Grace");
    expect(drawn.children?.[2]?.slots?.["subtitle"]?.props?.["data"]).toBe("2");
    expect(templateIdOf("t~item2")).toBe("t");
    expect(nodeIdOfHit("screen/t~item2#paint0")).toBe("t");
    expect(sourceOfItems({ $expr: "data.orders.value.lines" })).toBe("orders");
  });

  test("literal items are repeated too, up to the drawn maximum; unknown items keep the template once", () => {
    const many = Array.from({ length: 40 }, (_, i) => ({ name: `n${i}` }));
    expect(draw(list({ items: many })).children).toHaveLength(MAX_DRAWN_ITEMS);
    const unknown = list({ items: { $expr: "flags.rows" } });
    expect(draw(unknown)).toBe(unknown as unknown as Tree);
  });

  test("loading, error and empty draw the list's own slot, else a default view; hasMore adds a progress row", () => {
    const loading = draw(list({ items: { $expr: "data.orders.value" } }), { orders: "loading" });
    expect(loading.children?.[0]?.id).toBe("list~pad");
    const designed = { id: "e", type: "Text", props: { data: "No orders" } };
    const empty = draw(list({ items: { $expr: "data.orders.value" } }, { empty: designed }), { orders: "empty" });
    expect(empty.children).toEqual([designed]);
    expect(empty.slots).toEqual({});
    const error = draw(list({ items: [], status: "error" }));
    expect(JSON.stringify(error)).toContain("error_outline");
    const more = draw(list({ items: [{ name: "a" }], hasMore: true }));
    expect(more.children?.map((c) => c.id)).toEqual(["tile", "list~pad"]);
  });

  test("a state picked in the inspector wins over the data's", () => {
    const picked = new Map([["list", "empty" as const]]);
    const drawn = draw(list({ items: { $expr: "data.orders.value" } }), {}, picked);
    expect(JSON.stringify(drawn)).toContain("inbox");
  });
});
