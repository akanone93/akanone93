// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { overflowId } from "@plux/canvas";
import { useNavigate, useParams, useRouteContext, useSearch } from "@tanstack/react-router";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useScopedCommands } from "../../lib/commands/index.ts";
import type { RouterContext } from "../../router.tsx";
import { EditorProvider, useCanEdit, useEditor, useEditSession } from "../editor/editor-provider.tsx";
import { ProblemsDrawer } from "../problems/problems-panel.tsx";
import { useValidation } from "../problems/use-validation.ts";
import { browserValidator } from "../problems/validator-client.ts";
import { pluginCanvasMessages } from "./plugin-canvas.messages.ts";
import { MAX_ZOOM_PERCENT, MIN_ZOOM_PERCENT } from "./plugin-canvas.schema.ts";
import { readPlugin } from "./plugin-model.ts";
import { flowOrder, placeScreens, readSavedPositions } from "./positions.ts";
import {
  contentBounds,
  DEVICE,
  type Heights,
  heightsById,
  type PlacedScreen,
  screenDocuments,
  screenStyle,
} from "./scene.ts";
import { ScreensController } from "./screens-controller.tsx";
import { canvasCommands, type StructureKind } from "./select/canvas-commands.ts";
import type { EditFn } from "./select/edits.ts";
import { structureEdit } from "./select/gesture.ts";
import { handleKey } from "./select/keys.ts";
import { announce } from "./select/live-region.ts";
import * as sel from "./select/selection.ts";
import { type SelectionStore, useSelectionStore } from "./select/store.ts";
import { indexNodes, itemIdOf, type NodeIndex } from "./select/tree.ts";
import { useGestures } from "./select/use-gestures.ts";
import { browserSurface, type CanvasSurface } from "./surface.ts";
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
): void {
  const a = { x: from.x - box.left, y: from.y - box.top };
  const z = { x: e.clientX - box.left, y: e.clientY - box.top };
  if (Math.hypot(z.x - a.x, z.y - a.y) <= MARQUEE_PX) {
    const hits = surface.hitTest?.(z.x, z.y) ?? [];
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
export function PluginCanvasPage() {
  const params = useParams({ strict: false }) as { app?: string; plugin?: string };
  const docs = usePluginDocuments(params.app ?? "", params.plugin ?? "");
  return (
    <EditorProvider appId={params.app ?? ""} pluginId={params.plugin ?? ""} documents={docs.data}>
      <PluginCanvasView docs={docs} />
    </EditorProvider>
  );
}

function PluginCanvasView({ docs }: { docs: ReturnType<typeof usePluginDocuments> }) {
  const m = pluginCanvasMessages(useLingui().i18n);
  const params = useParams({ strict: false }) as { app?: string; plugin?: string };
  const search = useSearch({ strict: false }) as { x?: number; y?: number; z?: number };
  const navigate = useNavigate();
  const { createSurface = browserSurface, createValidator = browserValidator } = useRouteContext({
    strict: false,
  }) as Pick<RouterContext, "createSurface" | "createValidator">;
  const appDocs = useAppDocuments(params.app ?? "");
  const validation = useValidation(createValidator, appDocs.data, docs.data);
  const screens = useMemo(() => (docs.data === undefined ? [] : placeModel(docs.data)), [docs.data]);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const surfaceRef = useRef<CanvasSurface | null>(null);
  const [heights, setHeights] = useState<Heights>(new Map());
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
      fitView(surface, canvas, screens, byId, searchRef.current);
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
    void navigate({ to: ".", search: (prev: object) => ({ ...prev, x, y, z }), replace: true });
  };

  const zoomBy = (factor: number) => {
    const canvas = canvasRef.current;
    if (canvas === null) {
      return;
    }
    const r = canvas.getBoundingClientRect();
    surfaceRef.current?.zoomAbout(factor, r.left + r.width / 2, r.top + r.height / 2);
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
  const entryPage = useMemo(() => (docs.data === undefined ? undefined : readPlugin(docs.data).entryPage), [docs.data]);

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
            pointerSelect(surfaceRef.current, store, index, box, from, e);
          }
          sync();
        }}
        onWheel={(e) => {
          if (e.ctrlKey || e.metaKey) {
            surfaceRef.current?.zoomAbout(Math.exp(-e.deltaY / 200), e.clientX, e.clientY);
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
      <ol aria-label={m.screens} className="sr-only">
        {screens.map((s) => (
          <li key={s.id}>
            {s.title}
            {(heights.get(s.id) ?? 0) > DEVICE.height && <span id={overflowId(s.id)}> {m.overflow(s.title)}</span>}
          </li>
        ))}
      </ol>
      <ProblemsDrawer validation={validation} />
      {docs.data !== undefined && (
        <ScreensController
          appId={params.app ?? ""}
          pluginId={params.plugin ?? ""}
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
