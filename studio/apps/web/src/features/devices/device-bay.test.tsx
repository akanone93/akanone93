// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { create } from "@bufbuild/protobuf";
import { DevEventSchema, type DevSessionServiceSendCommandRequest } from "@plux/api";
import { createStudioI18n } from "@plux/i18n";
import { QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { type ReactNode, useState } from "react";
import { I18nProvider, useStudioI18n } from "../../i18n-provider.tsx";
import { createSession } from "../../lib/session.ts";
import { AppContextProvider } from "../../lib/use-app.tsx";
import { loadSourceCatalogue } from "../../test-harness.tsx";
import { createUpstream, type UpstreamData } from "../../test-upstream.ts";
import { DeviceBay, type DeviceBayProps } from "./device-bay.tsx";
import { type InspectScreen, nodePathOf, resolveNodePath } from "./inspect-path.ts";
import { createLatencyMeter } from "./latency.ts";
import { screenshotAssetName, storeScreenshot } from "./screenshots.ts";

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

function renderBay(state: UpstreamData, props: Partial<DeviceBayProps> = {}) {
  const studio = createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false });
  const full: DeviceBayProps = {
    appId: "a1",
    problems: <p>Problems body</p>,
    collapsed: <p>Collapsed body</p>,
    selectedPath: undefined,
    onInspect: () => undefined,
    accepted: 0,
    store: { upload: async () => undefined },
    route: "home",
    ...props,
  };
  const view = (p: DeviceBayProps) => (
    <I18nProvider studio={studio}>
      <Host state={state}>
        <DeviceBay {...p} />
      </Host>
    </I18nProvider>
  );
  const out = render(view(full));
  return { ...out, update: (next: Partial<DeviceBayProps>) => out.rerender(view({ ...full, ...next })) };
}

const devices = [{ id: "d1", name: "Pixel 9", connected: true }];
const screens: InspectScreen[] = [
  { id: "home", root: { id: "r", children: [{ id: "a" }, { id: "b", children: [{ id: "c" }] }] } },
];

describe("S6.4 device bay (STU-131 to STU-134, WGT-031, NFR-030)", () => {
  test("the toolbar opens the drawer, which pauses (DEV-020)", async () => {
    renderBay({ devSession: { devices } });
    expect(screen.queryByRole("complementary")).toBeNull();
    fireEvent.click(await screen.findByRole("button", { name: "Devices" }));
    expect(await screen.findByRole("button", { name: "Pause" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Pause" }));
    expect(screen.getByRole("button", { name: "Resume" })).toBeTruthy();
  });

  test("pairing and send are not in the bay toolbar: the action bar runs their commands (STU-060)", async () => {
    renderBay({ devSession: { devices } });
    await screen.findByRole("button", { name: "Devices" });
    expect(screen.queryByRole("button", { name: "Pair a device" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Send to device" })).toBeNull();
  });

  test("selecting a node in Studio highlights it on the device (STU-131)", async () => {
    const commands: DevSessionServiceSendCommandRequest[] = [];
    const bay = renderBay({ devSession: { devices, commands } });
    fireEvent.click(await screen.findByRole("button", { name: "Devices" }));
    fireEvent.click(await screen.findByRole("button", { name: "Highlight selection on device" }));
    bay.update({ selectedPath: "/pages/home/1/0" });
    await waitFor(() => expect(commands.map((c) => c.command.case)).toContain("inspectSelect"));
    const sent = commands.find((c) => c.command.case === "inspectSelect");
    expect(sent?.command.value).toMatchObject({ nodePath: "/pages/home/1/0" });
  });

  test("a tap on the device selects the node in Studio (STU-131)", async () => {
    const inspected: string[] = [];
    const tap = create(DevEventSchema, {
      sequence: 1n,
      deviceId: "d1",
      payload: { case: "inspect", value: { nodePath: "/pages/home/1/0", widget: "Text" } },
    });
    renderBay({ devSession: { devices, events: [tap] } }, { onInspect: (p) => inspected.push(p) });
    fireEvent.click(await screen.findByRole("button", { name: "Devices" }));
    await waitFor(() => expect(inspected).toEqual(["/pages/home/1/0"]));
    expect(resolveNodePath(screens, "/pages/home/1/0")).toEqual({ screen: "home", node: "c" });
  });

  test("device controls send one OpenScreen with locale, theme and text scale (STU-132)", async () => {
    const commands: DevSessionServiceSendCommandRequest[] = [];
    renderBay({ devSession: { devices, commands } });
    fireEvent.click(await screen.findByRole("button", { name: "Devices" }));
    fireEvent.change(await screen.findByLabelText("Language"), { target: { value: "de" } });
    fireEvent.click(screen.getByRole("button", { name: "Apply" }));
    await waitFor(() => expect(commands.length).toBe(1));
    expect(commands[0]?.command.case).toBe("openScreen");
    expect(commands[0]?.command.value).toMatchObject({ route: "home", locale: "de", textScale: 1 });
  });

  test("a screenshot is requested, shown and stored as the screen thumbnail (STU-134, WGT-031)", async () => {
    URL.createObjectURL = () => "blob:shot";
    URL.revokeObjectURL = () => undefined;
    const commands: DevSessionServiceSendCommandRequest[] = [];
    const uploads: string[] = [];
    const shot = create(DevEventSchema, {
      sequence: 1n,
      deviceId: "d1",
      payload: { case: "screenshot", value: { png: new Uint8Array([137, 80]), platformViewsNotCaptured: true } },
    });
    renderBay(
      { devSession: { devices, commands, events: [shot] } },
      {
        store: {
          upload: async (job) => {
            uploads.push(job.name);
          },
        },
      },
    );
    fireEvent.click(await screen.findByRole("button", { name: "Devices" }));
    fireEvent.click(await screen.findByRole("button", { name: "Capture screenshot" }));
    await waitFor(() => expect(commands.some((c) => c.command.case === "captureScreenshot")).toBe(true));
    expect(await screen.findByAltText("Screenshot of the paired device")).toBeTruthy();
    expect(screen.getByText("Native views on the screen are not part of the capture.")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Save screenshot" }));
    await waitFor(() => expect(uploads).toEqual(["thumbnails/screen-home.png"]));
  });

  test("the autosave-to-device time is measured and shown (NFR-030)", async () => {
    let release: () => void = () => undefined;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    const applied = create(DevEventSchema, {
      sequence: 1n,
      deviceId: "d1",
      payload: { case: "hotApply", value: { bundleSha256: "ab", pagesApplied: ["home"], applyMicros: 90_000n } },
    });
    const ticks = [1000, 1180];
    const bay = renderBay({ devSession: { devices, events: [applied], gate } }, { now: () => ticks.shift() ?? 0 });
    fireEvent.click(await screen.findByRole("button", { name: "Devices" }));
    await screen.findByRole("button", { name: "Pause" });
    bay.update({ accepted: 1 });
    await act(async () => release());
    expect((await screen.findByText("Autosave to device: 180 ms")).getAttribute("role")).toBe("status");
  });
});

describe("S6.4 pure parts", () => {
  test("a node path round-trips through the screens (DEV-030)", () => {
    expect(nodePathOf(screens, "c")).toBe("/pages/home/1/0");
    expect(nodePathOf(screens, "missing")).toBeUndefined();
    expect(resolveNodePath(screens, "/pages/home/9")).toBeUndefined();
    expect(resolveNodePath(screens, "elsewhere")).toBeUndefined();
  });

  test("the meter reports the time of the oldest unanswered autosave (NFR-030)", () => {
    const clock = [10, 20, 100, 150];
    const meter = createLatencyMeter(() => clock.shift() ?? 0);
    meter.accepted();
    meter.accepted();
    expect(meter.applied()).toBe(90);
    expect(meter.applied()).toBe(130);
    expect(meter.applied()).toBeUndefined();
  });

  test("screenshots are stored under per-use asset names (STU-134, WGT-031)", async () => {
    expect(screenshotAssetName("slotPreview", "map")).toBe("previews/slot-map.png");
    expect(screenshotAssetName("translationContext", "home")).toBe("l10n-context/screen-home.png");
    const names: string[] = [];
    const out = await storeScreenshot(
      { upload: async (j) => void names.push(j.name) },
      "thumbnail",
      "home",
      new Uint8Array([1]),
    );
    expect(names).toEqual(["thumbnails/screen-home.png"]);
    expect(out.sha256).toHaveLength(64);
  });
});
