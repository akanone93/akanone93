// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { ArrowPick, Guides } from "@plux/canvas";
import { type RefObject, useRef } from "react";
import type { CanvasSurface } from "../plugin-canvas/surface.ts";
import { BOX, type Point, select, snap } from "./app-canvas.model.ts";
import { boxOfHits, boxOfItem, type SceneBox } from "./app-canvas.scene.ts";

/** Movement in CSS pixels below which a press is a click, not a drag or a marquee. */
const CLICK_PX = 3;

export interface GestureInput {
  readonly surface: () => CanvasSurface | null;
  readonly canvas: RefObject<HTMLCanvasElement | null>;
  readonly boxes: readonly SceneBox[];
  readonly selected: ReadonlySet<string>;
  readonly setSelected: (next: ReadonlySet<string>) => void;
  /** Live positions while dragging; nothing is saved until `commit`. */
  readonly setPositions: (update: (prev: ReadonlyMap<string, Point>) => ReadonlyMap<string, Point>) => void;
  readonly commit: (positions: ReadonlyMap<string, Point>) => void;
  readonly setHover: (id: string | undefined) => void;
  readonly expand: (bundle: string | null) => void;
  readonly pick: (hits: readonly string[]) => ArrowPick | undefined;
  readonly onLane: (pick: ArrowPick) => void;
  readonly onOpen: (box: SceneBox) => void;
  readonly onView: () => void;
}

interface Drag {
  readonly id: string;
  readonly start: Point;
  readonly origin: ReadonlyMap<string, Point>;
  moved: boolean;
}

interface Press {
  readonly from: Point;
  to: Point;
  readonly additive: boolean;
}

const rectOf = (a: Point, b: Point) => ({
  l: Math.min(a.x, b.x),
  t: Math.min(a.y, b.y),
  r: Math.max(a.x, b.x),
  b: Math.max(a.y, b.y),
});

/** The guide lines of a snap, spanning every box, for the canvas overlay. */
function guideLines(guides: readonly { axis: "x" | "y"; at: number }[], boxes: readonly SceneBox[]): Guides {
  const top = Math.min(...boxes.map((b) => b.y)) - BOX.height;
  const bottom = Math.max(...boxes.map((b) => b.y)) + 2 * BOX.height;
  const left = Math.min(...boxes.map((b) => b.x)) - BOX.width;
  const right = Math.max(...boxes.map((b) => b.x)) + 2 * BOX.width;
  return {
    lines: guides.map((g) =>
      g.axis === "x" ? { axis: "x", at: g.at, from: top, to: bottom } : { axis: "y", at: g.at, from: left, to: right },
    ),
    labels: [],
  };
}

/**
 * Pointer handling of the app canvas: the main-thread hit index finds the box or arrow under the
 * pointer (ADR-0013), a drag moves the selection with snapping and overlay guides, a press on the
 * background draws a marquee in the overlay, a click on an arrow expands it or opens a lane, and the
 * wheel pans (with Ctrl or Cmd: zooms). The worker draws all of it.
 */
export function useCanvasGestures(input: GestureInput) {
  const drag = useRef<Drag | null>(null);
  const press = useRef<Press | null>(null);
  const local = (e: { clientX: number; clientY: number }): Point => {
    const r = input.canvas.current?.getBoundingClientRect();
    return { x: e.clientX - (r?.left ?? 0), y: e.clientY - (r?.top ?? 0) };
  };
  const hitsAt = (e: { clientX: number; clientY: number }): string[] => {
    const p = local(e);
    return input.surface()?.hitTest?.(p.x, p.y) ?? [];
  };

  /** Selects `box` as a press does; returns the selection the drag carries. */
  const pressBox = (box: SceneBox, additive: boolean): ReadonlySet<string> => {
    const next = input.selected.has(box.id) && !additive ? input.selected : select(input.selected, box.id, additive);
    input.setSelected(next);
    return next;
  };

  const startDrag = (e: React.PointerEvent, box: SceneBox) => {
    const next = pressBox(box, e.shiftKey || e.metaKey || e.ctrlKey);
    drag.current = {
      id: box.id,
      start: { x: e.clientX, y: e.clientY },
      origin: new Map(input.boxes.filter((b) => next.has(b.id)).map((b) => [b.id, { x: b.x, y: b.y }])),
      moved: false,
    };
  };

  const moveDrag = (e: React.PointerEvent, d: Drag) => {
    const zoom = input.surface()?.viewport.zoom ?? 1;
    const dx = (e.clientX - d.start.x) / zoom;
    const dy = (e.clientY - d.start.y) / zoom;
    const lead = d.origin.get(d.id);
    if (lead === undefined || (!d.moved && Math.abs(dx) + Math.abs(dy) < CLICK_PX)) {
      return;
    }
    d.moved = true;
    const others = input.boxes.filter((b) => !d.origin.has(b.id));
    const s = snap({ x: lead.x + dx, y: lead.y + dy }, others);
    input.surface()?.setGuides?.(s.guides.length === 0 ? null : guideLines(s.guides, input.boxes));
    const ox = s.point.x - lead.x;
    const oy = s.point.y - lead.y;
    input.setPositions((prev) => {
      const next = new Map(prev);
      for (const b of input.boxes) {
        next.set(b.id, next.get(b.id) ?? { x: b.x, y: b.y });
      }
      for (const [id, o] of d.origin) {
        next.set(id, { x: o.x + ox, y: o.y + oy });
      }
      return next;
    });
  };

  const moveMarquee = (e: React.PointerEvent, p: Press) => {
    p.to = local(e);
    const surface = input.surface();
    if (surface === null) {
      return;
    }
    const v = surface.viewport;
    const r = rectOf(p.from, p.to);
    surface.setGuides?.({
      lines: [],
      labels: [],
      indicator: { l: v.x + r.l / v.zoom, t: v.y + r.t / v.zoom, r: v.x + r.r / v.zoom, b: v.y + r.b / v.zoom },
    });
  };

  const endPress = (e: React.PointerEvent, p: Press) => {
    const surface = input.surface();
    surface?.setGuides?.(null);
    const r = rectOf(p.from, local(e));
    if (r.r - r.l + (r.b - r.t) >= CLICK_PX) {
      const hit = (surface?.hitTestRect?.(r, "intersect") ?? []).flatMap((h) => boxOfItem(h) ?? []);
      input.setSelected(new Set([...(p.additive ? input.selected : []), ...hit]));
      return;
    }
    const picked = input.pick(hitsAt(e));
    if (picked?.link !== undefined) {
      input.onLane(picked);
    } else if (picked !== undefined) {
      input.expand(picked.bundle.id);
    } else if (!p.additive) {
      input.setSelected(new Set());
    }
  };

  const handlers = {
    onPointerDown: (e: React.PointerEvent<HTMLCanvasElement>) => {
      if (e.button !== 0) {
        return;
      }
      e.currentTarget.setPointerCapture?.(e.pointerId);
      const id = boxOfHits(hitsAt(e));
      const box = input.boxes.find((b) => b.id === id);
      if (box !== undefined) {
        startDrag(e, box);
      } else {
        const at = local(e);
        press.current = { from: at, to: at, additive: e.shiftKey || e.metaKey || e.ctrlKey };
      }
    },
    onPointerMove: (e: React.PointerEvent<HTMLCanvasElement>) => {
      if (drag.current !== null) {
        moveDrag(e, drag.current);
      } else if (press.current !== null) {
        moveMarquee(e, press.current);
      } else {
        const hits = hitsAt(e);
        input.setHover(boxOfHits(hits));
        input.expand(input.pick(hits)?.bundle.id ?? null);
      }
    },
    onPointerUp: (e: React.PointerEvent<HTMLCanvasElement>) => {
      const d = drag.current;
      const p = press.current;
      drag.current = null;
      press.current = null;
      if (d !== null) {
        input.surface()?.setGuides?.(null);
        if (d.moved) {
          input.commit(new Map(input.boxes.map((b) => [b.id, { x: b.x, y: b.y }])));
        }
      } else if (p !== null) {
        endPress(e, p);
      }
    },
    onPointerLeave: () => {
      if (drag.current === null && press.current === null) {
        input.setHover(undefined);
        input.expand(null);
      }
    },
    onDoubleClick: (e: React.MouseEvent<HTMLCanvasElement>) => {
      const id = boxOfHits(hitsAt(e));
      const box = input.boxes.find((b) => b.id === id);
      if (box !== undefined) {
        input.onOpen(box);
      }
    },
    onWheel: (e: React.WheelEvent<HTMLCanvasElement>) => {
      const surface = input.surface();
      if (e.ctrlKey || e.metaKey) {
        const p = local(e);
        surface?.zoomAbout(Math.exp(-e.deltaY / 200), p.x, p.y);
      } else {
        surface?.pan(-e.deltaX, -e.deltaY);
      }
      input.onView();
    },
  };
  return { handlers, pressBox };
}
