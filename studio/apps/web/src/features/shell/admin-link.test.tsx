// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { orgA, renderApp } from "../../test-harness.tsx";

afterEach(cleanup);

describe("[STU-116] administration in the sidebar", () => {
  test("a person who holds the permission the admin area needs sees the link to it", async () => {
    renderApp(`/o/${orgA}`, { permissions: ["organization.manage"] });
    const nav = await screen.findByRole("navigation", { name: "Studio navigation" });
    fireEvent.click(await within(nav).findByRole("button", { name: "Administration" }));
    const link = await within(nav).findByRole("link", { name: "Settings" });
    expect(link.getAttribute("href")).toBe(`/o/${orgA}/admin`);
    expect(within(nav).queryByRole("link", { name: "Members" })).toBeNull();
  });

  test("the area lists only the pages the person's permissions open", async () => {
    renderApp(`/o/${orgA}`, { permissions: ["members.manage"] });
    const nav = await screen.findByRole("navigation", { name: "Studio navigation" });
    fireEvent.click(await within(nav).findByRole("button", { name: "Administration" }));
    const link = await within(nav).findByRole("link", { name: "Members" });
    expect(link.getAttribute("href")).toBe(`/o/${orgA}/admin/members`);
    expect(within(nav).queryByRole("link", { name: "Settings" })).toBeNull();
  });

  test("a person without any administration permission sees no administration", async () => {
    renderApp(`/o/${orgA}`, { permissions: ["app.read"] });
    const banner = await screen.findByRole("banner");
    await waitFor(() => expect(within(banner).getByText("Alpha Org")).toBeDefined());
    const nav = screen.getByRole("navigation", { name: "Studio navigation" });
    await within(nav).findByRole("button", { name: "Approvals" });
    expect(within(nav).queryByRole("button", { name: "Administration" })).toBeNull();
  });

  test("on an administration page its area is open and the page is current", async () => {
    renderApp(`/o/${orgA}/admin/members`, { permissions: ["members.manage", "organization.manage"] });
    const nav = await screen.findByRole("navigation", { name: "Studio navigation" });
    const link = await within(nav).findByRole("link", { name: "Members" });
    expect(link.hasAttribute("data-active")).toBe(true);
    expect(within(nav).getByRole("link", { name: "Settings" }).hasAttribute("data-active")).toBe(false);
  });
});
