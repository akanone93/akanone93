// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, setDefaultTimeout, test } from "bun:test";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { orgA, renderApp } from "../../test-harness.tsx";
import type { EditSession } from "../editor/edit-session.ts";
import type { CanvasSurface } from "../plugin-canvas/surface.ts";
import type { ValidatorClient } from "../problems/validator-client.ts";
import { draftDocument as dataDraft, parseImport } from "./data-source-overlay.tsx";
import { draftDocument as dbDraft, parseRename, splitList } from "./db-schema-overlay.tsx";
import { flattenTokens, tokenValue } from "./theme-overlay.tsx";
import { uuid7 } from "./uuid7.ts";

// Booting the whole app (router, documents, validator) takes seconds under the coverage run.
setDefaultTimeout(30000);
afterEach(cleanup);

const SOURCE = "01a0c450-6c00-7014-8000-000000000a01";
const TASKS = "01a0c450-6c00-7014-8000-000000000b01";
const plugin = {
  id: "pl",
  kind: "plugin",
  entryPage: "home",
  pages: ["home"],
  dataSources: [{ id: SOURCE, name: "rates", kind: "rest", type: "double", mock: 1.5 }],
  collections: [
    {
      id: TASKS,
      key: "tasks",
      version: 1,
      primaryKey: ["id"],
      fields: [
        { name: "id", type: "string" },
        { name: "title", type: "string" },
      ],
    },
  ],
};
const page = {
  kind: "page",
  path: "pages/home.page.json",
  json: { id: "home", key: "home", route: "home", kind: "page", pageKind: "screen", root: { id: "r", children: [] } },
};
const documents = [{ kind: "plugin", path: "plugin.json", json: plugin }, page];
const theme = {
  kind: "theme",
  path: "theme.json",
  json: {
    kind: "theme",
    tokens: {
      color: {
        $type: "color",
        primary: { $value: "#5B3DF5", $extensions: { "dev.plux.dark": "#8B78FF" } },
      },
    },
  },
};
const appDocuments = [{ kind: "app", path: "app.json", json: { kind: "app" } }, theme];
const path = `/o/${orgA}/apps/app1/plugins/pl`;

const surface = () => {
  const s: CanvasSurface = {
    layoutScreens: (docs) => Promise.resolve(docs.map((d) => ({ id: d.id, contentHeight: 300 }))),
    setViewport: () => undefined,
    setPaused: () => undefined,
    pan: () => undefined,
    zoomAbout: () => undefined,
    viewport: { x: 0, y: 0, zoom: 1 },
    dispose: () => undefined,
  };
  return () => s;
};

const toolCalls: { name: string; args: readonly string[] }[] = [];
const validator =
  (
    tool: ((name: string, args: readonly string[]) => unknown) | undefined,
    findings: (data: string) => { code: string; message: string }[] = () => [],
  ) =>
  (): ValidatorClient => ({
    load: async () => [],
    validatePage: async (file, data) => findings(data).map((f) => ({ ...f, severity: "error", file, path: "" })),
    tool: async (name, args) => {
      toolCalls.push({ name, args });
      return tool?.(name, args);
    },
    dispose: () => undefined,
  });

const open = async (name: string) => {
  const buttons = await screen.findAllByRole("button", { name }, { timeout: 10000 });
  const dock =
    buttons.find((b) => b.closest("[aria-label]")?.getAttribute("aria-label") === "Properties and accessibility") ??
    buttons[0];
  fireEvent.click(dock as HTMLElement);
};
const disabled = (el: HTMLElement) => el.matches(":disabled, [aria-disabled=true]");
const content = (edit: EditSession | undefined, file: string) =>
  edit?.store.getState().docs[file]?.content as Record<string, unknown>;

describe("STU-063 V6 data source designer (DAT-080)", () => {
  test("edits a source and its mock, applies a valid draft as a minimal patch, and blocks an invalid one", async () => {
    let edit: EditSession | undefined;
    const sent: NonNullable<Parameters<typeof renderApp>[1]>["sent"] = [];
    renderApp(path, {
      documents,
      appDocuments,
      surface: surface(),
      sent,
      editor: (e) => (edit = e),
      validator: validator(undefined, (data) =>
        data.includes('"bad"') ? [{ code: "PLX-9", message: "bad name" }] : [],
      ),
    });
    await open("Data sources");
    const name = (await screen.findAllByLabelText("Name")).at(0) as HTMLInputElement;
    expect(name.value).toBe("rates");
    const apply = screen.getByRole("button", { name: "Apply" });
    expect(disabled(apply)).toBe(true);

    fireEvent.change(name, { target: { value: "bad" } });
    await waitFor(() => expect(screen.getByText("PLX-9")).toBeTruthy());
    expect(disabled(apply)).toBe(true);

    fireEvent.change(name, { target: { value: "fx" } });
    fireEvent.change(screen.getByLabelText("Mock value (JSON)"), { target: { value: "{" } });
    expect(screen.getByText("The mock is not valid JSON.")).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Mock value (JSON)"), { target: { value: "2.5" } });
    await waitFor(() => expect(disabled(apply)).toBe(false));
    fireEvent.click(apply);
    expect(screen.getByText("Applied to the document.")).toBeTruthy();
    expect((content(edit, "plugin.json")["dataSources"] as Record<string, unknown>[])[0]).toMatchObject({
      name: "fx",
      mock: 2.5,
    });
    edit?.flushAll();
    await waitFor(() => expect(sent.length).toBeGreaterThan(0));
    expect(sent[0]?.path).toBe("plugin.json");
    expect(sent[0]?.ops.filter((o) => o.op !== "test").map((o) => o.path)).toEqual([
      "/dataSources/0/name",
      "/dataSources/0/mock",
    ]);
  });

  test("imports an OpenAPI document in the validator module and adds the result to the draft (L-12)", async () => {
    toolCalls.length = 0;
    let edit: EditSession | undefined;
    const imported = {
      fragment: {
        dataSources: [
          { id: "01a0c450-6c00-7014-8000-000000000a02", name: "pets", kind: "rest", type: "Pet", mock: {} },
        ],
        types: [{ name: "Pet", kind: "record" }],
        variables: [{ name: "petsUrl" }],
      },
      diagnostics: [{ code: "PLX-1", message: "left out a callback" }],
    };
    renderApp(path, {
      documents,
      appDocuments,
      surface: surface(),
      editor: (e) => (edit = e),
      validator: validator(() => imported),
    });
    await open("Data sources");
    fireEvent.change(await screen.findByLabelText("Source text: an OpenAPI document or a GraphQL schema"), {
      target: { value: '{"openapi":"3.0.0"}' },
    });
    fireEvent.click(screen.getByRole("button", { name: "Read the source" }));
    expect(await screen.findByText("PLX-1: left out a callback")).toBeTruthy();
    expect(screen.getByText(/app-level variables/)).toBeTruthy();
    expect(toolCalls[0]?.name).toBe("import");
    expect(toolCalls[0]?.args[0]).toBe("openapi");
    expect(JSON.parse(toolCalls[0]?.args[1] ?? "[]")).toEqual([{ file: "openapi", data: '{"openapi":"3.0.0"}' }]);
    fireEvent.click(screen.getByRole("button", { name: "Add the imported data sources" }));
    const apply = screen.getByRole("button", { name: "Apply" });
    await waitFor(() => expect(disabled(apply)).toBe(false));
    fireEvent.click(apply);
    const doc = content(edit, "plugin.json");
    expect((doc["dataSources"] as unknown[]).length).toBe(2);
    expect(doc["types"]).toEqual([{ name: "Pet", kind: "record" }]);
  });

  test("says so when the importer is missing", async () => {
    renderApp(path, {
      documents,
      appDocuments,
      surface: surface(),
      validator: () => ({ load: async () => [], validatePage: async () => [], dispose: () => undefined }),
    });
    await open("Data sources");
    fireEvent.change(await screen.findByLabelText("Source text: an OpenAPI document or a GraphQL schema"), {
      target: { value: "x" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Read the source" }));
    expect(await screen.findByText("The importer is not available.")).toBeTruthy();
  });
});

describe("STU-063 V7 DB schema designer (DB-005)", () => {
  test("a removed field shows the migration the Go planner computed against the last release, and a plan is applied through the pipeline", async () => {
    toolCalls.length = 0;
    let edit: EditSession | undefined;
    renderApp(path, {
      documents,
      appDocuments,
      published: [{ path: "plugin.json", json: plugin }],
      surface: surface(),
      editor: (e) => (edit = e),
      validator: validator((name, args) =>
        name === "collections" && args[1]?.includes("title")
          ? []
          : [{ code: "PLX-1191", severity: "error", message: "collection tasks lost title" }],
      ),
    });
    await open("DB schema");
    expect(await screen.findByDisplayValue("tasks")).toBeTruthy();
    await waitFor(() => expect(screen.getByText(/devices can update without losing data/)).toBeTruthy());
    fireEvent.click(screen.getAllByRole("button", { name: "Remove field" })[1] as HTMLElement);
    expect(await screen.findByText("collection tasks lost title")).toBeTruthy();
    const call = toolCalls.at(-1);
    expect(call?.name).toBe("collections");
    expect(JSON.parse(call?.args[0] ?? "[]")[0].fields).toHaveLength(2);
    expect(JSON.parse(call?.args[1] ?? "[]")[0].fields).toHaveLength(1);
    fireEvent.click(screen.getByRole("button", { name: "Add migration plan" }));
    fireEvent.change(screen.getByLabelText("Drop fields (comma-separated)"), { target: { value: "title" } });
    fireEvent.change(screen.getByLabelText("Version"), { target: { value: "2" } });
    const apply = screen.getByRole("button", { name: "Apply" });
    await waitFor(() => expect(disabled(apply)).toBe(false));
    fireEvent.click(apply);
    const tasks = (content(edit, "plugin.json")["collections"] as Record<string, unknown>[])[0];
    expect(tasks).toMatchObject({ version: 2, migrations: [{ from: 1, drop: ["title"] }] });
  });

  test("without a published release there is nothing to compare with", async () => {
    renderApp(path, { documents, appDocuments, surface: surface(), validator: validator(undefined) });
    await open("DB schema");
    expect(await screen.findByText(/has not been published yet/)).toBeTruthy();
  });

  test("a removed collection is listed as dropped, and a new one gets a UUIDv7", () => {
    const next = dbDraft(plugin, []);
    expect(next["droppedCollections"]).toEqual([TASKS]);
    expect(dbDraft(next, plugin.collections)["droppedCollections"]).toBeUndefined();
    expect(uuid7()).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/);
    expect(uuid7(0x018f_0000_0000, () => new Uint8Array(16)).startsWith("018f0000-0000-7")).toBe(true);
  });
});

describe("STU-063 V8 theme editor", () => {
  test("edits light and dark values and adds and removes tokens through the edit pipeline", async () => {
    const sent: NonNullable<Parameters<typeof renderApp>[1]>["sent"] = [];
    renderApp(path, {
      documents,
      appDocuments,
      surface: surface(),
      sent,
      validator: validator(undefined),
    });
    await open("Theme");
    const light = (await screen.findByLabelText("Light value of color.primary")) as HTMLInputElement;
    expect(light.value).toBe("#5B3DF5");
    await waitFor(() =>
      expect((screen.getByLabelText("Light value of color.primary") as HTMLInputElement).disabled).toBe(false),
    );
    fireEvent.change(light, { target: { value: "#112233" } });
    fireEvent.change(screen.getByLabelText("Dark value of color.primary"), { target: { value: "#445566" } });
    await waitFor(() =>
      expect((screen.getByLabelText("Light value of color.primary") as HTMLInputElement).value).toBe("#112233"),
    );
    fireEvent.change(screen.getByLabelText("Dark value of color.primary"), { target: { value: "" } });
    fireEvent.change(screen.getByLabelText("New token path, for example color.accent"), {
      target: { value: "color.accent" },
    });
    fireEvent.change(screen.getByLabelText("Value of the new token"), { target: { value: "#FF0000" } });
    fireEvent.click(screen.getByRole("button", { name: "Add token" }));
    expect(await screen.findByLabelText("Light value of color.accent")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Remove token color.accent" }));
    await waitFor(() => expect(screen.queryByLabelText("Light value of color.accent") === null).toBe(true));
    await waitFor(() => expect(sent.length).toBeGreaterThan(0), { timeout: 5000 });
    expect(sent[0]?.path).toBe("theme.json");
    const ops = sent.flatMap((p) => p.ops);
    expect(ops.some((o) => o.path.endsWith("/primary/$value") && o.value === "#112233")).toBe(true);
  });

  test("the theme is edited and saved under the app lock, not the plugin's (ADR-0095, SRV-030)", async () => {
    const sent: NonNullable<Parameters<typeof renderApp>[1]>["sent"] = [];
    const acquired: NonNullable<Parameters<typeof renderApp>[1]>["acquired"] = [];
    renderApp(path, {
      documents,
      appDocuments,
      surface: surface(),
      sent,
      acquired,
      validator: validator(undefined),
    });
    await open("Theme");
    fireEvent.change(await screen.findByLabelText("Light value of color.primary"), { target: { value: "#112233" } });
    await waitFor(() => expect(acquired.length).toBeGreaterThan(0));
    await waitFor(() => expect(sent.length).toBeGreaterThan(0), { timeout: 5000 });
    const theme = sent.filter((p) => p.path === "theme.json");
    expect(theme.length).toBeGreaterThan(0);
    expect(theme.every((p) => p.pluginId === "")).toBe(true);
    expect(acquired.every((l) => l.pluginId === "" && l.scope === "")).toBe(true);
  });

  test("read-only while someone else holds the lock", async () => {
    renderApp(path, {
      documents,
      appDocuments,
      surface: surface(),
      lockHeldElsewhere: true,
      validator: validator(undefined),
    });
    await open("Theme");
    const light = (await screen.findByLabelText("Light value of color.primary")) as HTMLInputElement;
    await waitFor(() => expect(light.disabled).toBe(true));
    expect((await screen.findAllByText(/Ana/)).length).toBeGreaterThan(0);
  });
});

describe("designer models", () => {
  test("flattenTokens inherits the type and reads the dark value", () => {
    const rows = flattenTokens(theme.json.tokens);
    expect(rows).toEqual([{ path: ["color", "primary"], type: "color", light: "#5B3DF5", dark: "#8B78FF" }]);
  });
  test("tokenValue keeps structured values structured", () => {
    expect(tokenValue("x", "a")).toBe("x");
    expect(tokenValue('{"unit":"px","value":4}', { unit: "px", value: 2 })).toEqual({ unit: "px", value: 4 });
    expect(tokenValue("{", { unit: "px" })).toEqual({ unit: "px" });
  });
  test("parseImport reads fragment and findings, and refuses anything else", () => {
    expect(parseImport(null)).toBeUndefined();
    expect(parseImport({ fragment: {}, diagnostics: [{ message: "m" }] })).toEqual({
      dataSources: [],
      types: [],
      variables: [],
      diagnostics: ["m"],
    });
  });
  test("an import never overwrites a type the plugin has", () => {
    const next = dataDraft({ types: [{ name: "Pet", own: true }] }, [], [{ name: "Pet" }, { name: "Cat" }]);
    expect(next["types"]).toEqual([{ name: "Pet", own: true }, { name: "Cat" }]);
  });
  test("list and rename text round-trip", () => {
    expect(splitList(" a, ,b ")).toEqual(["a", "b"]);
    expect(parseRename("n=o, bad, x=")).toEqual({ n: "o" });
  });
});
