// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { createStudioI18n } from "@plux/i18n";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { I18nProvider } from "../../i18n-provider.tsx";
import { loadSourceCatalogue } from "../../test-harness.tsx";
import type { LoadedDoc } from "../safety/index.ts";
import { appEntryRoute, appGraph } from "./app-canvas.scene.ts";
import { AppRoutes, appRoutes, type RouteScreen, searchRoutes } from "./app-routes.tsx";

afterEach(cleanup);

const page = (pluginId: string, id: string, route: string, steps: object[] = []): LoadedDoc => ({
  pluginId,
  path: `pages/${id}.page.json`,
  kind: "page",
  revision: 1,
  json: { id, key: id, route, title: id, kind: "page", root: { id: `${id}-r`, events: { onPressed: { steps } } } },
});
const nav = (route: string) => ({ action: "navigate", input: { route } });
const app: LoadedDoc = { pluginId: "", path: "app.json", kind: "app", revision: 1, json: { entryRoute: "start" } };
const docs: LoadedDoc[] = [
  app,
  page("launch", "splash", "start", [nav("home"), nav("camera")]),
  page("launch", "launchHome", "home"),
  page("main", "mainHome", "home", [nav("gone"), nav("gone"), nav("settings")]),
  page("main", "settings", "settings", [nav("gone")]),
];
const names: Record<string, string> = { launch: "Launch", main: "Main" };
const pluginName = (id: string) => names[id] ?? id;

describe("every route of the app on the app canvas (#268, #259)", () => {
  const graph = appGraph(docs, ["camera", "scanner"]);
  const routes = appRoutes(graph, ["camera", "scanner"], appEntryRoute(docs));

  test("a link to a route nothing has is a broken link, kept with its page and step", () => {
    expect(graph.broken.map((b) => [b.route, b.sourcePage])).toEqual([
      ["gone", "mainHome"],
      ["gone", "mainHome"],
      ["gone", "settings"],
    ]);
  });

  test("the entry route comes first, then every route by name: screens, native routes and missing ones", () => {
    expect(routes.map((r) => [r.route, r.kind, r.entry])).toEqual([
      ["start", "screen", true],
      ["camera", "native", false],
      ["gone", "missing", false],
      ["home", "screen", false],
      ["scanner", "native", false],
      ["settings", "screen", false],
    ]);
  });

  test("a name two screens share lists both; each route knows the screens linking to it, once each", () => {
    const home = routes.find((r) => r.route === "home");
    expect(home?.screens.map((s) => `${s.pluginId}/${s.pageId}`)).toEqual(["launch/launchHome", "main/mainHome"]);
    expect(home?.from.map((s) => s.pageId)).toEqual(["splash"]);
    expect(routes.find((r) => r.route === "gone")?.from.map((s) => s.pageId)).toEqual(["mainHome", "settings"]);
  });

  test("the search finds a route by its name, its screens' titles and their plugins' names", () => {
    expect(searchRoutes(routes, "sett", pluginName).map((r) => r.route)).toEqual(["settings"]);
    expect(searchRoutes(routes, "launch", pluginName).map((r) => r.route)).toEqual(["start", "home"]);
    expect(searchRoutes(routes, "  ", pluginName)).toHaveLength(routes.length);
  });

  test("the list marks the entry, flags the clash and the broken route, and opens a screen", async () => {
    const opened: RouteScreen[] = [];
    const studio = createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false });
    render(
      <I18nProvider studio={studio}>
        <AppRoutes routes={routes} pluginName={pluginName} onOpen={(s) => opened.push(s)} />
      </I18nProvider>,
    );
    const list = await screen.findByRole("list", { name: "Routes" });
    expect(within(list).getAllByRole("listitem")).toHaveLength(6);
    expect(screen.getByLabelText("The app opens here")).toBeTruthy();
    expect(screen.getAllByText(/several screens have this name/).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/No screen has this route/).length).toBeGreaterThan(0);
    // Main's home is a screen of `home` and links to the broken `gone`: it is offered under both.
    const mainHome = screen.getAllByRole("button", { name: "Open Main › mainHome" });
    expect(mainHome).toHaveLength(2);
    fireEvent.click(mainHome[0] as HTMLElement);
    expect(opened.at(-1)).toEqual({ pluginId: "main", pageId: "mainHome", title: "mainHome" });

    fireEvent.change(screen.getByRole("textbox", { name: "Search routes and screens" }), {
      target: { value: "nothing like it" },
    });
    expect(screen.getByText("No route matches the search.")).toBeTruthy();
  });
});
