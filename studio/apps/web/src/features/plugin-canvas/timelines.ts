// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Timelines on the canvas (PC-30, ANI-002, ANI-006): a page's timelines, each a set of tracks — the keyframes of one
 * prop of one widget — read and edited; the value of a track at any moment, eased into each keyframe by its curve
 * (numbers, colours and the numbers of an object such as an offset); and the page drawn at a moment of a timeline, so
 * it can be scrubbed and played on the canvas. A timeline driven by a scroll view follows that view's scroll offset.
 */

import { curveAt } from "./motion.ts";
import { findNode } from "./select/edits.ts";

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);
const num = (v: unknown): number | undefined => (typeof v === "number" && Number.isFinite(v) ? v : undefined);

export interface Keyframe {
  readonly atMs: number;
  readonly value: unknown;
  readonly curve?: string;
}

export interface Track {
  readonly node?: string;
  readonly prop: string;
  readonly keyframes: readonly Keyframe[];
}

export interface Timeline {
  readonly id: string;
  readonly name: string;
  readonly durationMs: number;
  readonly tracks: readonly Track[];
  readonly repeat?: number;
  readonly repeatForever?: boolean;
  readonly reverse?: boolean;
  readonly autoplay?: boolean;
  readonly driver?: { readonly kind: string; readonly node: string; readonly extent: number };
}

function trackOf(t: unknown): Track[] {
  if (!isObj(t) || typeof t["prop"] !== "string" || !Array.isArray(t["keyframes"])) {
    return [];
  }
  const keyframes = t["keyframes"]
    .filter(isObj)
    .map((k) => ({
      atMs: num(k["atMs"]) ?? 0,
      value: k["value"],
      ...(typeof k["curve"] === "string" ? { curve: k["curve"] } : {}),
    }))
    .sort((a, b) => a.atMs - b.atMs);
  return [{ ...(typeof t["node"] === "string" ? { node: t["node"] } : {}), prop: t["prop"], keyframes }];
}

/** A timeline's optional keys, as they are set. */
function optionsOf(t: Obj): Partial<Timeline> {
  const driver = isObj(t["driver"]) ? t["driver"] : undefined;
  const repeat = num(t["repeat"]);
  return {
    ...(repeat === undefined ? {} : { repeat }),
    ...(t["repeatForever"] === true ? { repeatForever: true } : {}),
    ...(t["reverse"] === true ? { reverse: true } : {}),
    ...(t["autoplay"] === true ? { autoplay: true } : {}),
    ...(driver === undefined
      ? {}
      : { driver: { kind: String(driver["kind"]), node: String(driver["node"]), extent: num(driver["extent"]) ?? 1 } }),
  };
}

function timelineOf(t: unknown): Timeline[] {
  if (!isObj(t) || typeof t["id"] !== "string") {
    return [];
  }
  return [
    {
      id: t["id"],
      name: String(t["name"] ?? t["id"]),
      durationMs: num(t["durationMs"]) ?? 1000,
      tracks: (Array.isArray(t["tracks"]) ? t["tracks"] : []).flatMap(trackOf),
      ...optionsOf(t),
    },
  ];
}

/** The timelines a page declares (`animations`). */
export function timelinesOf(page: unknown): Timeline[] {
  return isObj(page) && Array.isArray(page["animations"]) ? page["animations"].flatMap(timelineOf) : [];
}

/** A colour literal `#AARRGGBB` or `#RRGGBB` as its four channels, or undefined. */
function rgba(v: unknown): [number, number, number, number] | undefined {
  if (typeof v !== "string" || !/^#([0-9a-fA-F]{6}|[0-9a-fA-F]{8})$/.test(v)) {
    return undefined;
  }
  const hex = v.slice(1);
  const n = (i: number) => Number.parseInt(hex.slice(i, i + 2), 16);
  return hex.length === 6 ? [n(0), n(2), n(4), 255] : [n(0), n(2), n(4), n(6)];
}

const hex2 = (n: number) =>
  Math.round(Math.min(255, Math.max(0, n)))
    .toString(16)
    .padStart(2, "0")
    .toUpperCase();

/** The value between `a` and `b` at `t`: numbers and colours blend, objects blend their numbers, others switch. */
export function lerpValue(a: unknown, b: unknown, t: number): unknown {
  if (typeof a === "number" && typeof b === "number") {
    return a + (b - a) * t;
  }
  const ca = rgba(a);
  const cb = rgba(b);
  if (ca !== undefined && cb !== undefined) {
    return `#${ca.map((c, i) => hex2(c + ((cb[i] as number) - c) * t)).join("")}`;
  }
  if (isObj(a) && isObj(b) && a["$token"] === undefined && a["$expr"] === undefined) {
    return Object.fromEntries(Object.keys({ ...a, ...b }).map((k) => [k, lerpValue(a[k], b[k], t)]));
  }
  return t < 0.5 ? a : b;
}

/** A track's value at `ms`: before the first keyframe its first value, after the last its last, between eased. */
export function trackValue(track: Track, ms: number): unknown {
  const ks = track.keyframes;
  const first = ks[0];
  if (first === undefined) {
    return undefined;
  }
  if (ms <= first.atMs) {
    return first.value;
  }
  for (let i = 1; i < ks.length; i += 1) {
    const from = ks[i - 1] as Keyframe;
    const to = ks[i] as Keyframe;
    if (ms <= to.atMs) {
      const span = Math.max(1, to.atMs - from.atMs);
      return lerpValue(from.value, to.value, curveAt(to.curve, (ms - from.atMs) / span));
    }
  }
  return (ks[ks.length - 1] as Keyframe).value;
}

/** A copy of node `n` with the props its tracks animate at their values at `ms`. */
function withTracks(n: Obj, tracks: readonly Track[] | undefined, ms: number): Obj {
  if (tracks === undefined) {
    return { ...n };
  }
  const props: Obj = { ...(isObj(n["props"]) ? n["props"] : {}) };
  for (const t of tracks) {
    props[t.prop] = trackValue(t, ms);
  }
  return { ...n, props };
}

/** `root` drawn at `ms` of `timeline`: each track's widget with its prop at the track's value there. */
export function applyTimeline(root: unknown, timeline: Timeline, ms: number): unknown {
  const tracks = timeline.tracks.filter((t) => t.node !== undefined);
  if (tracks.length === 0 || !isObj(root)) {
    return root;
  }
  const byNode = new Map<string, Track[]>();
  for (const t of tracks) {
    byNode.set(t.node as string, [...(byNode.get(t.node as string) ?? []), t]);
  }
  const visit = (n: unknown): unknown => {
    if (Array.isArray(n)) {
      return n.map(visit);
    }
    if (!isObj(n)) {
      return n;
    }
    const out = withTracks(n, typeof n["id"] === "string" ? byNode.get(n["id"]) : undefined, ms);
    if (Array.isArray(n["children"])) {
      out["children"] = (n["children"] as unknown[]).map(visit);
    }
    if (isObj(n["slots"])) {
      out["slots"] = Object.fromEntries(Object.entries(n["slots"]).map(([k, v]) => [k, visit(v)]));
    }
    return out;
  };
  return visit(root);
}

/** Where a driven timeline stands for a scroll offset (ANI-006): the offset over the driver's extent, in ms. */
export function drivenMs(timeline: Timeline, offset: number): number {
  const extent = timeline.driver?.extent ?? 1;
  return Math.min(1, Math.max(0, offset / extent)) * timeline.durationMs;
}

/** The moment of a timeline `elapsed` ms after it started playing, with its repeats and reverse; undefined at its end. */
export function playedMs(timeline: Timeline, elapsed: number): number | undefined {
  const d = timeline.durationMs;
  const runs = timeline.repeatForever === true ? Number.POSITIVE_INFINITY : 1 + (timeline.repeat ?? 0);
  const run = Math.floor(elapsed / d);
  if (run >= runs) {
    return undefined;
  }
  const into = elapsed - run * d;
  return timeline.reverse === true && run % 2 === 1 ? d - into : into;
}

// ——— Edits (recipes over the page document) ———

function animations(page: Obj): Obj[] {
  if (!Array.isArray(page["animations"])) {
    page["animations"] = [];
  }
  return page["animations"] as Obj[];
}

const timelineAt = (page: Obj, id: string): Obj | undefined => animations(page).find((t) => t["id"] === id);

export const addTimelineRecipe =
  (id: string, name: string) =>
  (page: Obj): void => {
    animations(page).push({ id, name, durationMs: 1000, tracks: [] });
  };

export const removeTimelineRecipe =
  (id: string) =>
  (page: Obj): void => {
    const list = animations(page);
    const i = list.findIndex((t) => t["id"] === id);
    if (i >= 0) {
      list.splice(i, 1);
    }
    if (list.length === 0) {
      delete page["animations"];
    }
  };

/** Sets a key of a timeline (`name`, `durationMs`, `repeat`, `repeatForever`, `reverse`, `autoplay`, `driver`). */
export const setTimelineRecipe =
  (id: string, key: string, value: unknown) =>
  (page: Obj): void => {
    const t = timelineAt(page, id);
    if (t === undefined) {
      return;
    }
    if (value === undefined || value === false) {
      delete t[key];
    } else {
      t[key] = value;
    }
  };

/**
 * Puts a keyframe of `node`'s `prop` at `atMs` with `value` (PC-30): a new track starts from `from` at 0, so it has the
 * two keyframes a track needs; a keyframe already at that moment takes the value.
 */
export const putKeyframeRecipe =
  (id: string, node: string, prop: string, atMs: number, value: unknown, from: unknown) =>
  (page: Obj): void => {
    const t = timelineAt(page, id);
    if (t === undefined) {
      return;
    }
    const tracks = Array.isArray(t["tracks"]) ? (t["tracks"] as Obj[]) : [];
    t["tracks"] = tracks;
    let track = tracks.find((k) => k["node"] === node && k["prop"] === prop);
    if (track === undefined) {
      track = { node, prop, keyframes: atMs === 0 ? [] : [{ atMs: 0, value: from }] };
      tracks.push(track);
    }
    const keyframes = track["keyframes"] as Obj[];
    const at = keyframes.find((k) => k["atMs"] === atMs);
    if (at === undefined) {
      keyframes.push({ atMs, value });
      keyframes.sort((a, b) => (num(a["atMs"]) ?? 0) - (num(b["atMs"]) ?? 0));
    } else {
      at["value"] = value;
    }
    if (keyframes.length < 2) {
      keyframes.push({ atMs: num(t["durationMs"]) ?? 1000, value });
    }
  };

/** Removes the keyframe at `atMs` of a track; a track left with fewer than two keyframes goes. */
export const removeKeyframeRecipe =
  (id: string, node: string | undefined, prop: string, atMs: number) =>
  (page: Obj): void => {
    const t = timelineAt(page, id);
    const tracks = t !== undefined && Array.isArray(t["tracks"]) ? (t["tracks"] as Obj[]) : [];
    const i = tracks.findIndex((k) => k["node"] === node && k["prop"] === prop);
    const track = tracks[i];
    if (track === undefined) {
      return;
    }
    const keyframes = (track["keyframes"] as Obj[]).filter((k) => k["atMs"] !== atMs);
    if (keyframes.length < 2) {
      tracks.splice(i, 1);
    } else {
      track["keyframes"] = keyframes;
    }
  };

/** The value of `prop` that node `id` holds now, for a new track's first keyframe. */
export function currentValue(root: unknown, id: string, prop: string): unknown {
  const node = findNode(root, id)?.node as Obj | undefined;
  return isObj(node?.["props"]) ? (node["props"] as Obj)[prop] : undefined;
}

const SCROLL_VIEWS: ReadonlySet<string> = new Set([
  "ListView",
  "GridView",
  "SingleChildScrollView",
  "CustomScrollView",
]);

/** The children and slot contents of a node. */
const kidsOf = (n: Obj): unknown[] => [
  ...(Array.isArray(n["children"]) ? n["children"] : []),
  ...(isObj(n["slots"]) ? Object.values(n["slots"]).flatMap((v) => (Array.isArray(v) ? v : [v])) : []),
];

/** The scroll views of a page a timeline can be driven by (ANI-006), named by type and the end of their id. */
export function scrollViewsOf(root: unknown): { id: string; name: string }[] {
  const out: { id: string; name: string }[] = [];
  const visit = (n: unknown) => {
    if (!isObj(n)) {
      return;
    }
    if (typeof n["id"] === "string" && SCROLL_VIEWS.has(String(n["type"]))) {
      out.push({ id: n["id"], name: `${String(n["type"])} · ${n["id"].slice(-4)}` });
    }
    kidsOf(n).forEach(visit);
  };
  visit(root);
  return out;
}
