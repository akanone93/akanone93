// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { AssetService } from "@plux/api";
import {
  type ArrowBox,
  ArrowCanvas,
  type ArrowPick,
  ArrowRouter,
  type ArrowStyle,
  deriveBundles,
  type ItemSpec,
  type RouterOptions,
} from "@plux/canvas";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useApp } from "../../lib/use-app.tsx";
import type { LockController } from "../editor/lock-controller.ts";
import { uploadAsset } from "../panels/upload-asset.ts";
import { ArrowMotion } from "./arrow-motion.ts";
import {
  type AppScreen,
  deriveGhosts,
  type GhostGraph,
  type GhostStyle,
  ghostItems,
  type OwnScreen,
  type PlacedGhost,
  placeGhosts,
  SceneSync,
  sameEntries,
  targetKey,
} from "./ghosts.ts";
import { pluginCanvasMessages } from "./plugin-canvas.messages.ts";
import { arrowStyle, contentBounds, DEVICE, ghostStyle, type Heights, type PlacedScreen } from "./scene.ts";
import type { CanvasSurface } from "./surface.ts";
import { type ListedAsset, loadGhostImages, syncThumbnails } from "./thumbnails.ts";

/**
 * How the plugin canvas routes its arrows (#206): clear of the screen titles, which stand 30 CSS pixels above each
 * screen at every zoom (160 world units covers them from 19 % zoom up), and round every screen in the way — a plugin
 * has tens of screens, so no path is left crossing them.
 */
const ARROW_ROUTING: RouterOptions = { headroom: 160, maxThroughGaps: Number.POSITIVE_INFINITY };

/** Quiet time after the last layout before thumbnails are captured and uploaded (A5.6: rate-limited). */
const THUMBNAIL_DEBOUNCE_MS = 3000;

interface Drawn {
  readonly surface: CanvasSurface;
  readonly ghosts: SceneSync;
  readonly router: ArrowRouter;
  readonly arrows: ArrowCanvas;
  /** Hover expansion and live re-routing (STU-044, STU-045). */
  readonly motion: ArrowMotion;
  /** The surface's count of replaced scenes when `ghosts` last sent its items (#289). */
  readonly scene: number;
}

/**
 * The layer's drawing on `target`: a new one on a new surface. A layout that replaced the scene took the target boxes
 * and the arrows with it, while `ghosts` still held the boxes as sent, so they went missing until they changed (#289):
 * the same drawing then sends them all again. `fresh` says nothing of the layer is on the surface any more.
 */
function drawnOn(
  was: Drawn | null,
  target: CanvasSurface,
  style: ArrowStyle,
  lit: readonly string[],
): { readonly d: Drawn; readonly fresh: boolean } {
  const scene = target.scenesReplaced ?? 0;
  if (was?.surface !== target) {
    const router = new ArrowRouter([], [], ARROW_ROUTING);
    const arrows = new ArrowCanvas(router, style, target.viewport.zoom);
    was?.motion.dispose();
    const motion = new ArrowMotion(arrows, (ops) => target.patch?.(ops));
    motion.highlight(lit);
    return { d: { surface: target, ghosts: new SceneSync(), router, arrows, motion, scene }, fresh: true };
  }
  if (was.scene !== scene) {
    return { d: { ...was, ghosts: new SceneSync(), scene }, fresh: true };
  }
  return { d: was, fresh: false };
}

/** Where a dragged target is shown, by its key (#228). */
type ShownAt = Map<string, { readonly base: { x: number; y: number }; readonly at: { x: number; y: number } }>;

/** Forgets the drag of a target placed somewhere new (its drop saved); a drag still running keeps its items' place. */
function forgetDropped(shownAt: ShownAt, placed: readonly PlacedGhost[]): void {
  for (const [key, shown] of [...shownAt]) {
    const g = placed.find((p) => targetKey(p) === key);
    if (g === undefined || g.x !== shown.base.x || g.y !== shown.base.y) {
      shownAt.delete(key);
    }
  }
}

/** The boxes the arrows attach to: the plugin's screens at their laid-out heights, then the placed targets. */
function arrowBoxes(
  screens: readonly PlacedScreen[],
  placed: readonly PlacedGhost[],
  device: { readonly width: number; readonly height: number },
  heights: Heights,
): ArrowBox[] {
  return [
    ...screens.map((s) => ({
      id: s.id,
      x: s.at.x,
      y: s.at.y,
      w: device.width,
      h: Math.max(device.height, heights.get(s.id) ?? 0),
    })),
    ...placed,
  ];
}

/** What the layer needs from the page. */
export interface GhostLayerInput {
  readonly appId: string;
  readonly pluginId: string;
  /**
   * The app's editing lock. The screens' thumbnails are app assets (`assets/index.json`), written only under it; they
   * are uploaded while this tab holds it, and never take it, so a background upload does not keep another person
   * from editing the app's theme or translations.
   */
  readonly appLock?: LockController | undefined;
  readonly canEdit: boolean;
  readonly surface: () => CanvasSurface | null;
  readonly screens: readonly PlacedScreen[];
  readonly own: readonly OwnScreen[];
  readonly heights: Heights;
  readonly appScreens: readonly AppScreen[];
  readonly nativeRoutes: readonly string[];
  /** The device frame the screens are drawn in (PC-11); the generic phone when absent. */
  readonly device?: { readonly width: number; readonly height: number };
  /** The theme's arrows and target boxes (#224), as the app canvas draws them; the brand colours when absent. */
  readonly arrowStyle?: ArrowStyle;
  readonly ghostStyle?: GhostStyle;
  /** The canvas document's saved places: the targets' among them (#228). */
  readonly saved?: ReadonlyMap<string, { readonly x: number; readonly y: number }> | undefined;
}

/**
 * Draws the ghost screens, native nodes and navigation arrows of the plugin canvas as scene ops (STU-051,
 * STU-052, STU-054) and keeps the screens' thumbnails current (A5.6). Nothing here rasterises on the main
 * thread: the worker renders thumbnails and draws every item.
 */
export function useGhostLayer(input: GhostLayerInput): {
  readonly graph: GhostGraph;
  readonly placed: readonly PlacedGhost[];
  /** Redraws the arrows for the zoom (level of detail, STU-048). */
  readonly zoomed: (zoom: number) => void;
  /** The arrow lane under a click, from the hit ids (STU-046). */
  readonly pick: (hits: readonly string[]) => ArrowPick | undefined;
  /** Expands bundle `id` into its lanes at once (a click), or collapses all with null; animated (STU-044). */
  readonly expand: (id: string | null) => void;
  /** The pointer rests over bundle `id` (null: none): it expands once the pointer has settled on it (#206). */
  readonly hover: (id: string | null) => void;
  /** Highlights the arrows of these screens and dims the rest (#206): the screen being worked on. */
  readonly highlight: (screens: readonly string[]) => void;
  /** The pointer rests over a screen or target box (null: none); its arrows light up once it settles (#240). */
  readonly hoverBox: (id: string | null) => void;
  /**
   * Shows target `key` (#228) at `at` while it is dragged, its arrows re-routed, without a re-render; null puts it back
   * where it is placed.
   */
  readonly previewTarget: (key: string, at: { readonly x: number; readonly y: number } | null) => void;
  /** Where a target being dragged is shown now, by its key; undefined when it is not dragged. */
  readonly shownTarget: (key: string) => { readonly x: number; readonly y: number } | undefined;
  /** Calls `listener` whenever a dragged target moves (its label follows it); returns the unsubscribe. */
  readonly onTargetMoved: (listener: () => void) => () => void;
  /** Re-routes the arrows of screens shown at a dragged place, before the drop saves it (STU-045). */
  readonly follow: (moved: ReadonlyMap<string, { readonly x: number; readonly y: number }>) => void;
} {
  const { appId, pluginId, appLock, canEdit, surface, screens, own, heights, appScreens, nativeRoutes } = input;
  const device = input.device ?? DEVICE;
  const ghosts = input.ghostStyle ?? ghostStyle;
  // A new style comes with a new surface (the page makes one per theme), which builds its arrows anew.
  const arrows = input.arrowStyle ?? arrowStyle;
  const { i18n } = useLingui();
  const m = useMemo(() => pluginCanvasMessages(i18n), [i18n]);
  // The surface drawn on now. The effects follow it, not the getter: the page hands a new getter on every render, and
  // following that set the arrows' graph again several times for each selection (#218).
  const onSurface = surface();
  const { session } = useApp();
  const queryClient = useQueryClient();
  const assets = useMemo(() => createClient(AssetService, session.transport), [session.transport]);
  const graph = useMemo(
    () => deriveGhosts({ plugin: pluginId, screens: own, appScreens, nativeRoutes }),
    [pluginId, own, appScreens, nativeRoutes],
  );
  const placed = useMemo(() => {
    if (screens.length === 0 || heights.size === 0) {
      return [];
    }
    const b = contentBounds(screens, heights, device);
    return placeGhosts(graph.targets, b.r, b.t, input.saved);
  }, [graph.targets, screens, heights, device, input.saved]);

  const listed = useQuery({
    queryKey: ["assets", appId],
    enabled: appId !== "",
    queryFn: async () => (await assets.listAssets({ appId })).assets,
  });
  const [imageKeys, setImageKeys] = useState<ReadonlyMap<string, string>>(new Map());
  const loaded = useRef(new Set<string>());
  /** The surface the loaded images were sent to: a new one (a change of theme) holds none of them (#289). */
  const loadedOn = useRef<CanvasSurface | null>(null);
  useEffect(() => {
    const target = onSurface;
    const screenIds = placed.flatMap((g) => (g.screenId === undefined ? [] : [g.screenId]));
    if (target === null || listed.data === undefined || screenIds.length === 0) {
      return;
    }
    if (loadedOn.current !== target) {
      loadedOn.current = target;
      loaded.current = new Set();
    }
    let current = true;
    void loadGhostImages(
      listed.data as readonly ListedAsset[],
      screenIds,
      async (url) => {
        const res = await fetch(url);
        return res.ok ? new Uint8Array(await res.arrayBuffer()) : null;
      },
      loaded.current,
    ).then((images) => {
      if (!current) {
        return;
      }
      const fresh = images.flatMap((i) => (i.bytes === null ? [] : [{ key: i.key, bytes: i.bytes }]));
      if (fresh.length > 0) {
        target.setImages?.(fresh);
        for (const i of fresh) {
          loaded.current.add(i.key);
        }
      }
      // The same keys keep the same map: a new one would re-render the canvas, which places the ghosts and loads
      // their images again, without end.
      const next = new Map(images.map((i) => [i.screenId, i.key]));
      setImageKeys((was) => (sameEntries(was, next) ? was : next));
    });
    return () => {
      current = false;
    };
  }, [onSurface, listed.data, placed]);

  const drawn = useRef<Drawn | null>(null);
  /**
   * Each dragged target's place when its drag began (`base`) and where its items stand now (`at`), by key: a drag moves
   * them by the difference (#228). Kept across redraws until the target's place changes (its drop is saved).
   */
  const shownAt = useRef<ShownAt>(new Map());
  /** Who follows a dragged target (its label), told at each move. */
  const moveListeners = useRef(new Set<() => void>());
  /** The screens whose arrows are highlighted, kept for a redraw onto a new surface or graph. */
  const lit = useRef<readonly string[]>([]);
  /**
   * The boxes and bundles the arrows were last routed for, as JSON; undefined when they must be routed again (a new
   * surface or scene, or a box the router moved during a drag).
   */
  const routed = useRef<string | undefined>(undefined);
  useEffect(() => {
    const target = onSurface;
    if (target === null || target.patch === undefined || heights.size === 0) {
      return;
    }
    const { d, fresh } = drawnOn(drawn.current, target, arrows, lit.current);
    drawn.current = d;
    if (fresh) {
      shownAt.current.clear();
      routed.current = undefined;
    }
    forgetDropped(shownAt.current, placed);
    const items: ItemSpec[] = placed.flatMap((g) =>
      ghostItems(g, ghosts, { links: m.ghostLinks }, g.screenId === undefined ? undefined : imageKeys.get(g.screenId)),
    );
    const boxes = arrowBoxes(screens, placed, device, heights);
    const bundles = deriveBundles(graph.edges);
    // Every layout redraws this layer, an edit's included; the arrows are routed and repainted only when a box or a
    // link changed, not for each edit of a screen's content (STU-121).
    const key = JSON.stringify([boxes.map((b) => [b.id, b.x, b.y, b.w, b.h]), bundles]);
    const ops = d.ghosts.sync(items);
    if (routed.current !== key) {
      routed.current = key;
      d.router.setGraph(boxes, bundles);
      ops.push(...d.arrows.render(d.motion.now()));
    }
    if (ops.length > 0) {
      target.patch(ops);
    }
  }, [onSurface, screens, heights, placed, imageKeys, graph.edges, m.ghostLinks, device, ghosts, arrows]);

  const known = useRef(new Map<string, string>());
  useEffect(() => {
    const target = onSurface;
    if (!canEdit || target?.thumbnail === undefined || heights.size === 0 || appId === "" || appLock === undefined) {
      return;
    }
    const capture = target.thumbnail.bind(target);
    const timer = setTimeout(() => {
      if (appLock.store.getState().status !== "held") {
        return;
      }
      void syncThumbnails(
        {
          capture,
          list: async () => (await assets.listAssets({ appId })).assets,
          upload: (job) =>
            uploadAsset(assets, { ...job, appId, session: appLock.session }, { current: null }, () => undefined),
        },
        screens.map((s) => s.id),
        known.current,
      )
        .then(async (uploaded) => {
          if (uploaded.length > 0) {
            await queryClient.invalidateQueries({ queryKey: ["assets", appId] });
          }
        })
        .catch(() => undefined);
    }, THUMBNAIL_DEBOUNCE_MS);
    return () => clearTimeout(timer);
  }, [onSurface, canEdit, heights, screens, appId, appLock, assets, queryClient]);

  useEffect(() => () => drawn.current?.motion.dispose(), []);

  const zoomed = useCallback((zoom: number) => drawn.current?.motion.zoom(zoom), []);
  const pick = useCallback((hits: readonly string[]) => drawn.current?.arrows.pick(hits), []);
  const expand = useCallback((id: string | null) => drawn.current?.motion.expand(id), []);
  const hover = useCallback((id: string | null) => drawn.current?.motion.hover(id), []);
  const highlight = useCallback((screens: readonly string[]) => {
    lit.current = screens;
    drawn.current?.motion.highlight(screens);
  }, []);
  const follow = useCallback((moved: ReadonlyMap<string, { readonly x: number; readonly y: number }>) => {
    // The router now holds boxes where they are dragged: the next redraw routes the placed graph again.
    routed.current = undefined;
    drawn.current?.motion.follow(moved);
  }, []);
  const hoverBox = useCallback((id: string | null) => drawn.current?.motion.hoverBox(id), []);
  const placedRef = useRef(placed);
  placedRef.current = placed;
  const ghostsRef = useRef(ghosts);
  ghostsRef.current = ghosts;
  const previewTarget = useCallback((key: string, at: { readonly x: number; readonly y: number } | null) => {
    const d = drawn.current;
    const g = placedRef.current.find((p) => targetKey(p) === key);
    if (d === null || g === undefined) {
      return;
    }
    const base = { x: g.x, y: g.y };
    const from = shownAt.current.get(key)?.at ?? base;
    const to = at ?? base;
    if (at === null) {
      shownAt.current.delete(key);
    } else {
      shownAt.current.set(key, { base, at: to });
    }
    for (const listener of moveListeners.current) {
      listener();
    }
    const dx = to.x - from.x;
    const dy = to.y - from.y;
    if (dx === 0 && dy === 0) {
      return;
    }
    const ids = ghostItems(g, ghostsRef.current, { links: () => "" }, undefined).map((i) => i.id);
    d.surface.patch?.(ids.map((id) => ({ op: "move" as const, id, dx, dy })));
    routed.current = undefined;
    d.motion.follow(new Map([[g.id, to]]));
  }, []);
  const shownTarget = useCallback((key: string) => shownAt.current.get(key)?.at, []);
  const onTargetMoved = useCallback((listener: () => void) => {
    moveListeners.current.add(listener);
    return () => {
      moveListeners.current.delete(listener);
    };
  }, []);
  return {
    graph,
    placed,
    zoomed,
    pick,
    expand,
    hover,
    highlight,
    hoverBox,
    previewTarget,
    shownTarget,
    onTargetMoved,
    follow,
  };
}
