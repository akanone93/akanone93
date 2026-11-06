// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import type { WidgetNode } from "./screen.ts";
import { atSizeClass, sizeClassOf } from "./size-class.ts";

const node = (extra: object = {}): WidgetNode =>
  ({
    id: "box",
    type: "Container",
    props: { width: 100, color: "#FF0000FF" },
    responsive: { medium: { width: 200 }, expanded: { width: 300, padding: 24 } },
    ...extra,
  }) as WidgetNode;

describe("PC-33 window size classes on the canvas [WGT-010]", () => {
  test("Material 3's breakpoints: compact below 600, medium to 839, expanded from 840", () => {
    expect([599, 600, 839, 840].map(sizeClassOf)).toEqual(["compact", "medium", "medium", "expanded"]);
  });

  test("size-class layers cascade as the runtime applies them: medium from 600, expanded over it from 840", () => {
    expect(atSizeClass(node(), 412)?.props).toEqual({ width: 100, color: "#FF0000FF" });
    expect(atSizeClass(node(), 673)?.props).toEqual({ width: 200, color: "#FF0000FF" });
    expect(atSizeClass(node(), 1280)?.props).toEqual({ width: 300, color: "#FF0000FF", padding: 24 });
  });

  test("a widget Studio found shown only in some size classes is left out of the others, slots included", () => {
    const root = {
      id: "s",
      type: "Scaffold",
      slots: { bottomNavigationBar: { id: "bar", type: "NavigationBar", onlyIn: ["compact"] } },
      children: [
        { id: "rail", type: "NavigationRail", onlyIn: ["medium", "expanded"] },
        { id: "t", type: "Text" },
      ],
    } as unknown as WidgetNode;
    const ids = (r: WidgetNode | undefined) => [
      ...Object.values(r?.slots ?? {}).map((n) => n.id),
      ...(r?.children ?? []).map((n) => n.id),
    ];
    expect(ids(atSizeClass(root, 400))).toEqual(["bar", "t"]);
    expect(ids(atSizeClass(root, 700))).toEqual(["rail", "t"]);
  });

  test("a tree with nothing that depends on the size class is returned as it is", () => {
    const plain = { id: "c", type: "Column", children: [{ id: "t", type: "Text", props: { data: "Hi" } }] };
    expect(atSizeClass(plain, 900)).toBe(plain);
  });
});
