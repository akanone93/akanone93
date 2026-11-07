// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { createStudioI18n } from "@plux/i18n";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ReactNode } from "react";
import { I18nProvider } from "../../i18n-provider.tsx";
import { loadSourceCatalogue } from "../../test-harness.tsx";
import {
  type HeaderAction,
  type HeaderPointer,
  type HeaderScreen,
  headerPlace,
  routeWorthShowing,
  ScreenHeaders,
} from "./screen-header.tsx";
import type { Box } from "./select/geometry.ts";
import { createOverlayStore } from "./select/overlay-store.ts";
import type { CanvasSurface } from "./surface.ts";

afterEach(cleanup);

const home: HeaderScreen = { id: "home", root: "home-root", title: "Home", route: "home", kind: "screen", entry: true };
const sheet: HeaderScreen = {
  id: "pick",
  root: "pick-root",
  title: "Pick a size",
  route: "size-sheet",
  kind: "bottomSheet",
  entry: false,
};
const boxes: Readonly<Record<string, Box>> = {
  "home-root": { l: 0, t: 0, r: 390, b: 844 },
  "pick-root": { l: 500, t: 0, r: 890, b: 844 },
};

function mount(options: { canEdit?: boolean; selected?: readonly string[]; zoom?: number } = {}) {
  const calls = {
    down: [] as (readonly string[])[],
    up: 0,
    menu: [] as (readonly string[])[],
    actions: [] as [HeaderAction, string][],
    renames: [] as [string, string][],
    selects: [] as string[],
  };
  const pointer: HeaderPointer = {
    down: (_e, hits) => calls.down.push(hits),
    move: () => undefined,
    up: () => {
      calls.up += 1;
    },
    cancel: () => undefined,
    menu: (_e, hits) => calls.menu.push(hits),
  };
  const surface = { viewport: { x: -40, y: -60, zoom: options.zoom ?? 1 } } as unknown as CanvasSurface;
  const studio = createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false });
  const node: ReactNode = (
    <I18nProvider studio={studio}>
      <ScreenHeaders
        screens={[home, sheet]}
        store={createOverlayStore()}
        surface={() => surface}
        boxOf={(id) => boxes[id]}
        selected={new Set(options.selected ?? [])}
        canEdit={options.canEdit ?? true}
        pointer={pointer}
        onSelect={(s) => calls.selects.push(s.id)}
        onAction={(a, s) => calls.actions.push([a, s.id])}
        onRename={(s, t) => calls.renames.push([s.id, t])}
      />
    </I18nProvider>
  );
  return { calls, ...render(node) };
}

describe("[STU-050] MR-2 a header bar above each screen", () => {
  test("names each screen with its kind, marks the entry, and shows the route only when it says more", async () => {
    mount({ selected: ["pick-root"] });
    const bar = await screen.findByRole("toolbar", { name: "Screen Home" });
    expect(within(bar).getByText("Home")).toBeTruthy();
    expect(within(bar).getByText("Entry")).toBeTruthy();
    // The route is the title's own slug: it says nothing more.
    expect(bar.querySelector(".pc-header-route")).toBeNull();
    // The kind's icon (its name is the catalogue's select, which the test catalogue does not format).
    expect(bar.querySelector(".pc-header-kind svg")?.getAttribute("data-icon-name")).toBe("device-mobile");
    const other = screen.getByRole("toolbar", { name: "Screen Pick a size" });
    expect(within(other).queryByText("Entry")).toBeNull();
    expect(other.querySelector(".pc-header-route")?.textContent).toBe("size-sheet");
    expect(other.querySelector(".pc-header-kind svg")?.getAttribute("data-icon-name")).toBe("rows");
    expect(other.getAttribute("data-selected")).toBe("true");
    expect(bar.getAttribute("data-selected")).toBeNull();
  });

  test("sits on its frame's top start, as wide as the frame at the view's zoom", () => {
    expect(headerPlace({ l: 500, t: 0, r: 890, b: 844 }, { x: -40, y: -60, zoom: 0.5 })).toEqual({
      x: 270,
      y: 30,
      w: 195,
    });
    expect(routeWorthShowing("Pick a size", "pick-a-size")).toBe(false);
    expect(routeWorthShowing("Pick a size", "size-sheet")).toBe(true);
    expect(routeWorthShowing("Home", undefined)).toBe(false);
  });

  test("a press on the bar grips its screen through the canvas, as a hit on its title; its buttons never do", async () => {
    const { calls } = mount();
    const bar = await screen.findByRole("toolbar", { name: "Screen Home" });
    fireEvent.pointerDown(bar, { clientX: 10, clientY: 10, button: 0 });
    fireEvent.pointerUp(bar, { clientX: 60, clientY: 90, button: 0 });
    expect(calls.down).toEqual([["title:home"]]);
    expect(calls.up).toBe(1);
    fireEvent.pointerDown(within(bar).getByRole("button", { name: "Page settings" }), { button: 0 });
    expect(calls.down.length).toBe(1);
    fireEvent.contextMenu(bar);
    expect(calls.menu).toEqual([["title:home"]]);
  });

  test("the quick actions open the page settings, the device matrix and zoom to the screen", async () => {
    const { calls } = mount();
    const bar = await screen.findByRole("toolbar", { name: "Screen Pick a size" });
    for (const name of ["Page settings", "Show on every device", "Zoom to screen"]) {
      fireEvent.click(within(bar).getByRole("button", { name }));
    }
    expect(calls.actions).toEqual([
      ["settings", "pick"],
      ["matrix", "pick"],
      ["zoom", "pick"],
    ]);
  });

  test("a click on the title renames in place: Enter keeps the new title, Escape the old one", async () => {
    const { calls } = mount();
    const bar = await screen.findByRole("toolbar", { name: "Screen Home" });
    const title = within(bar).getByRole("button", { name: "Rename Home" });
    fireEvent.pointerDown(title, { clientX: 20, clientY: 10, button: 0 });
    fireEvent.pointerUp(bar, { clientX: 21, clientY: 11, button: 0 });
    const field = (await within(bar).findByRole("textbox", { name: "Screen title" })) as HTMLInputElement;
    expect(field.value).toBe("Home");
    fireEvent.change(field, { target: { value: "  Start  " } });
    fireEvent.keyDown(field, { key: "Enter" });
    expect(calls.renames).toEqual([["home", "Start"]]);
    expect(within(bar).queryByRole("textbox")).toBeNull();
    // Escape keeps the title; the keyboard opens the field too.
    fireEvent.click(within(bar).getByRole("button", { name: "Rename Home" }), { detail: 0 });
    const again = await within(bar).findByRole("textbox", { name: "Screen title" });
    fireEvent.change(again, { target: { value: "Other" } });
    fireEvent.keyDown(again, { key: "Escape" });
    expect(calls.renames.length).toBe(1);
    expect(within(bar).queryByRole("textbox")).toBeNull();
  });

  test("a drag from the title moves the screen and does not rename it; without the right to edit it never does", async () => {
    const { calls } = mount({ canEdit: false });
    const bar = await screen.findByRole("toolbar", { name: "Screen Home" });
    const title = within(bar).getByRole("button", { name: "Rename Home" });
    expect(title.getAttribute("aria-disabled")).toBe("true");
    fireEvent.pointerDown(title, { clientX: 20, clientY: 10, button: 0 });
    fireEvent.pointerUp(bar, { clientX: 21, clientY: 11, button: 0 });
    expect(within(bar).queryByRole("textbox")).toBeNull();
    cleanup();
    const editable = mount();
    const bar2 = await screen.findByRole("toolbar", { name: "Screen Home" });
    fireEvent.pointerDown(within(bar2).getByRole("button", { name: "Rename Home" }), { clientX: 20, clientY: 10 });
    fireEvent.pointerUp(bar2, { clientX: 140, clientY: 60 });
    expect(within(bar2).queryByRole("textbox")).toBeNull();
    expect(editable.calls.down).toEqual([["title:home"]]);
    expect(calls.renames).toEqual([]);
  });

  test("the ⋯ menu selects the screen and offers every screen action; the entry is not offered to the entry", async () => {
    const { calls } = mount();
    const bar = await screen.findByRole("toolbar", { name: "Screen Home" });
    fireEvent.click(within(bar).getByRole("button", { name: "More actions for Home" }));
    await waitFor(() => expect(screen.getAllByRole("menuitem").length).toBeGreaterThan(0));
    expect(calls.selects).toEqual(["home"]);
    const names = screen.getAllByRole("menuitem").map((i) => i.textContent);
    expect(names).toEqual([
      "Rename",
      "Duplicate",
      "Page settings",
      "Show on every device",
      "Zoom to screen",
      "Move to another plugin…",
      "Delete…",
    ]);
    fireEvent.click(screen.getByRole("menuitem", { name: "Duplicate" }));
    expect(calls.actions).toEqual([["duplicate", "home"]]);
  });

  test("another screen's menu offers to make it the entry; without the right to edit, the edits are off", async () => {
    const { calls } = mount({ canEdit: false });
    const bar = await screen.findByRole("toolbar", { name: "Screen Pick a size" });
    fireEvent.click(within(bar).getByRole("button", { name: "More actions for Pick a size" }));
    const entry = await screen.findByRole("menuitem", { name: "Make it the entry screen" });
    expect(entry.getAttribute("aria-disabled")).toBe("true");
    expect(screen.getByRole("menuitem", { name: "Delete…" }).getAttribute("aria-disabled")).toBe("true");
    expect(screen.getByRole("menuitem", { name: "Page settings" }).getAttribute("aria-disabled")).toBeNull();
    fireEvent.click(screen.getByRole("menuitem", { name: "Zoom to screen" }));
    expect(calls.actions).toEqual([["zoom", "pick"]]);
  });
});
