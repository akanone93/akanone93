// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { createStudioI18n } from "@plux/i18n";
import type { Problem } from "@plux/validator";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { I18nProvider } from "../../i18n-provider.tsx";
import { loadSourceCatalogue } from "../../test-harness.tsx";
import { type DocumentTransport, EditSession } from "../editor/edit-session.ts";
import type { JsonValue } from "../editor/json-patch.ts";
import type { LoadedDoc } from "../safety/index.ts";
import { StateDesignerOverlay } from "./state-designer-overlay.tsx";

afterEach(cleanup);

function renderI18n(node: ReactNode) {
  const studio = createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false });
  return render(<I18nProvider studio={studio}>{node}</I18nProvider>);
}

const pluginPath = "plugins/p1/plugin.json";
const plugin: JsonValue = {
  id: "p1",
  kind: "plugin",
  state: [
    { id: "s1", name: "count", type: "int", default: 1, persistence: "memory" },
    { id: "s2", name: "label", type: "string?", persistence: "session" },
  ],
};
const page = {
  id: "home",
  key: "home",
  kind: "page",
  root: { id: "n1", props: { text: { $expr: "state.count" } } },
};

const stateOf = (doc: unknown): Record<string, unknown>[] => (doc as { state: Record<string, unknown>[] }).state;

function setup(validate?: (file: string, data: string) => Promise<readonly Problem[]>, editable = true) {
  const transport: DocumentTransport = {
    patch: async () => {
      throw new Error("not saved in this test");
    },
    get: async () => {
      throw new Error("unused");
    },
    put: async (path, content) => ({ path, revision: 1n, content }),
    remove: async () => undefined,
    classify: () => "transient",
  };
  const session = new EditSession({
    transport,
    debounceMs: 1_000_000,
    maxReplays: 1,
    scheduler: { setTimeout: () => 0, clearTimeout: () => undefined },
    online: () => true,
  });
  session.open({ path: pluginPath, revision: 1n, content: plugin });
  const documents: LoadedDoc[] = [
    { pluginId: "p1", path: pluginPath, kind: "plugin", revision: 1, json: plugin },
    { pluginId: "p1", path: "plugins/p1/pages/home.page.json", kind: "page", revision: 1, json: page },
  ];
  let closed = 0;
  renderI18n(
    <StateDesignerOverlay
      open
      onClose={() => closed++}
      session={session}
      editable={editable}
      pluginPath={pluginPath}
      pluginId="p1"
      documents={documents}
      validatePage={validate}
      newId={() => "s3"}
    />,
  );
  const state = () => stateOf(session.store.getState().docs[pluginPath]?.content);
  return { state, closed: () => closed };
}

describe("STU-063 state designer V5", () => {
  test("lists the entries with their types and how often they are read", async () => {
    setup();
    const list = await screen.findByRole("navigation", { name: "State entries" });
    expect(list.textContent).toContain("count");
    expect(list.textContent).toContain("Used 1 time");
    expect(list.textContent).toContain("Not used");
  });

  test("adds an entry and changes its type with the pickers", async () => {
    const { state } = setup();
    fireEvent.click(await screen.findByRole("button", { name: "Add state entry" }));
    expect(state().map((e) => e["name"])).toEqual(["count", "label", "value1"]);
    expect(state()[2]).toMatchObject({ id: "s3", type: "string" });
    fireEvent.click(screen.getByLabelText("May be empty"));
    expect(state()[2]?.["type"]).toBe("string?");
  });

  test("renames only to a valid, unused name", async () => {
    const { state } = setup();
    const name = (await screen.findByLabelText("Name")) as HTMLInputElement;
    fireEvent.change(name, { target: { value: "label" } });
    expect(await screen.findByText("Another entry has this name.")).toBeDefined();
    fireEvent.change(name, { target: { value: "Bad" } });
    expect(state()[0]?.["name"]).toBe("count");
    fireEvent.change(name, { target: { value: "total" } });
    expect(state()[0]?.["name"]).toBe("total");
  });

  test("the default is JSON, written when valid and removed when empty", async () => {
    const { state } = setup();
    const field = (await screen.findByLabelText("Default (JSON)")) as HTMLInputElement;
    expect(field.value).toBe("1");
    fireEvent.change(field, { target: { value: "{oops" } });
    expect(await screen.findByText("The default is not valid JSON.")).toBeDefined();
    expect(state()[0]?.["default"]).toBe(1);
    fireEvent.change(field, { target: { value: "7" } });
    expect(state()[0]?.["default"]).toBe(7);
    fireEvent.change(field, { target: { value: "" } });
    expect("default" in (state()[0] ?? {})).toBe(false);
  });

  test("the validator's findings for the entry are shown", async () => {
    const seen: string[] = [];
    setup(async (file) => {
      seen.push(file);
      return [
        { code: "PLX-1001", severity: "error", file, path: "/state/0/default", message: "default is not an int" },
        { code: "PLX-1002", severity: "error", file, path: "/state/1/default", message: "other entry" },
      ];
    });
    expect(await screen.findByText("PLX-1001: default is not an int", {}, { timeout: 3000 })).toBeDefined();
    expect(screen.queryByText(/other entry/)).toBeNull();
    expect(seen[0]).toBe(pluginPath);
  });

  test("an entry that something reads cannot be removed; an unused one can", async () => {
    const { state } = setup();
    fireEvent.click(await screen.findByRole("button", { name: "Remove entry" }));
    expect(await screen.findByText("plugins/p1/pages/home.page.json")).toBeDefined();
    expect(state()).toHaveLength(2);
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    fireEvent.click(await screen.findByRole("button", { name: /label/ }));
    fireEvent.click(screen.getByRole("button", { name: "Remove entry" }));
    await waitFor(() => expect(state().map((e) => e["name"])).toEqual(["count"]));
  });

  test("a read-only editor changes nothing", async () => {
    const { state } = setup(undefined, false);
    expect(await screen.findByText(/read-only/)).toBeDefined();
    const add = screen.getByRole("button", { name: "Add state entry" });
    expect(add.matches(":disabled, [aria-disabled=true]")).toBe(true);
    fireEvent.click(add);
    expect(state()).toHaveLength(2);
  });
});
