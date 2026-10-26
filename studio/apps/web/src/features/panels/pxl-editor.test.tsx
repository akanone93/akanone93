// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { startCompletion } from "@codemirror/autocomplete";
import { EditorView } from "@codemirror/view";
import { type PxlBackend, PxlService } from "@plux/validator";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { Suspense } from "react";
import type { PxlLanguage } from "../problems/use-validation.ts";
import PxlEditor, { type PxlEditorSource } from "./pxl-editor.tsx";
import { PXL_SCOPE_TTL_MS, pxlSource } from "./pxl-source.ts";

afterEach(cleanup);

/** A backend that records what the editor asked and answers like the Go module would. */
function backend(): { readonly seen: string[]; readonly backend: PxlBackend } {
  const seen: string[] = [];
  return {
    seen,
    backend: {
      pxl: async (op, _scope, src, offset) => {
        seen.push(`${op}:${src}:${offset}`);
        switch (op) {
          case "check":
            return src.includes("nope")
              ? {
                  type: "",
                  diagnostics: [{ code: "PLX-9", severity: "error", message: "no such field", start: 5, end: 9 }],
                }
              : { type: "int", diagnostics: [] };
          case "complete":
            return {
              from: 0,
              to: src.length,
              items: [{ label: "months", kind: "field", detail: "int", insert: "months" }],
            };
          default:
            return null;
        }
      },
    },
  };
}

function mount(over: { value?: string; readOnly?: boolean; scope?: string | undefined } = {}) {
  const { seen, backend: b } = backend();
  const changes: string[] = [];
  const types: string[] = [];
  let blurs = 0;
  const source: PxlEditorSource = {
    service: new PxlService(b),
    scope: async () => ("scope" in over ? over.scope : "{}"),
  };
  render(
    <Suspense fallback={null}>
      <PxlEditor
        value={over.value ?? ""}
        label="data: PXL expression"
        readOnly={over.readOnly ?? false}
        source={source}
        onChange={(v) => changes.push(v)}
        onBlur={() => blurs++}
        onType={(t) => types.push(t)}
      />
    </Suspense>,
  );
  const content = screen.getByLabelText("data: PXL expression");
  const view = EditorView.findFromDOM(content.closest(".cm-editor") as HTMLElement) as EditorView;
  return { seen, changes, types, content, view, blurs: () => blurs };
}

describe("the inline PXL editor", () => {
  test("shows the expression under an accessible name, on one line", () => {
    const { content, view } = mount({ value: "page.months" });
    expect(content.textContent).toBe("page.months");
    expect(content.getAttribute("aria-multiline")).toBe("false");
    view.dispatch({ changes: { from: 0, insert: "a\n" } });
    expect(view.state.doc.toString()).toBe("page.months");
  });

  test("reports each edit but not the value it was given", () => {
    const { view, changes } = mount({ value: "a" });
    view.dispatch({ changes: { from: 1, insert: "b" }, userEvent: "input.type" });
    expect(changes).toEqual(["ab"]);
  });

  test("starts with the value of its owner", () => {
    const { view } = mount({ value: "a" });
    expect(view.state.doc.toString()).toBe("a");
  });

  test("a read-only editor takes no input", () => {
    const { view, content } = mount({ value: "x", readOnly: true });
    expect(view.state.readOnly).toBe(true);
    expect(content.getAttribute("contenteditable")).toBe("false");
  });

  test("marks the range of an error from the language service and reports the result type", async () => {
    const { view, types } = mount({ value: "page.nope" });
    view.dispatch({ changes: { from: 9, insert: " " }, userEvent: "input.type" });
    await waitFor(() => expect(document.querySelector(".cm-lintRange-error")).not.toBeNull(), { timeout: 2000 });
    expect(document.querySelector(".cm-lintRange-error")?.textContent).toBe("nope");
    expect(types.at(-1)).toBe("");
  });

  test("reports the type of a valid expression", async () => {
    const { view, types } = mount({ value: "1" });
    view.dispatch({ changes: { from: 1, insert: "2" }, userEvent: "input.type" });
    await waitFor(() => expect(types.at(-1)).toBe("int"), { timeout: 2000 });
  });

  test("offers completions from the language service", async () => {
    const { view, seen } = mount({ value: "pa" });
    view.dispatch({ selection: { anchor: 2 } });
    startCompletion(view);
    await waitFor(() => expect(seen.some((s) => s.startsWith("complete:pa:2"))).toBe(true), { timeout: 2000 });
  });

  test("says nothing where the page has no expression site", async () => {
    const { view, seen } = mount({ value: "pa", scope: undefined });
    startCompletion(view);
    await new Promise((r) => setTimeout(r, 50));
    expect(seen).toEqual([]);
    expect(document.querySelector(".cm-tooltip-autocomplete")).toBeNull();
  });

  test("tells its owner when the editor loses focus", () => {
    const { content, blurs } = mount({ value: "x" });
    fireEvent.blur(content);
    expect(blurs()).toBe(1);
  });
});

describe("the scope of a prop's expression", () => {
  const page = { root: { id: "r", type: "Text", props: { data: "Hi" } } };
  const language = (calls: string[]): PxlLanguage => ({
    service: new PxlService(backend().backend),
    sites: async (file, data) => {
      calls.push(`${file}:${(JSON.parse(data) as typeof page).root.props.data as unknown as string}`);
      return { sites: [{ pointer: "/root/props/data", env: 0 }], envs: [{ roots: { page: "int" } }] };
    },
  });

  test("asks the validator for the page with an expression at the prop", async () => {
    const calls: string[] = [];
    const source = pxlSource(language(calls), "a.page.json", page, "r", "compact", "data", new Map());
    expect(JSON.parse((await source?.scope()) ?? "{}")).toEqual({ roots: { page: "int" } });
    expect(calls).toHaveLength(1);
    expect(calls[0]).toContain("a.page.json");
  });

  test("shares one answer between requests until it ages", async () => {
    const calls: string[] = [];
    let now = 1000;
    const cache = new Map();
    const make = () => pxlSource(language(calls), "a.page.json", page, "r", "compact", "data", cache, () => now);
    await make()?.scope();
    await make()?.scope();
    expect(calls).toHaveLength(1);
    now += PXL_SCOPE_TTL_MS + 1;
    await make()?.scope();
    expect(calls).toHaveLength(2);
  });

  test("has no source without the language service, the page or the node", async () => {
    expect(pxlSource(undefined, "a", page, "r", "compact", "data", new Map())).toBeUndefined();
    expect(pxlSource(language([]), undefined, page, "r", "compact", "data", new Map())).toBeUndefined();
    expect(pxlSource(language([]), "a", undefined, "r", "compact", "data", new Map())).toBeUndefined();
    const missing = pxlSource(language([]), "a", page, "ghost", "compact", "data", new Map());
    expect(await missing?.scope()).toBeUndefined();
  });

  test("a failing validator leaves the field without a scope and is asked again", async () => {
    let fail = true;
    const lang: PxlLanguage = {
      service: new PxlService(backend().backend),
      sites: async () => {
        if (fail) {
          throw new Error("worker gone");
        }
        return { sites: [{ pointer: "/root/props/data", env: 0 }], envs: [{ roots: {} }] };
      },
    };
    const cache = new Map();
    const source = pxlSource(lang, "a", page, "r", "compact", "data", cache);
    expect(await source?.scope()).toBeUndefined();
    fail = false;
    expect(await source?.scope()).toBe('{"roots":{}}');
  });

  test("a responsive size class has its own pointer", async () => {
    const lang: PxlLanguage = {
      service: new PxlService(backend().backend),
      sites: async () => ({ sites: [{ pointer: "/root/responsive/medium/data", env: 0 }], envs: [{ roots: {} }] }),
    };
    const source = pxlSource(lang, "a", page, "r", "medium", "data", new Map());
    expect(await source?.scope()).toBe('{"roots":{}}');
  });
});
