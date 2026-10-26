// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import type { WidgetDescriptor } from "@plux/schema";
import { Button, Input, LayerTree, Tabs, TypeTag } from "@plux/ui";
import { type DragEvent, type ReactElement, useMemo, useState } from "react";
import type { PlacedScreen } from "../plugin-canvas/scene.ts";
import type { EditFn } from "../plugin-canvas/select/edits.ts";
import type { Selection } from "../plugin-canvas/select/selection.ts";
import type { SelectionStore } from "../plugin-canvas/select/store.ts";
import { itemIdOf, type NodeIndex } from "../plugin-canvas/select/tree.ts";
import type { CanvasSurface } from "../plugin-canvas/surface.ts";
import { catalogue, groupOf, insertGroups, search } from "./catalogue.ts";
import { insertionPoint, insertWidget, modelOf, moveLayer } from "./model.ts";
import { panelsMessages } from "./panels.messages.ts";
import { selectIds, treeNodes } from "./selection-model.ts";

export const WIDGET_DRAG_TYPE = "application/x-plux-widget";

export interface LeftDockProps {
  readonly screens: readonly PlacedScreen[];
  readonly entryPage: string | undefined;
  readonly index: NodeIndex;
  /** Node id to the id of the page document that holds it. */
  readonly screenOf: ReadonlyMap<string, string>;
  readonly selection: Selection;
  readonly store: SelectionStore;
  readonly edit: EditFn;
  readonly canEdit: boolean;
  readonly surface: () => CanvasSurface | null;
  readonly canvas: () => HTMLCanvasElement | null;
  /** Native-slot and custom widget descriptors of the app (WGT-030). */
  readonly nativeWidgets?: readonly WidgetDescriptor[];
}

/** F5: selecting from a panel pans the canvas to the node. */
function reveal(
  surface: CanvasSurface | null,
  canvas: HTMLCanvasElement | null,
  screen: string | undefined,
  id: string,
): void {
  const b = screen === undefined ? undefined : surface?.boundsOf?.(itemIdOf(screen, id));
  if (surface === null || canvas === null || b === undefined) {
    return;
  }
  const z = surface.viewport.zoom;
  surface.setViewport((b.l + b.r) / 2 - canvas.clientWidth / 2 / z, (b.t + b.b) / 2 - canvas.clientHeight / 2 / z, z);
}

/** The left dock of the plugin canvas (STU-061): Insert, Layers, Screens, Selection. Writes go through `edit` and stop when read-only. */
export function LeftDock(props: LeftDockProps): ReactElement {
  const { i18n } = useLingui();
  const m = panelsMessages(i18n);
  const { index, screenOf, selection, store, edit, canEdit } = props;
  const [query, setQuery] = useState("");
  const [note, setNote] = useState<string | null>(null);
  const model = useMemo(() => modelOf(props.screens.map((s) => s.root)), [props.screens]);
  const all = useMemo(() => catalogue(props.nativeWidgets), [props.nativeWidgets]);
  const found = useMemo(() => search(all, query), [all, query]);
  const docOf = (id: string) => screenOf.get(id);

  const select = (id: string, toggle: boolean) => {
    store.set(selectIds(index, store.get(), id, toggle));
    reveal(props.surface(), props.canvas(), docOf(id), id);
  };
  const insert = (d: WidgetDescriptor) => {
    const place = insertionPoint(model, index, store.get(), props.screens[0]?.root?.id);
    if (place === null) {
      setNote(m.cannotHold);
      return;
    }
    const id = insertWidget(d, place, edit, docOf, canEdit);
    setNote(null);
    if (id !== null) {
      store.set(selectIds(index, store.get(), id, false));
    }
  };
  const startDrag = (e: DragEvent, d: WidgetDescriptor) => {
    e.dataTransfer.setData(WIDGET_DRAG_TYPE, d.type);
    e.dataTransfer.effectAllowed = "copy";
  };

  const nodes = useMemo(() => treeNodes(props.screens, index, m.pluginRoot), [props.screens, index, m.pluginRoot]);
  const rtl = i18n.locale.startsWith("ar");
  const types = selection.ids.map((id) => index.type.get(id) ?? id);
  const summary = [...new Set(types)].map((t) => `${types.filter((x) => x === t).length} ${t}`).join(", ");

  const tabs = [
    {
      value: "insert",
      label: m.insertTab,
      panel: (
        <div className="flex flex-col gap-3">
          <Input
            aria-label={m.search}
            placeholder={m.search}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
          {!canEdit && <p className="text-body text-secondary">{m.readOnly}</p>}
          {note !== null && (
            <p role="status" className="text-body text-secondary">
              {note}
            </p>
          )}
          {found.length === 0 && <p className="text-body text-secondary">{m.noMatches}</p>}
          {insertGroups.map((g) => {
            const list = found.filter((d) => groupOf(d) === g);
            return list.length === 0 ? null : (
              <section key={g} aria-label={m[g]} className="flex flex-col gap-1">
                <h3 className="text-label font-medium text-secondary">{m[g]}</h3>
                <ul className="flex flex-col gap-0.5">
                  {list.map((d) => (
                    <li key={d.type} draggable={canEdit} onDragStart={(e) => startDrag(e, d)}>
                      <Button
                        disabled={!canEdit}
                        aria-label={m.insert(d.type)}
                        title={d.description}
                        onClick={() => insert(d)}
                      >
                        {d.type}
                      </Button>
                    </li>
                  ))}
                </ul>
              </section>
            );
          })}
        </div>
      ),
    },
    {
      value: "layers",
      label: m.layersTab,
      panel: (
        <LayerTree
          nodes={nodes}
          rootId="plugin"
          label={m.layersLabel}
          dir={rtl ? "rtl" : "ltr"}
          selected={selection.ids}
          onSelect={select}
          onDrop={canEdit ? (d, t, half) => moveLayer(model, index, d, t, half, edit, docOf, canEdit) : undefined}
        />
      ),
    },
    {
      value: "screens",
      label: m.screensTab,
      panel: (
        <ul aria-label={m.screensLabel} className="flex flex-col gap-0.5">
          {props.screens.map((s) => (
            <li key={s.id}>
              <button
                type="button"
                className="flex w-full items-center gap-2 rounded-md px-2 py-1 text-start text-body hover:bg-hover"
                onClick={() => s.root !== undefined && select(s.root.id, false)}
              >
                <span className="truncate">{s.title}</span>
                {s.id === props.entryPage && <TypeTag>{m.entryScreen}</TypeTag>}
              </button>
            </li>
          ))}
        </ul>
      ),
    },
    {
      value: "selection",
      label: m.selectionTab,
      panel:
        selection.ids.length === 0 ? (
          <p className="text-body text-secondary">{m.noSelection}</p>
        ) : (
          <div className="flex flex-col gap-2">
            {selection.ids.length > 1 && (
              <p className="text-body">
                {m.selectedCount(selection.ids.length)} ({summary})
              </p>
            )}
            <ul aria-label={m.selectionLabel} className="flex flex-col gap-0.5">
              {selection.ids.map((id) => (
                <li key={id} className="flex items-center gap-2 text-body">
                  <TypeTag>{index.type.get(id) ?? "?"}</TypeTag>
                  <code className="truncate text-secondary">{id}</code>
                </li>
              ))}
            </ul>
          </div>
        ),
    },
  ];

  return (
    <aside
      aria-label={m.dock}
      className="absolute start-3 top-3 bottom-16 z-10 flex w-72 flex-col gap-2 overflow-y-auto rounded-lg bg-raised p-3 shadow-e1"
    >
      <Tabs defaultValue="insert" tabs={tabs} />
    </aside>
  );
}
