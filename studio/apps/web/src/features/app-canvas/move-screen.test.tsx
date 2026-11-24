// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import type { ApplyChangeSetRequest } from "@plux/api";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { orgA, renderApp } from "../../test-harness.tsx";

afterEach(cleanup);

const plugin = (id: string, pages: string[]) => ({
  kind: "plugin",
  path: `plugins/${id}/plugin.json`,
  json: { kind: "plugin", id, key: id, name: id, pages, entryPage: pages[0] },
});
const page = (id: string, plug: string) => ({
  kind: "page",
  path: `plugins/${plug}/pages/${id}.page.json`,
  json: { id, key: id, route: id, title: id, kind: "page", pageKind: "screen", root: { id: `${id}-r` } },
});

describe("moving a screen to another plugin from the app canvas (#287)", () => {
  test("the Routes tab moves a screen with one change set across both plugins; the entry screen cannot move", async () => {
    const changeSets: ApplyChangeSetRequest[] = [];
    renderApp(`/o/${orgA}/apps/app1`, {
      summaries: [
        { id: "loans", key: "loans", name: "Loans" },
        { id: "cards", key: "cards", name: "Cards" },
      ],
      pluginDocuments: {
        loans: [plugin("loans", ["home", "terms"]), page("home", "loans"), page("terms", "loans")],
        cards: [plugin("cards", ["offers"]), page("offers", "cards")],
      },
      changeSets,
    });
    fireEvent.click(await screen.findByRole("tab", { name: /Routes/ }, { timeout: 5000 }));
    fireEvent.click(await screen.findByRole("button", { name: "Move Loans › terms to another plugin…" }));
    fireEvent.click(await screen.findByRole("combobox", { name: "Target plugin" }));
    fireEvent.click(await screen.findByRole("option", { name: "Cards" }));
    fireEvent.click(screen.getByRole("button", { name: "Move" }));
    await waitFor(() => expect(changeSets.length).toBe(1));
    expect(changeSets[0]?.drafts.map((d) => d.pluginId).sort()).toEqual(["cards", "loans"]);

    // The plugin's entry screen stays until another is chosen: the move says why and changes nothing.
    fireEvent.click(await screen.findByRole("button", { name: "Move Loans › home to another plugin…" }));
    fireEvent.click(await screen.findByRole("combobox", { name: "Target plugin" }));
    fireEvent.click(await screen.findByRole("option", { name: "Cards" }));
    fireEvent.click(screen.getByRole("button", { name: "Move" }));
    expect(await screen.findByText(/The screen cannot move/)).toBeTruthy();
    expect(changeSets.length).toBe(1);
  });
});
