// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { ScreenHeight } from "@plux/canvas";
import { useCallback, useEffect, useRef } from "react";
import { eventActions, impliedOf, isSwitcher, type LiveState, switchAt, withPick, withScroll } from "./live.ts";
import type { DrawerSide } from "./page-parts.ts";
import type { Node } from "./plugin-model.ts";
import type { Point } from "./positions.ts";
import type { PlacedScreen } from "./scene.ts";
import { findNode } from "./select/edits.ts";
import type { Box } from "./select/geometry.ts";
import { type NodeIndex, nodesOfHits, screenOfItem } from "./select/tree.ts";
import { type CanvasSurface, canvasPoint } from "./surface.ts";

export interface LiveSurfaceInput {
  readonly on: boolean;
  readonly surface: () => CanvasSurface | null;
  readonly canvas: () => HTMLCanvasElement | null;
  readonly screens: readonly PlacedScreen[];
  readonly index: NodeIndex;
  readonly screenOf: ReadonlyMap<string, string>;
  readonly drawers: ReadonlyMap<string, DrawerSide>;
  readonly toggleDrawer: (scaffold: string, side: DrawerSide) => void;
  /** The world box of a node, once the interaction knows it. */
  readonly boxOf: () => ((id: string) => Box | undefined) | undefined;
  readonly rtl: boolean;
  /** Shows a screen as the one a link opened: the canvas brings it into view. */
  readonly follow: (screen: string) => void;
  readonly announce: (text: string) => void;
  readonly runsText: (actions: string) => string;
  /** The live values, held by the page so the preview draws them. */
  readonly setLive: (update: (l: LiveState) => LiveState) => void;
}

/** The screen of a hit: a frame's, a header's, or the screen of the item (`<screen>/<node>`). */
function screenOfHit(hit: string): string | undefined {
  const own = screenOfItem(hit);
  if (own !== undefined) {
    return own;
  }
  const slash = hit.indexOf("/");
  return slash < 0 ? undefined : hit.slice(0, slash);
}

/** Every route the tree under `node` opens. */
function routesOpened(node: Node | undefined): string[] {
  if (node === undefined) {
    return [];
  }
  const own = eventActions(node).opens;
  const kids = [...(node.children ?? []), ...Object.values((node.slots ?? {}) as Readonly<Record<string, unknown>>)];
  return [
    ...own,
    ...kids.flatMap((k) => (Array.isArray(k) ? k.flatMap((x) => routesOpened(x as Node)) : routesOpened(k as Node))),
  ];
}

/**
 * The live surface (PC-15): the screens' live values, and what the Interact tool does with a click and the wheel — a
 * switcher picks the item under the pointer, an implied menu button opens the drawer and its scrim closes it, back and
 * close follow to the page that opens the screen, a link follows to its screen, other actions are announced as running
 * on a device; the wheel scrolls the screen's scroll view, once a frame.
 */
export function useLiveSurface(input: LiveSurfaceInput) {
  const scrolls = useRef(new Map<string, readonly { readonly node: string; readonly max: number }[]>());
  const latest = useRef(input);
  latest.current = input;
  const pending = useRef({ node: "", max: 0, delta: 0, frame: 0 });
  useEffect(() => () => cancelAnimationFrame(pending.current.frame), []);

  const setHeights = useCallback((heights: readonly ScreenHeight[]) => {
    for (const h of heights) {
      scrolls.current.set(h.id, h.scrolls ?? []);
    }
  }, []);

  /** The wheel on a live screen scrolls it; true when it did, so the canvas does not pan. */
  const wheel = useCallback((e: WheelEvent): boolean => {
    const i = latest.current;
    const surface = i.surface();
    const canvas = i.canvas();
    if (!i.on || e.ctrlKey || e.metaKey || surface === null || canvas === null) {
      return false;
    }
    const p = canvasPoint(canvas.getBoundingClientRect(), e.clientX, e.clientY);
    const screen = (surface.hitTest?.(p.x, p.y) ?? []).map(screenOfHit).find((s) => s !== undefined);
    const scroll = screen === undefined ? undefined : scrolls.current.get(screen)?.[0];
    if (scroll === undefined) {
      return false;
    }
    const q = pending.current;
    q.delta = q.node === scroll.node ? q.delta + e.deltaY : e.deltaY;
    q.node = scroll.node;
    q.max = scroll.max;
    if (q.frame === 0) {
      q.frame = requestAnimationFrame(() => {
        const { node, max, delta } = pending.current;
        pending.current = { node, max, delta: 0, frame: 0 };
        latest.current.setLive((l) => withScroll(l, node, delta, max));
      });
    }
    return true;
  }, []);

  const interact = useCallback(
    (world: Point, hits: readonly string[]) => interactWith(latest.current, world, hits),
    [],
  );

  return { wheel, interact, setHeights };
}

function nodeOf(i: LiveSurfaceInput, id: string): Node | undefined {
  const root = i.screens.find((s) => s.id === i.screenOf.get(id))?.root;
  const found = root === undefined ? null : findNode(root, id);
  return found === null ? undefined : (found.node as Node);
}

/** Back and close follow to the first page that opens the screen. */
function back(i: LiveSurfaceInput, screen: string): void {
  const route = i.screens.find((s) => s.id === screen)?.route;
  const opener = i.screens.find((s) => s.id !== screen && route !== undefined && routesOpened(s.root).includes(route));
  if (opener !== undefined) {
    i.follow(opener.id);
  }
}

/** An implied button: the menu opens a drawer, back and close follow; true when the hit was one. */
function implied(i: LiveSurfaceInput, hits: readonly string[]): boolean {
  for (const h of hits) {
    const kind = impliedOf(h);
    const screen = screenOfHit(h);
    const root = i.screens.find((s) => s.id === screen)?.root?.id;
    if (kind === undefined || screen === undefined || root === undefined) {
      continue;
    }
    if (kind === "menu" || kind === "endmenu") {
      i.toggleDrawer(root, kind === "menu" ? "drawer" : "endDrawer");
    } else {
      back(i, screen);
    }
    return true;
  }
  return false;
}

/** A click on a shown drawer's scrim, outside the drawer, closes it; true when it did. */
function scrim(i: LiveSurfaceInput, world: Point, ids: readonly string[]): boolean {
  const top = ids[0];
  const root = top === undefined ? undefined : i.screens.find((s) => s.id === i.screenOf.get(top))?.root;
  const side = root === undefined ? undefined : i.drawers.get(root.id);
  const drawer = side === undefined ? undefined : (root?.slots as Readonly<Record<string, Node>> | undefined)?.[side];
  if (root === undefined || side === undefined || drawer === undefined || top !== drawer.id) {
    return false;
  }
  const box = i.boxOf()?.(drawer.id);
  if (box !== undefined && world.x >= box.l && world.x <= box.r) {
    return false;
  }
  i.toggleDrawer(root.id, side);
  return true;
}

/** What a click on `node` does, if anything: pick a switcher's item, follow a link, or say what runs on a device. */
function actOn(i: LiveSurfaceInput, id: string, node: Node, world: Point): boolean {
  if (isSwitcher(node.type)) {
    const box = i.boxOf()?.(id);
    const pick = box === undefined ? undefined : switchAt(node, box, world.x, i.rtl);
    if (pick !== undefined) {
      i.setLive((l) => withPick(l, id, pick));
      return true;
    }
  }
  const { opens, runs } = eventActions(node);
  const target = i.screens.find((s) => s.route === opens[0]);
  if (target !== undefined) {
    i.follow(target.id);
    return true;
  }
  if (runs.length > 0) {
    i.announce(i.runsText(runs.join(", ")));
    return true;
  }
  return false;
}

/** A click with the Interact tool: an implied button, a drawer's scrim, or the first node under it that acts. */
function interactWith(i: LiveSurfaceInput, world: Point, hits: readonly string[]): void {
  if (implied(i, hits)) {
    return;
  }
  const rootOf = (screen: string) => i.screens.find((s) => s.id === screen)?.root?.id;
  const ids = nodesOfHits(hits, i.index, rootOf);
  if (scrim(i, world, ids)) {
    return;
  }
  for (const id of ids) {
    const node = nodeOf(i, id);
    if (node !== undefined && actOn(i, id, node, world)) {
      return;
    }
  }
}
