// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import {
  CanvasHost,
  type FontRequest,
  type FontSource,
  type Guides,
  type SceneOp,
  type ScreenDocument,
  type ScreenHeight,
  type ScreenStyle,
  type WorkerLike,
} from "@plux/canvas";
import { canvasWorkerScriptURL } from "../../lib/trusted-worker.ts";
import canvasWorkerURL from "./canvas-worker.ts?worker&url";

/**
 * The canvas-local CSS point of the page point (`clientX`, `clientY`), for a
 * canvas whose bounding rectangle is `rect`: `zoomAbout` and hit-testing take
 * canvas-local coordinates, pointer events give page ones.
 */
export function canvasPoint(
  rect: { readonly left: number; readonly top: number },
  clientX: number,
  clientY: number,
): { x: number; y: number } {
  return { x: clientX - rect.left, y: clientY - rect.top };
}

/** The part of the canvas host the plugin canvas uses; a stub in tests. */
export interface CanvasSurface {
  /** Replaces the scene with `screens`, laid out in the canvas worker; resolves with their content heights. */
  layoutScreens(screens: readonly ScreenDocument[], style: ScreenStyle): Promise<readonly ScreenHeight[]>;
  setViewport(x: number, y: number, zoom: number): void;
  pan(dx: number, dy: number): void;
  zoomAbout(factor: number, x: number, y: number): void;
  /** Item IDs under a CSS point / meeting a CSS rectangle, topmost first (main-thread hit-testing). */
  hitTest?(x: number, y: number): string[];
  hitTestRect?(rect: { l: number; t: number; r: number; b: number }, mode: "intersect" | "contain"): string[];
  setSelection?(ids: readonly string[]): void;
  /** World rectangle of an item, and the overlay feedback of a drag (guides, distance labels, insertion indicator). */
  boundsOf?(id: string): { l: number; t: number; r: number; b: number } | undefined;
  setGuides?(guides: Guides | null): void;
  /** Applies scene patches (ghosts, arrows, the pulse); they stay until the next first layout replaces the scene. */
  patch?(ops: readonly SceneOp[]): void;
  /** The PNG of scene item `id` rendered in the worker, `width` pixels wide, or null (STU-052). */
  thumbnail?(id: string, width: number): Promise<Uint8Array | null>;
  /** Makes encoded images available to scene items under their keys. */
  setImages?(images: readonly { key: string; bytes: Uint8Array }[]): void;
  /** Pauses (`true`) or resumes the worker's drawing; the scene and view are kept (STU-063). */
  setPaused?(paused: boolean): void;
  readonly viewport: { readonly x: number; readonly y: number; readonly zoom: number };
  dispose(): void;
}

export interface SurfaceSize {
  readonly width: number;
  readonly height: number;
  readonly dpr: number;
  /** The host app's font files (STU-006), read before the worker starts; absent when the page has none. */
  readonly hostFonts?: () => Promise<readonly FontSource[]>;
  /** The app's own font assets by their declared family (ADR-0032), read before the worker starts; absent when the page has none. */
  readonly deviceFonts?: () => Promise<readonly FontSource[]>;
}

/** The fonts the canvas asks for: the app's own assets rank first, then the host's, then Roboto and Noto. */
export function fontRequest(device: readonly FontSource[], host: readonly FontSource[]): FontRequest {
  return {
    device,
    host,
    roboto: toSource(FONT_FILES[0] as readonly [string, string]),
    noto: [...FONT_FILES.slice(1), ...NOTO_FILES].map(toSource),
    sfPro: false,
  };
}

/** Starts a surface on `canvas`. */
export type SurfaceFactory = (canvas: HTMLCanvasElement, size: SurfaceSize) => CanvasSurface;

/** Roboto and Noto Sans, regular and bold; Vite emits them as hashed assets. */
const FONT_FILES: readonly (readonly [string, string])[] = [
  ["Roboto", new URL("./fonts/Roboto_400Regular.ttf", import.meta.url).href],
  ["Roboto", new URL("./fonts/Roboto_700Bold.ttf", import.meta.url).href],
];
const NOTO_FILES: readonly (readonly [string, string])[] = [
  ["Noto Sans", new URL("./fonts/NotoSans_400Regular.ttf", import.meta.url).href],
  ["Noto Sans", new URL("./fonts/NotoSans_700Bold.ttf", import.meta.url).href],
];

const toSource = ([family, url]: readonly [string, string]): FontSource => ({ family, url });

/** The browser surface: the canvas host on a bundled worker, started once its fonts are loaded. */
export const browserSurface: SurfaceFactory = (canvas, size) => {
  let host: CanvasHost | null = null;
  let disposed = false;
  let view = { x: 0, y: 0, zoom: 1 };
  const queued: ((h: CanvasHost) => void)[] = [];
  const run = (action: (h: CanvasHost) => void): void => {
    if (host) {
      action(host);
    } else if (!disposed) {
      queued.push(action);
    }
  };
  // A canvas hands its control to a worker once. React's development double mount disposes the first
  // surface in the same task it was made, so the transfer waits for a microtask and skips a disposed surface.
  const start = async (): Promise<void> => {
    if (disposed) {
      return;
    }
    const none = (): readonly FontSource[] => [];
    const [deviceFonts, hostFonts] = await Promise.all([
      (size.deviceFonts?.() ?? Promise.resolve(none())).catch(none),
      (size.hostFonts?.() ?? Promise.resolve(none())).catch(none),
    ]);
    if (disposed) {
      return;
    }
    CanvasHost.create(
      canvas,
      // Only the dimensions go to the worker: the font loaders are functions, which a message cannot carry.
      { width: size.width, height: size.height, dpr: size.dpr },
      {
        renderer: "auto",
        tileSize: 512,
        background: 0xf2f4f9ff,
        fonts: [],
        fontFamily: "Noto Sans",
        measureFrames: false,
      },
      {
        createWorker: () =>
          new Worker(canvasWorkerScriptURL(canvasWorkerURL, window, import.meta.env.DEV), {
            type: "module",
          }) as unknown as WorkerLike,
        mainThread: () => {
          throw new Error("the plugin canvas needs a worker");
        },
      },
      {
        request: fontRequest(deviceFonts, hostFonts),
        deps: { fetchFont: (url) => fetch(url).then((r) => r.arrayBuffer()) },
      },
    )
      .then((created) => {
        if (disposed) {
          created.dispose();
          return;
        }
        host = created;
        for (const action of queued.splice(0)) {
          action(created);
        }
      })
      .catch(() => undefined);
  };
  queueMicrotask(() => void start());
  return {
    layoutScreens: (screens, style) =>
      new Promise((resolve) => run((h) => void h.layoutScreens(screens, style).then(resolve))),
    setViewport: (x, y, zoom) => {
      view = { x, y, zoom };
      run((h) => h.setViewport(x, y, zoom));
    },
    patch: (ops) => run((h) => h.patch(ops)),
    thumbnail: (id, width) => new Promise((resolve) => run((h) => void h.thumbnail(id, width).then(resolve))),
    setImages: (images) => run((h) => h.setImages(images)),
    setPaused: (paused) => run((h) => h.setPaused(paused)),
    pan: (dx, dy) => run((h) => h.pan(dx, dy)),
    zoomAbout: (factor, x, y) => run((h) => h.zoomAbout(factor, x, y)),
    hitTest: (x, y) => host?.hitTest(x, y) ?? [],
    hitTestRect: (rect, mode) => host?.hitTestRect(rect, mode) ?? [],
    boundsOf: (id) => host?.boundsOf(id),
    setGuides: (g) => run((h) => h.setGuides(g)),
    setSelection: (ids) => run((h) => h.setSelection(ids)),
    get viewport() {
      return host ? host.viewport : view;
    },
    dispose: () => {
      disposed = true;
      host?.dispose();
    },
  };
};
