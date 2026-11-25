// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { componentsOf, expandInstances, instanceMarks, instanceRef } from "./component-instances.ts";
import { bindMocks, type DesignScope, emptyScope, INSTANCE_MARK, OUTER_MARK } from "./mock-bindings.ts";
import type { Node } from "./plugin-model.ts";

const card = {
  kind: "component",
  id: "c1",
  name: "Card",
  version: 2,
  props: [
    { name: "title", type: "string", required: true },
    { name: "tone", type: "string", default: "calm" },
  ],
  slots: [{ name: "body" }],
  root: {
    id: "col",
    type: "Column",
    children: [
      { id: "t", type: "Text", props: { data: { $expr: "props.title" } } },
      { id: "tone", type: "Text", props: { data: { $expr: "props.tone" } } },
      {
        id: "s",
        type: "Slot",
        props: { name: "body" },
        slots: { fallback: { id: "fb", type: "Text", props: { data: "no body" } } },
      },
    ],
  },
};
const doc = (parsed: { kind: string }) => ({ kind: parsed.kind, path: "x.json", content: new Uint8Array(), parsed });
const components = componentsOf([doc(card), doc({ kind: "page" })]);

describe("PC-37 component instances drawn as their components [STU-084] [STU-085]", () => {
  test("an instance is its component's tree: its props bound, its defaults kept, its slot filled", () => {
    const inst = {
      id: "i1",
      component: { id: "c1", version: 1 },
      props: { title: "Hello" },
      slots: { body: { id: "own", type: "Text", props: { data: "mine" } } },
      visible: { $expr: "page.on" },
    };
    expect(instanceRef(inst)).toEqual({ id: "c1", version: 1 });
    const drawn = expandInstances({ id: "root", type: "Column", children: [inst] }, components) as unknown as {
      children: { id: string; type: string; visible: unknown; children: { id: string; props: { data: unknown } }[] }[];
    };
    const tree = drawn.children[0];
    expect(tree?.id).toBe("i1");
    expect(tree?.type).toBe("Column");
    expect(tree?.visible).toEqual({ $expr: "page.on" });
    expect(tree?.children.map((c) => c.id)).toEqual(["i1~ct", "i1~ctone", "own"]);
    expect(tree?.children.map((c) => c.props.data)).toEqual(["Hello", "calm", "mine"]);
  });

  test("an unfilled slot shows its fallback; an unknown component and a tree without instances are left as they are", () => {
    const bare = { id: "i2", component: { id: "c1", version: 2 }, props: { title: "T" } };
    const drawn = expandInstances(bare, components) as unknown as {
      children: { id: string; props: { data: unknown } }[];
    };
    // The slot stays, drawn by the layout as its fallback, as the device draws an unfilled slot.
    expect(drawn.children[2]).toMatchObject({
      id: "i2~cs",
      type: "Slot",
      slots: { fallback: { id: "i2~cfb", props: { data: "no body" } } },
    });
    const unknown = { id: "u", component: { id: "nope", version: 1 } };
    expect(expandInstances(unknown, components)).toBe(unknown);
    const plain = { id: "p", type: "Text" };
    expect(expandInstances(plain, components)).toBe(plain);
    expect(expandInstances(bare, new Map())).toBe(bare);
  });

  test("a component that contains itself stops expanding instead of looping", () => {
    const loop = {
      kind: "component",
      id: "c2",
      name: "Loop",
      version: 1,
      root: { id: "r", component: { id: "c2", version: 1 } },
    };
    const drawn = expandInstances({ id: "i", component: { id: "c2", version: 1 } }, componentsOf([doc(loop)]));
    expect(JSON.stringify(drawn).length).toBeLessThan(2000);
  });

  test("each instance's component, version against the newest, and its overrides", () => {
    const roots = [
      {
        id: "s",
        type: "Column",
        children: [
          {
            id: "i1",
            component: { id: "c1", version: 1 },
            props: { title: "A" },
            slots: { body: { id: "b", type: "Text" } },
          },
          { id: "i2", component: { id: "c1", version: 2 } },
          { id: "i3", component: { id: "gone", version: 1 } },
        ],
      },
    ];
    expect(Object.fromEntries(instanceMarks(roots, components))).toEqual({
      i1: { component: "c1", name: "Card", version: 1, latest: 2, overrides: 2, known: true },
      i2: { component: "c1", name: "Card", version: 2, latest: 2, overrides: 0, known: true },
      i3: { component: "gone", name: "gone", version: 1, latest: 1, overrides: 0, known: false },
    });
  });
  test("ADR-0112 an instance inside a component expands, and each drawn root names its component and props (row 138)", () => {
    const shell = {
      kind: "component",
      id: "shell",
      props: [{ name: "org", type: "Org", required: true }],
      root: {
        id: "frame",
        component: { id: "c1", version: 2 },
        props: { title: { $expr: "props.org.name" } },
        slots: { body: { id: "city", type: "Text", props: { data: { $expr: "props.org.city" } } } },
      },
    };
    const all = componentsOf([doc(card), doc(shell)]);
    const inst = { id: "i3", component: { id: "shell", version: 1 }, props: { org: { $expr: "page.org" } } };
    const drawn = expandInstances(inst, all) as unknown as Record<string, unknown> & { type: string };
    // The shell's root is the card's instance, drawn as the card's tree.
    expect(drawn.type).toBe("Column");
    expect(drawn[INSTANCE_MARK]).toEqual([
      { component: "shell", props: { org: { $expr: "page.org" } } },
      { component: "c1", props: { tone: "calm", title: { $expr: "props.org.name" } } },
    ]);
    // Bound, the shell's props take their values in the page, the card's in the shell, each component's in its own.
    const scope: DesignScope = {
      ...emptyScope,
      page: { org: { name: "Nordvia Bank", city: "Berlin" } },
      componentScope: () => ({ state: {} }),
    };
    const bound = JSON.stringify(bindMocks(drawn as unknown as Node, new Map(), {}, scope));
    expect(bound).toContain('"data":"Nordvia Bank"');
    expect(bound).toContain('"data":"Berlin"');
    expect(bound).toContain('"data":"calm"');
    expect(bound).not.toContain(INSTANCE_MARK);
    expect(bound).not.toContain(OUTER_MARK);
  });

  test("ADR-0112 a slot's fill is bound in the scope its instance stands in, the component's parts in the component's", () => {
    const evaluated: string[] = [];
    const inst = {
      id: "i4",
      component: { id: "c1", version: 2 },
      props: { title: "T" },
      slots: { body: { id: "mine", type: "Text", props: { data: { $expr: "props.label" } } } },
    };
    const drawn = expandInstances(inst, components) as unknown as Node;
    const scope: DesignScope = {
      ...emptyScope,
      // The page is itself inside a component that has a `label` prop; the card has none.
      props: { label: "outer" },
      componentScope: () => ({ state: {}, evaluate: (expr) => void evaluated.push(expr) }),
    };
    const bound = JSON.stringify(bindMocks(drawn, new Map(), {}, scope));
    expect(bound).toContain('"data":"outer"');
    expect(bound).toContain('"data":"T"');
    expect(evaluated).toEqual([]);
  });
});
