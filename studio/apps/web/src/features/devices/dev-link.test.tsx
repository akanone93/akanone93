// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { create } from "@bufbuild/protobuf";
import { DevEventSchema, DevJoinState } from "@plux/api";
import { createStudioI18n } from "@plux/i18n";
import { QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { type ReactNode, useState } from "react";
import { I18nProvider, useStudioI18n } from "../../i18n-provider.tsx";
import { createSession } from "../../lib/session.ts";
import { AppContextProvider } from "../../lib/use-app.tsx";
import { loadSourceCatalogue } from "../../test-harness.tsx";
import { createUpstream, type UpstreamData } from "../../test-upstream.ts";
import { DevLinkBar, DevLinkProvider } from "./dev-link.tsx";
import { DeviceBay } from "./device-bay.tsx";

afterEach(cleanup);

function Host({ state, children }: { state: UpstreamData; children: ReactNode }) {
  const studio = useStudioI18n();
  const [session] = useState(() => createSession("/api", createUpstream(state)));
  return (
    <AppContextProvider value={{ session, studio }}>
      <QueryClientProvider client={session.queryClient}>{children}</QueryClientProvider>
    </AppContextProvider>
  );
}

function renderLink(state: UpstreamData, children: ReactNode, enabled = true) {
  const studio = createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false });
  return render(
    <I18nProvider studio={studio}>
      <Host state={state}>
        <DevLinkProvider appId="a1" enabled={enabled}>
          {children}
        </DevLinkProvider>
      </Host>
    </I18nProvider>,
  );
}

const devices = [{ id: "d1", name: "Pixel 9", connected: true }];
const waiting = { state: DevJoinState.WAITING, number: 63, phoneName: "Pixel 8" };

describe("the app's link with its devices (DEV-002, DEV-020, ADR-0104)", () => {
  test("the app canvas shows the number a joining phone must pick, and whether a device is live", async () => {
    renderLink(
      {
        devSession: { devices, events: [] },
        phones: { phones: [], signIn: [], calls: [], joins: { r1: waiting }, listed: { r1: waiting } },
      },
      <DevLinkBar />,
    );
    const prompt = await screen.findByRole("alertdialog");
    expect(prompt.textContent).toContain("63");
    expect(prompt.textContent).toContain("Pixel 8");
    expect(await screen.findByText("Live · 1 device")).toBeTruthy();
    expect(screen.getByText("Pixel 9")).toBeTruthy();
  });

  test("a person who may not start a dev session sees no bar and no prompt", async () => {
    renderLink(
      {
        devSession: { devices, events: [] },
        phones: { phones: [], signIn: [], calls: [], joins: { r1: waiting }, listed: { r1: waiting } },
      },
      <DevLinkBar />,
      false,
    );
    await new Promise((resolve) => setTimeout(resolve, 100));
    expect(screen.queryByRole("region", { name: "Device connection" })).toBeNull();
    expect(screen.queryByRole("alertdialog")).toBeNull();
  });

  test("a plugin's console under the app's link shows the link's events: one stream for the app", async () => {
    const line = create(DevEventSchema, {
      sequence: 1n,
      deviceId: "d1",
      payload: { case: "log", value: { source: "app", message: "hello from the phone" } },
    });
    renderLink(
      { devSession: { devices, events: [line] } },
      <>
        <DevLinkBar />
        <DeviceBay
          appId="a1"
          problems={<p>Problems body</p>}
          selectedPath={undefined}
          onInspect={() => undefined}
          accepted={0}
          store={{ upload: async () => undefined }}
          route="home"
        />
      </>,
    );
    fireEvent.click(await screen.findByRole("button", { name: "Developer console" }));
    fireEvent.click(await screen.findByRole("tab", { name: /^Logs/ }));
    expect(await screen.findByText(/hello from the phone/)).toBeTruthy();
  });
});
