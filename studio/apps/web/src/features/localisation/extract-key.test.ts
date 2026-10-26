// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { type DocumentTransport, EditSession, type ServerDocument } from "../editor/edit-session.ts";
import type { JsonValue } from "../editor/json-patch.ts";
import { setPropRecipe } from "../panels/properties-model.ts";
import { extractKey } from "./extract-key.ts";

const noTimers = { setTimeout: () => 0, clearTimeout: () => undefined };

function session(docs: Record<string, JsonValue>, steps: { path: string; step: string | undefined }[]): EditSession {
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
    onEdit: (r) => steps.push({ path: r.path, step: r.step }),
  });
  for (const [path, content] of Object.entries(docs)) {
    s.open({ path, revision: 1n, content });
  }
  return s;
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
