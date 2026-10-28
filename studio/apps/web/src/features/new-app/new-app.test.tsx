// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { limit, limitDefault } from "@plux/schema";
import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { orgA, renderApp, type Upstream } from "../../test-harness.tsx";
import { packArchive } from "./plux-archive.ts";
import { jsonOf, readZip } from "./test-zip.ts";

afterEach(cleanup);

const home = `/o/${orgA}`;
const samples = [
  { id: "starter", key: "starter", name: "Starter", description: "The smallest app that runs.", size: 4096 },
  { id: "plux-bank", key: "plux-bank", name: "Plux Bank", description: "A banking app.", size: 8192 },
];
const manager = (over: Upstream = {}): Upstream => ({
  permissions: ["app.read", "app.manage"],
  creation: { calls: [], apps: [] },
  projects: { calls: [], archives: [], samples },
  ...over,
});
const open = async () => {
  fireEvent.click(await screen.findByRole("button", { name: "Create app" }, { timeout: 10000 }));
  return screen.findByRole("dialog", { name: "Create app" });
};
const next = (dialog: HTMLElement) => fireEvent.click(within(dialog).getByRole("button", { name: "Next" }));
const create = (dialog: HTMLElement) => fireEvent.click(within(dialog).getByRole("button", { name: "Create app" }));
const radio = (dialog: HTMLElement, name: string) => fireEvent.click(within(dialog).getByRole("radio", { name }));
const fillDetails = (dialog: HTMLElement, name: string, key?: string) => {
  fireEvent.change(within(dialog).getByLabelText("Name"), { target: { value: name } });
  if (key !== undefined) {
    fireEvent.change(within(dialog).getByLabelText("Key"), { target: { value: key } });
  }
};
const pick = async (name: string, option: string) => {
  fireEvent.click(await screen.findByRole("combobox", { name }));
  const opt = await screen.findByRole("option", { name: option });
  // Base UI ignores a pick made at once after the popup opens: wait, then press and release as a pointer does.
  await new Promise((r) => setTimeout(r, 400));
  fireEvent.pointerDown(opt);
  fireEvent.mouseDown(opt);
  fireEvent.pointerUp(opt);
  fireEvent.mouseUp(opt);
  fireEvent.click(opt);
};

describe("creating a blank app with the wizard [STU-024]", () => {
  test("every option of a blank app is asked for, then the project is imported as one archive and the app opens", async () => {
    const state = manager();
    const r = renderApp(home, state);
    const dialog = await open();
    expect(within(dialog).getByRole("radio", { name: "Blank app" })).toHaveProperty("checked", true);
    next(dialog);

    fillDetails(dialog, "Shop Front");
    expect((within(dialog).getByLabelText("Key") as HTMLInputElement).value).toBe("shop-front");
    fireEvent.change(within(dialog).getByLabelText("Description"), { target: { value: "Sells things" } });
    next(dialog);

    fireEvent.click(within(dialog).getByRole("checkbox", { name: "German (de)" }));
    fireEvent.click(within(dialog).getByRole("checkbox", { name: "Arabic (ar)" }));
    await pick("Default language", "German (de)");
    await waitFor(() => expect(within(dialog).getAllByRole("combobox").at(-1)?.textContent).toBe("German (de)"));
    next(dialog);

    radio(dialog, "Custom colour");
    fireEvent.change(within(dialog).getByLabelText("Primary colour"), { target: { value: "#12ab34" } });
    next(dialog);

    radio(dialog, "Strict");
    next(dialog);

    fireEvent.click(within(dialog).getByRole("checkbox", { name: "Staging" }));
    expect(within(dialog).getByRole("checkbox", { name: "Production (always included)" })).toHaveProperty(
      "disabled",
      true,
    );
    create(dialog);

    await waitFor(() => expect(state.projects?.calls.filter((c) => c.startsWith("import:")).length).toBe(1));
    const call = state.projects?.calls[0] ?? "";
    expect(call.startsWith(`import:${orgA}:shop-front:Shop Front:`)).toBe(true);
    const entries = readZip(state.projects?.archives?.[0] ?? new Uint8Array());
    expect(jsonOf(entries, "project/app.json")).toMatchObject({
      key: "shop-front",
      name: "Shop Front",
      description: "Sells things",
      defaultLocale: "de",
      supportedLocales: ["de", "en", "ar"],
      securityProfile: "strict",
      entryRoute: "home",
      plugins: ["main"],
      environments: [
        { key: "development", name: "Development" },
        { key: "production", name: "Production" },
      ],
    });
    expect(JSON.stringify(jsonOf(entries, "project/theme.json"))).toContain("#12AB34");
    expect(entries.map((e) => e.name)).toContain("project/plugins/main/pages/home.page.json");

    expect(await within(dialog).findByRole("heading", { name: "Shop Front was created" })).toBeTruthy();
    expect(within(dialog).getByText("8 documents written")).toBeTruthy();
    fireEvent.click(within(dialog).getByRole("button", { name: "Open the app" }));
    await waitFor(() => expect(r.router.state.location.pathname).toBe(`${home}/apps/app-shop-front`));
  });

  test("an empty name, a bad key, no language and a bad colour are refused on their steps before any request", async () => {
    const state = manager();
    renderApp(home, state);
    const dialog = await open();
    next(dialog);
    next(dialog);
    await within(dialog).findByText("Enter a name of 1 to 200 characters.");
    fillDetails(dialog, "Shop", "-bad-");
    next(dialog);
    await within(dialog).findByText(/Use 1 to 64 lower-case letters/);
    fillDetails(dialog, "Shop", "shop");
    next(dialog);

    fireEvent.click(within(dialog).getByRole("checkbox", { name: "English (en)" }));
    next(dialog);
    await within(dialog).findByText("Choose at least one language.");
    fireEvent.click(within(dialog).getByRole("checkbox", { name: "English (en)" }));
    next(dialog);

    radio(dialog, "Custom colour");
    next(dialog);
    await within(dialog).findByText("Enter a colour such as #5B3DF5.");
    expect(state.projects?.calls).toEqual([]);
  });

  test("Back returns to the previous step and keeps what was entered", async () => {
    renderApp(home, manager());
    const dialog = await open();
    next(dialog);
    fillDetails(dialog, "Kept");
    next(dialog);
    fireEvent.click(within(dialog).getByRole("button", { name: "Back" }));
    expect((within(dialog).getByLabelText("Name") as HTMLInputElement).value).toBe("Kept");
    expect(within(dialog).getByRole("heading", { name: "Step 2 of 7: Details" })).toBeTruthy();
  });

  test("the AI prompt option is listed but not available yet", async () => {
    renderApp(home, manager());
    const dialog = await open();
    const ai = within(dialog).getByRole("radio", { name: "From an AI prompt" });
    expect(ai).toHaveProperty("disabled", true);
    expect(within(dialog).getByText("Arrives in a later release.")).toBeTruthy();
  });

  test("the server's refusal, such as an app limit, is shown on the last step and nothing opens", async () => {
    const state = manager({
      projects: { calls: [], archives: [], samples, refuse: { import: "the organisation has reached its app limit" } },
    });
    const r = renderApp(home, state);
    const dialog = await open();
    next(dialog);
    fillDetails(dialog, "Extra", "extra");
    for (let i = 0; i < 4; i += 1) {
      next(dialog);
    }
    create(dialog);
    expect((await within(dialog).findByRole("alert")).textContent).toBe("the organisation has reached its app limit");
    expect(r.router.state.location.pathname).toBe(home);
  });

  test("the wizard is also a route of its own, and closing it returns to the home page", async () => {
    const r = renderApp(`${home}/apps/new`, manager());
    const dialog = await screen.findByRole("dialog", { name: "Create app" }, { timeout: 10000 });
    fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(r.router.state.location.pathname).toBe(home));
  });

  test("without app.manage the wizard route is forbidden", async () => {
    renderApp(`${home}/apps/new`, manager({ permissions: ["app.read"] }));
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Create app" }) === null).toBe(true));
  });
});

describe("creating an app from a template [STU-024]", () => {
  test("the server's sample apps are the templates; the choice prefills the name and key and imports the sample", async () => {
    const state = manager();
    const r = renderApp(home, state);
    const dialog = await open();
    radio(dialog, "From a template");
    expect(await within(dialog).findByRole("radio", { name: "Plux Bank" })).toBeTruthy();
    next(dialog);
    await within(dialog).findByText("Choose a template.");
    radio(dialog, "Plux Bank");
    next(dialog);
    expect((within(dialog).getByLabelText("Name") as HTMLInputElement).value).toBe("Plux Bank");
    expect((within(dialog).getByLabelText("Key") as HTMLInputElement).value).toBe("plux-bank");
    expect(within(dialog).getByRole("heading", { name: "Step 2 of 3: Details" })).toBeTruthy();
    fillDetails(dialog, "My Bank", "my-bank");
    create(dialog);
    await waitFor(() => expect(state.projects?.calls).toEqual([`sample:${orgA}:plux-bank:my-bank:My Bank`]));
    expect(await within(dialog).findByRole("heading", { name: "My Bank was created" })).toBeTruthy();
    fireEvent.click(within(dialog).getByRole("button", { name: "Open the app" }));
    await waitFor(() => expect(r.router.state.location.pathname).toBe(`${home}/apps/app-my-bank`));
  });

  test("a server without sample apps says so", async () => {
    renderApp(home, manager({ projects: { calls: [], samples: [] } }));
    const dialog = await open();
    radio(dialog, "From a template");
    expect(await within(dialog).findByText("This server has no sample apps.")).toBeTruthy();
  });
});

describe("creating an app from a .plux archive [STU-024]", () => {
  const pluxFile = async (name = "shop.plux") => {
    const bytes = await packArchive({ key: "shop", name: "Shop", description: "", tags: [] }, [
      { path: "app.json", content: new TextEncoder().encode("{}") },
    ]);
    return new File([bytes], name);
  };
  const choose = async (dialog: HTMLElement, file: File) => {
    radio(dialog, "From a .plux archive");
    const input = await within(dialog).findByLabelText("Archive file");
    fireEvent.change(input, { target: { files: [file] } });
  };

  test("the archive is sent as it is, with progress, and the name and key stay the archive's when left empty", async () => {
    const state = manager();
    renderApp(home, state);
    const dialog = await open();
    const file = await pluxFile();
    await choose(dialog, file);
    expect(await within(dialog).findByText(/^shop\.plux \(/)).toBeTruthy();
    next(dialog);
    expect(within(dialog).getByText("Leave the name and key empty to keep those in the archive.")).toBeTruthy();
    create(dialog);
    await waitFor(() => expect(state.projects?.calls.length).toBe(1));
    expect(state.projects?.calls[0]).toBe(`import:${orgA}:::${file.size}`);
    expect(state.projects?.archives?.[0]?.length).toBe(file.size);
    expect(await within(dialog).findByRole("heading", { name: "imported was created" })).toBeTruthy();
  });

  test("a name and key given here override the archive's", async () => {
    const state = manager();
    renderApp(home, state);
    const dialog = await open();
    await choose(dialog, await pluxFile());
    await within(dialog).findByText(/^shop\.plux \(/);
    next(dialog);
    fillDetails(dialog, "Renamed", "renamed");
    create(dialog);
    await waitFor(() => expect(state.projects?.calls[0]?.startsWith(`import:${orgA}:renamed:Renamed:`)).toBe(true));
  });

  test("a file that is not a .plux archive is refused before it is sent", async () => {
    const state = manager();
    renderApp(home, state);
    const dialog = await open();
    await choose(dialog, await pluxFile("shop.zip"));
    await within(dialog).findByText("The file name must end in .plux.");
    next(dialog);
    expect(within(dialog).getByRole("heading", { name: "Step 1 of 3: Start from" })).toBeTruthy();
    fireEvent.change(within(dialog).getByLabelText("Archive file"), {
      target: { files: [new File([new TextEncoder().encode("just text, not a zip file at all")], "x.plux")] },
    });
    await within(dialog).findByText("The file is not a .plux archive.");
    expect(state.projects?.calls).toEqual([]);
  });

  const filesOf = (count: number) =>
    Array.from({ length: count }, (_, i) => ({ path: `f${String(i).padStart(6, "0")}`, content: new Uint8Array() }));
  const shopApp = { key: "shop", name: "Shop", description: "", tags: [] };

  test("the hint names the default size limit, which an organisation's own limit may replace", async () => {
    renderApp(home, manager());
    const dialog = await open();
    radio(dialog, "From a .plux archive");
    const mebibytes = limitDefault("archive.bytes") / 1048576;
    expect(await within(dialog).findByText(new RegExp(`up to ${mebibytes}\\s?MB by default`))).toBeTruthy();
  });

  test("an archive with more files than any organisation may import is refused before it is sent", async () => {
    const state = manager();
    renderApp(home, state);
    const dialog = await open();
    const max = limit("archive.entries").max;
    await choose(dialog, new File([await packArchive(shopApp, filesOf(max))], "big.plux"));
    await within(dialog).findByText(`The archive holds ${max + 1} files; no organisation can import more than ${max}.`);
    next(dialog);
    expect(within(dialog).getByRole("heading", { name: "Step 1 of 3: Start from" })).toBeTruthy();
    expect(state.projects?.calls).toEqual([]);
  });

  test("more files than the default but within the maximum are sent, and the server's limit refusal is shown", async () => {
    const message = `PLX-1406 the archive holds ${limitDefault("archive.entries") + 1} files; the limit archive.entries is ${limitDefault("archive.entries")}`;
    const state = manager({ projects: { calls: [], archives: [], samples, refuse: { import: message } } });
    renderApp(home, state);
    const dialog = await open();
    await choose(
      dialog,
      new File([await packArchive(shopApp, filesOf(limitDefault("archive.entries") + 1))], "m.plux"),
    );
    await within(dialog).findByText(/^m\.plux \(/);
    next(dialog);
    create(dialog);
    expect((await within(dialog).findByRole("alert")).textContent).toBe(message);
    expect(state.projects?.calls.length).toBe(1);
  });

  test("an archive larger than the default size but within the maximum is sent, and the server's refusal is shown", async () => {
    const message = `PLX-1407 the archive is larger than archive.bytes (${limitDefault("archive.bytes")} bytes)`;
    const state = manager({ projects: { calls: [], archives: [], samples, refuse: { import: message } } });
    renderApp(home, state);
    const dialog = await open();
    const real = await packArchive(shopApp, [
      { path: "big.bin", content: new Uint8Array(limitDefault("archive.bytes")) },
    ]);
    await choose(dialog, new File([real], "large.plux"));
    await within(dialog).findByText(/^large\.plux \(/);
    next(dialog);
    create(dialog);
    expect((await within(dialog).findByRole("alert")).textContent).toBe(message);
    expect(state.projects?.archives?.[0]?.length).toBe(real.length);
    expect(real.length).toBeGreaterThan(limitDefault("archive.bytes"));
  });

  test("an archive that expands past the largest limit is refused before it is sent", async () => {
    const state = manager();
    renderApp(home, state);
    const dialog = await open();
    const small = new Uint8Array(await packArchive(shopApp, [{ path: "a", content: new Uint8Array([1]) }]));
    const view = new DataView(small.buffer);
    const centre = view.getUint32(small.length - 22 + 16, true);
    const first = 46 + view.getUint16(centre + 28, true);
    const max = limit("archive.expandedBytes").max;
    view.setUint32(centre + first + 24, max + 1, true);
    await choose(dialog, new File([small], "bomb.plux"));
    expect(
      (await within(dialog).findByText(/^The archive expands to .*; no organisation can import more than/)) !== null,
    ).toBe(true);
    expect(state.projects?.calls).toEqual([]);
  });

  test("without a file, Next asks for one", async () => {
    renderApp(home, manager());
    const dialog = await open();
    radio(dialog, "From a .plux archive");
    next(dialog);
    await within(dialog).findByText("Choose a .plux file.");
  });

  test("the server's refusal of an archive is shown as it is, with its code", async () => {
    const message = 'PLX-1405 the entry "../x" is outside project/';
    const state = manager({ projects: { calls: [], archives: [], samples, refuse: { import: message } } });
    renderApp(home, state);
    const dialog = await open();
    await choose(dialog, await pluxFile());
    await within(dialog).findByText(/^shop\.plux \(/);
    next(dialog);
    create(dialog);
    expect((await within(dialog).findByRole("alert")).textContent).toBe(message);
    expect(within(dialog).getByRole("button", { name: "Create app" })).toBeTruthy();
  });
});

describe("the generated Flutter project on the last step [STU-024]", () => {
  const finish = async (state: Upstream) => {
    renderApp(home, state);
    const dialog = await open();
    radio(dialog, "From a template");
    await within(dialog).findByRole("radio", { name: "Starter" });
    radio(dialog, "Starter");
    next(dialog);
    create(dialog);
    await within(dialog).findByRole("heading", { name: "Starter was created" });
    return dialog;
  };

  test("the project is generated as a job, followed until it is ready, and downloaded", async () => {
    let downloaded = "";
    const created = URL.createObjectURL;
    URL.createObjectURL = (b: Blob | MediaSource) => {
      downloaded = (b as Blob).type;
      return "blob:project";
    };
    const state = manager({ projects: { calls: [], samples, jobStatuses: ["queued", "running", "succeeded"] } });
    const dialog = await finish(state);
    fireEvent.click(within(dialog).getByRole("button", { name: "Download Flutter project" }));
    expect(await within(dialog).findByText("Downloaded shop-project.zip", {}, { timeout: 5000 })).toBeTruthy();
    URL.createObjectURL = created;
    expect(downloaded).toBe("application/zip");
    expect(state.projects?.calls.filter((c) => !c.startsWith("sample:"))).toEqual([
      "generate:app-starter:production:production",
      "job:job-1",
      "job:job-1",
      "job:job-1",
      "download:job-1",
    ]);
  });

  test("a failed job shows the server's explanation", async () => {
    const state = manager({
      projects: {
        calls: [],
        samples,
        jobStatuses: ["failed"],
        jobError: { code: "PLX-4012", message: "no release to embed" },
      },
    });
    const dialog = await finish(state);
    fireEvent.click(within(dialog).getByRole("button", { name: "Download Flutter project" }));
    expect((await within(dialog).findByRole("alert", {}, { timeout: 5000 })).textContent).toBe(
      "The project could not be generated: no release to embed",
    );
  });

  test("a refusal such as the hourly limit is shown and nothing is polled", async () => {
    const state = manager({
      projects: { calls: [], samples, refuse: { generate: "the app has queued 10 projects this hour" } },
    });
    const dialog = await finish(state);
    fireEvent.click(within(dialog).getByRole("button", { name: "Download Flutter project" }));
    expect((await within(dialog).findByRole("alert")).textContent).toBe(
      "The project could not be generated: the app has queued 10 projects this hour",
    );
    expect(state.projects?.calls.some((c) => c.startsWith("job:"))).toBe(false);
  });
});
