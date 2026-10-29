// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import {
  addEntryRecipe,
  composeType,
  declaredTypes,
  freshName,
  nameProblem,
  parseDefault,
  parseType,
  readState,
  removeEntryRecipe,
  setFieldRecipe,
} from "./state-model.ts";

describe("[STA-002] state entries", () => {
  test("entries are read with their defaults; malformed ones are skipped", () => {
    const plugin = {
      state: [
        { id: "a", name: "count", type: "int", default: 3, persistence: "session", sensitive: true },
        { id: "b", name: "total", type: "double", computed: "state.count * 2" },
        { id: "c" },
      ],
    };
    const [count, total, ...rest] = readState(plugin);
    expect(rest).toEqual([]);
    expect(count).toMatchObject({
      name: "count",
      defaultText: "3",
      persistence: "session",
      sensitive: true,
      computed: false,
    });
    expect(total).toMatchObject({ computed: true, defaultText: "", persistence: undefined });
    expect(readState(undefined)).toEqual([]);
  });

  test("type expressions round-trip through the pickers [SCH-010]", () => {
    for (const expr of ["string", "int?", "list<Transaction>", "map<string,decimal>?"]) {
      const parts = parseType(expr);
      expect(parts && composeType(parts)).toBe(expr);
    }
    expect(parseType("map<string, int>")).toEqual({ container: "map", base: "int", nullable: false });
    expect(parseType("list<list<int>>")).toBeUndefined();
  });

  test("declared object types of the plugin and the app are offered", () => {
    expect(declaredTypes({ types: [{ name: "Loan" }] }, { types: [{ name: "Account" }, { x: 1 }] }, undefined)).toEqual(
      ["Account", "Loan"],
    );
  });

  test("names are lowerCamelCase and unique", () => {
    expect(nameProblem("", [])).toBe("empty");
    expect(nameProblem("Count", [])).toBe("invalid");
    expect(nameProblem("count", ["count"])).toBe("taken");
    expect(nameProblem("loanAmount", ["count"])).toBeUndefined();
    expect(freshName(["value1", "value2"])).toBe("value3");
  });

  test("defaults are parsed as JSON", () => {
    expect(parseDefault("  ")).toBeNull();
    expect(parseDefault("[1,2]")).toEqual({ value: [1, 2] });
    expect(parseDefault("{oops")).toBeUndefined();
  });

  test("recipes add, change and remove an entry", () => {
    const d: Record<string, unknown> = {};
    addEntryRecipe({ id: "a", name: "count", type: "int" })(d);
    expect(d["state"]).toEqual([{ id: "a", name: "count", type: "int", persistence: "memory" }]);
    setFieldRecipe("a", "default", 5)(d);
    setFieldRecipe("a", "sensitive", true)(d);
    setFieldRecipe("a", "sensitive", undefined)(d);
    setFieldRecipe("zz", "default", 1)(d);
    expect(d["state"]).toEqual([{ id: "a", name: "count", type: "int", persistence: "memory", default: 5 }]);
    removeEntryRecipe("a")(d);
    expect(d["state"]).toEqual([]);
  });
});
