// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { ROUTE_TYPE } from "@plux/layout";
import { linkRecipe, linksOf, navigateStep } from "./links.ts";
import { RouteCache, routeRoot, UNDER, underlay } from "./page-routes.ts";
import { openersOf, readPlugin } from "./plugin-model.ts";
import { blankPage } from "./screens.ts";

const enc = (v: unknown) => new TextEncoder().encode(JSON.stringify(v));
const page = (id: string, kind: string | undefined, root: unknown) => ({
  kind: "page",
  content: enc({
    kind: "page",
    id,
    key: id,
    route: id,
    title: id,
    ...(kind === undefined ? {} : { pageKind: kind }),
    root,
  }),
});
const opens = (action: string, route: string) => ({
  id: `${route}-btn`,
  type: "FilledButton",
  events: { onPressed: { steps: [{ action, id: "s", input: { route } }] } },
});

describe("PC-13 dialog and sheet pages [SCH-022]", () => {
  const docs = [
    { kind: "plugin", content: enc({ kind: "plugin", pages: ["home", "confirm", "pick", "edit"] }) },
    page("home", "screen", {
      id: "h",
      type: "Column",
      children: [opens("openDialog", "confirm"), opens("openBottomSheet", "pick")],
    }),
    page("confirm", "dialog", { id: "c", type: "Padding" }),
    page("pick", "bottomSheet", { id: "p", type: "Padding" }),
    page("edit", "fullscreenDialog", { id: "e", type: "Scaffold" }),
  ];

  test("every page is on the canvas with its kind; each dialog and sheet knows the page that opens it", () => {
    const model = readPlugin(docs);
    expect(model.screens.map((s) => [s.id, s.kind])).toEqual([
      ["home", "screen"],
      ["confirm", "dialog"],
      ["pick", "bottomSheet"],
      ["edit", "fullscreenDialog"],
    ]);
    // openDialog and openBottomSheet are links like navigate.
    expect(model.edges.get("home")).toEqual(["confirm", "pick"]);
    expect(openersOf(model.screens)).toEqual(
      new Map([
        ["confirm", "home"],
        ["pick", "home"],
      ]),
    );
  });

  test("a link to a dialog opens it as a dialog, to a sheet as a sheet, to a screen by navigating", () => {
    expect(navigateStep("confirm", "s1", "dialog")).toEqual({
      action: "openDialog",
      id: "s1",
      input: { route: "confirm" },
    });
    expect(navigateStep("pick", "s1", "bottomSheet")["action"]).toBe("openBottomSheet");
    expect(navigateStep("home", "s1")["action"]).toBe("navigate");
    const root = { id: "r", type: "Column", children: [{ id: "b", type: "FilledButton" }] };
    linkRecipe("b", "onPressed", "confirm", "s9", "replace", "dialog")(root);
    expect(linksOf(root.children[0])).toEqual([{ event: "onPressed", step: 0, route: "confirm" }]);
  });

  test("the drawn copy wraps the page over its opener, whose ids nothing can pick; it is kept while unchanged", () => {
    const opener = { id: "h", type: "Column", children: [{ id: "t", type: "Text" }] };
    const under = underlay(opener);
    expect(under.id).toBe(`${UNDER}h`);
    expect(under.children?.[0]?.id).toBe(`${UNDER}t`);
    const pageRoot = { id: "c", type: "Padding" };
    const route = routeRoot("dialog", pageRoot, under) as unknown as {
      type: string;
      props: { kind: string };
      slots: Record<string, { id: string }>;
    };
    expect(route.type).toBe(ROUTE_TYPE);
    expect(route.props.kind).toBe("dialog");
    expect(route.slots["page"]?.id).toBe("c");
    const cache = new RouteCache();
    const a = cache.wrap("bottomSheet", pageRoot, opener);
    expect(cache.wrap("bottomSheet", pageRoot, opener)).toBe(a);
    expect(cache.wrap("dialog", pageRoot, opener)).not.toBe(a);
  });

  test("a new page of each kind starts with its root", () => {
    const names = { keys: new Set<string>(), routes: new Set<string>() };
    let n = 0;
    const id = () => `n${++n}`;
    expect(blankPage("Confirm", names, id, "dialog")).toMatchObject({ pageKind: "dialog", root: { type: "Padding" } });
    expect(blankPage("Pick", names, id, "bottomSheet")).toMatchObject({
      pageKind: "bottomSheet",
      root: { type: "SafeArea" },
    });
    const edit = blankPage("Edit", names, id, "fullscreenDialog") as {
      root: { slots: { appBar: { slots: { leading: { events: unknown } } } } };
    };
    expect(edit.root.slots.appBar.slots.leading.events).toEqual({
      onPressed: { steps: [{ action: "pop", id: "close" }] },
    });
    expect(blankPage("Home", names, id)).toMatchObject({ pageKind: "screen", root: { type: "Scaffold" } });
  });
});
