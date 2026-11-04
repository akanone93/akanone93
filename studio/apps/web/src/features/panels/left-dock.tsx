// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import type { WidgetDescriptor } from "@plux/schema";
import { Icon, type IconName, LayerTree, TypeTag } from "@plux/ui";
import { InputGroup, InputGroupAddon, InputGroupInput } from "@plux/ui/components/input-group";
import { type ComponentProps, type DragEvent, type ReactElement, useMemo, useState } from "react";
import type { DesignerDocument } from "../designers/use-document.ts";
import type { PlacedScreen } from "../plugin-canvas/scene.ts";
import type { EditFn } from "../plugin-canvas/select/edits.ts";
import type { Selection } from "../plugin-canvas/select/selection.ts";
import type { SelectionStore } from "../plugin-canvas/select/store.ts";
import { itemIdOf, type NodeIndex } from "../plugin-canvas/select/tree.ts";
import type { CanvasSurface } from "../plugin-canvas/surface.ts";
import { assetsMessages } from "./assets.messages.ts";
import { AssetsTab } from "./assets-tab.tsx";
import {
  catalogue,
  groupOf,
  insertGroups,
  kindIcon,
  kindOf,
  search,
  type WidgetKind,
  widgetKinds,
} from "./catalogue.ts";
import { DockTabs, type Pane } from "./dock-tabs.tsx";
import { insertionPoint, insertWidget, modelOf, moveLayer } from "./model.ts";
import { panelsMessages } from "./panels.messages.ts";
import { selectIds, treeNodes } from "./selection-model.ts";
import { DataTab, StateTab } from "./tab-panels.tsx";
import { tabMessages } from "./tabs.messages.ts";

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
  /** The plugin's documents, for the State and Data summaries. */
  readonly documents?: readonly DesignerDocument[] | undefined;
  /** Open the state designer (V5) and the data source designer (V6). */
  readonly onOpenState?: (() => void) | undefined;
  readonly onOpenData?: (() => void) | undefined;
  /** The Assets tab's client, app and editor session; the tab shows only when given. */
  readonly assets?: ComponentProps<typeof AssetsTab> | undefined;
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

/** The left dock of the plugin canvas (STU-061): Insert, Layers, Screens, State, Data, Selection. Writes go through `edit` and stop when read-only. */
export function LeftDock(props: LeftDockProps): ReactElement {
  const { i18n } = useLingui();
  const m = panelsMessages(i18n);
  const tm = tabMessages(i18n);
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

  const [kind, setKind] = useState<WidgetKind | "all">("all");
  const kindLabel: Record<WidgetKind, string> = {
    layout: m.kindLayout,
    content: m.kindContent,
    input: m.kindInput,
    interaction: m.kindInteraction,
    navigation: m.kindNavigation,
    motion: m.kindMotion,
    platform: m.kindPlatform,
    more: m.kindMore,
  };
  const basics = found.filter((d) => groupOf(d) === "layer1");
  const sections = [
    ...widgetKinds.map((k) => ({
      key: k as string,
      label: kindLabel[k],
      icon: kindIcon[k] as IconName,
      list: kind === "all" || kind === k ? basics.filter((d) => kindOf(d) === k) : [],
    })),
    ...insertGroups
      .filter((g) => g !== "layer1")
      .map((g) => ({
        key: g as string,
        label: m[g],
        icon: (g === "layer2" ? "component" : "puzzle-piece") as IconName,
        list: kind === "all" ? found.filter((d) => groupOf(d) === g) : [],
      })),
  ].filter((x) => x.list.length > 0);
  const tile = (d: WidgetDescriptor, icon: IconName, kind: string) => (
    <li key={d.type} draggable={canEdit} onDragStart={(e) => startDrag(e, d)}>
      <button
        type="button"
        className="gallery-tile"
        disabled={!canEdit}
        aria-label={m.insert(d.type)}
        title={d.description}
        onClick={() => insert(d)}
      >
        <span className="gallery-glyph" data-kind={kind} aria-hidden="true">
          <Icon name={icon} />
        </span>
        <span className="gallery-name">{d.type}</span>
      </button>
    </li>
  );

  const panes: readonly Pane[] = [
    {
      value: "insert",
      label: m.insertTab,
      icon: "plus",
      meta: String(found.length),
      panel: (
        <div className="flex flex-col gap-3">
          <InputGroup>
            <InputGroupAddon>
              <Icon name="magnifying-glass" />
            </InputGroupAddon>
            <InputGroupInput
              aria-label={m.search}
              placeholder={m.search}
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />
          </InputGroup>
          {/* biome-ignore lint/a11y/useSemanticElements: a labelled group of filter toggles, not a form's fieldset */}
          <div role="group" aria-label={m.kindAll} className="gallery-kinds">
            {(["all", ...widgetKinds] as const).map((k) => (
              <button
                key={k}
                type="button"
                className="gallery-kind"
                aria-pressed={kind === k}
                onClick={() => setKind(k)}
              >
                {k === "all" ? m.kindAll : kindLabel[k]}
              </button>
            ))}
          </div>
          {!canEdit && <p className="text-xs text-muted-foreground">{m.readOnly}</p>}
          {note !== null && (
            <p role="status" className="text-xs text-muted-foreground">
              {note}
            </p>
          )}
          {sections.length === 0 && <p className="text-xs text-muted-foreground">{m.noMatches}</p>}
          {sections.map((x) => (
            <section key={x.key} aria-label={x.label} className="flex flex-col gap-2">
              <h3 className="gallery-heading">
                <Icon name={x.icon} size="pill" />
                <span className="flex-1">{x.label}</span>
                <span className="gallery-count">{x.list.length}</span>
              </h3>
              <ul className="gallery-grid">{x.list.map((d) => tile(d, x.icon, x.key))}</ul>
            </section>
          ))}
        </div>
      ),
    },
    {
      value: "layers",
      label: m.layersTab,
      icon: "tree-structure",
      meta: String(props.screens.length),
      panel: (
        <div className="plugin-layers">
          <LayerTree
            nodes={nodes}
            rootId="plugin"
            label={m.layersLabel}
            dir={rtl ? "rtl" : "ltr"}
            selected={selection.ids}
            onSelect={select}
            onDrop={canEdit ? (d, t, half) => moveLayer(model, index, d, t, half, edit, docOf, canEdit) : undefined}
          />
        </div>
      ),
    },
    {
      value: "screens",
      label: m.screensTab,
      icon: "device-mobile",
      meta: String(props.screens.length),
      panel: (
        <ul aria-label={m.screensLabel} className="flex flex-col gap-1">
          {props.screens.map((s) => (
            <li key={s.id}>
              <button
                type="button"
                className="screen-row"
                data-current={selection.ids.some((id) => screenOf.get(id) === s.id) ? "" : undefined}
                onClick={() => s.root !== undefined && select(s.root.id, false)}
              >
                <span className="screen-glyph" aria-hidden="true" />
                <span className="min-w-0 flex-1 truncate">{s.title}</span>
                {s.id === props.entryPage && <TypeTag>{m.entryScreen}</TypeTag>}
              </button>
            </li>
          ))}
        </ul>
      ),
    },
    ...(props.assets === undefined
      ? []
      : [
          {
            value: "assets",
            label: assetsMessages(i18n).tab,
            icon: "image" as IconName,
            panel: <AssetsTab {...props.assets} />,
          },
        ]),
    {
      value: "state",
      label: tm.stateTab,
      icon: "braces",
      panel: <StateTab documents={props.documents} onOpen={props.onOpenState} />,
    },
    {
      value: "data",
      label: tm.dataTab,
      icon: "database",
      panel: <DataTab documents={props.documents} onOpen={props.onOpenData} />,
    },
    {
      value: "selection",
      label: m.selectionTab,
      icon: "cursor",
      meta: String(selection.ids.length),
      panel:
        selection.ids.length === 0 ? (
          <p className="text-xs text-muted-foreground">{m.noSelection}</p>
        ) : (
          <div className="flex flex-col gap-2">
            {selection.ids.length > 1 && (
              <p className="text-sm">
                {m.selectedCount(selection.ids.length)} ({summary})
              </p>
            )}
            <ul aria-label={m.selectionLabel} className="flex flex-col gap-1">
              {selection.ids.map((id) => (
                <li key={id} className="screen-row">
                  <TypeTag>{index.type.get(id) ?? "?"}</TypeTag>
                  <code className="truncate text-xs text-muted-foreground">{id}</code>
                </li>
              ))}
            </ul>
          </div>
        ),
    },
  ];

  return (
    <aside aria-label={m.dock} className="plugin-dock plugin-dock-start canvas-panel canvas-side glass">
      <DockTabs panes={panes} defaultValue="insert" />
    </aside>
  );
}
