// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Font loading for the canvas (STU-006, ADR-0013 Fonts). Text on the canvas
 * uses the font files the device uses: the verified font assets of the app
 * (ADR-0032), the font files its host app bundles (the native catalogue,
 * S4.6), then Roboto for Material and the Noto fallbacks per script,
 * all open-licensed and served by Plux. SF Pro is never served or stored by
 * Plux: on Chromium the user's own installed copy is read through Local Font
 * Access (`queryLocalFonts`) when the user permits it, and its bytes stay in
 * the browser. Wherever exact metrics cannot be guaranteed — SF Pro
 * substituted, a font that failed to load, a complex script — the canvas
 * raises the notice "device preview is authoritative".
 *
 * Fonts load before the worker starts: their bytes travel in the `init`
 * message, so no paragraph is laid out before them. A font that fails to load
 * is skipped and the next family serves as the fallback.
 */

/** A font file the canvas loads: its family and where its verified bytes are. */
export interface FontSource {
  readonly family: string;
  readonly url: string;
}

/** The fonts a canvas asks for. */
export interface FontRequest {
  /** The app's own font assets, as the device loads them (ADR-0032). */
  readonly device: readonly FontSource[];
  /**
   * The font files the host app bundles, which the device uses by name and
   * the native catalogue lists (S4.6). They rank after the app's own assets,
   * as on the device, where a family the plugin ships wins over the host's.
   * A file that is not available raises the notice for its family.
   */
  readonly host?: readonly FontSource[];
  /** Roboto, the Material default; null when not wanted. */
  readonly roboto: FontSource | null;
  /** The Noto fallbacks per script. */
  readonly noto: readonly FontSource[];
  /** Whether the canvas shows iOS frames, which use SF Pro. */
  readonly sfPro: boolean;
}

/** A font returned by Local Font Access (`FontData` in the browser). */
export interface LocalFontLike {
  readonly family: string;
  readonly postscriptName: string;
  blob(): Promise<{ arrayBuffer(): Promise<ArrayBuffer> }>;
}

/** How fonts are fetched; the browser passes `fetch` and `queryLocalFonts`. */
export interface FontLoadDeps {
  /** The bytes of a verified font asset; rejects when unavailable. */
  fetchFont(url: string): Promise<ArrayBuffer>;
  /**
   * Local Font Access, or undefined where the browser lacks it. It rejects
   * or returns nothing when the user does not permit it.
   */
  readonly queryLocalFonts?: () => Promise<readonly LocalFontLike[]>;
}

/** The family name SF Pro is matched by in Local Font Access. */
export const SF_PRO_FAMILY = /^SF Pro( (Text|Display))?$/;

/** The one notice of the canvas about text fidelity (STU-006). */
export const AUTHORITATIVE_NOTICE = "device-preview-authoritative";

/** Why the device preview is authoritative. */
export type NoticeReason = "sf-pro-substitute" | "font-unavailable" | "complex-script";

/** A notice the host raises: the device preview is authoritative, and why. */
export interface CanvasNotice {
  readonly code: typeof AUTHORITATIVE_NOTICE;
  readonly reason: NoticeReason;
  /** The family or script concerned. */
  readonly subject: string;
}

/** The loaded fonts, ready for `CanvasOptions`. */
export interface LoadedFonts {
  /** The font bytes in priority order, for `CanvasOptions.fonts` (transferred). */
  readonly buffers: ArrayBuffer[];
  /** The family of each buffer, in order. */
  readonly bufferFamilies: string[];
  /** The families loaded, in the same order and without repeats. */
  readonly families: readonly string[];
  /** The family to lay text out in: the first loaded, or `fallback` when none loaded. */
  readonly primary: string;
  readonly notices: readonly CanvasNotice[];
}

async function fetchAll(
  sources: readonly FontSource[],
  deps: FontLoadDeps,
  out: { family: string; bytes: ArrayBuffer }[],
  notices: CanvasNotice[],
): Promise<void> {
  const results = await Promise.allSettled(sources.map((source) => deps.fetchFont(source.url)));
  results.forEach((result, i) => {
    const family = sources[i]?.family ?? "";
    if (result.status === "fulfilled") {
      out.push({ family, bytes: result.value });
    } else {
      notices.push({ code: AUTHORITATIVE_NOTICE, reason: "font-unavailable", subject: family });
    }
  });
}

async function localSfPro(deps: FontLoadDeps): Promise<{ family: string; bytes: ArrayBuffer }[]> {
  if (!deps.queryLocalFonts) {
    return [];
  }
  try {
    const fonts = (await deps.queryLocalFonts()).filter((font) => SF_PRO_FAMILY.test(font.family));
    return await Promise.all(
      fonts.map(async (font) => ({ family: font.family, bytes: await (await font.blob()).arrayBuffer() })),
    );
  } catch {
    // Permission refused or the API failed: the substitute is used with the notice.
    return [];
  }
}

/** Loads the fonts of `request`; never rejects. */
export async function loadCanvasFonts(
  request: FontRequest,
  deps: FontLoadDeps,
  fallback = "Roboto",
): Promise<LoadedFonts> {
  const loaded: { family: string; bytes: ArrayBuffer }[] = [];
  const notices: CanvasNotice[] = [];
  await fetchAll(request.device, deps, loaded, notices);
  await fetchAll(request.host ?? [], deps, loaded, notices);
  if (request.sfPro) {
    const sf = await localSfPro(deps);
    if (sf.length === 0) {
      notices.push({ code: AUTHORITATIVE_NOTICE, reason: "sf-pro-substitute", subject: "SF Pro" });
    }
    loaded.push(...sf);
  }
  await fetchAll([...(request.roboto ? [request.roboto] : []), ...request.noto], deps, loaded, notices);
  const families = [...new Set(loaded.map((font) => font.family))];
  return {
    buffers: loaded.map((font) => font.bytes),
    bufferFamilies: loaded.map((font) => font.family),
    families,
    primary: families[0] ?? fallback,
    notices,
  };
}

const complexScripts: readonly [string, RegExp][] = [
  ["Hebrew", /\p{Script=Hebrew}/u],
  ["Arabic", /\p{Script=Arabic}/u],
  ["Devanagari", /\p{Script=Devanagari}/u],
  ["Bengali", /\p{Script=Bengali}/u],
  ["Tamil", /\p{Script=Tamil}/u],
  ["Thai", /\p{Script=Thai}/u],
  ["Khmer", /\p{Script=Khmer}/u],
  ["Myanmar", /\p{Script=Myanmar}/u],
];

/**
 * The complex script `text` uses, whose shaping on the canvas may differ from
 * the device, or null for text whose metrics the canvas reproduces.
 */
export function complexScriptOf(text: string): string | null {
  for (const [script, pattern] of complexScripts) {
    if (pattern.test(text)) {
      return script;
    }
  }
  return null;
}
