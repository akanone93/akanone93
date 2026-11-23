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
  headerRoom,
  routeWorthShowing,
  ScreenHeaders,
  viewToShow,
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
    lost: () => undefined,
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

  test("a press anywhere on the bar, the title included, grips its screen through the canvas; ⋯ never does", async () => {
    const { calls } = mount();
    const bar = await screen.findByRole("toolbar", { name: "Screen Home" });
    fireEvent.pointerDown(within(bar).getByText("Home"), { clientX: 10, clientY: 10, button: 0 });
    fireEvent.pointerUp(bar, { clientX: 11, clientY: 11, button: 0 });
    expect(calls.down).toEqual([["title:home"]]);
    expect(calls.up).toBe(1);
    // A click on the title no longer renames in place (#227): the title is the screen's handle.
    expect(within(bar).queryByRole("textbox")).toBeNull();
    fireEvent.pointerDown(within(bar).getByRole("button", { name: "More actions for Home" }), { button: 0 });
    expect(calls.down.length).toBe(1);
    fireEvent.contextMenu(bar);
    expect(calls.menu).toEqual([["title:home"]]);
  });

  test("[#214] at rest a bar holds no menu and no tooltip: only its ⋯ button; the menu mounts when it is opened", async () => {
    mount();
    const bar = await screen.findByRole("toolbar", { name: "Screen Home" });
    const parts = () => document.querySelectorAll("[data-slot=tooltip-trigger], [data-slot=dropdown-menu-trigger]");
    expect(parts().length).toBe(0);
    expect(document.querySelectorAll(".pc-header-button").length).toBe(2);
    fireEvent.click(within(bar).getByRole("button", { name: "More actions for Home" }));
    await waitFor(() => expect(parts().length).toBe(1));
  });

  test("the ⋯ menu selects the screen and offers the actions a screen has; the entry is not offered to the entry", async () => {
    const { calls } = mount();
    const bar = await screen.findByRole("toolbar", { name: "Screen Home" });
    fireEvent.click(within(bar).getByRole("button", { name: "More actions for Home" }));
    await waitFor(() => expect(screen.getAllByRole("menuitem").length).toBeGreaterThan(0));
    expect(calls.selects).toEqual(["home"]);
    const names = screen.getAllByRole("menuitem").map((i) => i.textContent);
    expect(names).toEqual([
      "Rename",
      "Duplicate",
      "Zoom to screen",
      "Show on every device",
      "Move to another plugin…",
      "Delete…",
    ]);
    fireEvent.click(screen.getByRole("menuitem", { name: "Duplicate" }));
    expect(calls.actions).toEqual([["duplicate", "home"]]);
  });

  test("Rename in the menu opens the field: Enter keeps the new title, Escape the old one", async () => {
    const { calls } = mount();
    const bar = await screen.findByRole("toolbar", { name: "Screen Home" });
    const rename = async () => {
      fireEvent.click(within(bar).getByRole("button", { name: "More actions for Home" }));
      fireEvent.click(await screen.findByRole("menuitem", { name: "Rename" }));
      return (await within(bar).findByRole("textbox", { name: "Screen title" })) as HTMLInputElement;
    };
    const field = await rename();
    expect(field.value).toBe("Home");
    fireEvent.change(field, { target: { value: "  Start  " } });
    fireEvent.keyDown(field, { key: "Enter" });
    expect(calls.renames).toEqual([["home", "Start"]]);
    await waitFor(() => expect(within(bar).queryByRole("textbox") === null).toBe(true));
    const again = await rename();
    fireEvent.change(again, { target: { value: "Other" } });
    fireEvent.keyDown(again, { key: "Escape" });
    expect(calls.renames.length).toBe(1);
  });

  test("another screen's menu offers to make it the entry; without the right to edit, the edits are off", async () => {
    const { calls } = mount({ canEdit: false });
    const bar = await screen.findByRole("toolbar", { name: "Screen Pick a size" });
    fireEvent.click(within(bar).getByRole("button", { name: "More actions for Pick a size" }));
    const entry = await screen.findByRole("menuitem", { name: "Make it the entry screen" });
    expect(entry.getAttribute("aria-disabled")).toBe("true");
    expect(screen.getByRole("menuitem", { name: "Delete…" }).getAttribute("aria-disabled")).toBe("true");
    expect(screen.getByRole("menuitem", { name: "Rename" }).getAttribute("aria-disabled")).toBe("true");
    expect(screen.getByRole("menuitem", { name: "Zoom to screen" }).getAttribute("aria-disabled")).toBeNull();
    fireEvent.click(screen.getByRole("menuitem", { name: "Zoom to screen" }));
    expect(calls.actions).toEqual([["zoom", "pick"]]);
  });
});

describe("[STU-050] a screen's title at low zoom (#215)", () => {
  test("reads into the free room after its frame, up to the next header in its band, less a gap", () => {
    const a = { l: 0, t: 0, r: 390, b: 844 };
    const b = { l: 590, t: 0, r: 980, b: 844 };
    const below = { l: 400, t: 2000, r: 790, b: 2844 };
    // At 20 %: 200 units to the next frame are 40 px; less the 10 px gap.
    expect(headerRoom(a, [a, b, below], 0.2)).toBe(30);
    // The last in its row: up to 240 px.
    expect(headerRoom(b, [a, b, below], 0.2)).toBe(230);
    // A frame below does not share the header band.
    expect(headerRoom(below, [a, b, below], 0.2)).toBe(230);
  });
});

describe("[STU-050] a header that takes focus outside the view (#229)", () => {
  const view = { x: 0, y: 0, zoom: 0.5 };
  const size = { width: 1000, height: 800 };
  test("leaves a header in view where it is", () => {
    expect(viewToShow({ l: 100, t: 200, r: 490, b: 1044 }, view, size)).toBeNull();
  });

  test("brings a screen off the view into it, centred across, its top under its header, at the same zoom", () => {
    const box = { l: -3000, t: 4000, r: -2610, b: 4844 };
    const to = viewToShow(box, view, size);
    expect(to).toEqual({ x: (box.l + box.r) / 2 - 1000, y: 4000 - 240 });
    // Its header now stands in the view.
    expect(viewToShow(box, { ...view, ...to }, size)).toBeNull();
  });

  test("brings a screen whose header a dock covers into the view's middle", () => {
    const box = { l: 1800, t: 200, r: 2190, b: 1044 };
    expect(viewToShow(box, view, size)).toBeNull();
    expect(viewToShow(box, view, size, true)).toEqual({ x: 1995 - 1000, y: 200 - 240 });
  });
});
