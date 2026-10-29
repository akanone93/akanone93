// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import {
  type ArrowBox,
  ArrowCanvas,
  type ArrowPick,
  ArrowRouter,
  type Bundle,
  deriveAppBundles,
  EXPAND_MS,
  type ItemSpec,
  type SceneOp,
} from "@plux/canvas";
import { useCallback, useEffect, useMemo, useRef } from "react";
import { SceneSync } from "../plugin-canvas/ghosts.ts";
import { arrowStyle } from "../plugin-canvas/scene.ts";
import type { CanvasSurface } from "../plugin-canvas/surface.ts";
import { BOX, type Frame } from "./app-canvas.model.ts";
import { type AppGraph, boxItem, boxItems, focusItem, frameItems, type SceneBox } from "./app-canvas.scene.ts";

interface Drawn {
  readonly surface: CanvasSurface;
  readonly sync: SceneSync;
  readonly router: ArrowRouter;
  readonly arrows: ArrowCanvas;
  /** The boxes and bundles the router was last given; a change of either re-routes everything. */
  graph: string;
  placed: Map<string, { x: number; y: number }>;
  focus: string | null;
  frame: number;
  clock: number;
}

export interface AppSceneInput {
  readonly surface: () => CanvasSurface | null;
  /** Bumped when a new surface exists, so the scene is drawn onto it. */
  readonly generation: number;
  readonly boxes: readonly SceneBox[];
  readonly frames: readonly Frame[];
  readonly selected: ReadonlySet<string>;
  /** The box the accessible list has focus on; drawn with a focus ring. */
  readonly focused: string | undefined;
  readonly graph: AppGraph;
}

const reducedMotion = (): boolean =>
  typeof window !== "undefined" && window.matchMedia?.("(prefers-reduced-motion: reduce)").matches === true;

/**
 * Draws the app canvas as Plux Canvas scene patches (ADR-0013 as revised, A4.19, A5.1): boxes, frames
 * and the focus ring through one SceneSync, the bundled arrows through ArrowCanvas (STU-040 to
 * STU-048). The worker draws; the main thread only diffs item specs and answers hits.
 */
export function useAppScene(input: AppSceneInput) {
  const { surface, generation, boxes, frames, selected, focused, graph } = input;
  const bundles = useMemo<Bundle[]>(
    () =>
      deriveAppBundles(
        graph.links.map((l) => l.edge),
        graph.boxOf,
      ),
    [graph],
  );
  const drawn = useRef<Drawn | null>(null);

  const current = useCallback((): Drawn | null => {
    const target = surface();
    if (target === null || target.patch === undefined) {
      return null;
    }
    if (drawn.current?.surface !== target) {
      const router = new ArrowRouter([], []);
      drawn.current = {
        surface: target,
        sync: new SceneSync(),
        router,
        arrows: new ArrowCanvas(router, arrowStyle, target.viewport.zoom),
        graph: "",
        placed: new Map(),
        focus: null,
        frame: 0,
        clock: 0,
      };
    }
    return drawn.current;
  }, [surface]);

  // biome-ignore lint/correctness/useExhaustiveDependencies: generation redraws onto a new surface
  useEffect(() => {
    const d = current();
    if (d === null) {
      return;
    }
    const items: ItemSpec[] = [...frames.flatMap((f) => frameItems(f)), ...boxes.flatMap((b) => boxItems(b))];
    const ring = boxes.find((b) => b.id === focused);
    if (ring !== undefined) {
      items.push(focusItem(ring));
    }
    const ops: SceneOp[] = [...d.sync.sync(items)];
    ops.push(...arrowOps(d, boxes, bundles));
    d.placed = new Map(boxes.map((b) => [b.id, { x: b.x, y: b.y }]));
    if (ops.length > 0) {
      d.surface.patch?.(ops);
    }
    d.surface.setSelection?.([...selected].filter((id) => d.placed.has(id)).map(boxItem));
  }, [current, generation, boxes, frames, bundles, focused, selected]);

  useEffect(() => () => cancelAnimationFrame(drawn.current?.frame ?? 0), []);

  /** Expands bundle `id` (hover, keyboard focus or click) or collapses all; animates in the worker. */
  const expand = useCallback(
    (id: string | null) => {
      const d = current();
      if (d === null || d.focus === id) {
        return;
      }
      d.focus = id;
      const t = now(d);
      const ops = d.arrows.setFocus(id, t);
      if (reducedMotion()) {
        d.clock = t + EXPAND_MS;
        ops.push(...d.arrows.frame(d.clock));
      }
      if (ops.length > 0) {
        d.surface.patch?.(ops);
      }
      cancelAnimationFrame(d.frame);
      const step = () => {
        const at = now(d);
        const more = d.arrows.frame(at);
        if (more.length > 0) {
          d.surface.patch?.(more);
        }
        if (d.arrows.animating(at)) {
          d.frame = requestAnimationFrame(step);
        }
      };
      if (d.arrows.animating(t)) {
        d.frame = requestAnimationFrame(step);
      }
    },
    [current],
  );

  /** Redraws the arrows for the zoom (level of detail, STU-048). */
  const zoomed = useCallback(
    (zoom: number) => {
      const d = current();
      const ops = d?.arrows.setZoom(zoom, now(d)) ?? [];
      if (ops.length > 0) {
        d?.surface.patch?.(ops);
      }
    },
    [current],
  );

  const pick = useCallback((hits: readonly string[]): ArrowPick | undefined => drawn.current?.arrows.pick(hits), []);
  return { bundles, expand, zoomed, pick };
}

/** Arrow patches: a full re-route when the boxes or bundles changed, else only the moved boxes' bundles. */
function arrowOps(d: Drawn, boxes: readonly SceneBox[], bundles: readonly Bundle[]): SceneOp[] {
  const ids = boxes.map((b) => b.id).join("\n");
  const key = `${ids}\n\n${bundles.map((b) => `${b.id}:${b.links.map((l) => l.id).join(",")}`).join("\n")}`;
  if (key !== d.graph) {
    const arrowBoxes: ArrowBox[] = boxes.map((b) => ({ id: b.id, x: b.x, y: b.y, w: BOX.width, h: BOX.height }));
    d.router.setGraph(arrowBoxes, bundles);
    d.graph = key;
    return d.arrows.render(now(d));
  }
  const ops: SceneOp[] = [];
  for (const b of boxes) {
    const was = d.placed.get(b.id);
    if (was === undefined || was.x !== b.x || was.y !== b.y) {
      ops.push(...d.arrows.moveBox(b.id, b.x, b.y, now(d)));
    }
  }
  return ops;
}

/** A clock that never runs backwards, also after a reduced-motion jump to the end of an animation. */
function now(d: Drawn | null): number {
  if (d === null) {
    return 0;
  }
  d.clock = Math.max(d.clock, typeof performance === "undefined" ? 0 : performance.now());
  return d.clock;
}
