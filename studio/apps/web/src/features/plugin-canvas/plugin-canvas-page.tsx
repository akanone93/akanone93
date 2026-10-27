// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { frameId, overflowId } from "@plux/canvas";
import { limitDefault } from "@plux/schema";
import { useNavigate, useParams, useRouteContext, useSearch } from "@tanstack/react-router";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { createUndoFocus, undoCommands, useScopedCommands } from "../../lib/commands/index.ts";
import type { RouterContext } from "../../router.tsx";
import { ActionGraphHost } from "../action-graph/index.ts";
import { DesignersHost } from "../designers/designers-host.tsx";
import { tabEditorSession } from "../editor/document-transport.ts";
import type { EditSession } from "../editor/edit-session.ts";
import { EditorProvider, useCanEdit, useEditor, useEditSession } from "../editor/editor-provider.tsx";
import { UndoManager } from "../editor/undo-stack.ts";
import { JsonEditorHost } from "../json-editor/index.ts";
import { L10nSessionsProvider } from "../localisation/index.ts";
import { useOverlayHost } from "../overlay/index.ts";
import { catalogue } from "../panels/catalogue.ts";
import { LeftDock } from "../panels/left-dock.tsx";
import { modelOf } from "../panels/model.ts";
import { RightDock } from "../panels/right-dock.tsx";
import { insertDropHandlers } from "../panels/use-insert-drop.ts";
import { ProblemsDrawer } from "../problems/problems-panel.tsx";
import { useValidation } from "../problems/use-validation.ts";
import { browserValidator } from "../problems/validator-client.ts";
import { RustEditorHost } from "../rust-editor/index.ts";
import { useStateTarget } from "../state-designer/index.ts";
import { ExtraOverlays } from "./extra-overlays.tsx";
import { GhostLeaveDialog } from "./ghost-dialog.tsx";
import { type GhostTarget, ghostOfHit, pulseSteps, readAppScreens } from "./ghosts.ts";
import { LaneDialog, useLane } from "./lane-dialog.tsx";
import { LinkLayer } from "./link-layer.tsx";
import { redirectRecipe, removeLinkRecipe } from "./links.ts";
import { pluginCanvasMessages } from "./plugin-canvas.messages.ts";
import { MAX_ZOOM_PERCENT, MIN_ZOOM_PERCENT } from "./plugin-canvas.schema.ts";
import { parseDocument, readPlugin } from "./plugin-model.ts";
import { flowOrder, placeScreens, readSavedPositions } from "./positions.ts";
import {
  contentBounds,
  DEVICE,
  ghostStyle,
  type Heights,
  heightsById,
  type PlacedScreen,
  screenDocuments,
  screenStyle,
} from "./scene.ts";
import { ScreensController } from "./screens-controller.tsx";
import { canvasCommands, type StructureKind } from "./select/canvas-commands.ts";
import { type EditFn, findNode } from "./select/edits.ts";
import { structureEdit } from "./select/gesture.ts";
import { handleKey } from "./select/keys.ts";
import { announce } from "./select/live-region.ts";
import * as sel from "./select/selection.ts";
import { type SelectionStore, useSelectionStore } from "./select/store.ts";
import { indexNodes, itemIdOf, type NodeIndex } from "./select/tree.ts";
import { useGestures } from "./select/use-gestures.ts";
import { browserSurface, type CanvasSurface, canvasPoint } from "./surface.ts";
import { useGhostLayer } from "./use-ghost-layer.ts";
import { useNativeRoutes, useNativeWidgets } from "./use-native-widgets.ts";
import { useAppDocuments, usePluginDocuments } from "./use-plugin-documents.ts";

const MARQUEE_PX = 4;

/** Screens placed: saved positions, else flow order (STU-050); the canvas worker lays them out. */
export function placeModel(documents: Parameters<typeof readPlugin>[0]): PlacedScreen[] {
  const model = readPlugin(documents);
  const ids = model.screens.map((s) => s.id);
  const order = flowOrder(ids, model.entryPage ?? ids[0], model.edges);
  const widths = new Map(ids.map((id) => [id, DEVICE.width]));
  const at = placeScreens(order, widths, readSavedPositions(documents));
  const byId = new Map(model.screens.map((s) => [s.id, s]));
  return order.flatMap((id) => {
    const s = byId.get(id);
    const p = at.get(id);
    return s === undefined || p === undefined ? [] : [{ id, title: s.title, at: p, root: s.root }];
  });
}

/** A pointer gesture ended: a click selects, Shift+drag is a marquee (Alt: contain). */
function pointerSelect(
  surface: CanvasSurface,
  store: SelectionStore,
  index: NodeIndex,
  box: { left: number; top: number },
  from: { x: number; y: number },
  e: { clientX: number; clientY: number; shiftKey: boolean; altKey: boolean; ctrlKey: boolean; metaKey: boolean },
  onGhost: (hits: readonly string[]) => boolean,
): void {
  const a = { x: from.x - box.left, y: from.y - box.top };
  const z = { x: e.clientX - box.left, y: e.clientY - box.top };
  if (Math.hypot(z.x - a.x, z.y - a.y) <= MARQUEE_PX) {
    const hits = surface.hitTest?.(z.x, z.y) ?? [];
    if (onGhost(hits)) {
      return;
    }
    store.set(sel.click(index, store.get(), hits, { toggle: e.shiftKey, deep: e.ctrlKey || e.metaKey }));
  } else if (e.shiftKey) {
    const rect = { l: Math.min(a.x, z.x), t: Math.min(a.y, z.y), r: Math.max(a.x, z.x), b: Math.max(a.y, z.y) };
    store.set(sel.marquee(index, store.get(), surface.hitTestRect?.(rect, e.altKey ? "contain" : "intersect") ?? []));
  }
}

/** Fits the first view to the laid-out screens, unless the URL sets it. */
function fitView(
  surface: CanvasSurface,
  canvas: HTMLCanvasElement,
  screens: readonly PlacedScreen[],
  laidOut: Heights,
  s: { x?: number; y?: number; z?: number },
): void {
  const width = canvas.clientWidth || window.innerWidth;
  const height = canvas.clientHeight || window.innerHeight;
  const b = contentBounds(screens, laidOut);
  const zoom =
    s.z !== undefined
      ? s.z / 100
      : Math.min(1, Math.max(MIN_ZOOM_PERCENT / 100, Math.min(width / (b.r - b.l + 160), height / (b.b - b.t + 160))));
  const cx = s.x ?? (b.l + b.r) / 2;
  const cy = s.y ?? (b.t + b.b) / 2;
  surface.setViewport(cx - width / 2 / zoom, cy - height / 2 / zoom, zoom);
}

/** C2: the plugin canvas with selection (S2.5); edits stay blocked until the editor reports \`canEdit\` (S2.3). */
/** The tab's sessionStorage, or undefined where the browser blocks it. It holds only the editor-session ID. */
function tabStorage(): Storage | undefined {
  try {
    return globalThis.sessionStorage;
  } catch {
    return undefined;
  }
}

/** Hands the plugin's edit pipeline to an observer (tests); renders nothing. */
function ObserveEditor({ observe }: { readonly observe: (edit: EditSession) => void }): null {
  const edit = useEditSession();
  useEffect(() => {
    observe(edit);
  }, [observe, edit]);
  return null;
}

/** Registers edit.undo and edit.redo for the plugin's stack, disabled with the reason while read-only. */
function UndoCommands(props: {
  readonly undo: UndoManager;
  readonly stack: string;
  readonly canEdit: { current: boolean };
}): null {
  const { i18n } = useLingui();
  const canEdit = useCanEdit();
  props.canEdit.current = canEdit;
  const { undo, stack } = props;
  const list = useMemo(() => {
    const focus = createUndoFocus();
    focus.set({ undo: () => undo.undo(stack), redo: () => undo.redo(stack) });
    return undoCommands(i18n, focus, !canEdit);
  }, [i18n, undo, stack, canEdit]);
  useScopedCommands("plugin-undo", list);
  return null;
}

export function PluginCanvasPage() {
  const params = useParams({ strict: false }) as { app?: string; plugin?: string };
  const docs = usePluginDocuments(params.app ?? "", params.plugin ?? "");
  const { observeEditor } = useRouteContext({ strict: false }) as Pick<RouterContext, "observeEditor">;
  const stack = params.plugin ?? "";
  const appDocs = useAppDocuments(params.app ?? "");
  // One undo stack per plugin (F3); edit.undo and edit.redo act on it while this page is open.
  // Undo is blocked while the editor is read-only or lost its lock (F3); UndoCommands keeps this current.
  const canEdit = useRef(true);
  const [undo] = useState(
    () =>
      new UndoManager({
        depth: limitDefault("studio.undoDepth"),
        blocked: () => (canEdit.current ? undefined : "read-only"),
      }),
  );
  const [editorSessionId] = useState(() => tabEditorSession(tabStorage()));
  return (
    <EditorProvider
      appId={params.app ?? ""}
      pluginId={stack}
      documents={docs.data}
      undo={undo}
      editorSessionId={editorSessionId}
    >
      {observeEditor !== undefined && <ObserveEditor observe={observeEditor} />}
      <UndoCommands undo={undo} stack={stack} canEdit={canEdit} />
      <L10nSessionsProvider appId={params.app ?? ""} documents={appDocs.data}>
        <PluginCanvasView docs={docs} editorSessionId={editorSessionId} />
      </L10nSessionsProvider>
    </EditorProvider>
  );
}

function PluginCanvasView({
  docs,
  editorSessionId,
}: {
  docs: ReturnType<typeof usePluginDocuments>;
  editorSessionId: string;
}) {
  const m = pluginCanvasMessages(useLingui().i18n);
  const params = useParams({ strict: false }) as { org?: string; app?: string; plugin?: string };
  const search = useSearch({ strict: false }) as { x?: number; y?: number; z?: number; focus?: string };
  const navigate = useNavigate();
  const { createSurface = browserSurface, createValidator = browserValidator } = useRouteContext({
    strict: false,
  }) as Pick<RouterContext, "createSurface" | "createValidator">;
  const appDocs = useAppDocuments(params.app ?? "");
  const nativeWidgets = useNativeWidgets(params.app ?? "");
  const validation = useValidation(createValidator, appDocs.data, docs.data);
  const checked = validation.state === "done" ? validation : undefined;
  const stateTarget = useStateTarget(docs.data, appDocs.data);
  const local = useEditor((st) => st.docs);
  const parsed = useMemo(() => (docs.data ?? []).map((d) => ({ ...d, parsed: parseDocument(d.content) })), [docs.data]);
  // The editor's documents, unsaved edits included, over the server's (STU-121); untouched documents keep
  // their parsed objects, so the canvas lays out again only the screens whose root changed.
  const live = useMemo(
    () => parsed.map((d) => (local[d.path] === undefined ? d : { ...d, parsed: local[d.path]?.content })),
    [parsed, local],
  );
  const screens = useMemo(() => (docs.data === undefined ? [] : placeModel(live)), [docs.data, live]);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const surfaceRef = useRef<CanvasSurface | null>(null);
  const fittedRef = useRef<CanvasSurface | null>(null);
  const [heights, setHeights] = useState<Heights>(new Map());
  const nativeRoutes = useNativeRoutes(params.app ?? "");
  const appScreens = useMemo(() => readAppScreens(appDocs.data ?? []), [appDocs.data]);
  const own = useMemo(() => (docs.data === undefined ? [] : readPlugin(live).screens), [docs.data, live]);
  const [leaving, setLeaving] = useState<GhostTarget | null>(null);
  const [focusNote, setFocusNote] = useState("");
  const session = useEditSession();
  const canEdit = useCanEdit();
  const store = useSelectionStore();
  const selection = useEditor((st) => st.selection);
  const pathOf = useMemo(() => {
    const out = new Map<string, string>();
    for (const d of docs.data ?? []) {
      try {
        const id = (JSON.parse(new TextDecoder().decode(d.content)) as { id?: unknown }).id;
        if (typeof id === "string") {
          out.set(id, d.path);
        }
      } catch {
        // Not JSON: not a page document.
      }
    }
    return out;
  }, [docs.data]);
  const edit = useCallback<EditFn>(
    (docId, recipe, options) => {
      const path = pathOf.get(docId);
      if (canEdit && path !== undefined) {
        session.edit(path, (draft) => recipe((draft as { root?: unknown }).root), options);
      }
    },
    [canEdit, pathOf, session],
  );
  const ghostLayer = useGhostLayer({
    appId: params.app ?? "",
    pluginId: params.plugin ?? "",
    editorSessionId,
    canEdit,
    surface: () => surfaceRef.current,
    screens,
    own,
    heights,
    appScreens,
    nativeRoutes,
  });
  const index = useMemo(() => indexNodes(screens.map((s) => s.root)), [screens]);
  const screenOf = useMemo(() => {
    const out = new Map<string, string>();
    for (const sc of screens) {
      for (const id of indexNodes([sc.root]).parent.keys()) {
        out.set(id, sc.id);
      }
    }
    return out;
  }, [screens]);
  const announcement = announce(
    { none: () => m.selectionNone, one: m.selectionOne, many: m.selectionMany },
    selection,
    (id) => id,
  );
  useEffect(() => {
    store.set(sel.prune(index, store.get()));
  }, [index, store]);
  useEffect(() => {
    surfaceRef.current?.setSelection?.(
      selection.ids.flatMap((id) => {
        const screen = screenOf.get(id);
        return screen === undefined ? [] : [itemIdOf(screen, id)];
      }),
    );
  }, [selection, screenOf]);
  const structure = useCallback(
    (kind: StructureKind) =>
      structureEdit(
        { index, edit, docOf: (id) => screenOf.get(id), boxOf: () => undefined, typeOf: (id) => index.type.get(id) },
        kind,
        store.get().ids,
        () => crypto.randomUUID(),
      ),
    [index, edit, screenOf, store],
  );
  const canvasCommandList = useMemo(
    () => canvasCommands({ wrap: m.wrapCommand, unwrap: m.unwrapCommand, duplicate: m.duplicateCommand }, structure),
    [m.wrapCommand, m.unwrapCommand, m.duplicateCommand, structure],
  );
  useScopedCommands("plugin-canvas", canvasCommandList);
  const gestures = useGestures({
    surface: () => surfaceRef.current,
    canvas: () => canvasRef.current,
    store,
    index,
    edit,
    screenOf,
  });
  const dropHandlers = useMemo(
    () =>
      insertDropHandlers({
        surface: () => surfaceRef.current,
        canvas: () => canvasRef.current,
        index,
        model: modelOf(screens.map((x) => x.root)),
        edit,
        screenOf,
        canEdit,
        descriptors: catalogue(),
      }),
    [index, screens, edit, screenOf, canEdit],
  );
  const overlays = useOverlayHost({ surface: () => surfaceRef.current, canvas: () => canvasRef.current });
  const searchRef = useRef(search);
  searchRef.current = search;

  useEffect(() => {
    const canvas = canvasRef.current;
    if (canvas === null) {
      return undefined;
    }
    const width = canvas.clientWidth || window.innerWidth;
    const height = canvas.clientHeight || window.innerHeight;
    const surface = createSurface(canvas, { width, height, dpr: window.devicePixelRatio || 1 });
    surfaceRef.current = surface;
    return () => {
      surface.dispose();
      surfaceRef.current = null;
    };
  }, [createSurface]);

  const focusRef = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (search.focus !== undefined) {
      focusRef.current = search.focus;
    }
  }, [search.focus]);
  const pulses = useRef<ReturnType<typeof setTimeout>[]>([]);
  useEffect(
    () => () => {
      for (const t of pulses.current) {
        clearTimeout(t);
      }
    },
    [],
  );
  /** Pans to the screen the URL names and pulses it for 2 s (STU-053), once. */
  const showFocus = (surface: CanvasSurface, canvas: HTMLCanvasElement, laidOut: Heights) => {
    const id = focusRef.current;
    const target = screens.find((x) => x.id === id);
    if (id === undefined || target === undefined) {
      return;
    }
    focusRef.current = undefined;
    const h = Math.max(DEVICE.height, laidOut.get(id) ?? 0);
    const zoom = Math.min(1, (canvas.clientHeight || window.innerHeight) / (h + 160));
    const width = canvas.clientWidth || window.innerWidth;
    const height = canvas.clientHeight || window.innerHeight;
    surface.setViewport(
      target.at.x + DEVICE.width / 2 - width / 2 / zoom,
      target.at.y + h / 2 - height / 2 / zoom,
      zoom,
    );
    const rect = { l: target.at.x, t: target.at.y, r: target.at.x + DEVICE.width, b: target.at.y + h };
    for (const step of pulseSteps(`pulse:${frameId(id)}`, rect, ghostStyle.pulse)) {
      pulses.current.push(setTimeout(() => surface.patch?.(step.ops), step.at));
    }
    setFocusNote(m.ghostPulse(target.title));
    void navigate({ to: ".", search: (prev: object) => ({ ...prev, focus: undefined }), replace: true });
  };

  const showFocusRef = useRef(showFocus);
  showFocusRef.current = showFocus;

  useEffect(() => {
    const surface = surfaceRef.current;
    const canvas = canvasRef.current;
    if (surface === null || canvas === null || screens.length === 0) {
      return;
    }
    let current = true;
    void surface.layoutScreens(screenDocuments(screens), screenStyle).then((laidOut) => {
      if (!current) {
        return;
      }
      const byId = heightsById(laidOut);
      setHeights(byId);
      // Only the first layout on a surface fits the view; an edit keeps where the user looks.
      if (fittedRef.current !== surface) {
        fittedRef.current = surface;
        fitView(surface, canvas, screens, byId, searchRef.current);
        focusRef.current = searchRef.current.focus;
      }
      showFocusRef.current(surface, canvas, byId);
    });
    return () => {
      current = false;
    };
  }, [screens]);

  const sync = () => {
    const surface = surfaceRef.current;
    const canvas = canvasRef.current;
    if (surface === null || canvas === null) {
      return;
    }
    const v = surface.viewport;
    const x = Math.round(v.x + canvas.clientWidth / 2 / v.zoom);
    const y = Math.round(v.y + canvas.clientHeight / 2 / v.zoom);
    const z = Math.round(Math.min(MAX_ZOOM_PERCENT, Math.max(MIN_ZOOM_PERCENT, v.zoom * 100)));
    ghostLayer.zoomed(v.zoom);
    void navigate({ to: ".", search: (prev: object) => ({ ...prev, x, y, z }), replace: true });
  };

  const zoomBy = (factor: number) => {
    const canvas = canvasRef.current;
    if (canvas === null) {
      return;
    }
    // `zoomAbout` takes canvas-local CSS pixels: the centre of the canvas, not of its place on the page.
    const r = canvas.getBoundingClientRect();
    surfaceRef.current?.zoomAbout(factor, r.width / 2, r.height / 2);
    sync();
  };
  const fit = () => {
    const surface = surfaceRef.current;
    const canvas = canvasRef.current;
    if (surface !== null && canvas !== null) {
      fitView(surface, canvas, screens, heights, {});
      sync();
    }
  };
  const linkScreens = useMemo(
    () =>
      docs.data === undefined
        ? []
        : readPlugin(live).screens.map((s) => ({ id: s.id, title: s.title, route: s.route })),
    [docs.data, live],
  );
  const linkWidget = useMemo(() => {
    const id = selection.ids.length === 1 ? selection.ids[0] : undefined;
    const screen = id === undefined ? undefined : screenOf.get(id);
    const root = screens.find((s) => s.id === screen)?.root;
    const node = id === undefined || root === undefined ? null : findNode(root, id)?.node;
    return id === undefined || screen === undefined || node === null || node === undefined
      ? undefined
      : { id, screen, node };
  }, [selection, screenOf, screens]);
  const entryPage = useMemo(() => (docs.data === undefined ? undefined : readPlugin(docs.data).entryPage), [docs.data]);

  const { lane, setLane, site: laneSite, routes: laneRoutes } = useLane(screens, linkScreens, appScreens);

  const drag = useRef<{ x: number; y: number } | null>(null);
  const start = useRef<{ x: number; y: number } | null>(null);
  const percent = Math.round(Math.min(MAX_ZOOM_PERCENT, Math.max(MIN_ZOOM_PERCENT, search.z ?? 100)));

  return (
    <section className="relative h-full w-full overflow-hidden">
      {/* biome-ignore lint/a11y/noInteractiveElementToNoninteractiveRole: the pan and zoom surface; its content is the screen list below */}
      <canvas
        role="application"
        aria-label={m.title}
        ref={canvasRef}
        className="absolute inset-0 h-full w-full touch-none focus-visible:outline-2 focus-visible:-outline-offset-2"
        tabIndex={0}
        {...dropHandlers}
        onKeyDown={(e) => {
          const handled = handleKey({ store, index, edit, docOf: (id) => screenOf.get(id) }, e);
          if (handled) {
            e.preventDefault();
          }
        }}
        onDoubleClick={() => store.set(sel.enter(index, store.get()))}
        onPointerDown={(e) => {
          if (!e.shiftKey && !e.ctrlKey && !e.metaKey && gestures.down(e)) {
            start.current = { x: e.clientX, y: e.clientY };
            return;
          }
          drag.current = { x: e.clientX, y: e.clientY };
          start.current = { x: e.clientX, y: e.clientY };
        }}
        onPointerMove={(e) => {
          if (gestures.move(e)) {
            return;
          }
          const d = drag.current;
          if (d !== null && !e.shiftKey) {
            surfaceRef.current?.pan(e.clientX - d.x, e.clientY - d.y);
            drag.current = { x: e.clientX, y: e.clientY };
          }
        }}
        onPointerUp={(e) => {
          drag.current = null;
          const moved = gestures.up(e);
          const from = moved ? null : start.current;
          start.current = null;
          const box = canvasRef.current?.getBoundingClientRect();
          if (from !== null && surfaceRef.current && box) {
            pointerSelect(surfaceRef.current, store, index, box, from, e, (hits) => {
              const lane = ghostLayer.pick(hits)?.link;
              if (lane !== undefined) {
                setLane(lane);
                return true;
              }
              const ghost = ghostOfHit(hits, ghostLayer.placed);
              if (ghost === undefined) {
                return false;
              }
              if (ghost.kind === "screen") {
                setLeaving(ghost);
              }
              return true;
            });
          }
          sync();
        }}
        onWheel={(e) => {
          if (e.ctrlKey || e.metaKey) {
            const p = canvasPoint(e.currentTarget.getBoundingClientRect(), e.clientX, e.clientY);
            surfaceRef.current?.zoomAbout(Math.exp(-e.deltaY / 200), p.x, p.y);
          } else {
            surfaceRef.current?.pan(-e.deltaX, -e.deltaY);
          }
          sync();
        }}
      />
      {docs.isPending && <p className="absolute start-4 top-4 text-body text-secondary">{m.loading}</p>}
      {docs.isError && (
        <p role="alert" className="absolute start-4 top-4 text-body">
          {m.failed}
        </p>
      )}
      {docs.isSuccess && screens.length === 0 && (
        <p className="absolute start-4 top-4 text-body text-secondary">{m.empty}</p>
      )}
      <p role="status" aria-live="polite" className="sr-only">
        {announcement}
      </p>
      <p role="status" aria-live="polite" className="sr-only">
        {focusNote}
      </p>
      <GhostLeaveDialog
        ghost={leaving}
        onCancel={() => setLeaving(null)}
        onOpen={(ghost) => {
          setLeaving(null);
          if (ghost.pluginId !== undefined && ghost.screenId !== undefined) {
            void navigate({
              to: "/o/$org/apps/$app/plugins/$plugin",
              params: { org: params.org ?? "", app: params.app ?? "", plugin: ghost.pluginId },
              search: { focus: ghost.screenId },
            });
          }
        }}
      />
      <ol aria-label={m.screens} className="sr-only">
        {screens.map((s) => (
          <li key={s.id}>
            {s.title}
            {(heights.get(s.id) ?? 0) > DEVICE.height && <span id={overflowId(s.id)}> {m.overflow(s.title)}</span>}
          </li>
        ))}
      </ol>
      <LeftDock
        nativeWidgets={nativeWidgets}
        screens={screens}
        entryPage={entryPage}
        index={index}
        screenOf={screenOf}
        selection={selection}
        store={store}
        edit={edit}
        canEdit={canEdit}
        surface={() => surfaceRef.current}
        canvas={() => canvasRef.current}
      />
      <LinkLayer
        surface={() => surfaceRef.current}
        canvas={() => canvasRef.current}
        widget={linkWidget}
        screens={linkScreens}
        canEdit={canEdit}
        edit={edit}
      />
      {lane !== null && (
        <LaneDialog
          title={lane.title}
          site={laneSite}
          routes={laneRoutes}
          canEdit={canEdit}
          onClose={() => setLane(null)}
          onRedirect={(route) => {
            if (laneSite !== undefined) {
              edit(lane.source, redirectRecipe(laneSite.nodeId, laneSite.link, route));
            }
            setLane(null);
          }}
          onRemove={() => {
            if (laneSite !== undefined) {
              edit(lane.source, removeLinkRecipe(laneSite.nodeId, laneSite.link));
            }
            setLane(null);
          }}
        />
      )}
      <ProblemsDrawer validation={validation} />
      <RightDock
        screens={screens}
        index={index}
        screenOf={screenOf}
        edit={edit}
        pathOf={pathOf}
        problems={checked?.problems ?? []}
        documents={docs.data}
        appDocuments={appDocs.data}
        validatePage={checked?.validatePage}
        language={checked?.language}
        onOpenJson={() => overlays.show("json")}
        onOpenLocalisation={() => overlays.show("l10n")}
        onOpenState={() => overlays.show("state")}
        schemaVersion={stateTarget.schemaVersion}
        onOpenOverlay={overlays.show}
        onOpenGraph={() => overlays.show("graph")}
        onOpenRust={() => overlays.show("rust")}
      />
      <DesignersHost
        open={overlays.open}
        onClose={overlays.hide}
        documents={docs.data}
        appDocuments={appDocs.data}
        validation={validation}
      />
      <JsonEditorHost
        open={overlays.open === "json"}
        onClose={overlays.hide}
        documents={docs.data}
        screenOf={screenOf}
        pathOf={pathOf}
        validation={validation}
      />
      <RustEditorHost open={overlays.open === "rust"} onClose={overlays.hide} />
      <ExtraOverlays
        open={overlays.open}
        onClose={overlays.hide}
        documents={docs.data}
        appDocuments={appDocs.data}
        target={stateTarget}
        validatePage={checked?.validatePage}
      />
      <ActionGraphHost
        open={overlays.open === "graph"}
        onClose={overlays.hide}
        documents={docs.data}
        screenOf={screenOf}
        pathOf={pathOf}
        validation={validation}
      />
      {docs.data !== undefined && (
        <ScreensController
          appId={params.app ?? ""}
          pluginId={params.plugin ?? ""}
          editorSessionId={editorSessionId}
          documents={docs.data}
          appDocuments={appDocs.data}
          screens={screens.map((x) => ({ id: x.id, title: x.title }))}
          entryPage={entryPage}
          validation={validation}
          percent={percent}
          onZoom={zoomBy}
          onFit={fit}
        />
      )}
    </section>
  );
}
