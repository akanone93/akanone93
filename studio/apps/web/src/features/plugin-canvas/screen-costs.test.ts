// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import type { Problem } from "@plux/validator";
import { costShown } from "./interaction-overlay.tsx";
import { costWords, countNodes, runningTimelines, type ScreenCost, screenCosts } from "./screen-costs.ts";

const problem = (file: string, severity: Problem["severity"]): Problem => ({
  code: "PLX-0001",
  severity,
  file,
  path: "",
  message: "m",
});

describe("PC-34 what each screen costs [LIM-001] [ANI-008] [SCH-040]", () => {
  test("counts a screen's widgets, slots and children included", () => {
    const root = {
      id: "s",
      type: "Scaffold",
      slots: { appBar: { id: "a", type: "AppBar" }, body: { id: "c", type: "Column", children: [{ id: "t" }, {}] } },
    };
    expect(countNodes(root)).toBe(5);
    expect(countNodes(undefined)).toBe(0);
  });

  test("counts the timelines that run on their own: autoplay or a scroll driver", () => {
    const page = {
      animations: [{ autoplay: true }, { driver: { kind: "scroll", node: "list" } }, { autoplay: false }, {}],
    };
    expect(runningTimelines(page)).toBe(2);
    expect(runningTimelines({})).toBe(0);
  });

  test("each screen against the page budget, with the errors and warnings of its own file", () => {
    const screens = [
      { id: "a", root: { id: "r", type: "Column", children: [{ id: "x" }] } },
      { id: "b", root: { id: "q", type: "Column" } },
    ];
    const files: Record<string, string> = { a: "pages/a.json", b: "pages/b.json" };
    const problems = [
      problem("pages/a.json", "error"),
      problem("pages/a.json", "warning"),
      problem("pages/c.json", "error"),
    ];
    const costs = screenCosts(
      screens,
      () => ({}),
      (id) => files[id],
      problems,
    );
    expect(costs.get("a")).toMatchObject({ nodes: 2, status: "within", errors: 1, warnings: 1, timelines: 0 });
    expect(costs.get("a")?.budget).toEqual({ warning: 1000, limit: 5000 });
    expect(costs.get("b")).toMatchObject({ errors: 0, warnings: 0 });
  });

  test("the chip says only what needs attention", () => {
    const m = {
      costNodes: (n: number, l: number) => `${n}/${l}`,
      costTitle: (w: number, l: number) => `${w}-${l}`,
      costTimelines: (r: number, l: number) => `${r}>${l}`,
      costErrors: (n: number) => `${n}e`,
      costWarnings: (n: number) => `${n}w`,
    };
    const base = {
      nodes: 3,
      budget: { warning: 1000, limit: 5000 },
      status: "within" as const,
      timelines: 2,
      timelineLimit: 6,
      errors: 0,
      warnings: 0,
    };
    expect(costWords(base, m)).toEqual({ nodes: "3/5000", title: "1000-5000" });
    expect(costWords({ ...base, timelines: 7, errors: 2, warnings: 1 }, m)).toEqual({
      nodes: "3/5000",
      title: "1000-5000",
      timelines: "7>6",
      errors: "2e",
      warnings: "1w",
    });
  });
});

describe("PC-34 a screen's cost line where it says something (#229)", () => {
  const cost = (over: Partial<ScreenCost> = {}): ScreenCost => ({
    nodes: 40,
    budget: { warning: 4000, limit: 5000 },
    status: "within",
    timelines: 0,
    timelineLimit: 8,
    errors: 0,
    warnings: 0,
    ...over,
  });
  const words = { nodes: "40 of 5000 nodes", title: "" };
  const screen = { l: 0, t: 0, r: 400, b: 800 };
  const inside = { l: 100, t: 100, r: 200, b: 140 };
  const elsewhere = { l: 900, t: 100, r: 1000, b: 140 };

  test("hidden on a screen within its budget and without problems", () => {
    expect(costShown(cost(), words, screen, undefined, [])).toBe("hidden");
    expect(costShown(cost(), words, screen, elsewhere, [{ box: elsewhere }])).toBe("hidden");
  });

  test("after a delay under the screen the pointer rests on; at once under the screen worked on", () => {
    expect(costShown(cost(), words, screen, inside, [])).toBe("delayed");
    expect(costShown(cost(), words, screen, undefined, [{ box: inside }])).toBe("shown");
  });

  test("at once when it warns: near or over the budget, or problems", () => {
    expect(costShown(cost({ status: "warning" }), words, screen, undefined, [])).toBe("shown");
    expect(costShown(cost(), { ...words, errors: "1 error" }, screen, undefined, [])).toBe("shown");
  });
});
