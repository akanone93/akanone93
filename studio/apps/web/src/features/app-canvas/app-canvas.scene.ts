// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The scene of the app canvas (C1, STU-030, STU-040, ADR-0013 as revised): plugin boxes, native nodes,
 * frames and the focus ring as Plux Canvas items, and the navigate links between plugins that the
 * bundled arrows are derived from. Pure: no React, no network, no clock.
 */

import type { PluginSummary } from "@plux/api";
import { primitives } from "@plux/brand";
import type { ColorRgba, ItemSpec, NavigateEdge } from "@plux/canvas";
import { rgba } from "../plugin-canvas/scene.ts";
import {
  buildWhereUsed,
  type ChangeSet,
  type LoadedDoc,
  type PageInfo,
  redirectIncoming,
  removeOutgoing,
  type Use,
} from "../safety/index.ts";
import type { appCanvasMessages } from "./app-canvas.messages.ts";
import { BOX, type Frame, NATIVE_BOX, type PlacedBox } from "./app-canvas.model.ts";

type Messages = ReturnType<typeof appCanvasMessages>;

/** A status badge of a box, always a word with its status (U10 status pill). */
export type BadgeStatus = "neutral" | "warning" | "danger" | "success";

export interface Badge {
  readonly status: BadgeStatus;
  readonly label: string;
}

/** The badges of a plugin box (STU-030): entry, archived, lock, unpublished changes, validation, kill switch. */
export function badgesOf(s: PluginSummary | undefined, entry: boolean, m: Messages): Badge[] {
  const out: Badge[] = [];
  if (entry) {
    out.push({ status: "neutral", label: m.entryBadge });
  }
  if (s?.plugin?.archived === true) {
    out.push({ status: "neutral", label: m.archivedBadge });
  }
  if (s !== undefined && s.lockHolder !== "") {
    out.push({ status: "warning", label: m.lockedBy(s.lockHolder) });
  }
  if (s?.unpublishedChanges === true) {
    out.push({ status: "warning", label: m.unpublished });
  }
  if (s !== undefined) {
    out.push(
      s.validationErrors > 0n
        ? { status: "danger", label: m.validationErrors(Number(s.validationErrors)) }
        : { status: "success", label: m.valid },
    );
  }
  if (s?.killed === true) {
    out.push({ status: "danger", label: m.killed });
  }
  return out;
}

/** A box as the scene draws it: a plugin or a native target. */
export interface SceneBox extends PlacedBox {
  readonly title: string;
  readonly native: boolean;
  readonly archived: boolean;
  readonly badges: readonly Badge[];
  /** The line under the name; the key when absent. */
  readonly subtitle?: string;
  /** How many screens the plugin has: the strip shows one miniature each, the entry screen first; none when absent. */
  readonly screens?: number;
  /** The figures beside the strip, one per line at the card's end (how many screens, how many links). */
  readonly figures?: readonly string[];
  /** The release chip: the newest version (`v3`) or `Draft`; none for a native target. */
  readonly release?: string;
}

/** The id of a native target's box. */
export const nativeBox = (route: string): string => `${NATIVE_BOX}${route}`;

/** Scene item ids: the main rectangle of a box, its parts `<id>::<part>`, frames and the focus ring. */
export const boxItem = (id: string): string => `app-box/${id}`;
export const frameItem = (id: string): string => `app-frame/${id}`;
export const FOCUS_ITEM = "app-box-focus";

/** The box an item of the hit index belongs to, or undefined (arrows, frames, nothing). */
export function boxOfItem(item: string): string | undefined {
  if (!item.startsWith("app-box/")) {
    return undefined;
  }
  const rest = item.slice("app-box/".length);
  const cut = rest.indexOf("::");
  return cut < 0 ? rest : rest.slice(0, cut);
}

/** The topmost box among hit ids (topmost first). */
export function boxOfHits(hits: readonly string[]): string | undefined {
  for (const h of hits) {
    const id = boxOfItem(h);
    if (id !== undefined) {
      return id;
    }
  }
  return undefined;
}

interface Pill {
  readonly fill: ColorRgba;
  readonly text: ColorRgba;
  readonly stroke: ColorRgba;
}

/** Porcelain colours of the app canvas scene; every text pair is a measured U2.5 pair. */
export interface AppSceneStyle {
  readonly fill: ColorRgba;
  readonly archivedFill: ColorRgba;
  readonly nativeFill: ColorRgba;
  /** The dashed outline of a native target (STU-033). */
  readonly nativeStroke: ColorRgba;
  readonly stroke: ColorRgba;
  readonly title: ColorRgba;
  readonly secondary: ColorRgba;
  /** The hairline between a card's body and its status strip, and the strip's fill. */
  readonly divider: ColorRgba;
  readonly footer: ColorRgba;
  /** The release chip at the card's top end: its fill and text. */
  readonly chip: ColorRgba;
  readonly chipText: ColorRgba;
  /** The plugin's solid mark and its letter; muted for native targets and archived plugins. */
  readonly tile: ColorRgba;
  readonly tileText: ColorRgba;
  readonly mutedTile: ColorRgba;
  readonly mutedTileText: ColorRgba;
  readonly frameFill: ColorRgba;
  readonly frameStroke: ColorRgba;
  readonly focus: ColorRgba;
  readonly pills: Readonly<Record<BadgeStatus, Pill>>;
  /** A light line along the card's top edge: lit glass in dark, the card itself in light. */
  readonly sheen: ColorRgba;
  /** The plugin hues (the theme's five chart colours): each box takes one, by its key, for its mark and screens. */
  readonly hues: readonly Hue[];
}

/** One plugin hue: the solid mark, and its tints into the card for the screens of the strip. */
export interface Hue {
  readonly solid: ColorRgba;
  /** A screen: its fill and outline; the entry screen is filled a step stronger. */
  readonly soft: ColorRgba;
  readonly strong: ColorRgba;
  readonly line: ColorRgba;
}

export const appSceneStyle: AppSceneStyle = {
  fill: rgba(primitives.white),
  archivedFill: rgba(primitives["porcelain.100"]),
  nativeFill: rgba(primitives["violet.700"], 0x14),
  nativeStroke: rgba(primitives["violet.700"], 0x8c),
  stroke: rgba(primitives["navy.alpha16"]),
  title: rgba(primitives["navy.900"]),
  secondary: rgba(primitives["navy.500"]),
  divider: rgba(primitives["navy.900"], 0x14),
  footer: rgba(primitives["porcelain.100"]),
  chip: rgba(primitives["porcelain.100"]),
  chipText: rgba(primitives["navy.500"]),
  tile: rgba(primitives["violet.700"]),
  tileText: rgba(primitives.white),
  mutedTile: rgba(primitives["porcelain.100"]),
  mutedTileText: rgba(primitives["navy.500"]),
  frameFill: rgba(primitives["navy.900"], 0x08),
  frameStroke: rgba(primitives["navy.900"], 0x30),
  focus: rgba(primitives["violet.700"]),
  sheen: rgba(primitives.white),
  hues: [
    {
      solid: rgba(primitives["violet.700"]),
      soft: rgba(primitives["violet.700"], 0x1c),
      strong: rgba(primitives["violet.700"], 0x40),
      line: rgba(primitives["violet.700"], 0x70),
    },
  ],
  pills: {
    neutral: {
      fill: rgba(primitives["porcelain.100"]),
      text: rgba(primitives["navy.900"]),
      stroke: rgba(primitives["navy.alpha16"]),
    },
    warning: {
      fill: rgba(primitives["amber.50"]),
      text: rgba(primitives["amber.800"]),
      stroke: rgba(primitives["amber.800"], 0x40),
    },
    danger: {
      fill: rgba(primitives.white),
      text: rgba(primitives["red.700"]),
      stroke: rgba(primitives["red.700"], 0x60),
    },
    success: {
      fill: rgba(primitives["green.50"]),
      text: rgba(primitives["green.700"]),
      stroke: rgba(primitives["green.700"], 0x40),
    },
  },
};

/**
 * Depths: frames under arrows (z 1), boxes over them, the expanded bundle (z 11) and focus on top. Boxes sit in bands
 * within [BOX_Z, BOX_Z + BOX_BANDS), each box's parts above its card: a box over another is a band above it and hides
 * the lower box's text and pills, while boxes that overlap nothing share a band, so their items draw in a few batches.
 */
const FRAME_Z = 0;
const BOX_Z = 4;
const BOX_BANDS = 6;
const FOCUS_Z = 20;

/** Where a box draws: the depth of its card and the band its parts sit in above it. */
export interface BoxDepth {
  readonly z: number;
  readonly band: number;
}

/** The depth of a box in band `layer` of `layers`, all of them under the expanded bundle. */
export function boxDepth(layer: number, layers: number): BoxDepth {
  const band = Math.min(1, BOX_BANDS / Math.max(1, layers));
  return { z: BOX_Z + layer * band, band };
}

/**
 * The band of each box, in drawing order: one above the highest band of the earlier boxes it overlaps, the lowest
 * when it overlaps none. Later boxes therefore draw over earlier ones where they overlap, and nowhere else.
 */
export function boxLayers(boxes: readonly PlacedBox[]): number[] {
  const layers: number[] = [];
  for (const [i, b] of boxes.entries()) {
    let layer = 0;
    for (let j = 0; j < i; j++) {
      const o = boxes[j];
      if (
        o !== undefined &&
        b.x < o.x + BOX.width &&
        o.x < b.x + BOX.width &&
        b.y < o.y + BOX.height &&
        o.y < b.y + BOX.height
      ) {
        layer = Math.max(layer, (layers[j] ?? 0) + 1);
      }
    }
    layers.push(layer);
  }
  return layers;
}

/**
 * The card's geometry in world units (STU-030), on the 260 × 152 box: a head with the plugin's mark (a tile in its hue
 * with its initial), its name and key and the release chip; a strip of miniature screens in its hue with the figures at
 * its end; and a status strip along the foot with the badges as pills.
 */
const CARD = {
  radius: 16,
  pad: 16,
  tile: 36,
  tileRadius: 11,
  /** Where the name and the line under it start: after the tile and a gap. */
  text: 16 + 36 + 12,
  chipWidth: 52,
  chipHeight: 20,
  /** The strip of screens: its top, and one miniature's size, corner and step. */
  stripTop: 64,
  phoneWidth: 20,
  phoneHeight: 34,
  phoneRadius: 5,
  phoneStep: 26,
  /** At most this many miniatures; the rest are counted. */
  phones: 5,
  /** The status strip: its top edge, and the pills' top within the card. */
  footer: 112,
  pillsTop: 122,
  pillHeight: 20,
} as const;

/** An upper bound of the advance of one character at 11 px; the worker lays the text out exactly. */
const PILL_CHAR = 6.2;
/** A pill's padding and its status dot: 8 before the dot, the 6 px dot, 6 after it, 8 after the label. */
const PILL_CHROME = 28;
const PILL_GAP = 6;

/** Pill rows that fit in a box's status row. */
const MAX_PILL_ROWS = 1;

interface PlacedPill extends Badge {
  readonly x: number;
  readonly row: number;
  readonly w: number;
}

const pillWidth = (label: string, inner: number): number =>
  Math.min(inner, Math.ceil(PILL_CHROME + label.length * PILL_CHAR));

/**
 * Badges wrapped into at most `rows` rows of `inner` width. What does not fit is summed up in a final "+N" pill; the
 * accessible layer and the detail card still name every badge.
 */
export function layoutPills(badges: readonly Badge[], inner: number, rows: number = MAX_PILL_ROWS): PlacedPill[] {
  const out: PlacedPill[] = [];
  let x = 0;
  let row = 0;
  for (const badge of badges) {
    const w = pillWidth(badge.label, inner);
    if (x > 0 && x + w > inner) {
      x = 0;
      row++;
    }
    if (row >= rows) {
      break;
    }
    out.push({ ...badge, x, row, w });
    x += w + PILL_GAP;
  }
  while (out.length < badges.length) {
    const more = `+${badges.length - out.length}`;
    const last = out.at(-1);
    const at = last === undefined ? 0 : last.x + last.w + PILL_GAP;
    const w = pillWidth(more, inner);
    if (last === undefined || at + w <= inner) {
      out.push({ status: "neutral", label: more, x: at, row: last?.row ?? 0, w });
      break;
    }
    out.pop();
  }
  return out;
}

/** The hue a box takes, by its key: stable across sessions and machines, spread over the theme's hues. */
export function hueOf(key: string, hues: number): number {
  let h = 0;
  for (const c of key) {
    h = (h * 31 + (c.codePointAt(0) ?? 0)) % 9973;
  }
  return hues > 0 ? h % hues : 0;
}

/** The initial on a box's tile: the first character of its title, in capitals. */
export function initialOf(title: string): string {
  const [first = ""] = [...title.trim()];
  return first.toLocaleUpperCase();
}

/** One status pill of a box: a soft capsule with a solid dot in the status colour and its word. */
function pillItems(
  main: string,
  i: number,
  p: PlacedPill,
  at: { readonly x: number; readonly y: number; readonly z: number; readonly band: number },
  style: AppSceneStyle,
): ItemSpec[] {
  const pill = style.pills[p.status];
  const { x, y, z, band } = at;
  const h = CARD.pillHeight;
  return [
    {
      kind: "rect",
      id: `${main}::badge${i}`,
      x,
      y,
      w: p.w,
      h,
      radius: h / 2,
      fill: pill.fill,
      stroke: { color: pill.stroke, width: 1 },
      z: z + band / 4,
    },
    {
      kind: "rect",
      id: `${main}::badge${i}d`,
      x: x + 8,
      y: y + (h - 6) / 2,
      w: 6,
      h: 6,
      radius: 3,
      fill: pill.text,
      z: z + band / 2,
    },
    {
      kind: "text",
      id: `${main}::badge${i}t`,
      x: x + 20,
      y: y + 3,
      w: Math.max(0, p.w - 26),
      text: p.label,
      size: 11,
      color: pill.text,
      maxLines: 1,
      z: z + band / 2,
    },
  ];
}

/** A box's outline: the focus colour when selected; a native target's dashed, so it never reads as a plugin (STU-033). */
function outlineOf(b: SceneBox, style: AppSceneStyle, selected: boolean) {
  if (selected) {
    return { color: style.focus, width: 2, ...(b.native ? { dash: 7 } : {}) };
  }
  return b.native ? { color: style.nativeStroke, width: 1.5, dash: 7 } : { color: style.stroke, width: 1 };
}

/**
 * The items of one box, a card resting on the canvas with a soft shadow (STU-030): its head (the solid mark with the
 * plugin's initial, the name in bold on one line, the key under it, and the release chip at the end), its figures, and
 * a tinted status strip along its foot with the badges as pills on one row. A selected box has a thicker outline in
 * the focus colour, drawn with the card itself.
 */
export function boxItems(
  b: SceneBox,
  style: AppSceneStyle = appSceneStyle,
  selected = false,
  depth: BoxDepth = { z: BOX_Z, band: 1 },
): ItemSpec[] {
  const { z, band } = depth;
  const main = boxItem(b.id);
  const { pad } = CARD;
  const muted = b.native || b.archived;
  const hue = style.hues[hueOf(b.key, style.hues.length)] ?? style.hues[0];
  const over = z + band / 4;
  const top = z + band / 2;
  const chip = b.release === undefined ? 0 : CARD.chipWidth + 8;
  const items: ItemSpec[] = [
    {
      kind: "rect",
      id: main,
      label: b.title,
      x: b.x,
      y: b.y,
      w: BOX.width,
      h: BOX.height,
      radius: CARD.radius,
      fill: b.native ? style.nativeFill : b.archived ? style.archivedFill : style.fill,
      stroke: outlineOf(b, style, selected),
      shadow: !b.native,
      z,
    },
    {
      kind: "rect",
      id: `${main}::tile`,
      x: b.x + pad,
      y: b.y + pad,
      w: CARD.tile,
      h: CARD.tile,
      radius: CARD.tileRadius,
      fill: muted || hue === undefined ? style.mutedTile : hue.solid,
      z: over,
    },
    {
      kind: "rect",
      id: `${main}::sheen`,
      x: b.x + CARD.radius,
      y: b.y + 1,
      w: BOX.width - 2 * CARD.radius,
      h: 1,
      radius: 0,
      fill: style.sheen,
      z: over,
    },
    {
      kind: "text",
      id: `${main}::initial`,
      x: b.x + pad,
      y: b.y + pad + 7,
      w: CARD.tile,
      text: initialOf(b.title),
      size: 16,
      weight: "bold",
      align: "center",
      color: muted ? style.mutedTileText : style.tileText,
      z: top,
    },
    {
      kind: "text",
      id: `${main}::title`,
      x: b.x + CARD.text,
      y: b.y + pad - 1,
      w: BOX.width - CARD.text - pad - chip,
      text: b.title,
      size: 15,
      weight: "bold",
      maxLines: 1,
      color: style.title,
      z: over,
    },
    {
      kind: "text",
      id: `${main}::key`,
      x: b.x + CARD.text,
      y: b.y + pad + 20,
      w: BOX.width - CARD.text - pad,
      text: b.subtitle ?? b.key,
      size: 12,
      maxLines: 1,
      color: style.secondary,
      z: over,
    },
  ];
  if (b.release !== undefined) {
    const x = b.x + BOX.width - pad - CARD.chipWidth;
    items.push(
      {
        kind: "rect",
        id: `${main}::chip`,
        x,
        y: b.y + pad,
        w: CARD.chipWidth,
        h: CARD.chipHeight,
        radius: CARD.chipHeight / 2,
        fill: style.chip,
        z: over,
      },
      {
        kind: "text",
        id: `${main}::release`,
        x,
        y: b.y + pad + 3,
        w: CARD.chipWidth,
        text: b.release,
        size: 11,
        weight: "bold",
        align: "center",
        maxLines: 1,
        color: style.chipText,
        z: top,
      },
    );
  }
  if (hue !== undefined && !muted) {
    items.push(...stripItems(main, b, { over, top }, hue, style));
  }
  if (b.badges.length > 0) {
    items.push(...footerItems(main, b, { z, band, over, top }, style));
  }
  return items;
}

/**
 * The strip of miniature screens: one phone per screen in the plugin's hue, the entry screen filled a step stronger,
 * each with a notch along its top; beyond `CARD.phones` a count; and the figures at the card's end, one per line.
 */
function stripItems(
  main: string,
  b: SceneBox,
  at: { readonly over: number; readonly top: number },
  hue: Hue,
  style: AppSceneStyle,
): ItemSpec[] {
  const count = b.screens ?? 0;
  const shown = Math.min(count, CARD.phones);
  const y = b.y + CARD.stripTop;
  const items: ItemSpec[] = [];
  for (let i = 0; i < shown; i++) {
    const x = b.x + CARD.pad + i * CARD.phoneStep;
    items.push(
      {
        kind: "rect",
        id: `${main}::phone${i}`,
        x,
        y,
        w: CARD.phoneWidth,
        h: CARD.phoneHeight,
        radius: CARD.phoneRadius,
        fill: i === 0 ? hue.strong : hue.soft,
        stroke: { color: hue.line, width: 1 },
        z: at.over,
      },
      {
        kind: "rect",
        id: `${main}::phone${i}n`,
        x: x + 6,
        y: y + 4,
        w: CARD.phoneWidth - 12,
        h: 2,
        radius: 1,
        fill: hue.line,
        z: at.top,
      },
    );
  }
  if (count > shown) {
    items.push({
      kind: "text",
      id: `${main}::more`,
      x: b.x + CARD.pad + shown * CARD.phoneStep,
      y: y + 10,
      w: 40,
      text: `+${count - shown}`,
      size: 12,
      weight: "bold",
      color: style.secondary,
      z: at.over,
    });
  }
  for (const [i, figure] of (b.figures ?? []).slice(0, 2).entries()) {
    items.push({
      kind: "text",
      id: `${main}::figure${i}`,
      x: b.x + BOX.width - CARD.pad - 96,
      y: y + 2 + i * 17,
      w: 96,
      text: figure,
      size: i === 0 ? 12 : 11,
      ...(i === 0 ? { weight: "bold" as const } : {}),
      align: "end",
      maxLines: 1,
      color: i === 0 ? style.title : style.secondary,
      z: at.over,
    });
  }
  return items;
}

/**
 * The status strip along the card's foot: a tinted band whose lower corners follow the card's (a rounded rectangle,
 * its upper corners squared by a second one), a hairline above it, and the badges as pills on one row.
 */
function footerItems(
  main: string,
  b: SceneBox,
  at: { readonly z: number; readonly band: number; readonly over: number; readonly top: number },
  style: AppSceneStyle,
): ItemSpec[] {
  const inset = 1;
  const y = b.y + CARD.footer;
  const h = BOX.height - CARD.footer - inset;
  const strip = at.z + at.band / 8;
  const items: ItemSpec[] = [
    {
      kind: "rect",
      id: `${main}::footer`,
      x: b.x + inset,
      y,
      w: BOX.width - 2 * inset,
      h,
      radius: CARD.radius - inset,
      fill: style.footer,
      z: strip,
    },
    {
      kind: "rect",
      id: `${main}::footer-top`,
      x: b.x + inset,
      y,
      w: BOX.width - 2 * inset,
      h: CARD.radius,
      radius: 0,
      fill: style.footer,
      z: strip,
    },
    {
      kind: "rect",
      id: `${main}::divider`,
      x: b.x + inset,
      y,
      w: BOX.width - 2 * inset,
      h: 1,
      radius: 0,
      fill: style.divider,
      z: at.over,
    },
  ];
  const inner = BOX.width - 2 * CARD.pad;
  for (const [i, p] of layoutPills(b.badges, inner).entries()) {
    items.push(
      ...pillItems(main, i, p, { x: b.x + CARD.pad + p.x, y: b.y + CARD.pillsTop, z: at.z, band: at.band }, style),
    );
  }
  return items;
}

/** A frame (D20): an outlined rectangle with its label. */
export function frameItems(f: Frame, style: AppSceneStyle = appSceneStyle): ItemSpec[] {
  const id = frameItem(f.id);
  return [
    {
      kind: "rect",
      id,
      label: f.label,
      x: f.x,
      y: f.y,
      w: f.width,
      h: f.height,
      radius: 16,
      fill: style.frameFill,
      stroke: { color: style.frameStroke, width: 1 },
      z: FRAME_Z,
    },
    {
      kind: "text",
      id: `${id}::label`,
      x: f.x + 12,
      y: f.y + 10,
      w: Math.max(0, f.width - 24),
      text: f.label,
      size: 12,
      weight: "bold",
      maxLines: 1,
      color: style.secondary,
      z: FRAME_Z + 1,
    },
  ];
}

/** The keyboard focus ring around box `b` (WCAG 2.2 focus visible): the DOM list is visually hidden. */
export function focusItem(b: PlacedBox, style: AppSceneStyle = appSceneStyle): ItemSpec {
  return {
    kind: "rect",
    id: FOCUS_ITEM,
    x: b.x - 4,
    y: b.y - 4,
    w: BOX.width + 8,
    h: BOX.height + 8,
    radius: CARD.radius + 4,
    stroke: { color: style.focus, width: 2 },
    z: FOCUS_Z,
  };
}

/** One navigate step between screens of the app, as an arrow lane and as the reference the fixes edit. */
export interface AppLink {
  readonly edge: NavigateEdge;
  readonly use: Use;
  /** The route the step navigates to now. */
  readonly route: string;
  readonly sourcePlugin: string;
  readonly sourcePage: string;
  /** The target page and plugin; both undefined for a native target. */
  readonly targetPage?: string;
  readonly targetPlugin?: string;
}

export interface AppGraph {
  readonly links: readonly AppLink[];
  /** The box each graph node is drawn in: a page in its plugin, a native target in itself. */
  readonly boxOf: ReadonlyMap<string, string>;
  /** The native routes some link reaches, sorted. */
  readonly natives: readonly string[];
  readonly pages: ReadonlyMap<string, PageInfo>;
}

type Json = Record<string, unknown>;
const isObj = (v: unknown): v is Json => typeof v === "object" && v !== null && !Array.isArray(v);
const ROUTE_ACTIONS = new Set(["navigate", "openDialog", "openBottomSheet"]);
const esc = (t: string) => t.replaceAll("~", "~0").replaceAll("/", "~1");

/** Every route step below `value`, with the JSON pointer of the step. */
function routeSteps(value: unknown, pointer: string, out: { pointer: string; route: string }[]): void {
  if (Array.isArray(value)) {
    for (const [i, v] of value.entries()) {
      routeSteps(v, `${pointer}/${i}`, out);
    }
    return;
  }
  if (!isObj(value)) {
    return;
  }
  const input = value["input"];
  const route = isObj(input) ? input["route"] : undefined;
  if (typeof value["action"] === "string" && ROUTE_ACTIONS.has(value["action"]) && typeof route === "string") {
    out.push({ pointer, route });
  }
  for (const [k, v] of Object.entries(value)) {
    routeSteps(v, `${pointer}/${esc(k)}`, out);
  }
}

/** The event a step pointer sits under, e.g. `onPressed`. */
function eventOf(pointer: string): string {
  const parts = pointer.split("/");
  const at = parts.lastIndexOf("events");
  return at >= 0 ? (parts[at + 1] ?? "") : "";
}

/** One route step of page `from` as an arrow lane and the reference its fixes edit. */
function linkOf(
  d: LoadedDoc,
  from: PageInfo,
  s: { pointer: string; route: string },
  target: PageInfo | undefined,
  targetNode: string,
): AppLink {
  const event = eventOf(s.pointer);
  const ends = target === undefined ? {} : { targetPage: target.id, targetPlugin: target.pluginId };
  return {
    edge: {
      id: `${d.pluginId}:${d.path}#${s.pointer}`,
      source: from.id,
      target: targetNode,
      trigger: event === "" ? from.title : `${from.title} · ${event}`,
      title: `${from.title} → ${target?.title ?? s.route}`,
    },
    use: {
      kind: "navigates",
      pluginId: d.pluginId,
      path: d.path,
      fromId: from.id,
      fromTitle: from.title,
      pointer: `${s.pointer}/input/route`,
      step: s.pointer,
      target: { kind: "page", id: targetNode },
      ...(target === undefined ? {} : { targetPluginId: target.pluginId }),
    },
    route: s.route,
    sourcePlugin: from.pluginId,
    sourcePage: from.id,
    ...ends,
  };
}

/** The page a document is, if it is a page the index knows. */
function pageOf(d: LoadedDoc, pages: ReadonlyMap<string, PageInfo>): PageInfo | undefined {
  return d.kind === "page" && isObj(d.json) ? pages.get(String(d.json["id"])) : undefined;
}

/**
 * The navigate links of the app (STU-040): every route step of every page to a page of the app or a
 * native route. Links to unknown routes are not arrows; the validator reports them. Sorted by id.
 */
export function appGraph(docs: readonly LoadedDoc[], nativeRoutes: readonly string[]): AppGraph {
  const { pages } = buildWhereUsed(docs);
  const byRoute = new Map([...pages.values()].map((p) => [p.route, p]));
  const native = new Set(nativeRoutes);
  const boxOf = new Map<string, string>([...pages.values()].map((p) => [p.id, p.pluginId]));
  const natives = new Set<string>();
  const links: AppLink[] = [];
  for (const d of docs) {
    const from = pageOf(d, pages);
    if (from === undefined) {
      continue;
    }
    const steps: { pointer: string; route: string }[] = [];
    routeSteps(d.json, "", steps);
    for (const s of steps) {
      const target = byRoute.get(s.route);
      if (target === undefined && !native.has(s.route)) {
        continue;
      }
      const targetNode = target?.id ?? nativeBox(s.route);
      if (target === undefined) {
        natives.add(s.route);
        boxOf.set(targetNode, targetNode);
      }
      links.push(linkOf(d, from, s, target, targetNode));
    }
  }
  links.sort((a, b) => (a.edge.id < b.edge.id ? -1 : a.edge.id > b.edge.id ? 1 : 0));
  return { links, boxOf, natives: [...natives].sort(), pages };
}

/** "Redirect" on a lane (STU-046): the one step now navigates to `toRoute`, checked against its route. */
export function redirectLink(docs: readonly LoadedDoc[], link: AppLink, toRoute: string): ChangeSet {
  const from: PageInfo = { id: link.use.target.id, key: "", route: link.route, title: "", pluginId: "", path: "" };
  return redirectIncoming(docs, [link.use], from, toRoute);
}

/** "Remove" on a lane (STU-046): the one step is removed. */
export function removeLink(docs: readonly LoadedDoc[], link: AppLink): ChangeSet {
  return removeOutgoing(docs, [link.use]);
}
