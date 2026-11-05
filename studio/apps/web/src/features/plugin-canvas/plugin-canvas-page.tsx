// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { FunctionAdminService } from "@plux/api";
import { frameId, overflowId } from "@plux/canvas";
import { limitDefault } from "@plux/schema";
import { useNavigate, useParams, useRouteContext, useSearch } from "@tanstack/react-router";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { createUndoFocus, undoCommands, useScopedCommands } from "../../lib/commands/index.ts";
import { useApp } from "../../lib/use-app.tsx";
import type { RouterContext } from "../../router.tsx";
import { ActionGraphHost } from "../action-graph/index.ts";
import { useSceneTheme } from "../app-canvas/scene-theme.ts";
import { useScreenImages } from "./screen-images.ts";
import "./plugin-editor.css";
import { CanvasStatus } from "../app-canvas/canvas-chrome.tsx";
import { GestureTracker } from "../app-canvas/use-canvas-gestures.ts";
import { DesignersHost } from "../designers/designers-host.tsx";
import type { DrawerTab } from "../devices/drawer-model.ts";
import { tabEditorSession } from "../editor/document-transport.ts";
import type { EditSession } from "../editor/edit-session.ts";
import { EditorProvider, useCanEdit, useEditor, useEditSession } from "../editor/editor-provider.tsx";
import { SaveStateChip } from "../editor/save-state.tsx";
import { UndoManager } from "../editor/undo-stack.ts";
import { JsonEditorHost } from "../json-editor/index.ts";
import { useLibraryActions } from "../library/canvas/use-library-actions.tsx";
import { useCan } from "../library/use-library.ts";
import { L10nSessionsProvider } from "../localisation/index.ts";
import { useOverlayHost } from "../overlay/index.ts";
import { catalogue } from "../panels/catalogue.ts";
import { LeftDock } from "../panels/left-dock.tsx";
import { modelOf } from "../panels/model.ts";
import { panelsMessages } from "../panels/panels.messages.ts";
import { RightDock } from "../panels/right-dock.tsx";
import { lockedIds } from "../panels/studio-meta.ts";
import { PaintThemeContext } from "../panels/style-editors.tsx";
import { insertDropHandlers } from "../panels/use-insert-drop.ts";
import { MatrixOverlay, usePreviewBar } from "../preview/preview-bar.tsx";
import { ProblemCounts } from "../problems/problems-panel.tsx";
import { useValidation, type Validation } from "../problems/use-validation.ts";
import { browserValidator } from "../problems/validator-client.ts";
import { RustEditorHost } from "../rust-editor/index.ts";
import { useStateTarget } from "../state-designer/index.ts";
import { targetHas, useClipboard } from "./clipboard/use-clipboard.tsx";
import { useCopyOver } from "./clipboard/use-copy-over.ts";
import { DeviceBayHost } from "./device-bay-host.tsx";
import { useDeviceFonts } from "./device-fonts.ts";
import { ExtraOverlays } from "./extra-overlays.tsx";
import { GhostLeaveDialog } from "./ghost-dialog.tsx";
import { type GhostTarget, ghostOfHit, pulseSteps, readAppScreens } from "./ghosts.ts";
import { useHostFonts } from "./host-fonts.ts";
import { LaneDialog, useLane } from "./lane-dialog.tsx";
import { LinkLayer } from "./link-layer.tsx";
import { redirectRecipe, removeLinkRecipe } from "./links.ts";
import { dataSourcesOf, messagesFor, titledScreens } from "./look.ts";
import { resolveNodeParam } from "./node-path.ts";
import { PagePartsHost, useShownDrawers } from "./page-parts.tsx";
import { useBrands } from "./paint-theme.ts";
import { pluginCanvasMessages } from "./plugin-canvas.messages.ts";
import { MAX_ZOOM_PERCENT, MIN_ZOOM_PERCENT } from "./plugin-canvas.schema.ts";
import { PluginIdentity } from "./plugin-identity.tsx";
import { pageStates, parseDocument, readPlugin } from "./plugin-model.ts";
import { flowOrder, type Point, placeScreens, readSavedPositions } from "./positions.ts";
import {
  contentBounds,
  DEVICE,
  type deviceOf,
  ghostStyle,
  type Heights,
  heightsById,
  type PlacedScreen,
  screenDocuments,
} from "./scene.ts";
import type { ScreenRequest } from "./screens-controller.tsx";
import { ScreensController } from "./screens-controller.tsx";
import { canvasCommands, type StructureKind } from "./select/canvas-commands.ts";
import { type EditFn, findNode } from "./select/edits.ts";
import { structureEdit } from "./select/gesture.ts";
import { handleKey } from "./select/keys.ts";
import { announce } from "./select/live-region.ts";
import { createOverlayStore } from "./select/overlay-store.ts";
import * as sel from "./select/selection.ts";
import { useSelectionStore } from "./select/store.ts";
import { indexNodes, itemIdOf, type NodeIndex } from "./select/tree.ts";
import { browserSurface, type CanvasSurface, canvasPoint } from "./surface.ts";
import { useAssetsTab } from "./use-assets-tab.ts";
import { guidesToOverlay, useCanvasInteraction } from "./use-canvas-interaction.tsx";
import { useGhostLayer } from "./use-ghost-layer.ts";
import { useNativeRoutes, useNativeWidgets } from "./use-native-widgets.ts";
import { useAppDocuments, usePluginDocuments } from "./use-plugin-documents.ts";
import { usePreviewScene } from "./use-preview-scene.ts";
import { useScreenPositions } from "./use-screen-positions.ts";

/**
 * Screens placed: saved positions, else flow order (STU-050); the canvas worker lays them out. `saved` — the edit
 * session's copy of the positions, or a drop on its way — wins over the documents' own.
 */
export function placeModel(
  documents: Parameters<typeof readPlugin>[0],
  saved?: ReadonlyMap<string, Point>,
  width: number = DEVICE.width,
): PlacedScreen[] {
  const model = readPlugin(documents);
  const ids = model.screens.map((s) => s.id);
  const order = flowOrder(ids, model.entryPage ?? ids[0], model.edges);
  const widths = new Map(ids.map((id) => [id, width]));
  const at = placeScreens(order, widths, saved ?? readSavedPositions(documents));
  const byId = new Map(model.screens.map((s) => [s.id, s]));
  return order.flatMap((id) => {
    const s = byId.get(id);
    const p = at.get(id);
    return s === undefined || p === undefined
      ? []
      : [
          {
            id,
            title: s.title,
            route: s.route,
            kind: s.kind,
            ...(s.titleKey === undefined ? {} : { titleKey: s.titleKey }),
            at: p,
            root: s.root,
          },
        ];
  });
}

/** What a screen's header shows after its name: the route, the page kind and the entry mark (PC-11). */
function headerMeta(
  m: ReturnType<typeof pluginCanvasMessages>,
  entry: string | undefined,
  // The locale the strings are in: a new locale is a new function, so the headers are drawn again.
  _locale: string,
): (s: PlacedScreen) => string | undefined {
  return (s) =>
    s.route === undefined ? undefined : m.screenMeta(s.route, m.pageKind(s.kind ?? "screen"), s.id === entry);
}

/** The size of the device frame the screens are drawn in. */
type Device = ReturnType<typeof deviceOf>;

/** The zoom factor of a wheel step with Ctrl or Cmd, kept within the zoom range (10–400 %). */
function wheelFactor(zoom: number, deltaY: number): number {
  const next = Math.min(MAX_ZOOM_PERCENT / 100, Math.max(MIN_ZOOM_PERCENT / 100, zoom * Math.exp(-deltaY / 200)));
  return next / zoom;
}

/**
 * One wheel step: with Ctrl or Cmd (a trackpad pinch) a zoom about the pointer, else a pan — a mouse wheel with Shift
 * scrolls sideways, a trackpad gives both deltas itself. A burst of steps is one gesture for the worker (M-12).
 */
function wheelStep(surface: CanvasSurface, tracker: GestureTracker, canvas: HTMLCanvasElement, e: WheelEvent): void {
  if (e.ctrlKey || e.metaKey) {
    tracker.wheel(surface, "zoom");
    const p = canvasPoint(canvas.getBoundingClientRect(), e.clientX, e.clientY);
    surface.zoomAbout(wheelFactor(surface.viewport.zoom, e.deltaY), p.x, p.y);
    return;
  }
  tracker.wheel(surface, "pan");
  const sideways = e.shiftKey && e.deltaX === 0;
  surface.pan(sideways ? -e.deltaY : -e.deltaX, sideways ? 0 : -e.deltaY);
}

/** The part of the canvas the docks and the top bar leave free, in canvas pixels. */
function freeArea(canvas: HTMLCanvasElement, width: number, height: number) {
  const r = canvas.getBoundingClientRect();
  const stage = canvas.parentElement;
  const docks = [".plugin-dock-start", ".plugin-dock-end"].flatMap((q) => {
    const d = stage?.querySelector(q)?.getBoundingClientRect();
    return d === undefined || d.width === 0 ? [] : [d];
  });
  const middle = r.left + width / 2;
  const left = Math.max(0, ...docks.filter((d) => d.right <= middle).map((d) => d.right - r.left));
  const right = Math.min(width, ...docks.filter((d) => d.left >= middle).map((d) => d.left - r.left));
  const top = Math.max(0, ...docks.map((d) => d.top - r.top));
  return { left, right: right > left ? right : width, top: top < height / 2 ? top : 0 };
}

/**
 * The first view (STU-067): 100 %, the screens centred in the room the docks leave, their tops under the top bar when
 * they are taller than that room; the URL's view wins when it names one. Fit fits them all on demand.
 */
function fitView(
  surface: CanvasSurface,
  canvas: HTMLCanvasElement,
  screens: readonly PlacedScreen[],
  laidOut: Heights,
  s: { x?: number; y?: number; z?: number },
  device: Device,
): void {
  const width = canvas.clientWidth || window.innerWidth;
  const height = canvas.clientHeight || window.innerHeight;
  const b = contentBounds(screens, laidOut, device);
  const zoom = s.z !== undefined ? s.z / 100 : 1;
  if (s.x !== undefined || s.y !== undefined) {
    const cx = s.x ?? (b.l + b.r) / 2;
    const cy = s.y ?? (b.t + b.b) / 2;
    surface.setViewport(cx - width / 2 / zoom, cy - height / 2 / zoom, zoom);
    return;
  }
  const free = freeArea(canvas, width, height);
  const x = (b.l + b.r) / 2 - (free.left + (free.right - free.left) / 2) / zoom;
  const room = height - free.top;
  const tall = (b.b - b.t) * zoom > room - 48;
  const y = tall ? b.t - (free.top + 24) / zoom : (b.t + b.b) / 2 - (free.top + room / 2) / zoom;
  surface.setViewport(x, y, zoom);
}

/** Brings a screen into view at the current zoom, when it is picked in the top bar. */
function centreOn(
  surface: CanvasSurface | null,
  canvas: HTMLCanvasElement | null,
  target: PlacedScreen | undefined,
  laidOut: Heights,
  device: Device,
): void {
  if (surface === null || canvas === null || target === undefined) {
    return;
  }
  const z = surface.viewport.zoom;
  const h = Math.max(device.height, laidOut.get(target.id) ?? 0);
  surface.setViewport(
    target.at.x + device.width / 2 - canvas.clientWidth / 2 / z,
    target.at.y + h / 2 - canvas.clientHeight / 2 / z,
    z,
  );
}

/** The next request to open the console on Problems: a new number, so the same tab opens again. */
const problemsRequest = (r: { readonly n: number } | undefined) => ({ tab: "problems" as const, n: (r?.n ?? 0) + 1 });

/** The end of the plugin canvas's top bar: the selection, the save state, the problem counts and the theme switch. */
function PluginStatus(props: {
  readonly ids: readonly string[];
  readonly index: NodeIndex;
  readonly validation: Validation;
  readonly onProblems: () => void;
}) {
  const { i18n } = useLingui();
  return (
    <CanvasStatus>
      <output aria-live="polite" className="canvas-status-text">
        {selectionText(props.ids, props.index, pluginCanvasMessages(i18n), panelsMessages(i18n))}
      </output>
      <SaveStateChip />
      <ProblemCounts validation={props.validation} onOpen={props.onProblems} />
    </CanvasStatus>
  );
}

/** The top bar's line about the selection: nothing, one widget by its type, or how many. */
function selectionText(
  ids: readonly string[],
  index: NodeIndex,
  m: ReturnType<typeof pluginCanvasMessages>,
  pm: ReturnType<typeof panelsMessages>,
): string {
  if (ids.length === 0) {
    return m.nothingSelected;
  }
  return ids.length === 1 ? m.oneSelected(index.type.get(ids[0] ?? "") ?? "") : pm.selectedCount(ids.length);
}

/** Fit: every laid-out screen in view, at most 100 % (STU-067). */
function fitAll(
  surface: CanvasSurface,
  canvas: HTMLCanvasElement,
  screens: readonly PlacedScreen[],
  laidOut: Heights,
  device: Device,
): void {
  const width = canvas.clientWidth || window.innerWidth;
  const height = canvas.clientHeight || window.innerHeight;
  const b = contentBounds(screens, laidOut, device);
  const zoom = Math.min(
    1,
    Math.max(MIN_ZOOM_PERCENT / 100, Math.min(width / (b.r - b.l + 160), height / (b.b - b.t + 160))),
  );
  surface.setViewport((b.l + b.r) / 2 - width / 2 / zoom, (b.t + b.b) / 2 - height / 2 / zoom, zoom);
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

/** Follows a change of the page's size: the surface draws the view at the page's size (the canvas element is the view). */
function followPage(page: HTMLElement, surface: CanvasSurface): ResizeObserver | undefined {
  if (typeof ResizeObserver === "undefined") {
    return undefined;
  }
  let size = `${page.clientWidth}x${page.clientHeight}`;
  const observer = new ResizeObserver(() => {
    const next = `${page.clientWidth}x${page.clientHeight}`;
    if (page.clientWidth > 0 && page.clientHeight > 0 && next !== size) {
      size = next;
      surface.resize?.(page.clientWidth, page.clientHeight, window.devicePixelRatio || 1);
    }
  });
  observer.observe(page);
  return observer;
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
  const assets = useAssetsTab(editorSessionId);
  const search = useSearch({ strict: false }) as { x?: number; y?: number; z?: number; focus?: string; node?: string };
  const navigate = useNavigate();
  const { createSurface = browserSurface, createValidator = browserValidator } = useRouteContext({
    strict: false,
  }) as Pick<RouterContext, "createSurface" | "createValidator">;
  const appDocs = useAppDocuments(params.app ?? "");
  const { session: app } = useApp();
  const hostFonts = useHostFonts(params.app);
  const deviceFonts = useDeviceFonts(params.app);
  const tests = useMemo(() => createClient(FunctionAdminService, app.transport), [app.transport]);
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
  const sources = useMemo(() => dataSourcesOf(docs.data ?? []), [docs.data]);
  const brands = useBrands(appDocs.data);
  const { look, bar, switcher, matrix } = usePreviewBar(
    params.app,
    sources.map((x) => x.id),
    brands,
  );
  // The device the screens are drawn in (STU-057, PC-11): the frame chosen in the action bar, in its orientation.
  const device = useMemo<Device>(
    () => ({ width: look.device.width, height: look.device.height }),
    [look.device.width, look.device.height],
  );
  // A translated title reads in the previewed locale, as the screen itself does.
  const screenPositions = useScreenPositions(docs.data, useCanEdit());
  const placed = useMemo(
    () => (docs.data === undefined ? [] : placeModel(live, screenPositions.positions, device.width)),
    [docs.data, live, screenPositions.positions, device.width],
  );
  const titleMessages = useMemo(
    () => messagesFor([...(appDocs.data ?? []), ...(docs.data ?? [])], look),
    [appDocs.data, docs.data, look],
  );
  const screens = useMemo(() => titledScreens(placed, titleMessages), [placed, titleMessages]);
  // Each screen's header names its route and kind, and marks the plugin's entry (PC-11).
  const entryId = useMemo(() => (docs.data === undefined ? undefined : readPlugin(docs.data).entryPage), [docs.data]);
  const { i18n } = useLingui();
  const meta = useMemo(
    () => headerMeta(pluginCanvasMessages(i18n), entryId, i18n.locale),
    [i18n, i18n.locale, entryId],
  );
  // A drawer is drawn open when the user shows it or the selection is inside it (PC-12).
  const picked = useEditor((st) => st.selection);
  const shownRoots = useMemo(() => screens.map((s) => s.root), [screens]);
  const { drawers, toggle: toggleDrawer } = useShownDrawers(shownRoots, picked.ids);
  const { shown, styled, paint } = usePreviewScene(screens, appDocs.data, docs.data, look, drawers);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const surfaceRef = useRef<CanvasSurface | null>(null);
  const [overlay] = useState(createOverlayStore);
  const fittedRef = useRef<CanvasSurface | null>(null);
  const [heights, setHeights] = useState<Heights>(new Map());
  const nativeRoutes = useNativeRoutes(params.app ?? "");
  const appScreens = useMemo(() => readAppScreens(appDocs.data ?? []), [appDocs.data]);
  const own = useMemo(() => (docs.data === undefined ? [] : readPlugin(live).screens), [docs.data, live]);
  const [leaving, setLeaving] = useState<GhostTarget | null>(null);
  const [focusNote, setFocusNote] = useState("");
  const session = useEditSession();
  const canEdit = useCanEdit();
  const canSend = useCan("devsession.create");
  const canPublish = useCan("release.publish");
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
    device,
  });
  const roots = useMemo(() => screens.map((s) => s.root), [screens]);
  useScreenImages(params.app, () => surfaceRef.current, roots, heights.size);
  const index = useMemo(() => indexNodes(roots), [roots]);
  const locked = useMemo(() => lockedIds(screens.map((s) => s.root)), [screens]);
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
    // Announced by what it is, not by its id (WCAG 4.1.3).
    (id) => index.type.get(id) ?? id,
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
    () =>
      canvasCommands(
        {
          wrap: m.wrapCommand,
          unwrap: m.unwrapCommand,
          duplicate: m.duplicateCommand,
          experiment: m.experimentCommand,
        },
        structure,
        () => {
          const ids = store.get().ids;
          if (ids.length > 0) {
            void navigate({
              to: "/o/$org/apps/$app/experiments/new",
              params: { org: params.org ?? "", app: params.app ?? "" },
              search: { target: ids.join(",") },
            } as never);
          }
        },
      ),
    [
      m.wrapCommand,
      m.unwrapCommand,
      m.duplicateCommand,
      m.experimentCommand,
      structure,
      store,
      navigate,
      params.org,
      params.app,
    ],
  );
  useScopedCommands("plugin-canvas", canvasCommandList);
  const clipboardSource = useMemo(
    () => ({
      ...(params.org === undefined ? {} : { organisation: params.org }),
      app: params.app ?? "",
      plugin: params.plugin ?? "",
    }),
    [params.org, params.app, params.plugin],
  );
  const clipboardHas = useMemo(
    () => targetHas([...(docs.data ?? []), ...(appDocs.data ?? [])]),
    [docs.data, appDocs.data],
  );
  const copyOverInput = useCopyOver(docs.data, session);
  const statesOfPages = useMemo(() => pageStates(live), [live]);
  const clipboardInput = {
    copyOver: copyOverInput,
    pageState: (screenId: string) => statesOfPages.get(screenId) ?? [],
    screens,
    screenOf,
    selected: () => store.get().ids,
    edit,
    canEdit,
    source: clipboardSource,
    has: clipboardHas,
    validatePage: checked?.validatePage,
  };
  const clipboardDialog = useClipboard(clipboardInput);
  const libraryDialog = useLibraryActions(clipboardInput);
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
  const [rustSource, setRustSource] = useState<string | undefined>(undefined);
  const overlays = useOverlayHost({ surface: () => surfaceRef.current, canvas: () => canvasRef.current });
  const searchRef = useRef(search);
  searchRef.current = search;

  // The surface, as on the app canvas (studio-development §9.8): drawn whole on every frame it changes, in full detail,
  // on the theme's canvas surface with its dot grid, and following the page's size. A canvas element is handed to the
  // worker once, so a change of theme makes a new element (`key`) and a new surface on it.
  const theme = useSceneTheme();
  const sectionRef = useRef<HTMLElement>(null);
  useEffect(() => {
    const canvas = canvasRef.current;
    if (canvas === null) {
      return undefined;
    }
    const page = sectionRef.current ?? canvas;
    const width = page.clientWidth || window.innerWidth;
    const height = page.clientHeight || window.innerHeight;
    const surface = createSurface(canvas, {
      width,
      height,
      dpr: window.devicePixelRatio || 1,
      background: theme.background,
      grid: theme.grid,
      present: "direct",
      detail: "full",
      hostFonts,
      deviceFonts,
    });
    surfaceRef.current = guidesToOverlay(surface, overlay);
    const observer = followPage(page, surface);
    return () => {
      observer?.disconnect();
      surface.dispose();
      surfaceRef.current = null;
    };
  }, [createSurface, hostFonts, deviceFonts, theme.background, theme.grid, overlay]);

  const focusRef = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (search.focus !== undefined) {
      focusRef.current = search.focus;
    }
  }, [search.focus]);
  /** Selects the node the URL names once the screens are loaded (ANL-040), once. */
  const selectedFromUrl = useRef<string | undefined>(undefined);
  useEffect(() => {
    const param = search.node;
    if (param === undefined || selectedFromUrl.current === param || screens.length === 0) {
      return;
    }
    // A node ID, or the `root/0` path an analytics error reports, read in the screen `focus` names.
    const focusScreen =
      screens.find((x) => x.id === search.focus) ??
      screens.find((x) => x.id === own.find((o) => o.route === search.focus)?.id);
    const node = resolveNodeParam(param, new Set(index.parent.keys()), focusScreen);
    selectedFromUrl.current = param;
    if (node !== undefined) {
      store.set({ ids: [node], anchor: node, scope: index.parent.get(node) ?? null });
    }
  }, [search.node, search.focus, index, screens, own, store]);
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
    const h = Math.max(device.height, laidOut.get(id) ?? 0);
    const zoom = Math.min(1, (canvas.clientHeight || window.innerHeight) / (h + 160));
    const width = canvas.clientWidth || window.innerWidth;
    const height = canvas.clientHeight || window.innerHeight;
    surface.setViewport(
      target.at.x + device.width / 2 - width / 2 / zoom,
      target.at.y + h / 2 - height / 2 / zoom,
      zoom,
    );
    const rect = { l: target.at.x, t: target.at.y, r: target.at.x + device.width, b: target.at.y + h };
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
    void surface.layoutScreens(screenDocuments(shown, meta), styled).then((laidOut) => {
      if (!current) {
        return;
      }
      const byId = heightsById(laidOut);
      setHeights(byId);
      // Only the first layout on a surface fits the view; an edit keeps where the user looks.
      if (fittedRef.current !== surface) {
        fittedRef.current = surface;
        fitView(surface, canvas, screens, byId, searchRef.current, device);
        focusRef.current = searchRef.current.focus;
      }
      showFocusRef.current(surface, canvas, byId);
    });
    return () => {
      current = false;
    };
  }, [screens, shown, styled, meta, device]);

  // The view's place in the URL and the ghosts' level of detail follow the view at most once a frame: a router update
  // per wheel step or pointer move re-rendered the whole editor on each, and the canvas stuttered.
  const syncFrame = useRef(0);
  useEffect(() => () => cancelAnimationFrame(syncFrame.current), []);
  const sync = () => {
    overlay.viewChanged();
    if (syncFrame.current === 0) {
      syncFrame.current = requestAnimationFrame(() => {
        syncFrame.current = 0;
        syncNow();
      });
    }
  };
  const syncNow = () => {
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

  // The wheel as a native, non-passive listener: React's is passive, so Ctrl + wheel (a pinch) zoomed the whole page.
  const syncRef = useRef(sync);
  syncRef.current = sync;
  // biome-ignore lint/correctness/useExhaustiveDependencies: a new canvas element (a new theme) needs the listener again
  useEffect(() => {
    const canvas = canvasRef.current;
    if (canvas === null) {
      return undefined;
    }
    // A burst of wheel steps is one pan or zoom gesture for the worker (M-12), as on the app canvas.
    const tracker = new GestureTracker();
    const onWheel = (e: WheelEvent) => {
      e.preventDefault();
      const surface = surfaceRef.current;
      if (surface !== null) {
        wheelStep(surface, tracker, canvas, e);
        syncRef.current();
      }
    };
    canvas.addEventListener("wheel", onWheel, { passive: false });
    return () => {
      canvas.removeEventListener("wheel", onWheel);
      tracker.end();
    };
  }, [theme.key]);

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
      fitAll(surface, canvas, screens, heights, device);
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
  const pluginInfo = useMemo(() => (docs.data === undefined ? undefined : readPlugin(docs.data)), [docs.data]);
  const entryPage = pluginInfo?.entryPage;

  const { lane, setLane, site: laneSite, routes: laneRoutes } = useLane(screens, linkScreens, appScreens);

  const resetZoom = () => zoomBy(1 / (surfaceRef.current?.viewport.zoom ?? 1));
  const showScreen = (id: string) => {
    // Picking a screen in the bar selects it, so the bar, the screen actions and the selection agree.
    const root = screens.find((x) => x.id === id)?.root?.id;
    if (root !== undefined) {
      store.set({ ids: [root], anchor: root, scope: null });
    }
    centreOn(
      surfaceRef.current,
      canvasRef.current,
      screens.find((x) => x.id === id),
      heights,
      device,
    );
    sync();
  };
  const [consoleRequest, setConsoleRequest] = useState<{ readonly tab: DrawerTab; readonly n: number } | undefined>();

  const [screenRequest, setScreenRequest] = useState<ScreenRequest | undefined>();
  const requestScreen = useCallback(
    (kind: ScreenRequest["kind"], screen: string) => setScreenRequest((r) => ({ kind, screen, n: (r?.n ?? 0) + 1 })),
    [],
  );
  /** Ghost screens and lanes take a click before the nodes (STU-052, STU-053). */
  const pickGhost = (hits: readonly string[]): boolean => {
    const picked = ghostLayer.pick(hits)?.link;
    if (picked !== undefined) {
      setLane(picked);
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
  };
  const interaction = useCanvasInteraction({
    surface: () => surfaceRef.current,
    canvas: canvasRef,
    store,
    selection,
    index,
    screenOf,
    edit,
    canEdit,
    locked,
    screens,
    shown,
    styled,
    meta,
    positions: screenPositions,
    onPick: pickGhost,
    onView: sync,
    fit,
    zoomBy,
    resetZoom,
    requestScreen,
    messages: m,
    overlay,
  });
  const percent = Math.round(Math.min(MAX_ZOOM_PERCENT, Math.max(MIN_ZOOM_PERCENT, search.z ?? 100)));

  return (
    <PaintThemeContext.Provider value={paint}>
      <section
        ref={sectionRef}
        className="plugin-canvas canvas-stage relative h-full w-full overflow-hidden"
        data-preview-theme={look.theme}
        data-preview-platform={look.platform}
      >
        <MatrixOverlay show={matrix} render={look} screen={shown[0]} style={styled} createSurface={createSurface} />
        {/* biome-ignore lint/a11y/noInteractiveElementToNoninteractiveRole: the pan and zoom surface; its content is the screen list below */}
        <canvas
          key={theme.key}
          role="application"
          aria-label={m.title}
          ref={canvasRef}
          className="absolute inset-0 h-full w-full touch-none focus-visible:outline-2 focus-visible:-outline-offset-2"
          tabIndex={0}
          {...dropHandlers}
          data-tool={interaction.tool}
          onKeyDown={(e) => {
            // A selection with a locked layer still moves the selection, but edits nothing.
            const frozen = selection.ids.some((id) => locked.has(id));
            const handled = handleKey(
              { store, index, edit: frozen ? () => undefined : edit, docOf: (id) => screenOf.get(id) },
              e,
            );
            if (handled) {
              e.preventDefault();
            } else if (e.key === "Backspace" && canEdit && selection.ids.length > 0) {
              // Delete is the registered command; Backspace deletes too, while the canvas has the focus.
              e.preventDefault();
              interaction.remove();
            }
          }}
          {...interaction.handlers}
        />
        {interaction.element}
        <PagePartsHost
          store={overlay}
          surface={() => surfaceRef.current}
          roots={roots}
          screens={screens}
          selection={selection}
          selectionStore={store}
          screenOf={screenOf}
          edit={edit}
          canEdit={canEdit}
          look={look}
          drawers={drawers}
          onToggleDrawer={toggleDrawer}
        />
        {docs.isPending && <p className="absolute start-4 top-4 text-body text-muted-foreground">{m.loading}</p>}
        {docs.isError && (
          <p role="alert" className="absolute start-4 top-4 text-body">
            {m.failed}
          </p>
        )}
        {docs.isSuccess && screens.length === 0 && (
          <p className="absolute start-4 top-4 text-body text-muted-foreground">{m.empty}</p>
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
        {/* The previewed locale's direction and text scale are the screens' (the canvas draws them so); Studio's own
            panels keep Studio's. The screen list read to assistive technology is in the previewed locale. */}
        <ol aria-label={m.screens} className="sr-only" dir={look.direction} lang={look.locale}>
          {screens.map((s) => (
            <li key={s.id}>
              {s.title}
              {(heights.get(s.id) ?? 0) > device.height && <span id={overflowId(s.id)}> {m.overflow(s.title)}</span>}
            </li>
          ))}
        </ol>
        <LeftDock
          title={pluginInfo?.name}
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
          documents={docs.data}
          onOpenState={() => overlays.show("state")}
          onOpenData={() => overlays.show("data")}
          assets={assets}
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
        {clipboardDialog}
        <DeviceBayHost
          appId={params.app}
          request={consoleRequest}
          live={canSend}
          validation={validation}
          screens={screens}
          store={store}
          index={index}
          selection={selection}
          local={local}
          assets={assets}
          screenOf={screenOf}
          entryPage={entryPage}
        />
        {libraryDialog}
        <RightDock
          tests={tests}
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
          onOpenRust={() => {
            setRustSource(undefined);
            overlays.show("rust");
          }}
          onOpenFunction={(name) => {
            setRustSource(name);
            overlays.show("rust");
          }}
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
        <RustEditorHost open={overlays.open === "rust"} onClose={overlays.hide} sourceId={rustSource} />
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
            selectedScreen={interaction.selectedScreen}
            tool={interaction.tool}
            onTool={interaction.setTool}
            request={screenRequest}
            entryPage={entryPage}
            validation={validation}
            percent={percent}
            onZoom={zoomBy}
            onZoomReset={resetZoom}
            onFit={fit}
            onShowScreen={showScreen}
            identity={<PluginIdentity org={params.org} appId={params.app} plugin={pluginInfo?.name} />}
            status={
              <PluginStatus
                ids={selection.ids}
                index={index}
                validation={validation}
                onProblems={() => setConsoleRequest(problemsRequest)}
              />
            }
            onHistory={() => overlays.show("history")}
            preview={
              <>
                {switcher}
                {bar}
              </>
            }
            canSend={canSend}
            canPublish={canPublish}
            onPublish={() =>
              void navigate({
                to: "/o/$org/apps/$app/publish",
                params: { org: params.org ?? "", app: params.app ?? "" },
              })
            }
          />
        )}
      </section>
    </PaintThemeContext.Provider>
  );
}
