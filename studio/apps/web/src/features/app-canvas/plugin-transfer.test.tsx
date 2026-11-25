// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { orgA, renderApp } from "../../test-harness.tsx";

afterEach(cleanup);

const path = `/o/${orgA}/apps/app1`;
const summaries = [{ id: "loans", key: "loans", name: "Loans" }];
const archive = () => new File([new Uint8Array([80, 75, 3, 4])], "cards.plux", { type: "application/zip" });

function choose(file: File): void {
  const input = screen.getByLabelText("Import plugin", { selector: "input" });
  fireEvent.change(input, { target: { files: [file] } });
}

describe("a plugin to and from a .plux archive (build log row 264)", () => {
  test("an imported plugin says what it added to the app and the problems it has there", async () => {
    const projects = { calls: [] as string[] };
    renderApp(path, { summaries, projects });
    await screen.findByRole("button", { name: /^Loans/ });
    choose(archive());
    await waitFor(() => expect(projects.calls).toEqual(["importPlugin:app1:::4"]));
    expect(
      await screen.findByText(
        /Imported Cards, adding 2 translation keys and 1 asset\. 1 problem to fix: see Problems\./,
      ),
    ).toBeDefined();
  });

  test("an archive whose key the app uses asks for another name and is sent again under it", async () => {
    const projects = { calls: [] as string[], takenPluginKeys: ["cards"] };
    renderApp(path, { summaries, projects });
    await screen.findByRole("button", { name: /^Loans/ });
    choose(archive());
    const dialog = await screen.findByRole("dialog", { name: "Import plugin" });
    expect(dialog.textContent).toContain("already has a plugin with the archive's key");
    fireEvent.change(within(dialog).getByRole("textbox"), { target: { value: "Cards EU" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Import plugin" }));
    await waitFor(() =>
      expect(projects.calls).toEqual(["importPlugin:app1:::4", "importPlugin:app1:cards-eu:Cards EU:4"]),
    );
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Import plugin" }) === null).toBe(true));
  });

  test("a refused import shows the server's reason", async () => {
    const projects = { calls: [] as string[], refuse: { importPlugin: "PLX-1020 a plugin archive holds no app.json" } };
    renderApp(path, { summaries, projects });
    await screen.findByRole("button", { name: /^Loans/ });
    choose(archive());
    expect(
      await screen.findByText(/The plugin was not imported\. PLX-1020 a plugin archive holds no app\.json/),
    ).toBeDefined();
  });

  test("the selected plugin exports from its actions", async () => {
    const projects = { calls: [] as string[] };
    renderApp(path, { summaries, projects });
    fireEvent.pointerDown(await screen.findByRole("button", { name: /^Loans/ }));
    const actions = await screen.findByRole("group", { name: "Plugin actions" });
    fireEvent.click(within(actions).getByRole("button", { name: "Export" }));
    await waitFor(() => expect(projects.calls).toEqual(["exportPlugin:loans"]));
  });
});
