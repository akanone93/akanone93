// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { Code, ConnectError } from "@connectrpc/connect";
import { limitDefault } from "@plux/schema";
import { classifySaveError } from "./document-transport.ts";
import { type DocumentTransport, EditSession, type Scheduler, type ServerDocument } from "./edit-session.ts";
import { applyPatch, type JsonPatchOp, type JsonValue } from "./json-patch.ts";

/** The test page's shape, for typed access inside edit recipes. */
interface Page {
  kind: string;
  props: { padding: number; title?: string; color?: string };
  children: string[];
}

function page(d: Record<string, unknown>): Page {
  return d as unknown as Page;
}

const debounceMs = limitDefault("studio.autosaveDebounceMs");
const maxReplays = limitDefault("studio.autosaveReplays");
const path = "screens/login.page.json";

function fakeClock() {
  let now = 0;
  let next = 0;
  const timers = new Map<number, { at: number; fn: () => void }>();
  const scheduler: Scheduler = {
    setTimeout(fn, ms) {
      next += 1;
      timers.set(next, { at: now + ms, fn });
      return next;
    },
    clearTimeout(handle) {
      timers.delete(handle as number);
    },
  };
  return {
    scheduler,
    advance(ms: number) {
      const end = now + ms;
      for (;;) {
        const due = [...timers.entries()].filter(([, t]) => t.at <= end).sort((a, b) => a[1].at - b[1].at)[0];
        if (due === undefined) {
          break;
        }
        timers.delete(due[0]);
        now = due[1].at;
        due[1].fn();
      }
      now = end;
    },
  };
}

interface Call {
  readonly ops: readonly JsonPatchOp[];
  readonly ifRevision: bigint;
  readonly keepalive: boolean;
}

/** A stub DocumentService: applies patches like the server, answering Aborted on a stale revision. */
function stubServer(content: JsonValue) {
  const state = { revision: 1n, content, hold: false, fail: undefined as Code | undefined };
  const calls: Call[] = [];
  const held: (() => void)[] = [];
  const doc = (): ServerDocument => ({ path, revision: state.revision, content: state.content });
  const transport: DocumentTransport = {
    patch(_path, ops, ifRevision, { keepalive }) {
      calls.push({ ops, ifRevision, keepalive });
      const run = (): ServerDocument => {
        if (state.fail !== undefined) {
          throw new ConnectError("stub", state.fail);
        }
        if (ifRevision !== state.revision) {
          throw new ConnectError("stale", Code.Aborted);
        }
        state.content = applyPatch(state.content, ops);
        state.revision += 1n;
        return doc();
      };
      if (!state.hold) {
        return Promise.resolve().then(run);
      }
      return new Promise((resolve, reject) =>
        held.push(() => {
          try {
            resolve(run());
          } catch (e) {
            reject(e);
          }
        }),
      );
    },
    get: () => Promise.resolve(doc()),
    put: (_p, c) => Promise.resolve({ path, revision: 1n, content: c }),
    remove: () => Promise.resolve(),
    classify: classifySaveError,
  };
  /** Another writer saves to the server. */
  const serverEdit = (next: JsonValue) => {
    state.content = next;
    state.revision += 1n;
  };
  return { state, calls, held, transport, doc, serverEdit };
}

const settle = () => new Promise((r) => setTimeout(r, 0));

const base: JsonValue = { kind: "page", props: { padding: 8, title: "Login" } };

function setup(options: { online?: () => boolean } = {}) {
  const clock = fakeClock();
  const server = stubServer(base);
  const confirmed: ServerDocument[] = [];
  const rebased: ServerDocument[] = [];
  const firstEdits: string[] = [];
  const session = new EditSession({
    transport: server.transport,
    debounceMs,
    maxReplays,
    scheduler: clock.scheduler,
    online: options.online ?? (() => true),
    onConfirmed: (d) => confirmed.push(d),
    onRebased: (d) => rebased.push(d),
    onFirstEdit: (p) => firstEdits.push(p),
  });
  session.open(server.doc());
  const set = (key: string, value: JsonValue) =>
    session.edit(path, (d) => {
      (page(d).props as Record<string, unknown>)[key] = value;
    });
  return { clock, server, session, confirmed, rebased, firstEdits, set };
}

describe("SRV-030 autosave pipeline", () => {
  test("debounce comes from studio.autosaveDebounceMs and restarts on each edit", async () => {
    const { clock, server, set } = setup();
    set("padding", 10);
    clock.advance(debounceMs - 1);
    set("padding", 12);
    clock.advance(debounceMs - 1);
    expect(server.calls).toHaveLength(0);
    clock.advance(1);
    await settle();
    expect(server.calls).toHaveLength(1);
    expect(server.state.content).toEqual({ kind: "page", props: { padding: 12, title: "Login" } });
  });

  test("one save in flight; edits meanwhile coalesce and go with the confirmed revision", async () => {
    const { clock, server, session, confirmed, firstEdits, set } = setup();
    server.state.hold = true;
    const record = set("padding", 10);
    expect(record?.inversePatches).toEqual([{ op: "replace", path: ["props", "padding"], value: 8 }]);
    clock.advance(debounceMs);
    expect(server.calls).toHaveLength(1);
    set("title", "Sign in");
    set("color", "red");
    clock.advance(debounceMs);
    expect(server.calls).toHaveLength(1);
    expect(session.waiting()).toBe(3);
    server.held.shift()?.();
    await settle();
    expect(server.calls).toHaveLength(2);
    expect(server.calls[1]?.ifRevision).toBe(2n);
    server.held.shift()?.();
    await settle();
    expect(server.state.content).toEqual({ kind: "page", props: { padding: 10, title: "Sign in", color: "red" } });
    expect(confirmed.map((d) => d.revision)).toEqual([2n, 3n]);
    expect(session.store.getState().docs[path]).toMatchObject({ revision: 3n, waiting: 0, status: "saved" });
    expect(firstEdits).toEqual([path]);
  });

  test("a conflict replays pending edits on the server's version while their tests hold", async () => {
    const { clock, server, session, rebased, set } = setup();
    server.serverEdit({ kind: "page", props: { padding: 8, title: "Welcome" } });
    set("padding", 24);
    clock.advance(debounceMs);
    await settle();
    await settle();
    expect(server.calls.map((c) => c.ifRevision)).toEqual([1n, 2n]);
    expect(server.state.content).toEqual({ kind: "page", props: { padding: 24, title: "Welcome" } });
    expect(rebased).toHaveLength(1);
    expect(session.store.getState().docs[path]?.content).toEqual(server.state.content);
    expect(session.store.getState().conflict).toBeNull();
  });

  test("a clash pauses autosave and opens D35; keep mine applies what fits and lists the rest", async () => {
    const { clock, server, session, set } = setup();
    server.serverEdit({ kind: "page", props: { padding: 4, title: "Login" } });
    set("padding", 24);
    set("title", "Sign in");
    clock.advance(debounceMs);
    await settle();
    await settle();
    const conflict = session.store.getState().conflict;
    expect(conflict?.wouldDrop).toEqual(["/props/padding"]);
    expect(conflict?.differences).toEqual(["/props/padding", "/props/title"]);
    expect(session.store.getState().docs[path]?.status).toBe("conflict");
    set("color", "red");
    clock.advance(debounceMs);
    await settle();
    expect(server.calls).toHaveLength(1);
    expect(session.keepMine()).toEqual(["/props/padding"]);
    await settle();
    expect(server.state.content).toEqual({ kind: "page", props: { padding: 4, title: "Sign in", color: "red" } });
    expect(session.store.getState().conflict).toBeNull();
  });

  test("take the server's discards the pending edits", async () => {
    const { clock, server, session, set } = setup();
    server.serverEdit({ kind: "page", props: { padding: 4, title: "Login" } });
    set("padding", 24);
    clock.advance(debounceMs);
    await settle();
    await settle();
    session.takeServer();
    expect(session.store.getState().docs[path]).toMatchObject({
      content: server.state.content,
      waiting: 0,
      status: "saved",
    });
    expect(server.calls).toHaveLength(1);
  });

  test("replays stop after studio.autosaveReplays conflicts in a row and ask", async () => {
    const { clock, server, session, set } = setup();
    const patch = server.transport.patch.bind(server.transport);
    server.transport.patch = (p, ops, rev, o) => {
      server.serverEdit(server.state.content);
      return patch(p, ops, rev, o);
    };
    set("padding", 24);
    clock.advance(debounceMs);
    for (let i = 0; i < maxReplays + 3; i++) {
      await settle();
    }
    expect(server.calls).toHaveLength(maxReplays + 1);
    expect(session.store.getState().conflict?.wouldDrop).toEqual([]);
  });

  test("hiding the tab flushes at once with keepalive", async () => {
    const { server, session, set } = setup();
    const win = new EventTarget();
    const doc = Object.assign(new EventTarget(), { visibilityState: "visible" });
    const detach = session.attach({ window: win, document: doc });
    set("padding", 10);
    doc.visibilityState = "hidden";
    doc.dispatchEvent(new Event("visibilitychange"));
    await settle();
    expect(server.calls).toEqual([expect.objectContaining({ keepalive: true })]);
    detach();
  });

  test("offline edits wait in memory and are sent in order on reconnect", async () => {
    let online = false;
    const { clock, server, session, set } = setup({ online: () => online });
    const win = new EventTarget();
    session.attach({ window: win, document: Object.assign(new EventTarget(), { visibilityState: "visible" }) });
    set("padding", 10);
    clock.advance(debounceMs);
    set("padding", 11);
    clock.advance(debounceMs);
    set("title", "Hi");
    clock.advance(debounceMs);
    expect(server.calls).toHaveLength(0);
    expect(session.waiting()).toBe(3);
    expect(session.store.getState().docs[path]?.status).toBe("waiting");
    online = true;
    win.dispatchEvent(new Event("online"));
    await settle();
    expect(server.calls).toHaveLength(1);
    const replaced = server.calls[0]?.ops.filter((o) => o.op === "replace").map((o) => o.value);
    expect(replaced).toEqual([10, 11, "Hi"]);
    expect(session.waiting()).toBe(0);
  });

  test("an unreachable server keeps the edits and retries with backoff", async () => {
    const { clock, server, session, set } = setup();
    server.state.fail = Code.Unavailable;
    set("padding", 10);
    clock.advance(debounceMs);
    await settle();
    expect(session.waiting()).toBe(1);
    server.state.fail = undefined;
    clock.advance(30_000);
    await settle();
    expect(server.calls).toHaveLength(2);
    expect(session.waiting()).toBe(0);
  });

  test("a save rejected for its content is undone and reported", async () => {
    const clock = fakeClock();
    const server = stubServer(base);
    const undone: string[] = [];
    const session = new EditSession({
      transport: server.transport,
      debounceMs,
      maxReplays,
      scheduler: clock.scheduler,
      online: () => true,
      onRejected: (_p, _e, paths) => undone.push(...paths),
    });
    session.open(server.doc());
    server.state.fail = Code.InvalidArgument;
    session.edit(path, (d) => {
      page(d).kind = "broken";
    });
    clock.advance(debounceMs);
    await settle();
    expect(undone).toEqual(["/kind"]);
    expect(session.store.getState().docs[path]?.content).toEqual(base);
  });
});
