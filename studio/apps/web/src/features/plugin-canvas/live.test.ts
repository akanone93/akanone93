// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import {
  applyLive,
  emptyLive,
  eventActions,
  impliedOf,
  switchAt,
  withImpliedButtons,
  withPick,
  withScroll,
} from "./live.ts";
import type { Node } from "./plugin-model.ts";

const nav: Node = {
  id: "nav",
  type: "NavigationBar",
  props: {
    selectedIndex: { $expr: "page.tab" } as never,
    destinations: [{ label: "A" }, { label: "B" }, { label: "C" }],
  } as never,
};
const stack: Node = { id: "st", type: "IndexedStack", props: { index: { $expr: "page.tab" } } as never };
const tabs: Node = { id: "tb", type: "TabBar", props: { tabs: [{ label: "One" }, { label: "Two" }] } as never };
const view: Node = { id: "tv", type: "TabBarView" };
const seg: Node = {
  id: "seg",
  type: "SegmentedButton",
  props: { segments: [{ value: "day" }, { value: "week" }], selected: ["day"] } as never,
};
const list: Node = { id: "list", type: "ListView" };
const root: Node = { id: "sc", type: "Scaffold", children: [nav, stack, tabs, view, seg, list] };
const box = { l: 0, r: 300 };
const propOf = (n: Node | undefined, name: string): unknown =>
  (n?.props as Record<string, unknown> | undefined)?.[name];

describe("PC-15 the live surface [STU-068]", () => {
  test("a click across a switcher picks the item under it, mirrored right to left", () => {
    expect(switchAt(nav, box, 250, false)).toEqual({ prop: "selectedIndex", value: 2, expr: "page.tab" });
    expect(switchAt(nav, box, 250, true)?.value).toBe(0);
    expect(switchAt(seg, box, 200, false)).toEqual({ prop: "selected", value: ["week"] });
    expect(switchAt({ id: "t", type: "Text" }, box, 10, false)).toBeUndefined();
  });

  test("a pick bound to page state sets that state for every prop bound to it; a tab drives its view", () => {
    let live = withPick(emptyLive, "nav", { value: 1, expr: "page.tab" });
    live = withPick(live, "tb", { value: 1 });
    const shown = applyLive(root, live);
    const kids = shown.children ?? [];
    expect(propOf(kids[0], "selectedIndex")).toBe(1);
    expect(propOf(kids[1], "index")).toBe(1);
    expect(propOf(kids[2], "selectedIndex")).toBe(1);
    expect(propOf(kids[3], "index")).toBe(1);
    // The document is untouched; a tree nothing live touches is returned as it is.
    expect((nav.props as Record<string, unknown>)["selectedIndex"]).toEqual({ $expr: "page.tab" });
    expect(applyLive(root, emptyLive)).toBe(root);
  });

  test("a scroll view keeps its offset between 0 and its end", () => {
    let live = withScroll(emptyLive, "list", 500, 300);
    expect(live.scroll.get("list")).toBe(300);
    live = withScroll(live, "list", -1000, 300);
    expect(live.scroll.get("list")).toBe(0);
    const shown = applyLive(root, withScroll(emptyLive, "list", 120, 300));
    expect((shown.children ?? [])[5]).toMatchObject({ "x-studio": { scroll: 120 } });
  });

  test("an app bar implies its menu, back or close button, as Flutter's does", () => {
    const page = (extra: object): Node =>
      ({ id: "sc", type: "Scaffold", slots: { appBar: { id: "bar", type: "AppBar" }, ...extra } }) as Node;
    const withDrawer = withImpliedButtons(page({ drawer: { id: "d", type: "Drawer" } }), "back", "material");
    expect(withDrawer).toMatchObject({ slots: { appBar: { slots: { leading: { id: "bar~implied-menu" } } } } });
    expect(impliedOf("s1/bar~implied-menu-icon#paint0")).toBe("menu");
    const backed = withImpliedButtons(page({}), "back", "cupertino");
    expect(backed).toMatchObject({
      slots: { appBar: { slots: { leading: { slots: { icon: { props: { icon: { name: "arrow_back_ios" } } } } } } } },
    });
    const own = page({});
    expect(withImpliedButtons(own, undefined, "material")).toBe(own);
    const end = withImpliedButtons(page({ endDrawer: { id: "e", type: "Drawer" } }), undefined, "material");
    expect(end).toMatchObject({ slots: { appBar: { slots: { actions: [{ id: "bar~implied-endmenu" }] } } } });
    expect(impliedOf("s1/bar~implied-endmenu#paint0")).toBe("endmenu");
  });

  test("a widget's events: the pages it opens and the actions only a device runs", () => {
    const node = {
      id: "b",
      type: "FilledButton",
      events: {
        onPressed: {
          steps: [
            { action: "haptic", id: "h" },
            { action: "openDialog", id: "o", input: { route: "confirm" } },
            { action: "apiCall", id: "a" },
          ],
        },
      },
    } as unknown as Node;
    expect(eventActions(node)).toEqual({ opens: ["confirm"], runs: ["apiCall"] });
  });
});
