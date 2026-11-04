// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { type Asset, AssetService } from "@plux/api";
import { assetImageKey } from "@plux/canvas";
import { useQuery } from "@tanstack/react-query";
import { useParams } from "@tanstack/react-router";
import { useEffect, useMemo, useRef } from "react";
import { useApp } from "../../lib/use-app.tsx";
import type { CanvasSurface } from "./surface.ts";

/** The slice of an asset the canvas reads to load its image. */
export type ImageAsset = Pick<Asset, "id" | "mediaType"> & {
  readonly variants: readonly {
    readonly kind: string;
    readonly density: number;
    readonly artifact?: { readonly urls: readonly string[] } | undefined;
  }[];
};

/** Every app asset a tree names: the `{"$asset": id}` values anywhere in it (an `Image`'s or an avatar's source). */
export function assetRefsOf(value: unknown, out: Set<string> = new Set()): Set<string> {
  if (Array.isArray(value)) {
    for (const v of value) {
      assetRefsOf(v, out);
    }
  } else if (typeof value === "object" && value !== null) {
    const record = value as Record<string, unknown>;
    if (typeof record["$asset"] === "string") {
      out.add(record["$asset"]);
    }
    for (const v of Object.values(record)) {
      assetRefsOf(v, out);
    }
  }
  return out;
}

const RASTER_KINDS = ["png", "webp", "original"];

/**
 * The URL the canvas loads an image asset from: a raster form CanvasKit decodes — PNG, then WebP, then the original
 * file when it is a raster image — at the highest density; none for a vector or a file that is not an image.
 */
export function imageUrlOf(asset: ImageAsset): string | undefined {
  if (!asset.mediaType.startsWith("image/") || asset.mediaType === "image/svg+xml") {
    return undefined;
  }
  for (const kind of RASTER_KINDS) {
    const forms = asset.variants
      .filter((v) => v.kind === kind && (v.artifact?.urls.length ?? 0) > 0)
      .sort((a, b) => b.density - a.density);
    const url = forms[0]?.artifact?.urls[0];
    if (url !== undefined && url !== "") {
      return url;
    }
  }
  return undefined;
}

/** Fetches the images of `ids` the canvas does not hold yet (`have`); an asset that cannot be loaded is left out. */
export async function loadAssetImages(
  assets: readonly ImageAsset[],
  ids: ReadonlySet<string>,
  fetchBytes: (url: string) => Promise<Uint8Array | null>,
  have: ReadonlySet<string>,
): Promise<{ key: string; bytes: Uint8Array }[]> {
  const out: { key: string; bytes: Uint8Array }[] = [];
  for (const asset of assets) {
    const key = assetImageKey(asset.id);
    const url = ids.has(asset.id) && !have.has(key) ? imageUrlOf(asset) : undefined;
    const bytes = url === undefined ? null : await fetchBytes(url).catch(() => null);
    if (bytes !== null) {
      out.push({ key, bytes });
    }
  }
  return out;
}

async function fetchBytes(url: string): Promise<Uint8Array | null> {
  const res = await fetch(url);
  return res.ok ? new Uint8Array(await res.arrayBuffer()) : null;
}

/**
 * Gives the canvas the images the screens show (AST-002): the app's image assets their `Image`s and avatars name,
 * fetched from the asset's processed forms once per canvas, after it has laid out `laidOut` screens. An image that
 * cannot be fetched stays a placeholder.
 */
export function useScreenImages(
  app: string | undefined,
  surface: () => CanvasSurface | null,
  roots: readonly unknown[],
  laidOut: number,
): void {
  const appId = app ?? "";
  const { session } = useApp();
  const client = useMemo(() => createClient(AssetService, session.transport), [session.transport]);
  const listed = useQuery({
    queryKey: ["assets", appId],
    enabled: appId !== "",
    queryFn: async () => (await client.listAssets({ appId })).assets,
  });
  const ids = useMemo(() => assetRefsOf(roots), [roots]);
  const loaded = useRef(new WeakMap<CanvasSurface, Set<string>>());
  useEffect(() => {
    const target = surface();
    // The canvas holds the images once it has laid the screens out (`laidOut` of them).
    if (target?.setImages === undefined || listed.data === undefined || ids.size === 0 || laidOut === 0) {
      return;
    }
    const have = loaded.current.get(target) ?? new Set<string>();
    loaded.current.set(target, have);
    let current = true;
    void loadAssetImages(listed.data, ids, fetchBytes, have).then((images) => {
      if (current && images.length > 0) {
        target.setImages?.(images);
        for (const i of images) {
          have.add(i.key);
        }
      }
    });
    return () => {
      current = false;
    };
  }, [surface, listed.data, ids, laidOut]);
}

/** The app's image assets with the URL of each one's preview, for the image picker. */
export function useAppImages(): readonly { readonly id: string; readonly name: string; readonly url: string }[] | null {
  const { session } = useApp();
  const { app } = useParams({ strict: false }) as { app?: string };
  const client = useMemo(() => createClient(AssetService, session.transport), [session.transport]);
  const listed = useQuery({
    queryKey: ["assets", app ?? ""],
    enabled: app !== undefined,
    queryFn: async () => (await client.listAssets({ appId: app ?? "" })).assets,
  });
  return useMemo(
    () =>
      listed.data?.flatMap((a) => {
        const url = imageUrlOf(a);
        return url === undefined ? [] : [{ id: a.id, name: a.name, url }];
      }) ?? null,
    [listed.data],
  );
}
