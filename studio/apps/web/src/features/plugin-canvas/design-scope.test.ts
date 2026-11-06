// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { bindingMarks, shortValue } from "./binding-marks.ts";
import { designScopes, paramMocks, stateDefaults } from "./design-scope.ts";
import { namedMocks } from "./look.ts";
import { bindMocks, boundPropsOf, evaluateMockBinding } from "./mock-bindings.ts";
import type { Node } from "./plugin-model.ts";

const doc = (json: object) => ({ kind: (json as { kind: string }).kind, content: new Uint8Array(), parsed: json });
const page = {
  kind: "page",
  id: "p1",
  params: [
    { name: "orderId", type: "string", mock: "A-1042" },
    { name: "tab", type: "int", default: 2 },
    { name: "none", type: "string" },
  ],
  state: [
    { id: "s1", name: "count", type: "int", default: 3 },
    { id: "s2", name: "label", type: "string", computed: "page.count + 1" },
    { id: "s3", name: "user", type: "map", default: { name: "Ada", tags: ["pro"] } },
  ],
};
const documents = [
  doc(page),
  doc({ kind: "plugin", id: "pl", state: [{ id: "a", name: "theme", type: "string", default: "ocean" }] }),
  doc({ kind: "app", id: "ap", state: [{ id: "b", name: "currency", type: "string", default: "EUR" }] }),
];

describe("PC-23 design-time values of bound widgets [STA-001] [DAT-080]", () => {
  test("a page's scope: each parameter's mock else default, each state entry's default; computed ones wait", () => {
    expect(paramMocks(page.params)).toEqual({ orderId: "A-1042", tab: 2 });
    expect(stateDefaults(page.state)).toEqual({ count: 3, user: { name: "Ada", tags: ["pro"] } });
    expect(designScopes(documents).get("p1")).toEqual({
      params: { orderId: "A-1042", tab: 2 },
      page: { count: 3, user: { name: "Ada", tags: ["pro"] } },
      plugin: { theme: "ocean" },
      app: { currency: "EUR" },
    });
  });

  test("paths into params, page, plugin and app state resolve with steps and fallbacks; unknown ones stay", () => {
    const scope = designScopes(documents).get("p1");
    const ev = (expr: string) => evaluateMockBinding(expr, new Map(), {}, scope);
    expect(ev("params.orderId")).toBe("A-1042");
    expect(ev("page.user.name")).toBe("Ada");
    expect(ev("page.user?.tags[0]")).toBe("pro");
    expect(ev("app.currency")).toBe("EUR");
    expect(ev("plugin.theme")).toBe("ocean");
    expect(ev('page.user.missing ?? "–"')).toBe("–");
    expect(ev("page.label")).toBeUndefined();
    expect(ev('params.orderId + "!"')).toBeUndefined();
  });

  test("bound widgets are drawn with their values and marked with what they bind", () => {
    const root = {
      id: "col",
      type: "Column",
      children: [
        { id: "t", type: "Text", props: { data: { $expr: "params.orderId" } } },
        {
          id: "b",
          type: "Badge",
          props: { label: { $expr: "page.count" }, color: { $expr: "flags.x" } },
          visible: { $expr: "page.count > 0" },
        },
      ],
    } as unknown as Node;
    const scopes = designScopes(documents);
    const drawn = bindMocks(root, new Map(), {}, scopes.get("p1")) as unknown as { children: { props: object }[] };
    expect(drawn.children[0]?.props).toEqual({ data: "A-1042" });
    expect(
      boundPropsOf(root)
        .get("b")
        ?.map((b) => b.prop),
    ).toEqual(["label", "color", "visible"]);
    const marks = bindingMarks([{ id: "p1", root }], new Map(), {}, scopes);
    expect(marks.get("t")).toEqual([{ prop: "data", expr: "params.orderId", shown: '"A-1042"' }]);
    expect(marks.get("b")?.[1]).toEqual({ prop: "color", expr: "flags.x" });
    expect(shortValue([1, 2, 3])).toBe("[3]");
    expect(shortValue({ a: 1, b: 2, c: 3, d: 4 })).toBe("{a, b, c, …}");
    expect(shortValue("x".repeat(40))).toHaveLength(30);
  });

  test("a data source is bound by its name, as expressions write it, with the state set for its id", () => {
    const sources = [{ id: "01c0c450-6c00-7000-8a1d-0000000f0003", name: "orders", mock: [{ customer: "Harbour" }] }];
    const named = namedMocks(sources, { "01c0c450-6c00-7000-8a1d-0000000f0003": "loading" });
    expect(evaluateMockBinding("data.orders.loading", named.mocks, named.states)).toBe(true);
    const ok = namedMocks(sources, {});
    expect(evaluateMockBinding("data.orders.value[0].customer", ok.mocks, ok.states)).toBe("Harbour");
  });
});
