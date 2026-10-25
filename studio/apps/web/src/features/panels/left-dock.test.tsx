// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, mock, test } from "bun:test";
import { createStudioI18n } from "@plux/i18n";
import { widget } from "@plux/schema";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { I18nProvider } from "../../i18n-provider.tsx";
import { loadSourceCatalogue } from "../../test-harness.tsx";
import type { Node } from "../plugin-canvas/plugin-model.ts";
import type { PlacedScreen } from "../plugin-canvas/scene.ts";
import type { EditFn } from "../plugin-canvas/select/edits.ts";
import { emptySelection, type Selection } from "../plugin-canvas/select/selection.ts";
import { indexNodes } from "../plugin-canvas/select/tree.ts";
import { buildNode, catalogue, search } from "./catalogue.ts";
import { LeftDock } from "./left-dock.tsx";
import { dropTarget, insertionPoint, insertRecipe, insertWidget, layerDrop, modelOf, moveLayer } from "./model.ts";
import { insertDropHandlers } from "./use-insert-drop.ts";

afterEach(cleanup);

const root: Node = {
  id: "r",
  type: "Column",
  children: [
    { id: "row", type: "Row", children: [{ id: "t1", type: "Text" }] },
    { id: "t2", type: "Text" },
  ],
};
const screens: PlacedScreen[] = [{ id: "pg", title: "Home", at: { x: 0, y: 0 }, root }];
const index = indexNodes([root]);
const model = modelOf([root]);
const screenOf = new Map([...index.parent.keys()].map((id) => [id, "pg"]));
const uuid7 = /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;

function dock(canEdit: boolean, selection: Selection, set: (s: Selection) => void, edit: EditFn) {
  const studio = createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false });
  let current = selection;
  return render(
    <I18nProvider studio={studio}>
      <LeftDock
        screens={screens}
        entryPage="pg"
        index={index}
        screenOf={screenOf}
        selection={selection}
        store={{
          get: () => current,
          set: (s) => {
            current = s;
            set(s);
          },
        }}
        edit={edit}
        canEdit={canEdit}
        surface={() => null}
        canvas={() => null}
      />
    </I18nProvider>,
  );
}

describe("STU-061 left panels", () => {
  test("Layers tree selects into the shared selection and shows the shared selection", async () => {
    const set = mock((_: Selection) => undefined);
    dock(true, { ids: ["t1"], anchor: "t1", scope: "row" }, set, mock());
    fireEvent.click(await screen.findByRole("tab", { name: "Layers" }));
    const rows = await screen.findAllByRole("treeitem");
    const t1 = rows.find((r) => r.getAttribute("data-item-id") === "t1");
    expect(t1?.getAttribute("aria-selected")).toBe("true");
    const t2 = rows.find((r) => r.getAttribute("data-item-id") === "t2");
    if (t2 === undefined) {
      throw new Error("row t2 missing");
    }
    fireEvent.click(t2);
    expect(set.mock.calls[0]?.[0]).toEqual({ ids: ["t2"], anchor: "t2", scope: "r" });
  });

  test("Selection tab summarises a multi-selection", async () => {
    dock(true, { ids: ["t1", "t2"], anchor: "t2", scope: "r" }, mock(), mock());
    fireEvent.click(await screen.findByRole("tab", { name: "Selection" }));
    await waitFor(() => expect(screen.getByText(/2 nodes selected/)).toBeTruthy());
    expect(screen.getByText("t1")).toBeTruthy();
  });

  test("insert builds a valid node from the descriptor with a fresh UUIDv7", () => {
    const d = widget("Text");
    if (d === undefined) {
      throw new Error("no Text descriptor");
    }
    const a = buildNode(d);
    expect(a.id).toMatch(uuid7);
    expect(a.type).toBe("Text");
    expect(buildNode(d).id).not.toBe(a.id);
    const draft = { id: "r", type: "Column", children: [{ id: "x" }] };
    insertRecipe(a, { parent: "r", index: 0, slot: null })(draft);
    expect(draft.children[0]).toBe(a);
    expect(search(catalogue(), "text").some((x) => x.type === "Text")).toBe(true);
  });

  test("keyboard insert goes to the selection or its nearest container, through edit", async () => {
    const edit = mock<EditFn>();
    const place = insertionPoint(model, index, { ids: ["t1"], anchor: "t1", scope: "row" }, "r");
    expect(place).toEqual({ parent: "row", index: 1, slot: null });
    const set = mock();
    dock(true, { ids: ["row"], anchor: "row", scope: "r" }, set, edit);
    fireEvent.click(await screen.findByRole("button", { name: "Insert Text" }));
    expect(edit).toHaveBeenCalledTimes(1);
    expect(edit.mock.calls[0]?.[0]).toBe("pg");
    expect(set.mock.calls[0]?.[0]).toMatchObject({ ids: [expect.any(String)] });
  });

  test("drop target resolves with the S2.5 hit-test and indicator, and a drop inserts", () => {
    const boxes: Record<string, { l: number; t: number; r: number; b: number }> = {
      r: { l: 0, t: 0, r: 100, b: 200 },
      row: { l: 0, t: 0, r: 100, b: 40 },
      t1: { l: 0, t: 0, r: 50, b: 40 },
      t2: { l: 0, t: 100, r: 100, b: 140 },
    };
    const env = {
      index,
      edit: mock<EditFn>(),
      docOf: () => "pg",
      boxOf: (id: string) => boxes[id],
      typeOf: (id: string) => index.type.get(id),
    };
    expect(dropTarget(env, 80, 20)).toMatchObject({ parent: "row", index: 1 });
    expect(dropTarget(env, 50, 180)?.parent).toBe("r");
    const edit = mock<EditFn>();
    const surface = {
      viewport: { x: 0, y: 0, zoom: 1 },
      boundsOf: (item: string) => boxes[item.split("/")[1] ?? ""],
      setGuides: mock(),
    };
    const canvas = { getBoundingClientRect: () => ({ left: 0, top: 0 }) };
    const h = insertDropHandlers({
      surface: () => surface as never,
      canvas: () => canvas as never,
      index,
      model,
      edit,
      screenOf,
      canEdit: true,
      descriptors: catalogue(),
    });
    const ev = {
      clientX: 80,
      clientY: 20,
      preventDefault: mock(),
      dataTransfer: { types: ["application/x-plux-widget"], getData: () => "Text" },
    };
    h.onDragOver(ev as never);
    expect(surface.setGuides).toHaveBeenCalled();
    h.onDrop(ev as never);
    expect(edit).toHaveBeenCalledTimes(1);
  });

  test("layer reorder uses reparent indices after removal", () => {
    expect(layerDrop(model, index, "t2", "t1", "before")).toEqual({ parent: "row", index: 0 });
    expect(layerDrop(model, index, "t1", "t2", "after")).toEqual({ parent: "r", index: 2 });
  });

  test("read-only: Insert is disabled and no write reaches edit", async () => {
    const edit = mock<EditFn>();
    dock(false, emptySelection, mock(), edit);
    const button = await screen.findByRole("button", { name: "Insert Text" });
    expect(button.hasAttribute("disabled") || button.getAttribute("aria-disabled") === "true").toBe(true);
    fireEvent.click(button);
    const d = widget("Text");
    if (d === undefined) {
      throw new Error("no Text descriptor");
    }
    expect(insertWidget(d, { parent: "r", index: 0, slot: null }, edit, () => "pg", false)).toBeNull();
    expect(moveLayer(model, index, "t2", "t1", "before", edit, () => "pg", false)).toBe(false);
    expect(edit).not.toHaveBeenCalled();
  });
});
