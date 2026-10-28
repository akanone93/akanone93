// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { orgA, renderApp, type Upstream } from "../../test-harness.tsx";
import type { CanvasSurface } from "../plugin-canvas/surface.ts";
import type { AppGraph } from "./app-canvas.scene.ts";
import { miniMapOf } from "./plugin-minimap.ts";

afterEach(cleanup);

const path = `/o/${orgA}/apps/app1`;
const nav = (route: string) => ({ id: `go-${route}`, action: "navigate", input: { route } });
const plugin = (id: string, pages: string[]) => ({
  kind: "plugin",
  path: `plugins/${id}/plugin.json`,
  json: { kind: "plugin", id, key: id, name: id, pages, entryPage: pages[0] },
});
const page = (id: string, plug: string, steps: object[] = []) => ({
  kind: "page",
  path: `plugins/${plug}/pages/${id}.page.json`,
  json: {
    id,
    key: id,
    route: id,
    title: id,
    kind: "page",
    pageKind: "screen",
    root: { id: `${id}-r`, events: { onPressed: { steps } } },
  },
});

const state = (over: Upstream = {}): Upstream => ({
  summaries: [
    {
      id: "loans",
      key: "loans",
      name: "Loans",
      description: "Apply for and track loans",
      latestVersion: 2,
      versions: 2,
      screens: 2,
      bundleSize: 12 * 1024,
      linksOut: ["cards", "cards"],
      linksIn: ["cards"],
      functionsUsed: ["loan.schedule", "loan.rate"],
      publishedAt: 1_700_000_000,
    },
    { id: "cards", key: "cards", name: "Cards" },
  ],
  pluginVersions: {
    loans: [
      { version: 1, by: "Bo", at: 1_690_000_000 },
      { version: 2, by: "Ana", at: 1_700_000_000 },
    ],
  },
  pluginDocuments: {
    loans: [
      plugin("loans", ["home", "detail"]),
      page("home", "loans", [nav("detail"), nav("offers")]),
      page("detail", "loans"),
    ],
    cards: [plugin("cards", ["offers"]), page("offers", "cards")],
  },
  appDocuments: [
    { kind: "canvas", path: "canvas.json", json: { kind: "canvas", id: "c1", positions: [], frames: [] } },
  ],
  ...over,
});

/** A surface that answers pointer hits from `hits`. */
function stub() {
  const hits = { point: [] as string[] };
  const surface: CanvasSurface = {
    layoutScreens: () => Promise.resolve([]),
    setViewport: () => undefined,
    pan: () => undefined,
    zoomAbout: () => undefined,
    hitTest: () => hits.point,
    hitTestRect: () => [],
    setSelection: () => undefined,
    setGuides: () => undefined,
    patch: () => undefined,
    viewport: { x: 0, y: 0, zoom: 1 },
    dispose: () => undefined,
  };
  return { hits, factory: () => surface };
}

const card = () => screen.queryByRole("complementary", { name: "Loans" });
const canvas = () => document.querySelector("canvas") as HTMLCanvasElement;

describe("the plugin detail card [STU-031]", () => {
  test("keyboard focus on a plugin shows its description, mini-map, links, functions, size and publisher", async () => {
    renderApp(path, state({ surface: stub().factory }));
    expect(card() === null).toBe(true);
    fireEvent.focus(await screen.findByRole("button", { name: /^Loans/ }));
    const detail = await screen.findByRole("complementary", { name: "Loans" });
    expect(detail.textContent).toContain("Apply for and track loans");
    expect(detail.textContent).toContain("12 KB bundle");
    expect(detail.textContent).toContain("2 links out");
    expect(detail.textContent).toContain("1 link in");
    expect(detail.textContent).toContain("2 functions");
    expect(detail.textContent).toContain("Uses loan.schedule, loan.rate");
    const map = await screen.findByRole("img", { name: "Screens: detail, home" });
    expect(map.querySelectorAll("rect").length).toBe(2);
    expect(map.querySelectorAll("line").length).toBe(1);
    await waitFor(() => expect(detail.textContent).toMatch(/Last published by Ana, /));
  });

  test("blurring the plugin hides the card again", async () => {
    renderApp(path, state({ surface: stub().factory }));
    const box = await screen.findByRole("button", { name: /^Loans/ });
    fireEvent.focus(box);
    await screen.findByRole("complementary", { name: "Loans" });
    fireEvent.blur(box);
    await waitFor(() => expect(card() === null).toBe(true));
  });

  test("hovering a plugin with the pointer shows the card and leaving the canvas hides it", async () => {
    const s = stub();
    renderApp(path, state({ surface: s.factory }));
    await screen.findByRole("button", { name: /^Loans/ });
    s.hits.point = ["app-box/loans"];
    fireEvent.pointerMove(canvas(), { clientX: 10, clientY: 10 });
    await screen.findByRole("complementary", { name: "Loans" });
    fireEvent.pointerLeave(canvas());
    await waitFor(() => expect(card() === null).toBe(true));
  });

  test("a plugin that was never published says so and an empty plugin has an empty mini-map", async () => {
    renderApp(
      path,
      state({
        surface: stub().factory,
        summaries: [{ id: "cards", key: "cards", name: "Cards" }],
        pluginDocuments: { cards: [plugin("cards", [])] },
      }),
    );
    fireEvent.focus(await screen.findByRole("button", { name: /^Cards/ }));
    const detail = await screen.findByRole("complementary", { name: "Cards" });
    expect(detail.textContent).toContain("Not published yet");
    expect(detail.textContent).toContain("No screens yet");
  });
});

describe("the mini-map of a plugin's screens [STU-031]", () => {
  const pageInfo = (id: string, pluginId: string, title: string) =>
    [id, { id, key: id, route: id, title, pluginId, path: `${id}.json` }] as const;
  const link = (source: string, target: string | undefined, sourcePlugin: string, targetPlugin?: string) =>
    ({
      edge: { id: `${source}>${target}`, source, target: target ?? "native", trigger: "", title: "" },
      route: "",
      sourcePlugin,
      sourcePage: source,
      ...(target === undefined ? {} : { targetPage: target, targetPlugin: targetPlugin ?? sourcePlugin }),
    }) as unknown as AppGraph["links"][number];
  const graph: AppGraph = {
    links: [link("a", "b", "p"), link("b", "c", "p"), link("a", "x", "p", "q"), link("b", undefined, "p")],
    boxOf: new Map(),
    natives: [],
    pages: new Map([
      pageInfo("a", "p", "Alpha"),
      pageInfo("b", "p", "Beta"),
      pageInfo("c", "p", "Gamma"),
      pageInfo("x", "q", "Other"),
    ]),
  };

  test("holds the plugin's screens and the links among them only, ordered by title", () => {
    const map = miniMapOf(graph, "p");
    expect(map.screens.map((s) => s.title)).toEqual(["Alpha", "Beta", "Gamma"]);
    expect(map.edges).toEqual([
      { from: "a", to: "b" },
      { from: "b", to: "c" },
    ]);
  });

  test("fits the box and is the same for the same app", () => {
    const map = miniMapOf(graph, "p");
    for (const s of map.screens) {
      expect(s.x).toBeGreaterThanOrEqual(0);
      expect(s.x + map.cell.width).toBeLessThanOrEqual(map.width);
      expect(s.y + map.cell.height).toBeLessThanOrEqual(map.height);
    }
    expect(miniMapOf(graph, "p")).toEqual(map);
    expect(miniMapOf(graph, "none").screens).toEqual([]);
  });
});
