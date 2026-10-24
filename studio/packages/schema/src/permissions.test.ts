// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { type PermissionName, permissions, roles } from "./index.ts";

describe("permission catalogue [GOV-002]", () => {
  test("names are unique and sorted", () => {
    const names = permissions.map((p) => p.name);
    expect(names.length).toBeGreaterThan(0);
    expect(new Set(names).size).toBe(names.length);
    expect([...names].sort()).toEqual(names);
  });

  test("every role grants only catalogued permissions", () => {
    const known = new Set<string>(permissions.map((p) => p.name));
    for (const role of roles) {
      for (const name of role.permissions) {
        expect(known.has(name)).toBe(true);
      }
    }
  });

  test("an unknown permission name does not compile", () => {
    // @ts-expect-error "no.such.permission" is not a PermissionName.
    const unknown: PermissionName = "no.such.permission";
    expect(permissions.some((p) => p.name === unknown)).toBe(false);
  });
});
