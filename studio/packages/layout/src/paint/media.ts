// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * What the data and media components paint (STU-005, WGT-021, WGT-031): charts drawn from their (mock) data in the
 * scheme's colours — bars, lines, sparklines and pie wedges with a baseline and labels — and the previews of what only
 * the device shows: a map's tiles, a camera's viewfinder, a video's first frame, an audio player, a QR code, an
 * animation and a native slot, each with the glyph of what it stands for.
 */

import type { ColorRole } from "../material-color/scheme.ts";
import type { PaintText, PartsPaint, PlacedGlyph } from "./fields.ts";
import { assetOf } from "./media-source.ts";
import { type BoxPaint, type LinePaint, type PaintTheme, type Rect, type Rgba, roleColor } from "./paint.ts";
import { M3_TYPE_SCALE, type ResolvedText, typeScaleOf } from "./text-style.ts";

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);
const num = (v: unknown): number | undefined => (typeof v === "number" && Number.isFinite(v) ? v : undefined);

/** What a media painter returns: parts, and the line segments of a line chart. */
export interface MediaPaint extends PartsPaint {
  readonly lines: readonly LinePaint[];
}

const SERIES: readonly ColorRole[] = ["primary", "tertiary", "secondary", "error"];
const seriesColor = (theme: PaintTheme, i: number): Rgba => roleColor(theme, SERIES[i % SERIES.length] as ColorRole);

const small = (theme: PaintTheme): ResolvedText => ({
  ...(typeScaleOf(theme).labelSmall ?? M3_TYPE_SCALE.labelSmall),
  italic: false,
});

const glyph = (at: Rect, name: string, size: number, color: Rgba): PlacedGlyph => ({
  x: at.x + (at.w - size) / 2,
  y: at.y + (at.h - size) / 2,
  w: size,
  h: size,
  name,
  set: "material",
  size,
  color,
});

const empty = (boxes: BoxPaint[] = [], glyphs: PlacedGlyph[] = [], texts: PaintText[] = []): MediaPaint => ({
  boxes,
  texts,
  glyphs,
  lines: [],
});

/** The values of each series of a chart, the longest first in length. */
function seriesOf(props: Obj): number[][] {
  const list = Array.isArray(props["series"]) ? props["series"] : [];
  return list.filter(isObj).map((s) => (Array.isArray(s["values"]) ? s["values"].map((v) => num(v) ?? 0) : []));
}

/** A chart's plot area: below its title row when it has one, above a row of labels. */
function plotOf(props: Obj, r: Rect): Rect {
  const top = typeof props["title"] === "string" ? 28 : 8;
  const bottom = Array.isArray(props["labels"]) ? 20 : 8;
  return { x: r.x + 8, y: r.y + top, w: Math.max(0, r.w - 16), h: Math.max(0, r.h - top - bottom) };
}

function labelsOf(props: Obj, plot: Rect, count: number, theme: PaintTheme): PaintText[] {
  const labels = Array.isArray(props["labels"]) ? props["labels"] : [];
  const step = count === 0 ? 0 : plot.w / count;
  return labels.slice(0, count).flatMap((l, i): PaintText[] =>
    typeof l === "string"
      ? [
          {
            text: l,
            x: plot.x + step * i,
            y: plot.y + plot.h + 4,
            w: step,
            style: small(theme),
            color: roleColor(theme, "onSurfaceVariant"),
          },
        ]
      : [],
  );
}

const baseline = (plot: Rect, theme: PaintTheme): BoxPaint => ({
  x: plot.x,
  y: plot.y + plot.h,
  w: plot.w,
  h: 1,
  fill: roleColor(theme, "outlineVariant"),
});

/** A bar chart: each value a bar in its series' colour, grouped by index, scaled to the largest value. */
function barChart(props: Obj, r: Rect, theme: PaintTheme): MediaPaint {
  const series = seriesOf(props);
  const count = Math.max(0, ...series.map((s) => s.length));
  const max = Math.max(1e-9, ...series.flat());
  const plot = plotOf(props, r);
  const group = count === 0 ? 0 : plot.w / count;
  const bar = series.length === 0 ? 0 : (group * 0.7) / series.length;
  const boxes: BoxPaint[] = [baseline(plot, theme)];
  series.forEach((values, s) => {
    values.forEach((v, i) => {
      const h = (Math.max(0, v) / max) * plot.h;
      const x = plot.x + group * i + group * 0.15 + bar * s;
      boxes.push({
        x,
        y: plot.y + plot.h - h,
        w: Math.max(1, bar - 2),
        h,
        radii: [4, 4, 0, 0],
        fill: seriesColor(theme, s),
      });
    });
  });
  return { boxes, texts: labelsOf(props, plot, count, theme), glyphs: [], lines: [] };
}

/** A line chart or a sparkline: each series a 2 pixel line through its values, with a dot at each in a chart. */
function lineChart(values: number[][], props: Obj, r: Rect, theme: PaintTheme, dots: boolean): MediaPaint {
  const count = Math.max(0, ...values.map((s) => s.length));
  const all = values.flat();
  const max = Math.max(...all, 1e-9);
  const min = Math.min(...all, 0);
  const plot = dots ? plotOf(props, r) : { x: r.x + 2, y: r.y + 2, w: r.w - 4, h: r.h - 4 };
  const at = (v: number, i: number) => ({
    x: plot.x + (count <= 1 ? plot.w / 2 : (plot.w * i) / (count - 1)),
    y: plot.y + plot.h - ((v - min) / (max - min || 1)) * plot.h,
  });
  const lines: LinePaint[] = [];
  const boxes: BoxPaint[] = dots ? [baseline(plot, theme)] : [];
  values.forEach((s, k) => {
    const color = seriesColor(theme, k);
    s.forEach((v, i) => {
      const p = at(v, i);
      const next = s[i + 1];
      if (next !== undefined) {
        const q = at(next, i + 1);
        lines.push({ x1: p.x, y1: p.y, x2: q.x, y2: q.y, color, width: 2 });
      }
      if (dots) {
        boxes.push({ x: p.x - 3, y: p.y - 3, w: 6, h: 6, oval: true, fill: color });
      }
    });
  });
  return { boxes, texts: dots ? labelsOf(props, plot, count, theme) : [], glyphs: [], lines };
}

/** A pie chart: a wedge per slice in the series' colours, clockwise from 12 o'clock. */
function pieChart(props: Obj, r: Rect, theme: PaintTheme): MediaPaint {
  const slices = (Array.isArray(props["slices"]) ? props["slices"] : [])
    .filter(isObj)
    .map((s) => Math.max(0, num(s["value"]) ?? 0));
  const total = slices.reduce((a, b) => a + b, 0);
  const plot = plotOf(props, r);
  const size = Math.min(plot.w, plot.h);
  const box = { x: plot.x + (plot.w - size) / 2, y: plot.y + (plot.h - size) / 2, w: size, h: size };
  let start = -90;
  const boxes = slices.map((v, i): BoxPaint => {
    const sweep = total === 0 ? 0 : (v / total) * 360;
    const wedge = { ...box, oval: true, fill: seriesColor(theme, i), arc: { start, sweep } };
    start += sweep;
    return wedge;
  });
  return empty(boxes);
}

/** A map's preview: its tiles' ground with a grid of streets, and a pin at its centre. */
function mapPreview(r: Rect, theme: PaintTheme): MediaPaint {
  const boxes: BoxPaint[] = [{ ...r, fill: roleColor(theme, "surfaceContainerHighest") }];
  const line = roleColor(theme, "outlineVariant");
  for (let x = r.x + 40; x < r.x + r.w; x += 56) {
    boxes.push({ x, y: r.y, w: 2, h: r.h, fill: line });
  }
  for (let y = r.y + 32; y < r.y + r.h; y += 48) {
    boxes.push({ x: r.x, y, w: r.w, h: 2, fill: line });
  }
  return empty(boxes, [glyph(r, "location_on", 36, roleColor(theme, "primary"))]);
}

/** A camera's preview: dark, with a viewfinder's four white corners and its hint. */
function scannerPreview(props: Obj, r: Rect, theme: PaintTheme): MediaPaint {
  const side = Math.min(r.w, r.h) * 0.6;
  const v = { x: r.x + (r.w - side) / 2, y: r.y + (r.h - side) / 2, w: side, h: side };
  const white = 0xffffffff;
  const arm = Math.max(12, side / 6);
  const corners: BoxPaint[] = [
    [v.x, v.y, arm, 3],
    [v.x, v.y, 3, arm],
    [v.x + v.w - arm, v.y, arm, 3],
    [v.x + v.w - 3, v.y, 3, arm],
    [v.x, v.y + v.h - 3, arm, 3],
    [v.x, v.y + v.h - arm, 3, arm],
    [v.x + v.w - arm, v.y + v.h - 3, arm, 3],
    [v.x + v.w - 3, v.y + v.h - arm, 3, arm],
  ].map(([x, y, w, h]) => ({ x: x as number, y: y as number, w: w as number, h: h as number, fill: white }));
  const hint = typeof props["overlayHint"] === "string" ? props["overlayHint"] : undefined;
  const body = { ...(typeScaleOf(theme).bodyMedium ?? M3_TYPE_SCALE.bodyMedium), italic: false };
  const texts: PaintText[] =
    hint === undefined ? [] : [{ text: hint, x: r.x + 16, y: v.y + v.h + 16, w: r.w - 32, style: body, color: white }];
  return empty([{ ...r, fill: 0x121212ff }, ...corners], [], texts);
}

/** The painters of the data and media components, by type. */
export function mediaPaint(node: { type?: string; props?: Obj }, r: Rect, theme: PaintTheme): MediaPaint | undefined {
  const props = node.props ?? {};
  const onVariant = roleColor(theme, "onSurfaceVariant");
  switch (node.type) {
    case "BarChart":
      return barChart(props, r, theme);
    case "LineChart":
      return lineChart(seriesOf(props), props, r, theme, true);
    case "Sparkline":
      return lineChart(
        [Array.isArray(props["values"]) ? props["values"].map((v) => num(v) ?? 0) : []],
        props,
        r,
        theme,
        false,
      );
    case "PieChart":
      return pieChart(props, r, theme);
    case "MapView":
    case "OrderTrackerMap":
      return mapPreview(r, theme);
    case "QrScanner":
    case "BarcodeScanner":
    case "KycCameraCapture":
      return scannerPreview(props, r, theme);
    case "VideoPlayer":
      return empty([{ ...r, fill: 0x000000ff }], [glyph(r, "play_circle", Math.min(56, r.h / 2), 0xffffffff)]);
    case "AudioPlayer":
      return empty(
        [{ ...r, radii: [12, 12, 12, 12], fill: roleColor(theme, "surfaceContainerHighest") }],
        [glyph({ x: r.x + 8, y: r.y, w: 40, h: r.h }, "play_arrow", 24, roleColor(theme, "primary"))],
      );
    case "QrCodeView":
      return empty([{ ...r, fill: 0xffffffff }], [glyph(r, "qr_code_2", Math.min(r.w, r.h) * 0.9, 0x000000ff)]);
    case "LottieView":
    case "RiveView":
      return empty(
        [{ ...r, radii: [12, 12, 12, 12], fill: roleColor(theme, "surfaceContainerHighest") }],
        [glyph(r, "animation", 32, onVariant)],
      );
    case "NativeSlot":
      return empty(
        [
          {
            ...r,
            radii: [12, 12, 12, 12],
            border: { kind: "all", side: { color: roleColor(theme, "outline"), width: 1 } },
          },
        ],
        [glyph(r, "extension", 28, onVariant)],
      );
    case "ImageGallery": {
      const first = Array.isArray(props["images"]) ? props["images"][0] : undefined;
      const asset = assetOf(first);
      return empty([
        {
          ...r,
          radii: [12, 12, 12, 12],
          fill: roleColor(theme, "surfaceContainerHighest"),
          ...(asset === undefined ? {} : { image: { asset, fit: "cover" as const, alignX: 0, alignY: 0 } }),
        },
      ]);
    }
    default:
      return undefined;
  }
}
