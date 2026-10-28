// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { EditorView } from "@codemirror/view";
import type { I18n } from "@lingui/core";
import { type PxlBackend, PxlService } from "@plux/validator";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { produce } from "immer";
import { panelMessages } from "./panels.messages.ts";
import {
  editorKind,
  isAccessibility,
  nodesById,
  pointerOf,
  setPropRecipe,
  setSemanticsRecipe,
  sharedProps,
  sharedValue,
  tokenPaths,
  valueAt,
} from "./properties-model.ts";
import { type EditorProps, ValueEditor } from "./right-dock.tsx";

afterEach(cleanup);

const m = panelMessages({ _: (d: { message: string }) => d.message } as unknown as I18n);
const none = new Set<string>();
const root = () => ({
  id: "r",
  children: [
    { id: "a", type: "Text", props: { data: "Hi", maxLines: 2 }, responsive: { medium: { maxLines: 4 } } },
    { id: "b", type: "Text", props: { data: "Yo", maxLines: 2 } },
    { id: "c", type: "Container", props: { width: 10 } },
  ],
});
interface TestNode {
  id: string;
  type?: string;
  props: { maxLines?: number; data?: string };
  responsive?: Record<string, object>;
  semantics?: object;
}
const apply = (r: (root: unknown) => void) => produce(root(), (d) => r(d)) as unknown as { children: TestNode[] };

function editor(over: Partial<EditorProps>) {
  const changes: unknown[] = [];
  const props: EditorProps = {
    type: "double",
    name: "width",
    value: undefined,
    mixed: false,
    unit: "dp",
    readOnly: false,
    sources: { tokens: [], keys: [], check: undefined },
    m,
    onChange: (v) => changes.push(v),
    onEnd: () => undefined,
    ...over,
  };
  render(<ValueEditor {...props} />);
  return changes;
}

describe("STU-121 panels: editors generate the right recipe", () => {
  test("property types pick their editors", () => {
    expect(editorKind("double", none)).toBe("number");
    expect(editorKind("bool?", none)).toBe("bool");
    expect(editorKind("color", none)).toBe("color");
    expect(editorKind("EdgeInsets", none)).toBe("insets");
    expect(editorKind("Alignment", none)).toBe("alignment");
    expect(editorKind("list<string>", none)).toBe("list");
    expect(editorKind("Clip", new Set(["Clip"]))).toBe("enum");
    expect(editorKind("BoxDecoration", none)).toBe("object");
  });

  test("a base edit writes props; an override writes responsive.<size class>", () => {
    expect(apply(setPropRecipe("a", "compact", "maxLines", 3)).children[0]?.props.maxLines).toBe(3);
    const out = apply(setPropRecipe("b", "expanded", "maxLines", 9)).children[1] as { responsive?: unknown };
    expect(out.responsive).toEqual({ expanded: { maxLines: 9 } });
  });

  test("reset to base removes the override only", () => {
    const out = apply(setPropRecipe("a", "medium", "maxLines", undefined)).children[0] as unknown as {
      props: { maxLines: number };
      responsive: { medium: object };
    };
    expect(out.responsive.medium).toEqual({});
    expect(out.props.maxLines).toBe(2);
  });

  test("an override is found and flagged; expanded cascades from medium", () => {
    const a = nodesById([root()]).get("a") ?? {};
    expect(valueAt(a, "maxLines", "medium")).toEqual({ value: 4, overridden: true });
    expect(valueAt(a, "maxLines", "expanded")).toEqual({ value: 4, overridden: false });
    expect(valueAt(a, "maxLines", "compact")).toEqual({ value: 2, overridden: false });
  });

  test("semantics edits create and prune the semantics object", () => {
    const set = apply(setSemanticsRecipe("c", "label", "Box")).children[2] as { semantics: object };
    expect(set.semantics).toEqual({ label: "Box" });
    const cleared = produce(set, () => undefined);
    expect(cleared).toBeDefined();
  });

  test("pointers locate a node for problem filtering; a11y codes are PLX-14xx", () => {
    expect(pointerOf({ root: root() }, "b")).toBe("/root/children/1");
    expect(isAccessibility({ code: "PLX-1401", severity: "warning", file: "", path: "", message: "" })).toBe(true);
    expect(isAccessibility({ code: "PLX-1001", severity: "error", file: "", path: "", message: "" })).toBe(false);
  });

  test("token paths come from W3C token trees", () => {
    expect(tokenPaths({ color: { primary: { $value: "#fff" }, $type: "color" } })).toEqual(["color.primary"]);
  });
});

describe("STU-121 panels: Mixed", () => {
  const nodes = [...nodesById([root()]).values()].filter((n) => n.type === "Text");
  test("equal values are shared, different ones are Mixed (F5)", () => {
    expect(sharedValue(nodes, "maxLines", "compact")).toEqual({ mixed: false, value: 2, overridden: false });
    expect(sharedValue(nodes, "data", "compact")).toEqual({ mixed: true });
  });
  test("only properties every selected type has are shown", () => {
    const names = sharedProps(["Text", "Container"]).map((p) => p.name);
    expect(names).not.toContain("data");
    expect(sharedProps(["Text", "Text"]).map((p) => p.name)).toContain("data");
  });
  test("the editor shows the Mixed placeholder", () => {
    editor({ type: "double", mixed: true });
    expect((screen.getByLabelText("width") as HTMLInputElement).placeholder).toBe("Mixed");
  });
});

describe("STU-121 panels: editors", () => {
  test("a number shows its unit and reports a parsed number", () => {
    const changes = editor({ value: 4 });
    expect(screen.getByText("dp")).toBeTruthy();
    fireEvent.change(screen.getByLabelText("width"), { target: { value: "12.5" } });
    expect(changes).toEqual([12.5]);
  });
  test("edge insets edit one side and keep the others", () => {
    const changes = editor({ type: "EdgeInsets", name: "padding", value: { left: 1 } });
    fireEvent.change(screen.getByLabelText("padding Top"), { target: { value: "8" } });
    expect(changes).toEqual([{ left: 1, top: 8 }]);
  });
  test("a bool reports its checked state", () => {
    const changes = editor({ type: "bool", name: "softWrap", value: false });
    fireEvent.click(screen.getByLabelText("softWrap"));
    expect(changes).toEqual([true]);
  });
  test("a list adds an item", () => {
    const changes = editor({ type: "list<string>", name: "tags", value: ["a"] });
    fireEvent.click(screen.getByText("Add item"));
    expect(changes).toEqual([["a", ""]]);
  });
  test("read-only fields change nothing", () => {
    editor({ value: 4, readOnly: true });
    expect((screen.getByLabelText("width") as HTMLInputElement).readOnly).toBe(true);
  });
  test("a text prop offers token-free literal, translation key and PXL modes", () => {
    editor({
      type: "string",
      name: "data",
      value: "Hi",
      sources: { tokens: [], keys: [{ id: "k1", key: "home.title" }], check: undefined },
    });
    expect((screen.getByLabelText("data") as HTMLInputElement).value).toBe("Hi");
  });

  test("a PXL expression opens the inline editor with the language service and reports its edits", async () => {
    const backend: PxlBackend = { pxl: async () => ({ type: "int", diagnostics: [] }) };
    const changes = editor({
      type: "string",
      name: "data",
      value: { $expr: "page.months" },
      sources: {
        tokens: [],
        keys: [],
        check: undefined,
        pxl: { service: new PxlService(backend), scope: async () => "{}" },
      },
    });
    await waitFor(() => expect(document.querySelector(".cm-content") !== null).toBe(true), { timeout: 3000 });
    const content = screen.getByLabelText("data: PXL expression");
    expect(content.textContent).toBe("page.months");
    const view = EditorView.findFromDOM(content.closest(".cm-editor") as HTMLElement) as EditorView;
    view.dispatch({ changes: { from: 11, insert: "+1" }, userEvent: "input.type" });
    expect(changes).toEqual([{ $expr: "page.months+1" }]);
    await waitFor(() => expect(screen.getByText("Result type: {type}")).toBeTruthy(), { timeout: 2000 });
  });

  test("without the language service a PXL expression stays a plain field", () => {
    editor({ type: "string", name: "data", value: { $expr: "page.months" } });
    expect((screen.getByLabelText("data: PXL expression") as HTMLInputElement).value).toBe("page.months");
  });
});
