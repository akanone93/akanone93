// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Accessibility on the canvas (PC-31, A11Y-002–A11Y-004, A11Y-006): what the Accessibility view marks on the screens —
 * the order focus moves through a screen's interactive widgets (document order, an explicit `semantics.focusOrder`
 * first), the touch targets smaller than the platform's minimum (48 × 48 dp on Android, 44 × 44 pt on iOS), and the
 * texts whose colour does not stand out enough from what is behind them (WCAG 2: 4.5:1, 3:1 for large text), worked
 * out from the theme's colours as the compiler checks them; a colour only known at run time is not checked.
 */

import { type PaintTheme, type Rgba, resolveColor, roleColor } from "@plux/layout";
import { widget } from "@plux/schema";

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);
const num = (v: unknown): number | undefined => (typeof v === "number" && Number.isFinite(v) ? v : undefined);

const kidsOf = (n: Obj): unknown[] => [
  ...(isObj(n["slots"]) ? Object.values(n["slots"]).flatMap((v) => (Array.isArray(v) ? v : [v])) : []),
  ...(Array.isArray(n["children"]) ? n["children"] : []),
];

/** Whether a widget of `type` takes focus and taps (its descriptor's `accessibility.interactive`). */
export const interactive = (type: unknown): boolean =>
  typeof type === "string" && widget(type)?.accessibility.interactive === true;

/**
 * The order focus moves through the interactive widgets under `root` (A11Y-006): those with an explicit
 * `semantics.focusOrder` first, by it, then the others in document order; numbered from 1.
 */
export function focusOrder(root: unknown): Map<string, number> {
  const found: { id: string; key: number | undefined; at: number }[] = [];
  const visit = (n: unknown) => {
    if (!isObj(n)) {
      return;
    }
    if (typeof n["id"] === "string" && interactive(n["type"])) {
      const sem = isObj(n["semantics"]) ? n["semantics"] : {};
      found.push({ id: n["id"], key: num(sem["focusOrder"]), at: found.length });
    }
    kidsOf(n).forEach(visit);
  };
  visit(root);
  found.sort((a, b) => (a.key ?? Number.POSITIVE_INFINITY) - (b.key ?? Number.POSITIVE_INFINITY) || a.at - b.at);
  return new Map(found.map((f, i) => [f.id, i + 1]));
}

/** A touch target smaller than `min` on a side: its size. */
export function smallTarget(size: { readonly w: number; readonly h: number }, min: number): boolean {
  return Math.round(size.w) < min || Math.round(size.h) < min;
}

/** The relative luminance of a colour (WCAG 2), from its `0xRRGGBBAA`. */
export function luminance(rgba: Rgba): number {
  const c = [(rgba >>> 24) & 255, (rgba >>> 16) & 255, (rgba >>> 8) & 255].map((v) => {
    const s = v / 255;
    return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * (c[0] as number) + 0.7152 * (c[1] as number) + 0.0722 * (c[2] as number);
}

/** The contrast ratio of two colours (WCAG 2), from 1 to 21. */
export function contrast(a: Rgba, b: Rgba): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x) as [number, number];
  return (hi + 0.05) / (lo + 0.05);
}

/** What paints behind the widgets inside a node, when it paints one: its colour, a decoration's, a surface's. */
function backgroundOf(n: Obj, theme: PaintTheme): Rgba | undefined {
  const props = isObj(n["props"]) ? n["props"] : {};
  const decoration = isObj(props["decoration"]) ? props["decoration"] : {};
  const own = resolveColor(props["color"] ?? props["backgroundColor"] ?? decoration["color"], theme);
  if (own !== undefined) {
    return own;
  }
  switch (n["type"]) {
    case "Scaffold":
      return roleColor(theme, "surface");
    case "Card":
      return roleColor(theme, "surfaceContainerLow");
    default:
      return undefined;
  }
}

/** A text's own colour, when it sets one the theme can resolve. */
function textColorOf(n: Obj, theme: PaintTheme): Rgba | undefined {
  const props = isObj(n["props"]) ? n["props"] : {};
  const style = isObj(props["style"]) ? props["style"] : {};
  return resolveColor(style["color"], theme);
}

/** Whether a text is large (WCAG: 18 pt, or 14 pt bold — 24 and 18.66 logical pixels). */
function large(n: Obj): boolean {
  const style = isObj(n["props"]) && isObj((n["props"] as Obj)["style"]) ? ((n["props"] as Obj)["style"] as Obj) : {};
  const size = num(style["fontSize"]) ?? 14;
  const bold =
    typeof style["fontWeight"] === "number" ? (style["fontWeight"] as number) >= 700 : style["fontWeight"] === "bold";
  return size >= 24 || (bold && size >= 18.66);
}

/** A text whose colour does not stand out enough: its ratio and the ratio it needs. */
export interface LowContrast {
  readonly ratio: number;
  readonly needed: number;
}

/**
 * The texts under `root` whose own colour does not stand out enough from the nearest background behind them (A11Y-003);
 * a text that sets no colour takes its component's, which the theme pairs with its surface, and is not checked here.
 */
export function lowContrast(root: unknown, theme: PaintTheme): Map<string, LowContrast> {
  const out = new Map<string, LowContrast>();
  const visit = (n: unknown, behind: Rgba) => {
    if (!isObj(n)) {
      return;
    }
    const here = backgroundOf(n, theme) ?? behind;
    const low = n["type"] === "Text" ? textContrast(n, here, theme) : undefined;
    if (low !== undefined && typeof n["id"] === "string") {
      out.set(n["id"], low);
    }
    for (const k of kidsOf(n)) {
      visit(k, here);
    }
  };
  visit(root, roleColor(theme, "surface"));
  return out;
}

/** A text's contrast over `behind`, when it sets its own colour and that colour does not stand out enough. */
function textContrast(n: Obj, behind: Rgba, theme: PaintTheme): LowContrast | undefined {
  const color = textColorOf(n, theme);
  if (color === undefined) {
    return undefined;
  }
  const ratio = contrast(color, behind);
  const needed = large(n) ? 3 : 4.5;
  return ratio < needed ? { ratio, needed } : undefined;
}

/** What the Accessibility view marks on `screens` (PC-31). */
export function a11yMarksOf(
  screens: readonly { readonly root?: unknown }[],
  theme: PaintTheme,
  types: ReadonlyMap<string, string>,
  platform: string,
): {
  readonly order: Map<string, number>;
  readonly low: Map<string, LowContrast>;
  readonly targets: Set<string>;
  readonly min: number;
} {
  const order = new Map<string, number>();
  const low = new Map<string, LowContrast>();
  for (const s of screens) {
    for (const [id, n] of focusOrder(s.root)) {
      order.set(id, n);
    }
    for (const [id, c] of lowContrast(s.root, theme)) {
      low.set(id, c);
    }
  }
  // Every interactive widget's laid-out box is its touch target: Material's controls pad theirs to 48 themselves, so
  // what falls short is a gesture on a small child or a control sized below it (A11Y-004).
  const targets = new Set([...types].filter(([, type]) => interactive(type)).map(([id]) => id));
  return { order, low, targets, min: platform === "cupertino" ? 44 : 48 };
}
