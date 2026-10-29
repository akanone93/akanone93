// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { Code, ConnectError } from "@connectrpc/connect";
import { classifySaveError } from "../editor/document-transport.ts";
import { type DocumentTransport, EditSession, type ServerDocument } from "../editor/edit-session.ts";
import type { JsonValue } from "../editor/json-patch.ts";
import {
  blockersOf,
  createScreen,
  deleteScreen,
  duplicateScreen,
  renameScreen,
  type ScreenContext,
} from "./screen-actions.ts";

const enc = (v: unknown) => new TextEncoder().encode(JSON.stringify(v));
const doc = (path: string, kind: string, json: unknown) => ({ path, kind, content: enc(json) });
const home = {
  id: "01e0c450-6c00-7000-8000-00000000000a",
  kind: "page",
  key: "home",
  route: "home",
  title: "Home",
  root: { id: "01e0c450-6c00-7000-8000-00000000000b", type: "Scaffold" },
};
const plugin = { kind: "plugin", id: "p", entryPage: home.id, pages: [home.id] };
const detail = {
  ...home,
  id: "01e0c450-6c00-7000-8000-00000000000c",
  key: "detail",
  route: "detail",
  title: "Detail",
  root: { id: "01e0c450-6c00-7000-8000-00000000000d", type: "Scaffold" },
};
const ctx: ScreenContext = {
  plugin: [
    doc("plugin.json", "plugin", plugin),
    doc("pages/home.page.json", "page", home),
    doc("pages/detail.page.json", "page", detail),
  ],
  app: [],
};

function content(session: EditSession, path: string): Record<string, unknown> {
  return (session.store.getState().docs[path]?.content ?? {}) as Record<string, unknown>;
}

function setup(online = true) {
  const puts: { path: string; content: JsonValue }[] = [];
  const removed: { path: string; rev: bigint }[] = [];
  const patches: unknown[] = [];
  const transport: DocumentTransport = {
    patch: (path, ops) => {
      patches.push(ops);
      return Promise.resolve({ path, revision: 3n, content: {} });
    },
    get: (path) => Promise.resolve({ path, revision: 1n, content: {} }),
    put: (path, content) => {
      puts.push({ path, content });
      return Promise.resolve({ path, revision: 1n, content });
    },
    remove: (path, rev) => {
      removed.push({ path, rev });
      return Promise.resolve();
    },
    classify: classifySaveError,
  };
  const session = new EditSession({ transport, debounceMs: 1000, maxReplays: 3, online: () => online });
  for (const d of ctx.plugin) {
    session.open({
      path: d.path,
      revision: 5n,
      content: JSON.parse(new TextDecoder().decode(d.content)) as JsonValue,
    } as ServerDocument);
  }
  return { session, puts, removed, patches, transport };
}

describe("[STU-050] screens through the pipeline", () => {
  test("create puts a blank page and lists it in the plugin", async () => {
    const t = setup();
    const r = await createScreen(t.session, ctx, "Settings");
    expect(r.ok).toBe(true);
    expect(t.puts[0]?.path).toBe("pages/settings.page.json");
    const pages = content(t.session, "plugin.json")["pages"] as string[];
    const created = t.puts[0]?.content as unknown as { id: string } | undefined;
    expect(pages).toContain(created?.id as string);
    t.session.dispose();
  });

  test("duplicate writes a copy with fresh ids", async () => {
    const t = setup();
    await duplicateScreen(t.session, ctx, home.id, "(copy)");
    const copy = t.puts[0]?.content as { id: string; key: string; root: { id: string } };
    expect(copy.key).toBe("home-copy");
    expect(copy.id).not.toBe(home.id);
    expect(copy.root.id).not.toBe(home.root.id);
    t.session.dispose();
  });

  test("offline, create and delete send nothing and say so", async () => {
    const t = setup(false);
    expect(await createScreen(t.session, ctx, "Settings")).toEqual({ ok: false, reason: "offline" });
    expect(await deleteScreen(t.session, ctx, detail.id)).toEqual({ ok: false, reason: "offline" });
    expect(t.puts).toEqual([]);
    expect(t.removed).toEqual([]);
    t.session.dispose();
  });

  test("a delete is refused while the page has unsaved edits", async () => {
    const t = setup();
    renameScreen(t.session, ctx, detail.id, "Details");
    expect(await deleteScreen(t.session, ctx, detail.id)).toEqual({ ok: false, reason: "pending" });
    t.session.dispose();
  });

  test("delete removes the page at its revision and drops it from the plugin list", async () => {
    const t = setup();
    expect((await deleteScreen(t.session, ctx, detail.id)).ok).toBe(true);
    expect(t.removed).toEqual([{ path: "pages/detail.page.json", rev: 5n }]);
    expect(t.session.store.getState().docs["pages/detail.page.json"]).toBeUndefined();
    t.session.dispose();
  });

  test("delete removes the reference before the document, so a failure leaves an orphan file, never a dangling id", async () => {
    const t = setup();
    const order: string[] = [];
    const realRemove = t.transport.remove;
    t.transport.remove = (path, rev) => {
      order.push(`remove:${JSON.stringify(content(t.session, "plugin.json")["pages"])}`);
      return realRemove(path, rev);
    };
    await deleteScreen(t.session, ctx, detail.id);
    expect(order).toEqual([`remove:${JSON.stringify([home.id])}`]);
    t.session.dispose();
  });

  test("the entry page cannot be deleted and nothing is sent", async () => {
    const t = setup();
    const r = await deleteScreen(t.session, ctx, home.id);
    expect(r).toMatchObject({ ok: false, reason: "blocked", blockers: [{ kind: "entry" }] });
    expect(t.removed).toEqual([]);
    expect(blockersOf(ctx, detail.id)).toEqual([]);
    t.session.dispose();
  });

  test("a rejected delete is reported, not hidden", async () => {
    const t = setup();
    t.session = new EditSession({
      transport: {
        ...({} as DocumentTransport),
        remove: () => Promise.reject(new ConnectError("no", Code.PermissionDenied)),
        classify: classifySaveError,
      },
      debounceMs: 0,
      maxReplays: 1,
      online: () => true,
    });
    t.session.open({ path: "pages/detail.page.json", revision: 1n, content: detail as unknown as JsonValue });
    expect(await deleteScreen(t.session, ctx, detail.id)).toEqual({ ok: false, reason: "rejected" });
  });

  test("rename changes the title and the default route when nothing navigates to it", () => {
    const t = setup();
    renameScreen(t.session, ctx, detail.id, "Details");
    const c = t.session.store.getState().docs["pages/detail.page.json"]?.content as {
      title: string;
      route: string;
      key: string;
    };
    expect([c.title, c.route, c.key]).toEqual(["Details", "details", "detail"]);
    t.session.dispose();
  });

  test("rename keeps the route when another page navigates to it", () => {
    const t = setup();
    const linked = {
      ...ctx,
      plugin: [
        ...ctx.plugin.slice(0, 2),
        doc("pages/home.page.json", "page", {
          ...home,
          root: { id: "r", events: { a: { steps: [{ action: "navigate", id: "s", input: { route: "detail" } }] } } },
        }),
        ctx.plugin[2] as (typeof ctx.plugin)[number],
      ],
    };
    renameScreen(t.session, linked, detail.id, "Details");
    expect(content(t.session, "pages/detail.page.json")["route"]).toBe("detail");
    t.session.dispose();
  });
});
