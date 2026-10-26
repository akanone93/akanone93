// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Screen thumbnails (STU-052, A5.6): rendered by the canvas worker from the scene it already holds,
 * uploaded as app assets only when their content hash changed, and read back to draw ghost screens
 * that other plugins' canvases produced.
 */

import { frameId } from "@plux/canvas";
import type { UploadJob } from "../panels/upload-asset.ts";

/** Width of a thumbnail in pixels. */
export const THUMBNAIL_WIDTH = 240;

/** The asset name that holds the thumbnail of a screen. */
export const thumbnailName = (screenId: string): string => `thumbnails/screen-${screenId}.png`;

/** The lower-case hex SHA-256 of `bytes`. */
export async function sha256Hex(bytes: Uint8Array<ArrayBuffer>): Promise<string> {
  const digest = new Uint8Array(await crypto.subtle.digest("SHA-256", bytes));
  return Array.from(digest, (b) => b.toString(16).padStart(2, "0")).join("");
}

/** The slice of an asset that the thumbnail sync reads. */
export interface ListedAsset {
  readonly name: string;
  readonly sha256: string;
  readonly variants?: readonly { readonly artifact?: { readonly urls: readonly string[] } | undefined }[];
}

export interface ThumbnailDeps {
  /** The screen's thumbnail rendered in the canvas worker, or null. */
  readonly capture: (item: string, width: number) => Promise<Uint8Array | null>;
  /** The app's assets. */
  readonly list: () => Promise<readonly ListedAsset[]>;
  /** Uploads a file; resolves when it is stored. */
  readonly upload: (job: Omit<UploadJob, "appId" | "session">) => Promise<unknown>;
}

/**
 * Uploads the thumbnail of every screen in `screenIds` whose content hash differs from the stored one
 * (`known` holds the hashes this tab already knew; it is updated). Returns the IDs uploaded.
 */
export async function syncThumbnails(
  deps: ThumbnailDeps,
  screenIds: readonly string[],
  known: Map<string, string>,
): Promise<string[]> {
  const uploaded: string[] = [];
  let listed: readonly ListedAsset[] | undefined;
  for (const id of screenIds) {
    const png = await deps.capture(frameId(id), THUMBNAIL_WIDTH);
    if (png === null) {
      continue;
    }
    const bytes = new Uint8Array(png);
    const hash = await sha256Hex(bytes);
    if (known.get(id) === hash) {
      continue;
    }
    listed ??= await deps.list();
    known.set(id, hash);
    const name = thumbnailName(id);
    if (listed.some((a) => a.name === name && a.sha256 === hash)) {
      continue;
    }
    await deps.upload({ name, file: new Blob([bytes], { type: "image/png" }) });
    uploaded.push(id);
  }
  return uploaded;
}

/** A thumbnail the ghosts can draw: the key scene items name, and the encoded image. */
export interface GhostImage {
  readonly screenId: string;
  readonly key: string;
  /** The encoded image, or null when the worker already holds the key. */
  readonly bytes: Uint8Array | null;
}

/** Fetches the stored thumbnails of `screenIds`; screens without one, or a failed fetch, are left out. */
export async function loadGhostImages(
  assets: readonly ListedAsset[],
  screenIds: readonly string[],
  fetchBytes: (url: string) => Promise<Uint8Array | null>,
  have: ReadonlySet<string>,
): Promise<GhostImage[]> {
  const out: GhostImage[] = [];
  for (const id of screenIds) {
    const asset = assets.find((a) => a.name === thumbnailName(id));
    const url = asset?.variants?.[0]?.artifact?.urls[0];
    if (asset === undefined) {
      continue;
    }
    if (have.has(asset.sha256)) {
      out.push({ screenId: id, key: asset.sha256, bytes: null });
      continue;
    }
    if (url === undefined) {
      continue;
    }
    const bytes = await fetchBytes(url).catch(() => null);
    if (bytes !== null) {
      out.push({ screenId: id, key: asset.sha256, bytes });
    }
  }
  return out;
}
