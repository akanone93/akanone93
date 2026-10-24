// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// No-GPU techniques (decision M-12): batching, a tile cache, level of detail and
// reduced resolution while interacting, measured on scene A and scene B on
// either a WebGL or a CanvasKit CPU raster surface. Prototype code, not product.

import type { Canvas, CanvasKit, FontMgr, Image, Paint, Paragraph, Path, Surface } from "canvaskit-wasm";
import { flatten, type PageDoc, type Summary, summarize } from "./scenes";

const FRAMES = 300;
const WARMUP = 30;
const TILE = 512;

interface Rect {
  l: number;
  t: number;
  r: number;
  b: number;
}

export interface DrawOpts {
  batch: boolean;
  straight: boolean;
  text: boolean;
  cull: boolean;
}

interface Counter {
  n: number;
}

export interface Scenario {
  name: string;
  zoom: number;
  dx: number;
  dy: number;
  drag: boolean;
}

export interface Content {
  scene: string;
  scenarios: Scenario[];
  setDragged(index: number): void;
  drawStatic(c: Canvas, v: Rect, zoom: number, o: DrawOpts, cnt: Counter): void;
  drawDynamic(c: Canvas, o: DrawOpts, u: number, cnt: Counter): void;
}

export interface Options {
  name: string;
  renderer: "gl" | "sw";
  batch: boolean;
  tile: boolean;
  lod: boolean;
}

export interface VariantResult {
  scene: string;
  variant: string;
  scenario: string;
  frameMs: Summary;
  drawCallsPerFrame: number;
  rebuildMs?: number;
}

interface Env {
  ck: CanvasKit;
  width: number;
  height: number;
  now(): number;
}

function must<T>(value: T | undefined | null): T {
  if (value === undefined || value === null) {
    throw new Error("missing value");
  }
  return value;
}

function prng(seed: number): () => number {
  let a = seed;
  return () => {
    a = (a + 0x6d2b79f5) | 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

function paragraph(ck: CanvasKit, fonts: FontMgr, text: string, size: number, width: number): Paragraph {
  const style = new ck.ParagraphStyle({
    textStyle: { color: ck.Color(30, 30, 40), fontSize: size, fontFamilies: ["Roboto"] },
    textAlign: ck.TextAlign.Left,
  });
  const builder = ck.ParagraphBuilder.Make(style, fonts);
  builder.addText(text);
  const p = builder.build();
  p.layout(width);
  builder.delete();
  return p;
}

function paint(ck: CanvasKit, stroke: boolean, r: number, g: number, b: number, width = 1): Paint {
  const p = new ck.Paint();
  p.setStyle(stroke ? ck.PaintStyle.Stroke : ck.PaintStyle.Fill);
  p.setStrokeWidth(width);
  p.setAntiAlias(true);
  p.setColor(ck.Color(r, g, b));
  return p;
}

const BOX_W = 180;
const BOX_H = 80;
const STEP_X = 260;
const STEP_Y = 180;
const COLS = 20;

interface Link {
  a: number;
  b: number;
  dx: number;
}

/** Scene A: 200 boxes and 1,000 cubic links, as in scenes.ts, with a draggable box. */
export function makeSceneA(ck: CanvasKit, font: ArrayBuffer): Content {
  const fonts = must(ck.FontMgr.FromData(font));
  const random = prng(1);
  const boxes = Array.from({ length: 200 }, (_, i) => ({
    x: (i % COLS) * STEP_X,
    y: Math.floor(i / COLS) * STEP_Y,
    title: paragraph(ck, fonts, `Node ${i} - Action step`, 15, BOX_W - 20),
  }));
  const links: Link[] = [];
  for (let i = 0; i < 1000; i++) {
    const a = Math.floor(random() * boxes.length);
    const ba = must(boxes[a]);
    const col = Math.max(0, Math.min(COLS - 1, Math.round(ba.x / STEP_X) + Math.floor(random() * 7) - 3));
    const row = Math.max(0, Math.min(9, Math.round(ba.y / STEP_Y) + Math.floor(random() * 7) - 3));
    links.push({ a, b: row * COLS + col, dx: 80 + random() * 60 });
  }
  const fill = paint(ck, false, 245, 247, 252);
  const edge = paint(ck, true, 60, 90, 160, 2);
  const linkPaint = paint(ck, true, 120, 130, 150, 1.5);
  const endpoints = (l: Link, moved: number, ox: number, oy: number): number[] => {
    const a = must(boxes[l.a]);
    const b = must(boxes[l.b]);
    return [
      a.x + BOX_W + (l.a === moved ? ox : 0),
      a.y + BOX_H / 2 + (l.a === moved ? oy : 0),
      b.x + (l.b === moved ? ox : 0),
      b.y + BOX_H / 2 + (l.b === moved ? oy : 0),
    ];
  };
  // The Flutter CanvasKit build has no Path.moveTo; SVG path syntax is shared by all builds.
  const seg = (l: Link, straight: boolean, moved: number, ox: number, oy: number): string => {
    const [sx = 0, sy = 0, ex = 0, ey = 0] = endpoints(l, moved, ox, oy);
    return straight
      ? `M${sx} ${sy}L${ex} ${ey}`
      : `M${sx} ${sy}C${sx + l.dx} ${sy} ${ex - l.dx} ${ey} ${ex} ${ey}`;
  };
  const build = (d: string): Path => must(ck.Path.MakeFromSVGString(d || "M0 0"));
  const curvePaths = links.map((l) => build(seg(l, false, -1, 0, 0)));
  const straightPaths = links.map((l) => build(seg(l, true, -1, 0, 0)));
  const boxRect = (i: number, ox = 0, oy = 0): Float32Array => {
    const b = must(boxes[i]);
    return ck.RRectXY(ck.LTRBRect(b.x + ox, b.y + oy, b.x + ox + BOX_W, b.y + oy + BOX_H), 10, 10);
  };
  const rects = boxes.map((_, i) => boxRect(i));
  const rrectSvg = (x: number, y: number): string =>
    `M${x + 10} ${y}h${BOX_W - 20}a10 10 0 0 1 10 10v${BOX_H - 20}a10 10 0 0 1 -10 10h${-(BOX_W - 20)}a10 10 0 0 1 -10 -10v${-(BOX_H - 20)}a10 10 0 0 1 10 -10z`;
  let dragged = -1;
  let staticIdx: number[] = [];
  let dynIdx: number[] = [];
  let staticCurve: Path | null = null;
  let staticStraight: Path | null = null;
  let boxFill: Path | null = null;
  const setDragged = (index: number): void => {
    dragged = index;
    staticIdx = [];
    dynIdx = [];
    links.forEach((l, i) => {
      (l.a === index || l.b === index ? dynIdx : staticIdx).push(i);
    });
    staticCurve?.delete();
    staticStraight?.delete();
    boxFill?.delete();
    staticCurve = build(staticIdx.map((i) => seg(must(links[i]), false, -1, 0, 0)).join(""));
    staticStraight = build(staticIdx.map((i) => seg(must(links[i]), true, -1, 0, 0)).join(""));
    boxFill = build(
      boxes
        .map((b, i) => (i === index ? "" : rrectSvg(b.x, b.y)))
        .join(""),
    );
  };
  setDragged(-1);
  const bounds = links.map((l) => {
    const [sx = 0, sy = 0, ex = 0, ey = 0] = endpoints(l, -1, 0, 0);
    return { l: Math.min(sx, ex) - 150, t: Math.min(sy, ey) - 10, r: Math.max(sx, ex) + 150, b: Math.max(sy, ey) + 10 };
  });
  const hits = (a: Rect, v: Rect): boolean => a.l < v.r && a.r > v.l && a.t < v.b && a.b > v.t;
  return {
    scene: "A (200 boxes, 1,000 links)",
    scenarios: [
      { name: "pan 0.6x", zoom: 0.6, dx: 3500, dy: 700, drag: false },
      { name: "pan 0.4x", zoom: 0.4, dx: 3500, dy: 700, drag: false },
      { name: "drag one box 0.6x", zoom: 0.6, dx: 0, dy: 0, drag: true },
    ],
    setDragged,
    drawStatic(c, v, _zoom, o, cnt) {
      if (o.batch) {
        cnt.n++;
        c.drawPath(must(o.straight ? staticStraight : staticCurve), linkPaint);
        cnt.n += 2;
        c.drawPath(must(boxFill), fill);
        c.drawPath(must(boxFill), edge);
      } else {
        for (const i of staticIdx) {
          if (!o.cull || hits(must(bounds[i]), v)) {
            cnt.n++;
            c.drawPath(must((o.straight ? straightPaths : curvePaths)[i]), linkPaint);
          }
        }
      }
      boxes.forEach((b, i) => {
        if (i === dragged || (o.cull && !hits({ l: b.x, t: b.y, r: b.x + BOX_W, b: b.y + BOX_H }, v))) {
          return;
        }
        if (!o.batch) {
          cnt.n += 2;
          c.drawRRect(must(rects[i]), fill);
          c.drawRRect(must(rects[i]), edge);
        }
        if (o.text) {
          cnt.n++;
          c.drawParagraph(b.title, b.x + 10, b.y + 10);
        }
      });
    },
    drawDynamic(c, o, u, cnt) {
      if (dragged < 0) {
        return;
      }
      const ox = 200 * Math.sin(u * Math.PI * 4);
      const oy = 100 * Math.cos(u * Math.PI * 4);
      if (o.batch) {
        const p = build(dynIdx.map((i) => seg(must(links[i]), o.straight, dragged, ox, oy)).join(""));
        cnt.n++;
        c.drawPath(p, linkPaint);
        p.delete();
      } else {
        for (const i of dynIdx) {
          const l = must(links[i]);
          const p = build(seg(l, o.straight, dragged, ox, oy));
          cnt.n++;
          c.drawPath(p, linkPaint);
          p.delete();
        }
      }
      const r = boxRect(dragged, ox, oy);
      cnt.n += 2;
      c.drawRRect(r, fill);
      c.drawRRect(r, edge);
      if (o.text) {
        cnt.n++;
        const b = must(boxes[dragged]);
        c.drawParagraph(b.title, b.x + ox + 10, b.y + oy + 10);
      }
    },
  };
}

/** Scene B: 100 phone-sized screens from the starter pages (no batching). */
export function makeSceneB(ck: CanvasKit, font: ArrayBuffer, pages: PageDoc[]): Content {
  const fonts = must(ck.FontMgr.FromData(font));
  const W = 390;
  const H = 844;
  const flat = pages.map((page) => {
    const out: { depth: number; label: string }[] = [];
    flatten(page.root, 0, out);
    return out;
  });
  const screens = Array.from({ length: 100 }, (_, i) => {
    const rows: { x: number; y: number; rect: Float32Array; p: Paragraph }[] = [];
    let y = 8;
    for (const item of must(flat[i % flat.length])) {
      const x = 8 + item.depth * 8;
      const w = W - x - 8;
      const p = paragraph(ck, fonts, item.label, 11, w - 8);
      const h = p.getHeight() + 8;
      rows.push({ x, y, rect: ck.RRectXY(ck.LTRBRect(x, y, x + w, y + h), 4, 4), p });
      y += h + 4;
    }
    return { x: (i % 10) * 430, y: Math.floor(i / 10) * 884, rows };
  });
  const fill = paint(ck, false, 240, 244, 250);
  const edge = paint(ck, true, 80, 110, 170);
  const frame = paint(ck, true, 30, 30, 40, 2);
  const page = paint(ck, false, 255, 255, 255);
  const screenRect = ck.RRectXY(ck.LTRBRect(0, 0, W, H), 24, 24);
  return {
    scene: "B (100 screens)",
    scenarios: [{ name: "pan 0.3x", zoom: 0.3, dx: 1500, dy: 5000, drag: false }],
    setDragged() {},
    drawStatic(c, v, _zoom, o, cnt) {
      for (const s of screens) {
        if (s.x > v.r || s.x + W < v.l || s.y > v.b || s.y + H < v.t) {
          continue;
        }
        c.save();
        c.translate(s.x, s.y);
        cnt.n += 2;
        c.drawRRect(screenRect, page);
        c.drawRRect(screenRect, frame);
        c.clipRRect(screenRect, ck.ClipOp.Intersect, true);
        for (const row of s.rows) {
          cnt.n += 2;
          c.drawRRect(row.rect, fill);
          c.drawRRect(row.rect, edge);
          if (o.text) {
            cnt.n++;
            c.drawParagraph(row.p, row.x + 4, row.y + 4);
          }
        }
        c.restore();
      }
    },
    drawDynamic() {},
  };
}

/** Runs every scenario of one scene for one technique combination. */
export async function runVariant(
  env: Env,
  surface: Surface,
  raf: (cb: (t: number) => void) => void,
  content: Content,
  o: Options,
): Promise<VariantResult[]> {
  const { ck } = env;
  const main = surface.getCanvas();
  const res = o.lod ? 0.5 : 1;
  const lowSurface = o.lod && !o.tile ? must(ck.MakeSurface(env.width * res, env.height * res)) : null;
  const tileSurface = o.tile ? must(ck.MakeSurface(TILE * res, TILE * res)) : null;
  const white = ck.Color(255, 255, 255);
  const syncInfo = {
    width: 1,
    height: 1,
    colorType: ck.ColorType.RGBA_8888,
    alphaType: ck.AlphaType.Unpremul,
    colorSpace: ck.ColorSpace.SRGB,
  };
  const out: VariantResult[] = [];
  const scenarios: (Scenario & { idle?: boolean })[] = [...content.scenarios];
  if (o.lod || o.tile) {
    scenarios.push({ ...must(scenarios[0]), name: "idle full-res redraw", dx: 0, dy: 0, idle: true });
  }
  for (const sc of scenarios) {
    const dragIndex = sc.drag ? 105 : -1;
    content.setDragged(dragIndex);
    const cache = new Map<string, Image>();
    let tileZoom = 0;
    const cnt: Counter = { n: 0 };
    const interacting = !sc.idle;
    const optsFor = (zoom: number): DrawOpts => ({
      batch: o.batch,
      straight: interacting && o.lod && zoom < 0.5,
      text: !(interacting && o.lod && zoom < 0.5),
      cull: o.tile || content.scene.startsWith("B"),
    });
    const frame = (u: number): void => {
      const tx = -u * sc.dx * sc.zoom;
      const ty = -u * sc.dy * sc.zoom;
      const view: Rect = { l: -tx / sc.zoom, t: -ty / sc.zoom, r: -tx / sc.zoom + env.width / sc.zoom, b: -ty / sc.zoom + env.height / sc.zoom };
      cnt.n = 0;
      main.clear(white);
      if (o.tile && interacting) {
        if (tileZoom === 0 || Math.abs(sc.zoom / tileZoom - 1) > 0.25) {
          for (const img of cache.values()) {
            img.delete();
          }
          cache.clear();
          tileZoom = sc.zoom;
        }
        const s = TILE / tileZoom;
        const opts = optsFor(tileZoom);
        for (let ix = Math.floor(view.l / s); ix <= Math.floor(view.r / s); ix++) {
          for (let iy = Math.floor(view.t / s); iy <= Math.floor(view.b / s); iy++) {
            const key = `${ix},${iy}`;
            let img = cache.get(key);
            if (!img) {
              const tc = must(tileSurface).getCanvas();
              tc.clear(ck.Color4f(1, 1, 1, 1));
              tc.save();
              tc.scale((TILE * res) / s, (TILE * res) / s);
              tc.translate(-ix * s, -iy * s);
              content.drawStatic(tc, { l: ix * s, t: iy * s, r: (ix + 1) * s, b: (iy + 1) * s }, tileZoom, opts, cnt);
              tc.restore();
              img = must(must(tileSurface).makeImageSnapshot());
              cache.set(key, img);
            }
            const dx = tx + ix * s * sc.zoom;
            const dy = ty + iy * s * sc.zoom;
            cnt.n++;
            const rx = Math.round(dx);
            const ry = Math.round(dy);
            const side = Math.round(TILE * (sc.zoom / tileZoom));
            main.drawImageRectOptions(img, ck.LTRBRect(0, 0, TILE * res, TILE * res), ck.LTRBRect(rx, ry, rx + side, ry + side), ck.FilterMode.Nearest, ck.MipmapMode.None, null);
          }
        }
        main.save();
        main.translate(tx, ty);
        main.scale(sc.zoom, sc.zoom);
        content.drawDynamic(main, opts, u, cnt);
        main.restore();
      } else {
        const opts = optsFor(sc.zoom);
        const target = interacting && lowSurface ? lowSurface.getCanvas() : main;
        const k = target === main ? 1 : res;
        target.clear(white);
        target.save();
        target.scale(k, k);
        target.translate(tx, ty);
        target.scale(sc.zoom, sc.zoom);
        content.drawStatic(target, view, sc.zoom, opts, cnt);
        content.drawDynamic(target, opts, u, cnt);
        target.restore();
        if (target !== main) {
          const img = must(must(lowSurface).makeImageSnapshot());
          cnt.n++;
          main.drawImageRectOptions(img, ck.LTRBRect(0, 0, env.width * res, env.height * res), ck.LTRBRect(0, 0, env.width, env.height), ck.FilterMode.Nearest, ck.MipmapMode.None, null);
          img.delete();
        }
      }
      surface.flush();
      if (o.renderer === "gl") {
        main.readPixels(0, 0, syncInfo);
      }
    };
    let rebuildMs: number | undefined;
    if (sc.drag && o.tile) {
      const t = env.now();
      frame(0);
      rebuildMs = env.now() - t;
    }
    const times: number[] = [];
    const calls: number[] = [];
    await new Promise<void>((resolve) => {
      let n = 0;
      const step = (): void => {
        const t0 = env.now();
        frame(Math.max(0, n - WARMUP) / FRAMES);
        const dt = env.now() - t0;
        if (n >= WARMUP) {
          times.push(dt);
          calls.push(cnt.n);
        }
        n++;
        if (n < FRAMES + WARMUP) {
          raf(step);
        } else {
          resolve();
        }
      };
      raf(step);
    });
    for (const img of cache.values()) {
      img.delete();
    }
    out.push({
      scene: content.scene,
      variant: o.name,
      scenario: sc.name,
      frameMs: summarize(times),
      drawCallsPerFrame: calls.reduce((a, b) => a + b, 0) / calls.length,
      ...(rebuildMs === undefined ? {} : { rebuildMs }),
    });
  }
  return out;
}
