// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { createStudioI18n } from "@plux/i18n";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { I18nProvider } from "../../i18n-provider.tsx";
import { loadSourceCatalogue } from "../../test-harness.tsx";
import {
  addPartRecipe,
  drawersOf,
  drawersOfSelection,
  missingParts,
  partTemplate,
  selectedPageScaffold,
  shownDrawers,
  withOpenDrawers,
} from "./page-parts.ts";
import { PageParts } from "./page-parts.tsx";
import type { Node } from "./plugin-model.ts";
import { createOverlayStore } from "./select/overlay-store.ts";
import type { CanvasSurface } from "./surface.ts";

afterEach(cleanup);

const texts = { title: "Loans", home: "Home", search: "Search", profile: "Profile", settings: "Settings" };
let n = 0;
const ids = () => `id-${++n}`;

const drawer: Node = {
  id: "dr",
  type: "Drawer",
  slots: { child: { id: "dl", type: "ListView", children: [{ id: "tile", type: "ListTile" }] } },
};
const page: Node = {
  id: "sc",
  type: "Scaffold",
  slots: { appBar: { id: "bar", type: "AppBar" }, body: { id: "body", type: "Column" }, drawer },
};

describe("PC-12 page parts [STU-067]", () => {
  test("a page's scaffold offers the parts it lacks, in order; other widgets offer none", () => {
    expect(missingParts(page)).toEqual(["bottomNavigationBar", "floatingActionButton"]);
    expect(missingParts({ id: "x", type: "Scaffold" })).toEqual([
      "appBar",
      "bottomNavigationBar",
      "floatingActionButton",
      "drawer",
    ]);
    expect(missingParts({ id: "c", type: "Column" })).toEqual([]);
    expect(drawersOf(page)).toEqual(["drawer"]);
  });

  test("each part is added as a starter widget with fresh ids, and never over a filled slot", () => {
    const bar = partTemplate("bottomNavigationBar", texts, ids);
    expect(bar).toMatchObject({ type: "NavigationBar", props: { selectedIndex: 0 } });
    expect((bar["props"] as { destinations: unknown[] }).destinations).toHaveLength(3);
    expect(partTemplate("appBar", texts, ids)).toMatchObject({
      type: "AppBar",
      slots: { title: { props: { data: "Loans" } } },
    });
    // The button has an action, so it is drawn enabled as on the device.
    expect(partTemplate("floatingActionButton", texts, ids)).toMatchObject({
      events: { onPressed: { steps: [{ action: "haptic" }] } },
    });
    const root = structuredClone(page) as unknown as Record<string, unknown>;
    addPartRecipe("sc", "bottomNavigationBar", bar)(root);
    expect((root["slots"] as Record<string, unknown>)["bottomNavigationBar"]).toBe(bar);
    const other = partTemplate("appBar", texts, ids);
    addPartRecipe("sc", "appBar", other)(root);
    expect((root["slots"] as Record<string, { id: string }>)["appBar"]?.id).toBe("bar");
  });

  test("the drawer the selection is in is shown; the user's choice wins; the document is never marked", () => {
    expect(drawersOfSelection([page], ["tile"])).toEqual(new Map([["sc", "drawer"]]));
    expect(drawersOfSelection([page], ["body"]).size).toBe(0);
    expect(shownDrawers(new Map([["sc", null]]), new Map([["sc", "drawer"]])).size).toBe(0);
    expect(shownDrawers(new Map([["sc", "endDrawer"]]), new Map()).get("sc")).toBe("endDrawer");
    const shown = withOpenDrawers(page, new Map([["sc", "drawer"]])) as unknown as Record<string, unknown>;
    expect(shown["x-studio"]).toEqual({ open: "drawer" });
    expect((page as unknown as Record<string, unknown>)["x-studio"]).toBeUndefined();
    expect(selectedPageScaffold([page], ["sc"])?.id).toBe("sc");
    expect(selectedPageScaffold([page], ["body"])).toBeUndefined();
  });

  test("the chips sit on the selected page's frame; a click adds the part or shows the drawer", async () => {
    const added: string[] = [];
    const toggled: string[] = [];
    const surface = {
      viewport: { x: 0, y: 0, zoom: 1 },
      boundsOf: (id: string) => (id === "screen:s1" ? { l: 100, t: 50, r: 490, b: 894 } : undefined),
    } as unknown as CanvasSurface;
    const wrap = (node: ReactNode) => (
      <I18nProvider studio={createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false })}>
        {node}
      </I18nProvider>
    );
    render(
      wrap(
        <PageParts
          store={createOverlayStore()}
          surface={() => surface}
          screen="s1"
          scaffold={page}
          insets={{ top: 47, bottom: 34 }}
          rtl={false}
          canEdit
          shown={new Map()}
          onAdd={(p) => added.push(p)}
          onToggleDrawer={(s) => toggled.push(s)}
        />,
      ),
    );
    const group = await screen.findByRole("group", { name: "Page parts" });
    expect(group).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Navigation bar" }));
    fireEvent.click(screen.getByRole("button", { name: "Show drawer" }));
    expect(added).toEqual(["bottomNavigationBar"]);
    expect(toggled).toEqual(["drawer"]);
    expect(screen.queryByRole("button", { name: "App bar" })).toBeNull();
    const nav = screen.getByRole("button", { name: "Navigation bar" });
    expect(nav.style.getPropertyValue("--y")).toBe(String(894 - 34 - 12));
  });
});
