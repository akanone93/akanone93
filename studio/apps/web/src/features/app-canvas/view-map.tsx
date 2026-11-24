// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { type ReactElement, useEffect, useRef, useState } from "react";
import { useStoredFlag } from "../panels/dock-tabs.tsx";
import type { CanvasSurface } from "../plugin-canvas/surface.ts";
import { Panel, ToolButton } from "./canvas-chrome.tsx";
import { fromMap, type MapFrame, mapFrame, onMap, panTo, type Rect, viewRect } from "./view-map.ts";
import "./view-map.css";

/** The map's size in CSS pixels. */
const MAP = { w: 184, h: 116 } as const;

/** A part of the canvas the map draws: a screen, a plugin box, a ghost screen. */
export interface MapItem extends Rect {
  readonly id: string;
  readonly kind?: "ghost" | "frame";
}

export interface ViewMapLabels {
  /** The map's accessible name, with how to use it. */
  readonly label: string;
  readonly hide: string;
  readonly show: string;
}

interface Seen {
  readonly x: number;
  readonly y: number;
  readonly zoom: number;
  readonly w: number;
  readonly h: number;
}

/**
 * The canvas's size as it changes: from a ResizeObserver, so following the view reads no layout. Reading the element's
 * size once a frame forced the browser to lay out the page at every frame of a pan or a zoom (#214).
 */
function useCanvasSize(canvas: () => HTMLElement | null, on: boolean): { readonly current: Size } {
  const size = useRef<Size>({ w: 0, h: 0 });
  const latest = useRef(canvas);
  latest.current = canvas;
  useEffect(() => {
    const el = latest.current();
    if (!on || el === null) {
      return undefined;
    }
    size.current = { w: el.clientWidth, h: el.clientHeight };
    if (typeof ResizeObserver === "undefined") {
      return undefined;
    }
    const observer = new ResizeObserver(() => {
      size.current = { w: el.clientWidth, h: el.clientHeight };
    });
    observer.observe(el);
    return () => observer.disconnect();
  }, [on]);
  return size;
}

interface Size {
  readonly w: number;
  readonly h: number;
}

/** The view and the canvas's size, compared once a frame and kept only when they change. */
function useSeen(surface: () => CanvasSurface | null, canvas: () => HTMLElement | null, on: boolean): Seen | null {
  const [seen, setSeen] = useState<Seen | null>(null);
  const latest = useRef(surface);
  latest.current = surface;
  const size = useCanvasSize(canvas, on);
  useEffect(() => {
    if (!on || typeof requestAnimationFrame === "undefined") {
      return undefined;
    }
    let last = "";
    let frame = 0;
    const tick = () => {
      const v = latest.current()?.viewport;
      const { w, h } = size.current;
      if (v !== undefined && w > 0) {
        const key = `${v.x}:${v.y}:${v.zoom}:${w}:${h}`;
        if (key !== last) {
          last = key;
          setSeen({ x: v.x, y: v.y, zoom: v.zoom, w, h });
        }
      }
      frame = requestAnimationFrame(tick);
    };
    frame = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(frame);
  }, [on, size]);
  return seen;
}

/**
 * The view map above the zoom (C5): every screen or box of the canvas in small, and a frame on the part the view shows.
 * Dragging the frame moves the view; a press elsewhere on the map brings the view there first. The arrow keys move the
 * view by a tenth of its size (Shift: half). It can be folded to a button, and stays as the person left it.
 */
export function ViewMap(props: {
  readonly surface: () => CanvasSurface | null;
  readonly canvas: () => HTMLElement | null;
  readonly items: readonly MapItem[];
  readonly labels: ViewMapLabels;
  /** The view moved: the page follows it (the URL, the overlay). */
  readonly onView: () => void;
}): ReactElement {
  const [open, setOpen] = useStoredFlag("plux.canvas.view-map", true);
  const seen = useSeen(props.surface, props.canvas, open);
  // While a drag runs the map keeps its frame: a frame refitted under the pointer would run away from it.
  const drag = useRef<{ frame: MapFrame; grab: { x: number; y: number } } | null>(null);
  const live = seen === null ? null : viewRect(seen, seen);
  const frame = drag.current?.frame ?? (live === null ? null : mapFrame(props.items, live, MAP));

  const moveTo = (centre: { x: number; y: number }) => {
    const surface = props.surface();
    if (surface === null || seen === null) {
      return;
    }
    const { dx, dy } = panTo(surface.viewport, seen, centre);
    if (dx !== 0 || dy !== 0) {
      surface.pan(dx, dy);
      props.onView();
    }
  };
  const mapPoint = (e: React.PointerEvent<HTMLButtonElement>) => {
    const box = e.currentTarget.getBoundingClientRect();
    return { x: e.clientX - box.left, y: e.clientY - box.top };
  };

  if (!open) {
    return (
      <Panel className="canvas-bar view-map-folded p-1.5">
        <ToolButton icon="frame" label={props.labels.show} onClick={() => setOpen(true)} />
      </Panel>
    );
  }
  return (
    <Panel className="view-map p-1.5">
      {/* A button: it takes the pointer and the arrow keys, and is reached by Tab like the zoom controls under it. */}
      <button
        type="button"
        aria-label={props.labels.label}
        className="view-map-area"
        style={{ inlineSize: MAP.w, blockSize: MAP.h }}
        onPointerDown={(e) => {
          if (frame === null || live === null || e.button !== 0) {
            return;
          }
          e.currentTarget.setPointerCapture?.(e.pointerId);
          const at = fromMap(frame, mapPoint(e));
          const inside = at.x >= live.l && at.x <= live.r && at.y >= live.t && at.y <= live.b;
          const centre = { x: (live.l + live.r) / 2, y: (live.t + live.b) / 2 };
          drag.current = { frame, grab: inside ? { x: at.x - centre.x, y: at.y - centre.y } : { x: 0, y: 0 } };
          if (!inside) {
            moveTo(at);
          }
        }}
        onPointerMove={(e) => {
          const d = drag.current;
          if (d !== null) {
            const at = fromMap(d.frame, mapPoint(e));
            moveTo({ x: at.x - d.grab.x, y: at.y - d.grab.y });
          }
        }}
        onPointerUp={() => {
          drag.current = null;
        }}
        onLostPointerCapture={() => {
          drag.current = null;
        }}
        onKeyDown={(e) => {
          const step = e.shiftKey ? 0.5 : 0.1;
          const move = { ArrowLeft: [-1, 0], ArrowRight: [1, 0], ArrowUp: [0, -1], ArrowDown: [0, 1] }[e.key];
          if (move === undefined || live === null) {
            return;
          }
          e.preventDefault();
          const w = live.r - live.l;
          const h = live.b - live.t;
          moveTo({
            x: (live.l + live.r) / 2 + (move[0] ?? 0) * w * step,
            y: (live.t + live.b) / 2 + (move[1] ?? 0) * h * step,
          });
        }}
      >
        {frame !== null && live !== null && (
          <svg width={MAP.w} height={MAP.h} aria-hidden="true">
            {props.items.map((item) => {
              const r = onMap(frame, item);
              return (
                <rect
                  key={item.id}
                  className="view-map-item"
                  data-kind={item.kind}
                  x={r.x}
                  y={r.y}
                  width={Math.max(1.5, r.w)}
                  height={Math.max(1.5, r.h)}
                  rx={Math.min(3, r.w / 6)}
                />
              );
            })}
            {(() => {
              const r = onMap(frame, live);
              return <rect className="view-map-view" x={r.x} y={r.y} width={r.w} height={r.h} rx={2} />;
            })()}
          </svg>
        )}
      </button>
      <span className="view-map-fold">
        <ToolButton icon="caret-down" label={props.labels.hide} onClick={() => setOpen(false)} />
      </span>
    </Panel>
  );
}
