// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, setDefaultTimeout, test } from "bun:test";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { orgA, renderApp } from "../../test-harness.tsx";
import type { EditSession } from "../editor/edit-session.ts";
import type { ValidatorClient } from "../problems/validator-client.ts";
import { parseIndexes } from "./collections-page.tsx";
import { operationsOf, patched, schemasOf, urlsOf } from "./source-page.tsx";

setDefaultTimeout(30000);
afterEach(cleanup);

const SOURCE = "01a0c450-6c00-7014-8000-000000000a01";
const TASKS = "01a0c450-6c00-7014-8000-000000000b01";
const plugin = {
  id: "pl",
  kind: "plugin",
  entryPage: "home",
  pages: ["home"],
  types: [{ name: "Rate" }],
  dataSources: [
    {
      id: SOURCE,
      name: "rates",
      kind: "rest",
      type: "Rate",
      mock: 1.5,
      config: {
        urls: { production: "https://api.example.com" },
        operations: [{ name: "list", method: "GET", path: "/r" }],
      },
    },
  ],
  collections: [
    {
      id: TASKS,
      key: "tasks",
      version: 2,
      primaryKey: ["id"],
      fields: [
        { name: "id", type: "string" },
        { name: "title", type: "string" },
      ],
      migrations: [{ from: 1 }],
    },
  ],
};
const documents = [{ kind: "plugin", path: "plugin.json", json: plugin }];
const appDocuments = [{ kind: "app", path: "app.json", json: { kind: "app" } }];
const base = `/o/${orgA}/apps/app1/data`;
const disabled = (el: HTMLElement) => el.matches(":disabled, [aria-disabled=true]");
const content = (edit: EditSession | undefined) =>
  edit?.store.getState().docs["plugin.json"]?.content as Record<string, unknown>;

const calls: { name: string; args: readonly string[] }[] = [];
const validator = (tool?: (name: string, args: readonly string[]) => unknown) => (): ValidatorClient => ({
  load: async () => [],
  validatePage: async () => [],
  tool: async (name, args) => {
    calls.push({ name, args });
    return tool?.(name, args);
  },
  dispose: () => undefined,
});
const state = (edit?: (e: EditSession) => void, tool?: (name: string, args: readonly string[]) => unknown) => ({
  permissions: ["app.read"],
  documents,
  appDocuments,
  pluginDocuments: { pl: documents },
  validator: validator(tool),
  ...(edit !== undefined && { editor: edit }),
});

describe("pure helpers", () => {
  test("read the operations, URLs, schemas and indexes of a definition", () => {
    expect(operationsOf({ operations: { get: { method: "GET" } } })).toEqual([{ name: "get", method: "GET" }]);
    expect(operationsOf(null)).toEqual([]);
    expect(urlsOf({ urls: { dev: "u" } })).toEqual([["dev", "u"]]);
    expect(schemasOf([{ name: "Rate" }, { name: "Other" }], "list<Rate>")).toEqual([{ name: "Rate" }]);
    expect(parseIndexes("a, b\n\n c")).toEqual([["a", "b"], ["c"]]);
    expect(patched({ a: 1, b: 2 }, { b: undefined })).toEqual({ a: 1 });
  });
});

describe("STU-113 A20 API definitions (DAT-002)", () => {
  test("lists the sources and imports an OpenAPI definition through the importer", async () => {
    let edit: EditSession | undefined;
    renderApp(
      base,
      state(
        (e) => (edit = e),
        () => ({
          fragment: {
            dataSources: [
              { id: "01a0c450-6c00-7014-8000-000000000a02", name: "pets", kind: "rest", type: "string", mock: "" },
            ],
          },
          diagnostics: [],
        }),
      ),
    );
    expect(await screen.findByText("rates", {}, { timeout: 10000 })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Import an API definition" }));
    fireEvent.change(await screen.findByLabelText("Source text: an OpenAPI document or a GraphQL schema"), {
      target: { value: "{}" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Read the source" }));
    fireEvent.click(await screen.findByRole("button", { name: "Add the imported data sources" }));
    const apply = await screen.findByRole("button", { name: "Apply" });
    await waitFor(() => expect(disabled(apply)).toBe(false));
    fireEvent.click(apply);
    expect((content(edit)["dataSources"] as unknown[]).length).toBe(2);
    expect(calls.at(-1)?.name).toBe("import");
  });
});

describe("STU-113 A21 data source (DAT-080)", () => {
  test("shows operations and URLs, edits route, config and transaction, and the mock in a dialog", async () => {
    let edit: EditSession | undefined;
    renderApp(
      `${base}/sources/${SOURCE}`,
      state((e) => (edit = e)),
    );
    expect(await screen.findByText("https://api.example.com", {}, { timeout: 10000 })).toBeTruthy();
    expect(screen.getByText("/r")).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Configuration (JSON)"), { target: { value: "[" } });
    expect(screen.getByText("The configuration must be a JSON object.")).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Configuration (JSON)"), { target: { value: '{"urls":{}}' } });
    fireEvent.change(screen.getByLabelText("Transaction pointers (JSON)"), {
      target: { value: '{"amount":"/a","currency":"/c","payee":"/p"}' },
    });
    fireEvent.click(screen.getByRole("button", { name: "Edit the mock" }));
    fireEvent.change(await screen.findByLabelText("Mock value (JSON)"), { target: { value: "3" } });
    fireEvent.click(screen.getByRole("button", { name: "Use this mock" }));
    const apply = screen.getByRole("button", { name: "Apply" });
    await waitFor(() => expect(disabled(apply)).toBe(false));
    fireEvent.click(apply);
    const source = (content(edit)["dataSources"] as Record<string, unknown>[])[0];
    expect(source).toMatchObject({ mock: 3, config: { urls: {} }, transaction: { payee: "/p" } });
  });
});

describe("STU-113 A22 and A23 collections (DB-004, DB-005)", () => {
  test("lists collections, edits indexes and previews the migration", async () => {
    let edit: EditSession | undefined;
    const published = [{ path: "plugin.json", json: plugin }];
    renderApp(`${base}/collections/${TASKS}`, {
      ...state(
        (e) => (edit = e),
        () => [{ code: "DB-W1", severity: "warning", message: "title is reset" }],
      ),
      published,
    });
    fireEvent.change(await screen.findByLabelText("Indexes", {}, { timeout: 10000 }), {
      target: { value: "title, id" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Preview the migration" }));
    expect(await screen.findByText("title is reset")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Close the migration preview" }));
    const apply = screen.getByRole("button", { name: "Apply" });
    await waitFor(() => expect(disabled(apply)).toBe(false));
    fireEvent.click(apply);
    expect((content(edit)["collections"] as Record<string, unknown>[])[0]?.["indexes"]).toEqual([["title", "id"]]);
  });

  test("the collections page names each collection and links to it", async () => {
    renderApp(`${base}/collections`, state());
    expect(await screen.findByText("tasks", {}, { timeout: 10000 })).toBeTruthy();
    expect(screen.getByRole("link", { name: "Open" }).getAttribute("href")).toContain(`/collections/${TASKS}`);
  });
});
