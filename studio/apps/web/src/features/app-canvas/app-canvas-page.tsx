// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { timestampDate } from "@bufbuild/protobuf/wkt";
import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { AppService, PluginService, type PluginSummary, ReleaseService } from "@plux/api";
import type { ArrowPick, NavigateEdge } from "@plux/canvas";
import { Icon } from "@plux/ui";
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@plux/ui/components/empty";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useParams, useRouteContext } from "@tanstack/react-router";
import { type ReactNode, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useApp } from "../../lib/use-app.tsx";
import type { RouterContext } from "../../router.tsx";
import { tabEditorSession } from "../editor/document-transport.ts";
import { EditorProvider, useEditor, useEditSession } from "../editor/editor-provider.tsx";
import { useDeviceFonts } from "../plugin-canvas/device-fonts.ts";
import { useHostFonts } from "../plugin-canvas/host-fonts.ts";
import { uuidv7 } from "../plugin-canvas/screens.ts";
import { browserSurface, type CanvasSurface } from "../plugin-canvas/surface.ts";
import { useNativeRoutes } from "../plugin-canvas/use-native-widgets.ts";
import { useAppDocuments, usePluginDocuments } from "../plugin-canvas/use-plugin-documents.ts";
import { type ChangeSet, loadedDocs, useChangeSets } from "../safety/index.ts";
import { appCanvasMessages } from "./app-canvas.messages.ts";
import {
  APP_CANVAS_PATH,
  BOX,
  type BoxLink,
  canvasDocument,
  canvasRecipe,
  changeFrame,
  type Frame,
  type FrameChange,
  fitViewport,
  type Point,
  placeBoxes,
  readSavedCanvas,
  type SavedCanvas,
  select,
} from "./app-canvas.model.ts";
import {
  type AppLink,
  appGraph,
  badgesOf,
  nativeBox,
  redirectLink,
  removeLink,
  type SceneBox,
} from "./app-canvas.scene.ts";
import { BoxList, DetailCard, type LastPublished, MoveToDialog } from "./box-list.tsx";
import { AppBar, CanvasToolbar, LayersPanel, Panel, PluginActions, ZoomControls } from "./canvas-chrome.tsx";
import { FrameTools } from "./frame-tools.tsx";
import { LaneMenu } from "./lane-menu.tsx";
import { latestPublisher } from "./latest-publisher.ts";
import { type Dialogue, PluginDialogs } from "./plugin-dialogs.tsx";
import { miniMapOf } from "./plugin-minimap.ts";
import { PluginSafety, type SafetyMode } from "./plugin-safety.tsx";
import { useSceneTheme } from "./scene-theme.ts";
import { TrashDialog } from "./trash-dialog.tsx";
import { useAppScene } from "./use-app-scene.ts";
import { type CanvasTool, type GuideLine, useCanvasGestures } from "./use-canvas-gestures.ts";
import { useCanvasKeys } from "./use-canvas-keys.ts";
import { useZoom } from "./use-zoom.ts";
import "./app-canvas.css";
import { useTeams } from "./use-teams.ts";

export { PluginBoxCard } from "./box-list.tsx";
export { PluginDialogs } from "./plugin-dialogs.tsx";

const decoder = new TextDecoder();

function parseJson(bytes: Uint8Array): unknown {
  try {
    return JSON.parse(decoder.decode(bytes));
  } catch {
    return undefined;
  }
}

export function AppCanvasPage(): ReactNode {
  const params = useParams({ strict: false }) as { app?: string };
  const appId = params.app ?? "";
  const docs = usePluginDocuments(appId, "");
  return (
    <EditorProvider appId={appId} pluginId="" documents={docs.data}>
      <AppCanvasView appId={appId} documents={docs.data} />
    </EditorProvider>
  );
}

interface ViewProps {
  readonly appId: string;
  readonly documents: ReturnType<typeof usePluginDocuments>["data"];
}

/**
 * The canvas fills the page: the surface follows a change of the page's size (the first report is the size `initial`
 * it has). The page is measured, not the canvas, which the host sizes itself with a margin around the view.
 */
function followSize(page: HTMLElement, surface: CanvasSurface, initial: string): ResizeObserver | undefined {
  if (typeof ResizeObserver === "undefined") {
    return undefined;
  }
  let size = initial;
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

/** The figures and release chip of a plugin's box (STU-030): screens, versions and links, and `v3` or `Draft`. */
function boxFacts(
  s: PluginSummary | undefined,
  m: ReturnType<typeof appCanvasMessages>,
): Pick<SceneBox, "stats" | "release"> {
  if (s === undefined) {
    return {};
  }
  const versions = Number(s.versions);
  const latest = s.plugin?.latestVersion ?? 0n;
  return {
    stats: [
      { value: String(Number(s.screens)), label: m.statScreens },
      { value: String(versions), label: m.statVersions },
      { value: String(s.linksOut.length + s.linksIn.length), label: m.statLinks },
    ],
    release:
      versions > 0 && latest > 0n
        ? m.releaseVersion(String(latest))
        : versions > 0
          ? m.releaseVersion(String(versions))
          : m.releaseDraft,
  };
}

/** The marquee being drawn, in view pixels (screen space has no writing direction); nothing when none is. */
function Marquee({ rect }: { readonly rect: { l: number; t: number; r: number; b: number } | null }): ReactNode {
  if (rect === null) {
    return null;
  }
  const style = { "--x": rect.l, "--y": rect.t, "--w": rect.r - rect.l, "--h": rect.b - rect.t } as React.CSSProperties;
  return <div aria-hidden="true" className="canvas-marquee" style={style} />;
}

/**
 * The snap guides of a drag over the canvas (STU-032), in view pixels: the browser surface has no overlay layer, and
 * its overlay would also draw resize handles plugin boxes do not have.
 */
function SnapGuides({
  lines,
  view,
}: {
  readonly lines: readonly GuideLine[] | null;
  readonly view: { readonly x: number; readonly y: number; readonly zoom: number } | undefined;
}): ReactNode {
  if (lines === null || view === undefined) {
    return null;
  }
  return (
    <div aria-hidden="true">
      {lines.map((l) => {
        const vertical = l.axis === "x";
        const at = vertical ? (l.at - view.x) * view.zoom : (l.at - view.y) * view.zoom;
        const from = vertical ? (l.from - view.y) * view.zoom : (l.from - view.x) * view.zoom;
        const length = (l.to - l.from) * view.zoom;
        const style = (
          vertical
            ? { "--x": at, "--y": from, "--w": 0, "--h": length }
            : { "--x": from, "--y": at, "--w": length, "--h": 0 }
        ) as React.CSSProperties;
        return <div key={`${l.axis}:${l.at}`} className="canvas-guide" data-axis={l.axis} style={style} />;
      })}
    </div>
  );
}

/** The size of the view in CSS pixels, the window's while the page is not laid out yet, and the pixel ratio. */
function viewSize(view: HTMLElement): { width: number; height: number; dpr: number } {
  return {
    width: view.clientWidth || window.innerWidth,
    height: view.clientHeight || window.innerHeight,
    dpr: window.devicePixelRatio || 1,
  };
}

/** The selection's positions moved by `delta`; every other box keeps its place. */
function moved(boxes: readonly SceneBox[], ids: ReadonlySet<string>, delta: Point): Map<string, Point> {
  return new Map(boxes.map((b) => [b.id, ids.has(b.id) ? { x: b.x + delta.x, y: b.y + delta.y } : { x: b.x, y: b.y }]));
}

/**
 * Saves the canvas document: created by the first change and edited by every later one. Whether it exists is the edit
 * session's to say: the loaded document list is not refreshed by the creation, so the second change used to create it
 * again, which the session refuses ("The change could not be saved"). A change made while the creation is in flight
 * is applied once it lands. `settled` follows each change once the session holds it (or the creation failed, after
 * `failed`).
 */
function useCanvasPersist(
  edit: ReturnType<typeof useEditSession>,
  failed: () => void,
  settled: () => void,
): (next: SavedCanvas, group?: string) => void {
  const creating = useRef(false);
  const waiting = useRef<{ readonly next: SavedCanvas; readonly group: string } | null>(null);
  const done = useRef({ failed, settled });
  done.current = { failed, settled };
  return useCallback(
    (next: SavedCanvas, group = "canvas") => {
      if (edit.store.getState().docs[APP_CANVAS_PATH] !== undefined) {
        edit.edit(APP_CANVAS_PATH, canvasRecipe(next), { group });
        done.current.settled();
      } else if (creating.current) {
        waiting.current = { next, group };
      } else {
        creating.current = true;
        void edit.create(APP_CANVAS_PATH, canvasDocument(uuidv7(), next)).then((r) => {
          creating.current = false;
          const later = waiting.current;
          waiting.current = null;
          if (!r.ok) {
            done.current.failed();
          } else if (later !== null) {
            edit.edit(APP_CANVAS_PATH, canvasRecipe(later.next), { group: later.group });
          }
          done.current.settled();
        });
      }
    },
    [edit],
  );
}

/**
 * The plugins' positions and the frames on the canvas. What is saved is the edit session's own copy of the canvas
 * document, pending edits included, so the server confirming an earlier save, or a refetch of the document list,
 * cannot move a box back while a later change is on its way (a box used to jump back and return after a drop). The
 * view runs ahead of it only while a drag moves boxes and until the session holds a change.
 */
function useCanvasLayout(documents: ViewProps["documents"], failed: () => void) {
  const edit = useEditSession();
  const local = useEditor((s) => s.docs[APP_CANVAS_PATH]?.content);
  const saved = useMemo(() => {
    if (local !== undefined) {
      return readSavedCanvas(local);
    }
    const doc = documents?.find((d) => d.path === APP_CANVAS_PATH);
    return readSavedCanvas(doc === undefined ? undefined : parseJson(doc.content));
  }, [local, documents]);
  const [ahead, setAhead] = useState<SavedCanvas | null>(null);
  const latest = useRef(saved);
  latest.current = saved;
  const persist = useCanvasPersist(edit, failed, () => setAhead(null));
  /** Live positions while a drag runs; nothing is saved until `commit`. */
  const setLive = useCallback(
    (update: (prev: ReadonlyMap<string, Point>) => ReadonlyMap<string, Point>) =>
      setAhead((prev) => ({
        positions: update(prev?.positions ?? latest.current.positions),
        frames: prev?.frames ?? latest.current.frames,
      })),
    [],
  );
  const commit = useCallback(
    (next: SavedCanvas, group?: string) => {
      setAhead(next);
      persist(next, group);
    },
    [persist],
  );
  return { positions: ahead?.positions ?? saved.positions, frames: ahead?.frames ?? saved.frames, setLive, commit };
}

/** C1: the app canvas (STU-030 to STU-035), drawn by Plux Canvas with bundled arrows (STU-040 to STU-048). */
function AppCanvasView({ appId, documents }: ViewProps): ReactNode {
  const { i18n } = useLingui();
  const m = appCanvasMessages(i18n);
  const { session } = useApp();
  const navigate = useNavigate();
  const params = useParams({ strict: false }) as { org?: string };
  const orgKey = params.org ?? "";
  const { createSurface = browserSurface } = useRouteContext({ strict: false }) as Pick<RouterContext, "createSurface">;
  const hostFonts = useHostFonts(appId);
  const deviceFonts = useDeviceFonts(appId);
  const queryClient = useQueryClient();
  const plugins = useMemo(() => createClient(PluginService, session.transport), [session]);
  const apps = useMemo(() => createClient(AppService, session.transport), [session]);

  // One bounded call per app (plan §4.3 #14); refreshed when the window regains focus, because the
  // events stream carries no plugin-level events yet.
  const summaries = useQuery({
    queryKey: ["plugin-summaries", appId],
    queryFn: async () => (await plugins.listPluginSummaries({ appId })).summaries,
    refetchOnWindowFocus: true,
  });
  const app = useQuery({ queryKey: ["app", appId], queryFn: async () => (await apps.getApp({ id: appId })).app });
  const list = summaries.data ?? [];
  const entryKey = app.data?.defaultPluginKey ?? "";

  // The navigate links of every page, for the arrows and their lane fixes (STU-040, STU-046).
  const project = useAppDocuments(appId);
  const nativeRoutes = useNativeRoutes(appId);
  const loaded = useMemo(() => loadedDocs(project.data ?? []), [project.data]);
  const graph = useMemo(() => appGraph(loaded, nativeRoutes), [loaded, nativeRoutes]);

  const [message, setMessage] = useState<string | null>(null);
  const layout = useCanvasLayout(documents, () => setMessage(m.failed));
  const { positions, frames } = layout;
  const links = useMemo<BoxLink[]>(() => {
    const idOf = new Map(list.map((s) => [s.plugin?.key ?? "", s.plugin?.id ?? ""]));
    const between = list.flatMap((s) =>
      s.linksOut.flatMap((to) => {
        const target = idOf.get(to);
        return target === undefined || s.plugin === undefined ? [] : [{ from: s.plugin.id, to: target }];
      }),
    );
    const natives = graph.links.flatMap((l) =>
      l.targetPage === undefined ? [{ from: l.sourcePlugin, to: nativeBox(l.route) }] : [],
    );
    return [...between, ...natives];
  }, [list, graph]);
  const summaryOf = useMemo(() => new Map(list.map((s) => [s.plugin?.id ?? "", s])), [list]);
  const boxes = useMemo<SceneBox[]>(() => {
    const nodes = [
      ...list.flatMap((s) => (s.plugin === undefined ? [] : [{ id: s.plugin.id, key: s.plugin.key }])),
      ...graph.natives.map((r) => ({ id: nativeBox(r), key: r })),
    ];
    return placeBoxes(nodes, positions, links).map((b) => {
      const s = summaryOf.get(b.id);
      const native = s === undefined && b.id === nativeBox(b.key);
      return {
        ...b,
        title: native ? b.key : (s?.plugin?.name ?? b.key),
        native,
        archived: s?.plugin?.archived === true,
        badges: native ? [] : badgesOf(s, b.key === entryKey, m),
        ...(native ? { subtitle: m.native } : boxFacts(s, m)),
      };
    });
  }, [list, graph.natives, positions, links, summaryOf, entryKey, m]);

  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set());
  const [hover, setHover] = useState<string | undefined>();
  const [focused, setFocused] = useState<string | undefined>();
  const [dialogue, setDialogue] = useState<Dialogue>(null);
  const [safety, setSafety] = useState<SafetyMode | null>(null);
  const [trashOpen, setTrashOpen] = useState(false);
  const [movingBox, setMovingBox] = useState<SceneBox | undefined>();
  const [lane, setLane] = useState<AppLink | undefined>();

  // The surface: Plux Canvas on a worker; the main thread keeps input, the hit index and the DOM layer. Its colours
  // follow the page's theme; a change of theme makes the surface again on the new background.
  const theme = useSceneTheme();
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const sectionRef = useRef<HTMLElement>(null);
  const surfaceRef = useRef<CanvasSurface | null>(null);
  const [generation, setGeneration] = useState(0);
  useEffect(() => {
    const canvas = canvasRef.current;
    if (canvas === null) {
      return undefined;
    }
    const page = sectionRef.current ?? canvas;
    const { width, height, dpr } = viewSize(page);
    const surface = createSurface(canvas, {
      width,
      height,
      dpr,
      background: theme.background,
      grid: theme.grid,
      // A few hundred cards draw within a frame: the worker draws the whole view on every frame it changes, crisp and
      // in one piece, and text stays text at every zoom (studio-development §9.8).
      present: "direct",
      detail: "full",
      hostFonts,
      deviceFonts,
    });
    surfaceRef.current = surface;
    setGeneration((g) => g + 1);
    const observer = followSize(page, surface, `${width}x${height}`);
    return () => {
      observer?.disconnect();
      surface.dispose();
      surfaceRef.current = null;
    };
  }, [createSurface, hostFonts, deviceFonts, theme.background, theme.grid]);
  const surface = useCallback(() => surfaceRef.current, []);
  /** The element whose box is the view: the page, or the canvas before the page is laid out. */
  const viewOf = useCallback(() => sectionRef.current ?? canvasRef.current, []);
  const scene = useAppScene({
    surface,
    generation,
    boxes,
    frames,
    selected,
    focused,
    graph,
    style: theme.style,
    arrowStyle: theme.arrows,
  });

  const { zoom, setZoom, viewChanged, zoomBy, resetZoom } = useZoom(surface, viewOf, scene.zoomed);

  const commitLayout = layout.commit;
  const commit = useCallback(
    (nextPositions: ReadonlyMap<string, Point>, nextFrames: readonly Frame[], group?: string) =>
      commitLayout({ positions: nextPositions, frames: nextFrames }, group),
    [commitLayout],
  );
  // A frame rename or resize is its own undo step through the editing core (D20, STU-035).
  const changeFrameOf = useCallback(
    (id: string, change: FrameChange) => commit(positions, changeFrame(frames, id, change), `frame-${uuidv7()}`),
    [commit, positions, frames],
  );
  const teams = useTeams();

  const fit = useCallback(() => {
    const target = surfaceRef.current;
    const view = viewOf();
    if (target === null || view === null) {
      return;
    }
    const v = fitViewport(boxes, { width: view.clientWidth, height: view.clientHeight });
    target.setViewport(v.x, v.y, v.z);
    scene.zoomed(v.z);
    setZoom(v.z);
  }, [boxes, scene, viewOf, setZoom]);
  const fitted = useRef(0);
  useEffect(() => {
    if (fitted.current !== generation && boxes.length > 0 && surfaceRef.current !== null) {
      fitted.current = generation;
      fit();
    }
  }, [boxes, fit, generation]);

  const arrange = useCallback(() => {
    const laid = placeBoxes(
      boxes.map((b) => ({ id: b.id, key: b.key })),
      new Map(),
      links,
    );
    commit(new Map(laid.map((b) => [b.id, { x: b.x, y: b.y }])), frames);
  }, [boxes, links, frames, commit]);

  const open = (b: SceneBox) => {
    if (!b.native) {
      void navigate({
        to: "/o/$org/apps/$app/plugins/$plugin",
        params: { org: orgKey, app: appId, plugin: b.id },
      });
    }
  };
  const openScreen = (plugin: string, page: string) =>
    void navigate({
      to: "/o/$org/apps/$app/plugins/$plugin",
      params: { org: orgKey, app: appId, plugin },
      search: { focus: page },
    });
  const linkOf = useMemo(() => new Map(graph.links.map((l) => [l.edge.id, l])), [graph]);
  const openLane = (edge: NavigateEdge | undefined) => setLane(edge === undefined ? undefined : linkOf.get(edge.id));

  const [tool, setTool] = useState<CanvasTool>("select");
  const [marquee, setMarquee] = useState<{ l: number; t: number; r: number; b: number } | null>(null);
  const [guides, setGuides] = useState<readonly GuideLine[] | null>(null);
  const space = useRef(false);
  const gestures = useCanvasGestures({
    tool,
    frame: sectionRef,
    onMarquee: setMarquee,
    onGuides: setGuides,
    spaceHeld: () => space.current,
    element: generation,
    surface,
    canvas: canvasRef,
    boxes,
    selected,
    setSelected,
    setPositions: layout.setLive,
    commit: (next) => commit(next, frames),
    setHover,
    expand: scene.expand,
    pick: scene.pick,
    onLane: (p: ArrowPick) => openLane(p.link),
    onOpen: open,
    onView: viewChanged,
  });

  const nudge = (box: SceneBox, delta: Point) => {
    const ids = selected.has(box.id) ? selected : new Set([box.id]);
    commit(moved(boxes, ids, delta), frames);
    setMessage(m.moved(box.title, Math.round(box.x + delta.x), Math.round(box.y + delta.y)));
  };
  const moveTo = (box: SceneBox, to: Point) => {
    commit(moved(boxes, new Set([box.id]), { x: to.x - box.x, y: to.y - box.y }), frames);
    setMessage(m.moved(box.title, Math.round(to.x), Math.round(to.y)));
    setMovingBox(undefined);
  };
  /** Keyboard focus on a box: the ring is drawn, and the view follows when the box is out of sight. */
  const focusBox = (id: string | undefined) => {
    setFocused(id);
    setHover(id);
    const b = boxes.find((x) => x.id === id);
    const target = surfaceRef.current;
    const canvas = canvasRef.current;
    if (b === undefined || target === null || canvas === null) {
      return;
    }
    const v = target.viewport;
    const view = viewOf() ?? canvas;
    const w = view.clientWidth / v.zoom;
    const h = view.clientHeight / v.zoom;
    if (b.x < v.x || b.y < v.y || b.x + BOX.width > v.x + w || b.y + BOX.height > v.y + h) {
      target.setViewport(b.x + BOX.width / 2 - w / 2, b.y + BOX.height / 2 - h / 2, v.zoom);
    }
  };

  const refresh = () => queryClient.invalidateQueries({ queryKey: ["plugin-summaries", appId] });
  const mutation = useMutation({
    mutationFn: async (run: () => Promise<unknown>) => run(),
    onSuccess: async () => {
      setMessage(null);
      setDialogue(null);
      await Promise.all([refresh(), queryClient.invalidateQueries({ queryKey: ["app", appId] })]);
    },
    onError: () => setMessage(m.failed),
  });
  const [editorSession] = useState(() => tabEditorSession(undefined));
  const applyChanges = useChangeSets(appId, editorSession, undefined);
  const applyLane = (changeSet: ChangeSet) => {
    setLane(undefined);
    void applyChanges(changeSet).then(async (ok) => {
      setMessage(ok ? null : m.changeRefused);
      await queryClient.invalidateQueries({ queryKey: ["project-documents", appId] });
    });
  };

  const one = selected.size === 1 ? summaryOf.get([...selected][0] ?? "") : undefined;
  const detail = summaryOf.get(hover ?? [...selected][0] ?? "");

  const detailPlugin = detail?.plugin;
  const publisher = useQuery({
    queryKey: ["plugin-last-published", detailPlugin?.id, detail?.versions?.toString()],
    enabled: detailPlugin !== undefined && (detail?.versions ?? 0n) > 0n,
    queryFn: async () => latestPublisher(createClient(ReleaseService, session.transport), detailPlugin?.id ?? ""),
  });
  const published = useMemo<LastPublished | undefined>(() => {
    const at =
      publisher.data?.at ?? (detail?.publishedAt === undefined ? undefined : timestampDate(detail.publishedAt));
    if (at === undefined) {
      return undefined;
    }
    return { by: publisher.data?.by ?? "", at: i18n.date(at, { dateStyle: "medium", timeStyle: "short" }) };
  }, [publisher.data, detail?.publishedAt, i18n]);
  const miniMap = useMemo(() => miniMapOf(graph, detailPlugin?.id ?? ""), [graph, detailPlugin?.id]);

  /** 100% about the middle of the canvas. */
  /** A row of the plugins panel: selects its plugin and brings it into view. */
  const showBox = (b: SceneBox, additive: boolean) => {
    setSelected(select(selected, b.id, additive));
    const target = surfaceRef.current;
    const canvas = viewOf();
    if (target === null || canvas === null) {
      return;
    }
    const v = target.viewport;
    target.setViewport(
      b.x + BOX.width / 2 - canvas.clientWidth / v.zoom / 2,
      b.y + BOX.height / 2 - canvas.clientHeight / v.zoom / 2,
      v.zoom,
    );
  };

  useCanvasKeys({ canvas: canvasRef, space, fit, zoomBy, resetZoom, setTool, clear: () => setSelected(new Set()) });

  const archiveOrRestore = () => {
    const p = one?.plugin;
    if (p?.archived === true) {
      mutation.mutate(() => plugins.archivePlugin({ id: p.id, archived: false }));
    } else if (p !== undefined) {
      setSafety("archive");
    }
  };
  // The selected plugin's actions, in its detail panel when the panel shows it.
  const selectedBox = boxes.find((b) => b.id === one?.plugin?.id);
  const actions = (
    <PluginActions
      selectedId={selectedBox?.id}
      shownId={detail?.plugin?.id}
      messages={m}
      archived={one?.plugin?.archived === true}
      onOpen={() => selectedBox !== undefined && open(selectedBox)}
      onDialogue={(kind) => setDialogue({ kind })}
      onMove={() => setMovingBox(selectedBox)}
      onArchive={archiveOrRestore}
      onDelete={() => setSafety("delete")}
    />
  );

  return (
    <section
      ref={sectionRef}
      aria-label={m.title}
      className="app-canvas relative h-full w-full overflow-hidden"
      data-tool={tool}
    >
      {/* A canvas hands its drawing to the worker once: a new theme gets a new element, and a new surface on it. */}
      <canvas
        key={theme.background}
        ref={canvasRef}
        className="absolute inset-0 size-full touch-none"
        {...gestures.handlers}
      />
      <Marquee rect={marquee} />
      <SnapGuides lines={guides} view={surfaceRef.current?.viewport} />
      <BoxList
        boxes={boxes}
        bundles={scene.bundles}
        frames={frames}
        selected={selected}
        messages={m}
        onPointerDown={(e, b) => gestures.pressBox(b, e.shiftKey || e.metaKey || e.ctrlKey)}
        onSelect={(b, additive) => setSelected(select(selected, b.id, additive))}
        onOpen={open}
        onFocusBox={focusBox}
        onNudge={nudge}
        onFocusBundle={scene.expand}
        onLane={openLane}
      />
      {/* The chrome floats over the scene; only the panels take the pointer, the canvas runs on beneath them. */}
      <div className="canvas-chrome pointer-events-none absolute inset-0 grid p-3">
        <div className="canvas-top flex items-start justify-between gap-3">
          <AppBar org={orgKey} app={app.data} plugins={list.length} messages={m} />
          <CanvasToolbar
            tool={tool}
            onTool={setTool}
            messages={m}
            selected={selected.size}
            onNew={() => setDialogue({ kind: "new" })}
            onFrame={() => setDialogue({ kind: "frame" })}
            onArrange={arrange}
            onTrash={() => setTrashOpen(true)}
          />
          <Panel className="canvas-bar flex px-4">
            <output aria-live="polite" className="text-xs text-muted-foreground">
              {message ?? m.selected(selected.size)}
            </output>
          </Panel>
        </div>
        <div className="canvas-middle flex min-h-0 items-start justify-between gap-3">
          <LayersPanel
            boxes={boxes}
            frames={frames}
            selected={selected}
            messages={m}
            onShow={showBox}
            onOpen={open}
            frameTools={<FrameTools frames={frames} messages={m} onChange={changeFrameOf} />}
          />
          {detail?.plugin === undefined ? null : (
            <DetailCard
              summary={detail}
              badges={boxes.find((b) => b.id === detail.plugin?.id)?.badges ?? []}
              messages={m}
              miniMap={miniMap}
              published={published}
              actions={actions}
            />
          )}
        </div>
        <div className="canvas-bottom flex items-end justify-between gap-3">
          <ZoomControls
            zoom={zoom}
            messages={m}
            onZoomIn={() => zoomBy(1.25)}
            onZoomOut={() => zoomBy(0.8)}
            onReset={resetZoom}
            onFit={fit}
          />
          <Panel className="canvas-bar canvas-hint hidden px-4 text-xs text-muted-foreground lg:flex">{m.hint}</Panel>
        </div>
      </div>
      {list.length === 0 && summaries.isSuccess ? (
        <div className="pointer-events-none absolute inset-0 grid place-items-center p-6">
          <Panel className="max-w-sm">
            <Empty>
              <EmptyHeader>
                <EmptyMedia variant="icon">
                  <Icon name="puzzle-piece" />
                </EmptyMedia>
                <EmptyTitle>{m.empty}</EmptyTitle>
                <EmptyDescription>{m.emptyHint}</EmptyDescription>
              </EmptyHeader>
            </Empty>
          </Panel>
        </div>
      ) : null}
      <MoveToDialog
        box={movingBox}
        messages={m}
        onMove={(to) => movingBox !== undefined && moveTo(movingBox, to)}
        onClose={() => setMovingBox(undefined)}
      />
      <LaneMenu
        key={lane?.edge.id}
        link={lane}
        pages={graph.pages}
        messages={m}
        onOpenScreen={openScreen}
        onRedirect={(l, route) => applyLane(redirectLink(loaded, l, route))}
        onRemove={(l) => applyLane(removeLink(loaded, l))}
        onClose={() => setLane(undefined)}
      />
      {safety !== null && one?.plugin !== undefined && (
        <PluginSafety
          appId={appId}
          plugin={one.plugin}
          mode={safety}
          onClose={() => setSafety(null)}
          onDone={() => {
            setSafety(null);
            setMessage(null);
            setSelected(new Set());
            void refresh();
          }}
          onFailed={(text) => {
            setSafety(null);
            setMessage(text);
          }}
        />
      )}
      {trashOpen && <TrashDialog appId={appId} onClose={() => setTrashOpen(false)} onRestored={() => void refresh()} />}
      <PluginDialogs
        dialogue={dialogue}
        messages={m}
        summary={one}
        taken={new Set(list.map((s) => s.plugin?.key ?? ""))}
        selectedBoxes={boxes.filter((b) => selected.has(b.id))}
        teams={teams}
        close={() => setDialogue(null)}
        onCreate={(name, key) => mutation.mutate(() => plugins.createPlugin({ appId, key, name }))}
        onSettings={(id, name, description, tags, owningTeamId) =>
          mutation.mutate(() =>
            plugins.updatePlugin({ id, name, metadata: { description, setTags: true, tags, owningTeamId } }),
          )
        }
        onDuplicate={(id, name, key) =>
          mutation.mutate(() => plugins.duplicatePlugin({ id, name, key, session: uuidv7() }))
        }
        onEntry={(key) =>
          mutation.mutate(() => apps.updateApp({ id: appId, name: app.data?.name ?? "", defaultPluginKey: key }))
        }
        onFrame={(label) => {
          const sel = boxes.filter((b) => selected.has(b.id));
          const minX = Math.min(...sel.map((b) => b.x)) - 24;
          const minY = Math.min(...sel.map((b) => b.y)) - 40;
          const maxX = Math.max(...sel.map((b) => b.x + BOX.width)) + 24;
          const maxY = Math.max(...sel.map((b) => b.y + BOX.height)) + 24;
          commit(new Map(boxes.map((b) => [b.id, { x: b.x, y: b.y }])), [
            ...frames,
            { id: uuidv7(), label, x: minX, y: minY, width: maxX - minX, height: maxY - minY },
          ]);
          setDialogue(null);
        }}
      />
    </section>
  );
}
