// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Likenesses of three Layer 2 components that drew as grey boxes (build log #127), as their packages build them on a
 * device: `SecurePinPad` (`plux_flutter`'s secure widgets) and `CurrencyPicker` and `CountryPicker`
 * (`plux_components`). The scene lays each out at the device's size; this draws its parts.
 */

import type { PaintText, PartsPaint, PlacedGlyph } from "./fields.ts";
import { type BoxPaint, type PaintTheme, type Rect, roleColor } from "./paint.ts";
import { M3_TYPE_SCALE, type ResolvedText, type TypeRole, typeScaleOf } from "./text-style.ts";

type Obj = Record<string, unknown>;

const styleOf = (theme: PaintTheme, role: TypeRole): ResolvedText => ({
  ...(typeScaleOf(theme)[role] ?? M3_TYPE_SCALE[role]),
  italic: false,
});

/** A box `x` from the start of `r`, `w` wide, at its mirror image in a right-to-left screen. */
function fromStart(r: Rect, x: number, w: number, rtl: boolean): number {
  return rtl ? r.x + r.w - x - w : r.x + x;
}

/** The pin pad's dot cell, the dot inside it, the gap under the dots and a key, as the runtime builds them. */
const CELL = 26;
const DOT = 14;
const GAP = 16;
const KEY_W = 80;
const KEY_H = 60;
/** The keys row by row; "" is the blank cell left of 0, and the last is delete. */
const KEYS = ["1", "2", "3", "4", "5", "6", "7", "8", "9", "", "0", "⌫"];

/**
 * `SecurePinPad`: `length` empty rings of 14 pixels in `onSurface`, each in a 26-pixel cell, centred; 16 below them
 * four rows of three 80 by 60 keys with their digits in `headlineSmall`, the last row a blank, 0 and delete. The
 * canvas draws the digits in order; a pad with `randomizeLayout` shuffles them on the device only. A right-to-left
 * screen lays each row from the right, as a `Row` does.
 */
export function securePinPadPaint(props: Obj, r: Rect, theme: PaintTheme): PartsPaint {
  const length = Math.min(12, Math.max(0, Math.trunc(typeof props["length"] === "number" ? props["length"] : 4)));
  const ink = roleColor(theme, "onSurface");
  const rtl = theme.rtl === true;
  const boxes: BoxPaint[] = [];
  const dotsStart = (r.w - length * CELL) / 2;
  for (let i = 0; i < length; i += 1) {
    const x = fromStart(r, dotsStart + i * CELL + (CELL - DOT) / 2, DOT, rtl);
    boxes.push({
      x,
      y: r.y + (CELL - DOT) / 2,
      w: DOT,
      h: DOT,
      oval: true,
      border: { kind: "all", side: { color: ink, width: 1.5 } },
    });
  }
  const style = styleOf(theme, "headlineSmall");
  const lineHeight = style.size * style.height;
  const keysStart = (r.w - 3 * KEY_W) / 2;
  const texts: PaintText[] = [];
  const glyphs: PlacedGlyph[] = [];
  KEYS.forEach((label, i) => {
    const x = fromStart(r, keysStart + (i % 3) * KEY_W, KEY_W, rtl);
    const y = r.y + CELL + GAP + Math.floor(i / 3) * KEY_H;
    if (label === "⌫") {
      glyphs.push({
        name: "backspace",
        set: "material",
        size: 24,
        color: ink,
        x: x + (KEY_W - 24) / 2,
        y: y + (KEY_H - 24) / 2,
        w: 24,
        h: 24,
      });
    } else if (label !== "") {
      texts.push({ text: label, x, y: y + (KEY_H - lineHeight) / 2, w: KEY_W, style, color: ink, align: "center" });
    }
  });
  return { boxes, texts, glyphs };
}

/** The picker's search field, the room around it, a list tile and the tile's start padding and lead, as Material 3. */
const FIELD = 56;
const PAD = 8;
const TILE = 72;
const INSET = 16;
const LEAD = 40;

/** The currencies a picker lists without `currencies` and without locale data, as the runtime's. */
const COMMON_CURRENCIES = ["CHF", "EUR", "GBP", "JPY", "USD"];
/** The territories the canvas lists for a `CountryPicker`: a few, where the device lists every one. */
const SAMPLE_TERRITORIES = ["AT", "CH", "DE", "FR", "GB", "US"];

/** One entry: its code, the name shown and the lead before it. */
interface Entry {
  readonly code: string;
  readonly name: string;
  readonly lead: string;
}

/** A code's English name as Intl gives it, or the code. */
function englishName(code: string, type: "currency" | "region"): string {
  try {
    return new Intl.DisplayNames(["en"], { type }).of(code) ?? code;
  } catch {
    return code;
  }
}

/** The entries a picker of `kind` lists, sorted by name. */
function entriesOf(props: Obj, kind: "currency" | "country"): Entry[] {
  const only = Array.isArray(props["currencies"])
    ? props["currencies"].filter((c): c is string => typeof c === "string")
    : undefined;
  const codes = kind === "currency" ? (only ?? COMMON_CURRENCIES) : SAMPLE_TERRITORIES;
  return codes
    .map((code) => ({ code, name: englishName(code, kind === "currency" ? "currency" : "region"), lead: code }))
    .sort((a, b) => a.name.localeCompare(b.name));
}

/**
 * `CurrencyPicker` and `CountryPicker`: a search field 8 pixels in — a search icon and the hint, over the underline —
 * then two-line list tiles: the lead (the currency's code; a territory's code where the device shows its flag), the
 * name in `bodyLarge` and the code in `bodyMedium` under it, the selected one in `primary` with a check at its end. The
 * canvas has no locale data, so it names them in English; the device names them in its locale and lists every
 * territory.
 */
export function pickerPaint(props: Obj, r: Rect, theme: PaintTheme, kind: "currency" | "country"): PartsPaint {
  const rtl = theme.rtl === true;
  const field: Rect = { x: r.x + PAD, y: r.y + PAD, w: Math.max(0, r.w - 2 * PAD), h: FIELD };
  const muted = roleColor(theme, "onSurfaceVariant");
  const large = styleOf(theme, "bodyLarge");
  const medium = styleOf(theme, "bodyMedium");
  const boxes: BoxPaint[] = [{ ...field, border: { kind: "sides", bottom: { color: muted, width: 1 } } }];
  const glyphs: PlacedGlyph[] = [
    {
      name: "search",
      set: "material",
      size: 24,
      color: muted,
      x: fromStart(field, 12, 24, rtl),
      y: field.y + (FIELD - 24) / 2,
      w: 24,
      h: 24,
    },
  ];
  const hint = typeof props["searchHint"] === "string" ? props["searchHint"] : "Search";
  const align = rtl ? ("end" as const) : undefined;
  const texts: PaintText[] = [
    {
      text: hint,
      x: fromStart(field, 48, Math.max(0, field.w - 60), rtl),
      y: field.y + (FIELD - large.size * large.height) / 2,
      w: Math.max(0, field.w - 60),
      style: large,
      color: muted,
      ...(align === undefined ? {} : { align }),
    },
  ];
  const top = r.y + FIELD + 2 * PAD;
  const fits = Math.max(0, Math.floor((r.y + r.h - top) / TILE));
  entriesOf(props, kind)
    .slice(0, fits)
    .forEach((entry, i) => {
      const y = top + i * TILE;
      const selected = entry.code === props["value"];
      const color = roleColor(theme, selected ? "primary" : "onSurface");
      const textStart = INSET + LEAD + INSET;
      const textW = Math.max(0, r.w - textStart - INSET - (selected ? 40 : 0));
      const lead = { x: fromStart(r, INSET, LEAD, rtl), w: LEAD };
      const body = { x: fromStart(r, textStart, textW, rtl), w: textW };
      const titleH = large.size * large.height;
      const subH = medium.size * medium.height;
      const textTop = y + (TILE - titleH - subH) / 2;
      texts.push(
        {
          text: entry.lead,
          ...lead,
          y: y + (TILE - large.size * large.height) / 2,
          style: medium,
          color: roleColor(theme, selected ? "primary" : "onSurfaceVariant"),
        },
        { text: entry.name, ...body, y: textTop, style: large, color, ...(align === undefined ? {} : { align }) },
        {
          text: entry.code,
          ...body,
          y: textTop + titleH,
          style: medium,
          color: selected ? color : muted,
          ...(align === undefined ? {} : { align }),
        },
      );
      if (selected) {
        glyphs.push({
          name: "check",
          set: "material",
          size: 24,
          color,
          x: fromStart(r, r.w - INSET - 24, 24, rtl),
          y: y + (TILE - 24) / 2,
          w: 24,
          h: 24,
        });
      }
    });
  return { boxes, texts, glyphs };
}
