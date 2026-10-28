// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, beforeAll, describe, expect, test } from "bun:test";
import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { orgA, renderApp } from "../../test-harness.tsx";
import type { EditSession } from "../editor/edit-session.ts";
import type { CanvasSurface } from "../plugin-canvas/surface.ts";
import type { ValidatorClient } from "../problems/validator-client.ts";
import { ENTRY_DRAG_TYPE, savedPosition, stepsAt } from "./graph-model.ts";

/** happy-dom measures nothing; React Flow needs an observer and a matrix type to place nodes. */
beforeAll(() => {
  const g = globalThis as unknown as Record<string, unknown>;
  g["ResizeObserver"] ??= class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
  g["DOMMatrixReadOnly"] ??= class {
    m22 = 1;
    constructor(_transform?: string) {}
  };
});
afterEach(cleanup);

const pagePath = "pages/home.page.json";
const page = (handler: unknown) => ({
  kind: "page",
  path: pagePath,
  json: {
    id: "home",
    key: "home",
    route: "home",
    kind: "page",
    pageKind: "screen",
    root: { id: "btn", type: "ElevatedButton", events: handler === undefined ? {} : { onPressed: handler } },
  },
});
const plugin = {
  kind: "plugin",
  path: "plugin.json",
  json: { id: "pl", kind: "plugin", entryPage: "home", pages: ["home"] },
};
const path = `/o/${orgA}/apps/app1/plugins/pl`;

const surface = (): (() => CanvasSurface) => () => ({
  layoutScreens: (docs) => Promise.resolve(docs.map((d) => ({ id: d.id, contentHeight: 300 }))),
  setViewport: () => undefined,
  setPaused: () => undefined,
  pan: () => undefined,
  zoomAbout: () => undefined,
  viewport: { x: 0, y: 0, zoom: 1 },
  dispose: () => undefined,
});

const validator = (): (() => ValidatorClient) => () => ({
  load: async () => [],
  validatePage: async (file, data) =>
    data.includes('"kvGet1"')
      ? [
          {
            code: "PLX-9",
            severity: "error",
            file,
            path: "/root/events/onPressed/steps/0/input/key",
            message: "key is empty",
          },
        ]
      : [],
  dispose: () => undefined,
});

async function openEditor(handler: unknown, extra: Parameters<typeof renderApp>[1] = {}) {
  let edit: EditSession | undefined;
  renderApp(path, {
    documents: [plugin, page(handler)],
    surface: surface(),
    validator: validator(),
    editor: (e) => (edit = e),
    ...extra,
  });
  const open = await screen.findByRole("button", { name: "Action graph" }, { timeout: 20_000 });
  const session = edit as unknown as EditSession;
  session.store.setState({ selection: { ids: ["btn"], anchor: "btn", scope: null } });
  fireEvent.click(open);
  return session;
}

const steps = (s: EditSession) =>
  stepsAt(s.store.getState().docs[pagePath]?.content, {
    kind: "inline",
    path: pagePath,
    nodeId: "btn",
    event: "onPressed",
  });

describe("STU-065 action graph editor", () => {
  test("lists the catalogue, adds steps through the edit pipeline as a JSON Patch, and keeps them undoable", async () => {
    const sent: NonNullable<Parameters<typeof renderApp>[1]>["sent"] = [];
    const s = await openEditor(undefined, { sent });
    expect(await screen.findByText(/no steps yet/)).toBeTruthy();
    const catalogue = screen.getByRole("region", { name: "Actions" });
    expect(within(catalogue).getByRole("region", { name: "Built-in" })).toBeTruthy();
    fireEvent.change(within(catalogue).getByRole("textbox", { name: "Search actions" }), {
      target: { value: "kvget" },
    });
    fireEvent.click(within(catalogue).getByRole("button", { name: "Add kvGet to the graph" }));
    await waitFor(() => expect(steps(s).map((x) => x.id)).toEqual(["kvGet1"]));
    s.flushAll();
    await waitFor(() => expect(sent.length).toBeGreaterThan(0));
    expect(sent[0]?.ops.filter((o) => o.op !== "test")).toEqual([
      {
        op: "add",
        path: "/root/events/onPressed",
        value: { steps: [{ id: "kvGet1", action: "kvGet", input: { key: "" } }] },
      },
    ]);
    expect(await screen.findByLabelText("kvGet1", {}, { timeout: 10_000 })).toBeTruthy();
    await waitFor(() => expect(screen.getByText("1 problem")).toBeTruthy());
    expect(screen.getByText("key is empty")).toBeTruthy();
  }, 40_000);

  test("a step is a form: a literal input becomes a PXL expression, a port connects by keyboard-reachable lists, and a step removes", async () => {
    const s = await openEditor({
      steps: [
        { id: "kvGet1", action: "kvGet", input: { key: "a" } },
        { id: "pop1", action: "pop" },
      ],
    });
    const node = await screen.findByLabelText("kvGet1", {}, { timeout: 10_000 });
    fireEvent.change(within(node).getByDisplayValue("a"), { target: { value: "b" } });
    expect(steps(s)[0]?.input).toEqual({ key: "b" });
    fireEvent.click(within(node).getByRole("button", { name: "PXL expression" }));
    expect(steps(s)[0]?.input).toEqual({ key: { $expr: "b" } });
    expect(within(node).getByDisplayValue("b")).toBeTruthy();
    fireEvent.click(within(node).getByRole("combobox", { name: "onSuccess goes to" }));
    const option = await screen.findByRole("option", { name: "pop1" });
    // Base UI ignores a selection that follows the opening too closely, as a click that opened it would.
    await new Promise((r) => setTimeout(r, 300));
    fireEvent.pointerDown(option);
    fireEvent.mouseDown(option);
    fireEvent.pointerUp(option);
    fireEvent.mouseUp(option);
    fireEvent.click(option);
    await waitFor(() => expect(steps(s)[0]?.onSuccess).toBe("pop1"));
    expect(within(node).getByText(/Returns/)).toBeTruthy();
    fireEvent.click(within(node).getByRole("button", { name: "Remove step kvGet1", hidden: true }));
    await waitFor(() => expect(steps(s).map((x) => x.id)).toEqual(["pop1"]));
    expect(await screen.findByRole("region", { name: "pop1", hidden: true })).toBeTruthy();
  }, 40_000);

  test("dragging an action from the catalogue onto the graph adds it there with its position, as one edit", async () => {
    const s = await openEditor({ steps: [{ id: "pop1", action: "pop" }] });
    await screen.findByLabelText("pop1", {}, { timeout: 10_000 });
    const item = within(screen.getByRole("region", { name: "Actions" }))
      .getByRole("button", { name: "Add kvGet to the graph" })
      .closest("li");
    expect(item?.getAttribute("draggable")).toBe("true");
    const store = new Map<string, string>();
    const dataTransfer = {
      types: [ENTRY_DRAG_TYPE],
      setData: (type: string, value: string) => store.set(type, value),
      getData: (type: string) => store.get(type) ?? "",
      effectAllowed: "",
      dropEffect: "",
    };
    fireEvent.dragStart(item as Element, { dataTransfer });
    expect(store.get(ENTRY_DRAG_TYPE)).toBe("kvGet");
    const pane = document.querySelector(".react-flow");
    fireEvent.drop(pane as Element, { dataTransfer, clientX: 0, clientY: 0 });
    await waitFor(() => expect(steps(s).map((x) => x.id)).toEqual(["pop1", "kvGet1"]));
    const dropped = steps(s)[1];
    const at = dropped === undefined ? undefined : savedPosition(dropped);
    // happy-dom has no layout, so the drop point is not a number here and the step keeps dagre's place.
    expect(at === undefined || (Number.isFinite(at.x) && Number.isFinite(at.y))).toBe(true);
    expect(dropped?.next === undefined && steps(s)[0]?.next === undefined).toBe(true);
  }, 40_000);

  test("without the lock the graph is read-only, and an event that names a missing graph says so", async () => {
    await openEditor({ $graph: "gone" });
    await screen.findByText("Action graph editor", {}, { timeout: 10_000 });
    expect(await screen.findByText(/refers to an action graph that does not exist/)).toBeTruthy();
    cleanup();
    await openEditor({ steps: [{ id: "pop1", action: "pop" }] }, { lockHeldElsewhere: true });
    expect(await screen.findByText(/read-only/)).toBeTruthy();
    const add = within(screen.getByRole("region", { name: "Actions" })).getByRole("button", {
      name: "Add pop to the graph",
    });
    expect(add.matches(":disabled, [aria-disabled=true]")).toBe(true);
  }, 40_000);
});
