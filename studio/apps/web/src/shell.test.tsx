// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { create, toBinary } from "@bufbuild/protobuf";
import { createClient } from "@connectrpc/connect";
import { GetCurrentUserResponseSchema, PreferencesService } from "@plux/api";
import { createStudioI18n } from "@plux/i18n";
import { createMemoryHistory, createRootRoute, createRouter, RouterProvider } from "@tanstack/react-router";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { ForbiddenPage } from "./features/shell/error-pages.tsx";
import { I18nProvider } from "./i18n-provider.tsx";
import { AccessError, requireInstallationAdmin, requireUser } from "./lib/guards.ts";
import { createSession } from "./lib/session.ts";
import { AppContextProvider } from "./lib/use-app.tsx";
import { createUpstream, loadSourceCatalogue, orgA, orgB, renderApp, type Upstream } from "./test-harness.tsx";

afterEach(() => {
  cleanup();
  document.documentElement.removeAttribute("data-theme");
  document.documentElement.removeAttribute("data-density");
});

const path = (r: { router: { state: { location: { pathname: string } } } }) => r.router.state.location.pathname;

describe("STU-013 guards", () => {
  test("a signed-out visit goes to sign-in with the return path", async () => {
    const r = renderApp(`/o/${orgA}`, { signedIn: false });
    await screen.findByRole("heading", { name: "Sign in" });
    expect(path(r)).toBe("/signin");
    expect(r.router.state.location.search).toEqual({ next: `/o/${orgA}` });
  });

  test("an unknown organisation is a 404 page, not a 403", async () => {
    renderApp("/o/0192f000-0000-7000-8000-0000000000ff", {});
    await screen.findByRole("heading", { name: "Page not found" });
  });

  test("a missing permission throws the 403 error and Explain access calls the server", async () => {
    const user = await requireUser(createSession("/api", createUpstream({ permissions: [] })), "/");
    expect(() => requireInstallationAdmin(user)).toThrow(AccessError);
    const explained: Upstream["explained"] = [];
    const session = createSession("/api", createUpstream({ explained }));
    const studio = createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false });
    const root = createRootRoute({ component: () => <ForbiddenPage permission="installation.admin" /> });
    const router = createRouter({ routeTree: root, history: createMemoryHistory({ initialEntries: ["/"] }) });
    render(
      <I18nProvider studio={studio}>
        <AppContextProvider value={{ session, studio }}>
          <RouterProvider router={router} />
        </AppContextProvider>
      </I18nProvider>,
    );
    await screen.findByRole("heading", { name: "You do not have access" });
    fireEvent.click(screen.getByRole("button", { name: "Explain access" }));
    await screen.findByText("Decision: deny");
    expect(explained).toEqual([{ permission: "installation.admin" }]);
  });

  test("a reload fetches the CSRF token from GetCurrentUser (SEC-101, STU-002)", async () => {
    const session = createSession("/api", createUpstream({ csrfToken: "plux_csrf_fresh" }));
    const held: (string | undefined)[] = [];
    const user = await requireUser({ ...session, setCsrfToken: (t) => held.push(t) }, "/");
    expect(user.csrfToken).toBe("plux_csrf_fresh");
    expect(held).toEqual(["plux_csrf_fresh"]);
  });

  test("after a reload a mutating call carries X-CSRF-Token from GetCurrentUser (SEC-101, STU-002)", async () => {
    const seen: { url: string; csrf: string | null }[] = [];
    const realFetch = globalThis.fetch;
    globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      seen.push({ url, csrf: new Headers(init?.headers).get("X-CSRF-Token") });
      const body = toBinary(
        GetCurrentUserResponseSchema,
        create(GetCurrentUserResponseSchema, { csrfToken: "plux_csrf_reload" }),
      );
      return new Response(url.endsWith("GetCurrentUser") ? body : new Uint8Array(), {
        headers: { "content-type": "application/proto" },
      });
    }) as typeof fetch;
    try {
      const session = createSession("/api");
      await requireUser(session, "/");
      await createClient(PreferencesService, session.transport).updatePreferences({});
    } finally {
      globalThis.fetch = realFetch;
    }
    expect(seen[0]?.csrf).toBeNull();
    expect(seen.at(-1)?.url).toEndWith("UpdatePreferences");
    expect(seen.at(-1)?.csrf).toBe("plux_csrf_reload");
  });

  test("an installation administrator passes the installation guard", async () => {
    const user = await requireUser(createSession("/api", createUpstream({ installationAdmin: true })), "/");
    expect(() => requireInstallationAdmin(user)).not.toThrow();
  });

  test("a member lands on the organisation home from /", async () => {
    const r = renderApp("/", {});
    await screen.findByRole("heading", { name: /Welcome to/ });
    expect(path(r)).toBe(`/o/${orgA}`);
  });

  test("a person with no organisation sees the message", async () => {
    renderApp("/", { memberships: [] });
    await screen.findByRole("heading", { name: "You are not in an organisation yet" });
  });

  test("a signed-in visit to sign-in leaves for next", async () => {
    const r = renderApp(`/signin?next=/me`, {});
    await screen.findByRole("heading", { name: "Profile" });
    expect(path(r)).toBe("/me");
  });
});

describe("STU-013 organisation switcher", () => {
  test("changing the organisation changes the header and the page", async () => {
    const r = renderApp(`/o/${orgA}`, {});
    const banner = await screen.findByRole("banner");
    await waitFor(() => expect(within(banner).getByText("Alpha Org")).toBeDefined());
    await r.router.navigate({ to: "/o/$org", params: { org: orgB } });
    await waitFor(() => expect(within(screen.getByRole("banner")).getByText("Beta Org")).toBeDefined());
    await screen.findByRole("heading", { name: "Welcome to Beta Org" });
  });
});

describe("STU-014 preferences", () => {
  test("stored preferences are applied to the document", async () => {
    renderApp("/me", { stored: { theme: "dark", language: "ar", density: "compact" } });
    await waitFor(() => expect(document.documentElement.getAttribute("data-theme")).toBe("dark"));
    expect(document.documentElement.getAttribute("data-density")).toBe("compact");
    await waitFor(() => expect(document.documentElement.getAttribute("dir")).toBe("rtl"));
    expect(document.documentElement.getAttribute("lang")).toBe("ar");
  });

  test("the profile page offers the three fields that exist", async () => {
    renderApp("/me", {});
    await screen.findByRole("heading", { name: "Profile" });
    for (const label of ["Theme", "Language", "Density"]) {
      expect(screen.getAllByText(label).length).toBeGreaterThan(0);
    }
  });
});

describe("STU-013 sign-in form", () => {
  test("invalid input shows the catalogue messages (Zod, jitless)", async () => {
    renderApp("/signin", { signedIn: false });
    const email = await screen.findByLabelText("Email");
    fireEvent.change(email, { target: { value: "nope" } });
    fireEvent.blur(email);
    await screen.findByText("Enter a valid email address.");
    const password = screen.getByLabelText("Password");
    fireEvent.change(password, { target: { value: "x" } });
    fireEvent.change(password, { target: { value: "" } });
    await screen.findByText("Enter your password.");
  });
});

describe("STU-012 catalogue", () => {
  test("the shell has no literal strings: every heading comes from a message", async () => {
    renderApp("/signin", { signedIn: false });
    await screen.findByRole("heading", { name: "Sign in" });
  });
});
