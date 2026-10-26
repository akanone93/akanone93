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
import { BOX, type Frame, type PlacedBox } from "./app-canvas.model.ts";

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
}

/** The id of a native target's box. */
export const nativeBox = (route: string): string => `native:${route}`;

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
  readonly stroke: ColorRgba;
  readonly title: ColorRgba;
  readonly secondary: ColorRgba;
  readonly frameStroke: ColorRgba;
  readonly focus: ColorRgba;
  readonly pills: Readonly<Record<BadgeStatus, Pill>>;
}

export const appSceneStyle: AppSceneStyle = {
  fill: rgba(primitives.white),
  archivedFill: rgba(primitives["porcelain.100"]),
  nativeFill: rgba(primitives["violet.700"], 0x14),
  stroke: rgba(primitives["navy.alpha16"]),
  title: rgba(primitives["navy.900"]),
  secondary: rgba(primitives["navy.500"]),
  frameStroke: rgba(primitives["navy.900"], 0x60),
  focus: rgba(primitives["violet.700"]),
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

/** Depths: frames under arrows (z 1), boxes over them, the expanded bundle (z 11) and focus on top. */
const FRAME_Z = 0;
const BOX_Z = 4;
const FOCUS_Z = 20;
const PAD = 12;
const PILL_H = 18;
const PILL_ROW = 20;
const PILLS_TOP = 52;
/** An upper bound of the advance of one character at 11 px; the worker lays the text out exactly. */
const PILL_CHAR = 6.2;

/** Pill rows that fit in a box; the pill rows cannot hold beyond this. */
const MAX_PILL_ROWS = 3;

interface PlacedPill extends Badge {
  readonly x: number;
  readonly row: number;
  readonly w: number;
}

const pillWidth = (label: string, inner: number): number => Math.min(inner, Math.ceil(16 + label.length * PILL_CHAR));

/**
 * Badges wrapped into at most three rows of `inner` width. What does not fit is summed up in a final
 * "+N" pill; the accessible layer and the detail card still name every badge.
 */
export function layoutPills(badges: readonly Badge[], inner: number): PlacedPill[] {
  const out: PlacedPill[] = [];
  let x = 0;
  let row = 0;
  for (const badge of badges) {
    const w = pillWidth(badge.label, inner);
    if (x > 0 && x + w > inner) {
      x = 0;
      row++;
    }
    if (row >= MAX_PILL_ROWS) {
      break;
    }
    out.push({ ...badge, x, row, w });
    x += w + 4;
  }
  while (out.length < badges.length) {
    const more = `+${badges.length - out.length}`;
    const last = out.at(-1);
    const at = last === undefined ? 0 : last.x + last.w + 4;
    const w = pillWidth(more, inner);
    if (last === undefined || at + w <= inner) {
      out.push({ status: "neutral", label: more, x: at, row: last?.row ?? 0, w });
      break;
    }
    out.pop();
  }
  return out;
}

/** The items of one box: rectangle, title, key and its badges as pills, wrapped within the box. */
export function boxItems(b: SceneBox, style: AppSceneStyle = appSceneStyle): ItemSpec[] {
  const main = boxItem(b.id);
  const inner = BOX.width - 2 * PAD;
  const items: ItemSpec[] = [
    {
      kind: "rect",
      id: main,
      label: b.title,
      x: b.x,
      y: b.y,
      w: BOX.width,
      h: BOX.height,
      radius: 8,
      fill: b.native ? style.nativeFill : b.archived ? style.archivedFill : style.fill,
      stroke: { color: style.stroke, width: 1 },
      z: BOX_Z,
    },
    {
      kind: "text",
      id: `${main}::title`,
      x: b.x + PAD,
      y: b.y + 10,
      w: inner,
      text: b.title,
      size: 14,
      color: style.title,
      z: BOX_Z + 1,
    },
    {
      kind: "text",
      id: `${main}::key`,
      x: b.x + PAD,
      y: b.y + 30,
      w: inner,
      text: b.key,
      size: 12,
      color: style.secondary,
      z: BOX_Z + 1,
    },
  ];
  for (const [i, p] of layoutPills(b.badges, inner).entries()) {
    const pill = style.pills[p.status];
    const x = b.x + PAD + p.x;
    const y = b.y + PILLS_TOP + p.row * PILL_ROW;
    items.push(
      {
        kind: "rect",
        id: `${main}::badge${i}`,
        x,
        y,
        w: p.w,
        h: PILL_H,
        radius: PILL_H / 2,
        fill: pill.fill,
        stroke: { color: pill.stroke, width: 1 },
        z: BOX_Z + 1,
      },
      {
        kind: "text",
        id: `${main}::badge${i}t`,
        x: x + 8,
        y: y + 3,
        w: p.w - 16,
        text: p.label,
        size: 11,
        color: pill.text,
        z: BOX_Z + 2,
      },
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
      radius: 8,
      stroke: { color: style.frameStroke, width: 1 },
      z: FRAME_Z,
    },
    {
      kind: "text",
      id: `${id}::label`,
      x: f.x + 8,
      y: f.y + 8,
      w: Math.max(0, f.width - 16),
      text: f.label,
      size: 12,
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
    radius: 10,
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
