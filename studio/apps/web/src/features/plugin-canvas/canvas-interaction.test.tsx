// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { orgA, renderApp } from "../../test-harness.tsx";
import type { EditSession } from "../editor/edit-session.ts";
import { menuEntries } from "./canvas-menu.tsx";
import type { Box } from "./select/geometry.ts";
import type { CanvasSurface } from "./surface.ts";

afterEach(cleanup);

const text = (id: string, data: string) => ({ id, type: "Text", props: { data } });
const page = (id: string, body: unknown[]) => ({
  kind: "page",
  path: `pages/${id}.page.json`,
  json: {
    id,
    key: id,
    route: id,
    kind: "page",
    pageKind: "screen",
    root: { id: `${id}-root`, type: "Scaffold", slots: { body: { id: `${id}-body`, type: "Column", children: body } } },
  },
});
const plugin = {
  kind: "plugin",
  path: "plugin.json",
  json: { id: "pl", kind: "plugin", entryPage: "home", pages: ["home", "detail"] },
};
const documents = [plugin, page("home", [text("t1", "Hello")]), page("detail", [text("t2", "Two")])];
const path = `/o/${orgA}/apps/app1/plugins/pl`;

/**
 * A surface whose hit index is the real one's shape: widgets answer as `<screen>/<node>` items and their parts, screens
 * as their frame (their names are the header bars above them, MR-2). Where a screen stands follows the documents the
 * page lays out.
 */
function surface() {
  const at = new Map<string, { x: number; y: number }>();
  const layouts: { id: string; x: number; y: number }[][] = [];
  const boxes = (): Record<string, Box> => {
    const out: Record<string, Box> = {};
    for (const [id, p] of at) {
      out[`screen:${id}`] = { l: p.x, t: p.y, r: p.x + 390, b: p.y + 844 };
    }
    const home = at.get("home");
    if (home !== undefined) {
      out["home/home-body"] = { l: home.x, t: home.y + 100, r: home.x + 390, b: home.y + 844 };
      out["home/t1#paint0"] = { l: home.x + 10, t: home.y + 110, r: home.x + 100, b: home.y + 130 };
    }
    return out;
  };
  const inside = (b: Box, x: number, y: number) => x >= b.l && x <= b.r && y >= b.t && y <= b.b;
  const area = (b: Box) => (b.r - b.l) * (b.b - b.t);
  const s: CanvasSurface = {
    layoutScreens: (docs) => {
      layouts.push(docs.map((d) => ({ id: d.id, x: d.x, y: d.y })));
      for (const d of docs) {
        at.set(d.id, { x: d.x, y: d.y });
      }
      return Promise.resolve(docs.map((d) => ({ id: d.id, contentHeight: 300 })));
    },
    setViewport: () => undefined,
    pan: () => undefined,
    zoomAbout: () => undefined,
    viewport: { x: 0, y: 0, zoom: 1 },
    hitTest: (x, y) =>
      Object.entries(boxes())
        .filter(([, b]) => inside(b, x, y))
        .sort(([, a], [, b]) => area(a) - area(b))
        .map(([id]) => id),
    boundsOf: (id) => boxes()[id] ?? boxes()[`${id}#paint0`],
    setSelection: () => undefined,
    dispose: () => undefined,
  };
  return { factory: () => s, at, layouts };
}

const press = (canvas: HTMLElement, x: number, y: number, mods: Record<string, boolean> = {}) => {
  fireEvent.pointerDown(canvas, { clientX: x, clientY: y, button: 0, ...mods });
  fireEvent.pointerUp(canvas, { clientX: x, clientY: y, button: 0, ...mods });
};

describe("[STU-067] the plugin canvas's pointer on real canvas items", () => {
  test("[STU-067] a click on a widget's item selects it: the bar, the inspector and the overlay show it", async () => {
    const s = surface();
    renderApp(path, { documents, surface: s.factory });
    const canvas = await screen.findByLabelText("Plugin canvas");
    await waitFor(() => expect(s.at.get("home")).toBeDefined());
    const home = s.at.get("home") ?? { x: 0, y: 0 };
    press(canvas, home.x + 20, home.y + 120, { ctrlKey: true });
    // The bar and the live region both say it, by what it is (not its id).
    expect((await screen.findAllByText("Text selected")).length).toBe(2);
    expect(((await screen.findByRole("textbox", { name: "data" })) as HTMLInputElement).value).toBe("Hello");
    await waitFor(() => expect(document.querySelector('.pc-outline[data-kind="selected"]') === null).toBe(false));
    expect(document.querySelector(".pc-tag")?.textContent).toBe("Text");
    // A click on the screen's header bar selects the screen, marked as its device frame; a page without a title is
    // named after its first text, on its header as in the bar.
    const header = await screen.findByRole("toolbar", { name: "Screen Hello" });
    press(header, home.x + 100, home.y - 20);
    await waitFor(() => expect(document.querySelector('.pc-outline[data-kind="screen"]') === null).toBe(false));
    expect(header.getAttribute("data-selected")).toBe("true");
    expect(header.querySelector(".pc-header-name")?.textContent).toBe("Hello");
    // A click on the empty canvas clears.
    press(canvas, home.x - 300, home.y - 300);
    await waitFor(() => expect(document.querySelector(".pc-outline") === null).toBe(true));
  });

  test("[STU-050] dragging a screen by its header bar saves every screen's place in the plugin's canvas document", async () => {
    const s = surface();
    let edit: EditSession | undefined;
    const canvasDoc = {
      kind: "canvas",
      path: "canvas.json",
      json: { schemaVersion: "1.0.0", kind: "canvas", id: "01c0c450-6c00-7000-8000-0000000000c1", positions: [] },
    };
    renderApp(path, { documents: [...documents, canvasDoc], surface: s.factory, editor: (e) => (edit = e) });
    const canvas = await screen.findByLabelText("Plugin canvas");
    await waitFor(() => expect(edit?.store.getState().docs["canvas.json"]).toBeDefined());
    await waitFor(() => expect(s.at.get("detail")).toBeDefined());
    const detail = s.at.get("detail") ?? { x: 0, y: 0 };
    const from = { x: detail.x + 100, y: detail.y - 20 };
    expect(canvas).toBeTruthy();
    const header = await screen.findByRole("toolbar", { name: "Screen Two" });
    fireEvent.pointerDown(header, { clientX: from.x, clientY: from.y, button: 0 });
    fireEvent.pointerMove(header, { clientX: from.x + 120, clientY: from.y + 200, button: 0 });
    // The drag shows the screen at its new place before anything is saved.
    await waitFor(() => expect(s.layouts.at(-1)?.find((d) => d.id === "detail")?.y).toBe(detail.y + 200));
    fireEvent.pointerUp(header, { clientX: from.x + 120, clientY: from.y + 200, button: 0 });
    await waitFor(() => {
      const content = edit?.store.getState().docs["canvas.json"]?.content as { positions?: unknown[] } | undefined;
      const positions = content?.positions;
      expect(positions).toContainEqual({ id: "detail", x: detail.x + 120, y: detail.y + 200 });
      expect(positions?.length).toBe(2);
    });
    expect((await screen.findAllByText("Scaffold selected")).length).toBe(2);
  });
});

describe("[STU-067] the canvas's context menu", () => {
  const states = ["edit.copy", "edit.paste", "edit.delete", "screen.rename", "screen.delete", "screen.new"].map(
    (id) => ({
      command: { id, title: id, group: "edit" as const, permission: null, run: () => undefined },
      enabled: id !== "edit.paste",
    }),
  );
  const actions = {
    selectParent: () => undefined,
    selectChildren: () => undefined,
    zoomToSelection: () => undefined,
    fitAll: () => undefined,
  };
  const text = {
    selectParent: "parent",
    selectChildren: "children",
    zoomToSelection: "zoom",
    zoomToScreen: "zoom screen",
    fitAll: "fit",
  };
  const ids = (target: "node" | "screen" | "canvas") =>
    menuEntries(target, states, actions, text).flatMap((e) =>
      e.kind === "item" ? [`${e.id}${e.disabled === true ? "!" : ""}${e.danger === true ? "*" : ""}`] : [],
    );

  test("[STU-067] a widget, a screen and the empty canvas each offer their own registered actions", () => {
    expect(ids("node")).toEqual(["edit.copy", "edit.paste!", "parent", "children", "zoom", "edit.delete*"]);
    expect(ids("screen")).toEqual(["screen.rename", "edit.paste!", "children", "zoom", "screen.delete*"]);
    expect(ids("canvas")).toEqual(["screen.new", "edit.paste!", "fit"]);
  });
});
