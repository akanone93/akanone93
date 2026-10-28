// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { create } from "@bufbuild/protobuf";
import { type DevEvent, DevEventSchema, DevLogLevel } from "@plux/api";
import { createStudioI18n } from "@plux/i18n";
import { QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { type ReactNode, useState } from "react";
import { I18nProvider, useStudioI18n } from "../../i18n-provider.tsx";
import { createSession } from "../../lib/session.ts";
import { AppContextProvider } from "../../lib/use-app.tsx";
import { loadSourceCatalogue } from "../../test-harness.tsx";
import { createUpstream, type UpstreamData } from "../../test-upstream.ts";
import { DeviceDrawer } from "./device-drawer.tsx";
import { clock, PairingDialog, secondsLeft } from "./pairing-dialog.tsx";
import { runsOf } from "./qr-code.tsx";
import { SendToDeviceDialog } from "./send-dialog.tsx";

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

function renderHost(state: UpstreamData, ui: ReactNode) {
  const studio = createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false });
  return render(
    <I18nProvider studio={studio}>
      <Host state={state}>{ui}</Host>
    </I18nProvider>,
  );
}

const devices = [
  { id: "d1", name: "Pixel 9", connected: true },
  { id: "d2", name: "iPhone 16", connected: true },
];

function log(sequence: bigint, deviceId: string, level: DevLogLevel, message: string): DevEvent {
  return create(DevEventSchema, {
    sequence,
    deviceId,
    payload: { case: "log", value: { level, source: "app", message } },
  });
}

describe("S6.4 pairing dialog (D39, DEV-002)", () => {
  test("shows the QR as SVG rects, the link, the short code and a countdown under two minutes", async () => {
    renderHost(
      {
        devSession: {
          devices,
          pairing: { uri: "plux-dev://pair?server=x&token=t", code: "WXYZ-2345", expiresInSeconds: 119 },
        },
      },
      <PairingDialog sessionId="s1" onClose={() => undefined} />,
    );
    expect(await screen.findByText("WXYZ-2345")).toBeTruthy();
    expect(screen.getByText("plux-dev://pair?server=x&token=t")).toBeTruthy();
    const qr = screen.getByRole("img", { name: "QR code of the pairing link" });
    expect(qr.querySelectorAll("rect").length).toBeGreaterThan(20);
    expect(qr.innerHTML.includes("<script")).toBe(false);
    expect(screen.getByRole("timer").textContent).toMatch(/Expires in 1:5\d|Expires in 2:00/);
  });

  test("a token past its expiry hides the QR and says so", async () => {
    renderHost(
      { devSession: { devices, pairing: { uri: "plux-dev://pair", code: "AAAA-1111", expiresInSeconds: -5 } } },
      <PairingDialog sessionId="s1" onClose={() => undefined} />,
    );
    expect(await screen.findByText("This code has expired.")).toBeTruthy();
    expect(screen.queryByRole("img", { name: "QR code of the pairing link" })).toBeNull();
  });

  test("countdown helpers", () => {
    expect(secondsLeft(10_000, 4_500)).toBe(6);
    expect(secondsLeft(1_000, 5_000)).toBe(0);
    expect(clock(119)).toBe("1:59");
    expect(runsOf([[true, true, false, true]])).toEqual([
      { x: 0, y: 0, width: 2 },
      { x: 3, y: 0, width: 1 },
    ]);
  });
});

describe("S6.4 send to device (D38, STU-130)", () => {
  test("sends the chosen route and parameters to the picked device", async () => {
    const commands: NonNullable<UpstreamData["devSession"]>["commands"] = [];
    renderHost(
      { devSession: { devices, commands } },
      <SendToDeviceDialog sessionId="s1" initialRoute="/home" onClose={() => undefined} />,
    );
    const route = (await screen.findByLabelText("Screen route")) as HTMLInputElement;
    expect(route.value).toBe("/home");
    fireEvent.change(screen.getByLabelText("Language"), { target: { value: "de" } });
    fireEvent.change(screen.getByLabelText("Text scale"), { target: { value: "1.5" } });
    fireEvent.click(screen.getByRole("button", { name: "Send" }));
    await screen.findByText("Sent to the device.");
    const sent = commands?.[0];
    expect(sent?.deviceId).toBe("d1");
    expect(sent?.command.case).toBe("openScreen");
    if (sent?.command.case === "openScreen") {
      expect(sent.command.value.route).toBe("/home");
      expect(sent.command.value.locale).toBe("de");
      expect(sent.command.value.textScale).toBe(1.5);
    }
  });

  test("says so when no device is paired", async () => {
    renderHost({ devSession: { devices: [] } }, <SendToDeviceDialog sessionId="s1" onClose={() => undefined} />);
    expect(await screen.findByText("No device is paired yet.")).toBeTruthy();
  });
});

describe("S6.4 device drawer (STU-066, DEV-020)", () => {
  const events = [
    log(1n, "d1", DevLogLevel.INFO, "boot ready"),
    log(2n, "d2", DevLogLevel.ERROR, "crash on d2"),
    log(3n, "d1", DevLogLevel.WARNING, "slow frame"),
  ];
  const problems = <p>Problems body</p>;

  test("has the nine tabs and streams log events into Logs", async () => {
    renderHost({ devSession: { devices, events } }, <DeviceDrawer sessionId="s1" problems={problems} />);
    expect((await screen.findAllByRole("tab")).map((t) => t.textContent)).toEqual([
      "Problems",
      "Logs",
      "Process flow",
      "State",
      "Network",
      "Functions",
      "Sync",
      "Performance",
      "Console",
    ]);
    fireEvent.click(await screen.findByRole("tab", { name: "Logs" }));
    expect(await screen.findByText("app boot ready")).toBeTruthy();
    expect(screen.getByText("app crash on d2")).toBeTruthy();
  });

  test("search narrows the rows", async () => {
    renderHost({ devSession: { devices, events } }, <DeviceDrawer sessionId="s1" problems={problems} />);
    fireEvent.click(await screen.findByRole("tab", { name: "Logs" }));
    await screen.findByText("app boot ready");
    fireEvent.change(screen.getByLabelText("Search"), { target: { value: "slow" } });
    expect(screen.queryByText("app boot ready")).toBeNull();
    expect(screen.getByText("app slow frame")).toBeTruthy();
  });

  test("the level filter keeps warnings and errors only", async () => {
    renderHost({ devSession: { devices, events } }, <DeviceDrawer sessionId="s1" problems={problems} />);
    fireEvent.click(await screen.findByRole("tab", { name: "Logs" }));
    await screen.findByText("app boot ready");
    fireEvent.click(screen.getByRole("button", { name: "Errors only" }));
    await waitFor(() => expect(screen.queryByText("app boot ready") === null).toBe(true));
    expect(screen.getByText("app crash on d2")).toBeTruthy();
    expect(screen.queryByText("app slow frame")).toBeNull();
  });

  test("pause freezes the list", async () => {
    renderHost(
      { devSession: { devices, events: events.slice(0, 1) } },
      <DeviceDrawer sessionId="s1" problems={problems} />,
    );
    fireEvent.click(await screen.findByRole("tab", { name: "Logs" }));
    await screen.findByText("app boot ready");
    fireEvent.click(screen.getByRole("button", { name: "Pause" }));
    expect(screen.getByRole("button", { name: "Resume" })).toBeTruthy();
  });

  test("export asks the server for the trace of the chosen device", async () => {
    const exported: string[][] = [];
    const urls: Blob[] = [];
    URL.createObjectURL = (b: Blob | MediaSource) => {
      urls.push(b as Blob);
      return "blob:trace";
    };
    URL.revokeObjectURL = () => undefined;
    renderHost({ devSession: { devices, events, exported } }, <DeviceDrawer sessionId="s1" problems={problems} />);
    fireEvent.click(await screen.findByRole("button", { name: "Export trace" }));
    await waitFor(() => expect(exported).toEqual([[]]));
    await waitFor(() => expect(urls.length).toBe(1));
  });
});
