// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { cleanup, screen, within } from "@testing-library/react";
import { renderApp } from "../../test-harness.tsx";

afterEach(cleanup);

describe("[STU-014] the profile's own pages (#233)", () => {
  test("each part of the profile has its own tab, and the open one is marked", async () => {
    renderApp("/me/phones");
    const tabs = await screen.findByRole("navigation", { name: "Profile pages" }, { timeout: 20_000 });
    const links = within(tabs).getAllByRole("link");
    expect(links.map((a) => [a.textContent, a.getAttribute("href")])).toEqual([
      ["Profile", "/me"],
      ["Security keys", "/me/security-keys"],
      ["Phones", "/me/phones"],
      ["Appearance", "/me/appearance"],
      ["Notifications", "/me/notifications"],
      ["Keyboard shortcuts", "/me/shortcuts"],
    ]);
    expect(links.filter((a) => a.getAttribute("aria-current") === "page").map((a) => a.textContent)).toEqual([
      "Phones",
    ]);
  }, 40_000);
});
