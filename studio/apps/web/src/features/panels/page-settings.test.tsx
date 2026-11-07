// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { orgA, renderApp } from "../../test-harness.tsx";
import type { EditSession } from "../editor/edit-session.ts";
import type { CanvasSurface } from "../plugin-canvas/surface.ts";
import {
  addParam,
  freshParamName,
  paramNameProblem,
  readPage,
  removeParam,
  retarget,
  routeProblem,
  setGuard,
  setNested,
  setParam,
  validType,
  valueOfText,
} from "./page-settings-model.ts";

afterEach(() => {
  cleanup();
  localStorage.clear();
});

const page = {
  kind: "page",
  id: "p1",
  key: "loan",
  route: "loan",
  title: "Loan",
  pageKind: "dialog",
  params: [{ name: "amount", type: "decimal", required: true, mock: "1200.00" }],
  state: [{ id: "s", name: "x", type: "int", default: 0 }],
  lifecycle: { onEnter: { steps: [{ action: "haptic", id: "h", input: { pattern: "light" } }] } },
  routeOptions: { transition: "fade", guards: [{ $graph: "g1" }] },
  security: { secure: true, requiresAssurance: "AL2" },
};

describe("PC-14 page settings model [SCH-022] [SCH-024] [SCH-025]", () => {
  test("a page document reads as its settings", () => {
    const s = readPage(page);
    expect(s).toMatchObject({ route: "loan", kind: "dialog", title: { text: "Loan" }, state: 1, dataSources: 0 });
    expect(s?.params).toEqual([{ name: "amount", type: "decimal", required: true, mock: "1200.00" }]);
    expect([...(s?.lifecycle ?? [])]).toEqual(["onEnter"]);
    expect(s).toMatchObject({ transition: "fade", guards: ["g1"], secure: true, assurance: "AL2" });
    expect(readPage({ ...page, title: { $t: "loan.title" } })?.title).toEqual({ key: "loan.title" });
    expect(readPage({ kind: "plugin" })).toBeUndefined();
  });

  test("route names, parameter names and types are checked as the schema checks them", () => {
    const taken = new Set(["home", "loan"]);
    expect(routeProblem("loan-calculator", taken, "loan")).toBeUndefined();
    expect(routeProblem("loan", taken, "loan")).toBeUndefined();
    expect(routeProblem("home", taken, "loan")).toBe("taken");
    expect(routeProblem("Loan Calc", taken, "loan")).toBe("form");
    expect(paramNameProblem("amount", ["term"])).toBeUndefined();
    expect(paramNameProblem("term", ["term"])).toBe("taken");
    expect(paramNameProblem("Amount", [])).toBe("form");
    expect(validType("list<Transaction>")).toBe(true);
    expect(validType("decimal?")).toBe(true);
    expect(validType("1x")).toBe(false);
  });

  test("defaults and mocks typed as text become values of their type", () => {
    expect(valueOfText("int", "12.7")).toBe(12);
    expect(valueOfText("double", "1.5")).toBe(1.5);
    expect(valueOfText("bool", "true")).toBe(true);
    expect(valueOfText("decimal", "1200.00")).toBe("1200.00");
    expect(valueOfText("list<string>", '["a","b"]')).toEqual(["a", "b"]);
    expect(valueOfText("string", "  ")).toBeUndefined();
  });

  test("the recipes add, change and remove parameters, options and guards, leaving no empty objects", () => {
    const d = structuredClone(page) as Record<string, unknown>;
    addParam(freshParamName(readPage(d)?.params ?? []))(d);
    expect((d["params"] as { name: string }[]).map((p) => p.name)).toEqual(["amount", "param"]);
    setParam(1, "required", true)(d);
    setParam(1, "required", false)(d);
    expect((d["params"] as Record<string, unknown>[])[1]).toEqual({ name: "param", type: "string" });
    removeParam(1)(d);
    removeParam(0)(d);
    expect("params" in d).toBe(false);
    setGuard("g1", false)(d);
    setNested("routeOptions", "transition", undefined)(d);
    expect("routeOptions" in d).toBe(false);
    setNested("security", "secure", false)(d);
    setNested("security", "requiresAssurance", undefined)(d);
    expect("security" in d).toBe(false);
  });

  test("a route rename takes every step that opens it along", () => {
    const doc = {
      root: {
        id: "b",
        events: {
          onPressed: {
            steps: [
              { action: "openDialog", id: "1", input: { route: "loan" } },
              { action: "navigate", id: "2", input: { route: "home" } },
            ],
          },
        },
      },
    };
    expect(retarget(doc, "loan", "loan-calc")).toBe(1);
    expect(doc.root.events.onPressed.steps[0]?.input.route).toBe("loan-calc");
    expect(doc.root.events.onPressed.steps[1]?.input.route).toBe("home");
  });
});

const text = (id: string, data: string) => ({ id, type: "Text", props: { data } });
const documents = [
  {
    kind: "plugin",
    path: "plugin.json",
    json: { id: "pl", kind: "plugin", entryPage: "home", pages: ["home", "loan"] },
  },
  {
    kind: "page",
    path: "pages/home.page.json",
    json: {
      id: "home",
      key: "home",
      route: "home",
      kind: "page",
      title: "Home",
      pageKind: "screen",
      root: {
        id: "home-root",
        type: "Column",
        children: [
          {
            id: "open",
            type: "FilledButton",
            events: { onPressed: { steps: [{ action: "openDialog", id: "s", input: { route: "loan" } }] } },
            slots: { child: text("t1", "Open") },
          },
        ],
      },
    },
  },
  {
    kind: "page",
    path: "pages/loan.page.json",
    json: { ...page, id: "loan", root: { id: "loan-root", type: "Column" } },
  },
];
const path = `/o/${orgA}/apps/app1/plugins/pl`;

function surface(hits: { current: string[] }): () => CanvasSurface {
  return () => ({
    layoutScreens: (docs) => Promise.resolve(docs.map((d) => ({ id: d.id, contentHeight: 300 }))),
    setViewport: () => undefined,
    pan: () => undefined,
    zoomAbout: () => undefined,
    viewport: { x: 0, y: 0, zoom: 1 },
    hitTest: () => hits.current,
    dispose: () => undefined,
  });
}

describe("PC-14 the Page pane [SCH-022] [SCH-025]", () => {
  test("the selected page's settings edit its document; a route rename updates the plugin's links", async () => {
    const hits = { current: ["loan-root"] };
    let edit: EditSession | undefined;
    renderApp(path, { documents, surface: surface(hits), editor: (e) => (edit = e) });
    const canvas = await screen.findByLabelText("Plugin canvas");
    fireEvent.pointerDown(canvas, { clientX: 1, clientY: 1, ctrlKey: true });
    fireEvent.pointerUp(canvas, { clientX: 1, clientY: 1, ctrlKey: true });
    fireEvent.click(await screen.findByRole("tab", { name: "Page" }));
    const route = (await screen.findByRole("textbox", { name: "Route name" })) as HTMLInputElement;
    expect(route.value).toBe("loan");
    expect(screen.getByText("1 link in this plugin follows the new name.")).toBeTruthy();
    fireEvent.change(route, { target: { value: "home" } });
    expect(await screen.findByText("Another page of the app has this route name.")).toBeTruthy();
    fireEvent.change(route, { target: { value: "loan-calculator" } });
    fireEvent.blur(route);
    await waitFor(() =>
      expect((edit?.store.getState().docs["pages/loan.page.json"]?.content as { route?: string })?.route).toBe(
        "loan-calculator",
      ),
    );
    const home = edit?.store.getState().docs["pages/home.page.json"]?.content as {
      root: { children: { events: { onPressed: { steps: { input: { route: string } }[] } } }[] };
    };
    expect(home.root.children[0]?.events.onPressed.steps[0]?.input.route).toBe("loan-calculator");
    // Page, the first section, is open; Parameters opens from its head (MR-3).
    expect(screen.queryByRole("textbox", { name: "Mock" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: /^Parameters/ }));
    const mock = (await screen.findByRole("textbox", { name: "Mock" })) as HTMLInputElement;
    expect(mock.value).toBe("1200.00");
    fireEvent.click(screen.getByRole("button", { name: "Add parameter" }));
    await waitFor(() =>
      expect(
        ((edit?.store.getState().docs["pages/loan.page.json"]?.content as { params?: unknown[] })?.params ?? []).length,
      ).toBe(2),
    );
  });
});
