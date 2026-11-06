// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { componentsOf, expandInstances, instanceMarks, instanceRef } from "./component-instances.ts";

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
});
