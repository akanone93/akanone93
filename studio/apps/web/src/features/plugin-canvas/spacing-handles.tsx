// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { type CSSProperties, type PointerEvent, type ReactNode, useRef } from "react";
import { pluginCanvasMessages } from "./plugin-canvas.messages.ts";
import type { Node } from "./plugin-model.ts";
import type { EditFn } from "./select/edits.ts";
import type { Box } from "./select/geometry.ts";
import { type OverlayStore, useOverlayState } from "./select/overlay-store.ts";
import type { Selection } from "./select/selection.ts";
import type { NodeIndex } from "./select/tree.ts";
import {
  draggedSides,
  dragsGap,
  dragsPadding,
  gapRecipe,
  paddingRecipe,
  pixels,
  readSides,
  type Side,
  snapSpacing,
} from "./spacing-handles.ts";
import type { CanvasSurface } from "./surface.ts";
import "./spacing-handles.css";

/** One handle: what it drags, where it stands (world units) and along which axis it moves. */
type Handle =
  | { readonly kind: "padding"; readonly side: Side; readonly x: number; readonly y: number }
  | { readonly kind: "gap"; readonly axis: "x" | "y"; readonly x: number; readonly y: number; readonly n: number };

/** Three quarters along a span: a handle stands beside its band's value, not over it. */
const along = (from: number, to: number): number => from + (to - from) * 0.75;

/** The padding handles of a box around one child: each in its band, beside the band's value. */
function paddingHandles(box: Box, kid: Box): Handle[] {
  const cx = along(kid.l, kid.r);
  const cy = along(kid.t, kid.b);
  return [
    { kind: "padding", side: "top", x: cx, y: (box.t + kid.t) / 2 },
    { kind: "padding", side: "bottom", x: cx, y: (kid.b + box.b) / 2 },
    { kind: "padding", side: "left", x: (box.l + kid.l) / 2, y: cy },
    { kind: "padding", side: "right", x: (kid.r + box.r) / 2, y: cy },
  ];
}

/** The gap handles between children in a row or a column, at the middle of each gap. */
function gapHandles(kids: readonly Box[]): Handle[] {
  const byY = [...kids].sort((a, b) => a.t - b.t);
  const byX = [...kids].sort((a, b) => a.l - b.l);
  const inLine = (list: Box[], axis: "x" | "y") =>
    list.every((k, i) => {
      const p = list[i - 1];
      return p === undefined || (axis === "y" ? k.t >= p.b - 0.5 : k.l >= p.r - 0.5);
    });
  const axis = inLine(byY, "y") ? "y" : inLine(byX, "x") ? "x" : undefined;
  if (axis === undefined) {
    return [];
  }
  const list = axis === "y" ? byY : byX;
  return list.slice(1).map((k, i) => {
    const p = list[i] as Box;
    return axis === "y"
      ? { kind: "gap", axis, n: i, x: along(Math.max(p.l, k.l), Math.min(p.r, k.r)), y: (p.b + k.t) / 2 }
      : { kind: "gap", axis, n: i, x: (p.r + k.l) / 2, y: along(Math.max(p.t, k.t), Math.min(p.b, k.b)) };
  });
}

/** A drag in progress: where it started and the values it started from. */
interface Drag {
  readonly handle: Handle;
  readonly x: number;
  readonly y: number;
  readonly start: Record<Side, number> | number;
  readonly directional: boolean;
  readonly serial: number;
  frame: number;
}

/**
 * Padding and gap handles of the selected widget (PC-18): a short bar on each side of its child, or in each gap
 * between its children. Dragging one changes the padding of that side (Alt: both sides of the axis, Shift: all four)
 * or the gap, live, as one undo step; the value snaps to a spacing token near it.
 */
export function SpacingHandles(props: {
  readonly store: OverlayStore;
  readonly surface: () => CanvasSurface | null;
  readonly selection: Selection;
  readonly index: NodeIndex;
  readonly boxOf: (id: string) => Box | undefined;
  readonly nodeOf: (id: string) => Node | undefined;
  readonly docOf: (id: string) => string | undefined;
  readonly edit: EditFn;
  readonly canEdit: boolean;
  readonly locked: ReadonlySet<string>;
  readonly dims: Readonly<Record<string, number>>;
  readonly rtl: boolean;
}): ReactNode {
  const state = useOverlayState(props.store);
  const m = pluginCanvasMessages(useLingui().i18n);
  const drag = useRef<Drag | null>(null);
  const serial = useRef(0);
  const surface = props.surface();
  const id = props.selection.ids.length === 1 ? props.selection.ids[0] : undefined;
  const type = id === undefined ? undefined : props.index.type.get(id);
  const box = id === undefined ? undefined : props.boxOf(id);
  const busy = state.ghost !== null || state.marquee !== null;
  if (surface === null || id === undefined || box === undefined || busy || !props.canEdit || props.locked.has(id)) {
    return null;
  }
  const kids = (props.index.children.get(id) ?? []).flatMap((k) => props.boxOf(k) ?? []);
  const handles =
    dragsPadding(type) && kids.length === 1
      ? paddingHandles(box, kids[0] as Box)
      : dragsGap(type) && kids.length > 1
        ? gapHandles(kids)
        : [];
  if (handles.length === 0) {
    return null;
  }
  const v = surface.viewport;
  const at = (h: Handle): CSSProperties =>
    ({ "--x": (h.x - v.x) * v.zoom, "--y": (h.y - v.y) * v.zoom }) as CSSProperties;

  const begin = (h: Handle, e: PointerEvent<HTMLSpanElement>) => {
    const props0 = (props.nodeOf(id)?.props ?? {}) as Record<string, unknown>;
    const read = readSides(props0["padding"], props.rtl);
    const start =
      h.kind === "padding"
        ? (Object.fromEntries(Object.entries(read.sides).map(([k, s]) => [k, pixels(s, props.dims)])) as Record<
            Side,
            number
          >)
        : pixels(props0["spacing"], props.dims);
    serial.current += 1;
    drag.current = {
      handle: h,
      x: e.clientX,
      y: e.clientY,
      start,
      directional: read.directional,
      serial: serial.current,
      frame: 0,
    };
    e.currentTarget.setPointerCapture(e.pointerId);
    e.stopPropagation();
  };

  const move = (e: PointerEvent<HTMLSpanElement>) => {
    const d = drag.current;
    const doc = props.docOf(id);
    if (d === null || doc === undefined) {
      return;
    }
    const zoom = props.surface()?.viewport.zoom ?? 1;
    const dx = (e.clientX - d.x) / zoom;
    const dy = (e.clientY - d.y) / zoom;
    const reach = 4 / zoom;
    const group = `spacing:${id}:${d.serial}`;
    const recipe = (() => {
      if (d.handle.kind === "gap" || typeof d.start === "number") {
        const delta = d.handle.kind === "gap" && d.handle.axis === "x" ? dx : dy;
        return gapRecipe(id, snapSpacing((d.start as number) + delta, props.dims, reach));
      }
      const start = d.start;
      const side = d.handle.side;
      const delta = { top: dy, bottom: -dy, left: dx, right: -dx }[side];
      const next = Object.fromEntries(Object.entries(start).map(([k, s]) => [k, s])) as Record<
        Side,
        number | { $token: string }
      >;
      for (const s of draggedSides(side, e.altKey, e.shiftKey)) {
        next[s] = snapSpacing(start[side] + delta, props.dims, reach);
      }
      return paddingRecipe(id, next, d.directional, props.rtl);
    })();
    cancelAnimationFrame(d.frame);
    d.frame = requestAnimationFrame(() => props.edit(doc, recipe, { group }));
  };

  const end = (e: PointerEvent<HTMLSpanElement>) => {
    if (drag.current !== null) {
      e.currentTarget.releasePointerCapture(e.pointerId);
      drag.current = null;
    }
  };

  return (
    // A pointer affordance over the canvas, as the overlay is: the inspector's fields set the same values by keyboard.
    <div className="pc-spacing-handles" aria-hidden="true">
      {handles.map((h) => (
        <span
          key={h.kind === "padding" ? h.side : `gap${h.n}`}
          title={h.kind === "padding" ? m.paddingHandle(h.side) : m.gapHandle}
          className="pc-spacing-handle"
          data-axis={h.kind === "padding" ? (h.side === "top" || h.side === "bottom" ? "y" : "x") : h.axis}
          style={at(h)}
          onPointerDown={(e) => begin(h, e)}
          onPointerMove={move}
          onPointerUp={end}
          onPointerCancel={end}
        />
      ))}
    </div>
  );
}
