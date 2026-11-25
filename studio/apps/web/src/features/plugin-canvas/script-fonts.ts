// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The faces the canvas loads only for the texts that need them (build log rows 116 and 126): Noto Sans Ethiopic for
 * Amharic, Tigrinya and the other languages written in Ethiopic, and Noto Sans Symbols for arrows, technical and
 * enclosed symbols — what Noto Sans and Roboto lack, and a device draws from its system fonts. Most apps need neither,
 * so a canvas opens without the 920 KB they weigh together.
 */

import type { FontSource } from "@plux/canvas";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useAppDocuments } from "./use-plugin-documents.ts";

/** A face loaded only when a text needs it. */
export type ScriptFont = "ethiopic" | "symbols";

/** Code point ranges, first and last, in order. */
type Ranges = readonly (readonly [number, number])[];

/** The code points Noto Sans Symbols draws and Noto Sans and Roboto do not; fonts.test.ts checks them against the files. */
export const SYMBOLS_RANGES: Ranges = [
  [0x20dd, 0x20e0],
  [0x20e2, 0x20e4],
  [0x2160, 0x2182],
  [0x2185, 0x2188],
  [0x218a, 0x218b],
  [0x2190, 0x2199],
  [0x2300, 0x230f],
  [0x2311, 0x2315],
  [0x2317, 0x2317],
  [0x231c, 0x231f],
  [0x2322, 0x2323],
  [0x2329, 0x232a],
  [0x232c, 0x2335],
  [0x237c, 0x237c],
  [0x2380, 0x2394],
  [0x2396, 0x239a],
  [0x23af, 0x23af],
  [0x23be, 0x23cd],
  [0x23d0, 0x23db],
  [0x23e2, 0x23e8],
  [0x2460, 0x24ff],
  [0x260a, 0x260d],
  [0x2613, 0x2613],
  [0x2624, 0x262f],
  [0x2638, 0x263b],
  [0x263d, 0x2653],
  [0x2669, 0x267e],
  [0x2690, 0x269d],
  [0x26a2, 0x26a9],
  [0x26ad, 0x26bc],
  [0x26ce, 0x26ce],
  [0x26e2, 0x26ff],
  [0x271d, 0x2721],
  [0x2776, 0x2793],
  [0x2921, 0x2922],
  [0x1f100, 0x1f10c],
  [0x1f110, 0x1f16c],
  [0x1f170, 0x1f190],
  [0x1f19b, 0x1f1ac],
  [0x1f546, 0x1f549],
  [0x1f54f, 0x1f54f],
  [0x1f610, 0x1f610],
  [0x1f700, 0x1f773],
];

/** The Ethiopic script; Noto Sans Ethiopic also draws ⋮ (U+22EE), which no other face has. */
const ETHIOPIC = /\p{Script=Ethiopic}|\u{22EE}/u;

/** A character only these faces may draw: a string without one needs none of them. */
const BEYOND_LATIN = /[\u{1200}-\u{10FFFF}]/u;

/** Whether `ranges` hold `cp`. */
function holds(ranges: Ranges, cp: number): boolean {
  let lo = 0;
  let hi = ranges.length - 1;
  while (lo <= hi) {
    const mid = (lo + hi) >> 1;
    const [first, last] = ranges[mid] ?? [0, -1];
    if (cp < first) {
      hi = mid - 1;
    } else if (cp > last) {
      lo = mid + 1;
    } else {
      return true;
    }
  }
  return false;
}

/** The face that draws `ch` when Noto Sans and Roboto cannot. */
export function scriptFontOf(ch: string): ScriptFont | undefined {
  const cp = ch.codePointAt(0) ?? 0;
  if (ETHIOPIC.test(ch)) {
    return "ethiopic";
  }
  return holds(SYMBOLS_RANGES, cp) ? "symbols" : undefined;
}

/** The faces a string needs, added to `out`; most strings are tested once and need none. */
function stringFonts(text: string, out: Set<ScriptFont>): void {
  if (!BEYOND_LATIN.test(text)) {
    return;
  }
  for (const ch of text) {
    const font = scriptFontOf(ch);
    if (font !== undefined) {
      out.add(font);
    }
  }
}

/** Every face that the strings of a JSON value (a document, a translation file) need, added to `out`. */
export function scriptFontsIn(value: unknown, out: Set<ScriptFont> = new Set()): Set<ScriptFont> {
  if (typeof value === "string") {
    stringFonts(value, out);
  } else if (Array.isArray(value)) {
    for (const v of value) {
      scriptFontsIn(v, out);
    }
  } else if (typeof value === "object" && value !== null) {
    for (const v of Object.values(value)) {
      scriptFontsIn(v, out);
    }
  }
  return out;
}

/** The files of the faces, in the canvas's fonts folder (pinned in fonts/VERSION); Vite emits them as hashed assets. */
export function scriptFontSources(fonts: ReadonlySet<ScriptFont>): FontSource[] {
  return [
    ...(fonts.has("ethiopic")
      ? [
          {
            family: "Noto Sans Ethiopic",
            url: new URL("./fonts/NotoSansEthiopic_400Regular.ttf", import.meta.url).href,
          },
          { family: "Noto Sans Ethiopic", url: new URL("./fonts/NotoSansEthiopic_700Bold.ttf", import.meta.url).href },
        ]
      : []),
    ...(fonts.has("symbols")
      ? [{ family: "Noto Sans Symbols", url: new URL("./fonts/NotoSansSymbols_400Regular.ttf", import.meta.url).href }]
      : []),
  ];
}

/** How long the canvas waits for the app's documents before it opens without these faces. */
const WAIT_MS = 10_000;

/**
 * A loader of the faces the app's texts need, for the canvas surface: it waits for the app's documents (its plugins'
 * pages and its translations, which the canvas reads anyway) and answers with the files of the faces their strings
 * need (`useFontLoader`).
 */
export function useScriptFonts(appId: string | undefined): () => Promise<readonly FontSource[]> {
  const docs = useAppDocuments(appId ?? "");
  const needs = useMemo(() => {
    const out = new Set<ScriptFont>();
    for (const d of docs.data ?? []) {
      scriptFontsIn(d.parsed, out);
    }
    return out;
  }, [docs.data]);
  return useFontLoader(docs.data !== undefined || docs.isError ? needs : null);
}

/**
 * The loader of the faces `current` names (null while the documents are loading): one called before they arrive waits
 * for them, at most `waitMs`, then opens the canvas without the faces. When the faces needed later grow beyond what the
 * running canvas loaded — a symbol typed after it opened — the loader changes, and with it the surface, which opens
 * again with them (their files come from the browser's cache).
 */
export function useFontLoader(
  current: ReadonlySet<ScriptFont> | null,
  waitMs = WAIT_MS,
): () => Promise<readonly FontSource[]> {
  const latest = useRef(current);
  latest.current = current;
  const waiting = useRef<((fonts: ReadonlySet<ScriptFont>) => void)[]>([]);
  // What the running surface loaded; null before its loader ran.
  const loaded = useRef<ReadonlySet<ScriptFont> | null>(null);
  const [generation, setGeneration] = useState(0);
  useEffect(() => {
    if (current === null) {
      return;
    }
    const waiters = waiting.current.splice(0);
    if (waiters.length > 0) {
      loaded.current = current;
      for (const resolve of waiters) {
        resolve(current);
      }
    } else if (loaded.current !== null && [...current].some((f) => !loaded.current?.has(f))) {
      setGeneration((g) => g + 1);
    }
  }, [current]);
  // biome-ignore lint/correctness/useExhaustiveDependencies: a new generation is a new loader, and so a new surface.
  return useCallback(async () => {
    const now = latest.current;
    if (now !== null) {
      loaded.current = now;
      return scriptFontSources(now);
    }
    const fonts = await new Promise<ReadonlySet<ScriptFont>>((resolve) => {
      const answer = (fonts: ReadonlySet<ScriptFont>) => {
        clearTimeout(timer);
        resolve(fonts);
      };
      // Late documents then find no waiter: what they need beyond nothing makes a new loader.
      const timer = setTimeout(() => {
        waiting.current = waiting.current.filter((w) => w !== answer);
        loaded.current ??= new Set();
        resolve(new Set());
      }, waitMs);
      waiting.current.push(answer);
    });
    return scriptFontSources(fonts);
  }, [generation, waitMs]);
}
