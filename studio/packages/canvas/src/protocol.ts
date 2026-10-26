// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The messages between the main thread and the canvas worker (STU-003,
 * studio-development §2.5, §9.1). Every message carries the protocol version
 * and is structured-clone data only: no functions, no class instances, colours
 * as `0xRRGGBBAA` numbers. The only transferables are the OffscreenCanvases
 * (base and interaction layer) and the font buffers in `init`.
 */

import type { ScreenTheme, WidgetNode } from "@plux/layout";
import type { BaseOrigin, LayerBox } from "./compositor.ts";

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
  /** Key of an image set with `images`: drawn over the box, clipped to its corner radius (STU-052). */
  readonly image?: string;
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
  /** The family of each buffer in `fonts`, in order; text layout registers them under these names. */
  readonly fontFamilies?: readonly string[];
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
  /** Whether this frame re-rendered the base surface. */
  readonly base: boolean;
  /** Time spent drawing the interaction layer, in ms. */
  readonly layerMs: number;
  /** Device pixels of the interaction layer redrawn and presented this frame; 0 without it. */
  readonly layerPixels: number;
  /** The duration of the progressive base job this message reports, in ms; 0 for a frame (M-12). */
  readonly jobMs: number;
  /** Progressive base jobs still queued (M-12). */
  readonly baseJobs: number;
  /** Of `baseJobs`, those before every tile the view shows is current (M-12). */
  readonly visibleJobs: number;
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

/** A screen document to lay out in the worker (ADR-0013): its widget tree and where it stands. */
export interface ScreenDocument {
  readonly id: string;
  readonly title: string;
  readonly x: number;
  readonly y: number;
  readonly root: WidgetNode | null;
}

/** How laid-out screens are drawn: colours, the device height and the layout theme. */
export interface ScreenStyle {
  readonly deviceHeight: number;
  readonly theme: ScreenTheme;
  readonly frame: ColorRgba;
  readonly frameStroke: ColorRgba;
  readonly box: ColorRgba;
  readonly boxStroke: ColorRgba;
  readonly placeholder: ColorRgba;
  readonly text: ColorRgba;
  readonly overflowFill: ColorRgba;
  readonly overflowBox: ColorRgba;
  readonly overflowText: ColorRgba;
  readonly overflowStroke: ColorRgba;
}

/** The laid-out height of one screen's content (STU-056). */
export interface ScreenHeight {
  readonly id: string;
  readonly contentHeight: number;
}

/** An encoded image and the key scene items refer to it by. */
export interface ImageData {
  readonly key: string;
  readonly bytes: Uint8Array;
}

/** Main thread → worker. */
export type HostMessage =
  | {
      readonly v: number;
      readonly type: "init";
      readonly canvas: OffscreenCanvasLike;
      /** The interaction layer above the base, for dragged items (M-12). */
      readonly layer?: OffscreenCanvasLike;
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
  /** Decodes encoded images (PNG) under keys that `RectSpec.image` names; a key is set once. */
  | { readonly v: number; readonly type: "images"; readonly images: readonly ImageData[] }
  /** Renders the world rectangle of item `id` into an encoded PNG `width` pixels wide, answered by `thumbnail`. */
  | {
      readonly v: number;
      readonly type: "thumbnail";
      readonly requestId: number;
      readonly id: string;
      readonly width: number;
    }
  /** Replaces the scene with `screens`, laid out in the worker with SkParagraph. */
  | {
      readonly v: number;
      readonly type: "screens";
      readonly requestId: number;
      readonly screens: readonly ScreenDocument[];
      readonly style: ScreenStyle;
    }
  /**
   * Lays out again only the screens that changed and drops `removed`, against
   * the screens of the last `screens` request (STU-121); answered by a
   * `screens` message whose ops change only the items that differ and whose
   * heights cover only `screens`. Without an earlier `screens` request in the
   * same style, the host sends `screens` instead.
   */
  | {
      readonly v: number;
      readonly type: "screensUpdate";
      readonly requestId: number;
      readonly screens: readonly ScreenDocument[];
      readonly removed: readonly string[];
      readonly style: ScreenStyle;
    }
  /** Pauses (`true`) or resumes (`false`) all drawing; the scene and view keep their state (STU-063). */
  | { readonly v: number; readonly type: "pause"; readonly paused: boolean }
  | { readonly v: number; readonly type: "dispose" };

/** Worker → main thread. */
export type WorkerMessage =
  | { readonly v: number; readonly type: "ready"; readonly renderer: RendererInfo }
  | {
      readonly v: number;
      readonly type: "frame";
      readonly stats: FrameStats;
      /** Where the base surface shown from this frame on was rendered. */
      readonly origin?: BaseOrigin;
      /** The interaction layer's box on the base, or null when it is hidden. */
      readonly layer?: LayerBox | null;
    }
  | { readonly v: number; readonly type: "snapshot"; readonly requestId: number; readonly model: SceneModel }
  /** The PNG of a `thumbnail` request, or null when the item is unknown or the surface cannot render it. */
  | { readonly v: number; readonly type: "thumbnail"; readonly requestId: number; readonly png: Uint8Array | null }
  /** The screens of a `screens` request, laid out: the ops applied to the scene and each content height. */
  | {
      readonly v: number;
      readonly type: "screens";
      readonly requestId: number;
      readonly ops: readonly SceneOp[];
      readonly heights: readonly ScreenHeight[];
    }
  /** The surface was lost, or rebuilt from the scene after a loss. */
  | { readonly v: number; readonly type: "surface"; readonly state: "lost" | "restored" }
  | { readonly v: number; readonly type: "error"; readonly message: string };

const hostTypes: ReadonlySet<string> = new Set([
  "init",
  "patch",
  "viewport",
  "interaction",
  "resize",
  "snapshot",
  "images",
  "thumbnail",
  "screens",
  "screensUpdate",
  "pause",
  "dispose",
]);
const workerTypes: ReadonlySet<string> = new Set([
  "ready",
  "frame",
  "snapshot",
  "thumbnail",
  "screens",
  "surface",
  "error",
]);

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
