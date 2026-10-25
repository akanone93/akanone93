// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { Code, ConnectError } from "@connectrpc/connect";
import { createStudioI18n } from "@plux/i18n";
import { createMemoryHistory, createRootRoute, createRouter, RouterProvider } from "@tanstack/react-router";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { I18nProvider } from "../../i18n-provider.tsx";
import { createSession } from "../../lib/session.ts";
import { AppContextProvider } from "../../lib/use-app.tsx";
import { createUpstream, loadSourceCatalogue, orgA, renderApp } from "../../test-harness.tsx";
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

describe("STU-075 O14 inbox filter", () => {
  test("the unread flag in the URL is validated and the page renders", async () => {
    const r = renderApp(`/o/${orgA}/notifications?unread=true&type=`, {});
    await screen.findByRole("heading", { name: "Notifications" });
    expect((screen.getByLabelText("Unread only") as HTMLInputElement).checked).toBe(true);
    fireEvent.click(screen.getByLabelText("Unread only"));
    await waitFor(() => expect(r.router.state.location.search).not.toHaveProperty("unread", true));
  });
});
