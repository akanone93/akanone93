// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { actions } from "@plux/schema";
import { layoutSteps } from "./graph-layout.ts";
import {
  addStepRecipe,
  buildCatalogue,
  canConnect,
  connectRecipe,
  edgesOf,
  entryOf,
  exprOf,
  extrasOf,
  freshId,
  groupProblems,
  inputAt,
  literalText,
  literalValue,
  makeEntryRecipe,
  portsOf,
  removeStepRecipe,
  resolveLocation,
  type Step,
  savedPosition,
  setInputRecipe,
  setPositionsRecipe,
  stepsAt,
  stepsPointer,
  targetOf,
} from "./graph-model.ts";

const inline = { kind: "inline", path: "pages/home.page.json", nodeId: "btn", event: "onPressed" } as const;
const page = () => ({
  kind: "page",
  root: { id: "root", type: "Column", children: [{ id: "btn", type: "ElevatedButton", events: {} }] },
});
const catalogue = buildCatalogue({
  custom: [
    {
      name: "scanReceipt",
      description: "Scans.",
      inputs: [{ name: "side", type: "string", required: true }],
      output: "Receipt",
    },
  ],
  functions: ["score"],
  flows: ["checkout"],
});
const entry = (key: string) => {
  const e = catalogue.find((c) => c.key === key);
  if (e === undefined) {
    throw new Error(key);
  }
  return e;
};

describe("[STU-065] action graph model", () => {
  test("the catalogue has every action of the registry (A4.5), then custom actions [ACT-060], functions and flows", () => {
    const builtin = catalogue.filter((e) => e.group === "builtin");
    expect(builtin.map((e) => e.action)).toEqual(
      [...actions.map((a) => a.name)].sort((a, b) => a.localeCompare(b, "en")),
    );
    expect(catalogue.filter((e) => e.group !== "builtin").map((e) => e.key)).toEqual([
      "custom:scanReceipt",
      "function:score",
      "flow:checkout",
    ]);
    const custom = entry("custom:scanReceipt");
    expect(custom).toMatchObject({ action: "callNative", preset: { action: "scanReceipt" }, output: "Receipt" });
    expect(custom.fields.map((f) => f.path.join("."))).toEqual(["input.side"]);
    expect(entry("function:score").preset).toEqual({ function: "score" });
    expect(entry("condition").branches).toEqual(["then", "else"]);
  });

  test("a step is the entry of its catalogue line: custom, function and flow steps keep their own names", () => {
    const step: Step = { id: "a", action: "callNative", input: { action: "scanReceipt" } };
    expect(entryOf(step, catalogue)?.key).toBe("custom:scanReceipt");
    expect(entryOf({ id: "b", action: "callNative", input: { action: "unknown" } }, catalogue)?.key).toBe("callNative");
    expect(entryOf({ id: "c", action: "navigate" }, catalogue)?.key).toBe("navigate");
  });

  test("ports follow §5: named branches, or onSuccess when the action returns a value, or next; onError always", () => {
    const names = (action: string, input?: Record<string, unknown>) =>
      portsOf(entryOf({ id: "s", action }, catalogue), { id: "s", action, ...(input ? { input } : {}) }).map(
        (p) => p.name,
      );
    expect(names("condition")).toEqual(["then", "else", "onError"]);
    expect(names("navigate")).toEqual(["next", "onError"]);
    expect(names("kvGet")).toEqual(["onSuccess", "onError"]);
    const typed = portsOf(entryOf({ id: "s", action: "kvGet" }, catalogue), { id: "s", action: "kvGet" })[0];
    expect(typed?.type).not.toBe("");
    const from = actions.find((a) => a.branchesFrom !== undefined);
    if (from?.branchesFrom !== undefined) {
      const step = { id: "s", action: from.name, input: { [from.branchesFrom]: ["alpha"] } };
      expect(portsOf(entryOf(step, catalogue), step).map((p) => p.name)).toContain("alpha");
    }
  });

  test("addStepRecipe creates the handler, fixes the preset and required inputs, and continues from the last step", () => {
    const doc = page();
    addStepRecipe(inline, entry("kvGet"), "kvGet1", catalogue)(doc);
    addStepRecipe(inline, entry("function:score"), "invokeFunction1", catalogue, "kvGet1")(doc);
    const steps = stepsAt(doc, inline);
    expect(steps.map((s) => s.id)).toEqual(["kvGet1", "invokeFunction1"]);
    expect(steps[0]).toMatchObject({ input: { key: "" }, onSuccess: "invokeFunction1" });
    expect(steps[1]?.input).toEqual({ function: "score" });
    expect(freshId(steps, "kvGet")).toBe("kvGet2");
    expect(freshId([], "kvGet")).toBe("kvGet1");
  });

  test("connect, disconnect and remove keep edges consistent, for ports and named branches", () => {
    const doc = page();
    for (const [e, id] of [
      [entry("condition"), "condition1"],
      [entry("navigate"), "navigate1"],
      [entry("pop"), "pop1"],
    ] as const) {
      addStepRecipe(inline, e, id, catalogue)(doc);
    }
    const cond = () => stepsAt(doc, inline)[0] as Step;
    const then = portsOf(entryOf(cond(), catalogue), cond())[0];
    if (then === undefined) {
      throw new Error("port");
    }
    connectRecipe(inline, "condition1", then, "navigate1")(doc);
    connectRecipe(inline, "condition1", { name: "onError", kind: "onError", type: "" }, "pop1")(doc);
    expect(cond().branches?.["then"]).toBe("navigate1");
    expect(cond().onError).toBe("pop1");
    expect(edgesOf(stepsAt(doc, inline), catalogue).map((e) => e.id)).toEqual([
      "condition1:then",
      "condition1:onError",
    ]);
    expect(targetOf(cond(), then)).toBe("navigate1");
    connectRecipe(inline, "condition1", then, undefined)(doc);
    expect(cond().branches).toBeUndefined();
    connectRecipe(inline, "condition1", then, "navigate1")(doc);
    removeStepRecipe(inline, "navigate1")(doc);
    expect(cond().branches).toBeUndefined();
    expect(cond().onError).toBe("pop1");
    makeEntryRecipe(inline, "pop1")(doc);
    expect(stepsAt(doc, inline).map((s) => s.id)).toEqual(["pop1", "condition1"]);
  });

  test("inputs are set at a path, cleared with undefined, and PXL values are recognised", () => {
    const doc = page();
    addStepRecipe(inline, entry("custom:scanReceipt"), "callNative1", catalogue)(doc);
    setInputRecipe(inline, "callNative1", ["input", "side"], { $expr: "state.side" })(doc);
    const step = stepsAt(doc, inline)[0] as Step;
    expect(exprOf(inputAt(step, ["input", "side"]))).toBe("state.side");
    setInputRecipe(inline, "callNative1", ["input", "side"], undefined)(doc);
    expect(inputAt(stepsAt(doc, inline)[0] as Step, ["input", "side"])).toBeUndefined();
    expect(literalText({ a: 1 })).toBe('{"a":1}');
    expect(literalText(undefined)).toBe("");
    expect(literalValue("12", "int")).toBe(12);
    expect(literalValue("12", "string")).toBe("12");
    expect(literalValue("nope", "int")).toBe("nope");
    expect(literalValue("", "int")).toBeUndefined();
  });

  test("a step never connects to itself or into the entry", () => {
    const steps: Step[] = [
      { id: "a", action: "pop" },
      { id: "b", action: "pop" },
    ];
    expect(canConnect(steps, "a", "b")).toBe(true);
    expect(canConnect(steps, "b", "a")).toBe(false);
    expect(canConnect(steps, "a", "a")).toBe(false);
    expect(canConnect(steps, "a", "zz")).toBe(false);
  });

  test("a $graph reference resolves to its document by id or key, and a missing one to nothing", () => {
    const docs = {
      "pages/home.page.json": { root: { id: "btn", events: { onPressed: { $graph: "pay" } } } },
      "actions/pay.graph.json": { kind: "actionGraph", id: "g1", key: "pay", steps: [{ id: "x", action: "pop" }] },
    };
    const found = resolveLocation(docs, "pages/home.page.json", "btn", "onPressed");
    expect(found).toEqual({ kind: "graph", path: "actions/pay.graph.json" });
    expect(stepsAt(docs["actions/pay.graph.json"], found as never).map((s) => s.id)).toEqual(["x"]);
    expect(
      resolveLocation({ ...docs, "actions/pay.graph.json": {} }, "pages/home.page.json", "btn", "onPressed"),
    ).toBeUndefined();
    expect(resolveLocation(docs, "pages/home.page.json", "btn", "other")).toEqual({
      kind: "inline",
      path: "pages/home.page.json",
      nodeId: "btn",
      event: "other",
    });
  });

  test("extras come from plugin flows and function grants, the pointer names the steps, and findings group by step", () => {
    const extras = extrasOf(
      [
        { kind: "actionGraph", key: "checkout" },
        { kind: "actionGraph", key: "pageBound", page: "p" },
        { kind: "plugin", capabilities: { functions: [{ function: "score" }, { function: "x", alias: "risk" }] } },
        "not an object",
      ],
      [],
    );
    expect(extras).toEqual({ custom: [], functions: ["score", "risk"], flows: ["checkout"] });
    const doc = { root: { id: "root", children: [{ id: "btn" }] } };
    expect(stepsPointer(doc, inline, (_p, id) => (id === "btn" ? "/root/children/0" : null))).toBe(
      "/root/children/0/events/onPressed/steps",
    );
    expect(stepsPointer(doc, { kind: "graph", path: "g" }, () => null)).toBe("/steps");
    expect(stepsPointer(doc, inline, () => null)).toBeNull();
    const grouped = groupProblems([{ path: "/s/1/input/key" }, { path: "/s" }, { path: "/elsewhere" }], "/s", [
      { id: "a", action: "pop" },
      { id: "b", action: "pop" },
    ]);
    expect([...grouped.keys()]).toEqual(["b", ""]);
    expect(groupProblems([{ path: "/s/0" }], null, []).size).toBe(0);
  });

  test("dagre lays out left to right, deterministically, with a step after the one it follows", () => {
    const steps: Step[] = [
      { id: "a", action: "pop", next: "b" },
      { id: "b", action: "pop" },
    ];
    const edges = edgesOf(steps, catalogue);
    const first = layoutSteps(steps, edges, () => 1);
    expect(layoutSteps(steps, edges, () => 1)).toEqual(first);
    expect((first.get("b")?.x ?? 0) > (first.get("a")?.x ?? 0)).toBe(true);
  });
});

describe("[STU-065] saved step positions ([SCH-004] x- properties)", () => {
  const doc = () => ({
    kind: "page",
    root: {
      id: "root",
      type: "Column",
      children: [
        {
          id: "btn",
          type: "ElevatedButton",
          events: {
            onPressed: {
              steps: [
                { id: "a", action: "pop" },
                { id: "b", action: "pop" },
              ],
            },
          },
        },
      ],
    },
  });
  const read = (d: unknown) => stepsAt(d, inline);

  test("positions are saved on the steps, rounded, for all dragged steps in one recipe", () => {
    const d = doc();
    setPositionsRecipe(
      inline,
      new Map([
        ["a", { x: 10.4, y: -20.6 }],
        ["b", { x: 300, y: 5 }],
        ["gone", { x: 1, y: 1 }],
      ]),
    )(d);
    expect(read(d).map((s) => savedPosition(s))).toEqual([
      { x: 10, y: -21 },
      { x: 300, y: 5 },
    ]);
  });

  test("a position the document holds in another shape is ignored", () => {
    expect(savedPosition({ id: "a", action: "pop", "x-position": { x: "1", y: 2 } })).toBeUndefined();
    expect(savedPosition({ id: "a", action: "pop" })).toBeUndefined();
  });

  test("a step dropped from the catalogue is added at the drop point and joins no edge", () => {
    const d = doc();
    addStepRecipe(inline, entry("pop"), "pop1", catalogue, undefined, { x: 120.2, y: 64 })(d);
    const added = read(d).find((s) => s.id === "pop1");
    expect(added === undefined ? undefined : savedPosition(added)).toEqual({ x: 120, y: 64 });
    expect(read(d).every((s) => s.next === undefined)).toBe(true);
  });
});

describe("[STU-065] positions that JSON cannot hold", () => {
  test("a drop point that is not a number saves no position", () => {
    const d = {
      kind: "page",
      root: { id: "root", type: "Column", children: [{ id: "btn", type: "ElevatedButton", events: {} }] },
    };
    addStepRecipe(inline, entry("pop"), "pop1", catalogue, undefined, { x: Number.NaN, y: 4 })(d);
    expect(stepsAt(d, inline).map((s) => savedPosition(s))).toEqual([undefined]);
    expect(Object.keys(stepsAt(d, inline)[0] ?? {})).not.toContain("x-position");
  });
});
