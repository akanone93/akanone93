// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { AssetService } from "@plux/api";
import type { FontSource } from "@plux/canvas";
import { limitDefault } from "@plux/schema";
import { useCallback, useMemo, useRef } from "react";
import { useApp } from "../../lib/use-app.tsx";
import { fontFamilyName } from "./font-family.ts";

const FONT_TYPES: ReadonlySet<string> = new Set(["font/ttf", "font/otf"]);

/** The slice of an asset the device fonts are read from. */
export interface DeviceFontAsset {
  readonly mediaType: string;
  readonly size: bigint | number;
  readonly variants: readonly { readonly artifact?: { readonly urls: readonly string[] } | undefined }[];
}

/**
 * The font assets of an app (ADR-0032) as canvas font sources: each TrueType or OpenType asset named by the
 * family its own `name` table declares, which is how plugin documents reference it. A font that is too large
 * (`asset.fileSize`), cannot be read or has no readable family name is skipped; the others still load.
 */
export async function deviceFontSources(
  assets: readonly DeviceFontAsset[],
  readBytes: (url: string) => Promise<Uint8Array>,
  maxBytes: number = limitDefault("asset.fileSize"),
): Promise<FontSource[]> {
  const fonts = assets.flatMap((a) => {
    const url = a.variants[0]?.artifact?.urls[0];
    return FONT_TYPES.has(a.mediaType) && url !== undefined && Number(a.size) <= maxBytes ? [url] : [];
  });
  const read = await Promise.all(
    fonts.map(async (url): Promise<FontSource | null> => {
      try {
        const bytes = await readBytes(url);
        const family = bytes.byteLength <= maxBytes ? fontFamilyName(bytes) : null;
        return family === null ? null : { family, url };
      } catch {
        return null;
      }
    }),
  );
  return read.flatMap((f) => (f === null ? [] : [f]));
}

/**
 * A loader of the device fonts of `appId` for the canvas surface: it lists the app's assets when the surface
 * starts and answers with no fonts when the read fails, so the canvas still opens with Roboto and Noto.
 */
export function useDeviceFonts(appId: string | undefined): () => Promise<readonly FontSource[]> {
  const { session } = useApp();
  const assets = useMemo(() => createClient(AssetService, session.transport), [session.transport]);
  const ref = useRef(appId ?? "");
  ref.current = appId ?? "";
  return useCallback(async () => {
    const id = ref.current;
    if (id === "") {
      return [];
    }
    try {
      const listed = (await assets.listAssets({ appId: id })).assets;
      return await deviceFontSources(listed, async (url) => {
        const response = await fetch(url);
        if (!response.ok) {
          throw new Error("font unavailable");
        }
        return new Uint8Array(await response.arrayBuffer());
      });
    } catch {
      return [];
    }
  }, [assets]);
}
