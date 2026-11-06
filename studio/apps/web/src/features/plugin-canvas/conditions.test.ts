// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { applyConditions, evaluateCondition } from "./conditions.ts";
import type { DesignScope } from "./mock-bindings.ts";
import type { Node } from "./plugin-model.ts";

type Tree = Record<string, unknown> & {
  children?: Tree[];
  slots?: Record<string, Tree>;
  props?: Record<string, unknown>;
};
const scope: DesignScope = {
  params: { tab: "paid" },
  page: { count: 3, items: [], name: "Ada", vip: true },
  plugin: {},
  app: {},
};
const ev = (e: string) => evaluateCondition(e, new Map(), {}, scope);
const text = (id: string) => ({ id, type: "Text", props: { data: id } });
const draw = (root: object, picks = new Map<string, string>()) =>
  applyConditions(root as Node, new Map(), {}, scope, picks) as unknown as Tree;

describe("PC-25 conditions on the canvas [SCH-023]", () => {
  test("comparisons, negation, emptiness and && / || work out at design time; anything else is unknown", () => {
    expect(ev("page.count > 0")).toBe(true);
    expect(ev("page.count >= 4")).toBe(false);
    expect(ev('params.tab == "paid"')).toBe(true);
    expect(ev("!page.vip")).toBe(false);
    expect(ev("page.items.isEmpty")).toBe(true);
    expect(ev("page.items.isNotEmpty || page.vip")).toBe(true);
    expect(ev("page.vip && page.count < 2")).toBe(false);
    expect(ev("flags.beta")).toBeUndefined();
    expect(ev("(page.count > 1)")).toBeUndefined();
    expect(ev("flags.beta || page.vip")).toBe(true);
  });

  test("an If draws the branch its condition takes, or the one picked", () => {
    const root = {
      id: "if",
      type: "If",
      props: { condition: { $expr: "page.count == 0" } },
      // biome-ignore lint/suspicious/noThenProperty: an If's slot is named `then` in the widget registry
      slots: { then: text("a"), else: text("b") },
    };
    expect(draw(root).props).toEqual({ condition: false });
    expect(draw(root, new Map([["if", "then"]])).props).toEqual({ condition: true });
    const unknown = { ...root, props: { condition: { $expr: "flags.x" } } };
    expect(draw(unknown).props).toEqual({ condition: true });
  });

  test("a Match takes its value's case or the one picked; otherwise picks the otherwise slot", () => {
    const root = {
      id: "m",
      type: "Match",
      props: { value: { $expr: "params.tab" }, cases: ["open", "paid"] },
      children: [text("o"), text("p")],
      slots: { otherwise: text("x") },
    };
    expect(draw(root).props?.["value"]).toBe("paid");
    expect(draw(root, new Map([["m", "case:0"]])).props?.["value"]).toBe("open");
    expect(draw(root, new Map([["m", "otherwise"]])).props?.["value"]).toBe("~otherwise");
  });

  test("visible false leaves a widget out unless it is picked shown; a ForEach repeats its template", () => {
    const col = {
      id: "col",
      type: "Column",
      props: { spacing: 8 },
      children: [
        { ...text("hidden"), visible: { $expr: "page.count > 5" } },
        { ...text("shown"), visible: true },
        {
          id: "fe",
          type: "ForEach",
          props: { items: [{ n: "a" }, { n: "b" }] },
          slots: { item: { id: "t", type: "Text", props: { data: { $expr: "item.n" } } } },
        },
      ],
    };
    const drawn = draw(col);
    expect(drawn.children?.map((c) => c["id"])).toEqual(["shown", "fe"]);
    expect(drawn.children?.[1]).toMatchObject({ type: "Flex", props: { direction: "vertical", spacing: 8 } });
    expect(drawn.children?.[1]?.children?.map((c) => c.props?.["data"])).toEqual(["a", "b"]);
    expect(draw(col, new Map([["hidden", "shown"]])).children).toHaveLength(3);
    const plain = { id: "c", type: "Column", children: [text("a")] };
    expect(draw(plain)).toBe(plain as unknown as Tree);
  });
});
