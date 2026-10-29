// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { createStudioI18n } from "@plux/i18n";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { I18nProvider } from "../../i18n-provider.tsx";
import { loadSourceCatalogue } from "../../test-harness.tsx";
import { type DocumentTransport, EditSession } from "../editor/edit-session.ts";
import type { JsonValue } from "../editor/json-patch.ts";
import { UndoManager } from "../editor/undo-stack.ts";
import { setPropRecipe } from "../panels/properties-model.ts";
import { ExtractKeyDialog } from "./extract-key-dialog.tsx";
import type { L10nSessions } from "./l10n-sessions.tsx";
import { LocalisationOverlay } from "./localisation-overlay.tsx";

afterEach(cleanup);

function renderI18n(node: ReactNode) {
  const studio = createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false });
  return render(<I18nProvider studio={studio}>{node}</I18nProvider>);
}

function session(
  docs: Record<string, JsonValue>,
  undo?: { readonly manager: UndoManager; readonly stack: string },
): EditSession {
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
  const s = new EditSession({
    transport,
    debounceMs: 1_000_000,
    maxReplays: 1,
    scheduler: { setTimeout: () => 0, clearTimeout: () => undefined },
    online: () => true,
    // Wired as EditorProvider wires a pipeline to its undo stack.
    ...(undo === undefined
      ? {}
      : {
          stepFor: (_path: string, group: string | undefined) => undo.manager.stepFor(undo.stack, group),
          onEdit: (r: Parameters<UndoManager["record"]>[1]) => undo.manager.record(undo.stack, r),
          onCreated: (r: Parameters<UndoManager["recordCreated"]>[1]) => undo.manager.recordCreated(undo.stack, r),
        }),
  });
  undo?.manager.attach(undo.stack, s);
  for (const [path, content] of Object.entries(docs)) {
    s.open({ path, revision: 1n, content });
  }
  return s;
}

const keysDoc = {
  kind: "translationKeys",
  keys: [
    { id: "k1", key: "home.title" },
    { id: "k2", key: "home.cta" },
  ],
};
let counter = 0;
const newId = () => `id-${++counter}`;

function overlay(over: Partial<Parameters<typeof LocalisationOverlay>[0]> = {}) {
  const keys = session({ "translations/keys.json": keysDoc });
  const messages = session({
    "translations/en.json": { kind: "translations", locale: "en", messages: { k1: "Welcome" } },
  });
  const props = {
    open: true,
    onClose: () => undefined,
    locales: ["en", "de"],
    locale: "en",
    onLocale: () => undefined,
    keysSession: keys,
    messagesSession: messages,
    canEditKeys: true,
    canEditMessages: true,
    otherMessages: new Map([["de", { k1: "Willkommen" }]]),
    pluginKeys: new Set(["k1"]),
    selectionKeys: new Set<string>(),
    schemaVersion: 1,
    newId,
    ...over,
  };
  renderI18n(<LocalisationOverlay {...props} />);
  return { keys, messages };
}

describe("[STU-063] localisation table V4", () => {
  test("lists the plugin's keys with every locale; the others are read-only", async () => {
    overlay();
    expect(await screen.findByRole("table", { name: "Translation keys by locale" })).toBeDefined();
    expect(screen.getByText("home.title")).toBeDefined();
    expect(screen.queryByText("home.cta")).toBeNull();
    expect(screen.getByText("Willkommen")).toBeDefined();
    expect((screen.getByLabelText("home.title in en") as HTMLInputElement).value).toBe("Welcome");
  });

  test("editing a cell goes through the edit pipeline into the locale's document", async () => {
    const { messages } = overlay();
    const cell = await screen.findByLabelText("home.title in en");
    fireEvent.change(cell, { target: { value: "Hello" } });
    expect(messages.store.getState().docs["translations/en.json"]?.content).toEqual({
      kind: "translations",
      locale: "en",
      messages: { k1: "Hello" },
    });
  });

  test("a read-only locale does not change", async () => {
    const { messages } = overlay({ canEditMessages: false });
    const cell = await screen.findByLabelText("home.title in en");
    fireEvent.change(cell, { target: { value: "Hello" } });
    expect(JSON.stringify(messages.store.getState().docs["translations/en.json"]?.content)).toContain("Welcome");
    expect(screen.getByText(/read-only/)).toBeDefined();
  });

  test("a new key is checked and added to the keys document", async () => {
    const { keys } = overlay();
    const field = await screen.findByLabelText("New key");
    fireEvent.click(screen.getByRole("button", { name: "Add key" }));
    expect(await screen.findByText("Enter a key name.")).toBeDefined();
    fireEvent.change(field, { target: { value: "home.title" } });
    fireEvent.click(screen.getByRole("button", { name: "Add key" }));
    expect(await screen.findByText("A key with this name exists already.")).toBeDefined();
    fireEvent.change(field, { target: { value: "home.footer" } });
    fireEvent.click(screen.getByRole("button", { name: "Add key" }));
    const stored = keys.store.getState().docs["translations/keys.json"]?.content as { keys: { key: string }[] };
    expect(stored.keys.map((k) => k.key)).toEqual(["home.title", "home.cta", "home.footer"]);
  });

  test("the scope list widens the table to every key", async () => {
    overlay({ selectionKeys: new Set(["k2"]) });
    expect(await screen.findByText("home.cta")).toBeDefined();
    expect(screen.queryByText("home.title")).toBeNull();
  });
});

describe("[I18N-007] extraction dialog", () => {
  function dialog(canEditKeys: boolean, manager?: UndoManager) {
    const on = (stack: string) => (manager === undefined ? undefined : { manager, stack });
    const keys = session({ "translations/keys.json": { kind: "translationKeys", keys: [] } }, on(""));
    const messages = session(
      { "translations/en.json": { kind: "translations", locale: "en", messages: {} } },
      on("translations/en"),
    );
    const plugin = session(
      { "pages/home.page.json": { root: { id: "n1", type: "Button", props: { text: "Pay now" } } } },
      on("pl"),
    );
    const sessions: L10nSessions = {
      locales: ["en", "de"],
      defaultLocale: "en",
      locale: "en",
      setLocale: () => undefined,
      keys,
      canEditKeys,
      messages,
      canEditMessages: true,
    };
    const replaced: unknown[] = [];
    let closed = 0;
    renderI18n(
      <ExtractKeyDialog
        open
        onClose={() => closed++}
        sessions={sessions}
        text="Pay now"
        prefix="Button"
        schemaVersion={1}
        newId={newId}
        replaceProp={(ref, step) => {
          replaced.push([ref, step]);
          plugin.edit("pages/home.page.json", (d) => setPropRecipe("n1", "compact", "text", ref)(d["root"]), { step });
        }}
      />,
    );
    const content = () =>
      [keys, messages, plugin].map((x) => Object.values(x.store.getState().docs).map((d) => d.content));
    return { keys, messages, replaced, closed: () => closed, content };
  }

  test("proposes a key from the text and replaces the prop on confirmation", async () => {
    const { keys, messages, replaced, closed } = dialog(true);
    const field = (await screen.findByLabelText("New key")) as HTMLInputElement;
    expect(field.value).toBe("button.pay.now");
    fireEvent.click(screen.getByRole("button", { name: "Create key" }));
    await waitFor(() => expect(closed()).toBe(1));
    expect(JSON.stringify(keys.store.getState().docs["translations/keys.json"]?.content)).toContain("button.pay.now");
    expect(JSON.stringify(messages.store.getState().docs["translations/en.json"]?.content)).toContain("Pay now");
    expect(replaced).toHaveLength(1);
  });

  test("[STU-069] the extraction is one client undo step across the keys, the message and the page", async () => {
    const manager = new UndoManager({ depth: 10 });
    const { closed, content } = dialog(true, manager);
    const before = content();
    fireEvent.click(await screen.findByRole("button", { name: "Create key" }));
    await waitFor(() => expect(closed()).toBe(1));
    const after = content();
    expect(after).not.toEqual(before);
    expect(manager.undo("pl").kind).toBe("done");
    expect(content()).toEqual(before);
    expect(manager.canUndo("pl")).toBe(false);
    expect(manager.redo("pl").kind).toBe("done");
    expect(content()).toEqual(after);
  });

  test("an invalid name disables the action; a held lock explains itself", async () => {
    dialog(false);
    expect(await screen.findByText(/once the draft lock/)).toBeDefined();
    expect(screen.getByRole("button", { name: "Create key" }).matches(":disabled, [aria-disabled=true]")).toBe(true);
  });
});
