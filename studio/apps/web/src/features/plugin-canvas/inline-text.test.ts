// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { type DocumentTransport, EditSession, type ServerDocument } from "../editor/edit-session.ts";
import type { JsonValue } from "../editor/json-patch.ts";
import type { L10nSessions } from "../localisation/index.ts";
import { commitInlineText, textValue } from "./inline-text.tsx";
import type { Node } from "./plugin-model.ts";

const transport: DocumentTransport = {
  patch: async () => {
    throw new Error("not saved in this test");
  },
  get: async () => {
    throw new Error("unused");
  },
  put: async (path, content): Promise<ServerDocument> => ({ path, revision: 1n, content }),
  remove: async () => undefined,
  classify: () => "transient",
};

function session(docs: Record<string, JsonValue>): EditSession {
  const s = new EditSession({
    transport,
    debounceMs: 1_000_000,
    maxReplays: 1,
    scheduler: { setTimeout: () => 0, clearTimeout: () => undefined },
    online: () => true,
  });
  for (const [path, content] of Object.entries(docs)) {
    s.open({ path, revision: 1n, content });
  }
  return s;
}

const page = (data: JsonValue): JsonValue => ({
  kind: "page",
  id: "home",
  root: { id: "col", type: "Column", children: [{ id: "t1", type: "Text", props: { data } }] },
});

function setup(data: JsonValue, editable = true) {
  const plugin = session({ "pages/home.page.json": page(data) });
  const keys = session({ "translations/keys.json": { kind: "translationKeys", keys: [{ id: "k0", key: "home.hi" }] } });
  const messages = session({ "translations/en.json": { kind: "translations", locale: "en", messages: { k0: "Hi" } } });
  const sessions = {
    locales: ["en", "de"],
    defaultLocale: "en",
    locale: "en",
    setLocale: () => undefined,
    keys,
    canEditKeys: editable,
    messages,
    canEditMessages: editable,
  } as unknown as L10nSessions;
  const edit = (_doc: string, recipe: (d: unknown) => void, options?: { group?: string; step?: string }) =>
    plugin.edit("pages/home.page.json", (d) => recipe((d as { root: unknown }).root), options);
  const content = () => ({
    page: plugin.store.getState().docs["pages/home.page.json"]?.content as {
      root: { children: { props: { data: unknown } }[] };
    },
    keys: keys.store.getState().docs["translations/keys.json"]?.content as { keys: { id: string; key: string }[] },
    messages: messages.store.getState().docs["translations/en.json"]?.content as { messages: Record<string, string> },
  });
  return { sessions, edit, content };
}

const text = (data: unknown): Node => ({ id: "t1", type: "Text", props: { data } as never });

describe("PC-17 a text edited in place [I18N-007]", () => {
  test("what a text shows: its literal, or its key's message in the source locale", () => {
    const { sessions } = setup("x");
    expect(textValue(text("Hello"), sessions)).toEqual({ text: "Hello" });
    expect(textValue(text({ $t: "k0" }), sessions)).toEqual({ text: "Hi", key: "k0" });
  });

  test("a literal is saved and extracted to a new key named after the page and its words", async () => {
    const { sessions, edit, content } = setup("Hello");
    const key = await commitInlineText({
      edit,
      screen: "home",
      id: "t1",
      text: "Pay now",
      current: { text: "Hello" },
      sessions,
      prefix: "home",
      schemaVersion: 1,
    });
    expect(key).toBe("home.pay.now");
    const after = content();
    const id = after.keys.keys.find((k) => k.key === "home.pay.now")?.id ?? "";
    expect(after.page.root.children[0]?.props.data).toEqual({ $t: id });
    expect(after.messages.messages[id]).toBe("Pay now");
  });

  test("a keyed text changes its source message; without the right to write keys a literal stays a literal", async () => {
    const keyed = setup({ $t: "k0" });
    await commitInlineText({
      edit: keyed.edit,
      screen: "home",
      id: "t1",
      text: "Hello there",
      current: { text: "Hi", key: "k0" },
      sessions: keyed.sessions,
      prefix: "home",
      schemaVersion: 1,
    });
    expect(keyed.content().messages.messages["k0"]).toBe("Hello there");
    const plain = setup("Hello", false);
    const key = await commitInlineText({
      edit: plain.edit,
      screen: "home",
      id: "t1",
      text: "Bye",
      current: { text: "Hello" },
      sessions: plain.sessions,
      prefix: "home",
      schemaVersion: 1,
    });
    expect(key).toBeUndefined();
    expect(plain.content().page.root.children[0]?.props.data).toBe("Bye");
  });
});
