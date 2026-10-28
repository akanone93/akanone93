// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, setDefaultTimeout, test } from "bun:test";
import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { orgA, renderApp, type Upstream } from "../../test-harness.tsx";
import type { ProjectsData, ShellFixture } from "../../test-projects.ts";

setDefaultTimeout(30_000);
afterEach(cleanup);

const wait = { timeout: 10_000 };
const apps = [{ id: "a-bank", key: "bank", name: "Bank" }];
const shellPath = `/o/${orgA}/apps/a-bank/settings/shell`;
const generated: ShellFixture = {
  name: "Bank",
  package: "bank",
  applicationId: "com.example.bank",
  bundleId: "com.example.bank",
  appVersion: "1.0.0+1",
  splashColor: "#FFFFFF",
  router: "",
  deviceApis: ["camera"],
  push: false,
  deepLinkHosts: [],
  deepLinkSchemes: [],
  packages: [],
  entryRoute: "home",
};
const manager = ["app.read", "app.manage"];
const setup = (
  projects: Partial<ProjectsData> = {},
  permissions: string[] = manager,
  assets: string[] = [],
): Upstream => ({
  permissions,
  assets,
  creation: { calls: [], apps },
  projects: { calls: [], shell: { current: generated, generated }, ...projects },
});
const page = async () => {
  await screen.findByRole("heading", { name: "Native shell" }, wait);
  return screen;
};

describe("the native shell settings and the shell update comparison [GEN-004]", () => {
  test("settings equal to those the project was generated from are up to date and list no changes", async () => {
    renderApp(shellPath, setup());
    expect(await screen.findByText("Shell is up to date", {}, wait)).toBeTruthy();
    expect(screen.queryByRole("table") === null).toBe(true);
    expect(screen.queryByText("Shell update required") === null).toBe(true);
  });

  test("an app whose project was never generated says so and needs no update", async () => {
    renderApp(shellPath, setup({ shell: { current: generated } }));
    expect(await screen.findByText("No project generated yet", {}, wait)).toBeTruthy();
    expect(screen.queryByRole("table") === null).toBe(true);
  });

  test("a change that needs a store build is flagged with exactly what changed, from what, and why", async () => {
    const current = { ...generated, appVersion: "1.1.0+2", push: true, deviceApis: ["camera", "location"] };
    renderApp(shellPath, setup({ shell: { current, generated } }));
    expect(await screen.findByText("Shell update required", {}, wait)).toBeTruthy();
    const table = await screen.findByRole("table", { name: "Settings that changed since the project was generated" });
    const rows = within(table).getAllByRole("row").slice(1);
    expect(rows.length).toBe(3);
    const version = within(rows[0] as HTMLElement);
    expect(version.getByRole("rowheader").textContent).toBe("Version");
    expect(version.getByText("1.0.0+1")).toBeTruthy();
    expect(version.getByText("1.1.0+2")).toBeTruthy();
    expect(version.getByText(/shown by the stores/)).toBeTruthy();
    const push = within(rows[2] as HTMLElement);
    expect(push.getByText("Off")).toBeTruthy();
    expect(push.getByText("On")).toBeTruthy();
    expect(within(rows[1] as HTMLElement).getByText("camera,location")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Regenerate project" })).toBeTruthy();
  });

  test("the settings the server holds fill the form", async () => {
    renderApp(shellPath, setup());
    expect(((await screen.findByLabelText("App name", {}, wait)) as HTMLInputElement).value).toBe("Bank");
    expect((screen.getByLabelText("Android application ID") as HTMLInputElement).value).toBe("com.example.bank");
    expect((screen.getByRole("checkbox", { name: "Camera" }) as HTMLInputElement).checked).toBe(true);
    expect((screen.getByRole("checkbox", { name: "Push notifications" }) as HTMLInputElement).checked).toBe(false);
  });

  test("a manager saves the settings, and the comparison then shows the change", async () => {
    const state = setup();
    renderApp(shellPath, state);
    fireEvent.change(await screen.findByLabelText("Version", {}, wait), { target: { value: "2.0.0+7" } });
    fireEvent.change(screen.getByLabelText("Deep-link hosts"), { target: { value: "bank.example, app.example" } });
    fireEvent.click(screen.getByRole("checkbox", { name: "Push notifications" }));
    fireEvent.click(screen.getByRole("checkbox", { name: "Camera" }));
    fireEvent.click(screen.getByRole("button", { name: "Save settings" }));
    expect(await screen.findByText("The shell settings were saved.", {}, wait)).toBeTruthy();
    const call = state.projects?.calls.find((c) => c.startsWith("shell:")) ?? "";
    expect(call.startsWith("shell:a-bank:")).toBe(true);
    expect(JSON.parse(call.slice("shell:a-bank:".length))).toMatchObject({
      appVersion: "2.0.0+7",
      deepLinkHosts: ["bank.example", "app.example"],
      push: true,
      deviceApis: [],
      router: "",
    });
    const table = await screen.findByRole("table", {}, wait);
    expect(within(table).getByText("2.0.0+7")).toBeTruthy();
    expect(await screen.findByText("Shell update required")).toBeTruthy();
  });

  test("the server's refusal of the settings is shown as it is", async () => {
    renderApp(shellPath, setup({ refuse: { shell: "applicationId is not a valid Android application ID" } }));
    await screen.findByLabelText("App name", {}, wait);
    fireEvent.click(screen.getByRole("button", { name: "Save settings" }));
    expect((await screen.findByRole("alert", {}, wait)).textContent).toBe(
      "applicationId is not a valid Android application ID",
    );
  });

  test("without app.manage the settings are shown but cannot be changed or saved", async () => {
    renderApp(shellPath, setup({}, ["app.read"]));
    const name = (await screen.findByLabelText("App name", {}, wait)) as HTMLInputElement;
    expect(name.disabled).toBe(true);
    expect((screen.getByRole("checkbox", { name: "Camera" }) as HTMLInputElement).disabled).toBe(true);
    expect(screen.getByRole("button", { name: "Save settings" }).getAttribute("aria-disabled")).toBe("true");
    expect(screen.getByRole("button", { name: "Regenerate project" }).getAttribute("aria-disabled")).toBe("true");
    expect(screen.getByText("Changing the shell settings needs permission to manage the app.")).toBeTruthy();
  });

  test("without app.read the page is forbidden", async () => {
    renderApp(shellPath, setup({}, []));
    expect(await screen.findByRole("heading", { name: /Forbidden|access|permission/i }, wait)).toBeTruthy();
  });
});

describe("regenerating and downloading the host project from the shell settings [GEN-004]", () => {
  test("the project is generated as a job, followed until ready, and downloaded", async () => {
    let downloaded = "";
    const created = URL.createObjectURL;
    URL.createObjectURL = (b: Blob | MediaSource) => {
      downloaded = (b as Blob).type;
      return "blob:project";
    };
    const state = setup({ jobStatuses: ["queued", "running", "succeeded"] });
    renderApp(shellPath, state);
    fireEvent.click(await screen.findByRole("button", { name: "Regenerate project" }, wait));
    expect(await screen.findByText("Downloaded shop-project.zip", {}, { timeout: 5000 })).toBeTruthy();
    URL.createObjectURL = created;
    expect(downloaded).toBe("application/zip");
    expect(state.projects?.calls.filter((c) => !c.startsWith("shell:"))).toEqual([
      "generate:a-bank:production:production",
      "job:job-1",
      "job:job-1",
      "job:job-1",
      "download:job-1",
    ]);
  });

  test("a failed job shows the server's explanation", async () => {
    renderApp(
      shellPath,
      setup({ jobStatuses: ["failed"], jobError: { code: "PLX-4012", message: "no release to embed" } }),
    );
    fireEvent.click(await screen.findByRole("button", { name: "Regenerate project" }, wait));
    await waitFor(() =>
      expect(screen.getByRole("alert").textContent).toBe("The project could not be generated: no release to embed"),
    );
  });

  test("a refusal such as the hourly limit is shown and nothing is polled", async () => {
    const state = setup({ refuse: { generate: "the app has queued 10 projects this hour" } });
    renderApp(shellPath, state);
    fireEvent.click(await screen.findByRole("button", { name: "Regenerate project" }, wait));
    expect((await screen.findByRole("alert", {}, wait)).textContent).toBe(
      "The project could not be generated: the app has queued 10 projects this hour",
    );
    expect(state.projects?.calls.some((c) => c.startsWith("job:"))).toBe(false);
  });
});

describe("the app icon of the native shell [GEN-004]", () => {
  test("the picker offers the default icon and the app's PNG assets, and the saved icon is selected", async () => {
    const current = { ...generated, iconAssetId: "asset-1" };
    renderApp(shellPath, setup({ shell: { current, generated: current } }, manager, ["images/a.png", "images/b.png"]));
    const group = await screen.findByRole("group", { name: "App icon" }, wait);
    expect(
      (await within(group).findByRole("radio", { name: "images/b.png" }, wait)) as HTMLInputElement,
    ).toHaveProperty("checked", true);
    expect((within(group).getByRole("radio", { name: "Default icon" }) as HTMLInputElement).checked).toBe(false);
    expect(within(group).getAllByRole("radio").length).toBe(3);
  });

  test("choosing an image saves its asset as the icon, and the comparison reports the icon change with its reason", async () => {
    const state = setup({ shell: { current: generated, generated } }, manager, ["images/a.png"]);
    renderApp(shellPath, state);
    fireEvent.click(await screen.findByRole("radio", { name: "images/a.png" }, wait));
    fireEvent.click(screen.getByRole("button", { name: "Save settings" }));
    expect(await screen.findByText("The shell settings were saved.", {}, wait)).toBeTruthy();
    const call = state.projects?.calls.find((c) => c.startsWith("shell:")) ?? "";
    expect(JSON.parse(call.slice("shell:a-bank:".length))).toMatchObject({ iconAssetId: "asset-0" });
    const table = await screen.findByRole("table", {}, wait);
    const row = within(within(table).getAllByRole("row")[1] as HTMLElement);
    expect(row.getByRole("rowheader").textContent).toBe("App icon");
    expect(row.getByText("Default icon")).toBeTruthy();
    expect(row.getByText("asset-0")).toBeTruthy();
    expect(row.getByText(/image files compiled into the native project/)).toBeTruthy();
  });

  test("an app without PNG assets says so, and the default icon stays chosen", async () => {
    renderApp(shellPath, setup());
    expect(await screen.findByText(/The app has no PNG assets yet/, {}, wait)).toBeTruthy();
    expect((screen.getByRole("radio", { name: "Default icon" }) as HTMLInputElement).checked).toBe(true);
  });

  test("without app.manage the icon cannot be changed", async () => {
    renderApp(shellPath, setup({}, ["app.read"], ["images/a.png"]));
    const radio = await screen.findByRole("radio", { name: "images/a.png" }, wait);
    expect((radio as HTMLInputElement).disabled).toBe(true);
  });
});

describe("downloading a patch instead of the whole project [GEN-004]", () => {
  const changed = { ...generated, appVersion: "1.1.0+2" };

  test("the patch is offered beside the regenerate button only while an update is required for a generated project", async () => {
    renderApp(shellPath, setup({ shell: { current: changed, generated } }));
    expect(await screen.findByRole("button", { name: "Download patch" }, wait)).toBeTruthy();
    expect(screen.getByRole("button", { name: "Regenerate project" })).toBeTruthy();
    cleanup();
    renderApp(shellPath, setup());
    await screen.findByRole("button", { name: "Regenerate project" }, wait);
    expect(screen.queryByRole("button", { name: "Download patch" }) === null).toBe(true);
    cleanup();
    renderApp(shellPath, setup({ shell: { current: changed } }));
    await screen.findByRole("button", { name: "Regenerate project" }, wait);
    expect(screen.queryByRole("button", { name: "Download patch" }) === null).toBe(true);
  });

  test("the patch is generated as a patch job, followed until ready, and downloaded", async () => {
    const state = setup({ shell: { current: changed, generated }, jobStatuses: ["running", "succeeded"] });
    renderApp(shellPath, state);
    fireEvent.click(await screen.findByRole("button", { name: "Download patch" }, wait));
    expect(await screen.findByText("Downloaded shop-patch.zip", {}, { timeout: 5000 })).toBeTruthy();
    expect(state.projects?.calls.filter((c) => !c.startsWith("shell:"))).toEqual([
      "generate:a-bank:production:production:patch",
      "job:job-1",
      "job:job-1",
      "download:job-1",
    ]);
  });

  test("a refusal because no project was generated before is shown", async () => {
    const state = setup({
      shell: { current: changed, generated },
      refuse: { generate: "a patch needs a project generated before; generate the project first" },
    });
    renderApp(shellPath, state);
    fireEvent.click(await screen.findByRole("button", { name: "Download patch" }, wait));
    expect((await screen.findByRole("alert", {}, wait)).textContent).toBe(
      "The project could not be generated: a patch needs a project generated before; generate the project first",
    );
  });
});

describe("the link from the app settings to the shell settings [GEN-004]", () => {
  const settings = `/o/${orgA}/apps/a-bank/settings`;

  test("the app settings link to the shell settings, which open", async () => {
    const r = renderApp(settings, setup());
    fireEvent.click(await screen.findByRole("link", { name: "Native shell settings" }, wait));
    await waitFor(() => expect(r.router.state.location.pathname).toBe(shellPath));
    await page();
  });

  test("the link carries a warning while the shell needs an update, and none when it is current", async () => {
    const current = { ...generated, push: true };
    renderApp(settings, setup({ shell: { current, generated } }));
    const link = await screen.findByRole("link", { name: "Native shell settings" }, wait);
    expect(await within(link.parentElement as HTMLElement).findByText("Shell update required", {}, wait)).toBeTruthy();
    cleanup();
    renderApp(settings, setup());
    const calm = await screen.findByRole("link", { name: "Native shell settings" }, wait);
    await waitFor(() => expect(screen.queryByText("Shell update required") === null).toBe(true));
    expect(calm).toBeTruthy();
  });
});
