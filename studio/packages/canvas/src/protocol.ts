// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The messages between the main thread and the canvas worker (STU-003,
 * studio-development §2.5, §9.1). Every message carries the protocol version
 * and is structured-clone data only: no functions, no class instances, colours
 * as `0xRRGGBBAA` numbers. The only transferables are the OffscreenCanvas and
 * the font buffers in `init`.
 */

/** The protocol version; a message with another version is rejected. */
export const PROTOCOL_VERSION = 1;

/** A colour as `0xRRGGBBAA`. */
export type ColorRgba = number;

/** A point in world units. */
export interface Point {
  readonly x: number;
  readonly y: number;
}

/** A stroke: colour and width in world units. */
export interface StrokeStyle {
  readonly color: ColorRgba;
  readonly width: number;
}

/** A device frame or screen: a container with an optional pre-rendered shadow. */
export interface FrameSpec {
  readonly kind: "frame";
  readonly id: string;
  readonly name: string;
  readonly x: number;
  readonly y: number;
  readonly w: number;
  readonly h: number;
  readonly radius: number;
  readonly fill: ColorRgba;
  readonly stroke?: StrokeStyle;
  readonly shadow: boolean;
  readonly z: number;
}

/** A box: a rectangle with corner radius, fill and stroke. */
export interface RectSpec {
  readonly kind: "rect";
  readonly id: string;
  readonly frame?: string;
  readonly label?: string;
  readonly x: number;
  readonly y: number;
  readonly w: number;
  readonly h: number;
  readonly radius: number;
  readonly fill?: ColorRgba;
  readonly stroke?: StrokeStyle;
  readonly z: number;
}

/** A text run laid out as one paragraph of width `w`. */
export interface TextSpec {
  readonly kind: "text";
  readonly id: string;
  readonly frame?: string;
  readonly x: number;
  readonly y: number;
  readonly w: number;
  readonly text: string;
  readonly size: number;
  readonly color: ColorRgba;
  readonly z: number;
}

/** A link: a segment, or a cubic Bézier when both control points are given. */
export interface LinkSpec {
  readonly kind: "link";
  readonly id: string;
  readonly from: Point;
  readonly to: Point;
  readonly c1?: Point;
  readonly c2?: Point;
  readonly stroke: StrokeStyle;
  readonly z: number;
}

/** Any scene item. */
export type ItemSpec = FrameSpec | RectSpec | TextSpec | LinkSpec;

/** One change to the retained scene. */
export type SceneOp =
  | { readonly op: "add"; readonly item: ItemSpec }
  | { readonly op: "replace"; readonly item: ItemSpec }
  | { readonly op: "move"; readonly id: string; readonly dx: number; readonly dy: number }
  | { readonly op: "remove"; readonly id: string }
  | { readonly op: "clear" };

/** A change of the view: pan by CSS pixels, zoom about a CSS point, or set. */
export type ViewportChange =
  | { readonly op: "pan"; readonly dx: number; readonly dy: number }
  | { readonly op: "zoom"; readonly factor: number; readonly x: number; readonly y: number }
  | { readonly op: "set"; readonly x: number; readonly y: number; readonly zoom: number };

/** What the user is doing; used for reduced resolution and live layers. */
export type InteractionKind = "pan" | "zoom" | "drag";

/** Which renderer to use: chosen by measurement, or forced. */
export type RendererPreference = "auto" | "webgl" | "raster";

/** The canvas the worker draws into; an OffscreenCanvas in the browser. */
export interface OffscreenCanvasLike {
  width: number;
  height: number;
}

/** Start-up options of the canvas. */
export interface CanvasOptions {
  readonly renderer: RendererPreference;
  /** Tile cache on or off; default on (M-12). */
  readonly tiles?: boolean;
  readonly tileSize: number;
  readonly background: ColorRgba;
  readonly fonts: readonly ArrayBuffer[];
  readonly fontFamily: string;
  /** Wait for the GPU at the end of each frame so frame times include it (benchmarks only). */
  readonly measureFrames: boolean;
}

/** How the renderer was chosen, reported once at start-up. */
export interface RendererInfo {
  readonly kind: "webgl" | "raster";
  readonly glRenderer: string;
  readonly software: boolean;
  readonly majorPerformanceCaveat: boolean;
  readonly tiles: boolean;
  readonly measuredMs: { readonly webgl: number; readonly raster: number } | null;
}

/** Per-frame statistics sent back to the main thread. */
export interface FrameStats {
  readonly frame: number;
  readonly ms: number;
  readonly drawCalls: number;
  readonly tilesDrawn: number;
  readonly staleTilesDrawn: number;
  readonly regions: number;
  readonly liveItems: number;
  readonly idleQueue: number;
  readonly resolution: number;
  readonly tileMs: number;
  readonly regionMs: number;
  readonly liveMs: number;
  readonly flushMs: number;
  readonly tileRenders: number;
  readonly tileRenderMs: number;
}

/** A node of the read-only scene model (studio-development §9.2). */
export interface SceneModelNode {
  readonly id: string;
  readonly kind: ItemSpec["kind"];
  readonly frame: string | null;
  readonly label: string;
  readonly bounds: { readonly x: number; readonly y: number; readonly w: number; readonly h: number };
  readonly z: number;
}

/** The read-only scene model: nodes in z-order, frames, and the view. */
export interface SceneModel {
  readonly version: number;
  readonly viewport: {
    readonly x: number;
    readonly y: number;
    readonly zoom: number;
    readonly width: number;
    readonly height: number;
  };
  readonly frames: readonly string[];
  readonly nodes: readonly SceneModelNode[];
}

/** Main thread → worker. */
export type HostMessage =
  | {
      readonly v: number;
      readonly type: "init";
      readonly canvas: OffscreenCanvasLike;
      readonly width: number;
      readonly height: number;
      readonly dpr: number;
      readonly options: CanvasOptions;
    }
  | { readonly v: number; readonly type: "patch"; readonly ops: readonly SceneOp[] }
  | { readonly v: number; readonly type: "viewport"; readonly change: ViewportChange }
  | {
      readonly v: number;
      readonly type: "interaction";
      readonly phase: "start" | "end";
      readonly kind: InteractionKind;
      readonly ids: readonly string[];
    }
  | {
      readonly v: number;
      readonly type: "resize";
      readonly width: number;
      readonly height: number;
      readonly dpr: number;
    }
  | { readonly v: number; readonly type: "snapshot"; readonly requestId: number }
  | { readonly v: number; readonly type: "dispose" };

/** Worker → main thread. */
export type WorkerMessage =
  | { readonly v: number; readonly type: "ready"; readonly renderer: RendererInfo }
  | { readonly v: number; readonly type: "frame"; readonly stats: FrameStats }
  | { readonly v: number; readonly type: "snapshot"; readonly requestId: number; readonly model: SceneModel }
  | { readonly v: number; readonly type: "error"; readonly message: string };

const hostTypes: ReadonlySet<string> = new Set([
  "init",
  "patch",
  "viewport",
  "interaction",
  "resize",
  "snapshot",
  "dispose",
]);
const workerTypes: ReadonlySet<string> = new Set(["ready", "frame", "snapshot", "error"]);

function versioned(data: unknown, types: ReadonlySet<string>): boolean {
  if (typeof data !== "object" || data === null) {
    return false;
  }
  const { v, type } = data as { v?: unknown; type?: unknown };
  return v === PROTOCOL_VERSION && typeof type === "string" && types.has(type);
}

/** `data` as a host message, or undefined when it is not one of this version. */
export function readHostMessage(data: unknown): HostMessage | undefined {
  return versioned(data, hostTypes) ? (data as HostMessage) : undefined;
}

/** `data` as a worker message, or undefined when it is not one of this version. */
export function readWorkerMessage(data: unknown): WorkerMessage | undefined {
  return versioned(data, workerTypes) ? (data as WorkerMessage) : undefined;
}
