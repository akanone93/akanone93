// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import type { WidgetDescriptor } from "@plux/schema";
import { Icon, type IconName, layerFolders } from "@plux/ui";
import { InputGroup, InputGroupAddon, InputGroupInput } from "@plux/ui/components/input-group";
import { type ComponentProps, type ReactElement, useCallback, useContext, useMemo, useRef, useState } from "react";
import type { DesignerDocument } from "../designers/use-document.ts";
import type { PlacedScreen } from "../plugin-canvas/scene.ts";
import type { EditFn } from "../plugin-canvas/select/edits.ts";
import type { Selection } from "../plugin-canvas/select/selection.ts";
import type { SelectionStore } from "../plugin-canvas/select/store.ts";
import { itemIdOf, type NodeIndex } from "../plugin-canvas/select/tree.ts";
import type { CanvasSurface } from "../plugin-canvas/surface.ts";
import { assetsMessages } from "./assets.messages.ts";
import { AssetsTab } from "./assets-tab.tsx";
import { catalogue, search } from "./catalogue.ts";
import { DockTabs, type Pane, useStoredFlag } from "./dock-tabs.tsx";
import { InsertPane } from "./insert-pane.tsx";
import { InstancePreviewContext } from "./instance-card.tsx";
import { LayersPane, LayersToolbar } from "./layers-pane.tsx";
import { LocalePreviewContext } from "./locale-tools.tsx";
import { insertionPoint, insertWidget, modelOf, moveLayer } from "./model.ts";
import { panelsMessages } from "./panels.messages.ts";
import { selectIds, treeNodes } from "./selection-model.ts";
import { metaRecipe } from "./studio-meta.ts";
import { DataPane } from "./tab-panels.tsx";
import { tabMessages } from "./tabs.messages.ts";

export { WIDGET_DRAG_TYPE } from "./insert-pane.tsx";

export interface LeftDockProps {
  /** The plugin's name, under the editor's name in the dock's header. */
  readonly title?: string | undefined;
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

/** The room above the canvas's content that the top bar and a screen's name take, in CSS pixels. */
const REVEAL_TOP = 120;

/**
 * F5: selecting from a panel pans the canvas to the node: centred, or — for a node taller than the room, a whole
 * screen — with its top (and a screen's name above it) just below the top bar, where it can be grabbed.
 */
export function reveal(
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
  const tall = (b.b - b.t) * z > canvas.clientHeight - 2 * REVEAL_TOP;
  const y = tall ? b.t - REVEAL_TOP / z : (b.t + b.b) / 2 - canvas.clientHeight / 2 / z;
  surface.setViewport((b.l + b.r) / 2 - canvas.clientWidth / 2 / z, y, z);
}

/**
 * The left dock of the plugin canvas (STU-061): the editor's rail and header over four panes — Insert, Layers (with the
 * screens and the selection), Assets and Data (state and data sources). Writes go through `edit` and stop when
 * read-only.
 */
export function LeftDock(props: LeftDockProps): ReactElement {
  const { i18n } = useLingui();
  const m = panelsMessages(i18n);
  const tm = tabMessages(i18n);
  const { index, screenOf, selection, store, edit, canEdit } = props;
  const [query, setQuery] = useState("");
  const [note, setNote] = useState<string | null>(null);
  const [filter, setFilter] = useState("");
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

  // The Insert pane redraws only when its own inputs change: its tiles animate, and measuring them on every edit or
  // selection of the canvas slowed the whole editor.
  const insertRef = useRef(insert);
  insertRef.current = insert;
  const onInsert = useCallback((d: WidgetDescriptor) => insertRef.current(d), []);

  const entry = useMemo(() => ({ page: props.entryPage, label: m.entryScreen }), [props.entryPage, m.entryScreen]);
  // A translated text's row shows its message in the previewed locale (PC-32).
  const textOf = useContext(LocalePreviewContext)?.textOf;
  const instances = useContext(InstancePreviewContext);
  const nodes = useMemo(
    () => treeNodes(props.screens, index, m.pluginRoot, entry, textOf, (id) => instances?.markOf(id)?.name),
    [props.screens, index, m.pluginRoot, entry, textOf, instances],
  );
  // A locked layer stays where it is: it neither moves nor takes another layer into it.
  const dropLayer = (dragged: string, target: string, half: "before" | "after") => {
    if (nodes[dragged]?.locked !== true) {
      moveLayer(model, index, dragged, target, half, edit, docOf, canEdit);
    }
  };

  // The first screen opens at first and the others stay folded (MR-3); a selection unfolds its own path, and the
  // person folds and unfolds from there.
  const roots = useMemo(() => (nodes["plugin"]?.children ?? []).slice(0, 1), [nodes]);
  const [expanded, setExpanded] = useState<readonly string[] | null>(null);
  const open = expanded ?? roots;
  const rtl = i18n.locale.startsWith("ar");
  const [folded, setFolded] = useStoredFlag("plux.dock.start.folded", false);

  const panes: readonly Pane[] = [
    {
      value: "insert",
      label: m.insertTab,
      icon: "plus",
      toolbar: (
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
      ),
      panel: <InsertPane found={found} all={all} canEdit={canEdit} note={note} onInsert={onInsert} />,
    },
    {
      value: "layers",
      label: m.layersTab,
      icon: "tree-structure",
      meta: String(props.screens.length),
      toolbar: (
        <LayersToolbar
          filter={filter}
          onFilter={setFilter}
          onExpandAll={() => setExpanded(layerFolders(nodes, "plugin"))}
          onCollapseAll={() => setExpanded([])}
        />
      ),
      panel: (
        <LayersPane
          nodes={nodes}
          selected={selection.ids}
          filter={filter}
          expanded={open}
          dir={rtl ? "rtl" : "ltr"}
          onExpanded={setExpanded}
          onSelect={select}
          onReveal={(id) => reveal(props.surface(), props.canvas(), docOf(id), id)}
          onClear={() => store.set({ ids: [], anchor: null, scope: null })}
          onDrop={canEdit ? (d, t, half) => dropLayer(d, t, half) : undefined}
          onMeta={
            canEdit
              ? (id, change) => {
                  const doc = docOf(id);
                  if (doc !== undefined) {
                    edit(doc, metaRecipe(id, change), { group: `layer:${id}` });
                  }
                }
              : undefined
          }
        />
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
      value: "data",
      label: tm.dataTab,
      icon: "database",
      panel: <DataPane documents={props.documents} onOpenState={props.onOpenState} onOpenData={props.onOpenData} />,
    },
  ];

  const widgets = Math.max(0, index.parent.size - roots.length);
  return (
    <aside
      aria-label={m.dock}
      className="plugin-dock plugin-dock-start canvas-panel canvas-side glass"
      data-folded={folded || undefined}
    >
      <DockTabs
        panes={panes}
        defaultValue="insert"
        mark="puzzle-piece"
        folded={folded}
        onFold={setFolded}
        foldLabel={m.foldDock}
        unfoldLabel={m.unfoldDock}
        head={
          <div className="flex min-w-0 flex-col">
            <span className="dock-head-kicker">{m.editorTitle}</span>
            <h2 className="dock-head-title">{props.title ?? m.pluginRoot}</h2>
            <span className="dock-head-meta">{m.editorSummary(props.screens.length, widgets)}</span>
          </div>
        }
      />
    </aside>
  );
}
