// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { limitDefault } from "./index.ts";

describe("limitDefault [LIM-001]", () => {
  test("returns the registry default", () => {
    expect(limitDefault("page.nodes")).toBe(5000);
    expect(limitDefault("a11y.waiversPerPlugin")).toBe(50);
  });

  test("rejects a key outside the registry", () => {
    // @ts-expect-error not a LimitKey: the typed signature refuses it at compile time
    expect(() => limitDefault("no.such.limit")).toThrow("unknown limit key");
  });
});
