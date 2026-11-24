// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { evaluateCondition } from "./conditions.ts";
import { bindMocks, type DesignScope, evaluateMockBinding } from "./mock-bindings.ts";
import type { Node } from "./plugin-model.ts";
import { dataRoot, type PassContext, PxlPreview, type ScreenData } from "./pxl-preview.ts";

interface Batch {
  readonly locale: string;
  readonly messages?: Record<string, string>;
  readonly now: number;
  readonly pages: readonly { readonly file: string; readonly data: object }[];
  readonly values: readonly { readonly base?: number; readonly roots: Record<string, unknown> }[];
  readonly items: readonly { readonly page: number; readonly values: number; readonly src: string }[];
}

/** A preview whose engine answers each item from `answer`, with the batches and answered screens it saw. */
function rig(answer: (src: string, roots: Record<string, unknown>) => unknown) {
  const batches: Batch[] = [];
  const answered: string[][] = [];
  let release: () => void = () => undefined;
  const preview = new PxlPreview(
    async (request) => {
      const batch = JSON.parse(request) as Batch;
      batches.push(batch);
      await new Promise<void>((r) => {
        release = r;
      });
      const rootsOf = (i: number): Record<string, unknown> => {
        const set = batch.values[i];
        return set === undefined ? {} : { ...(set.base === undefined ? {} : rootsOf(set.base)), ...set.roots };
      };
      return {
        results: batch.items.map((it) => {
          const v = answer(it.src, rootsOf(it.values));
          return v === undefined ? { error: "does not evaluate" } : { value: v };
        }),
      };
    },
    (screens) => answered.push([...screens].sort()),
    1_000,
  );
  return { preview, batches, answered, release: () => release() };
}

const context = (over: Partial<PassContext> = {}): PassContext => ({
  locale: "en",
  messages: { km: "{n} km" },
  ...over,
});

const data: ScreenData = { mocks: new Map([["orgs", [{ name: "Nordvia" }]]]), states: {} };

const page = { file: "plugins/p/pages/a.page.json", doc: { kind: "page", id: "a" } };
const scope: DesignScope = { params: {}, page: { scale: 2 }, plugin: {}, app: {}, device: { textScale: 1.5 } };
const settle = () => new Promise((r) => setTimeout(r, 0));

describe("ADR-0112 the PXL engine evaluates design-time expressions for the canvas [STU-005]", () => {
  test("a pass asks for what the readers do not resolve, in one batch after it, and the answers redraw its screens", async () => {
    const r = rig((src, roots) =>
      src === "40.0 * device.textScale" ? 40 * Number((roots["device"] as { textScale: number }).textScale) : undefined,
    );
    r.preview.begin(context());
    const evaluate = r.preview.forScreen("a", page, data);
    const at = { ...scope, evaluate };
    // A plain path stays with the readers; the engine is asked only for the rest.
    expect(evaluateMockBinding("page.scale", new Map(), {}, at)).toBe(2);
    expect(evaluateMockBinding("40.0 * device.textScale", new Map(), {}, at)).toBeUndefined();
    expect(evaluateMockBinding("bogus(", new Map(), {}, at)).toBeUndefined();
    expect(r.preview.pending).toBe(true);
    r.preview.flush();
    expect(r.batches).toHaveLength(1);
    const [batch] = r.batches;
    expect(batch?.pages).toEqual([{ file: page.file, data: page.doc }]);
    expect(batch?.items.map((i) => i.src)).toEqual(["40.0 * device.textScale", "bogus("]);
    expect(batch?.values).toHaveLength(1);
    expect(batch?.values[0]?.roots["data"]).toEqual(dataRoot(data.mocks, {}));
    expect(batch?.messages).toEqual({ km: "{n} km" });
    expect(batch?.now).toBe(1_000);
    // Asked again before the answer comes, nothing is sent twice.
    r.preview.begin(context());
    expect(evaluateMockBinding("40.0 * device.textScale", new Map(), {}, at)).toBeUndefined();
    expect(r.preview.pending).toBe(false);
    r.release();
    await settle();
    expect(r.answered).toEqual([["a"]]);
    expect(evaluateMockBinding("40.0 * device.textScale", new Map(), {}, at)).toBe(60);
    // What does not evaluate stays as it is, and is not asked for again.
    expect(evaluateMockBinding("bogus(", new Map(), {}, at)).toBeUndefined();
    expect(r.preview.pending).toBe(false);
    // A bound prop takes the answer.
    const root = {
      id: "t",
      type: "SizedBox",
      props: { width: { $expr: "40.0 * device.textScale" } },
    } as unknown as Node;
    expect(bindMocks(root, new Map(), {}, at) as unknown).toEqual({ id: "t", type: "SizedBox", props: { width: 60 } });
  });

  test("a list item's values extend its page's; other values, another page version or locale ask again", async () => {
    const r = rig((src, roots) => (src === "format.number(item)" ? String(roots["item"]) : undefined));
    r.preview.begin(context());
    const evaluate = r.preview.forScreen("a", page, data);
    const base = { ...scope, evaluate };
    for (const [index, item] of [3, 5].entries()) {
      evaluateMockBinding("format.number(item)", new Map(), {}, { ...base, item, index });
    }
    r.preview.flush();
    const batch = r.batches[0];
    expect(batch?.values.map((v) => v.base)).toEqual([undefined, 0, 0]);
    expect(batch?.values.slice(1).map((v) => v.roots)).toEqual([
      { item: 3, index: 0 },
      { item: 5, index: 1 },
    ]);
    r.release();
    await settle();
    expect(evaluateMockBinding("format.number(item)", new Map(), {}, { ...base, item: 5, index: 1 })).toBe("5");
    // An edited page is a new document: its expressions are asked for again.
    const edited = r.preview.forScreen("a", { ...page, doc: { ...page.doc } }, data);
    expect(
      evaluateMockBinding("format.number(item)", new Map(), {}, { ...base, evaluate: edited, item: 5, index: 1 }),
    ).toBeUndefined();
    // So are they in another locale.
    r.preview.begin(context({ locale: "de" }));
    expect(evaluateMockBinding("format.number(item)", new Map(), {}, { ...base, item: 5, index: 1 })).toBeUndefined();
  });

  test("each page's expressions go in a batch of their own, so each page is drawn as soon as it is answered", () => {
    const r = rig(() => 1);
    r.preview.begin(context());
    const other = { file: "plugins/p/pages/b.page.json", doc: { kind: "page", id: "b" } };
    evaluateMockBinding("1 + 1", new Map(), {}, { ...scope, evaluate: r.preview.forScreen("a", page, data) });
    evaluateMockBinding("2 + 2", new Map(), {}, { ...scope, evaluate: r.preview.forScreen("b", other, data) });
    r.preview.flush();
    expect(r.batches.map((b) => [b.pages.map((p) => p.file), b.items.map((i) => i.src)])).toEqual([
      [[page.file], ["1 + 1"]],
      [[other.file], ["2 + 2"]],
    ]);
  });

  test("the messages go to the engine only when they change", async () => {
    const r = rig(() => "x");
    const ask = (src: string) => {
      evaluateMockBinding(src, new Map(), {}, { ...scope, evaluate: r.preview.forScreen("a", page, data) });
      r.preview.flush();
      r.release();
    };
    r.preview.begin(context());
    ask("t('a')");
    await settle();
    r.preview.begin(context({ messages: { km: "{n} km" } }));
    ask("t('b')");
    await settle();
    expect(r.batches.map((b) => b.messages !== undefined)).toEqual([true, false]);
  });

  test("a condition goes to the engine whole, never by its operands", () => {
    const r = rig(() => true);
    r.preview.begin(context());
    const at = { ...scope, evaluate: r.preview.forScreen("a", page, data) };
    expect(evaluateCondition("page.scale > 1", new Map(), {}, at)).toBe(true);
    expect(r.preview.pending).toBe(false);
    expect(evaluateCondition("(page.scale * 2) > 3", new Map(), {}, at)).toBeUndefined();
    r.preview.flush();
    expect(r.batches[0]?.items.map((i) => i.src)).toEqual(["(page.scale * 2) > 3"]);
  });

  test("a data source reads as PXL's data root under its mock state", () => {
    const mocks = new Map<string, unknown>([
      ["orgs", [{ name: "Nordvia" }]],
      ["me", { name: "A" }],
    ]);
    expect(dataRoot(mocks, { orgs: "empty", me: "error" })).toEqual({
      orgs: { value: [], loading: false, error: null, hasMore: false, status: "ready" },
      me: { value: null, loading: false, error: { message: "" }, hasMore: false, status: "error" },
    });
  });
});
