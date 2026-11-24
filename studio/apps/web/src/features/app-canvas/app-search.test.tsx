// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { createStudioI18n } from "@plux/i18n";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { I18nProvider } from "../../i18n-provider.tsx";
import { loadSourceCatalogue } from "../../test-harness.tsx";
import { buildWhereUsed, type LoadedDoc } from "../safety/index.ts";
import { searchApp, searchables } from "./app-search.ts";
import { AppSearch } from "./app-search.tsx";

afterEach(cleanup);

const doc = (pluginId: string, path: string, kind: string, json: unknown): LoadedDoc => ({
  pluginId,
  path,
  kind,
  revision: 1,
  json,
});
const docs: LoadedDoc[] = [
  doc("", "app.json", "app", { kind: "app", state: [{ name: "cartCount" }] }),
  doc("", "translations/keys.json", "translationKeys", {
    kind: "translationKeys",
    keys: [{ id: "k1", key: "home.title" }],
  }),
  doc("main", "plugins/main/plugin.json", "plugin", {
    kind: "plugin",
    id: "main",
    key: "main",
    state: [{ name: "draft" }],
  }),
  doc("main", "plugins/main/components/badge.component.json", "component", {
    kind: "component",
    id: "c1",
    name: "Badge",
  }),
  doc("main", "plugins/main/pages/home.page.json", "page", {
    kind: "page",
    id: "home",
    key: "home",
    route: "home",
    title: "Home",
    root: {
      id: "r",
      type: "Column",
      children: [
        { id: "t", type: "Text", props: { data: { $t: "k1" } } },
        { id: "b", type: "Badge", component: { id: "c1" }, props: { label: "${state.cartCount}" } },
        { id: "i", type: "Image", props: { image: { $asset: "logo.png" } } },
      ],
    },
  }),
];
const plugins = [{ id: "main", key: "main", name: "Main", functionsUsed: ["loan.rate"] }];
const all = searchables({ docs, index: buildWhereUsed(docs), plugins });

describe("one search across the app, with where each thing is used (#265)", () => {
  test("each kind is found with the screens that use it; a thing nothing uses says so", () => {
    const one = (q: string) => [...searchApp(all, q)].map(([kind, hits]) => [kind, hits.map((h) => h.title)]);
    expect(one("home")).toEqual([
      ["screen", ["Home"]],
      ["key", ["home.title"]],
    ]);
    const key = searchApp(all, "home.title").get("key")?.[0];
    expect(key !== undefined && "uses" in key ? key.uses : []).toEqual([
      { pluginId: "main", pageId: "home", title: "Home" },
    ]);
    expect(one("badge")).toEqual([["component", ["Badge"]]]);
    const cart = searchApp(all, "cartCount").get("state")?.[0];
    expect(cart !== undefined && "uses" in cart ? cart.uses.map((p) => p.pageId) : []).toEqual(["home"]);
    const draft = searchApp(all, "draft").get("state")?.[0];
    expect(draft !== undefined && "uses" in draft ? draft.uses : undefined).toEqual([]);
    expect(one("logo")).toEqual([["asset", ["logo.png"]]]);
    expect(one("loan")).toEqual([["function", ["loan.rate"]]]);
    expect(one("Main")).toEqual([["plugin", ["Main"]]]);
    expect(searchApp(all, "  ").size).toBe(0);
  });

  test("the tab lists the groups and opens the screen a key is used on", async () => {
    const opened: string[] = [];
    const studio = createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false });
    render(
      <I18nProvider studio={studio}>
        <AppSearch
          all={all}
          actions={{
            onPlugin: (id) => opened.push(`plugin:${id}`),
            onScreen: (plugin, page) => opened.push(`screen:${plugin}/${page}`),
            pluginName: (id) => (id === "main" ? "Main" : id),
          }}
        />
      </I18nProvider>,
    );
    fireEvent.change(await screen.findByRole("textbox", { name: /Search plugins, screens/ }), {
      target: { value: "home.title" },
    });
    const keys = screen.getByRole("region", { name: "Translation keys" });
    expect(within(keys).getByText("Used in 1 place")).toBeTruthy();
    fireEvent.click(within(keys).getByRole("button", { name: "Main › Home" }));
    expect(opened).toEqual(["screen:main/home"]);
  });
});
