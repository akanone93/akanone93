// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { createStudioI18n } from "@plux/i18n";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { I18nProvider } from "../../i18n-provider.tsx";
import { loadSourceCatalogue } from "../../test-harness.tsx";
import type { PageInfo } from "../safety/index.ts";
import { appQuality } from "./app-quality.ts";
import { AppQuality } from "./app-quality.tsx";

afterEach(cleanup);

const encoder = new TextEncoder();
const page = (id: string, pluginId: string): PageInfo => ({
  id,
  key: id,
  route: id,
  title: id,
  pluginId,
  path: `${id}.json`,
});
const text = (id: string, color?: string) => ({
  id,
  type: "Text",
  props: { data: id, ...(color === undefined ? {} : { style: { color } }) },
});
const roots: Record<string, unknown> = {
  // White text on the light surface: too little contrast in light, fine in dark.
  faint: { id: "r1", type: "Column", children: [text("a", "#FFFFFFFF"), text("b")] },
  // Over the page's warning count of widgets (LIM-001: 1000), every text taking its theme colour.
  big: { id: "r2", type: "Column", children: Array.from({ length: 1001 }, (_, i) => text(`t${i}`)) },
  fine: { id: "r3", type: "Column", children: [text("c")] },
};
const app = { kind: "app", content: encoder.encode(JSON.stringify({ kind: "app" })) };

describe("the quality of each plugin on the app canvas (#288)", () => {
  const quality = appQuality([app], [page("faint", "p1"), page("big", "p2"), page("fine", "p2")], (p) => roots[p.id]);

  test("a screen with a low-contrast text, and one near its widget budget, are listed by plugin; a fine one is not", () => {
    expect(quality.map((q) => [q.pluginId, q.screens.map((s) => s.pageId)])).toEqual([
      ["p1", ["faint"]],
      ["p2", ["big"]],
    ]);
    expect(quality[0]?.screens[0]).toMatchObject({ lowContrast: 1, modes: ["light"], status: "within" });
    expect(quality[1]?.screens[0]).toMatchObject({ lowContrast: 0, nodes: 1002, status: "warning" });
  });

  test("the tab names each finding and opens its screen", async () => {
    const opened: string[] = [];
    const studio = createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false });
    render(
      <I18nProvider studio={studio}>
        <AppQuality
          plugins={quality}
          pluginName={(id) => id.toUpperCase()}
          onScreen={(p, s) => opened.push(`${p}/${s}`)}
        />
      </I18nProvider>,
    );
    const p1 = await screen.findByRole("region", { name: "P1" });
    expect(p1.textContent).toContain("1 text with too little contrast (light)");
    expect(screen.getByRole("region", { name: "P2" }).textContent).toContain("1002 of 5000 widgets");
    fireEvent.click(within(p1).getByRole("button"));
    expect(opened).toEqual(["p1/faint"]);
  });
});
