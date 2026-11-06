// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * A device's system areas drawn over a screen (PC-11, STU-057): the status bar with the time, signal, Wi-Fi and
 * battery, the camera cutout (a punch hole, a notch or an island), and the home indicator, the gesture handle or the
 * three navigation buttons. Their icons take the colour that reads over what the page paints behind them, as a page's
 * `SystemUiOverlayStyle` follows its app bar.
 */

import type { BoxPaint, Placed } from "@plux/layout";
import type { DeviceChromeSpec, ItemSpec, RectSpec, ScreenDocument, TextSpec } from "./protocol.ts";

/** The scene id of a part of a screen's chrome; no document node owns it, so a click passes to the page below. */
export const chromeId = (screen: string, part: string): string => `chrome:${screen}#${part}`;

/** Chrome is drawn above every widget of the page, as the system draws over the app. */
const CHROME_Z = 9000;

/** The relative luminance of a 0xRRGGBBAA colour (WCAG). */
function luminance(rgba: number): number {
  const channel = (shift: number) => {
    const c = ((rgba >>> shift) & 0xff) / 255;
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  };
  return 0.2126 * channel(24) + 0.7152 * channel(16) + 0.0722 * channel(8);
}

/**
 * The colour a page paints at `(x, y)` of the screen: the fill of the topmost opaque painted box there, else the
 * screen's own background.
 */
export function backdropAt(boxes: readonly Placed[], x: number, y: number, background: number): number {
  let best: { order: number; fill: number } | undefined;
  for (const b of boxes) {
    const paint = b.paint;
    if (paint === undefined || paint.role !== "paint" || paint.opacity < 1) {
      continue;
    }
    const fill = solidFillAt(paint.boxes, x, y);
    if (fill !== undefined && (best === undefined || paint.order >= best.order)) {
      best = { order: paint.order, fill };
    }
  }
  return best?.fill ?? background;
}

/** The last opaque rectangular fill among `boxes` that covers `(x, y)`. */
function solidFillAt(boxes: readonly BoxPaint[], x: number, y: number): number | undefined {
  let out: number | undefined;
  for (const box of boxes) {
    const fill = box.fill ?? box.gradient?.colors[0];
    const inside = x >= box.x && x <= box.x + box.w && y >= box.y && y <= box.y + box.h;
    if (fill !== undefined && (fill & 0xff) === 0xff && inside && box.arc === undefined && box.oval !== true) {
      out = fill;
    }
  }
  return out;
}

/** The icon colour over `backdrop`: dark glyphs on a light surface, light glyphs on a dark one. */
export function glyphColour(chrome: DeviceChromeSpec, backdrop: number): number {
  return luminance(backdrop) > 0.4 ? chrome.onLight : chrome.onDark;
}

interface Painter {
  readonly s: ScreenDocument;
  readonly chrome: DeviceChromeSpec;
  readonly frame: string;
  readonly families: readonly string[];
  readonly items: ItemSpec[];
}

function rect(p: Painter, part: string, r: Omit<RectSpec, "kind" | "id" | "z" | "frame">): void {
  p.items.push({ kind: "rect", id: chromeId(p.s.id, part), z: CHROME_Z, frame: p.frame, ...r });
}

function text(p: Painter, part: string, t: Omit<TextSpec, "kind" | "id" | "z" | "frame" | "families">): void {
  p.items.push({ kind: "text", id: chromeId(p.s.id, part), z: CHROME_Z, frame: p.frame, families: p.families, ...t });
}

/** Signal bars rising to the right, their bottoms on `base`, starting at `x`; returns their width. */
function signal(p: Painter, x: number, base: number, colour: number, scale: number): number {
  const heights = [0.36, 0.55, 0.77, 1].map((f) => f * 10.5 * scale);
  const bar = 3 * scale;
  const gap = 1.5 * scale;
  heights.forEach((h, i) => {
    rect(p, `signal${i}`, { x: x + i * (bar + gap), y: base - h, w: bar, h, radius: scale, fill: colour });
  });
  return 4 * bar + 3 * gap;
}

/** Wi-Fi as three arcs over a wedge, centred on `cx` with its point on `base`; returns its width. */
function wifi(p: Painter, cx: number, base: number, colour: number, scale: number): number {
  const radii = [3.2, 6.6, 10].map((r) => r * scale);
  radii.forEach((r, i) => {
    const arc = i === 0 ? { start: 225, sweep: 90 } : { start: 225, sweep: 90, width: 2 * scale };
    rect(p, `wifi${i}`, { x: cx - r, y: base - r, w: 2 * r, h: 2 * r, radius: 0, oval: true, fill: colour, arc });
  });
  return 2 * 10 * scale * Math.SQRT1_2;
}

/** A battery on its side (iOS) or upright (Android), filled to 80 %, at `x`; returns its width. */
function battery(p: Painter, x: number, mid: number, colour: number, upright: boolean): number {
  if (upright) {
    const [w, h] = [8, 14];
    rect(p, "battery-cap", { x: x + 2.5, y: mid - h / 2 - 1.5, w: 3, h: 2, radius: 0.5, fill: colour });
    rect(p, "battery", { x, y: mid - h / 2, w, h, radius: 1.5, fill: colour });
    return w;
  }
  const [w, h] = [25, 12];
  rect(p, "battery", {
    x,
    y: mid - h / 2,
    w,
    h,
    radius: 3.5,
    stroke: { color: (colour & 0xffffff00) | 0x60, width: 1 },
  });
  rect(p, "battery-level", { x: x + 2, y: mid - h / 2 + 2, w: (w - 4) * 0.8, h: h - 4, radius: 2, fill: colour });
  rect(p, "battery-cap", { x: x + w + 1, y: mid - 2, w: 1.5, h: 4, radius: 0.75, fill: (colour & 0xffffff00) | 0x80 });
  return w + 2.5;
}

/** The cutout's width at the top of a portrait screen, for placing the status bar's two sides around it. */
function cutoutWidth(chrome: DeviceChromeSpec): number {
  switch (chrome.cutout) {
    case "notch":
      return 162;
    case "island":
      return 126;
    case "punch":
      return 12;
    default:
      return 0;
  }
}

/** The camera at the left edge of a landscape screen. */
function sideCutout(p: Painter, height: number): void {
  const { chrome, s } = p;
  const across = cutoutWidth(chrome);
  const notch = chrome.cutout === "notch";
  const deep = notch ? 30 : chrome.cutout === "island" ? 37 : 12;
  const x = chrome.cutout === "island" ? s.x + 11 : notch ? s.x : s.x + 10;
  const r = deep / 2;
  const radii: [number, number, number, number] = notch ? [0, 20, 20, 0] : [r, r, r, r];
  rect(p, "cutout", {
    x,
    y: s.y + (height - across) / 2,
    w: deep,
    h: across,
    radius: 0,
    radii,
    fill: chrome.cutoutColor,
  });
}

function cutout(p: Painter, width: number, height: number): void {
  const { chrome, s } = p;
  const fill = chrome.cutoutColor;
  if (chrome.landscape) {
    sideCutout(p, height);
    return;
  }
  const w = cutoutWidth(chrome);
  const x = s.x + (width - w) / 2;
  if (chrome.cutout === "notch") {
    rect(p, "cutout", { x, y: s.y, w, h: 32, radius: 0, radii: [0, 0, 20, 20], fill });
  } else if (chrome.cutout === "island") {
    rect(p, "cutout", { x, y: s.y + 11, w, h: 37, radius: 18.5, fill });
  } else if (chrome.cutout === "punch") {
    rect(p, "cutout", { x, y: s.y + (chrome.top - w) / 2, w, h: w, radius: 0, oval: true, fill });
  }
}

/** iOS: the time in the left ear (or centred without a cutout), signal, Wi-Fi and battery in the right one. */
function cupertinoStatus(p: Painter, width: number, colour: number): void {
  const { chrome, s } = p;
  const top = chrome.top;
  const ear = (width - cutoutWidth(chrome)) / 2;
  const small = chrome.cutout === "none";
  const size = small ? 12 : 17;
  const mid = s.y + (small ? top / 2 : Math.min(top / 2 + 2, 30));
  if (small) {
    text(p, "time", {
      x: s.x,
      y: mid - size / 2,
      w: width,
      text: chrome.time,
      size,
      weight: 600,
      height: 1,
      align: "center",
      color: colour,
    });
    const sw = signal(p, s.x + 6, mid + 4.5, colour, 0.85);
    wifi(p, s.x + 6 + sw + 12, mid + 4, colour, 0.85);
    battery(p, s.x + width - 6 - 27.5, mid, colour, false);
    return;
  }
  text(p, "time", {
    x: s.x,
    y: mid - size / 2,
    w: ear,
    text: chrome.time,
    size,
    weight: 600,
    height: 1,
    align: "center",
    color: colour,
  });
  const end = s.x + width - (ear - 78) / 2;
  const bw = battery(p, end - 27.5, mid, colour, false);
  wifi(p, end - bw - 5 - 7, mid + 4.5, colour, 0.9);
  signal(p, end - bw - 5 - 14 - 5 - 17, mid + 5, colour, 1);
}

/** Android: the time at the start, Wi-Fi, signal and an upright battery at the end. */
function materialStatus(p: Painter, width: number, colour: number): void {
  const { chrome, s } = p;
  const mid = s.y + chrome.top / 2;
  const inset = 16 + chrome.left;
  text(p, "time", {
    x: s.x + inset,
    y: mid - 7,
    w: 64,
    text: chrome.time,
    size: 14,
    weight: 500,
    height: 1,
    color: colour,
  });
  const end = s.x + width - 16 - chrome.right;
  const bw = battery(p, end - 8, mid, colour, true);
  const sw = signal(p, end - bw - 6 - 13, mid + 5.5, colour, 0.9);
  wifi(p, end - bw - 6 - sw - 6 - 7, mid + 5.5, colour, 0.9);
}

/** A foldable's crease: a soft shade either side of a hairline down (or across) the middle of the display. */
function fold(p: Painter, width: number, height: number, along: "vertical" | "horizontal"): void {
  const { s } = p;
  const band = 24;
  const shade = [0x00000000, 0x00000014, 0x00000000] as const;
  if (along === "vertical") {
    const x = s.x + width / 2;
    rect(p, "fold", {
      x: x - band / 2,
      y: s.y,
      w: band,
      h: height,
      radius: 0,
      gradient: { kind: "linear", colors: shade, from: [x - band / 2, s.y], to: [x + band / 2, s.y], radius: 0 },
    });
    rect(p, "fold-line", { x: x - 0.5, y: s.y, w: 1, h: height, radius: 0, fill: 0x0000001f });
    return;
  }
  const y = s.y + height / 2;
  rect(p, "fold", {
    x: s.x,
    y: y - band / 2,
    w: width,
    h: band,
    radius: 0,
    gradient: { kind: "linear", colors: shade, from: [s.x, y - band / 2], to: [s.x, y + band / 2], radius: 0 },
  });
  rect(p, "fold-line", { x: s.x, y: y - 0.5, w: width, h: 1, radius: 0, fill: 0x0000001f });
}

/** The home indicator, the gesture handle or the three buttons, centred in the bottom inset. */
function indicator(p: Painter, width: number, height: number, colour: number): void {
  const { chrome, s } = p;
  const bottom = s.y + height;
  if (chrome.indicator === "home" || chrome.indicator === "gesture") {
    const ios = chrome.indicator === "home";
    const w = ios ? (chrome.landscape ? 200 : 134) : 108;
    const h = ios ? 5 : 4;
    const y = ios ? bottom - 8 - h : bottom - chrome.bottom / 2 - h / 2;
    rect(p, "indicator", { x: s.x + (width - w) / 2, y, w, h, radius: h / 2, fill: colour });
  } else if (chrome.indicator === "buttons") {
    const mid = bottom - chrome.bottom / 2;
    const step = width / 4;
    // Back (a wedge pointing to the start), home (a ring) and recents (a rounded square).
    const r = 14;
    rect(p, "nav-back", {
      x: s.x + step - 7 - r,
      y: mid - r,
      w: 2 * r,
      h: 2 * r,
      radius: 0,
      oval: true,
      fill: colour,
      arc: { start: 330, sweep: 60 },
    });
    rect(p, "nav-home", {
      x: s.x + 2 * step - 8,
      y: mid - 8,
      w: 16,
      h: 16,
      radius: 0,
      oval: true,
      fill: colour,
      arc: { start: 0, sweep: 360, width: 2 },
    });
    rect(p, "nav-recents", {
      x: s.x + 3 * step - 7,
      y: mid - 7,
      w: 14,
      h: 14,
      radius: 2,
      stroke: { color: colour, width: 2 },
    });
  }
}

/** Mirrors `items` left to right across a screen from `x`, `width` wide; a wedge's or arc's angles mirror too. */
function mirror(items: ItemSpec[], x: number, width: number): void {
  items.forEach((item, i) => {
    if (item.kind !== "rect" && item.kind !== "text") {
      return;
    }
    const at = 2 * x + width - item.x - item.w;
    if (item.kind === "rect" && item.arc !== undefined) {
      const start = (540 - item.arc.start - item.arc.sweep) % 360;
      items[i] = { ...item, x: at, arc: { ...item.arc, start } };
    } else {
      items[i] = { ...item, x: at };
    }
  });
}

/**
 * The chrome items of one laid-out screen of `width` by `height`. `boxes` are the page's laid-out boxes, so the icons
 * take the colour that reads over what the page paints under them; `background` is the screen's own surface.
 */
export function chromeItems(
  s: ScreenDocument,
  chrome: DeviceChromeSpec,
  frame: string,
  size: { readonly width: number; readonly height: number },
  boxes: readonly Placed[],
  background: number,
  families: readonly string[],
  rtl = false,
): ItemSpec[] {
  if (!chrome.visible) {
    return [];
  }
  const p: Painter = { s, chrome, frame, families, items: [] };
  const { width, height } = size;
  const statusShown = chrome.top > 0 && !(chrome.platform === "cupertino" && chrome.landscape);
  if (statusShown) {
    const colour = glyphColour(chrome, backdropAt(boxes, width / 2, chrome.top / 2, background));
    if (chrome.platform === "cupertino") {
      cupertinoStatus(p, width, colour);
    } else {
      materialStatus(p, width, colour);
    }
    if (rtl) {
      // A right-to-left system mirrors its status bar: the time at the end, the battery at the start.
      mirror(p.items, s.x, width);
    }
  }
  if (chrome.fold !== undefined) {
    fold(p, width, height, chrome.fold);
  }
  if (chrome.cutout !== "none") {
    cutout(p, width, height);
  }
  if (chrome.indicator !== "none" && chrome.bottom > 0) {
    const colour = glyphColour(chrome, backdropAt(boxes, width / 2, height - chrome.bottom / 2, background));
    indicator(p, width, height, colour);
  }
  return p.items;
}
