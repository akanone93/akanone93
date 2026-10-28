// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { AppService, AssetService } from "@plux/api";
import type { FontSource } from "@plux/canvas";
import { useCallback, useMemo, useRef } from "react";
import { useApp } from "../../lib/use-app.tsx";

/** The slice of an app that names its host fonts. */
export interface HostFontList {
  readonly hostFonts: readonly { readonly family: string; readonly assetId: string }[];
}

/** The slice of an asset the font URLs are read from. */
export interface FontAsset {
  readonly id: string;
  readonly variants: readonly { readonly artifact?: { readonly urls: readonly string[] } | undefined }[];
}

/**
 * The font files of the host app (STU-006) as canvas font sources, in the order the app lists them: each is the
 * verified asset the entry names. An entry whose asset is gone or has no URL is left out; the canvas then raises
 * no notice for it, since the entry no longer resolves to a file.
 */
export function hostFontSources(app: HostFontList, assets: readonly FontAsset[]): FontSource[] {
  const byId = new Map(assets.map((a) => [a.id, a]));
  return app.hostFonts.flatMap((font) => {
    const url = byId.get(font.assetId)?.variants[0]?.artifact?.urls[0];
    return url === undefined ? [] : [{ family: font.family, url }];
  });
}

/**
 * A loader of the host fonts of `appId` for the canvas surface: it reads the app and its assets when the surface
 * starts, and answers with no fonts when either read fails, so the canvas still opens with Roboto and Noto.
 */
export function useHostFonts(appId: string | undefined): () => Promise<readonly FontSource[]> {
  const { session } = useApp();
  const apps = useMemo(() => createClient(AppService, session.transport), [session.transport]);
  const assets = useMemo(() => createClient(AssetService, session.transport), [session.transport]);
  const ref = useRef({ id: appId ?? "", apps, assets });
  ref.current = { id: appId ?? "", apps, assets };
  // Stable on purpose: the canvas surface is made once per canvas (its control moves to a worker only once), so
  // the loader reads the current app and clients when it runs instead of changing identity with them.
  return useCallback(async () => {
    const { id, apps, assets } = ref.current;
    if (id === "") {
      return [];
    }
    try {
      const { app } = await apps.getApp({ id });
      if (app === undefined || app.hostFonts.length === 0) {
        return [];
      }
      return hostFontSources(app, (await assets.listAssets({ appId: id })).assets);
    } catch {
      return [];
    }
  }, []);
}
