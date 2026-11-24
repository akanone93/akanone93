// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { cleanup, screen, waitFor } from "@testing-library/react";
import { orgA, renderApp } from "../../test-harness.tsx";
import type { CanvasSurface } from "./surface.ts";

afterEach(cleanup);

const surface = (): CanvasSurface => ({
  layoutScreens: (docs) => Promise.resolve(docs.map((d) => ({ id: d.id, contentHeight: 300 }))),
  setViewport: () => undefined,
  pan: () => undefined,
  zoomAbout: () => undefined,
  viewport: { x: 0, y: 0, zoom: 1 },
  dispose: () => undefined,
});

/** A screen whose root holds `n` texts in a column: `n + 1` widgets inside it. */
const page = (id: string, n: number) => ({
  kind: "page",
  path: `plugins/pl/pages/${id}.page.json`,
  json: {
    id,
    key: id,
    route: id,
    title: id,
    kind: "page",
    pageKind: "screen",
    root: {
      id: `${id}-root`,
      type: "Scaffold",
      slots: {
        body: {
          id: `${id}-col`,
          type: "Column",
          children: Array.from({ length: n }, (_, i) => ({ id: `${id}-t${i}`, type: "Text", props: { data: "x" } })),
        },
      },
    },
  },
});

describe("the plugin's widget count (#5)", () => {
  test("the editor's header and the Overview count the same widgets: every node but each screen's root", async () => {
    const documents = [
      {
        kind: "plugin",
        path: "plugins/pl/plugin.json",
        json: { kind: "plugin", id: "pl", key: "pl", name: "pl", pages: ["a", "b", "c"], entryPage: "a" },
      },
      page("a", 1),
      page("b", 2),
      page("c", 3),
    ];
    renderApp(`/o/${orgA}/apps/app1/plugins/pl`, { documents, surface });
    // 3 screens: (1+1) + (2+1) + (3+1) = 9 widgets inside them.
    expect(await screen.findByText("3 screens · 9 widgets", undefined, { timeout: 5000 })).toBeTruthy();
    const overview = await screen.findByText("Widgets");
    await waitFor(() => expect(overview.parentElement?.parentElement?.textContent).toContain("9"));
  });
});
