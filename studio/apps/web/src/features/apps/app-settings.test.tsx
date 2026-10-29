// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, setDefaultTimeout, test } from "bun:test";
import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import type { AppsGridFixture } from "../../test-apps.ts";
import { orgA, renderApp, type Upstream } from "../../test-harness.tsx";

setDefaultTimeout(30_000);
afterEach(cleanup);

const wait = { timeout: 10_000 };
const path = `/o/${orgA}/apps/a-bank/settings`;
const limits = [
  { key: "release.appSize", unit: "bytes", value: 52_428_800, effective: 104_857_600 },
  { key: "app.plugins", unit: "count", value: 170, effective: 200, warning: true },
  { key: "plugin.pages", unit: "count", value: 200, effective: 200 },
  { key: "function.timeout", unit: "milliseconds", value: 0, effective: 5000 },
];
const setup = (permissions: string[]): { grid: AppsGridFixture; state: Upstream } => {
  const grid: AppsGridFixture = {
    apps: [{ id: "a-bank", key: "bank", name: "Bank", description: "Mobile banking." }],
    limits,
    calls: [],
  };
  return { grid, state: { permissions, appsGrid: grid } };
};
const manager = ["app.read", "app.manage"];
const row = async (key: string): Promise<HTMLElement> => {
  const cell = await screen.findByText(key, { selector: "span" }, wait);
  const tr = cell.closest("tr");
  if (tr === null) {
    throw new Error(`no row for ${key}`);
  }
  return tr;
};

describe("the limits usage table of the app settings [STU-025] [LIM-005]", () => {
  test("every limit is listed with its usage against the limit in force", async () => {
    renderApp(path, setup(["app.read"]).state);
    const table = await screen.findByRole("table", { name: "Limits usage" }, wait);
    expect(within(table).getAllByRole("row").length).toBe(limits.length + 1);
    const size = within(await row("release.appSize"));
    expect(size.getByText("Release size")).toBeTruthy();
    expect(size.getByText("100 MiB")).toBeTruthy();
    const meter = size.getByRole("meter", { name: "Release size" });
    expect(meter.getAttribute("aria-valuenow")).toBe("52428800");
    expect(meter.getAttribute("aria-valuetext")).toBe("50 MiB of 100 MiB");
    const timeout = within(await row("function.timeout"));
    expect(timeout.getAllByText("5,000 ms").length).toBeGreaterThan(0);
  });

  test("a limit near or at its limit carries a badge, one within bounds does not", async () => {
    renderApp(path, setup(["app.read"]).state);
    expect(within(await row("app.plugins")).getByText("Near limit")).toBeTruthy();
    expect(within(await row("plugin.pages")).getByText("At limit")).toBeTruthy();
    const calm = within(await row("release.appSize"));
    expect(calm.queryByText("Near limit") === null).toBe(true);
    expect(calm.queryByText("At limit") === null).toBe(true);
  });
});

describe("the usage of one plugin in the app settings [LIM-005] [STU-025]", () => {
  const pluginSetup = (permissions: string[]): { grid: AppsGridFixture; state: Upstream } => {
    const grid: AppsGridFixture = {
      apps: [{ id: "a-bank", key: "bank", name: "Bank" }],
      limits,
      plugins: [
        { id: "p-loans", key: "loans", name: "Loans" },
        { id: "p-cards", key: "cards", name: "Cards" },
        { id: "p-gone", key: "gone", name: "Elsewhere" },
      ],
      pluginLimits: {
        "p-loans": [
          { key: "plugin.pages", unit: "count", value: 12, effective: 200 },
          { key: "bundle.pluginSize", unit: "bytes", value: 1_048_576, effective: 4_194_304 },
        ],
        "p-cards": [{ key: "plugin.pages", unit: "count", value: 3, effective: 200 }],
      },
      calls: [],
    };
    return { grid, state: { permissions, appsGrid: grid } };
  };

  test("the first plugin's usage shows, and choosing another plugin shows its own", async () => {
    const { grid, state } = pluginSetup(["app.read", "plugin.read"]);
    renderApp(path, state);
    const group = await screen.findByRole("group", { name: "Plugins" }, wait);
    const tables = async () => await screen.findAllByRole("table", { name: "Limits usage" }, wait);
    await waitFor(async () => expect((await tables()).length).toBe(2), wait);
    expect(within((await tables())[1] as HTMLElement).getByText("Largest plugin bundle")).toBeTruthy();
    fireEvent.click(within(group).getByRole("button", { name: "Cards" }));
    await waitFor(() => expect(grid.calls).toContain("pluginLimits:p-cards"), wait);
    await waitFor(
      async () => expect(within((await tables())[1] as HTMLElement).queryAllByRole("row").length).toBe(2),
      wait,
    );
  });

  test("a plugin the server does not find shows a failure instead of a table", async () => {
    const { state } = pluginSetup(["app.read", "plugin.read"]);
    renderApp(path, state);
    const group = await screen.findByRole("group", { name: "Plugins" }, wait);
    fireEvent.click(within(group).getByRole("button", { name: "Elsewhere" }));
    expect((await screen.findByText("The plugin's limits could not be loaded.", {}, wait)) !== null).toBe(true);
  });

  test("without plugin.read the section is not offered", async () => {
    renderApp(path, pluginSetup(["app.read"]).state);
    await screen.findByRole("table", { name: "Limits usage" }, wait);
    expect(screen.queryByRole("group", { name: "Plugins" }) === null).toBe(true);
  });
});

describe("the app settings page [STU-023]", () => {
  test("the Settings entry of the app navigation opens it", async () => {
    const { state } = setup(["app.read"]);
    const r = renderApp(`/o/${orgA}/apps/a-bank/releases`, state);
    const nav = await screen.findByRole("navigation", { name: "App areas" }, { timeout: 20_000 });
    fireEvent.click(within(nav).getByRole("link", { name: "Settings" }));
    await waitFor(() => expect(r.router.state.location.pathname).toBe(path));
    expect((await screen.findByRole("heading", { name: "Bank settings" }, wait)) !== null).toBe(true);
  });

  test("a manager changes the name and description", async () => {
    const { grid, state } = setup(manager);
    renderApp(path, state);
    const name = (await screen.findByLabelText("Name", {}, wait)) as HTMLInputElement;
    await waitFor(() => expect(name.value).toBe("Bank"));
    expect((screen.getByLabelText("Description") as HTMLTextAreaElement).value).toBe("Mobile banking.");
    fireEvent.change(name, { target: { value: "Plux Bank" } });
    fireEvent.change(screen.getByLabelText("Description"), { target: { value: "Banking for everyone." } });
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    await screen.findByText("Saved.", {}, wait);
    expect(grid.calls).toContain("update:a-bank:Plux Bank:Banking for everyone.");
    expect(await screen.findByRole("heading", { name: "Plux Bank settings" }, wait)).toBeTruthy();
  });

  test("an empty name is refused before any request", async () => {
    const { grid, state } = setup(manager);
    renderApp(path, state);
    const name = (await screen.findByLabelText("Name", {}, wait)) as HTMLInputElement;
    await waitFor(() => expect(name.value).toBe("Bank"));
    fireEvent.change(name, { target: { value: " " } });
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    await screen.findByText("Enter a name of 1 to 200 characters.");
    expect(grid.calls.filter((c) => c.startsWith("update"))).toEqual([]);
  });

  test("without app.manage the fields are read-only and there is no save button", async () => {
    renderApp(path, setup(["app.read"]).state);
    const name = (await screen.findByLabelText("Name", {}, wait)) as HTMLInputElement;
    expect(name.disabled).toBe(true);
    expect(screen.getByText("You may read these settings but not change them.")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Save changes" }) === null).toBe(true);
  });

  test("links lead to the host fonts, and to the organisation's administration for those who may open it", async () => {
    renderApp(path, setup([...manager, "organization.manage"]).state);
    const fonts = await screen.findByRole("link", { name: "Manage host fonts" }, wait);
    expect(fonts.getAttribute("href")).toBe(`/o/${orgA}/apps/a-bank/design/typography`);
    const admin = await screen.findByRole("link", { name: "Open organisation administration" }, wait);
    expect(admin.getAttribute("href")).toBe(`/o/${orgA}/admin`);
    cleanup();
    renderApp(path, setup(manager).state);
    await screen.findByRole("link", { name: "Manage host fonts" }, wait);
    expect(screen.queryByRole("link", { name: "Open organisation administration" }) === null).toBe(true);
  });
});
