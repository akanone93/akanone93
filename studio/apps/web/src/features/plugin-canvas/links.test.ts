// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import {
  bindIssue,
  bindingOf,
  eventsOf,
  linkRecipe,
  linksOf,
  redirectRecipe,
  removeLinkRecipe,
  routeOptions,
  targetIssues,
} from "./links.ts";

const page = () => ({
  id: "root",
  type: "Column",
  children: [
    { id: "a", type: "ElevatedButton" },
    {
      id: "b",
      type: "ElevatedButton",
      events: { onPressed: { steps: [{ action: "navigate", id: "s1", input: { route: "detail" } }] } },
    },
    { id: "c", type: "ElevatedButton", events: { onPressed: { steps: [{ action: "showToast", id: "t" }] } } },
  ],
});
const node = (root: ReturnType<typeof page>, id: string) => root.children.find((c) => c.id === id);

describe("link creation (STU-040, STU-047)", () => {
  test("events come from the widget descriptor", () => {
    expect(eventsOf("ElevatedButton")).toContain("onPressed");
    expect(eventsOf("NoSuchWidget")).toEqual([]);
    expect(eventsOf(undefined)).toEqual([]);
  });

  test("a free event gets a navigate handler", () => {
    const root = page();
    linkRecipe("a", "onPressed", "home", "n1")(root);
    expect(node(root, "a")).toEqual({
      id: "a",
      type: "ElevatedButton",
      events: { onPressed: { steps: [{ action: "navigate", id: "n1", input: { route: "home" } }] } },
    });
  });

  test("a bound event is replaced or appended to", () => {
    const replaced = page();
    linkRecipe("c", "onPressed", "home", "n1")(replaced);
    expect(linksOf(node(replaced, "c"))).toEqual([{ event: "onPressed", step: 0, route: "home" }]);
    const appended = page();
    linkRecipe("c", "onPressed", "home", "n1", "append")(appended);
    expect(linksOf(node(appended, "c"))).toEqual([{ event: "onPressed", step: 1, route: "home" }]);
  });

  test("an unknown node is left alone", () => {
    const root = page();
    linkRecipe("zzz", "onPressed", "home", "n1")(root);
    expect(root).toEqual(page());
  });

  test("bindings and their issues", () => {
    const root = page();
    expect(bindingOf(node(root, "a"), "onPressed")).toBe("free");
    expect(bindingOf(node(root, "b"), "onPressed")).toBe("navigate");
    expect(bindingOf(node(root, "c"), "onPressed")).toBe("other");
    expect(bindIssue(node(root, "a"), "onPressed")).toBeUndefined();
    expect(bindIssue(node(root, "b"), "onPressed")).toEqual({
      kind: "event-bound",
      event: "onPressed",
      binding: "navigate",
    });
  });
});

describe("assisted fixes (F6, STU-046)", () => {
  const routes = routeOptions([{ route: "home", title: "Home" }], [{ route: "other", title: "Other" }]);

  test("route options list own screens first and add other plugins once", () => {
    const own = [{ route: "home", title: "Home" }];
    expect(routeOptions(own, [...own, { route: "other", title: "Other" }])).toEqual([
      { route: "home", title: "Home", own: true },
      { route: "other", title: "Other", own: false },
    ]);
  });

  test("a missing target is found, redirected and removed", () => {
    const root = page();
    const issues = targetIssues(node(root, "b"), routes);
    expect(issues).toHaveLength(1);
    const link = linksOf(node(root, "b"))[0];
    if (link === undefined) {
      throw new Error("no link");
    }
    redirectRecipe("b", link, "home")(root);
    expect(targetIssues(node(root, "b"), routes)).toEqual([]);
    const moved = linksOf(node(root, "b"))[0];
    if (moved === undefined) {
      throw new Error("no link");
    }
    removeLinkRecipe("b", moved)(root);
    expect(linksOf(node(root, "b"))).toEqual([]);
  });

  test("a stale link changes nothing", () => {
    const root = page();
    redirectRecipe("b", { event: "onPressed", step: 0, route: "gone" }, "home")(root);
    removeLinkRecipe("b", { event: "onPressed", step: 0, route: "gone" })(root);
    expect(root).toEqual(page());
  });
});
