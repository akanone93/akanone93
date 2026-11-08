// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { type DocumentTransport, EditSession, type ServerDocument } from "./edit-session.ts";
import { applyPatch, type JsonValue } from "./json-patch.ts";
import { UndoManager } from "./undo-stack.ts";

const noTimers = { setTimeout: () => 0, clearTimeout: () => undefined };

/** One plugin's pipeline on a server that stores every save, except those of `refuse`; wired as EditorProvider wires it. */
function pipeline(manager: UndoManager, stack: string, docs: Record<string, JsonValue>, refuse?: string) {
  const stored = new Map<string, ServerDocument>();
  const sent: string[] = [];
  const transport: DocumentTransport = {
    patch: async (path, ops, ifRevision, options) => {
      sent.push(`${path} ${options.step ?? ""}`);
      if (path === refuse) {
        throw new Error("refused");
      }
      const doc = stored.get(path) as ServerDocument;
      expect(doc.revision).toBe(ifRevision);
      const next = { path, revision: doc.revision + 1n, content: applyPatch(doc.content, ops) };
      stored.set(path, next);
      return next;
    },
    get: async () => {
      throw new Error("unused");
    },
    put: async (path, content) => ({ path, revision: 1n, content }),
    remove: async () => undefined,
    classify: (error) => ((error as Error).message === "refused" ? "rejected" : "transient"),
  };
  const rejected: string[][] = [];
  const session: EditSession = new EditSession({
    transport,
    debounceMs: 1_000_000,
    maxReplays: 1,
    scheduler: noTimers,
    online: () => true,
    stepFor: (_path, group) => manager.stepFor(stack, group),
    onEdit: (r) => manager.record(stack, r),
    onRejected: (path, _error, _undone, steps) => {
      rejected.push([...steps]);
      manager.abandon(steps, { stack, path });
    },
  });
  manager.attach(stack, session);
  for (const [path, content] of Object.entries(docs)) {
    const doc = { path, revision: 1n, content };
    stored.set(path, doc);
    session.open(doc);
  }
  const content = (path: string) => session.store.getState().docs[path]?.content;
  const server = (path: string) => stored.get(path)?.content;
  return { session, content, server, rejected, sent };
}

describe("[F4] an edit that spans documents applies as a whole or not at all", () => {
  test("a step whose save one plugin refuses is rolled back in the other plugin, on the server too", async () => {
    const manager = new UndoManager({ depth: 10 });
    const from = pipeline(manager, "a", { "plugins/a/pages/p.page.json": { screens: ["home", "cart"] } });
    const to = pipeline(
      manager,
      "b",
      { "plugins/b/pages/p.page.json": { screens: [] } },
      "plugins/b/pages/p.page.json",
    );
    // A cross-plugin move: one step, two documents.
    manager.step(() => {
      from.session.edit("plugins/a/pages/p.page.json", (d) => {
        d["screens"] = ["home"];
      });
      to.session.edit("plugins/b/pages/p.page.json", (d) => {
        d["screens"] = ["cart"];
      });
    });
    expect(await from.session.whenSaved("plugins/a/pages/p.page.json")).toEqual({ screens: ["home"] });
    await to.session.whenSaved("plugins/b/pages/p.page.json");
    expect(to.rejected).toHaveLength(1);
    // The refused plugin is back to the server's version, and the other plugin's part was undone and saved.
    expect(to.content("plugins/b/pages/p.page.json")).toEqual({ screens: [] });
    await from.session.whenSaved("plugins/a/pages/p.page.json");
    expect(from.content("plugins/a/pages/p.page.json")).toEqual({ screens: ["home", "cart"] });
    expect(from.server("plugins/a/pages/p.page.json")).toEqual({ screens: ["home", "cart"] });
    // The rollback is tagged as the step's undo, so the server's rebuild regroups it; nothing is left to undo.
    expect(from.sent.at(-1)).toStartWith("plugins/a/pages/p.page.json undo:");
    expect(manager.canUndo("a")).toBe(false);
    expect(manager.canUndo("b")).toBe(false);
  });

  test("the later edits of a refused step in the same document are not sent on their own", async () => {
    const manager = new UndoManager({ depth: 10 });
    const p = pipeline(manager, "a", { "a.json": { n: 0 } }, "a.json");
    manager.step(() => {
      p.session.edit("a.json", (d) => {
        d["n"] = 1;
      });
    });
    await p.session.whenSaved("a.json");
    expect(p.content("a.json")).toEqual({ n: 0 });
    expect(p.sent).toHaveLength(1);
  });

  test("other steps are untouched by a refusal", async () => {
    const manager = new UndoManager({ depth: 10 });
    const a = pipeline(manager, "a", { "a.json": { n: 0 } });
    const b = pipeline(manager, "b", { "b.json": { n: 0 } }, "b.json");
    a.session.edit("a.json", (d) => {
      d["n"] = 1;
    });
    await a.session.whenSaved("a.json");
    b.session.edit("b.json", (d) => {
      d["n"] = 1;
    });
    await b.session.whenSaved("b.json");
    expect(a.server("a.json")).toEqual({ n: 1 });
    expect(manager.canUndo("a")).toBe(true);
  });
});
