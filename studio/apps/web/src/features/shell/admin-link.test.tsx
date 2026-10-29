// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import { orgA, renderApp } from "../../test-harness.tsx";

afterEach(cleanup);

describe("[STU-116] administration link in the shell", () => {
  test("a person who holds the permission the admin area needs sees the link to it", async () => {
    renderApp(`/o/${orgA}`, { permissions: ["organization.manage"] });
    const banner = await screen.findByRole("banner");
    const link = await within(banner).findByRole("link", { name: "Administration" });
    expect(link.getAttribute("href")).toBe(`/o/${orgA}/admin`);
  });

  test("a person without it sees no link", async () => {
    renderApp(`/o/${orgA}`, { permissions: ["members.manage"] });
    const banner = await screen.findByRole("banner");
    await waitFor(() => expect(within(banner).getByText("Alpha Org")).toBeDefined());
    expect(within(banner).queryByRole("link", { name: "Administration" })).toBeNull();
  });
});
