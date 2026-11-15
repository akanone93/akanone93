// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { orgA, renderApp } from "../../test-harness.tsx";
import type { CanvasSurface } from "./surface.ts";

afterEach(cleanup);

const documents = [
  { kind: "plugin", path: "plugin.json", json: { id: "pl", kind: "plugin", entryPage: "home", pages: ["home"] } },
  {
    kind: "page",
    path: "pages/home.page.json",
    json: {
      id: "home",
      key: "home",
      route: "home",
      kind: "page",
      pageKind: "screen",
      title: "Home",
      root: { id: "r", type: "Scaffold" },
    },
  },
];

/** A surface that lays screens out where the page asks and says where each frame stands (the bars sit on them). */
function surface(): CanvasSurface {
  const at = new Map<string, { x: number; y: number }>();
  return {
    layoutScreens: (docs) => {
      for (const d of docs) {
        at.set(d.id, { x: d.x, y: d.y });
      }
      return Promise.resolve(docs.map((d) => ({ id: d.id, contentHeight: 300 })));
    },
    setViewport: () => undefined,
    setPaused: () => undefined,
    pan: () => undefined,
    zoomAbout: () => undefined,
    viewport: { x: 0, y: 0, zoom: 1 },
    hitTest: () => [],
    boundsOf: (id) => {
      const p = at.get(id.replace(/^screen:/, ""));
      return p === undefined ? undefined : { l: p.x, t: p.y, r: p.x + 390, b: p.y + 844 };
    },
    setSelection: () => undefined,
    dispose: () => undefined,
  };
}

const selected = (name: RegExp) => screen.getByRole("tab", { name }).getAttribute("aria-selected") === "true";

describe("[STU-063] the inspector's tabs after a quick action", () => {
  test("Page settings opens the Page tab once; every other tab can be chosen afterwards", async () => {
    const s = surface();
    renderApp(`/o/${orgA}/apps/app1/plugins/pl`, { documents, surface: () => s });
    const bar = await screen.findByRole("toolbar", { name: "Screen Home" }, { timeout: 20_000 });
    fireEvent.click(within(bar).getByRole("button", { name: "Page settings" }));
    await waitFor(() => expect(selected(/^Page/)).toBe(true));
    fireEvent.click(screen.getByRole("tab", { name: /^Properties/ }));
    await waitFor(() => expect(selected(/^Properties/)).toBe(true));
    fireEvent.click(screen.getByRole("tab", { name: /^Editors/ }));
    await waitFor(() => expect(selected(/^Editors/)).toBe(true));
    // Another quick action switches again.
    fireEvent.click(within(bar).getByRole("button", { name: "Page settings" }));
    await waitFor(() => expect(selected(/^Page/)).toBe(true));
  }, 30_000);
});
