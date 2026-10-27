// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { Code, ConnectError } from "@connectrpc/connect";
import { createStudioI18n } from "@plux/i18n";
import { createMemoryHistory, createRootRoute, createRouter, RouterProvider } from "@tanstack/react-router";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { I18nProvider } from "../../i18n-provider.tsx";
import { fetchServerVersion } from "../../lib/server-version.ts";
import { createSession } from "../../lib/session.ts";
import { AppContextProvider } from "../../lib/use-app.tsx";
import { createUpstream, loadSourceCatalogue, orgA, renderApp } from "../../test-harness.tsx";
import { isMutable, kindOf } from "../notifications/notification-kinds.ts";
import { HelpMenu } from "./help-menu.tsx";
import { StatusChips, UpdateBanner } from "./status-chips.tsx";
import { backoffMs } from "./studio-events.ts";

afterEach(cleanup);

function renderPlain(node: React.ReactNode) {
  const studio = createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false });
  const root = createRootRoute({ component: () => node });
  const router = createRouter({ routeTree: root, history: createMemoryHistory({ initialEntries: ["/"] }) });
  return render(
    <I18nProvider studio={studio}>
      <AppContextProvider value={{ session: createSession("/api", createUpstream({})), studio }}>
        <RouterProvider router={router} />
      </AppContextProvider>
    </I18nProvider>,
  );
}

describe("STU-075 E13 chips and E14 banner", () => {
  test("offline wins over reconnecting; live shows nothing", async () => {
    const view = renderPlain(<StatusChips online={false} stream="reconnecting" />);
    await screen.findByText("Offline");
    expect(screen.queryByText("Reconnecting")).toBeNull();
    view.unmount();
    renderPlain(<StatusChips online stream="reconnecting" />);
    await screen.findByText("Reconnecting");
  });

  test("the banner shows only when the versions differ", async () => {
    const view = renderPlain(<UpdateBanner loadedVersion="1.0.0" serverVersion="1.0.0" />);
    await waitFor(() => expect(view.container.textContent).toBe(""));
    view.unmount();
    renderPlain(<UpdateBanner loadedVersion="1.0.0" serverVersion="1.1.0" />);
    await screen.findByText(/Studio was updated/);
  });

  test("backoff doubles and is capped", () => {
    expect(backoffMs(0)).toBe(500);
    expect(backoffMs(1)).toBe(1000);
    expect(backoffMs(20)).toBe(30_000);
    expect(new ConnectError("x", Code.Unavailable).code).toBe(Code.Unavailable);
  });
});

describe("STU-075 E11 help menu and E14 version source", () => {
  const json = (body: unknown, ok = true) => (async () => ({ ok, json: async () => body })) as unknown as typeof fetch;

  test("/version is validated; failures and bad shapes read as unknown", async () => {
    expect(await fetchServerVersion(json({ studio: "1.0.0", docs: "https://d.example" }))).toEqual({
      studio: "1.0.0",
      docs: "https://d.example",
    });
    expect(await fetchServerVersion(json({ studio: 1 }))).toBeUndefined();
    expect(await fetchServerVersion(json({}, false))).toBeUndefined();
    expect(
      await fetchServerVersion((async () => Promise.reject(new Error("x"))) as unknown as typeof fetch),
    ).toBeUndefined();
  });

  test("documentation entries need a configured docs URL", async () => {
    const view = renderPlain(<HelpMenu docsUrl={undefined} />);
    fireEvent.click(await screen.findByRole("button", { name: "Help" }));
    await screen.findByText("Keyboard shortcuts");
    expect(screen.queryByText("Documentation")).toBeNull();
    view.unmount();
    renderPlain(<HelpMenu docsUrl="https://d.example" />);
    fireEvent.click(await screen.findByRole("button", { name: "Help" }));
    await screen.findByText("Documentation");
  });
});

describe("STU-075 O14 inbox filter", () => {
  test("the unread flag in the URL is validated and the page renders", async () => {
    const r = renderApp(`/o/${orgA}/notifications?unread=true&type=`, {});
    await screen.findByRole("heading", { name: "Notifications" });
    expect((screen.getByLabelText("Unread only") as HTMLInputElement).checked).toBe(true);
    fireEvent.click(screen.getByLabelText("Unread only"));
    await waitFor(() => expect(r.router.state.location.search).not.toHaveProperty("unread", true));
  });

  const inbox = [
    { id: "n1", eventType: "release.published", title: "Release one", read: false },
    { id: "n2", eventType: "member.added", title: "Member two", read: true },
    { id: "n3", eventType: "release.published", title: "Release three", read: true },
  ];

  test("?type filters the list to that event type, repeats grouped and worded from the type", async () => {
    renderApp(`/o/${orgA}/notifications?type=release.published`, { inbox });
    await screen.findByText("2 publishes finished");
    expect(screen.queryByText("Release one")).toBeNull();
    expect(screen.queryByText("Member two")).toBeNull();
  });

  test("?unread asks for unread items only", async () => {
    renderApp(`/o/${orgA}/notifications?unread=true`, { inbox });
    await screen.findByText("1 publish finished");
    expect(screen.queryByText("Member two")).toBeNull();
  });

  test("F16 retention comes from the registry and mark all read is offered", async () => {
    renderApp(`/o/${orgA}/notifications`, { inbox });
    await screen.findByText("2 publishes finished");
    expect(screen.getByText(/kept for 90 days/)).toBeTruthy();
    expect((screen.getByRole("button", { name: "Mark all as read" }) as HTMLButtonElement).disabled).toBe(false);
  });

  test("F16 approvals link to their audited record and sit under their own heading", async () => {
    const approvals = [{ id: "a1", eventType: "approval.requested", read: false, link: "/o/x/audit/1" }];
    renderApp(`/o/${orgA}/notifications`, { inbox: approvals });
    await screen.findByText("1 approval requested");
    expect(screen.getByRole("heading", { name: "Approvals" })).toBeTruthy();
    expect(screen.getByRole("link", { name: "View audited record" }).getAttribute("href")).toBe("/o/x/audit/1");
  });
});

describe("F16 kinds and mutes", () => {
  test("approval and security types cannot be muted", () => {
    expect(isMutable("approval.requested")).toBe(false);
    expect(isMutable("breakglass.used")).toBe(false);
    expect(isMutable("security.role_changed")).toBe(false);
    expect(isMutable("release.published")).toBe(true);
    expect(kindOf("rollout.paused")).toBe("operations");
  });

  test("the profile lists every type, locking the approval ones", async () => {
    renderApp("/me");
    const approval = (await screen.findAllByRole("checkbox")).find((c) => (c as HTMLInputElement).disabled);
    expect(approval).toBeTruthy();
    const publish = screen.getAllByRole("checkbox").find((c) => !(c as HTMLInputElement).disabled) as HTMLInputElement;
    fireEvent.click(publish);
    await waitFor(() => expect(publish.checked).toBe(true));
    fireEvent.click(publish);
    await waitFor(() => expect(publish.checked).toBe(false));
  });

  test("a mute is stored in the server preferences under its own mask path (STU-013)", async () => {
    const updates: NonNullable<Parameters<typeof renderApp>[1]>["updates"] = [];
    renderApp("/me", { updates });
    const publish = (await screen.findAllByRole("checkbox")).find((c) => !(c as HTMLInputElement).disabled);
    fireEvent.click(publish as HTMLElement);
    await waitFor(() => expect(updates?.length).toBe(1));
    expect(updates?.[0]?.paths).toEqual(["muted_notification_types"]);
    expect(updates?.[0]?.mutedNotificationTypes?.length).toBe(1);
    expect(localStorage.getItem("plux.notifications.muted")).toBeNull();
  });
});
