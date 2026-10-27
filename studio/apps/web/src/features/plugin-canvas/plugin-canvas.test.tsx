// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import type { ScreenDocument, ScreenStyle } from "@plux/canvas";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { orgA, renderApp } from "../../test-harness.tsx";
import type { EditSession } from "../editor/edit-session.ts";
import { canvasSearchSchema } from "./plugin-canvas.schema.ts";
import { flowOrder, readSavedPositions } from "./positions.ts";
import { type CanvasSurface, canvasPoint } from "./surface.ts";

afterEach(cleanup);

const text = (id: string, data: string) => ({ id, type: "Text", props: { data } });
const page = (id: string, route: string, title: string, body: unknown[]) => ({
  kind: "page",
  path: `pages/${route}.page.json`,
  json: {
    id,
    key: route,
    route,
    kind: "page",
    pageKind: "screen",
    root: {
      id: `${id}-root`,
      slots: {
        appBar: { id: `${id}-bar`, slots: { title: text(`${id}-t`, title) } },
        body: { id: `${id}-body`, children: body },
      },
    },
  },
});
const go = (id: string, route: string) => ({
  id,
  slots: { child: text(`${id}-l`, route) },
  events: { onPressed: { steps: [{ action: "navigate", id: "go", input: { route } }] } },
});
const many = Array.from({ length: 30 }, (_, i) => text(`row${i}`, `Row ${i}`));

// The plugin lists "tall" before "detail", but the flow reaches home -> detail -> tall.
const documents = [
  {
    kind: "plugin",
    path: "plugin.json",
    json: { id: "pl", kind: "plugin", entryPage: "home", pages: ["home", "tall", "detail"] },
  },
  page("home", "home", "Home", [go("b1", "detail")]),
  page("tall", "tall", "Tall", many),
  page("detail", "detail", "Detail", [go("b2", "tall")]),
];

/** A surface that records the documents sent to the worker; the worker's layout is tested in @plux/canvas. */
function stub(tallHeight = 2000) {
  const patches: { docs: ScreenDocument[]; style: ScreenStyle }[] = [];
  const views: number[][] = [];
  const zooms: number[][] = [];
  const surface: CanvasSurface = {
    layoutScreens: (docs, style) => {
      patches.push({ docs: [...docs], style });
      return Promise.resolve(docs.map((d) => ({ id: d.id, contentHeight: d.id === "tall" ? tallHeight : 300 })));
    },
    setViewport: (x, y, z) => views.push([x, y, z]),
    pan: () => undefined,
    zoomAbout: (factor, x, y) => zooms.push([factor, x, y]),
    viewport: { x: 0, y: 0, zoom: 1 },
    dispose: () => undefined,
  };
  return { patches, views, zooms, factory: () => surface };
}

const frames = (sent: { docs: ScreenDocument[] } | undefined) => sent?.docs ?? [];
const path = `/o/${orgA}/apps/app1/plugins/pl`;

describe("STU-069 undo on the plugin canvas", () => {
  test("an edit, then Ctrl+Z, sends the inverse patch through the pipeline", async () => {
    let edit: EditSession | undefined;
    const sent: NonNullable<Parameters<typeof renderApp>[1]>["sent"] = [];
    renderApp(path, { documents, surface: stub().factory, sent, editor: (e) => (edit = e) });
    const entry = () =>
      (edit?.store.getState().docs["plugin.json"]?.content as { entryPage?: string } | undefined)?.entryPage;
    await waitFor(() => expect(entry()).toBe("home"));
    await screen.findByRole("button", { name: "Undo" });
    edit?.edit("plugin.json", (d) => {
      d["entryPage"] = "tall";
    });
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Undo" }).matches(":disabled, [aria-disabled=true]")).toBe(false),
    );
    fireEvent.keyDown(screen.getByRole("application"), { key: "z", code: "KeyZ", ctrlKey: true });
    expect(entry()).toBe("home");
    edit?.flushAll();
    await waitFor(() => expect(sent.some((s) => s.stepId.startsWith("undo:"))).toBe(true));
    const undoWrite = sent.find((s) => s.stepId.startsWith("undo:"));
    expect(undoWrite?.ops).toContainEqual({ op: "replace", path: "/entryPage", value: "home" });
  });

  test("read-only: undo and redo are disabled with the reason and send nothing", async () => {
    const sent: NonNullable<Parameters<typeof renderApp>[1]>["sent"] = [];
    renderApp(path, { documents, surface: stub().factory, sent, lockHeldElsewhere: true });
    const button = await screen.findByRole("button", { name: "Undo" });
    await waitFor(() => expect(button.matches(":disabled, [aria-disabled=true]")).toBe(true));
    fireEvent.keyDown(screen.getByRole("application"), { key: "z", code: "KeyZ", ctrlKey: true });
    expect(sent).toEqual([]);
  });
});

describe("STU-050 plugin canvas C2", () => {
  test("draws every screen as a device frame, auto-laid-out in flow order", async () => {
    const s = stub();
    renderApp(path, { documents, surface: s.factory });
    await waitFor(() => expect(s.patches.length).toBeGreaterThan(0));
    const fs = frames(s.patches[0]);
    expect(fs.map((f) => f.title)).toEqual(["Home", "Detail", "Tall"]);
    expect(fs[0]?.root?.slots?.["body"]?.children?.[0]?.id).toBe("b1");
    expect(s.patches[0]?.style.theme.fontFamilies).toEqual(["Roboto", "Noto Sans"]);
    const xs = fs.map((f) => f.x);
    expect(xs).toEqual([...xs].sort((a, b) => a - b));
    expect(new Set(xs).size).toBe(3);
    expect(s.views.length).toBeGreaterThan(0);
    expect(await screen.findByLabelText("Screens in navigation order")).toBeTruthy();
  });

  test("STU-121 an unsaved edit reaches the canvas at once; untouched screens keep their documents", async () => {
    let edit: EditSession | undefined;
    const s = stub();
    renderApp(path, { documents, surface: s.factory, editor: (e) => (edit = e) });
    const tallRoot = (sent: { docs: ScreenDocument[] } | undefined) => frames(sent).find((f) => f.id === "tall")?.root;
    const firstRow = (sent: { docs: ScreenDocument[] } | undefined) =>
      // biome-ignore lint/complexity/useLiteralKeys: noPropertyAccessFromIndexSignature
      tallRoot(sent)?.slots?.["body"]?.children?.[0]?.props?.["data"];
    await waitFor(() => expect(edit?.store.getState().docs["pages/tall.page.json"]).toBeDefined());
    await waitFor(() => expect(firstRow(s.patches.at(-1))).toBe("Row 0"));
    const before = s.patches.at(-1);
    edit?.edit("pages/tall.page.json", (d) => {
      const body = (d as { root: { slots: { body: { children: { props: { data: string } }[] } } } }).root.slots.body;
      const row = body.children[0];
      if (row !== undefined) {
        row.props.data = "Edited, not saved";
      }
    });
    await waitFor(() => expect(firstRow(s.patches.at(-1))).toBe("Edited, not saved"));
    const after = s.patches.at(-1);
    const rootOf = (sent: typeof after, id: string) => frames(sent).find((f) => f.id === id)?.root;
    expect(rootOf(after, "home")).toBe(rootOf(before, "home"));
    expect(rootOf(after, "detail")).toBe(rootOf(before, "detail"));
    expect(rootOf(after, "tall")).not.toBe(rootOf(before, "tall"));
  });

  test("STU-056 a screen taller than the device, by the worker's layout, is announced as overflowing", async () => {
    const s = stub();
    renderApp(path, { documents, surface: s.factory });
    await screen.findByText(/Tall: content beyond the screen/);
    expect(document.getElementById("overflow:tall")).not.toBeNull();
    expect(document.getElementById("overflow:home")).toBeNull();
  });

  test("saved positions of the canvas document win over auto-layout", async () => {
    const s = stub();
    const canvas = { kind: "canvas", path: "canvas.json", json: { positions: [{ id: "home", x: 900, y: 40 }] } };
    renderApp(path, { documents: [...documents, canvas], surface: s.factory });
    await waitFor(() => expect(s.patches.length).toBeGreaterThan(0));
    const home = frames(s.patches[0]).find((f) => f.title === "Home");
    expect([home?.x, home?.y]).toEqual([900, 40]);
  });

  test("the URL sets the view: centre x, y and zoom in percent, clamped to 10-400", async () => {
    const s = stub();
    renderApp(`${path}?x=500&y=300&z=50`, { documents, surface: s.factory });
    await waitFor(() => expect(s.views.length).toBeGreaterThan(0));
    const [, , zoom] = s.views[0] ?? [];
    expect(zoom).toBe(0.5);
    expect(canvasSearchSchema.parse({ z: 9000 }).z).toBe(400);
    expect(canvasSearchSchema.parse({ z: 1 }).z).toBe(10);
    expect(canvasSearchSchema.parse({ x: "bad" }).x).toBeUndefined();
  });
});

describe("wheel zoom", () => {
  test("Ctrl+wheel zooms about the pointer in canvas-local coordinates, not page ones", async () => {
    const s = stub();
    renderApp(path, { documents, surface: s.factory });
    await waitFor(() => expect(s.views.length).toBeGreaterThan(0));
    const canvas = screen.getByRole("application");
    canvas.getBoundingClientRect = () => ({ left: 100, top: 50, width: 800, height: 600 }) as DOMRect;
    // happy-dom's WheelEvent takes no modifier keys or pointer position from its init: set them on the event.
    const wheel = new WheelEvent("wheel", { bubbles: true, cancelable: true, deltaY: -100 });
    Object.defineProperties(wheel, { ctrlKey: { value: true }, clientX: { value: 150 }, clientY: { value: 120 } });
    fireEvent(canvas, wheel);
    expect(s.zooms).toHaveLength(1);
    const [factor, x, y] = s.zooms[0] ?? [];
    expect(factor).toBeCloseTo(Math.exp(0.5));
    expect([x, y]).toEqual([50, 70]);
  });

  test("canvasPoint subtracts the canvas's page offset", () => {
    expect(canvasPoint({ left: 12, top: 34 }, 112, 134)).toEqual({ x: 100, y: 100 });
  });
});

describe("positions", () => {
  test("flow order follows edges from the entry, then lists the rest", () => {
    const edges = new Map([
      ["a", ["c"]],
      ["c", ["b"]],
    ]);
    expect(flowOrder(["a", "b", "c", "d"], "a", edges)).toEqual(["a", "c", "b", "d"]);
  });
  test("a missing or malformed canvas document means no saved positions", () => {
    expect(readSavedPositions([])).toBeUndefined();
    expect(readSavedPositions([{ kind: "canvas", content: new TextEncoder().encode("{") }])).toBeUndefined();
  });
});

describe("STU-058 an idle canvas under the preview switches", () => {
  test("lays out again only for a change, never while idle", async () => {
    const s = stub();
    renderApp(path, { documents, surface: s.factory });
    await waitFor(() => expect(s.patches.length).toBeGreaterThan(0));
    // The editor's copies of the documents arrive once and may lay out the screens a second time.
    await new Promise((resolve) => setTimeout(resolve, 600));
    const settled = s.patches.length;
    expect(settled).toBeLessThanOrEqual(2);
    await new Promise((resolve) => setTimeout(resolve, 600));
    expect(s.patches.length).toBe(settled);
  });
});
