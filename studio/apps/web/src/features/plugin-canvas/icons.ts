// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { FontSource, IconNames } from "@plux/canvas";

/**
 * The icon sets of the plugin canvas (THM-005): Material Symbols Outlined and the Cupertino icons, the fonts the
 * server subsets for each release at their default instance (`scripts/gen-icons.ts`). Material Symbols is split into
 * parts, one family each, that the canvas lists as fallbacks of one another. Vite emits every file as a hashed asset,
 * fetched when a canvas starts.
 */
export const ICON_FAMILIES = {
  material: ["Plux Material Symbols 1", "Plux Material Symbols 2"],
  cupertino: ["Plux Cupertino Icons"],
} as const;

/** The icon fonts, by family. */
export const ICON_FONTS: readonly FontSource[] = [
  { family: "Plux Material Symbols 1", url: new URL("./icons/MaterialSymbols-1.ttf", import.meta.url).href },
  { family: "Plux Material Symbols 2", url: new URL("./icons/MaterialSymbols-2.ttf", import.meta.url).href },
  { family: "Plux Cupertino Icons", url: new URL("./icons/CupertinoIcons.ttf", import.meta.url).href },
];

const NAME_FILES = {
  material: new URL("./icons/material.codepoints", import.meta.url).href,
  cupertino: new URL("./icons/cupertino.codepoints", import.meta.url).href,
  mirrored: new URL("./icons/mirrored.txt", import.meta.url).href,
};

/** A `<name> <hex code point>` list as code points by name. */
export function codePointsOf(text: string): Record<string, number> {
  const out: Record<string, number> = {};
  for (const line of text.split("\n")) {
    const [name, hex] = line.trim().split(" ");
    if (name !== undefined && name !== "" && hex !== undefined) {
      out[name] = Number.parseInt(hex, 16);
    }
  }
  return out;
}

/** The icons right-to-left text mirrors, as `<set> <name>` lines. */
export function mirroredOf(text: string): string[] {
  return text
    .split("\n")
    .map((line) => line.trim())
    .filter((line) => line !== "" && !line.startsWith("#"));
}

let names: Promise<IconNames> | null = null;

/** The icon names, fetched once per page; rejects when a list cannot be fetched. */
export function loadIconNames(fetchText: (url: string) => Promise<string> = defaultFetch): Promise<IconNames> {
  names ??= Promise.all([
    fetchText(NAME_FILES.material),
    fetchText(NAME_FILES.cupertino),
    fetchText(NAME_FILES.mirrored),
  ]).then(([material, cupertino, mirrored]) => ({
    material: codePointsOf(material),
    cupertino: codePointsOf(cupertino),
    families: ICON_FAMILIES,
    mirrored: mirroredOf(mirrored),
  }));
  names.catch(() => {
    names = null;
  });
  return names;
}

async function defaultFetch(url: string): Promise<string> {
  const response = await fetch(url);
  if (!response.ok) {
    throw new Error(`icon names: ${response.status}`);
  }
  return response.text();
}
