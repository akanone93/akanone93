// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { cleanup, fireEvent, screen, within } from "@testing-library/react";
import { orgA, renderApp } from "../../test-harness.tsx";
import type { CanvasSurface } from "../plugin-canvas/surface.ts";
import { budgetStatus, compactJson, functionsIn, nodeBudget, stringsIn, subtreeOf } from "./tab-model.ts";

afterEach(cleanup);

const pay = { id: "t2", type: "Text", props: { data: "Pay" } };
const button = {
  id: "b1",
  type: "Button",
  children: [pay],
  events: {
    onPressed: { steps: [{ action: "invokeFunction", id: "fee", input: { function: "bank.transferFee" } }] },
  },
};
const text = { id: "t1", type: "Text", props: { data: { $t: "home.title" } } };
const documents = [
  {
    kind: "plugin",
    path: "plugin.json",
    json: {
      id: "pl",
      kind: "plugin",
      entryPage: "home",
      pages: ["home"],
      state: [
        { id: "s1", name: "count", type: "int" },
        { id: "s2", name: "double", type: "int", computed: "count * 2" },
      ],
      dataSources: [{ id: "d1", name: "rates", mock: { rate: 1 } }],
      capabilities: { functions: [{ function: "bank.transferFee" }] },
    },
  },
  {
    kind: "page",
    path: "pages/home.page.json",
    json: {
      id: "home",
      key: "home",
      route: "home",
      kind: "page",
      pageKind: "screen",
      root: { id: "home-root", slots: { body: { id: "home-body", children: [text, button] } } },
    },
  },
];
const path = `/o/${orgA}/apps/app1/plugins/pl`;
const surface = (hit: string) => (): CanvasSurface => ({
  layoutScreens: (docs) => Promise.resolve(docs.map((d) => ({ id: d.id, contentHeight: 300 }))),
  setViewport: () => undefined,
  pan: () => undefined,
  zoomAbout: () => undefined,
  viewport: { x: 0, y: 0, zoom: 1 },
  hitTest: () => [hit],
  dispose: () => undefined,
});
const pick = (canvas: HTMLElement) => {
  fireEvent.pointerDown(canvas, { clientX: 1, clientY: 1, ctrlKey: true });
  fireEvent.pointerUp(canvas, { clientX: 1, clientY: 1, ctrlKey: true });
};

describe("S5.6 panel tabs (STU-061, STU-062)", () => {
  test("STU-061 the left State and Data tabs summarise the plugin and open the designers", async () => {
    renderApp(path, { documents, surface: surface("b1") });
    fireEvent.click(await screen.findByRole("tab", { name: "State" }, { timeout: 5000 }));
    expect(await screen.findByText("2 state entries, 1 computed value")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Open the state designer" })).toBeTruthy();
    fireEvent.click(screen.getByRole("tab", { name: "Data" }));
    expect(await screen.findByText("1 data source, 1 mock, 0 collections")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Open the data source designer" }));
    expect(await screen.findByRole("dialog")).toBeTruthy();
  });

  test("STU-062 the right JSON, Code, Localisation and Performance tabs describe the selection", async () => {
    renderApp(path, {
      documents,
      surface: surface("b1"),
      summaries: [
        {
          id: "pl",
          key: "pl",
          name: "Pl",
          bundleSize: 2048,
          screens: 1,
          versions: 3,
          functionsUsed: ["bank.transferFee"],
        },
      ],
    });
    pick(await screen.findByLabelText("Plugin canvas", undefined, { timeout: 5000 }));
    fireEvent.click(await screen.findByRole("tab", { name: "JSON" }));
    expect((await screen.findByLabelText("JSON of the selection")).textContent).toContain('"id":"b1"');

    fireEvent.click(screen.getByRole("tab", { name: "Code" }));
    const used = await screen.findByRole("list", { name: "Functions used by the selection" });
    expect(within(used).getByText("bank.transferFee")).toBeTruthy();

    fireEvent.click(screen.getByRole("tab", { name: "Localisation" }));
    expect(within(await screen.findByRole("list", { name: "Strings of the selection" })).getByText("Pay")).toBeTruthy();
    expect(screen.getByText("Not translated")).toBeTruthy();

    fireEvent.click(screen.getByRole("tab", { name: "Performance" }));
    expect(await screen.findByText("Within budget")).toBeTruthy();
    expect(await screen.findByText("2.0 KB")).toBeTruthy();
    expect(screen.getByRole("meter", { name: "Node budget" })).toBeTruthy();
  });
});

describe("tab model", () => {
  test("subtreeOf, compactJson, functionsIn and stringsIn read a node", () => {
    const nodes = subtreeOf({ id: "r", slots: { body: { id: "b", children: [text, button] } } });
    expect(nodes.map((n) => n["id"])).toEqual(["r", "b", "t1", "b1", "t2"]);
    expect(compactJson([text])).toBe('{"id":"t1","type":"Text","props":{"data":{"$t":"home.title"}}}');
    expect(functionsIn(nodes)).toEqual(["bank.transferFee"]);
    expect(stringsIn(nodes).map((s) => [s.node, s.key])).toEqual([
      ["t1", "home.title"],
      ["t2", undefined],
    ]);
  });

  test("budgetStatus follows the page.nodes limit", () => {
    const b = nodeBudget();
    expect(budgetStatus(1, b)).toBe("within");
    expect(budgetStatus(b.warning + 1, b)).toBe("warning");
    expect(budgetStatus(b.limit + 1, b)).toBe("over");
  });
});

describe("Code tab functions (FN-033, FN-034)", () => {
  test("a listed function opens the Rust editor on its source and offers its tests", async () => {
    renderApp(path, { documents, surface: surface("b1") });
    pick(await screen.findByLabelText("Plugin canvas", undefined, { timeout: 5000 }));
    fireEvent.click(await screen.findByRole("tab", { name: "Code" }));
    const used = await screen.findByRole("list", { name: "Functions used by the selection" });
    expect(within(used).getByRole("button", { name: "Run the tests of bank.transferFee" })).toBeTruthy();
    fireEvent.click(within(used).getByRole("button", { name: "Edit bank.transferFee in the Rust editor" }));
    const dialog = await screen.findByRole("dialog");
    expect((within(dialog).getByLabelText("Uploaded source") as HTMLInputElement).value).toBe("bank.transferFee");
  });
});
