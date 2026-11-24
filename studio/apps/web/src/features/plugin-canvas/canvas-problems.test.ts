// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import type { Validation } from "../problems/use-validation.ts";
import { unbuiltKeyOf, unbuiltProblems, withProblems } from "./canvas-problems.ts";

const page = {
  path: "plugins/main/pages/home.page.json",
  kind: "page",
  parsed: {
    id: "home",
    root: { id: "r", type: "Scaffold", slots: { body: { id: "col", type: "Column", children: [{ id: "chip" }] } } },
  },
};
const message = (type: string, reason: string) => `${type}: ${reason}`;

describe("what the canvas could not draw, in Problems (#80)", () => {
  test("each widget the layout could not build is a warning at its node, in its page", () => {
    const key = unbuiltKeyOf([
      { id: "home", contentHeight: 800, unbuilt: [{ id: "chip", type: "Chip", reason: "no label" }] },
      { id: "other", contentHeight: 800 },
    ]);
    expect(unbuiltProblems(key, [page], message)).toEqual([
      {
        code: "PLX-4001",
        severity: "warning",
        file: "plugins/main/pages/home.page.json",
        path: "/root/slots/body/children/0",
        message: "Chip: no label",
      },
    ]);
  });

  test("an unchanged layout keeps its key; nothing found is no problem", () => {
    const at = (reason: string) => [
      {
        id: "home",
        contentHeight: 1,
        unbuilt: [
          { id: "b", type: "Chip", reason },
          { id: "a", type: "Chip", reason },
        ],
      },
    ];
    expect(unbuiltKeyOf(at("x"))).toBe(unbuiltKeyOf(at("x")));
    expect(unbuiltKeyOf([{ id: "home", contentHeight: 1 }])).toBe("");
    expect(unbuiltProblems("", [page], message)).toEqual([]);
  });

  test("a node inside a component instance points at the page's root", () => {
    const key = unbuiltKeyOf([{ id: "home", contentHeight: 1, unbuilt: [{ id: "inst/x", type: "Row", reason: "r" }] }]);
    expect(unbuiltProblems(key, [page], message)[0]?.path).toBe("/root");
  });

  test("the problems join a finished validation, and leave one still running as it is", () => {
    const extra = unbuiltProblems(
      unbuiltKeyOf([{ id: "home", contentHeight: 1, unbuilt: [{ id: "chip", type: "Chip", reason: "r" }] }]),
      [page],
      message,
    );
    const running: Validation = { state: "running" };
    expect(withProblems(running, extra)).toBe(running);
    const done = { state: "done", problems: [], validatePage: async () => [], language: undefined, tool: undefined };
    const merged = withProblems(done as unknown as Validation, extra);
    expect(merged.state === "done" ? merged.problems : []).toEqual(extra);
    expect(withProblems(done as unknown as Validation, [])).toBe(done as unknown as Validation);
  });
});
