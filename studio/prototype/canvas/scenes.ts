// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Scene A (STU-045, STU-120) and scene B (STU-121) of the Plux Canvas
// prototype. The code runs unchanged in a worker and on the main thread; it
// never touches `window` or `document`.

import type { Canvas, CanvasKit, FontMgr, Paint, Paragraph, Path, Surface } from "canvaskit-wasm";

export interface PageNode {
  type?: string;
  testId?: string;
  props?: Record<string, unknown>;
  slots?: Record<string, PageNode>;
  children?: PageNode[];
}

export interface PageDoc {
  root: PageNode;
}

export interface Env {
  ck: CanvasKit;
  font: ArrayBuffer;
  pages: PageDoc[];
  width: number;
  height: number;
  raf(cb: (t: number) => void): void;
  now(): number;
}

export interface Summary {
  n: number;
  mean: number;
  p50: number;
  p95: number;
  p99: number;
  max: number;
}

export interface PassResult {
  name: string;
  gpuSync: boolean;
  frameMs: Summary;
  rafIntervalMs: Summary;
  fpsFromRaf: number;
  drawCallsPerFrame: number;
}

export interface SceneResult {
  scene: string;
  details: Record<string, number | string>;
  passes: PassResult[];
}

const FRAMES = 300;
const WARMUP = 30;

function must<T>(value: T | undefined): T {
  if (value === undefined) {
    throw new Error("missing value");
  }
  return value;
}

export function summarize(values: number[]): Summary {
  const sorted = [...values].sort((a, b) => a - b);
  const n = sorted.length;
  const rank = (p: number): number => must(sorted[Math.max(0, Math.ceil(p * n) - 1)]);
  const sum = sorted.reduce((a, b) => a + b, 0);
  return { n, mean: sum / n, p50: rank(0.5), p95: rank(0.95), p99: rank(0.99), max: must(sorted[n - 1]) };
}

/** Counts the draw calls a frame issues; clip and transform calls are not draws. */
class Counted {
  calls = 0;
  constructor(readonly canvas: Canvas) {}

  rrect(rect: Float32Array, paint: Paint): void {
    this.calls++;
    this.canvas.drawRRect(rect, paint);
  }

  path(path: Path, paint: Paint): void {
    this.calls++;
    this.canvas.drawPath(path, paint);
  }

  paragraph(paragraph: Paragraph, x: number, y: number): void {
    this.calls++;
    this.canvas.drawParagraph(paragraph, x, y);
  }
}

interface Camera {
  tx: number;
  ty: number;
  zoom: number;
}

async function runFrames(
  env: Env,
  surface: Surface,
  name: string,
  gpuSync: boolean,
  camera: (u: number) => Camera,
  draw: (d: Counted, cam: Camera) => void,
): Promise<PassResult> {
  const canvas = surface.getCanvas();
  const d = new Counted(canvas);
  const frameMs: number[] = [];
  const intervals: number[] = [];
  const calls: number[] = [];
  const info = {
    width: 1,
    height: 1,
    colorType: env.ck.ColorType.RGBA_8888,
    alphaType: env.ck.AlphaType.Unpremul,
    colorSpace: env.ck.ColorSpace.SRGB,
  };
  await new Promise<void>((resolve) => {
    let frame = 0;
    let last = -1;
    const step = (timestamp: number): void => {
      const measured = frame >= WARMUP;
      if (measured && last >= 0) {
        intervals.push(timestamp - last);
      }
      last = timestamp;
      const t0 = env.now();
      d.calls = 0;
      draw(d, camera(Math.max(0, frame - WARMUP) / FRAMES));
      surface.flush();
      if (gpuSync) {
        canvas.readPixels(0, 0, info);
      }
      if (measured) {
        frameMs.push(env.now() - t0);
        calls.push(d.calls);
      }
      frame++;
      if (frame < FRAMES + WARMUP) {
        env.raf(step);
      } else {
        resolve();
      }
    };
    env.raf(step);
  });
  const rafIntervalMs = summarize(intervals);
  return {
    name,
    gpuSync,
    frameMs: summarize(frameMs),
    rafIntervalMs,
    fpsFromRaf: 1000 / rafIntervalMs.mean,
    drawCallsPerFrame: calls.reduce((a, b) => a + b, 0) / calls.length,
  };
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

function makeParagraph(
  env: Env,
  fonts: FontMgr,
  text: string,
  size: number,
  width: number,
): Paragraph {
  const { ck } = env;
  const style = new ck.ParagraphStyle({
    textStyle: { color: ck.Color(30, 30, 40), fontSize: size, fontFamilies: ["Roboto"] },
    textAlign: ck.TextAlign.Left,
  });
  const builder = ck.ParagraphBuilder.Make(style, fonts);
  builder.addText(text);
  const paragraph = builder.build();
  paragraph.layout(width);
  builder.delete();
  return paragraph;
}

function strokePaint(ck: CanvasKit, r: number, g: number, b: number, width: number): Paint {
  const paint = new ck.Paint();
  paint.setStyle(ck.PaintStyle.Stroke);
  paint.setStrokeWidth(width);
  paint.setAntiAlias(true);
  paint.setColor(ck.Color(r, g, b));
  return paint;
}

function fillPaint(ck: CanvasKit, r: number, g: number, b: number): Paint {
  const paint = new ck.Paint();
  paint.setStyle(ck.PaintStyle.Fill);
  paint.setAntiAlias(true);
  paint.setColor(ck.Color(r, g, b));
  return paint;
}

/** Scene A: 200 titled boxes and 1,000 cubic Bezier links, panned and zoomed. */
export async function sceneA(env: Env, surface: Surface): Promise<SceneResult> {
  const { ck } = env;
  const t0 = env.now();
  const fonts = must(ck.FontMgr.FromData(env.font));
  const random = prng(1);
  const cols = 20;
  const rows = 10;
  const boxW = 180;
  const boxH = 80;
  const stepX = 260;
  const stepY = 180;
  const boxes: { x: number; y: number; rect: Float32Array; title: Paragraph }[] = [];
  for (let i = 0; i < cols * rows; i++) {
    const x = (i % cols) * stepX;
    const y = Math.floor(i / cols) * stepY;
    const rect = ck.RRectXY(ck.LTRBRect(x, y, x + boxW, y + boxH), 10, 10);
    boxes.push({ x, y, rect, title: makeParagraph(env, fonts, `Node ${i} - Action step`, 15, boxW - 20) });
  }
  const links: Path[] = [];
  for (let i = 0; i < 1000; i++) {
    const a = must(boxes[Math.floor(random() * boxes.length)]);
    const col = Math.max(0, Math.min(cols - 1, Math.round(a.x / stepX) + Math.floor(random() * 7) - 3));
    const row = Math.max(0, Math.min(rows - 1, Math.round(a.y / stepY) + Math.floor(random() * 7) - 3));
    const b = must(boxes[row * cols + col]);
    const sx = a.x + boxW;
    const sy = a.y + boxH / 2;
    const ex = b.x;
    const ey = b.y + boxH / 2;
    const dx = 80 + random() * 60;
    // SVG path syntax is the one construction API shared by the npm and the Flutter CanvasKit builds.
    const path = ck.Path.MakeFromSVGString(
      `M${sx} ${sy}C${sx + dx} ${sy} ${ex - dx} ${ey} ${ex} ${ey}`,
    );
    links.push(must(path ?? undefined));
  }
  const fill = fillPaint(ck, 245, 247, 252);
  const edge = strokePaint(ck, 60, 90, 160, 2);
  const link = strokePaint(ck, 120, 130, 150, 1.5);
  const white = ck.Color(255, 255, 255);
  const buildMs = env.now() - t0;
  const camera = (u: number): Camera => {
    const zoom = 0.45 + 0.4 * (0.5 + 0.5 * Math.sin(u * Math.PI * 4));
    return { zoom, tx: -u * 3500 * zoom, ty: -u * 700 * zoom };
  };
  const draw = (d: Counted, cam: Camera): void => {
    const canvas = d.canvas;
    canvas.clear(white);
    canvas.save();
    canvas.translate(cam.tx, cam.ty);
    canvas.scale(cam.zoom, cam.zoom);
    for (const path of links) {
      d.path(path, link);
    }
    for (const box of boxes) {
      d.rrect(box.rect, fill);
      d.rrect(box.rect, edge);
      d.paragraph(box.title, box.x + 10, box.y + 10);
    }
    canvas.restore();
  };
  const passes: PassResult[] = [];
  for (const gpuSync of [false, true]) {
    passes.push(await runFrames(env, surface, "pan-zoom, no culling", gpuSync, camera, draw));
  }
  return { scene: "A", details: { boxes: boxes.length, links: links.length, buildMs }, passes };
}

interface Row {
  x: number;
  y: number;
  w: number;
  h: number;
  rect: Float32Array;
  paragraph: Paragraph;
}

function describe(node: PageNode): string {
  const props = Object.entries(node.props ?? {})
    .map(([key, value]) => {
      const text = typeof value === "object" ? JSON.stringify(value) : String(value);
      return `${key}=${text.slice(0, 24)}`;
    })
    .join(", ");
  return `${node.type ?? "node"}${node.testId ? ` #${node.testId}` : ""}\n${props.slice(0, 70)}`;
}

function flatten(node: PageNode, depth: number, out: { depth: number; label: string }[]): void {
  out.push({ depth, label: describe(node) });
  for (const key of Object.keys(node.slots ?? {}).sort()) {
    flatten(must(node.slots?.[key]), depth + 1, out);
  }
  for (const child of node.children ?? []) {
    flatten(child, depth + 1, out);
  }
}

/** Scene B: 100 phone-sized screens built from the starter pages. */
export async function sceneB(env: Env, surface: Surface): Promise<SceneResult> {
  const { ck } = env;
  const screenW = 390;
  const screenH = 844;
  const gridCols = 10;
  const spaceX = 430;
  const spaceY = 884;
  const fonts = must(ck.FontMgr.FromData(env.font));
  const flat = env.pages.map((page) => {
    const out: { depth: number; label: string }[] = [];
    flatten(page.root, 0, out);
    return out;
  });
  const tBuild = env.now();
  let layoutMs = 0;
  let nodeCount = 0;
  const screens: { x: number; y: number; rows: Row[] }[] = [];
  for (let i = 0; i < 100; i++) {
    const rows: Row[] = [];
    let y = 8;
    for (const item of must(flat[i % flat.length])) {
      const x = 8 + item.depth * 8;
      const w = screenW - x - 8;
      const style = new ck.ParagraphStyle({
        textStyle: { color: ck.Color(30, 30, 40), fontSize: 11, fontFamilies: ["Roboto"] },
        textAlign: ck.TextAlign.Left,
      });
      const builder = ck.ParagraphBuilder.Make(style, fonts);
      builder.addText(item.label);
      const paragraph = builder.build();
      const t = env.now();
      paragraph.layout(w - 8);
      layoutMs += env.now() - t;
      builder.delete();
      const h = paragraph.getHeight() + 8;
      rows.push({ x, y, w, h, rect: ck.RRectXY(ck.LTRBRect(x, y, x + w, y + h), 4, 4), paragraph });
      y += h + 4;
      nodeCount++;
    }
    screens.push({ x: (i % gridCols) * spaceX, y: Math.floor(i / gridCols) * spaceY, rows });
  }
  const buildMs = env.now() - tBuild;
  const fill = fillPaint(ck, 240, 244, 250);
  const edge = strokePaint(ck, 80, 110, 170, 1);
  const frame = strokePaint(ck, 30, 30, 40, 2);
  const page = fillPaint(ck, 255, 255, 255);
  const backdrop = ck.Color(225, 228, 235);
  const screenRect = ck.RRectXY(ck.LTRBRect(0, 0, screenW, screenH), 24, 24);
  const view = (cull: boolean) => (d: Counted, cam: Camera) => {
    const canvas = d.canvas;
    canvas.clear(backdrop);
    canvas.save();
    canvas.translate(cam.tx, cam.ty);
    canvas.scale(cam.zoom, cam.zoom);
    const left = -cam.tx / cam.zoom;
    const top = -cam.ty / cam.zoom;
    const right = left + env.width / cam.zoom;
    const bottom = top + env.height / cam.zoom;
    for (const screen of screens) {
      if (cull && (screen.x > right || screen.x + screenW < left || screen.y > bottom || screen.y + screenH < top)) {
        continue;
      }
      canvas.save();
      canvas.translate(screen.x, screen.y);
      d.rrect(screenRect, page);
      d.rrect(screenRect, frame);
      canvas.clipRRect(screenRect, ck.ClipOp.Intersect, true);
      for (const row of screen.rows) {
        d.rrect(row.rect, fill);
        d.rrect(row.rect, edge);
        d.paragraph(row.paragraph, row.x + 4, row.y + 4);
      }
      canvas.restore();
    }
    canvas.restore();
  };
  const pan = (zoom: number) => (u: number): Camera => ({ zoom, tx: -u * 1500 * zoom, ty: -u * 5000 * zoom });
  const tFirst = env.now();
  const first = surface.getCanvas();
  view(false)(new Counted(first), pan(0.3)(0));
  surface.flush();
  first.readPixels(0, 0, {
    width: 1,
    height: 1,
    colorType: ck.ColorType.RGBA_8888,
    alphaType: ck.AlphaType.Unpremul,
    colorSpace: ck.ColorSpace.SRGB,
  });
  const firstFrameMs = env.now() - tFirst;
  const passes: PassResult[] = [];
  const variants: [string, boolean, number][] = [
    ["pan at 0.3x, all 100 screens drawn", false, 0.3],
    ["pan at 0.3x, visible screens only", true, 0.3],
    ["pan at 1.0x, visible screens only", true, 1],
  ];
  for (const gpuSync of [false, true]) {
    for (const [name, cull, zoom] of variants) {
      passes.push(await runFrames(env, surface, name, gpuSync, pan(zoom), view(cull)));
    }
  }
  return {
    scene: "B",
    details: {
      screens: screens.length,
      nodes: nodeCount,
      buildMs,
      layoutMs,
      firstFrameMs,
      firstDrawMs: buildMs + firstFrameMs,
    },
    passes,
  };
}
