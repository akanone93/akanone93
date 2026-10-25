// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import type { ItemSpec, SceneOp } from "@plux/canvas";
import { cleanup, screen, waitFor } from "@testing-library/react";
import { orgA, renderApp } from "../../test-harness.tsx";
import { canvasSearchSchema } from "./plugin-canvas.schema.ts";
import { flowOrder, readSavedPositions } from "./positions.ts";
import type { CanvasSurface } from "./surface.ts";

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

function stub() {
  const patches: SceneOp[][] = [];
  const views: number[][] = [];
  const surface: CanvasSurface = {
    patch: (ops) => patches.push([...ops]),
    setViewport: (x, y, z) => views.push([x, y, z]),
    pan: () => undefined,
    zoomAbout: () => undefined,
    viewport: { x: 0, y: 0, zoom: 1 },
    dispose: () => undefined,
  };
  return { patches, views, factory: () => surface };
}

const adds = (ops: SceneOp[]) => ops.flatMap((o) => (o.op === "add" ? [o.item] : []));
const frames = (items: ItemSpec[]) => items.flatMap((i) => (i.kind === "frame" ? [i] : []));
const path = `/o/${orgA}/apps/app1/plugins/pl`;

describe("STU-050 plugin canvas C2", () => {
  test("draws every screen as a device frame, auto-laid-out in flow order", async () => {
    const s = stub();
    renderApp(path, { documents, surface: s.factory });
    await waitFor(() => expect(s.patches.length).toBeGreaterThan(0));
    const fs = frames(adds(s.patches[0] ?? []));
    expect(fs.map((f) => f.name)).toEqual(["Home", "Detail", "Tall"]);
    const xs = fs.map((f) => f.x);
    expect(xs).toEqual([...xs].sort((a, b) => a - b));
    expect(new Set(xs).size).toBe(3);
    expect(s.views.length).toBeGreaterThan(0);
    expect(await screen.findByLabelText("Screens in navigation order")).toBeTruthy();
  });

  test("STU-056 a tall screen gets a translucent overflow region below its frame", async () => {
    const s = stub();
    renderApp(path, { documents, surface: s.factory });
    await waitFor(() => expect(s.patches.length).toBeGreaterThan(0));
    const items = adds(s.patches[0] ?? []);
    const tall = frames(items).find((f) => f.name === "Tall");
    const region = items.find((i) => i.id === "overflow:tall");
    expect(region?.kind).toBe("rect");
    if (region?.kind === "rect" && tall !== undefined) {
      expect(region.y).toBe(tall.y + tall.h);
      expect(region.h).toBeGreaterThan(0);
    }
    expect(items.some((i) => i.id === "overflow:home")).toBe(false);
    await screen.findByText(/Tall: content beyond the screen/);
  });

  test("saved positions of the canvas document win over auto-layout", async () => {
    const s = stub();
    const canvas = { kind: "canvas", path: "canvas.json", json: { screens: { home: { x: 900, y: 40 } } } };
    renderApp(path, { documents: [...documents, canvas], surface: s.factory });
    await waitFor(() => expect(s.patches.length).toBeGreaterThan(0));
    const home = frames(adds(s.patches[0] ?? [])).find((f) => f.name === "Home");
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
