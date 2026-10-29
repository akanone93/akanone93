// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { type App, type Asset, AssetService } from "@plux/api";
import { cx } from "@plux/ui";
import { useQuery } from "@tanstack/react-query";
import { type ReactElement, useCallback, useState } from "react";
import { useApp } from "../../lib/use-app.tsx";
import { appsMessages } from "./apps.messages.ts";
import { monogramOf, monogramTone } from "./apps-model.ts";

/** The name prefix of the screen thumbnails the plugin canvas uploads as app assets (STU-052). */
const THUMBNAIL_PREFIX = "thumbnails/screen-";

/** The first URL of an asset's processed forms, or undefined while it has none. */
function urlOf(asset: Asset | undefined): string | undefined {
  return asset?.variants.map((v) => v.artifact?.urls[0]).find((u) => u !== undefined && u !== "");
}

/** The picture of a card in order of preference (STU-021): a screenshot of the plugin canvas, else the uploaded image. */
export function pickVisual(
  app: Pick<App, "iconAssetId">,
  assets: readonly Asset[],
): { readonly kind: "screenshot" | "image"; readonly url: string } | undefined {
  const shot = assets
    .filter((a) => a.name.startsWith(THUMBNAIL_PREFIX) && urlOf(a) !== undefined)
    .sort((a, b) => a.name.localeCompare(b.name))[0];
  const shotUrl = urlOf(shot);
  if (shotUrl !== undefined) {
    return { kind: "screenshot", url: shotUrl };
  }
  const imageUrl = app.iconAssetId === "" ? undefined : urlOf(assets.find((a) => a.id === app.iconAssetId));
  return imageUrl === undefined ? undefined : { kind: "image", url: imageUrl };
}

/** A ref callback that turns true once the element is on screen; without IntersectionObserver it is true at once. */
function useOnScreen(): readonly [(el: HTMLElement | null) => void, boolean] {
  const [seen, setSeen] = useState(typeof IntersectionObserver === "undefined");
  const ref = useCallback((el: HTMLElement | null) => {
    if (el === null || typeof IntersectionObserver === "undefined") {
      return;
    }
    const observer = new IntersectionObserver((entries) => {
      if (entries.some((e) => e.isIntersecting)) {
        setSeen(true);
        observer.disconnect();
      }
    });
    observer.observe(el);
  }, []);
  return [ref, seen];
}

/**
 * The visual of an app's card (STU-021): a live screenshot of its plugin canvas, an uploaded image, or a monogram of
 * the name's first character on a colour derived from the app. The assets are read only once the card is on screen,
 * so a long grid does not make one call per card up front.
 */
export function CardVisual({ app, compact = false }: { readonly app: App; readonly compact?: boolean }): ReactElement {
  const m = appsMessages(useLingui().i18n);
  const { session } = useApp();
  const [ref, seen] = useOnScreen();
  const assets = useQuery({
    queryKey: ["app-visual", app.id],
    enabled: seen,
    staleTime: 5 * 60_000,
    queryFn: async () => (await createClient(AssetService, session.transport).listAssets({ appId: app.id })).assets,
  });
  const visual = assets.data === undefined ? undefined : pickVisual(app, assets.data);
  const frame = compact ? "size-10 rounded-md" : "w-full rounded-lg";
  return (
    <div
      ref={ref}
      data-visual={visual?.kind ?? "monogram"}
      style={compact ? undefined : { aspectRatio: "16 / 10" }}
      className={cx("flex shrink-0 items-center justify-center overflow-hidden", frame, monogramTone(app))}
    >
      {visual === undefined ? (
        <span aria-hidden="true" className={cx("font-serif", compact ? "text-title" : "text-screen")}>
          {monogramOf(app)}
        </span>
      ) : (
        <img
          src={visual.url}
          alt={visual.kind === "screenshot" ? m.cardVisual(app.name) : m.cardIcon(app.name)}
          className="size-full object-cover"
        />
      )}
    </div>
  );
}
