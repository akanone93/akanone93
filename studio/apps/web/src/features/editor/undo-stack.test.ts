// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// STU-069: undo and redo (F3) — one stack per plugin and one for app documents, grouping,
// atomic cross-plugin steps, selection restored, and the rebuild from the server's entries.

import { describe, expect, test } from "bun:test";
import { createRegistry } from "../../lib/commands/registry.ts";
import { createUndoFocus, undoCommands } from "../../lib/commands/undo-commands.ts";
import {
  type DocumentDraft,
  type DocumentTransport,
  EditSession,
  type Scheduler,
  type ServerDocument,
} from "./edit-session.ts";
import { applyPatch, type JsonValue } from "./json-patch.ts";
import { pointerSegments, toImmerPatches, UndoManager } from "./undo-stack.ts";

const path = "screens/login.page.json";

/** Sets a key of an edit draft (documents are index-signature objects). */
function set(d: DocumentDraft, key: string, value: unknown): void {
  d[key] = value;
}

/** A stub server that stores what it is sent; timers never fire on their own. */
function stubTransport(): DocumentTransport & { sent: number } {
  const docs = new Map<string, ServerDocument>();
  const t = {
    sent: 0,
    async patch(p: string, ops: Parameters<DocumentTransport["patch"]>[1], ifRevision: bigint) {
      t.sent++;
      const old = docs.get(p) ?? { path: p, revision: ifRevision, content: {} };
      const next = { path: p, revision: old.revision + 1n, content: applyPatch(old.content, ops) as JsonValue };
      docs.set(p, next);
      return next;
    },
    async get(p: string) {
      return docs.get(p) ?? { path: p, revision: 1n, content: {} };
    },
    classify: () => "rejected" as const,
  };
  return t;
}

const idle: Scheduler = { setTimeout: () => 0, clearTimeout: () => undefined };

function pipeline(undo: UndoManager, stack: string, content: Record<string, JsonValue>) {
  const session = new EditSession({
    transport: stubTransport(),
    debounceMs: 1000,
    maxReplays: 3,
    scheduler: idle,
    online: () => true,
    onEdit: (r) => undo.record(stack, r),
  });
  session.open({ path, revision: 1n, content });
  undo.attach(stack, session);
  return session;
}

function contentOf(s: EditSession): unknown {
  return s.store.getState().docs[path]?.content;
}

function setup(options: Partial<ConstructorParameters<typeof UndoManager>[0]> = {}) {
  let clock = 0;
  let selection: unknown = "none";
  const restored: unknown[] = [];
  const undo = new UndoManager({
    depth: 200,
    now: () => clock,
    selection: () => selection,
    restoreSelection: (s) => restored.push(s),
    ...options,
  });
  return {
    undo,
    restored,
    tick: (ms: number) => {
      clock += ms;
    },
    select: (s: unknown) => {
      selection = s;
    },
  };
}

describe("STU-069 undo stacks", () => {
  test("undo applies the inverse and redo the forward, through the pipeline", () => {
    const { undo } = setup();
    const s = pipeline(undo, "p1", { title: "A" });
    s.edit(path, (d) => {
      set(d, "title", "B");
    });
    expect(undo.canUndo("p1")).toBe(true);
    expect(undo.undo("p1").kind).toBe("done");
    expect(contentOf(s)).toEqual({ title: "A" });
    expect(undo.redo("p1").kind).toBe("done");
    expect(contentOf(s)).toEqual({ title: "B" });
    // Undo and redo were writes, not new steps: one step remains.
    expect(undo.undo("p1").kind).toBe("done");
    expect(undo.undo("p1").kind).toBe("empty");
  });

  test("a typing burst is one step until a pause or a seal; a new edit clears redo", () => {
    const { undo, tick } = setup();
    const s = pipeline(undo, "p1", { title: "" });
    for (const t of ["H", "Hi", "Hi!"]) {
      s.edit(
        path,
        (d) => {
          set(d, "title", t);
        },
        { group: "type:title" },
      );
      tick(200);
    }
    tick(1500);
    s.edit(
      path,
      (d) => {
        set(d, "title", "Hi!!");
      },
      { group: "type:title" },
    );
    undo.undo("p1");
    expect(contentOf(s)).toEqual({ title: "Hi!" });
    undo.undo("p1");
    expect(contentOf(s)).toEqual({ title: "" });
    expect(undo.canRedo("p1")).toBe(true);
    s.edit(path, (d) => {
      set(d, "title", "new");
    });
    expect(undo.canRedo("p1")).toBe(false);

    s.edit(
      path,
      (d) => {
        set(d, "x", 1);
      },
      { group: "drag" },
    );
    undo.seal("p1");
    s.edit(
      path,
      (d) => {
        set(d, "x", 2);
      },
      { group: "drag" },
    );
    undo.undo("p1");
    expect(contentOf(s)).toEqual({ title: "new", x: 1 });
  });

  test("plugins and app documents keep separate stacks", () => {
    const { undo } = setup();
    const a = pipeline(undo, "p1", { v: 1 });
    const app = pipeline(undo, "", { v: 1 });
    a.edit(path, (d) => {
      set(d, "v", 2);
    });
    app.edit(path, (d) => {
      set(d, "v", 3);
    });
    undo.undo("p1");
    expect(contentOf(a)).toEqual({ v: 1 });
    expect(contentOf(app)).toEqual({ v: 3 });
    expect(undo.canUndo("p1")).toBe(false);
    expect(undo.canUndo("")).toBe(true);
  });

  test("a cross-plugin step undoes atomically or not at all", () => {
    const blocked = new Set<string>();
    const { undo } = setup({ blocked: (k) => (blocked.has(k) ? "read-only" : undefined) });
    const a = pipeline(undo, "p1", { screens: ["login"] });
    const b = pipeline(undo, "p2", { screens: [] });
    undo.step(() => {
      a.edit(path, (d) => {
        set(d, "screens", []);
      });
      b.edit(path, (d) => {
        set(d, "screens", ["login"]);
      });
    });
    blocked.add("p2");
    expect(undo.undo("p1")).toEqual({ kind: "blocked", reason: "read-only", stack: "p2" });
    expect(contentOf(a)).toEqual({ screens: [] });
    blocked.clear();
    // A later step of the other plugin sits above it: blocked, nothing changes.
    b.edit(path, (d) => {
      set(d, "extra", true);
    });
    expect(undo.undo("p1")).toEqual({ kind: "blocked", reason: "later-step", stack: "p2" });
    undo.undo("p2");
    const done = undo.undo("p1");
    expect(done.kind).toBe("done");
    expect(contentOf(a)).toEqual({ screens: ["login"] });
    expect(contentOf(b)).toEqual({ screens: [] });
    expect(undo.canUndo("p2")).toBe(false);
    expect(undo.redo("p2").kind).toBe("done");
    expect(contentOf(a)).toEqual({ screens: [] });
    expect(contentOf(b)).toEqual({ screens: ["login"] });
  });

  test("selection is restored on undo and redo but is never a step", () => {
    const { undo, restored, select } = setup();
    const s = pipeline(undo, "p1", { v: 1 });
    select("node-a");
    s.edit(path, (d) => {
      set(d, "v", 2);
    });
    select("node-b");
    s.edit(path, (d) => {
      set(d, "v", 3);
    });
    select("node-c");
    undo.undo("p1");
    expect(restored).toEqual(["node-b"]);
    undo.redo("p1");
    expect(restored).toEqual(["node-b", "node-b"]);
    undo.undo("p1");
    undo.undo("p1");
    expect(restored.at(-1)).toBe("node-a");
    expect(undo.undo("p1").kind).toBe("empty");
  });

  test("a step whose document changed underneath is blocked as a conflict", () => {
    const { undo } = setup();
    pipeline(undo, "p9", { v: 1 });
    undo.rebuild("p9", [{ path, inverse: [{ op: "remove", path: "/missing/deep" }] }]);
    expect(undo.undo("p9")).toEqual({ kind: "blocked", reason: "conflict", stack: "p9" });
  });

  test("the stack is rebuilt from the server's entries, newest first, within the depth", () => {
    const { undo } = setup({ depth: 2 });
    const s = pipeline(undo, "p1", { title: "C" });
    undo.rebuild("p1", [
      { path, inverse: [{ op: "replace", path: "/title", value: "B" }] },
      { path, inverse: [{ op: "replace", path: "/title", value: "A" }] },
      { path, inverse: [{ op: "replace", path: "/title", value: "0" }] },
    ]);
    undo.undo("p1");
    expect(contentOf(s)).toEqual({ title: "B" });
    undo.undo("p1");
    expect(contentOf(s)).toEqual({ title: "A" });
    expect(undo.undo("p1").kind).toBe("empty");
    expect(undo.redo("p1").kind).toBe("done");
    expect(contentOf(s)).toEqual({ title: "B" });
  });

  test("JSON pointers become Immer paths", () => {
    expect(pointerSegments("/a~1b/0/c~0d")).toEqual(["a/b", 0, "c~d"]);
    expect(toImmerPatches([{ op: "test", path: "/a", value: 1 }])).toEqual([]);
  });
});

describe("STU-069 undo commands", () => {
  test("mod+z, shift+mod+z and mod+y reach the focused editor", () => {
    const calls: string[] = [];
    const focus = createUndoFocus();
    const i18n = { _: (d: { message: string }) => d.message } as unknown as Parameters<typeof undoCommands>[0];
    const registry = createRegistry(undoCommands(i18n, focus));
    registry.byShortcut("mod+z")?.run();
    expect(calls).toEqual([]);
    focus.current = { undo: () => calls.push("undo"), redo: () => calls.push("redo") };
    registry.byShortcut("mod+z")?.run();
    registry.byShortcut("shift+mod+z")?.run();
    registry.byShortcut("mod+y")?.run();
    expect(calls).toEqual(["undo", "redo", "redo"]);
  });
});
