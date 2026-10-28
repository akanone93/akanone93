// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterAll, beforeAll, describe, expect, test } from "bun:test";
import { layoutScreen, type ScreenLayout, type WidgetNode } from "@plux/layout";
import type { CanvasKitParagraphEngine } from "../../../../../packages/layout/src/text/canvaskit-engine.ts";
import { loadCanvasKit, testParagraphEngine } from "../../../../../packages/layout/test-support/canvaskit.ts";
import type { MockState } from "../preview/switches.ts";
import { bindMocks, evaluateMockBinding } from "./mock-bindings.ts";
import type { Node } from "./plugin-model.ts";
import { screenStyle } from "./scene.ts";

let engine: CanvasKitParagraphEngine;
beforeAll(async () => {
  engine = testParagraphEngine(await loadCanvasKit());
});
afterAll(() => engine.dispose());

const mocks = new Map<string, unknown>([
  ["orders", { customer: { name: "Harbour Trading" }, lines: [{ sku: "A-1" }, { sku: "B-2" }] }],
  ["tags", ["new", "paid"]],
]);
const states = (s: Record<string, MockState>) => s;

describe("[DAT-080] data bindings take the selected mock state", () => {
  test("success reads the mock by path and index", () => {
    expect(evaluateMockBinding("data.orders.value.customer.name", mocks, {})).toBe("Harbour Trading");
    expect(evaluateMockBinding("data.orders.value.lines[1].sku", mocks, {})).toBe("B-2");
    expect(evaluateMockBinding("data.orders.value?.customer?.name", mocks, {})).toBe("Harbour Trading");
    expect(evaluateMockBinding("data.orders.loading", mocks, {})).toBe(false);
  });

  test("loading, empty and error change value, loading and error", () => {
    const loading = states({ orders: "loading" });
    expect(evaluateMockBinding("data.orders.loading", mocks, loading)).toBe(true);
    expect(evaluateMockBinding("data.orders.value.customer.name", mocks, loading)).toBeNull();
    expect(evaluateMockBinding("data.tags.value", mocks, states({ tags: "empty" }))).toEqual([]);
    expect(evaluateMockBinding("data.orders.error", mocks, states({ orders: "error" }))).toBe(true);
  });

  test("a fallback literal fills what the path does not reach", () => {
    expect(evaluateMockBinding('data.orders.value.missing ?? "n/a"', mocks, {})).toBe("n/a");
  });

  test("expressions that are not plain data paths are left alone", () => {
    expect(evaluateMockBinding("page.amount + 1", mocks, {})).toBeUndefined();
    expect(evaluateMockBinding("data.unknown.value", mocks, {})).toBeUndefined();
  });

  const tree: Node = {
    id: "col",
    type: "Column",
    children: [
      { id: "name", type: "Text", props: { data: { $expr: "data.orders.value.customer.name" } } as never },
      { id: "wait", type: "Text", props: { data: "Please wait", visible: { $expr: "data.orders.loading" } } as never },
      { id: "code", type: "Text", props: { data: { $expr: "page.code" } } as never },
    ],
  };
  const kids = (n: Node | undefined) => (n?.children ?? []).map((c) => c.props as unknown as Record<string, unknown>);

  test("bound props become the mock values, per state", () => {
    const [name, wait, code] = kids(bindMocks(tree, mocks, {}));
    expect(name?.["data"]).toBe("Harbour Trading");
    expect(wait?.["visible"]).toBe(false);
    expect(code?.["data"]).toEqual({ $expr: "page.code" });
    const [loadingName, loadingWait] = kids(bindMocks(tree, mocks, states({ orders: "loading" })));
    expect("data" in (loadingName ?? {})).toBe(false);
    expect(loadingWait?.["visible"]).toBe(true);
  });

  test("a tree without data sources is returned as it is", () => {
    expect(bindMocks(tree, new Map(), {})).toBe(tree);
  });

  test("the canvas layout shows the mock: a bound Text has width, a loading one has none", () => {
    const layout = (s: Record<string, MockState>): ScreenLayout =>
      layoutScreen(bindMocks(tree, mocks, s) as unknown as WidgetNode, { engine, theme: screenStyle.theme });
    const width = (l: ScreenLayout) => l.boxes.find((b) => b.id === "name")?.w ?? 0;
    expect(width(layout({}))).toBeGreaterThan(0);
    expect(width(layout({ orders: "loading" }))).toBe(0);
  });
});
