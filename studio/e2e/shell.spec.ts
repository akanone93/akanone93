// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import AxeBuilder from "@axe-core/playwright";
import { create, type DescMessage, type MessageShape, toBinary } from "@bufbuild/protobuf";
import { expect, type Page, test } from "@playwright/test";
import {
  GetCurrentUserResponseSchema,
  GetPreferencesResponseSchema,
  ListOrganizationsResponseSchema,
  ListSignInProvidersResponseSchema,
  MemberSchema,
  OrganizationSchema,
  PreferencesSchema,
  SessionSchema,
  StartPasswordLoginResponseSchema,
  UserSchema,
} from "@plux/api";

const org = "0192f000-0000-7000-8000-00000000000a";

function proto<S extends DescMessage>(schema: S, init: Parameters<typeof create<S>>[1]) {
  return {
    status: 200,
    contentType: "application/proto",
    body: Buffer.from(toBinary(schema, create(schema, init) as MessageShape<S>)),
  };
}

/** The BFF's /api/* answered in-process: the same unary Connect calls the server would receive (STU-013). */
async function mockUpstream(page: Page) {
  let signedIn = false;
  await page.route("**/api/plux.v1.*/*", async (route) => {
    const method = new URL(route.request().url()).pathname.split("/").pop();
    switch (method) {
      case "GetCurrentUser":
        if (!signedIn) {
          return route.fulfill({
            status: 401,
            contentType: "application/json",
            body: JSON.stringify({ code: "unauthenticated", message: "no session" }),
          });
        }
        return route.fulfill(
          proto(GetCurrentUserResponseSchema, {
            user: create(UserSchema, { id: "u1", displayName: "Ada", email: "ada@example.org" }),
            memberships: [create(MemberSchema, { organizationId: org, userId: "u1" })],
          }),
        );
      case "StartPasswordLogin":
        signedIn = true;
        return route.fulfill(
          proto(StartPasswordLoginResponseSchema, { session: create(SessionSchema, { id: "s1", csrfToken: "csrf" }) }),
        );
      case "ListSignInProviders":
        return route.fulfill(proto(ListSignInProvidersResponseSchema, {}));
      case "ListOrganizations":
        return route.fulfill(
          proto(ListOrganizationsResponseSchema, {
            organizations: [create(OrganizationSchema, { id: org, key: "alpha", name: "Alpha Org" })],
          }),
        );
      case "GetPreferences":
        return route.fulfill(proto(GetPreferencesResponseSchema, { preferences: create(PreferencesSchema, {}) }));
      default:
        return route.fulfill({
          status: 404,
          contentType: "application/json",
          body: JSON.stringify({ code: "not_found", message: "unmocked" }),
        });
    }
  });
}

async function signIn(page: Page) {
  await page.goto("/");
  await expect(page.getByRole("heading", { name: /Sign in|Sīgn/i })).toBeVisible();
  await page.getByLabel(/Email|Ēmāīl/i).fill("ada@example.org");
  await page.getByLabel(/Password|Pāśśẃōŕď/i).fill("correct horse battery");
  await page.getByRole("button", { name: /^Sign in|^Sīgn/i }).click();
  await expect(page).toHaveURL(new RegExp(`/o/${org}`));
}

const axeTags = ["wcag2a", "wcag2aa", "wcag21aa"];

test("sign in lands on the organisation home, axe clean in light [STU-013]", async ({ page }) => {
  await mockUpstream(page);
  await signIn(page);
  await expect(page.getByRole("banner")).toBeVisible();
  expect((await new AxeBuilder({ page }).withTags(axeTags).analyze()).violations).toEqual([]);
});

test.describe("dark", () => {
  test.use({ colorScheme: "dark" });
  test("the home is axe clean in dark [STU-014]", async ({ page }) => {
    await mockUpstream(page);
    await signIn(page);
    await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
    expect((await new AxeBuilder({ page }).withTags(axeTags).analyze()).violations).toEqual([]);
  });
});

test.describe("ar-XB", () => {
  test.use({ locale: "ar-XB" });
  test("the sign-in page is mirrored and axe clean in ar-XB [STU-012]", async ({ page }) => {
    await mockUpstream(page);
    await page.goto("/signin");
    await expect(page.locator("main h1")).toBeVisible();
    expect((await new AxeBuilder({ page }).withTags(axeTags).analyze()).violations).toEqual([]);
  });
});
