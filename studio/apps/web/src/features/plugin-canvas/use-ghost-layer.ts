// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { AssetService } from "@plux/api";
import { type ArrowBox, ArrowCanvas, type ArrowPick, ArrowRouter, deriveBundles, type ItemSpec } from "@plux/canvas";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useRef, useState } from "react";
import { useApp } from "../../lib/use-app.tsx";
import { uploadAsset } from "../panels/upload-asset.ts";
import {
  type AppScreen,
  deriveGhosts,
  type GhostGraph,
  ghostItems,
  type OwnScreen,
  type PlacedGhost,
  placeGhosts,
  SceneSync,
} from "./ghosts.ts";
import { pluginCanvasMessages } from "./plugin-canvas.messages.ts";
import { arrowStyle, contentBounds, DEVICE, ghostStyle, type Heights, type PlacedScreen } from "./scene.ts";
import type { CanvasSurface } from "./surface.ts";
import { type ListedAsset, loadGhostImages, syncThumbnails } from "./thumbnails.ts";

/** Quiet time after the last layout before thumbnails are captured and uploaded (A5.6: rate-limited). */
const THUMBNAIL_DEBOUNCE_MS = 3000;

interface Drawn {
  readonly surface: CanvasSurface;
  readonly ghosts: SceneSync;
  readonly router: ArrowRouter;
  readonly arrows: ArrowCanvas;
}

/** What the layer needs from the page. */
export interface GhostLayerInput {
  readonly appId: string;
  readonly pluginId: string;
  readonly editorSessionId: string;
  readonly canEdit: boolean;
  readonly surface: () => CanvasSurface | null;
  readonly screens: readonly PlacedScreen[];
  readonly own: readonly OwnScreen[];
  readonly heights: Heights;
  readonly appScreens: readonly AppScreen[];
  readonly nativeRoutes: readonly string[];
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
} {
  const { appId, pluginId, editorSessionId, canEdit, surface, screens, own, heights, appScreens, nativeRoutes } = input;
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
    const b = contentBounds(screens, heights);
    return placeGhosts(graph.targets, b.r, b.t);
  }, [graph.targets, screens, heights]);

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
      setImageKeys(new Map(images.map((i) => [i.screenId, i.key])));
    });
    return () => {
      current = false;
    };
  }, [surface, listed.data, placed]);

  const drawn = useRef<Drawn | null>(null);
  useEffect(() => {
    const target = surface();
    if (target === null || target.patch === undefined || heights.size === 0) {
      return;
    }
    if (drawn.current?.surface !== target) {
      const router = new ArrowRouter([], []);
      drawn.current = {
        surface: target,
        ghosts: new SceneSync(),
        router,
        arrows: new ArrowCanvas(router, arrowStyle, target.viewport.zoom),
      };
    }
    const d = drawn.current;
    const items: ItemSpec[] = placed.flatMap((g) =>
      ghostItems(
        g,
        ghostStyle,
        { links: m.ghostLinks },
        g.screenId === undefined ? undefined : imageKeys.get(g.screenId),
      ),
    );
    const boxes: ArrowBox[] = [
      ...screens.map((s) => ({
        id: s.id,
        x: s.at.x,
        y: s.at.y,
        w: DEVICE.width,
        h: Math.max(DEVICE.height, heights.get(s.id) ?? 0),
      })),
      ...placed,
    ];
    d.router.setGraph(boxes, deriveBundles(graph.edges));
    target.patch([...d.ghosts.sync(items), ...d.arrows.render()]);
  }, [surface, screens, heights, placed, imageKeys, graph.edges, m.ghostLinks]);

  const known = useRef(new Map<string, string>());
  useEffect(() => {
    const target = surface();
    if (!canEdit || target?.thumbnail === undefined || heights.size === 0 || appId === "" || editorSessionId === "") {
      return;
    }
    const capture = target.thumbnail.bind(target);
    const timer = setTimeout(() => {
      void syncThumbnails(
        {
          capture,
          list: async () => (await assets.listAssets({ appId })).assets,
          upload: (job) =>
            uploadAsset(assets, { ...job, appId, session: editorSessionId }, { current: null }, () => undefined),
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
  }, [surface, canEdit, heights, screens, appId, editorSessionId, assets, queryClient]);

  const zoomed = (zoom: number) => {
    const d = drawn.current;
    if (d !== null) {
      const ops = d.arrows.setZoom(zoom);
      if (ops.length > 0) {
        d.surface.patch?.(ops);
      }
    }
  };
  const pick = (hits: readonly string[]) => drawn.current?.arrows.pick(hits);
  return { graph, placed, zoomed, pick };
}
