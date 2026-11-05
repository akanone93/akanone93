// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/** The app asset an `ImageSource` names, or undefined for a web image or an unbound source. */
export function assetOf(source: unknown): string | undefined {
  if (typeof source !== "object" || source === null) {
    return undefined;
  }
  const asset = (source as Record<string, unknown>)["asset"];
  const id = typeof asset === "object" && asset !== null ? (asset as Record<string, unknown>)["$asset"] : asset;
  return typeof id === "string" ? id : undefined;
}
