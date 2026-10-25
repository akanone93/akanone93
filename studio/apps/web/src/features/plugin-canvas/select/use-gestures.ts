// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useRef } from "react";
import type { CanvasSurface } from "../surface.ts";
import type { EditFn } from "./edits.ts";
import { HANDLE_ZONE } from "./geometry.ts";
import { finishMove, finishResize, type GestureEnv, planMove, reparentPreview } from "./gesture.ts";
import type { SelectionStore } from "./store.ts";
import { itemIdOf, type NodeIndex } from "./tree.ts";

interface Drag {
  readonly kind: "move" | "resize";
  readonly id: string;
  readonly from: { x: number; y: number };
}

interface Args {
  readonly surface: () => CanvasSurface | null;
  readonly canvas: () => HTMLCanvasElement | null;
  readonly store: SelectionStore;
  readonly index: NodeIndex;
  readonly edit: EditFn;
  readonly screenOf: ReadonlyMap<string, string>;
}

interface PointerLike {
  readonly clientX: number;
  readonly clientY: number;
}

/** Drag-to-move, resize and reparent on the selection; feedback is drawn on the overlay only. */
export function useGestures(a: Args) {
  const drag = useRef<Drag | null>(null);

  const world = (e: PointerLike): { x: number; y: number } | null => {
    const s = a.surface();
    const box = a.canvas()?.getBoundingClientRect();
    if (s === null || box === undefined) {
      return null;
    }
    return {
      x: s.viewport.x + (e.clientX - box.left) / s.viewport.zoom,
      y: s.viewport.y + (e.clientY - box.top) / s.viewport.zoom,
    };
  };
  const boxOf = (id: string) => {
    const screen = a.screenOf.get(id);
    return screen === undefined ? undefined : a.surface()?.boundsOf?.(itemIdOf(screen, id));
  };
  const env = (): GestureEnv => ({
    index: a.index,
    edit: a.edit,
    docOf: (id) => a.screenOf.get(id),
    boxOf,
    typeOf: (id) => a.index.type.get(id),
  });
  const others = (id: string) =>
    [...(a.index.children.get(a.index.parent.get(id) ?? "") ?? [])]
      .filter((k) => k !== id)
      .flatMap((k) => boxOf(k) ?? []);

  return {
    /** Starts a move or resize when the pointer is on the selection; returns true when it did. */
    down(e: PointerLike): boolean {
      const id = a.store.get().anchor;
      const w = world(e);
      const box = id === null ? undefined : boxOf(id);
      if (id === null || w === null || box === undefined) {
        return false;
      }
      const zoom = a.surface()?.viewport.zoom ?? 1;
      const zone = HANDLE_ZONE / zoom;
      const onHandle = Math.abs(w.x - box.r) <= zone && Math.abs(w.y - box.b) <= zone;
      const inside = w.x >= box.l && w.x <= box.r && w.y >= box.t && w.y <= box.b;
      if (!onHandle && !inside) {
        return false;
      }
      drag.current = { kind: onHandle ? "resize" : "move", id, from: w };
      return true;
    },
    move(e: PointerLike): boolean {
      const d = drag.current;
      const w = world(e);
      const box = d === null ? undefined : boxOf(d.id);
      if (d === null || w === null || box === undefined) {
        return false;
      }
      if (d.kind === "move") {
        const plan = planMove(
          box,
          others(d.id),
          { x: w.x - d.from.x, y: w.y - d.from.y },
          a.surface()?.viewport.zoom ?? 1,
        );
        const drop = reparentPreview(env(), [d.id], w.x, w.y);
        a.surface()?.setGuides?.(drop ?? plan.guides);
      }
      return true;
    },
    up(e: PointerLike): boolean {
      const d = drag.current;
      drag.current = null;
      const w = world(e);
      a.surface()?.setGuides?.(null);
      if (d === null || w === null) {
        return false;
      }
      const delta = { x: w.x - d.from.x, y: w.y - d.from.y };
      if (Math.hypot(delta.x, delta.y) < 1) {
        return false;
      }
      if (d.kind === "move") {
        finishMove(env(), d.id, delta, w.x, w.y);
      } else {
        finishResize(env(), d.id, delta);
      }
      return true;
    },
  };
}
