// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { create } from "@bufbuild/protobuf";
import { FunctionVersionSchema, PluginSchema, PluginSummarySchema } from "@plux/api";
import { createStudioI18n } from "@plux/i18n";
import { createMemoryHistory, createRootRoute, createRouter, RouterProvider } from "@tanstack/react-router";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { I18nProvider } from "../../i18n-provider.tsx";
import { loadSourceCatalogue } from "../../test-harness.tsx";
import { AppFunctions, type DeployedState, type FunctionUse, functionUses } from "./app-functions.tsx";

afterEach(cleanup);

const summary = (id: string, name: string, functionsUsed: string[]) =>
  create(PluginSummarySchema, { plugin: create(PluginSchema, { id, key: id, name }), functionsUsed });
const deployed = (name: string, version: number, testsPassed = true) =>
  create(FunctionVersionSchema, { name, version, placement: "server", testsPassed });

describe("the app's functions on the app canvas (#266)", () => {
  const uses = functionUses(
    [
      summary("loans", "Loans", ["loan.rate", "loan.schedule", "loan.rate"]),
      summary("cards", "Cards", ["loan.rate", "card.limit"]),
    ],
    [deployed("loan.rate", 3), deployed("loan.schedule", 1, false), deployed("audit.export", 2)],
  );

  test("each function once, by name, with the plugins calling it and its latest version", () => {
    expect(uses.map((u) => [u.name, u.plugins.map((p) => p.name), u.deployed?.version])).toEqual([
      ["audit.export", [], 2],
      ["card.limit", ["Cards"], undefined],
      ["loan.rate", ["Cards", "Loans"], 3],
      ["loan.schedule", ["Loans"], 1],
    ]);
  });

  async function renderList(list: readonly FunctionUse[], state: DeployedState, onShow: (id: string) => void) {
    const studio = createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false });
    const rootRoute = createRootRoute({
      component: () => (
        <I18nProvider studio={studio}>
          <AppFunctions uses={list} deployed={state} org="o" app="a" onShowPlugin={onShow} />
        </I18nProvider>
      ),
    });
    const router = createRouter({ routeTree: rootRoute, history: createMemoryHistory({ initialEntries: ["/"] }) });
    render(<RouterProvider router={router} />);
    return screen.findByRole("list", { name: "Functions" });
  }

  test("a called function not deployed, failing tests and an unused one are said; a caller is shown on the canvas", async () => {
    const shown: string[] = [];
    const list = await renderList(uses, "done", (id) => shown.push(id));
    expect(list.textContent).toContain("Not deployed in development");
    expect(list.textContent).toContain("Its latest version's tests fail");
    expect(list.textContent).toContain("No plugin calls it");
    expect(list.textContent).toContain("v3 · server");
    fireEvent.click(screen.getAllByRole("button", { name: "Loans" })[0] as HTMLElement);
    expect(shown).toEqual(["loans"]);
  });

  test("without the environment's functions, nothing is said of deployment", async () => {
    const list = await renderList(
      functionUses([summary("loans", "Loans", ["loan.rate"])], []),
      "failed",
      () => undefined,
    );
    expect(list.textContent).not.toContain("Not deployed");
    expect(screen.getByText(/could not be read/)).toBeTruthy();
  });
});
