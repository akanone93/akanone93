// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
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
      root: {
        id: "r",
        type: "Scaffold",
        slots: {
          body: {
            id: "b",
            type: "Center",
            slots: { child: { id: "go", type: "ElevatedButton", slots: { child: { id: "t", type: "Text" } } } },
          },
        },
      },
    },
  },
];

/** A surface whose hit test answers with the button, and which says where each laid-out frame stands. */
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
    hitTest: () => ["go"],
    boundsOf: (id) => {
      const p = at.get(id.replace(/^screen:/, ""));
      return p === undefined ? { l: 10, t: 10, r: 110, b: 50 } : { l: p.x, t: p.y, r: p.x + 390, b: p.y + 844 };
    },
    setSelection: () => undefined,
    dispose: () => undefined,
  };
}

const selected = (name: RegExp) => screen.getByRole("tab", { name }).getAttribute("aria-selected") === "true";

describe("[STU-063] the inspector's tabs after a quick action", () => {
  test("Add an action opens the Actions tab once; every other tab can be chosen afterwards", async () => {
    const s = surface();
    renderApp(`/o/${orgA}/apps/app1/plugins/pl`, { documents, surface: () => s });
    const canvas = await screen.findByLabelText("Plugin canvas", undefined, { timeout: 20_000 });
    fireEvent.pointerDown(canvas, { clientX: 1, clientY: 1, ctrlKey: true });
    fireEvent.pointerUp(canvas, { clientX: 1, clientY: 1, ctrlKey: true });
    fireEvent.click(await screen.findByRole("button", { name: "Add an action" }, { timeout: 10_000 }));
    await waitFor(() => expect(selected(/^Actions/)).toBe(true));
    fireEvent.click(screen.getByRole("tab", { name: /^Properties/ }));
    await waitFor(() => expect(selected(/^Properties/)).toBe(true));
    fireEvent.click(screen.getByRole("tab", { name: /^Editors/ }));
    await waitFor(() => expect(selected(/^Editors/)).toBe(true));
    // The same quick action switches again.
    fireEvent.click(screen.getByRole("button", { name: "Add an action" }));
    await waitFor(() => expect(selected(/^Actions/)).toBe(true));
  }, 30_000);
});
