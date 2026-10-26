// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import type { I18n } from "@lingui/core";
import type { PluginSummary } from "@plux/api";
import type { Guides, SceneOp } from "@plux/canvas";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { orgA, renderApp, type Upstream } from "../../test-harness.tsx";
import type { CanvasSurface } from "../plugin-canvas/surface.ts";
import { appCanvasMessages } from "./app-canvas.messages.ts";
import { badgesOf, type SceneBox } from "./app-canvas.scene.ts";
import { PluginBoxCard } from "./app-canvas-page.tsx";

afterEach(cleanup);

// A catalogue-free translator: the message with its values appended.
const i18n = {
  _: (d: { message: string; values?: Record<string, unknown> }) =>
    `${d.message}${d.values === undefined ? "" : ` ${JSON.stringify(d.values)}`}`,
} as unknown as I18n;
const m = appCanvasMessages(i18n);

const summary = (over: Record<string, unknown>) =>
  ({
    plugin: { id: "p1", key: "loans", name: "Loans", archived: false, tags: [] },
    lockHolder: "",
    unpublishedChanges: false,
    validationErrors: 0n,
    killed: false,
    linksOut: [],
    linksIn: [],
    functionsUsed: [],
    ...over,
  }) as unknown as PluginSummary;

const sceneBox = (s: PluginSummary, entry: boolean): SceneBox => ({
  id: "p1",
  key: "loans",
  x: 10,
  y: 20,
  title: "Loans",
  native: false,
  archived: false,
  badges: badgesOf(s, entry, m),
});

const renderCard = (s: PluginSummary, entry = false, on: Partial<Parameters<typeof PluginBoxCard>[0]> = {}) =>
  render(
    <PluginBoxCard
      box={sceneBox(s, entry)}
      selected={false}
      messages={m}
      onPointerDown={() => {}}
      onOpen={() => {}}
      onHover={() => {}}
      {...on}
    />,
  );

describe("plugin box badges in the accessible layer (STU-030, STU-011)", () => {
  test("a clean plugin shows Valid and no warnings", () => {
    renderCard(summary({}));
    expect(screen.getByText(m.valid)).toBeTruthy();
    expect(screen.queryByText(m.unpublished)).toBeNull();
    expect(screen.queryByText(m.killed)).toBeNull();
  });

  test("lock, unpublished changes, validation errors, kill switch and entry are all shown", () => {
    renderCard(summary({ lockHolder: "Ada", unpublishedChanges: true, validationErrors: 3n, killed: true }), true);
    expect(screen.getByText(m.lockedBy("Ada"))).toBeTruthy();
    expect(screen.getByText(m.unpublished)).toBeTruthy();
    expect(screen.getByText(m.validationErrors(3))).toBeTruthy();
    expect(screen.getByText(m.killed)).toBeTruthy();
    expect(screen.getByText(m.entryBadge)).toBeTruthy();
  });

  test("Enter and double click open the plugin canvas", () => {
    let opened = 0;
    renderCard(summary({}), false, { onOpen: () => opened++ });
    const box = screen.getByRole("button", { name: m.plugin("Loans", "loans") });
    fireEvent.keyDown(box, { key: "Enter" });
    fireEvent.doubleClick(box);
    expect(opened).toBe(2);
  });

  test("Space selects and the arrow keys nudge by 1, with Shift by 10", () => {
    const picks: boolean[] = [];
    const nudges: { x: number; y: number }[] = [];
    renderCard(summary({}), false, { onSelect: (a) => picks.push(a), onNudge: (d) => nudges.push(d) });
    const box = screen.getByRole("button", { name: m.plugin("Loans", "loans") });
    fireEvent.keyDown(box, { key: " ", shiftKey: true });
    fireEvent.keyDown(box, { key: "ArrowRight" });
    fireEvent.keyDown(box, { key: "ArrowUp", shiftKey: true });
    expect(picks).toEqual([true]);
    expect(nudges).toEqual([
      { x: 1, y: 0 },
      { x: 0, y: -10 },
    ]);
  });
});

// The page against the in-memory upstream, with the canvas host stubbed as the plugin-canvas tests do.
const nav = (route: string) => ({ id: `go-${route}`, action: "navigate", input: { route } });
const plugin = (id: string, pages: string[]) => ({
  kind: "plugin",
  path: `plugins/${id}/plugin.json`,
  json: { kind: "plugin", id, key: id, name: id, pages, entryPage: pages[0] },
});
const page = (id: string, plug: string, steps: object[] = []) => ({
  kind: "page",
  path: `plugins/${plug}/pages/${id}.page.json`,
  json: {
    id,
    key: id,
    route: id,
    title: id,
    kind: "page",
    pageKind: "screen",
    root: { id: `${id}-r`, events: { onPressed: { steps } } },
  },
});
const canvasDoc = {
  kind: "canvas",
  path: "canvas.json",
  json: { kind: "canvas", id: "c1", positions: [], frames: [] },
};
const state = (over: Upstream = {}): Upstream => ({
  summaries: [
    { id: "loans", key: "loans", name: "Loans" },
    { id: "cards", key: "cards", name: "Cards" },
  ],
  pluginDocuments: {
    loans: [plugin("loans", ["home"]), page("home", "loans", [nav("offers")])],
    cards: [plugin("cards", ["offers"]), page("offers", "cards")],
  },
  appDocuments: [canvasDoc],
  ...over,
});
const path = `/o/${orgA}/apps/app1`;

/** A surface that records patches, selection and guides, and answers hits from `hits`. */
function stub() {
  const patches: SceneOp[] = [];
  const selections: string[][] = [];
  const guides: (Guides | null)[] = [];
  const views: number[][] = [];
  const hits = { point: [] as string[], rect: [] as string[] };
  const surface: CanvasSurface = {
    layoutScreens: () => Promise.resolve([]),
    setViewport: (x, y, z) => views.push([x, y, z]),
    pan: () => undefined,
    zoomAbout: () => undefined,
    hitTest: () => hits.point,
    hitTestRect: () => hits.rect,
    setSelection: (ids) => selections.push([...ids]),
    setGuides: (g) => guides.push(g),
    patch: (ops) => patches.push(...ops),
    viewport: { x: 0, y: 0, zoom: 1 },
    dispose: () => undefined,
  };
  const added = () => new Set(patches.flatMap((o) => (o.op === "add" || o.op === "replace" ? [o.item.id] : [])));
  return { patches, selections, guides, views, hits, added, factory: () => surface };
}

const canvas = () => document.querySelector("canvas") as HTMLCanvasElement;

describe("C1 on Plux Canvas (STU-030, STU-040, ADR-0013)", () => {
  test("boxes, badges and the bundled arrow between plugins are scene items; zoom to fit sets the view", async () => {
    const s = stub();
    renderApp(path, state({ surface: s.factory }));
    await waitFor(() => expect(s.added().has("app-box/loans")).toBe(true));
    await waitFor(() => expect([...s.added()].some((id) => id.startsWith("arrow:loans→cards:"))).toBe(true));
    expect(s.added().has("app-box/cards::title")).toBe(true);
    expect(s.views.length).toBeGreaterThan(0);
    // Nothing is drawn in the DOM: the boxes there are the visually hidden accessible layer.
    expect(screen.getByRole("button", { name: /^Loans/ }).closest(".sr-only")).not.toBeNull();
  });

  test("a click on a box selects it in the overlay; a marquee selects every box it meets", async () => {
    const s = stub();
    renderApp(path, state({ surface: s.factory }));
    await waitFor(() => expect(s.added().has("app-box/cards")).toBe(true));
    s.hits.point = ["app-box/loans::title"];
    fireEvent.pointerDown(canvas(), { button: 0, clientX: 10, clientY: 10 });
    fireEvent.pointerUp(canvas(), { clientX: 10, clientY: 10 });
    await waitFor(() => expect(s.selections.at(-1)).toEqual(["app-box/loans"]));

    s.hits.point = [];
    s.hits.rect = ["app-box/loans", "app-box/cards::key"];
    fireEvent.pointerDown(canvas(), { button: 0, clientX: 0, clientY: 0 });
    fireEvent.pointerMove(canvas(), { clientX: 400, clientY: 300 });
    expect(s.guides.at(-1)?.indicator).toBeDefined();
    fireEvent.pointerUp(canvas(), { clientX: 400, clientY: 300 });
    await waitFor(() => expect([...(s.selections.at(-1) ?? [])].sort()).toEqual(["app-box/cards", "app-box/loans"]));
    expect(s.guides.at(-1)).toBeNull();
  });

  test("dragging a box re-routes its arrow and saves its position in canvas.json (STU-032, STU-045)", async () => {
    const s = stub();
    const sent: NonNullable<Upstream["sent"]> = [];
    renderApp(path, state({ surface: s.factory, sent }));
    await waitFor(() => expect([...s.added()].some((id) => id.startsWith("arrow:"))).toBe(true));
    const before = s.patches.length;
    s.hits.point = ["app-box/cards"];
    fireEvent.pointerDown(canvas(), { button: 0, clientX: 10, clientY: 10 });
    fireEvent.pointerMove(canvas(), { clientX: 410, clientY: 210 });
    fireEvent.pointerUp(canvas(), { clientX: 410, clientY: 210 });
    await waitFor(() => expect(sent.some((p) => p.path === "canvas.json")).toBe(true));
    const moves = s.patches.slice(before).flatMap((o) => (o.op === "add" || o.op === "replace" ? [o.item.id] : []));
    expect(moves).toContain("app-box/cards");
    expect(moves.some((id) => id.startsWith("arrow:loans→cards:"))).toBe(true);
  });

  test("the keyboard nudges a focused box, draws its focus ring and announces where it went", async () => {
    const s = stub();
    const sent: NonNullable<Upstream["sent"]> = [];
    renderApp(path, state({ surface: s.factory, sent }));
    const box = await screen.findByRole("button", { name: /Cards/ });
    fireEvent.focus(box);
    await waitFor(() => expect(s.added().has("app-box-focus")).toBe(true));
    fireEvent.keyDown(box, { key: "ArrowDown", shiftKey: true });
    expect(await screen.findByText(/^Cards moved to/)).toBeDefined();
    await waitFor(() => expect(sent.some((p) => p.path === "canvas.json")).toBe(true));
  });

  test("Move to… places a box at exact coordinates", async () => {
    const s = stub();
    const sent: NonNullable<Upstream["sent"]> = [];
    renderApp(path, state({ surface: s.factory, sent }));
    fireEvent.pointerDown(await screen.findByRole("button", { name: /^Loans/ }));
    fireEvent.click(screen.getByRole("button", { name: "Move to…" }));
    fireEvent.change(await screen.findByLabelText("X position"), { target: { value: "500" } });
    fireEvent.change(screen.getByLabelText("Y position"), { target: { value: "40" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(await screen.findByText("Loans moved to 500, 40")).toBeDefined();
    await waitFor(() => expect(JSON.stringify(sent)).toContain('"x":500'));
  });

  test("a lane's menu removes the one link as a single change set (STU-046)", async () => {
    const s = stub();
    const changeSets: NonNullable<Upstream["changeSets"]> = [];
    renderApp(path, state({ surface: s.factory, changeSets }));
    const lane = await screen.findByRole("button", { name: "Link home → offers" });
    fireEvent.focus(lane);
    fireEvent.click(lane);
    fireEvent.click(await screen.findByRole("button", { name: "Remove link" }));
    await waitFor(() => expect(changeSets.length).toBe(1));
  });

  test("a click on a collapsed arrow expands it into lanes in the worker", async () => {
    const s = stub();
    renderApp(path, state({ surface: s.factory }));
    await waitFor(() => expect(s.added().has("arrow:loans→cards:s0")).toBe(true));
    s.hits.point = ["arrow:loans→cards:s0"];
    fireEvent.pointerDown(canvas(), { button: 0, clientX: 5, clientY: 5 });
    fireEvent.pointerUp(canvas(), { clientX: 5, clientY: 5 });
    await waitFor(() => expect([...s.added()].some((id) => id.startsWith("arrow:loans→cards:l0"))).toBe(true));
  });
});
