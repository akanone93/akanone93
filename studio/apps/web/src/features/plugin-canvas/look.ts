// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { DeviceChromeSpec, ScreenStyle } from "@plux/canvas";
import { defaultScreenTheme, LAST_RESORT_FAMILIES, type ScreenTheme, TextDirection } from "@plux/layout";
import { readLocales, readMessages } from "../localisation/l10n-model.ts";
import { genericDevice, type MockState, type PreviewDevice, type PreviewRender } from "../preview/switches.ts";
import { format as formatIcu, pluralArgs } from "./icu.ts";
import { withLocaleOverrides } from "./locale-overrides.ts";
import { documentJson, type Node } from "./plugin-model.ts";

/** Reads the preview switches into what the canvas draws (STU-058, I18N-010, A11Y-005, THM-001). */

export interface DocLike {
  readonly kind: string;
  readonly content: Uint8Array;
  /** The document already parsed (an edit session's unsaved copy); wins over `content`. */
  readonly parsed?: unknown;
}

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);

/** The Cupertino control and bar heights; Material keeps the theme's. */
const CUPERTINO = { appBarHeight: 44, controlHeight: 44 } as const;

/** The parsed documents of `kind`. */
export const jsonOf = (docs: readonly DocLike[], kind: string): Obj[] =>
  docs.map((d) => documentJson(d)).filter((d): d is Obj => isObj(d) && d["kind"] === kind);

/**
 * The value of a design token at `name` for `brand` (its `brands.<brand>.<name>` value first, as the device reads it,
 * THM-003), in dark mode from `$extensions.dev.plux.dark` when `dark`.
 */
function tokenValue(tokens: unknown, name: string, dark: boolean, brand = ""): unknown {
  const at = (path: string): unknown =>
    path.split(".").reduce<unknown>((node, key) => (isObj(node) ? node[key] : undefined), tokens);
  const own = brand === "" ? undefined : at(`brands.${brand}.${name}`);
  const token = isObj(own) ? own : at(name);
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

/** Tokens of the app theme with the overlay's tokens over them, name by name. */
/** The tokens of the app's theme: the one the app document names, else the only one there is. */
function appTokens(themes: readonly Obj[], appTheme: string): Obj {
  const theme =
    themes.find((t) => t["id"] === appTheme || t["key"] === appTheme) ?? (themes.length === 1 ? themes[0] : undefined);
  return theme !== undefined && isObj(theme["tokens"]) ? theme["tokens"] : {};
}

const familiesOf = (v: unknown): string[] =>
  Array.isArray(v) ? v.filter((f): f is string => typeof f === "string") : typeof v === "string" ? [v] : [];

/**
 * The families of every `font.script.<code>` token, in path order, which the device appends to every text style's
 * fallbacks (THM-004, `theme.dart`), so a script the theme gives a font for shows in it on the canvas too.
 */
function scriptFamilies(tokens: Obj, dark: boolean, brand: string): string[] {
  const font = tokens["font"];
  const script = isObj(font) ? font["script"] : undefined;
  if (!isObj(script)) {
    return [];
  }
  return Object.keys(script)
    .filter((k) => !k.startsWith("$"))
    .sort()
    .flatMap((code) => familiesOf(tokenValue(tokens, `font.script.${code}`, dark, brand)));
}

/**
 * The layout theme: the canvas's faces with the app theme's script fonts as fallbacks, read for the previewed brand and
 * mode (THM-001, THM-004), and the platform's bars. What else a page is laid out with — text styles, sizes — the theme's
 * tokens give it as the device reads them; top-level `fontFamily`, `fontSize`, `pagePadding` or `spacing` tokens, which
 * the device does not read, are not read here either (build log row 27).
 */
export function resolveTheme(documents: readonly DocLike[], look: PreviewRender): ScreenTheme {
  const app = jsonOf(documents, "app")[0];
  const appTheme = typeof app?.["theme"] === "string" ? app["theme"] : "";
  const tokens = appTokens(jsonOf(documents, "theme"), appTheme);
  const dark = look.theme === "dark";
  const brand = look.brandOverlay;
  const base = defaultScreenTheme;
  const bars = look.platform === "cupertino" ? CUPERTINO : {};
  return {
    ...base,
    // The app's script fonts come before the canvas's last-resort faces, which only fill what nothing else draws.
    fontFamilies: [
      ...new Set([
        ...base.fontFamilies.filter((f) => !LAST_RESORT_FAMILIES.includes(f)),
        ...scriptFamilies(tokens, dark, brand),
        ...LAST_RESORT_FAMILIES,
      ]),
    ],
    ...bars,
  };
}

/** The locales a message is looked up in, most specific first (I18N-006): `de-AT`, then `de`, then the app's default. */
export function localeChain(locale: string, fallback: string): string[] {
  return [...new Set([locale, locale.split("-")[0] ?? locale, fallback])];
}

/** The messages of `look.locale` by key id, along its fallback chain; pseudo-locales read the default locale's. */
export function messagesFor(documents: readonly DocLike[], look: PreviewRender): Record<string, string> {
  const app = jsonOf(documents, "app")[0];
  const fallback = readLocales(app)?.defaultLocale ?? "en";
  const byLocale = new Map(jsonOf(documents, "translations").map((d) => [String(d["locale"]), readMessages(d)]));
  const chain = localeChain(look.locale, fallback).reverse();
  return Object.assign({}, ...chain.map((l) => byLocale.get(l) ?? {})) as Record<string, string>;
}

/** The props whose strings are visible texts. */
export const TEXT_PROPS: ReadonlySet<string> = new Set(["data", "text", "label", "title", "hint", "tooltip"]);

/** A Text node's `text` prop is read as the layout's `data` when `data` is absent. */
function withData(node: Obj): Obj {
  const props = node["props"];
  if (node["type"] !== "Text" || !isObj(props) || props["data"] !== undefined || props["text"] === undefined) {
    return node;
  }
  return { ...node, props: { ...props, data: props["text"] } };
}

/** How texts are localised on the canvas (PC-32): the previewed locale, plural samples by node, argument values. */
export interface LocalizeOptions {
  readonly locale: string;
  /** The count a plural message of a node is shown with, picked in the inspector. */
  readonly samples?: ReadonlyMap<string, number>;
  /** An argument's design-time value: an expression resolved, a literal as it is; undefined when unknown. */
  readonly arg?: (value: unknown) => unknown;
  /**
   * Whether an argument has its value only later — one that reads a list item, known in each copy of the item
   * template — so its text is left as it is, to be localised again once the lists are drawn; `deferred` counts them.
   */
  readonly defer?: (value: unknown) => boolean;
  readonly deferred?: { count: number };
}

interface Walk {
  readonly messages: Readonly<Record<string, string>>;
  readonly fn: Fn;
  readonly options: LocalizeOptions | undefined;
  /** The node the value belongs to, for its plural sample. */
  readonly node: string | undefined;
}

/** Whether a reference waits for later values (`LocalizeOptions.defer`); counted in `deferred` when it does. */
function waits(given: readonly [string, unknown][], options: LocalizeOptions): boolean {
  const { defer, deferred } = options;
  if (defer === undefined || !given.some(([, v]) => defer(v))) {
    return false;
  }
  if (deferred !== undefined) {
    deferred.count += 1;
  }
  return true;
}

/**
 * A translation reference's text: its message in the locale with its arguments (and the node's sample) filled;
 * undefined when an argument has its value only later (`LocalizeOptions.defer`).
 */
function translated(ref: Obj, w: Walk): string | undefined {
  const id = String(ref["$t"]);
  const message = w.messages[id] ?? id;
  const options = w.options;
  if (!message.includes("{") || options === undefined) {
    return message;
  }
  const given = Object.entries(isObj(ref["args"]) ? ref["args"] : {});
  if (waits(given, options)) {
    return undefined;
  }
  const args: Record<string, unknown> = {};
  for (const [k, v] of given) {
    const value = options.arg === undefined ? v : options.arg(v);
    if (value !== undefined) {
      args[k] = value;
    }
  }
  const sample = w.node === undefined ? undefined : options.samples?.get(w.node);
  if (sample !== undefined) {
    for (const name of pluralArgs(message)) {
      args[name] = sample;
    }
  }
  return formatIcu(message, args, options.locale);
}

/** `value` with `{"$t": id}` resolved to the locale's message and visible texts passed through `localize`. */
function localizeValue(value: unknown, key: string, w: Walk): unknown {
  if (Array.isArray(value)) {
    return value.map((v) => localizeValue(v, key, w));
  }
  if (!isObj(value)) {
    return typeof value === "string" && TEXT_PROPS.has(key) ? w.fn(value) : value;
  }
  if (typeof value["$t"] === "string") {
    const text = translated(value, w);
    return text === undefined ? value : w.fn(text);
  }
  // A node takes its overrides for the previewed locale (I18N-012) and is the owner of the texts below it.
  const isNode = typeof value["id"] === "string" && typeof value["type"] === "string";
  const own = isNode && w.options !== undefined ? withLocaleOverrides(value, w.options.locale) : value;
  const walk = isNode ? { ...w, node: String(value["id"]) } : w;
  return Object.fromEntries(Object.entries(withData(own)).map(([k, v]) => [k, localizeValue(v, k, walk)]));
}
type Fn = (text: string) => string;

/** The widget tree with its texts in the previewed locale (I18N-010, PC-32). */
export function localizeTree(
  root: Node | undefined,
  messages: Readonly<Record<string, string>>,
  localize: Fn,
  options?: LocalizeOptions,
): Node | undefined {
  return root === undefined
    ? undefined
    : (localizeValue(root, "", { messages, fn: localize, options, node: undefined }) as Node);
}

/** The canvas style under the preview switches: theme, text scale and text direction. */
export function lookedStyle(base: ScreenStyle, theme: ScreenTheme, look: PreviewRender): ScreenStyle {
  const d = look.device;
  return {
    ...base,
    theme: { ...theme, width: d.width },
    deviceHeight: d.height,
    textScale: look.textScale,
    textDirection: look.direction === "rtl" ? TextDirection.rtl : TextDirection.ltr,
    locale: look.locale,
    ...(d === genericDevice ? {} : { chrome: chromeSpec(d, look.platform) }),
  };
}

/** The status bar's icons over a light and a dark backdrop, and the camera's black: the device's, not the theme's. */
const CHROME_ON_LIGHT = 0x000000e6;
const CHROME_ON_DARK = 0xffffffff;
const CHROME_CUTOUT = 0x000000ff;

/** The device's system areas as the canvas draws them (PC-11). */
export function chromeSpec(d: PreviewDevice, platform: PreviewRender["platform"]): DeviceChromeSpec {
  return {
    ...d.padding,
    cutout: d.cutout,
    indicator: d.indicator,
    platform,
    radius: d.radius,
    landscape: d.landscape,
    time: "9:41",
    visible: d.chrome,
    ...(d.fold === undefined ? {} : { fold: d.fold }),
    onLight: CHROME_ON_LIGHT,
    onDark: CHROME_ON_DARK,
    cutoutColor: CHROME_CUTOUT,
  };
}

/** The data sources of a plugin's documents, by id, with their design-time mock (DAT-080). */
export function dataSourcesOf(documents: readonly DocLike[]): { id: string; name: string; mock: unknown }[] {
  return jsonOf(documents, "plugin").flatMap(sourcesOf);
}

/** The data sources a document declares (`dataSources`), with their mocks. */
function sourcesOf(doc: Obj): { id: string; name: string; mock: unknown }[] {
  return Array.isArray(doc["dataSources"])
    ? doc["dataSources"].flatMap((s) =>
        isObj(s) && typeof s["id"] === "string"
          ? [{ id: s["id"], name: String(s["name"] ?? s["id"]), mock: s["mock"] }]
          : [],
      )
    : [];
}

/** Each page's own data sources, by page id: a page reads them beside the plugin's, as `data.<name>`. */
export function pageDataSourcesOf(
  documents: readonly DocLike[],
): ReadonlyMap<string, readonly { id: string; name: string; mock: unknown }[]> {
  const out = new Map<string, { id: string; name: string; mock: unknown }[]>();
  for (const page of jsonOf(documents, "page")) {
    const own = sourcesOf(page);
    if (typeof page["id"] === "string" && own.length > 0) {
      out.set(page["id"], own);
    }
  }
  return out;
}

/**
 * The data sources' mocks and preview states as expressions name them (`data.<name>`, PC-23): the preview switch keys
 * a source's state by its id, PXL by its name. A source is found by its id too, for documents that name it so.
 */
export function namedMocks(
  sources: readonly { readonly id: string; readonly name: string; readonly mock: unknown }[],
  states: Readonly<Record<string, MockState>>,
): { readonly mocks: ReadonlyMap<string, unknown>; readonly states: Readonly<Record<string, MockState>> } {
  const mocks = new Map<string, unknown>();
  const named: Record<string, MockState> = {};
  for (const src of sources) {
    for (const key of [src.id, src.name]) {
      mocks.set(key, src.mock);
      const state = states[src.id];
      if (state !== undefined) {
        named[key] = state;
      }
    }
  }
  return { mocks, states: named };
}

/** Screens named in the previewed locale: a translated title takes its message; others keep their name. */
export function titledScreens<T extends { readonly title: string; readonly titleKey?: string }>(
  screens: readonly T[],
  messages: Readonly<Record<string, string>>,
): T[] {
  return screens.map((s) => {
    const text = s.titleKey === undefined ? undefined : messages[s.titleKey];
    return text === undefined || text.trim() === "" ? s : { ...s, title: text };
  });
}
