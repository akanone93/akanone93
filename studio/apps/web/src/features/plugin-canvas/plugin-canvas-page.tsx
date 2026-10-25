// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { useNavigate, useParams, useRouteContext, useSearch } from "@tanstack/react-router";
import { useEffect, useMemo, useRef } from "react";
import type { RouterContext } from "../../router.tsx";
import { pluginCanvasMessages } from "./plugin-canvas.messages.ts";
import { MAX_ZOOM_PERCENT, MIN_ZOOM_PERCENT } from "./plugin-canvas.schema.ts";
import { readPlugin } from "./plugin-model.ts";
import { flowOrder, placeScreens, readSavedPositions } from "./positions.ts";
import { contentBounds, overflowId, type PlacedScreen, sceneOps } from "./scene.ts";
import { DEVICE, layoutScreen } from "./screen-layout.ts";
import { browserSurface, type CanvasSurface } from "./surface.ts";
import { usePluginDocuments } from "./use-plugin-documents.ts";

/** Screens laid out and placed: saved positions, else flow order (STU-050). */
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
    return s === undefined || p === undefined
      ? []
      : [{ id, title: s.title, at: p, layout: layoutScreen(s.root, s.title) }];
  });
}

/** C2: the plugin canvas, read-only (S1.4). */
export function PluginCanvasPage() {
  const m = pluginCanvasMessages(useLingui().i18n);
  const params = useParams({ strict: false }) as { app?: string; plugin?: string };
  const search = useSearch({ strict: false }) as { x?: number; y?: number; z?: number };
  const navigate = useNavigate();
  const { createSurface = browserSurface } = useRouteContext({ strict: false }) as Pick<RouterContext, "createSurface">;
  const docs = usePluginDocuments(params.app ?? "", params.plugin ?? "");
  const screens = useMemo(() => (docs.data === undefined ? [] : placeModel(docs.data)), [docs.data]);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const surfaceRef = useRef<CanvasSurface | null>(null);
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
    surface.patch(sceneOps(screens));
    const width = canvas.clientWidth || window.innerWidth;
    const height = canvas.clientHeight || window.innerHeight;
    const s = searchRef.current;
    const b = contentBounds(screens);
    const zoom =
      s.z !== undefined
        ? s.z / 100
        : Math.min(
            1,
            Math.max(MIN_ZOOM_PERCENT / 100, Math.min(width / (b.r - b.l + 160), height / (b.b - b.t + 160))),
          );
    const cx = s.x ?? (b.l + b.r) / 2;
    const cy = s.y ?? (b.t + b.b) / 2;
    surface.setViewport(cx - width / 2 / zoom, cy - height / 2 / zoom, zoom);
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

  const drag = useRef<{ x: number; y: number } | null>(null);
  const percent = Math.round(Math.min(MAX_ZOOM_PERCENT, Math.max(MIN_ZOOM_PERCENT, search.z ?? 100)));

  return (
    <section className="relative h-full w-full overflow-hidden">
      {/* biome-ignore lint/a11y/noInteractiveElementToNoninteractiveRole: the pan and zoom surface; its content is the screen list below */}
      <canvas
        role="application"
        aria-label={m.title}
        ref={canvasRef}
        className="absolute inset-0 h-full w-full touch-none"
        onPointerDown={(e) => {
          drag.current = { x: e.clientX, y: e.clientY };
        }}
        onPointerMove={(e) => {
          const d = drag.current;
          if (d !== null) {
            surfaceRef.current?.pan(e.clientX - d.x, e.clientY - d.y);
            drag.current = { x: e.clientX, y: e.clientY };
          }
        }}
        onPointerUp={() => {
          drag.current = null;
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
      <ol aria-label={m.screens} className="sr-only">
        {screens.map((s) => (
          <li key={s.id}>
            {s.title}
            {s.layout.contentHeight > DEVICE.height && <span id={overflowId(s.id)}> {m.overflow(s.title)}</span>}
          </li>
        ))}
      </ol>
      <p className="absolute end-4 bottom-4 text-caption text-secondary">{m.zoom(percent)}</p>
    </section>
  );
}
