// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import {
  appendRecipe,
  buildDocument,
  type Draft,
  emptyDraft,
  fileOf,
  filterRows,
  isScenarioPath,
  pathOf,
  problems,
  removeRecipe,
  scenarioOf,
  scenariosOf,
} from "./tests.model.ts";

const draft: Draft = {
  file: "checkout",
  name: " Pays ",
  page: "checkout",
  steps: [
    { kind: "tap", testId: "pay", text: "" },
    { kind: "enterText", testId: "card", text: "4242" },
  ],
  expects: [
    { kind: "visible", value: "done", text: "" },
    { kind: "textEquals", value: "total", text: "9" },
    { kind: "navigatedTo", value: "receipt", text: "" },
  ],
};

describe("TST-001 scenario authoring model", () => {
  test("builds a scenario that follows scenario.schema.json", () => {
    expect(scenarioOf(draft)).toEqual({
      name: "Pays",
      page: "checkout",
      steps: [{ tap: "pay" }, { enterText: { testId: "card", text: "4242" } }],
      expect: [{ visible: "done" }, { textEquals: { testId: "total", text: "9" } }, { navigatedTo: "receipt" }],
    });
    expect(buildDocument(draft)).toMatchObject({ schemaVersion: "1.0.0", kind: "scenarios" });
    expect(pathOf("checkout")).toBe("tests/checkout.scenario.json");
  });

  test("names what keeps a draft from being saved", () => {
    expect(problems(draft)).toEqual([]);
    expect(problems(emptyDraft)).toEqual(["file", "name", "page", "expect"]);
    expect(problems({ ...draft, steps: [{ kind: "enterText", testId: "card", text: "" }] })).toEqual(["step"]);
  });

  test("reads, filters, appends to and removes from a scenarios document", () => {
    const bytes = new TextEncoder().encode(JSON.stringify(buildDocument(draft)));
    const rows = scenariosOf("tests/checkout.scenario.json", JSON.parse(new TextDecoder().decode(bytes)));
    expect(rows).toEqual([
      { path: "tests/checkout.scenario.json", name: "Pays", target: "checkout", steps: 2, expectations: 3 },
    ]);
    expect(scenariosOf("x", { scenarios: "nope" })).toEqual([]);
    expect(filterRows(rows, "RECEIPT")).toEqual([]);
    expect(filterRows(rows, "check")).toHaveLength(1);
    const doc: Record<string, unknown> = { scenarios: [{ name: "a" }] };
    appendRecipe({ name: "b" })(doc);
    expect((doc["scenarios"] as unknown[]).length).toBe(2);
    removeRecipe("a")(doc);
    expect(doc["scenarios"]).toEqual([{ name: "b" }]);
    expect(isScenarioPath("tests/a.scenario.json")).toBe(true);
    expect(isScenarioPath("plugin.json")).toBe(false);
    expect(fileOf("tests/a.scenario.json")).toBe("a");
  });
});
