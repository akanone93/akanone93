// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, mock, test } from "bun:test";
import { createStudioI18n } from "@plux/i18n";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { I18nProvider } from "../../i18n-provider.tsx";
import { type Command, CommandsProvider, createRegistry } from "../../lib/commands/index.ts";
import { loadSourceCatalogue } from "../../test-harness.tsx";
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
        />
      </CommandsProvider>
    </I18nProvider>,
  );
}

describe("STU-050 action bar", () => {
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
