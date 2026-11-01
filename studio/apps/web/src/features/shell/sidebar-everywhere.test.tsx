// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { act, cleanup, screen, within } from "@testing-library/react";
import { createSession } from "../../lib/session.ts";
import { createAppRouter } from "../../router.tsx";
import { orgA, renderApp } from "../../test-harness.tsx";

afterEach(cleanup);

const everything = {
  permissions: ["app.read", "organization.manage", "members.manage"],
  installationAdmin: true,
};

/** Every signed-in page outside an organisation, read from the route tree, so a page added later is checked too. */
function pagesOutsideAnOrganisation(): string[] {
  const router = createAppRouter({ session: createSession("/api") });
  type Node = { readonly id: string; readonly fullPath: string; readonly parentRoute?: Node };
  const underShell = (route: Node | undefined): boolean =>
    route !== undefined && (route.id === "/shell" || underShell(route.parentRoute));
  const paths = (Object.values(router.routesById) as Node[])
    .filter(
      (route) => underShell(route.parentRoute) && !route.fullPath.startsWith("/o/") && !route.fullPath.includes("$"),
    )
    .map((route) => route.fullPath);
  return [...new Set(paths)].filter((p) => p !== "/");
}

/** The sidebar of the home organisation: its home page and its areas the person may open. */
async function expectFullSidebar(page: string): Promise<void> {
  const nav = await screen.findByRole("navigation", { name: "Studio navigation" }, { timeout: 10_000 });
  const home = await within(nav).findByRole("link", { name: "Home" }, { timeout: 10_000 });
  expect([page, home.getAttribute("href")]).toEqual([page, `/o/${orgA}`]);
  expect(within(nav).getByRole("button", { name: "Approvals" })).toBeTruthy();
  expect(within(nav).getByRole("button", { name: "Administration" })).toBeTruthy();
  expect(within(nav).getByRole("button", { name: "Installation" })).toBeTruthy();
}

describe("[STU-014] the sidebar on every page", () => {
  const pages = pagesOutsideAnOrganisation();

  test("the route tree has pages outside an organisation to check", () => {
    expect(pages).toContain("/me");
    expect(pages).toContain("/installation/organisations");
  });

  test("a page outside an organisation, opened directly, keeps the home organisation's navigation", async () => {
    for (const page of pages) {
      renderApp(page, everything);
      await expectFullSidebar(page);
      cleanup();
    }
  });

  test("going from an organisation to a page outside it keeps that organisation's navigation", async () => {
    for (const page of pages) {
      const r = renderApp(`/o/${orgA}`, everything);
      await screen.findByRole("heading", { name: /Welcome to/ }, { timeout: 10_000 });
      await act(() => r.router.navigate({ to: page as never }));
      await expectFullSidebar(page);
      cleanup();
    }
  });
});
