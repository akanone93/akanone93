// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { type DocumentTransport, EditSession, type ServerDocument } from "../editor/edit-session.ts";
import type { JsonValue } from "../editor/json-patch.ts";
import { UndoManager } from "../editor/undo-stack.ts";
import { setPropRecipe } from "../panels/properties-model.ts";
import { extractKey } from "./extract-key.ts";

const noTimers = { setTimeout: () => 0, clearTimeout: () => undefined };

function session(
  docs: Record<string, JsonValue>,
  steps: { path: string; step: string | undefined }[],
  undo?: { readonly manager: UndoManager; readonly stack: string },
): EditSession {
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
  const s = new EditSession({
    transport,
    debounceMs: 1_000_000,
    maxReplays: 1,
    scheduler: noTimers,
    online: () => true,
    onEdit: (r) => {
      steps.push({ path: r.path, step: r.step });
      undo?.manager.record(undo.stack, r);
    },
    // Wired as EditorProvider wires a pipeline to its undo stack.
    ...(undo === undefined
      ? {}
      : {
          stepFor: (_path: string, group: string | undefined) => undo.manager.stepFor(undo.stack, group),
          onCreated: (r: { path: string; content: JsonValue; step: string }) =>
            undo.manager.recordCreated(undo.stack, r),
        }),
  });
  undo?.manager.attach(undo.stack, s);
  for (const [path, content] of Object.entries(docs)) {
    s.open({ path, revision: 1n, content });
  }
  return s;
}

/** The keys, source-locale and page sessions of one plugin canvas, sharing its undo stacks. */
function canvas(docs: { keys?: JsonValue; messages?: JsonValue }) {
  const manager = new UndoManager({ depth: 10 });
  const keys = session(docs.keys === undefined ? {} : { "translations/keys.json": docs.keys }, [], {
    manager,
    stack: "",
  });
  const source = session(docs.messages === undefined ? {} : { "translations/en.json": docs.messages }, [], {
    manager,
    stack: "translations/en",
  });
  const plugin = session({ "pages/home.page.json": page }, [], { manager, stack: "pl" });
  const content = () => ({
    keys: keys.store.getState().docs["translations/keys.json"]?.content,
    messages: source.store.getState().docs["translations/en.json"]?.content,
    page: plugin.store.getState().docs["pages/home.page.json"]?.content,
  });
  const extract = () =>
    extractKey({
      keys,
      source,
      sourceLocale: "en",
      schemaVersion: 1,
      newId,
      key: "home.pay",
      text: "Pay now",
      replaceProp: (reference, step) =>
        plugin.edit("pages/home.page.json", (d) => setPropRecipe("n1", "compact", "text", reference)(d["root"]), {
          step,
        }),
    });
  return { manager, content, extract };
}

const page: JsonValue = { root: { id: "n1", type: "Text", props: { text: "Pay now" } } };
let counter = 0;
const newId = () => `id-${++counter}`;

describe("I18N-007 automatic key extraction", () => {
  test("the key, the source message and the prop change under one step", async () => {
    const steps: { path: string; step: string | undefined }[] = [];
    const keys = session({ "translations/keys.json": { kind: "translationKeys", keys: [] } }, steps);
    const source = session({ "translations/en.json": { kind: "translations", locale: "en", messages: {} } }, steps);
    const plugin = session({ "pages/home.page.json": page }, steps);
    const result = await extractKey({
      keys,
      source,
      sourceLocale: "en",
      schemaVersion: 1,
      newId,
      key: "home.pay",
      text: "Pay now",
      replaceProp: (reference, step) =>
        plugin.edit("pages/home.page.json", (d) => setPropRecipe("n1", "compact", "text", reference)(d["root"]), {
          step,
        }),
    });
    expect(result.ok).toBe(true);
    const id = result.ok ? result.id : "";
    expect(keys.store.getState().docs["translations/keys.json"]?.content).toEqual({
      kind: "translationKeys",
      keys: [{ id, key: "home.pay" }],
    });
    expect(source.store.getState().docs["translations/en.json"]?.content).toEqual({
      kind: "translations",
      locale: "en",
      messages: { [id]: "Pay now" },
    });
    expect(JSON.stringify(plugin.store.getState().docs["pages/home.page.json"]?.content)).toContain(`{"$t":"${id}"}`);
    expect(steps).toHaveLength(3);
    expect(new Set(steps.map((s) => s.step)).size).toBe(1);
    expect(steps[0]?.step).toBeDefined();
  });

  test("an app without keys or messages gets both documents created", async () => {
    const keys = session({}, []);
    const source = session({}, []);
    let replaced: unknown;
    const result = await extractKey({
      keys,
      source,
      sourceLocale: "en",
      schemaVersion: "1.0.0",
      newId,
      key: "home.pay",
      text: "Pay now",
      replaceProp: (reference) => {
        replaced = reference;
      },
    });
    expect(result.ok).toBe(true);
    const created = keys.store.getState().docs["translations/keys.json"]?.content as {
      kind: string;
      schemaVersion: string;
    };
    expect(created.kind).toBe("translationKeys");
    expect(created.schemaVersion).toBe("1.0.0");
    expect(source.store.getState().docs["translations/en.json"]).toBeDefined();
    expect(replaced).toBeDefined();
  });

  test("STU-069 undo reverts the key, the message and the prop as one step; redo re-applies all three", async () => {
    const c = canvas({
      keys: { kind: "translationKeys", keys: [] },
      messages: { kind: "translations", locale: "en", messages: {} },
    });
    const before = c.content();
    expect((await c.extract()).ok).toBe(true);
    const after = c.content();
    const undone = c.manager.undo("pl");
    expect(undone.kind).toBe("done");
    expect(undone.kind === "done" ? [...undone.stacks].sort() : []).toEqual(["", "pl", "translations/en"]);
    expect(c.content()).toEqual(before);
    expect(c.manager.canUndo("")).toBe(false);
    expect(c.manager.canUndo("translations/en")).toBe(false);
    expect(c.manager.redo("pl").kind).toBe("done");
    expect(c.content()).toEqual(after);
  });

  test("STU-069 undo deletes the documents an extraction created; redo creates them again", async () => {
    const c = canvas({});
    expect((await c.extract()).ok).toBe(true);
    const after = c.content();
    expect(after.keys).toBeDefined();
    expect(after.messages).toBeDefined();
    const undone = c.manager.undo("pl");
    expect(undone.kind === "done" && (await undone.written)).toBe(true);
    expect(c.content()).toEqual({ keys: undefined, messages: undefined, page });
    const redone = c.manager.redo("pl");
    expect(redone.kind === "done" && (await redone.written)).toBe(true);
    expect(c.content()).toEqual(after);
  });

  test("a taken or invalid key changes nothing", async () => {
    const keys = session({ "translations/keys.json": { keys: [{ id: "k", key: "home.pay" }] } }, []);
    const source = session({}, []);
    let replaced = 0;
    const base = {
      keys,
      source,
      sourceLocale: "en",
      schemaVersion: 1,
      newId,
      text: "Pay",
      replaceProp: () => {
        replaced++;
      },
    };
    expect(await extractKey({ ...base, key: "home.pay" })).toEqual({ ok: false, reason: "taken" });
    expect(await extractKey({ ...base, key: "Bad Key" })).toEqual({ ok: false, reason: "invalid" });
    expect(replaced).toBe(0);
  });
});
