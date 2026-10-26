// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { AppService, PluginService } from "@plux/api";
import type { ArrowPick, NavigateEdge } from "@plux/canvas";
import { Button } from "@plux/ui";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useParams, useRouteContext } from "@tanstack/react-router";
import { type ReactNode, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useApp } from "../../lib/use-app.tsx";
import type { RouterContext } from "../../router.tsx";
import { tabEditorSession } from "../editor/document-transport.ts";
import { EditorProvider, useEditSession } from "../editor/editor-provider.tsx";
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
  type Frame,
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
import { BoxList, DetailCard, MoveToDialog } from "./box-list.tsx";
import { LaneMenu } from "./lane-menu.tsx";
import { type Dialogue, PluginDialogs } from "./plugin-dialogs.tsx";
import { PluginSafety, type SafetyMode } from "./plugin-safety.tsx";
import { TrashDialog } from "./trash-dialog.tsx";
import { useAppScene } from "./use-app-scene.ts";
import { useCanvasGestures } from "./use-canvas-gestures.ts";

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

/** The selection's positions moved by `delta`; every other box keeps its place. */
function moved(boxes: readonly SceneBox[], ids: ReadonlySet<string>, delta: Point): Map<string, Point> {
  return new Map(boxes.map((b) => [b.id, ids.has(b.id) ? { x: b.x + delta.x, y: b.y + delta.y } : { x: b.x, y: b.y }]));
}

/** C1: the app canvas (STU-030 to STU-035), drawn by Plux Canvas with bundled arrows (STU-040 to STU-048). */
function AppCanvasView({ appId, documents }: ViewProps): ReactNode {
  const { i18n } = useLingui();
  const m = appCanvasMessages(i18n);
  const { session } = useApp();
  const navigate = useNavigate();
  const params = useParams({ strict: false }) as { org?: string };
  const { createSurface = browserSurface } = useRouteContext({ strict: false }) as Pick<RouterContext, "createSurface">;
  const queryClient = useQueryClient();
  const edit = useEditSession();
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

  const saved = useMemo(() => {
    const doc = documents?.find((d) => d.path === APP_CANVAS_PATH);
    return readSavedCanvas(doc === undefined ? undefined : parseJson(doc.content));
  }, [documents]);
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
  const [positions, setPositions] = useState<ReadonlyMap<string, Point>>(new Map());
  const [frames, setFrames] = useState<readonly Frame[]>([]);
  useEffect(() => {
    setPositions(saved.positions);
    setFrames(saved.frames);
  }, [saved]);
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
      };
    });
  }, [list, graph.natives, positions, links, summaryOf, entryKey, m]);

  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set());
  const [hover, setHover] = useState<string | undefined>();
  const [focused, setFocused] = useState<string | undefined>();
  const [message, setMessage] = useState<string | null>(null);
  const [dialogue, setDialogue] = useState<Dialogue>(null);
  const [safety, setSafety] = useState<SafetyMode | null>(null);
  const [trashOpen, setTrashOpen] = useState(false);
  const [movingBox, setMovingBox] = useState<SceneBox | undefined>();
  const [lane, setLane] = useState<AppLink | undefined>();

  // The surface: Plux Canvas on a worker; the main thread keeps input, the hit index and the DOM layer.
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const surfaceRef = useRef<CanvasSurface | null>(null);
  const [generation, setGeneration] = useState(0);
  useEffect(() => {
    const canvas = canvasRef.current;
    if (canvas === null) {
      return undefined;
    }
    const width = canvas.clientWidth || window.innerWidth;
    const height = canvas.clientHeight || window.innerHeight;
    const surface = createSurface(canvas, { width, height, dpr: window.devicePixelRatio || 1 });
    surfaceRef.current = surface;
    setGeneration((g) => g + 1);
    return () => {
      surface.dispose();
      surfaceRef.current = null;
    };
  }, [createSurface]);
  const surface = useCallback(() => surfaceRef.current, []);
  const scene = useAppScene({ surface, generation, boxes, frames, selected, focused, graph });

  const persist = useCallback(
    (next: SavedCanvas) => {
      const exists = documents?.some((d) => d.path === APP_CANVAS_PATH) === true;
      if (exists) {
        edit.edit(APP_CANVAS_PATH, canvasRecipe(next), { group: "canvas" });
      } else {
        void edit.create(APP_CANVAS_PATH, canvasDocument(uuidv7(), next)).then((r) => {
          if (!r.ok) {
            setMessage(m.failed);
          }
        });
      }
    },
    [documents, edit, m.failed],
  );
  const commit = useCallback(
    (nextPositions: ReadonlyMap<string, Point>, nextFrames: readonly Frame[]) => {
      setPositions(nextPositions);
      setFrames(nextFrames);
      persist({ positions: nextPositions, frames: nextFrames });
    },
    [persist],
  );

  const fit = useCallback(() => {
    const canvas = canvasRef.current;
    const target = surfaceRef.current;
    if (target === null) {
      return;
    }
    const v = fitViewport(boxes, { width: canvas?.clientWidth ?? 0, height: canvas?.clientHeight ?? 0 });
    target.setViewport(v.x, v.y, v.z);
    scene.zoomed(v.z);
  }, [boxes, scene]);
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
        params: { org: params.org ?? "", app: appId, plugin: b.id },
      });
    }
  };
  const openScreen = (plugin: string, page: string) =>
    void navigate({
      to: "/o/$org/apps/$app/plugins/$plugin",
      params: { org: params.org ?? "", app: appId, plugin },
      search: { focus: page },
    });
  const linkOf = useMemo(() => new Map(graph.links.map((l) => [l.edge.id, l])), [graph]);
  const openLane = (edge: NavigateEdge | undefined) => setLane(edge === undefined ? undefined : linkOf.get(edge.id));

  const gestures = useCanvasGestures({
    surface,
    canvas: canvasRef,
    boxes,
    selected,
    setSelected,
    setPositions,
    commit: (next) => commit(next, frames),
    setHover,
    expand: scene.expand,
    pick: scene.pick,
    onLane: (p: ArrowPick) => openLane(p.link),
    onOpen: open,
    onView: () => scene.zoomed(surfaceRef.current?.viewport.zoom ?? 1),
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
    const w = canvas.clientWidth / v.zoom;
    const h = canvas.clientHeight / v.zoom;
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

  return (
    <section aria-label={m.title} className="relative flex h-full w-full flex-col bg-canvas">
      <div role="toolbar" aria-label={m.toolbar} className="flex items-center gap-2 p-2">
        <Button onClick={() => setDialogue({ kind: "new" })}>{m.newPlugin}</Button>
        <Button disabled={selected.size === 0} onClick={() => setDialogue({ kind: "frame" })}>
          {m.newFrame}
        </Button>
        <Button onClick={arrange}>{m.autoLayout}</Button>
        <Button onClick={fit}>{m.zoomToFit}</Button>
        <Button disabled={one === undefined} onClick={() => setDialogue({ kind: "settings" })}>
          {m.settings}
        </Button>
        <Button disabled={one === undefined} onClick={() => setDialogue({ kind: "duplicate" })}>
          {m.duplicate}
        </Button>
        <Button
          disabled={one?.plugin === undefined}
          onClick={() => {
            const p = one?.plugin;
            if (p?.archived === true) {
              mutation.mutate(() => plugins.archivePlugin({ id: p.id, archived: false }));
            } else if (p !== undefined) {
              setSafety("archive");
            }
          }}
        >
          {one?.plugin?.archived === true ? m.restore : m.archive}
        </Button>
        <Button disabled={one?.plugin === undefined} onClick={() => setSafety("delete")}>
          {m.deletePlugin}
        </Button>
        <Button disabled={selected.size !== 1} onClick={() => setMovingBox(boxes.find((b) => selected.has(b.id)))}>
          {m.moveSelection}
        </Button>
        <Button onClick={() => setTrashOpen(true)}>{m.trash}</Button>
        <Button disabled={one === undefined} onClick={() => setDialogue({ kind: "entry" })}>
          {m.setEntry}
        </Button>
        <output aria-live="polite" className="text-data text-secondary">
          {message ?? m.selected(selected.size)}
        </output>
      </div>
      <div className="relative flex-1 overflow-hidden">
        <canvas ref={canvasRef} className="absolute inset-0 h-full w-full touch-none" {...gestures.handlers} />
        {list.length === 0 && summaries.isSuccess ? (
          <p className="pointer-events-none absolute start-4 top-4 text-secondary">{m.empty}</p>
        ) : null}
        {detail?.plugin === undefined ? null : (
          <DetailCard
            summary={detail}
            badges={boxes.find((b) => b.id === detail.plugin?.id)?.badges ?? []}
            messages={m}
          />
        )}
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
      </div>
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
        close={() => setDialogue(null)}
        onCreate={(name, key) => mutation.mutate(() => plugins.createPlugin({ appId, key, name }))}
        onSettings={(id, name, description, tags) =>
          mutation.mutate(() => plugins.updatePlugin({ id, name, metadata: { description, setTags: true, tags } }))
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
