// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * What Material 3's text fields paint (STU-005, STA-020), with the defaults of Flutter's `_InputDecoratorDefaultsM3`:
 * the container (an underline, a filled box with an indicator, or an outline), the label resting in the input line or
 * floating above it, the value or else the hint, the prefix and suffix, the helper or error text, and the decoration's
 * icons — each at the part the scene laid out for it (`<id>#input`, `#hint`, `#subtext`, `#prefixIcon` …). An error
 * text turns the indicator, the label and the suffix icon to `error`.
 */

import type { ColorRole } from "../material-color/scheme.ts";
import {
  type BoxPaint,
  elevationShadows,
  type IconPaint,
  type PaintTheme,
  type Rect,
  type Rgba,
  roleColor,
} from "./paint.ts";
import { M3_TYPE_SCALE, type ResolvedText, type TypeRole, typeScaleOf } from "./text-style.ts";

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);
const str = (v: unknown): string | undefined => (typeof v === "string" && v !== "" ? v : undefined);

/** A text a widget draws itself, at a place it chose: a field's label or hint, a segment's label. */
export interface PaintText {
  readonly text: string;
  readonly x: number;
  readonly y: number;
  readonly w: number;
  readonly style: ResolvedText;
  readonly color: Rgba;
}

/** An icon a widget draws itself, in a box of its own. */
export interface PlacedGlyph extends IconPaint {
  readonly x: number;
  readonly y: number;
  readonly w: number;
  readonly h: number;
}

/** What a part-drawing widget paints: its boxes, and the texts and icons it places itself. */
export interface PartsPaint {
  readonly boxes: readonly BoxPaint[];
  readonly texts: readonly PaintText[];
  readonly glyphs: readonly PlacedGlyph[];
}

/** A field's decoration and value as the canvas reads them: a `TextField`'s `decoration`, or a menu's own props. */
export interface FieldModel {
  readonly label?: string;
  readonly hint?: string;
  readonly helper?: string;
  readonly error?: string;
  readonly value?: string;
  readonly prefix?: string;
  readonly suffix?: string;
  readonly counter?: string;
  readonly filled: boolean;
  readonly border: "underline" | "outline" | "none";
  readonly icons: Readonly<Record<"prefixIcon" | "suffixIcon" | "icon", unknown>>;
}

/** The field model of a `TextField` or `TextFormField` node, or of a field-like composite's flat props. */
export function fieldModel(props: Obj): FieldModel {
  const d = isObj(props["decoration"]) ? props["decoration"] : props;
  const border = isObj(d["border"]) ? d["border"]["kind"] : d["border"];
  const pick = (name: string) => {
    const v = str(d[name]);
    return v === undefined ? {} : { [name]: v };
  };
  const value = str(props["value"]) ?? str(props["text"]);
  return {
    ...rename(pick("labelText"), "label"),
    ...rename(pick("hintText"), "hint"),
    ...rename(pick("helperText"), "helper"),
    ...rename(pick("errorText"), "error"),
    ...rename(pick("prefixText"), "prefix"),
    ...rename(pick("suffixText"), "suffix"),
    ...rename(pick("counterText"), "counter"),
    ...(value === undefined ? {} : { value }),
    filled: d["filled"] === true,
    border: border === "outline" ? "outline" : border === "none" ? "none" : "underline",
    icons: { prefixIcon: d["prefixIcon"], suffixIcon: d["suffixIcon"], icon: d["icon"] },
  };
}

function rename(o: Obj, to: string): Obj {
  const v = Object.values(o)[0];
  return v === undefined ? {} : { [to]: v };
}

/** The decoration of a field, flattened as the scene's text field reads it, and the icon parts it lays out. */
export function fieldSceneProps(model: FieldModel): Obj {
  const set = (name: string, v: string | undefined) => (v === undefined ? {} : { [name]: v });
  return {
    ...set("labelText", model.label),
    ...set("hintText", model.hint),
    ...set("helperText", model.helper),
    ...set("errorText", model.error),
    ...set("prefixText", model.prefix),
    ...set("suffixText", model.suffix),
    ...set("counterText", model.counter),
    ...set("text", model.value),
    ...(model.filled ? { filled: true } : {}),
    ...(model.border === "outline" ? { border: "outline" } : {}),
  };
}

/**
 * The field's container and texts. `rectOf` gives the rect of each part the scene laid out; `r` is the field's own
 * box (its decorator, then its helper line).
 */
export function fieldPaint(
  model: FieldModel,
  id: string,
  r: Rect,
  theme: PaintTheme,
  rectOf: (id: string) => Rect | undefined,
): PartsPaint {
  const part = (name: string) => rectOf(`${id}#${name}`);
  const color = (role: ColorRole) => roleColor(theme, role);
  const error = model.error !== undefined;
  const subtext = part("subtext");
  const box: Rect = subtext === undefined ? r : { ...r, h: Math.max(0, subtext.y - r.y - 4) };
  const scale = typeScaleOf(theme);
  const style = (role: TypeRole): ResolvedText => ({ ...(scale[role] ?? M3_TYPE_SCALE[role]), italic: false });
  const input = part("input");
  const texts: PaintText[] = [];
  const text = (t: string | undefined, at: Rect | undefined, role: TypeRole, c: Rgba) => {
    if (t !== undefined && at !== undefined) {
      texts.push({ text: t, x: at.x, y: at.y, w: Math.max(at.w, r.x + r.w - at.x), style: style(role), color: c });
    }
  };
  text(model.value, input, "bodyLarge", color("onSurface"));
  if (model.value === undefined && model.label === undefined) {
    text(model.hint, part("hint") ?? input, "bodyLarge", color("onSurfaceVariant"));
  }
  const boxes: BoxPaint[] = [...containerOf(model, box, theme, error)];
  // A floating label starts where the content does, before a prefix.
  const prefix = part("prefixText");
  const start = input === undefined ? undefined : { ...input, x: Math.min(input.x, prefix?.x ?? input.x) };
  const label = labelPaint(model, box, start, theme, error, style("bodyLarge"), style("bodySmall"));
  texts.push(...label.texts);
  boxes.push(...label.boxes);
  text(model.prefix, part("prefixText"), "bodyLarge", color("onSurfaceVariant"));
  text(model.suffix, part("suffixText"), "bodyLarge", color("onSurfaceVariant"));
  text(model.error ?? model.helper, subtext, "bodySmall", error ? color("error") : color("onSurfaceVariant"));
  text(model.counter, part("counter"), "bodySmall", color("onSurfaceVariant"));
  return { boxes, texts, glyphs: glyphsOf(model, part, theme, error) };
}

/**
 * The field's label: resting in the input line in `bodyLarge` while the field is empty, else floating above it in
 * `bodySmall` — over the outline, which leaves a gap for it.
 */
function labelPaint(
  model: FieldModel,
  box: Rect,
  input: Rect | undefined,
  theme: PaintTheme,
  error: boolean,
  large: ResolvedText,
  small: ResolvedText,
): { texts: PaintText[]; boxes: BoxPaint[] } {
  if (model.label === undefined || input === undefined) {
    return { texts: [], boxes: [] };
  }
  const color = error ? roleColor(theme, "error") : roleColor(theme, "onSurfaceVariant");
  if (model.value === undefined) {
    return { texts: [{ text: model.label, x: input.x, y: input.y, w: box.w, style: large, color }], boxes: [] };
  }
  const outline = model.border === "outline";
  const y = outline ? box.y - (small.size * small.height) / 2 : box.y + 8;
  const gap = model.label.length * small.size * 0.6 + 8;
  return {
    texts: [{ text: model.label, x: input.x, y, w: box.w, style: small, color }],
    // Drawn over the outline (a painted box: corners of 0 keep it out of the batch drawn below).
    boxes: outline
      ? [{ x: input.x - 4, y: box.y - 1, w: gap, h: 2, radii: [0, 0, 0, 0], fill: roleColor(theme, "surface") }]
      : [],
  };
}

/** The field's container: an underline, a filled box with its indicator, or an outline, in `error` on an error. */
function containerOf(model: FieldModel, box: Rect, theme: PaintTheme, error: boolean): BoxPaint[] {
  const line = error ? roleColor(theme, "error") : null;
  if (model.border === "none") {
    return model.filled ? [{ ...box, fill: roleColor(theme, "surfaceContainerHighest") }] : [];
  }
  if (model.border === "outline") {
    const side = { color: line ?? roleColor(theme, "outline"), width: 1 };
    return [{ ...box, radii: [4, 4, 4, 4], border: { kind: "all", side } }];
  }
  const bottom = { color: line ?? roleColor(theme, "onSurfaceVariant"), width: 1 };
  return [
    {
      ...box,
      ...(model.filled ? { fill: roleColor(theme, "surfaceContainerHighest"), radii: [4, 4, 0, 0] as const } : {}),
      border: { kind: "sides", bottom },
    },
  ];
}

/** The decoration's icons and a menu's arrow, in `onSurfaceVariant` (an error's suffix icon in `error`). */
function glyphsOf(
  model: FieldModel,
  part: (name: string) => Rect | undefined,
  theme: PaintTheme,
  error: boolean,
): PlacedGlyph[] {
  const out: PlacedGlyph[] = [];
  for (const name of ["prefixIcon", "suffixIcon", "icon", "arrow"] as const) {
    const at = part(name);
    const data = name === "arrow" ? { name: "arrow_drop_down" } : model.icons[name];
    if (at !== undefined && isObj(data) && typeof data["name"] === "string") {
      const role: ColorRole = error && name === "suffixIcon" ? "error" : "onSurfaceVariant";
      out.push({
        ...at,
        name: data["name"],
        set: data["set"] === "cupertino" ? "cupertino" : "material",
        size: Math.min(24, at.w, at.h),
        color: roleColor(theme, role),
      });
    }
  }
  return out;
}

/**
 * Material 3's `SearchBar`: a 56 pixel pill in `surfaceContainerHigh` at elevation 6, a search icon at its start and
 * its value, or else its hint, in `bodyLarge`.
 */
export function searchBarPaint(props: Obj, r: Rect, theme: PaintTheme): PartsPaint {
  const value = str(props["value"]);
  const shown = value ?? str(props["hintText"]);
  const large = { ...(typeScaleOf(theme).bodyLarge ?? M3_TYPE_SCALE.bodyLarge), italic: false };
  const lineHeight = large.size * large.height;
  return {
    boxes: [
      {
        ...r,
        radii: [r.h / 2, r.h / 2, r.h / 2, r.h / 2],
        fill: roleColor(theme, "surfaceContainerHigh"),
        shadows: elevationShadows(6, roleColor(theme, "shadow")),
      },
    ],
    texts:
      shown === undefined
        ? []
        : [
            {
              text: shown,
              x: r.x + 56,
              y: r.y + (r.h - lineHeight) / 2,
              w: Math.max(0, r.w - 72),
              style: large,
              color: roleColor(theme, value === undefined ? "onSurfaceVariant" : "onSurface"),
            },
          ],
    glyphs: [
      {
        x: r.x + 16,
        y: r.y + (r.h - 24) / 2,
        w: 24,
        h: 24,
        name: "search",
        set: "material",
        size: 24,
        color: roleColor(theme, "onSurface"),
      },
    ],
  };
}

/** The widget types drawn as fields. */
export const fieldTypes: ReadonlySet<string> = new Set(["TextField", "TextFormField", "Autocomplete", "DropdownMenu"]);
