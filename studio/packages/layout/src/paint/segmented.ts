// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * A `SegmentedButton` as Material 3 draws it (build log #127): a pill 40 high in the middle of its 48-pixel tap
 * target, outlined, its segments as wide as one another (the scene sizes them to the widest), divided by the outline,
 * the selected ones filled in `secondaryContainer` with a check before their label. The scene lays the button out as
 * Flutter's goldens have it; this draws it.
 */

import type { PaintText, PlacedGlyph } from "./fields.ts";
import { type BoxPaint, type PaintTheme, type Rect, roleColor } from "./paint.ts";
import { M3_TYPE_SCALE, type ResolvedText, typeScaleOf } from "./text-style.ts";

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);

/** The track's height, the check's size and the room around the check, as Material 3's segmented button has them. */
const TRACK = 40;
const CHECK = 18;
const PAD = 12;
const GAP = 8;

/** What a segmented button paints: its boxes, its labels and its checks. */
export interface SegmentedPaint {
  readonly boxes: readonly BoxPaint[];
  readonly texts: readonly PaintText[];
  readonly glyphs: readonly PlacedGlyph[];
}

/** The segments of `props` with whether each is selected; none when `segments` is not a list. */
function segmentsOf(props: Obj): { label: string; selected: boolean }[] {
  const selected = Array.isArray(props["selected"]) ? props["selected"] : [];
  const segments = Array.isArray(props["segments"]) ? props["segments"] : [];
  return segments.filter(isObj).map((s) => ({
    label: typeof s["label"] === "string" ? s["label"] : "",
    selected: selected.includes(s["value"]),
  }));
}

/** What every segment shares: the track, a segment's width, the label's style and the outline. */
interface Track {
  readonly rect: Rect;
  readonly width: number;
  readonly count: number;
  readonly rtl: boolean;
  readonly style: ResolvedText;
  readonly showCheck: boolean;
}

/** The fill of a selected segment, rounded at the track's ends. */
function selectedFill(seg: Rect, first: boolean, last: boolean, theme: PaintTheme): BoxPaint {
  const r = TRACK / 2;
  return {
    ...seg,
    radii: [first ? r : 0, last ? r : 0, last ? r : 0, first ? r : 0],
    fill: roleColor(theme, "secondaryContainer"),
  };
}

/** One segment's fill, divider, check and label into `out`. */
function drawSegment(
  segment: { readonly label: string; readonly selected: boolean },
  i: number,
  t: Track,
  theme: PaintTheme,
  out: { boxes: BoxPaint[]; texts: PaintText[]; glyphs: PlacedGlyph[] },
): void {
  // In a right-to-left layout the first segment stands at the right.
  const at = t.rtl ? t.count - 1 - i : i;
  const seg: Rect = { x: t.rect.x + at * t.width, y: t.rect.y, w: t.width, h: TRACK };
  if (segment.selected) {
    out.boxes.push(selectedFill(seg, at === 0, at === t.count - 1, theme));
  }
  if (at > 0) {
    out.boxes.push({ x: seg.x, y: seg.y, w: 1, h: TRACK, fill: roleColor(theme, "outline") });
  }
  const color = roleColor(theme, segment.selected ? "onSecondaryContainer" : "onSurface");
  const check = segment.selected && t.showCheck;
  if (check) {
    const y = seg.y + (TRACK - CHECK) / 2;
    out.glyphs.push({ name: "check", set: "material", size: CHECK, color, x: seg.x + PAD, y, w: CHECK, h: CHECK });
  }
  if (segment.label === "") {
    return;
  }
  const start = seg.x + PAD + (check ? CHECK + GAP : 0);
  const lineHeight = t.style.size * t.style.height;
  out.texts.push({
    text: segment.label,
    x: start,
    y: seg.y + (TRACK - lineHeight) / 2,
    w: Math.max(0, seg.x + seg.w - PAD - start),
    style: t.style,
    color,
    align: "center",
  });
}

/** The paint of a `SegmentedButton` in `r`; undefined when it has no segments. */
export function segmentedPaint(props: Obj, r: Rect, theme: PaintTheme): SegmentedPaint | undefined {
  const segments = segmentsOf(props);
  if (segments.length === 0) {
    return undefined;
  }
  const rect: Rect = { x: r.x, y: r.y + (r.h - TRACK) / 2, w: r.w, h: TRACK };
  const track: Track = {
    rect,
    width: rect.w / segments.length,
    count: segments.length,
    rtl: theme.rtl === true,
    style: { ...(typeScaleOf(theme).labelLarge ?? M3_TYPE_SCALE.labelLarge), italic: false },
    showCheck: props["showSelectedIcon"] !== false,
  };
  const out = { boxes: [] as BoxPaint[], texts: [] as PaintText[], glyphs: [] as PlacedGlyph[] };
  segments.forEach((segment, i) => {
    drawSegment(segment, i, track, theme, out);
  });
  const radius = TRACK / 2;
  out.boxes.push({
    ...rect,
    radii: [radius, radius, radius, radius],
    border: { kind: "all", side: { color: roleColor(theme, "outline"), width: 1 } },
  });
  return out;
}
