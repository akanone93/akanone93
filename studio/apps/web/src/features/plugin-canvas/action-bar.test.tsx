// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, mock, test } from "bun:test";
import { createStudioI18n } from "@plux/i18n";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { I18nProvider } from "../../i18n-provider.tsx";
import { type Command, CommandsProvider, createRegistry } from "../../lib/commands/index.ts";
import { loadSourceCatalogue } from "../../test-harness.tsx";
import { PreviewSwitcher } from "../preview/preview-switcher.tsx";
import { defaultPreview } from "../preview/switches.ts";
import { ActionBar } from "./action-bar.tsx";

afterEach(cleanup);

function mount(
  commands: Command[],
  onZoom: (f: number) => void = () => undefined,
  onFit: () => void = () => undefined,
) {
  const registry = createRegistry(commands);
  const studio = createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false });
  render(
    <I18nProvider studio={studio}>
      <CommandsProvider value={{ registry, states: registry.stateFor([]) }}>
        <ActionBar
          validation={{ state: "idle" } as never}
          percent={80}
          onZoom={onZoom}
          onFit={onFit}
          screens={[{ id: "a", title: "Home" }]}
          current="a"
          onCurrent={() => undefined}
          onNewScreen={() => undefined}
          onPublish={() => undefined}
        />
      </CommandsProvider>
    </I18nProvider>,
  );
}

describe("[STU-050] action bar", () => {
  test("undo and redo are disabled while no edit.undo or edit.redo command is registered", async () => {
    mount([]);
    expect(
      ((await screen.findByRole("button", { name: "Undo" })) as HTMLButtonElement).getAttribute("aria-disabled") ??
        (screen.getByRole("button", { name: "Undo" }) as HTMLButtonElement).disabled,
    ).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Redo" }).hasAttribute("disabled") ||
        screen.getByRole("button", { name: "Redo" }).getAttribute("aria-disabled") === "true" ||
        (screen.getByRole("button", { name: "Redo" }) as HTMLButtonElement).disabled,
    ).toBe(true);
  });

  test("undo runs the registered command; zoom and fit call back", async () => {
    const run = mock(() => undefined);
    const onZoom = mock((_f: number) => undefined);
    const onFit = mock(() => undefined);
    mount([{ id: "edit.undo", title: "Undo", group: "navigate", permission: null, run }], onZoom, onFit);
    fireEvent.click(await screen.findByRole("button", { name: "Undo" }));
    expect(run).toHaveBeenCalledTimes(1);
    fireEvent.click(screen.getByRole("button", { name: "Zoom in" }));
    fireEvent.click(screen.getByRole("button", { name: "Zoom out" }));
    expect(onZoom.mock.calls.map((c) => c[0])).toEqual([1.25, 1 / 1.25]);
    fireEvent.click(screen.getByRole("button", { name: "Fit" }));
    expect(onFit).toHaveBeenCalledTimes(1);
    expect(screen.getByText("80%")).toBeTruthy();
  });
});

interface Wiring {
  canSend: boolean;
  canPublish: boolean;
  onPublish: () => void;
}

function mountFull(wiring: Wiring, runs: { send: () => void; pair: () => void; history: () => void }) {
  const commands: Command[] = [
    { id: "device.send", title: "Send", group: "navigate", permission: null, run: runs.send },
    { id: "device.pair", title: "Pair", group: "navigate", permission: null, run: runs.pair },
  ];
  const registry = createRegistry(commands);
  const studio = createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false });
  render(
    <I18nProvider studio={studio}>
      <CommandsProvider value={{ registry, states: registry.stateFor([]) }}>
        <ActionBar
          validation={{ state: "idle" } as never}
          percent={80}
          onZoom={() => undefined}
          onFit={() => undefined}
          screens={[{ id: "a", title: "Home" }]}
          current="a"
          onCurrent={() => undefined}
          onNewScreen={() => undefined}
          onHistory={runs.history}
          preview={
            <PreviewSwitcher part="main" state={defaultPreview} dataSources={[]} onChange={() => {}} includePseudo />
          }
          {...wiring}
        />
      </CommandsProvider>
    </I18nProvider>,
  );
}

const off = (b: HTMLElement) => b.hasAttribute("disabled") || b.getAttribute("aria-disabled") === "true";

const idle = { send: () => undefined, pair: () => undefined, history: () => undefined };

describe("[STU-060] action bar controls", () => {
  test("the toolbar holds the preview switches, send, history and Publish in that order; the switches open as a popover", async () => {
    mountFull({ canSend: true, canPublish: true, onPublish: () => undefined }, idle);
    const bar = await screen.findByRole("toolbar", { name: "Canvas actions" });
    const buttons = [...bar.querySelectorAll("button")];
    const at = (name: string) =>
      buttons.findIndex((b) => b.getAttribute("aria-label") === name || (b.textContent ?? "").trim() === name);
    const order = ["Preview", "Send to device", "History", "Publish"].map(at);
    expect(order.every((x) => x >= 0)).toBe(true);
    expect([...order].sort((a, b) => a - b)).toEqual(order);
    // The preview switches (frame, theme, locale, text scale) are in the Preview popover, in that order.
    fireEvent.click(within(bar).getByRole("button", { name: "Preview" }));
    const popover = await screen.findByRole("dialog");
    const text = popover.textContent ?? "";
    const marks = ["Device frame", "Theme", "Locale", "Text scale"].map((t) => text.indexOf(t));
    expect(marks.every((x) => x >= 0)).toBe(true);
    expect([...marks].sort((a, b) => a - b)).toEqual(marks);
  });

  test("send, pair, history and Publish each fire their action", async () => {
    const send = mock(() => undefined);
    const pair = mock(() => undefined);
    const history = mock(() => undefined);
    const publish = mock(() => undefined);
    mountFull({ canSend: true, canPublish: true, onPublish: publish }, { send, pair, history });
    fireEvent.click(await screen.findByRole("button", { name: "Send to device" }));
    fireEvent.click(screen.getByRole("button", { name: "Pair a device" }));
    fireEvent.click(screen.getByRole("button", { name: "History" }));
    fireEvent.click(screen.getByRole("button", { name: "Publish" }));
    expect([send, pair, history, publish].map((f) => f.mock.calls.length)).toEqual([1, 1, 1, 1]);
  });

  test("without the permissions send and Publish are disabled and name the permission", async () => {
    mountFull({ canSend: false, canPublish: false, onPublish: () => undefined }, idle);
    const send = (await screen.findByRole("button", { name: "Send to device" })) as HTMLButtonElement;
    const publish = screen.getByRole("button", { name: "Publish" }) as HTMLButtonElement;
    expect(off(send)).toBe(true);
    expect(send.title).toBe("Needs the permission devsession.create");
    expect(off(publish)).toBe(true);
    expect(publish.title).toBe("Needs the permission release.publish");
  });

  test("the arrow keys move focus along the controls", async () => {
    mountFull({ canSend: true, canPublish: true, onPublish: () => undefined }, idle);
    const first = await screen.findByRole("button", { name: "New screen" });
    first.focus();
    fireEvent.keyDown(first, { key: "End" });
    expect(document.activeElement?.textContent).toBe("Publish");
    fireEvent.keyDown(document.activeElement as Element, { key: "Home" });
    expect(document.activeElement).toBe(first);
  });
});

describe("action bar layout (S6.5)", () => {
  test("the bar lane runs across the top, above the docks, and does not catch pointer events; the zoom sits apart", async () => {
    mount([]);
    const bar = await screen.findByRole("toolbar", { name: "Canvas actions" });
    const lane = bar.parentElement as HTMLElement;
    expect(lane.className).toContain("pointer-events-none");
    expect(lane.className).toContain("plugin-topbar");
    expect(lane.className).toContain("top-3");
    expect(bar.className).toContain("pointer-events-auto");
    const zoom = screen.getByRole("group", { name: "Zoom" });
    expect(bar.contains(zoom)).toBe(false);
    expect(within(zoom).getByRole("button", { name: "Zoom in" })).toBeTruthy();
  });
});
