// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { orgA, renderApp } from "../../test-harness.tsx";
import { copiesOf, inOrder } from "./plugin-bulk.tsx";

afterEach(cleanup);

const path = `/o/${orgA}/apps/app1`;
const summaries = [
  { id: "loans", key: "loans", name: "Loans" },
  { id: "cards", key: "cards", name: "Cards" },
];

describe("plugins acted on together on the app canvas (#264)", () => {
  test("each copy gets a numbered name and a key free among the app's and the other copies'", () => {
    const copies = copiesOf(
      [
        { id: "a", name: "Loans" },
        { id: "b", name: "Loans" },
      ],
      new Set(["loans", "loans-2"]),
    );
    expect(copies.map((c) => c.name)).toEqual(["Loans 2", "Loans 2"]);
    const keys = copies.map((c) => c.key);
    expect(new Set(keys).size).toBe(2);
    expect(keys.some((k) => k === "loans" || k === "loans-2")).toBe(false);
  });

  test("calls run one after another and stop at the first refused", async () => {
    const ran: number[] = [];
    const ok = await inOrder([
      async () => ran.push(1),
      async () => {
        throw new Error("refused");
      },
      async () => ran.push(3),
    ]);
    expect(ok).toBe(false);
    expect(ran).toEqual([1]);
  });

  test("two selected plugins are archived together after one confirmation", async () => {
    const pluginCalls: string[] = [];
    renderApp(path, { summaries, pluginCalls });
    fireEvent.pointerDown(await screen.findByRole("button", { name: /^Loans/ }));
    fireEvent.pointerDown(await screen.findByRole("button", { name: /^Cards/ }), { shiftKey: true });
    const panel = await screen.findByRole("complementary", { name: "2 plugins selected" });
    fireEvent.click(within(panel).getByRole("button", { name: "Archive 2 plugins" }));
    const dialog = await screen.findByRole("alertdialog");
    expect(dialog.textContent).toContain("They stay in the app");
    fireEvent.click(within(dialog).getByRole("button", { name: "Archive 2 plugins" }));
    await waitFor(() => expect(pluginCalls).toEqual(["archive:loans", "archive:cards"]));
  });

  test("archiving one plugin says it archives, not that it deletes", async () => {
    renderApp(path, { summaries });
    fireEvent.pointerDown(await screen.findByRole("button", { name: /^Cards/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Archive plugin" }));
    const dialog = await screen.findByRole("alertdialog");
    expect(dialog.textContent).toContain("Archive Cards?");
    expect(dialog.textContent).not.toContain("trash");
    expect(within(dialog).getByRole("button", { name: "Archive plugin" })).toBeTruthy();
  });
});
