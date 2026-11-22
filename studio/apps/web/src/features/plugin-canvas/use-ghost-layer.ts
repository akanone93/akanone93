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
  const m = pluginCanvasMessages(useLingui().i18n);
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
  useEffect(() => {
    const target = surface();
    const screenIds = placed.flatMap((g) => (g.screenId === undefined ? [] : [g.screenId]));
    if (target === null || listed.data === undefined || screenIds.length === 0) {
      return;
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
  }, [surface, listed.data, placed]);

  const drawn = useRef<Drawn | null>(null);
  /**
   * Each dragged target's place when its drag began (`base`) and where its items stand now (`at`), by key: a drag moves
   * them by the difference (#228). Kept across redraws until the target's place changes (its drop is saved).
   */
  const shownAt = useRef(
    new Map<string, { readonly base: { x: number; y: number }; readonly at: { x: number; y: number } }>(),
  );
  /** Who follows a dragged target (its label), told at each move. */
  const moveListeners = useRef(new Set<() => void>());
  /** The screens whose arrows are highlighted, kept for a redraw onto a new surface or graph. */
  const lit = useRef<readonly string[]>([]);
  useEffect(() => {
    const target = surface();
    if (target === null || target.patch === undefined || heights.size === 0) {
      return;
    }
    if (drawn.current?.surface !== target) {
      const router = new ArrowRouter([], [], ARROW_ROUTING);
      const canvas = new ArrowCanvas(router, arrows, target.viewport.zoom);
      drawn.current?.motion.dispose();
      drawn.current = {
        surface: target,
        ghosts: new SceneSync(),
        router,
        arrows: canvas,
        motion: new ArrowMotion(canvas, (ops) => target.patch?.(ops)),
      };
      drawn.current.motion.highlight(lit.current);
    }
    const d = drawn.current;
    // A target placed somewhere new (its drop saved) is drawn there; a drag still running keeps its items' place.
    for (const [key, shown] of [...shownAt.current]) {
      const g = placed.find((p) => targetKey(p) === key);
      if (g === undefined || g.x !== shown.base.x || g.y !== shown.base.y) {
        shownAt.current.delete(key);
      }
    }
    const items: ItemSpec[] = placed.flatMap((g) =>
      ghostItems(g, ghosts, { links: m.ghostLinks }, g.screenId === undefined ? undefined : imageKeys.get(g.screenId)),
    );
    const boxes: ArrowBox[] = [
      ...screens.map((s) => ({
        id: s.id,
        x: s.at.x,
        y: s.at.y,
        w: device.width,
        h: Math.max(device.height, heights.get(s.id) ?? 0),
      })),
      ...placed,
    ];
    d.router.setGraph(boxes, deriveBundles(graph.edges));
    target.patch([...d.ghosts.sync(items), ...d.arrows.render(d.motion.now())]);
  }, [surface, screens, heights, placed, imageKeys, graph.edges, m.ghostLinks, device, ghosts, arrows]);

  const known = useRef(new Map<string, string>());
  useEffect(() => {
    const target = surface();
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
  }, [surface, canEdit, heights, screens, appId, appLock, assets, queryClient]);

  useEffect(() => () => drawn.current?.motion.dispose(), []);

  const zoomed = useCallback((zoom: number) => drawn.current?.motion.zoom(zoom), []);
  const pick = useCallback((hits: readonly string[]) => drawn.current?.arrows.pick(hits), []);
  const expand = useCallback((id: string | null) => drawn.current?.motion.expand(id), []);
  const hover = useCallback((id: string | null) => drawn.current?.motion.hover(id), []);
  const highlight = useCallback((screens: readonly string[]) => {
    lit.current = screens;
    drawn.current?.motion.highlight(screens);
  }, []);
  const follow = useCallback(
    (moved: ReadonlyMap<string, { readonly x: number; readonly y: number }>) => drawn.current?.motion.follow(moved),
    [],
  );
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
