// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { type DocumentTransport, EditSession, type ServerDocument } from "../editor/edit-session.ts";
import { applyPatch, type JsonValue } from "../editor/json-patch.ts";
import type { L10nSessions } from "../localisation/index.ts";
import { commitInlineText, inlineTextStyle, textValue } from "./inline-text.tsx";
import type { Node } from "./plugin-model.ts";

/**
 * A session over a server that stores every save, as extract-key.test.ts has: extracting a key waits until the key
 * and its message are stored before the page refers to them (b35be536), so a server that never stores would leave the
 * extraction waiting for ever.
 */
function session(docs: Record<string, JsonValue>): EditSession {
  const stored = new Map<string, ServerDocument>();
  const transport: DocumentTransport = {
    patch: async (path, ops, ifRevision) => {
      const doc = stored.get(path);
      if (doc === undefined || doc.revision !== ifRevision) {
        throw new Error("conflict");
      }
      const next = { path, revision: doc.revision + 1n, content: applyPatch(doc.content, ops) };
      stored.set(path, next);
      return next;
    },
    get: async () => {
      throw new Error("unused");
    },
    put: async (path, content): Promise<ServerDocument> => {
      const doc = { path, revision: 1n, content };
      stored.set(path, doc);
      return doc;
    },
    remove: async (path) => {
      stored.delete(path);
    },
    classify: () => "rejected",
  };
  const s = new EditSession({
    transport,
    debounceMs: 1_000_000,
    maxReplays: 1,
    scheduler: { setTimeout: () => 0, clearTimeout: () => undefined },
    online: () => true,
  });
  for (const [path, content] of Object.entries(docs)) {
    const doc = { path, revision: 1n, content };
    stored.set(path, doc);
    s.open(doc);
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

describe("PC-17 a text edited where it is (#242)", () => {
  const theme = { scheme: { onSurface: 0x111111ff } } as unknown as Parameters<typeof inlineTextStyle>[1];
  const node = {
    id: "t",
    type: "Text",
    props: { data: "Hello", textAlign: "center", style: { fontSize: 40, fontWeight: "w600", color: "#1E88E5FF" } },
  } as Node;

  test("the field takes the text's place and the style the canvas draws it in, at the view's zoom", () => {
    const style = inlineTextStyle(node, theme, { l: 100, t: 200, r: 300, b: 260 }, { x: 0, y: 0, zoom: 0.5 }) as Record<
      string,
      unknown
    >;
    expect(style["fontSize"]).toBe("20px");
    expect(style["fontWeight"]).toBe(600);
    expect(style["color"]).toBe("rgb(30 136 229 / 1.000)");
    expect(style["textAlign"]).toBe("center");
    expect(style["--y"]).toBe(100);
    // Centred: the field's room is shared on both sides of the text's box.
    const w = style["--w"] as number;
    expect(style["--x"]).toBeCloseTo(50 - (w - 100) / 2, 6);
    expect(String(style["fontFamily"])).toStartWith('"Roboto"');
  });

  test("a text without its own colour takes the theme's on-surface colour", () => {
    const plain: Node = { id: "p", type: "Text", props: { data: "Hi" } };
    const style = inlineTextStyle(plain, theme, { l: 0, t: 0, r: 10, b: 10 }, { x: 0, y: 0, zoom: 1 }) as Record<
      string,
      unknown
    >;
    expect(style["color"]).toBe("rgb(17 17 17 / 1.000)");
  });
});
