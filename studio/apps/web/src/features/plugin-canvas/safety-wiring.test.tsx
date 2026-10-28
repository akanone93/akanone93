// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import type { ApplyChangeSetRequest } from "@plux/api";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { orgA, renderApp } from "../../test-harness.tsx";
import type { CanvasSurface } from "./surface.ts";

afterEach(cleanup);

const surface = (): CanvasSurface => ({
  layoutScreens: (docs) => Promise.resolve(docs.map((d) => ({ id: d.id, contentHeight: 300 }))),
  setViewport: () => undefined,
  pan: () => undefined,
  zoomAbout: () => undefined,
  viewport: { x: 0, y: 0, zoom: 1 },
  dispose: () => undefined,
});

const page = (id: string, key: string, steps: object[] = []) => ({
  kind: "page",
  path: `plugins/pl/pages/${key}.page.json`,
  json: {
    id,
    key,
    route: key,
    title: key,
    kind: "page",
    pageKind: "screen",
    root: { id: `${id}-root`, type: "Button", events: { onPressed: { steps } } },
  },
});
const nav = (route: string) => ({ id: `go-${route}`, action: "navigate", input: { route } });
const plugin = (id: string, key: string, pages: string[], entry?: string) => ({
  kind: "plugin",
  path: `plugins/${key}/plugin.json`,
  json: { kind: "plugin", id, key, name: key, pages, ...(entry === undefined ? {} : { entryPage: entry }) },
});

const path = `/o/${orgA}/apps/app1/plugins/pl`;

async function openMenu(entry: "Rename screen" | "Move to plugin" | "Delete screen") {
  // The canvas boots its worker and fonts first; under a full-suite load that takes over the 1 s default.
  await screen.findByRole("button", { name: "Undo" }, { timeout: 5000 });
  const trigger = await screen.findByRole("button", { name: "Screen menu" });
  fireEvent.pointerDown(trigger, { button: 0 });
  fireEvent.click(trigger);
  fireEvent.click(await screen.findByRole("menuitem", { name: entry }));
}

describe("[STU-070] [STU-071] deleting a screen runs the where-used check (F13)", () => {
  const documents = [
    plugin("pl", "pl", ["home", "detail"], "home"),
    page("home", "home", [nav("detail")]),
    page("detail", "detail"),
  ];

  test("the entry screen opens the blocked-delete panel with the entry rule", async () => {
    renderApp(path, { documents, surface });
    await openMenu("Delete screen");
    expect(await screen.findByText("This cannot be deleted yet")).toBeDefined();
    expect(screen.getByText("It is an entry point. Choose another entry first.")).toBeDefined();
  });
});

describe("[STU-074] deleting in an app with a production release asks for the name", () => {
  const documents = [plugin("pl", "pl", ["lonely"]), page("lonely", "lonely")];

  test("with a production release the name must be typed", async () => {
    renderApp(path, { documents, surface, productionRelease: true });
    await openMenu("Delete screen");
    expect(await screen.findByLabelText("Type lonely to confirm")).toBeDefined();
  });

  test("without one a plain confirmation is enough", async () => {
    renderApp(path, { documents, surface });
    await openMenu("Delete screen");
    expect(await screen.findByRole("button", { name: "Delete" })).toBeDefined();
    expect(screen.queryByLabelText("Type lonely to confirm")).toBeNull();
  });
});

describe("D23 move a screen to another plugin", () => {
  const pluginDocuments = {
    pl: [plugin("pl", "pl", ["lonely"]), page("lonely", "lonely")],
    other: [plugin("other", "other", [])],
  };

  test("the target picker lists the other plugins and confirming sends one change set", async () => {
    const changeSets: ApplyChangeSetRequest[] = [];
    renderApp(path, { pluginDocuments, surface, changeSets });
    await openMenu("Move to plugin");
    fireEvent.click(await screen.findByRole("combobox", { name: "Target plugin" }));
    fireEvent.click(await screen.findByRole("option", { name: "other" }));
    fireEvent.click(screen.getByRole("button", { name: "Move" }));
    await waitFor(() => expect(changeSets.length).toBe(1));
    expect(changeSets[0]?.drafts.map((d) => d.pluginId).sort()).toEqual(["other", "pl"]);
  });
});

describe("D27 rename warning", () => {
  const documents = [plugin("pl", "pl", ["lonely"]), page("lonely", "lonely")];

  test("a rename that changes the route lists the deep link and generated API it affects", async () => {
    renderApp(path, { documents, surface });
    await openMenu("Rename screen");
    const field = await screen.findByLabelText("Screen title");
    fireEvent.change(field, { target: { value: "Welcome" } });
    expect(await screen.findByRole("alert")).toBeDefined();
    expect(screen.getByText("/lonely → /welcome")).toBeDefined();
  });
});
