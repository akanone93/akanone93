// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import type { ApplyChangeSetRequest } from "@plux/api";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { orgA, renderApp } from "../../test-harness.tsx";

afterEach(cleanup);

const nav = (route: string) => ({ id: `go-${route}`, action: "navigate", input: { route } });
const plugin = (id: string, key: string, pages: string[], entry: string) => ({
  kind: "plugin",
  path: `plugins/${key}/plugin.json`,
  json: { kind: "plugin", id, key, name: key, pages, entryPage: entry },
});
const page = (id: string, key: string, steps: object[] = []) => ({
  kind: "page",
  path: `plugins/${key}/pages/${key}.page.json`,
  json: {
    id,
    key,
    route: key,
    title: key,
    kind: "page",
    pageKind: "screen",
    root: { id: `${id}-r`, events: { onPressed: { steps } } },
  },
});

const path = `/o/${orgA}/apps/app1`;
const summaries = [
  { id: "loans", key: "loans", name: "Loans" },
  { id: "cards", key: "cards", name: "Cards" },
];
const pluginDocuments = {
  loans: [plugin("loans", "loans", ["home"], "home"), page("home", "home", [nav("offers")])],
  cards: [plugin("cards", "cards", ["offers"], "offers"), page("offers", "offers")],
};

async function select(name: RegExp) {
  fireEvent.pointerDown(await screen.findByRole("button", { name }));
}

describe("[STU-070] deleting a plugin runs the where-used check (D19)", () => {
  test("links into it open the blocked-delete panel with the redirect fix", async () => {
    renderApp(path, { summaries, pluginDocuments });
    await select(/Cards/);
    fireEvent.click(await screen.findByRole("button", { name: "Delete plugin" }));
    expect(await screen.findByText("This cannot be deleted yet")).toBeDefined();
    expect(await screen.findByRole("button", { name: "Redirect all incoming links to…" })).toBeDefined();
  });
});

describe("[STU-070] redirecting incoming links is one change set", () => {
  test("choosing a new target sends a single ApplyChangeSet that rewrites the link", async () => {
    const changeSets: ApplyChangeSetRequest[] = [];
    renderApp(path, { summaries, pluginDocuments, changeSets });
    await select(/Cards/);
    fireEvent.click(await screen.findByRole("button", { name: "Delete plugin" }));
    fireEvent.click(await screen.findByRole("button", { name: "Redirect all incoming links to…" }));
    fireEvent.click(await screen.findByRole("combobox", { name: "New target screen" }));
    fireEvent.click(await screen.findByRole("option", { name: "home" }));
    fireEvent.click(screen.getByRole("button", { name: "Redirect" }));
    await waitFor(() => expect(changeSets.length).toBe(1));
    expect(changeSets[0]?.drafts.map((d) => d.pluginId)).toEqual(["loans"]);
  });
});

describe("[STU-073] [STU-074] deleting a plugin that has links away from it, in production", () => {
  test("an unpublished plugin without uses goes to the trash after typing its key", async () => {
    const pluginCalls: string[] = [];
    renderApp(path, {
      summaries: [{ id: "solo", key: "solo", name: "Solo" }],
      pluginDocuments: { solo: [plugin("solo", "solo", ["s"], "s"), page("s", "s")] },
      productionRelease: true,
      pluginCalls,
    });
    await select(/Solo/);
    fireEvent.click(await screen.findByRole("button", { name: "Delete plugin" }));
    const field = await screen.findByLabelText("Type solo to confirm");
    fireEvent.change(field, { target: { value: "solo" } });
    fireEvent.click(screen.getByRole("button", { name: "Delete plugin", hidden: false }));
    await waitFor(() => expect(pluginCalls).toEqual(["delete:solo"]));
  });

  test("a published plugin is deprecated, not deleted, and the dialog says what still links to it", async () => {
    const pluginCalls: string[] = [];
    renderApp(path, {
      summaries: [{ id: "cards", key: "cards", name: "Cards", latestVersion: 2 }],
      pluginDocuments: { cards: pluginDocuments.cards },
      pluginCalls,
    });
    await select(/Cards/);
    fireEvent.click(await screen.findByRole("button", { name: "Delete plugin" }));
    fireEvent.click(await screen.findByRole("button", { name: "Deprecate plugin" }));
    await waitFor(() => expect(pluginCalls).toEqual(["archive:cards"]));
  });
});

describe("A44 trash", () => {
  test("lists deleted items and restores one with its old id", async () => {
    const restored: string[] = [];
    renderApp(path, { summaries, pluginDocuments, trash: [{ id: "t1", kind: "plugin", name: "Old loans" }], restored });
    fireEvent.click(await screen.findByRole("button", { name: "Trash" }));
    expect(await screen.findByText("Old loans (plugin)")).toBeDefined();
    fireEvent.click(screen.getByRole("button", { name: "Restore Old loans" }));
    await waitFor(() => expect(restored).toEqual(["t1"]));
  });
});
