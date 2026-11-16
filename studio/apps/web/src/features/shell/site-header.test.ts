// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { appOfPath } from "./site-header.tsx";

describe("the breadcrumb names the app of an app's page", () => {
  test("an app's pages have one; the wizard and the organisation's pages do not", () => {
    expect(appOfPath("/o/org1/apps/app1/design/themes")).toEqual({ org: "org1", app: "app1" });
    expect(appOfPath("/o/org1/apps/app1")).toEqual({ org: "org1", app: "app1" });
    expect(appOfPath("/o/org1/apps/new")).toBeUndefined();
    expect(appOfPath("/o/org1")).toBeUndefined();
    expect(appOfPath("/me")).toBeUndefined();
  });
});
