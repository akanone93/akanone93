// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { ScreenStyle } from "@plux/canvas";
import { defaultScreenTheme, type ScreenTheme, TextDirection } from "@plux/layout";
import { readLocales, readMessages } from "../localisation/l10n-model.ts";
import type { PreviewRender } from "../preview/switches.ts";
import { type Node, parseDocument } from "./plugin-model.ts";

/** Reads the preview switches into what the canvas draws (STU-058, I18N-010, A11Y-005, THM-001). */

interface DocLike {
  readonly kind: string;
  readonly content: Uint8Array;
}

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);

/** The Cupertino control and bar heights; Material keeps the theme's. */
const CUPERTINO = { appBarHeight: 44, controlHeight: 44 } as const;

const jsonOf = (docs: readonly DocLike[], kind: string): Obj[] =>
  docs.map((d) => parseDocument(d.content)).filter((d): d is Obj => isObj(d) && d["kind"] === kind);

/** The value of a design token at `name`, in dark mode from `$extensions.dev.plux.dark` when `dark`. */
function tokenValue(tokens: unknown, name: string, dark: boolean): unknown {
  const token = isObj(tokens) ? tokens[name] : undefined;
  if (!isObj(token)) {
    return undefined;
  }
  if (dark) {
    const ext = token["$extensions"];
    const plux = isObj(ext) && isObj(ext["dev"]) ? ext["dev"]["plux"] : undefined;
    if (isObj(plux) && plux["dark"] !== undefined) {
      return plux["dark"];
    }
  }
  return token["$value"];
}

const dimension = (v: unknown): number | undefined => {
  const n = typeof v === "number" ? v : typeof v === "string" ? Number.parseFloat(v) : Number.NaN;
  return Number.isFinite(n) && n >= 0 ? n : undefined;
};

/** Tokens of the app theme with the overlay's tokens over them, name by name. */
function mergedTokens(themes: readonly Obj[], appTheme: string, overlay: string): Obj {
  const find = (ref: string): Obj | undefined => themes.find((t) => t["id"] === ref || t["key"] === ref);
  const tokens = (t: Obj | undefined): Obj => (t !== undefined && isObj(t["tokens"]) ? t["tokens"] : {});
  return { ...tokens(find(appTheme)), ...(overlay === "" ? {} : tokens(find(overlay))) };
}

/** The layout theme: the app's theme document plus the brand overlay when present, else the default (THM-001). */
export function resolveTheme(documents: readonly DocLike[], look: PreviewRender): ScreenTheme {
  const app = jsonOf(documents, "app")[0];
  const appTheme = typeof app?.["theme"] === "string" ? app["theme"] : "";
  const tokens = mergedTokens(jsonOf(documents, "theme"), appTheme, look.brandOverlay);
  const dark = look.theme === "dark";
  const pick = (name: string, fallback: number): number => dimension(tokenValue(tokens, name, dark)) ?? fallback;
  const family = tokenValue(tokens, "fontFamily", dark);
  const families = Array.isArray(family)
    ? family.filter((f): f is string => typeof f === "string")
    : typeof family === "string"
      ? [family]
      : [];
  const base = defaultScreenTheme;
  const bars = look.platform === "cupertino" ? CUPERTINO : {};
  return {
    ...base,
    fontFamilies: families.length > 0 ? [...families, ...base.fontFamilies] : base.fontFamilies,
    fontSize: pick("fontSize", base.fontSize),
    pagePadding: pick("pagePadding", base.pagePadding),
    spacing: pick("spacing", base.spacing),
    ...bars,
  };
}

/** The messages of `look.locale` by key id; the app's default locale fills what is missing, and pseudo-locales read it. */
export function messagesFor(documents: readonly DocLike[], look: PreviewRender): Record<string, string> {
  const app = jsonOf(documents, "app")[0];
  const fallback = readLocales(app)?.defaultLocale ?? "en";
  const byLocale = new Map(jsonOf(documents, "translations").map((d) => [String(d["locale"]), readMessages(d)]));
  return { ...byLocale.get(fallback), ...byLocale.get(look.locale) };
}

const TEXT_PROPS: ReadonlySet<string> = new Set(["text", "label", "title", "hint", "tooltip"]);

/** `value` with `{"$t": id}` resolved to the locale's message and visible texts passed through `localize`. */
function localizeValue(value: unknown, key: string, messages: Readonly<Record<string, string>>, fn: Fn): unknown {
  if (Array.isArray(value)) {
    return value.map((v) => localizeValue(v, key, messages, fn));
  }
  if (isObj(value)) {
    const id = value["$t"];
    if (typeof id === "string") {
      return fn(messages[id] ?? id);
    }
    return Object.fromEntries(Object.entries(value).map(([k, v]) => [k, localizeValue(v, k, messages, fn)]));
  }
  return typeof value === "string" && TEXT_PROPS.has(key) ? fn(value) : value;
}
type Fn = (text: string) => string;

/** The widget tree with its texts in the previewed locale (I18N-010). */
export function localizeTree(
  root: Node | undefined,
  messages: Readonly<Record<string, string>>,
  localize: Fn,
): Node | undefined {
  return root === undefined ? undefined : (localizeValue(root, "", messages, localize) as Node);
}

/** The canvas style under the preview switches: theme, text scale and text direction. */
export function lookedStyle(base: ScreenStyle, theme: ScreenTheme, look: PreviewRender): ScreenStyle {
  return {
    ...base,
    theme,
    textScale: look.textScale,
    textDirection: look.direction === "rtl" ? TextDirection.rtl : TextDirection.ltr,
  };
}

/** The data sources of a plugin's documents, by id, with their design-time mock (DAT-080). */
export function dataSourcesOf(documents: readonly DocLike[]): { id: string; name: string; mock: unknown }[] {
  return jsonOf(documents, "plugin").flatMap((p) =>
    Array.isArray(p["dataSources"])
      ? p["dataSources"].flatMap((s) =>
          isObj(s) && typeof s["id"] === "string"
            ? [{ id: s["id"], name: String(s["name"] ?? s["id"]), mock: s["mock"] }]
            : [],
        )
      : [],
  );
}
