// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { type CopyOverIO, copyItems, copyOver, resolveCopies } from "./copy-over.ts";
import { applyResolutions, choiceKey, type ReportItem } from "./envelope.ts";

const item = (id: string, kind: ReportItem["kind"] = "dataSources"): ReportItem => ({
  kind,
  id,
  options: ["copy", "drop"],
  preset: { action: "copy" },
});

const node = (id: string, n: number) => `0190b3a0-0000-7000-8000-${String(id.length * 100 + n).padStart(12, "0")}`;

const counter = () => {
  let n = 0;
  return () => {
    n += 1;
    return `new-${String(n)}`;
  };
};

const io = (readable: ReadonlySet<string>, refuse: ReadonlySet<string> = new Set()) => {
  const writes: { path: string; content: unknown }[] = [];
  const api: CopyOverIO = {
    read: async (_k, id) =>
      readable.has(id)
        ? { content: { id, root: { id: node(id, 1), children: [{ id: node(id, 2), ref: node(id, 1) }] } } }
        : undefined,
    write: async (path, content) => {
      if ([...refuse].some((r) => path.includes(r))) {
        return false;
      }
      writes.push({ path, content });
      return true;
    },
  };
  return { api, writes };
};

describe("[STU-067] copy over at paste time (F7)", () => {
  test("copies across plugins and apps with new IDs and consistent references", async () => {
    const { api, writes } = io(new Set(["ds1", "tok"]));
    const items = [item("ds1"), item("tok", "assets")];
    const result = await copyOver(items, api, { prefix: "plugins/b/", newId: counter() });
    expect(result.dropped).toEqual([]);
    expect(result.written).toHaveLength(2);
    expect(writes[0]?.path).toStartWith("plugins/b/data-sources/");
    const doc = writes[0]?.content as { id: string; root: { id: string; children: { id: string; ref: string }[] } };
    expect(doc.id).not.toBe("ds1");
    expect(doc.root.id).not.toBe(node("ds1", 1));
    expect(doc.root.children[0]?.ref).toBe(doc.root.id);
    const choices = new Map(items.map((i) => [choiceKey(i.kind, i.id), i.preset]));
    const resolved = resolveCopies(choices, items, result);
    const nodes = applyResolutions([{ id: "x", dataSourceId: "ds1" }], resolved);
    expect(nodes).toEqual([{ id: "x", dataSourceId: result.renamed.get("ds1") }]);
  });

  test("an item the user cannot read is dropped and listed, and its reference removed", async () => {
    const { api } = io(new Set(["ds1"]));
    const items = [item("ds1"), item("secret")];
    const result = await copyOver(items, api, { prefix: "", newId: counter() });
    expect(result.dropped).toEqual([{ kind: "dataSources", id: "secret", reason: "forbidden" }]);
    const choices = new Map(items.map((i) => [choiceKey(i.kind, i.id), i.preset]));
    const nodes = applyResolutions([{ id: "x", dataSourceId: "secret" }], resolveCopies(choices, items, result));
    expect(nodes).toEqual([{ id: "x" }]);
  });

  test("a server refusal is dropped and listed", async () => {
    const { api } = io(new Set(["ds1"]), new Set(["data-sources"]));
    const result = await copyOver([item("ds1")], api, { prefix: "", newId: counter() });
    expect(result.dropped).toEqual([{ kind: "dataSources", id: "ds1", reason: "rejected" }]);
  });

  test("the change-set limit caps what is written; the rest is listed", async () => {
    const { api, writes } = io(new Set(["a", "b", "c"]));
    const result = await copyOver([item("a"), item("b"), item("c")], api, { prefix: "", room: 2, newId: counter() });
    expect(writes).toHaveLength(2);
    expect(result.dropped).toEqual([{ kind: "dataSources", id: "c", reason: "limit" }]);
  });

  test("only the items chosen as copy are fetched", () => {
    const items = [item("a"), item("b")];
    const choices = new Map([
      [choiceKey("dataSources", "a"), { action: "copy" as const }],
      [choiceKey("dataSources", "b"), { action: "drop" as const }],
    ]);
    expect(copyItems(items, choices).map((i) => i.id)).toEqual(["a"]);
  });
});
