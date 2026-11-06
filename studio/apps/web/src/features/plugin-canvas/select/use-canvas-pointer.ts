// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useRef } from "react";
import { GestureTracker } from "../../app-canvas/use-canvas-gestures.ts";
import type { Point } from "../positions.ts";
import type { CanvasSurface } from "../surface.ts";
import type { EditFn } from "./edits.ts";
import { type Box, type Handle, resizeBox, snapBox, translate } from "./geometry.ts";
import { dropAt, finishMove, finishResize, type GestureEnv, planMove } from "./gesture.ts";
import { handleAt, handleCursor } from "./handles.ts";
import type { OverlayStore } from "./overlay-store.ts";
import * as sel from "./selection.ts";
import type { SelectionStore } from "./store.ts";
import { itemIdOf, lineage, type NodeIndex, nodesOfHits, screenOfItem } from "./tree.ts";

/** Movement in CSS pixels below which a press is a click, not a drag or a marquee. */
export const CLICK_PX = 3;
/** The reach of a resize handle, in CSS pixels. */
export const HANDLE_PX = 6;

/** The pointer tools: select (pick, drag, marquee) or hand (the whole canvas pans). */
/** The canvas's tools: select and edit, pan, and interact with the screens as on a device (PC-15). */
export type PointerTool = "select" | "hand" | "interact";

/** What a secondary click opened its menu on. */
export type MenuTarget = "node" | "screen" | "canvas";

/** The screens' places: read, previewed while a drag runs, saved on the drop. */
export interface ScreenPlaces {
  /** Every screen's place now, by screen id. */
  readonly all: () => ReadonlyMap<string, Point>;
  /** Shows the screens at `moved` without saving: the canvas translates them (STU-050). */
  readonly preview: (moved: ReadonlyMap<string, Point>) => void;
  /** Saves every screen's place, `moved` included, as one undoable change. */
  readonly commit: (positions: ReadonlyMap<string, Point>) => void;
}

export interface PointerInput {
  readonly surface: () => CanvasSurface | null;
  readonly canvas: () => HTMLCanvasElement | null;
  readonly store: SelectionStore;
  readonly index: NodeIndex;
  readonly edit: EditFn;
  /** The screen that holds a node. */
  readonly screenOf: ReadonlyMap<string, string>;
  /** The root node of a screen: a press on its frame or title is a press on it. */
  readonly rootOf: (screen: string) => string | undefined;
  /** The world box of a node; a root's is its device frame. */
  readonly boxOf: (id: string) => Box | undefined;
  /** Locked layers are not picked; the pointer reaches what is under them (STU-061). */
  readonly locked: ReadonlySet<string>;
  readonly canEdit: boolean;
  readonly resizable: (id: string) => boolean;
  readonly overlay: OverlayStore;
  readonly tool: PointerTool;
  readonly spaceHeld: () => boolean;
  readonly screens: ScreenPlaces;
  /** Ghost screens and lanes take a click before the nodes; true when one did. */
  readonly onPick: (hits: readonly string[]) => boolean;
  /** The view panned. */
  readonly onView: () => void;
  readonly onContextMenu: (at: Point, target: MenuTarget) => void;
  /** A double-click on a screen's title renames it. */
  readonly onRenameScreen: (screen: string) => void;
  /** A double-click that reaches a text edits it in place (PC-17). */
  readonly onEditText?: (id: string) => void;
  /** With the Interact tool, a click is the screens' (PC-15): the world point and what is under it, topmost first. */
  readonly onInteract?: (world: Point, hits: readonly string[]) => void;
}

type Mode =
  | { readonly kind: "pan"; last: Point }
  | {
      readonly kind: "press";
      readonly from: Point;
      readonly world: Point;
      readonly target: string | null;
      /** Whether the press moves a screen: on its title, or on a screen already selected; else its body draws a marquee. */
      readonly grip: boolean;
      readonly hits: readonly string[];
      /** Whether a click asks the ghosts and arrows (`onPick`): one is on top, or no node is under the pointer. */
      readonly ask: boolean;
      readonly additive: boolean;
      readonly alt: boolean;
    }
  | { readonly kind: "marquee"; readonly from: Point; to: Point; readonly additive: boolean; readonly alt: boolean }
  | { readonly kind: "node"; readonly id: string; readonly world: Point; readonly box: Box; delta: Point; last: Point }
  | {
      readonly kind: "resize";
      readonly id: string;
      readonly handle: Handle;
      readonly world: Point;
      readonly box: Box;
      delta: Point;
    }
  | {
      readonly kind: "screens";
      readonly world: Point;
      readonly origin: ReadonlyMap<string, Point>;
      moved: ReadonlyMap<string, Point>;
    };

interface PointerLike {
  readonly clientX: number;
  readonly clientY: number;
  readonly button?: number;
  readonly buttons?: number;
  readonly shiftKey: boolean;
  readonly altKey: boolean;
  readonly ctrlKey: boolean;
  readonly metaKey: boolean;
  readonly pointerId?: number;
  readonly currentTarget?: unknown;
  preventDefault?(): void;
}

const rectOf = (a: Point, b: Point): Box => ({
  l: Math.min(a.x, b.x),
  t: Math.min(a.y, b.y),
  r: Math.max(a.x, b.x),
  b: Math.max(a.y, b.y),
});

const meets = (a: Box, b: Box): boolean => a.l <= b.r && b.l <= a.r && a.t <= b.b && b.t <= a.b;
const within = (inner: Box, outer: Box): boolean =>
  inner.l >= outer.l && inner.r <= outer.r && inner.t >= outer.t && inner.b <= outer.b;

function capture(e: PointerLike): void {
  const target = e.currentTarget as { setPointerCapture?: (id: number) => void } | null | undefined;
  if (e.pointerId !== undefined) {
    try {
      target?.setPointerCapture?.(e.pointerId);
    } catch {
      // The pointer is gone already (a synthetic event): nothing to capture.
    }
  }
}

/**
 * The plugin canvas's pointer, as design tools have it (STU-050, STU-067): a press picks what a click selects (the
 * node in the current scope, the innermost with Ctrl or Cmd, toggled with Shift; a screen by its frame or title) and a
 * drag moves it — a node within or into a flow container at the insertion indicator, inside a Stack by its offset with
 * snap guides and distances, a screen with every selected screen, snapping to the others, saved on the drop. A handle
 * of the single selection resizes it; a press on the empty canvas draws a marquee (screens, or the nodes of the scope
 * when it stays inside one screen). The hand tool, Space held or the middle button pan. Hovering shows what a click
 * would select. Feedback is drawn by the page's overlay; the worker draws only the screens.
 */
export function useCanvasPointer(input: PointerInput) {
  const mode = useRef<Mode | null>(null);
  const latest = useRef(input);
  latest.current = input;
  const tracker = useRef(new GestureTracker());
  return pointerHandlers(() => latest.current, mode, tracker.current);
}

/** The handlers of `useCanvasPointer`, over the latest input; separate so tests drive them without React. */
export function pointerHandlers(
  get: () => PointerInput,
  mode: { current: Mode | null },
  tracker: GestureTracker = new GestureTracker(),
) {
  const local = (e: PointerLike): Point => {
    const r = get().canvas()?.getBoundingClientRect();
    return { x: e.clientX - (r?.left ?? 0), y: e.clientY - (r?.top ?? 0) };
  };
  const zoom = () => get().surface()?.viewport.zoom ?? 1;
  const toWorld = (p: Point): Point => {
    const v = get().surface()?.viewport ?? { x: 0, y: 0, zoom: 1 };
    return { x: v.x + p.x / v.zoom, y: v.y + p.y / v.zoom };
  };
  const rawHits = (p: Point): string[] => get().surface()?.hitTest?.(p.x, p.y) ?? [];
  const nodesAt = (hits: readonly string[]): string[] => {
    const i = get();
    return nodesOfHits(hits, i.index, i.rootOf).filter((id) => !i.locked.has(id));
  };
  const isRoot = (id: string): boolean => (get().index.depth.get(id) ?? 1) === 0;
  const env = (): GestureEnv => {
    const i = get();
    return {
      index: i.index,
      edit: i.edit,
      docOf: (id) => i.screenOf.get(id),
      boxOf: (id) => (isRoot(id) ? undefined : i.boxOf(id)),
      typeOf: (id) => i.index.type.get(id),
    };
  };

  /** The resize handle of the single selection under `w`, if the selection can be resized. */
  const handleUnder = (w: Point): { id: string; handle: Handle; box: Box } | undefined => {
    const i = get();
    const s = i.store.get();
    const id = s.ids.length === 1 ? s.anchor : null;
    const box = id === null ? undefined : i.boxOf(id);
    if (id === null || box === undefined || !i.canEdit || isRoot(id) || i.locked.has(id) || !i.resizable(id)) {
      return undefined;
    }
    const handle = handleAt(box, w, HANDLE_PX / zoom());
    return handle === undefined ? undefined : { id, handle, box };
  };

  /**
   * Picks on a press. A plain press inside the selection keeps it, so a drag carries it (a selected screen gives way to
   * the node a click selects in it); otherwise it selects as a click does. Returns what the press drags, or null when
   * a Shift press took it out of the selection.
   */
  const pick = (e: PointerLike, nodes: readonly string[]): string | null => {
    const i = get();
    const s = i.store.get();
    const deep = e.ctrlKey || e.metaKey;
    const target = sel.resolveHit(i.index, nodes, s.scope, deep);
    if (!deep && !e.shiftKey && target !== null) {
      const held = lineage(i.index, target).find((c) => s.ids.includes(c));
      if (held !== undefined && (held === target || !isRoot(held))) {
        return held;
      }
    }
    const next = sel.click(i.index, s, nodes, { toggle: e.shiftKey, deep });
    i.store.set(next);
    return target !== null && next.ids.includes(target) ? target : null;
  };

  const startPan = (e: PointerLike): void => {
    e.preventDefault?.();
    capture(e);
    mode.current = { kind: "pan", last: { x: e.clientX, y: e.clientY } };
    get().canvas()?.setAttribute("data-panning", "true");
    const surface = get().surface();
    if (surface !== null) {
      tracker.begin(surface, "pan");
    }
  };

  /** A press of the select tool: a handle of the selection resizes it, anything else picks and waits for a drag. */
  const pressAt = (e: PointerLike, p: Point): Mode => {
    const i = get();
    const w = toWorld(p);
    const handle = handleUnder(w);
    if (handle !== undefined) {
      return { kind: "resize", ...handle, world: w, delta: { x: 0, y: 0 } };
    }
    const hits = rawHits(p);
    // A ghost screen or an arrow on top takes the click (`onPick`) before the screens under it.
    const top = hits[0];
    const foreign = top !== undefined && nodesOfHits([top], i.index, i.rootOf).length === 0;
    const nodes = foreign ? [] : nodesAt(hits);
    const before = i.store.get().ids;
    const target = nodes.length === 0 ? null : pick(e, nodes);
    const title = hits.some((h) => h.startsWith("title:"));
    return {
      kind: "press",
      from: p,
      world: w,
      target,
      grip: target !== null && isRoot(target) && (title || before.includes(target)),
      hits,
      ask: hits.length > 0 && nodes.length === 0,
      additive: e.shiftKey || e.ctrlKey || e.metaKey,
      alt: e.altKey,
    };
  };

  const down = (e: PointerLike): void => {
    const i = get();
    const button = e.button ?? 0;
    if (button === 1 || (button === 0 && (i.tool === "hand" || i.spaceHeld()))) {
      startPan(e);
      return;
    }
    if (button === 0 && i.tool === "interact") {
      // A live screen takes the click; nothing is selected or moved.
      const p = local(e);
      i.onInteract?.(toWorld(p), rawHits(p));
      return;
    }
    if (button === 0) {
      capture(e);
      mode.current = pressAt(e, local(e));
    }
  };

  /** A drag of a screen: every selected screen moves with it, from where each stands. */
  const startScreens = (target: string, world: Point): Mode => {
    const i = get();
    const all = i.screens.all();
    const roots = i.store.get().ids.filter(isRoot);
    const ids = (roots.includes(target) ? roots : [target]).flatMap((r) => i.screenOf.get(r) ?? []);
    const origin = new Map(ids.flatMap((s) => (all.has(s) ? [[s, all.get(s) as Point] as const] : [])));
    return { kind: "screens", world, origin, moved: origin };
  };

  /** A press that moved past the click distance becomes a drag of what it pressed, or a marquee. */
  const startDrag = (m: Extract<Mode, { kind: "press" }>, p: Point): Mode | null => {
    const i = get();
    // The empty canvas, or the body of a screen not yet selected, draws a marquee: a screen moves by its title.
    if (m.target === null || (isRoot(m.target) && !m.grip)) {
      return m.target !== null || nodesAt(m.hits).length === 0
        ? { kind: "marquee", from: m.from, to: p, additive: m.additive, alt: m.alt }
        : null;
    }
    if (!i.canEdit || i.locked.has(m.target)) {
      return null;
    }
    if (isRoot(m.target)) {
      return startScreens(m.target, m.world);
    }
    const box = i.boxOf(m.target);
    return box === undefined
      ? null
      : { kind: "node", id: m.target, world: m.world, box, delta: { x: 0, y: 0 }, last: m.world };
  };

  const moveNode = (m: Extract<Mode, { kind: "node" }>, w: Point): void => {
    const i = get();
    const e = env();
    const raw = { x: w.x - m.world.x, y: w.y - m.world.y };
    const siblings = (i.index.children.get(i.index.parent.get(m.id) ?? "") ?? [])
      .filter((k) => k !== m.id)
      .flatMap((k) => i.boxOf(k) ?? []);
    const drop = dropAt(e, [m.id], w.x, w.y);
    const plan = planMove(m.box, siblings, raw, zoom());
    const flow = drop !== null && drop.position === undefined;
    m.delta = flow ? raw : plan.delta;
    m.last = w;
    const parent = i.index.parent.get(m.id);
    i.overlay.set({
      ghost: translate(m.box, m.delta),
      guides: flow ? { lines: [], labels: [], indicator: drop.indicator } : plan.guides,
      drop: flow && drop.parent !== parent ? drop.parent : null,
      hover: null,
    });
  };

  const moveScreens = (m: Extract<Mode, { kind: "screens" }>, w: Point): void => {
    const i = get();
    const raw = { x: w.x - m.world.x, y: w.y - m.world.y };
    const all = i.screens.all();
    const frames = (ids: Iterable<string>) =>
      [...ids].flatMap((s) => {
        const root = i.rootOf(s);
        const box = root === undefined ? undefined : i.boxOf(root);
        const at = all.get(s);
        // The box as it stands at the drag's start: the preview moves the frames, the snap measures from the origin.
        const origin = m.origin.get(s) ?? at;
        return box === undefined || at === undefined || origin === undefined
          ? []
          : [translate(box, { x: origin.x - at.x, y: origin.y - at.y })];
      });
    const own = frames(m.origin.keys());
    const lead = own[0];
    const others = frames([...all.keys()].filter((s) => !m.origin.has(s)));
    const snap =
      lead === undefined ? { delta: { x: 0, y: 0 }, guides: [] } : snapBox(translate(lead, raw), others, zoom());
    const d = { x: Math.round(raw.x + snap.delta.x), y: Math.round(raw.y + snap.delta.y) };
    m.moved = new Map([...m.origin].map(([s, o]) => [s, { x: o.x + d.x, y: o.y + d.y }]));
    i.screens.preview(m.moved);
    i.overlay.set({ guides: snap.guides.length === 0 ? null : { lines: [...snap.guides], labels: [] }, hover: null });
  };

  const hover = (e: PointerLike, p: Point): void => {
    const i = get();
    if (i.tool === "interact") {
      i.overlay.set({ hover: null });
      return;
    }
    const w = toWorld(p);
    const canvas = i.canvas();
    const handle = handleUnder(w);
    if (handle !== undefined) {
      canvas?.setAttribute("data-cursor", "true");
      canvas?.style.setProperty("--pc-cursor", handleCursor(handle.handle));
      i.overlay.set({ hover: null });
      return;
    }
    canvas?.removeAttribute("data-cursor");
    const nodes = nodesAt(rawHits(p));
    const target = sel.resolveHit(i.index, nodes, i.store.get().scope, e.ctrlKey || e.metaKey);
    i.overlay.set({ hover: target });
  };

  const move = (e: PointerLike): void => {
    const m = mode.current;
    const p = local(e);
    if (m === null) {
      hover(e, p);
      return;
    }
    if (m.kind === "pan") {
      get()
        .surface()
        ?.pan(e.clientX - m.last.x, e.clientY - m.last.y);
      m.last = { x: e.clientX, y: e.clientY };
      get().onView();
      return;
    }
    if (m.kind === "press") {
      if (Math.hypot(p.x - m.from.x, p.y - m.from.y) <= CLICK_PX) {
        return;
      }
      mode.current = startDrag(m, p);
      if (mode.current !== null) {
        move(e);
      }
      return;
    }
    dragMove(m, p);
  };

  const dragMove = (m: Mode, p: Point): void => {
    const w = toWorld(p);
    if (m.kind === "marquee") {
      m.to = p;
      get().overlay.set({ marquee: rectOf(m.from, m.to), hover: null });
    } else if (m.kind === "node") {
      moveNode(m, w);
    } else if (m.kind === "resize") {
      m.delta = { x: w.x - m.world.x, y: w.y - m.world.y };
      get().overlay.set({ ghost: resizeBox(m.box, m.handle, m.delta), hover: null });
    } else if (m.kind === "screens") {
      moveScreens(m, w);
    }
  };

  /** The marquee's pick: screens meeting it, or — when it stays inside one screen — the nodes of the scope. */
  const endMarquee = (m: Extract<Mode, { kind: "marquee" }>): void => {
    const i = get();
    const r = rectOf(m.from, m.to);
    const world = rectOf(toWorld({ x: r.l, y: r.t }), toWorld({ x: r.r, y: r.b }));
    const roots = [...i.screens.all().keys()].flatMap((s) => {
      const root = i.rootOf(s);
      const box = root === undefined ? undefined : i.boxOf(root);
      return root === undefined || box === undefined ? [] : [{ root, box }];
    });
    const inside = roots.find((x) => within(world, x.box));
    const base = m.additive ? i.store.get() : sel.emptySelection;
    if (inside !== undefined) {
      const hits = nodesAt(i.surface()?.hitTestRect?.(r, m.alt ? "contain" : "intersect") ?? []);
      const scoped = base.scope === null || i.screenOf.get(base.scope) !== i.screenOf.get(inside.root);
      const s = scoped ? { ...base, scope: inside.root } : base;
      i.store.set(sel.marquee(i.index, s, hits, { toggle: m.additive }));
      return;
    }
    const picked = roots.filter((x) => (m.alt ? within(x.box, world) : meets(x.box, world))).map((x) => x.root);
    const ids = m.additive ? [...base.ids.filter((id) => !picked.includes(id)), ...picked] : picked;
    i.store.set({ ids, anchor: ids[ids.length - 1] ?? null, scope: null });
  };

  const endPress = (m: Extract<Mode, { kind: "press" }>): void => {
    const i = get();
    if (m.target !== null) {
      return;
    }
    if (m.ask && i.onPick(m.hits)) {
      return;
    }
    if (!m.additive && nodesAt(m.hits).length === 0) {
      i.store.set(sel.emptySelection);
    }
  };

  const finishDrag = (m: Extract<Mode, { kind: "node" | "resize" }>): void => {
    if (Math.hypot(m.delta.x, m.delta.y) < 1) {
      return;
    }
    if (m.kind === "node") {
      finishMove(env(), m.id, m.delta, m.last.x, m.last.y);
    } else {
      finishResize(env(), m.id, m.delta, m.handle);
    }
  };

  const finishScreens = (m: Extract<Mode, { kind: "screens" }>): void => {
    const all = new Map(get().screens.all());
    for (const [s, at] of m.moved) {
      all.set(s, at);
    }
    get().screens.commit(all);
  };

  const finish = (m: Mode): void => {
    if (m.kind === "press") {
      endPress(m);
    } else if (m.kind === "marquee") {
      endMarquee(m);
    } else if (m.kind === "node" || m.kind === "resize") {
      finishDrag(m);
    } else if (m.kind === "screens") {
      finishScreens(m);
    }
  };

  const up = (e: PointerLike): void => {
    const m = mode.current;
    mode.current = null;
    const i = get();
    if (m?.kind === "pan") {
      i.canvas()?.removeAttribute("data-panning");
      tracker.end();
      return;
    }
    if (m !== null && m.kind !== "press") {
      // The release point is the drag's last: a drag that ends without a move event still lands where it was let go.
      dragMove(m, local(e));
    }
    i.overlay.set({ marquee: null, guides: null, drop: null, ghost: null });
    if (m !== null) {
      finish(m);
    }
    hover(e, local(e));
  };

  return {
    onPointerDown: down,
    onPointerMove: move,
    onPointerUp: up,
    onPointerCancel: (e: PointerLike) => {
      const m = mode.current;
      mode.current = null;
      get().overlay.set({ marquee: null, guides: null, drop: null, ghost: null });
      if (m?.kind === "screens") {
        get().screens.preview(m.origin);
      }
      if (m?.kind === "pan") {
        get().canvas()?.removeAttribute("data-panning");
        tracker.end();
      }
      void e;
    },
    onPointerLeave: () => {
      if (mode.current === null) {
        get().overlay.set({ hover: null });
      }
    },
    onDoubleClick: (e: PointerLike) => {
      const i = get();
      const hits = rawHits(local(e));
      const title = hits.find((h) => h.startsWith("title:"));
      if (title !== undefined) {
        i.onRenameScreen(screenOfItem(title) ?? "");
        return;
      }
      const next = sel.drill(i.index, i.store.get(), nodesAt(hits));
      i.store.set(next);
      // Drilling down to a text opens it for editing in place, as design tools do.
      const only = next.ids.length === 1 ? next.ids[0] : undefined;
      if (only !== undefined && i.index.type.get(only) === "Text" && i.canEdit && !i.locked.has(only)) {
        i.onEditText?.(only);
      }
    },
    onContextMenu: (e: PointerLike) => {
      e.preventDefault?.();
      const i = get();
      const p = local(e);
      const nodes = nodesAt(rawHits(p));
      if (nodes.length === 0) {
        i.onContextMenu(p, "canvas");
        return;
      }
      const s = i.store.get();
      const target = sel.resolveHit(i.index, nodes, s.scope, e.ctrlKey || e.metaKey);
      if (target !== null && !s.ids.includes(target)) {
        i.store.set(sel.click(i.index, s, nodes, { deep: e.ctrlKey || e.metaKey }));
      }
      const anchor = i.store.get().anchor;
      i.onContextMenu(p, anchor !== null && isRoot(anchor) ? "screen" : "node");
    },
  };
}

/** The item id of a node's box on the canvas: its screen's frame for a root, else `<screen>/<node>`. */
export function boxItemOf(index: NodeIndex, screenOf: ReadonlyMap<string, string>, id: string): string | undefined {
  const screen = screenOf.get(id);
  if (screen === undefined) {
    return undefined;
  }
  return (index.depth.get(id) ?? 1) === 0 ? `screen:${screen}` : itemIdOf(screen, id);
}
