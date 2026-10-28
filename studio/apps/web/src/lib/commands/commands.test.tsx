// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { createStudioI18n } from "@plux/i18n";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { ShortcutsPage } from "../../features/profile/shortcuts-page.tsx";
import { CommandPalette } from "../../features/shell/command-palette.tsx";
import { I18nProvider } from "../../i18n-provider.tsx";
import { loadSourceCatalogue } from "../../test-harness.tsx";
import { type Command, CommandsProvider, createRegistry, mergeScoped } from "./index.ts";

afterEach(cleanup);

const noop = () => undefined;
const cmd = (id: string, title: string, extra: Partial<Command> = {}): Command => ({
  id,
  title,
  group: "navigate",
  permission: null,
  run: noop,
  ...extra,
});

describe("STU-013 registry", () => {
  test("rejects duplicate ids and shortcut collisions", () => {
    expect(() => createRegistry([cmd("a", "A"), cmd("a", "B")])).toThrow("duplicate command id");
    expect(() => createRegistry([cmd("a", "A", { shortcut: "mod+k" }), cmd("b", "B", { shortcut: "Mod+K" })])).toThrow(
      "shortcut",
    );
  });

  test("STU-013 the collision check spans global and scoped commands", () => {
    const global = createRegistry([cmd("a", "A", { shortcut: "mod+k" })]);
    expect(mergeScoped(global, [cmd("canvas.wrap", "Wrap", { shortcut: "mod+alt+g" })]).commands).toHaveLength(2);
    expect(() => mergeScoped(global, [cmd("a", "Again")])).toThrow("duplicate command id");
    expect(() => mergeScoped(global, [cmd("b", "B", { shortcut: "Mod+K" })])).toThrow("shortcut");
    expect(() =>
      mergeScoped(global, [cmd("c", "C", { shortcut: "mod+j" }), cmd("d", "D", { shortcut: "mod+j" })]),
    ).toThrow("shortcut");
  });

  test("a command without the permission is disabled and names it, not hidden", () => {
    const registry = createRegistry([cmd("a", "A", { permission: "app.manage" })]);
    expect(registry.stateFor([])[0]).toMatchObject({ enabled: false, missing: "app.manage" });
    expect(registry.stateFor(["app.manage"])[0]?.enabled).toBe(true);
  });
});

function wrap(commands: Command[], permissions: string[], children: ReactNode) {
  const registry = createRegistry(commands);
  const studio = createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false });
  return (
    <I18nProvider studio={studio}>
      <CommandsProvider value={{ registry, states: registry.stateFor(permissions) }}>{children}</CommandsProvider>
    </I18nProvider>
  );
}

describe("STU-013 palette", () => {
  const ran: string[] = [];
  const commands = [
    cmd("home", "Go to home", { run: () => void ran.push("home") }),
    cmd("profile", "Go to profile"),
    cmd("publish", "Publish release", { permission: "release.publish" }),
  ];

  test("filters by fuzzy match and runs the active command on Enter", async () => {
    render(wrap(commands, [], <CommandPalette open onOpenChange={noop} />));
    const box = await screen.findByRole("combobox");
    fireEvent.change(box, { target: { value: "prof" } });
    expect(screen.getAllByRole("option")).toHaveLength(1);
    fireEvent.change(box, { target: { value: "home" } });
    fireEvent.keyDown(box, { key: "Enter" });
    expect(ran).toEqual(["home"]);
  });

  test("a disabled command shows the reason and does not run", async () => {
    render(wrap(commands, [], <CommandPalette open onOpenChange={noop} />));
    const option = (await screen.findByText("Publish release")).closest("[role=option]");
    expect(option?.getAttribute("aria-disabled")).toBe("true");
    expect(screen.getByText("Needs the permission release.publish")).toBeTruthy();
  });
});

describe("STU-013 shortcut sheet", () => {
  test("lists exactly the commands that have a shortcut", async () => {
    render(wrap([cmd("a", "Alpha", { shortcut: "alt+a" }), cmd("b", "Beta")], [], <ShortcutsPage />));
    await screen.findByRole("heading", { name: "Keyboard shortcuts" });
    expect(screen.getByText("Alpha")).toBeTruthy();
    expect(screen.queryByText("Beta")).toBeNull();
  });
});

describe("STU-013 global shortcuts", () => {
  test("fire outside inputs and are ignored inside them", async () => {
    const hits: string[] = [];
    render(wrap([cmd("t", "T", { shortcut: "alt+t", run: () => void hits.push("t") })], [], <input aria-label="x" />));
    const input = await screen.findByLabelText("x");
    // The shortcut listener is installed by an effect: wait until a press outside an input reaches it.
    await waitFor(() => {
      fireEvent.keyDown(document.body, { key: "t", code: "KeyT", altKey: true });
      expect(hits.length > 0).toBe(true);
    });
    hits.length = 0;
    fireEvent.keyDown(document.body, { key: "t", code: "KeyT", altKey: true });
    fireEvent.keyDown(input, { key: "t", code: "KeyT", altKey: true });
    expect(hits).toEqual(["t"]);
  });
});
