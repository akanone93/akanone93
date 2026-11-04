// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import type { I18n } from "@lingui/core";
import { areaOf } from "./page-area.ts";

// The messages' source text stands in for a catalogue.
const i18n = {
  _: (d: { message: string; values?: Record<string, unknown> }) => d.message,
} as unknown as I18n;

describe("page area (ADR-0103)", () => {
  test("an app's area takes the sidebar's icon and name, whatever page of it is open", () => {
    expect(areaOf(i18n, "/o/acme/apps/shop/security/keys")).toEqual({ icon: "lock", label: "Security" });
    expect(areaOf(i18n, "/o/acme/apps/shop/releases")?.icon).toBe("rocket-launch");
  });

  test("an organisation area names the page the sidebar lists, the longest path first", () => {
    expect(areaOf(i18n, "/o/acme/admin/teams/t1")).toMatchObject({ icon: "gear", page: "Teams" });
    expect(areaOf(i18n, "/o/acme/approvals/break-glass")?.page).toBe("Break-glass reviews");
    expect(areaOf(i18n, "/o/acme/approvals")?.icon).toBe("check-circle");
  });

  test("the installation, the profile and the library have their own areas", () => {
    expect(areaOf(i18n, "/installation/jobs")?.icon).toBe("server");
    expect(areaOf(i18n, "/me")?.icon).toBe("user");
    expect(areaOf(i18n, "/o/acme/library/components/c1")?.icon).toBe("book");
  });

  test("the home page, the canvases and unknown paths have none", () => {
    expect(areaOf(i18n, "/o/acme")).toBeUndefined();
    expect(areaOf(i18n, "/o/acme/apps/shop")).toBeUndefined();
    expect(areaOf(i18n, "/o/acme/apps/shop/plugins/pl")).toBeUndefined();
    expect(areaOf(i18n, "/signin")).toBeUndefined();
  });
});
