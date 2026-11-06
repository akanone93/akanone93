// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The messages between the main thread and the canvas worker (STU-003,
 * studio-development §2.5, §9.1). Every message carries the protocol version
 * and is structured-clone data only: no functions, no class instances, colours
 * as `0xRRGGBBAA` numbers. The only transferables are the OffscreenCanvases
 * (base and interaction layer) and the font buffers in `init`.
 */

import type { PaintTheme, ScreenTheme, TextDirection, WidgetNode } from "@plux/layout";
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

/** A stroke: colour and width in world units; `dash` draws it dashed, dashes of that length in world units. */
export interface StrokeStyle {
  readonly color: ColorRgba;
  readonly width: number;
  readonly dash?: number;
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
  /** A soft, low elevation shadow under the box (a card resting on the canvas). */
  readonly shadow?: boolean;
  /**
   * What a screen's widget paints (STU-005), drawn one box at a time rather than batched: per-corner radii (top left,
   * top right, bottom right, bottom left) in place of `radius`, an ellipse, a gradient fill, box shadows, a border side
   * per edge in place of `stroke`, the opacity and the clips of the widget's ancestors.
   */
  readonly radii?: readonly [number, number, number, number];
  readonly oval?: boolean;
  readonly gradient?: GradientSpec;
  readonly shadows?: readonly ShadowSpec[];
  readonly sides?: SidesSpec;
  readonly opacity?: number;
  /**
   * How the image of `image` sits in a painted box (STU-005, AST-002): fitted inside the box's shape over its fill,
   * with a placeholder colour drawn while the image is missing.
   */
  /**
   * Only an arc of the box's oval is drawn: from `start` degrees (clockwise from 3 o'clock) through `sweep` degrees,
   * a wedge filled with `fill`, or with `width` a stroke in the fill's colour (STU-005: pie charts, progress).
   */
  readonly arc?: { readonly start: number; readonly sweep: number; readonly width?: number };
  readonly imageFit?: ImageFitSpec;
  readonly clips?: readonly ClipSpec[];
  readonly z: number;
}

/** A gradient in world units: along `from` → `to` (linear) or round `from` with `radius` (radial, sweep). */
export interface GradientSpec {
  readonly kind: "linear" | "radial" | "sweep";
  readonly colors: readonly ColorRgba[];
  readonly stops?: readonly number[];
  readonly from: readonly [number, number];
  readonly to: readonly [number, number];
  readonly radius: number;
}

/** A box shadow in world units: offset, blur radius and spread, as Flutter's `BoxShadow`. */
export interface ShadowSpec {
  readonly color: ColorRgba;
  readonly dx: number;
  readonly dy: number;
  readonly blur: number;
  readonly spread: number;
}

/** A border side per edge. */
export interface SidesSpec {
  readonly top?: { readonly color: ColorRgba; readonly width: number };
  readonly right?: { readonly color: ColorRgba; readonly width: number };
  readonly bottom?: { readonly color: ColorRgba; readonly width: number };
  readonly left?: { readonly color: ColorRgba; readonly width: number };
}

/** A shape an item is clipped to, in world units. */
export interface ClipSpec {
  readonly x: number;
  readonly y: number;
  readonly w: number;
  readonly h: number;
  readonly radii?: readonly [number, number, number, number];
  readonly oval?: boolean;
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
  /** Bold text, or a weight from 100 to 900; regular when absent. */
  readonly weight?: "bold" | number;
  /** The rest of a screen text's style, as it was measured (STU-006): slant, tracking, line height (a multiple of the
   * size, with the leading spread evenly as Material 3 does), a decoration line. */
  readonly italic?: boolean;
  readonly letterSpacing?: number;
  readonly height?: number;
  readonly decoration?: "underline" | "lineThrough" | "overline";
  /** How a text longer than `maxLines` ends: an ellipsis (the default) or cut off. */
  readonly overflow?: "clip" | "ellipsis" | "fade" | "visible";
  /** Centred in `w`, against its end, or justified; start-aligned when absent. */
  readonly align?: "center" | "end" | "justify";
  /** At most this many lines, the last ending in an ellipsis; unlimited when absent. */
  readonly maxLines?: number;
  /**
   * The font families, in fallback order, the text was measured with (a screen's theme); the canvas's own when absent.
   * Drawing with the measuring fonts keeps a line the layout fitted on one line from wrapping onto the next.
   */
  readonly families?: readonly string[];
  /** Drawn mirrored left to right about its centre: an icon right-to-left text mirrors. */
  readonly mirror?: boolean;
  /** A rich text's runs, each drawn in its own style over the text's; `text` is then their characters together. */
  readonly spans?: readonly TextRunSpec[];
  /** The opacity and the clips of the widget's ancestors on a screen (STU-005). */
  readonly opacity?: number;
  readonly clips?: readonly ClipSpec[];
  readonly z: number;
}

/** An image's `BoxFit` and alignment (`x` and `y` from -1 to 1) in its box, and its placeholder's colour. */
export interface ImageFitSpec {
  readonly fit: "fill" | "contain" | "cover" | "fitWidth" | "fitHeight" | "none" | "scaleDown";
  readonly alignX: number;
  readonly alignY: number;
  readonly placeholder?: ColorRgba;
}

/** One run of a rich text: its characters and what differs from the text's own style. */
export interface TextRunSpec {
  readonly text: string;
  readonly size?: number;
  readonly color?: ColorRgba;
  readonly weight?: "bold" | number;
  readonly italic?: boolean;
  readonly letterSpacing?: number;
  readonly height?: number;
  readonly decoration?: "underline" | "lineThrough" | "overline";
  /** The run's own family, before the text's families. */
  readonly family?: string;
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

/**
 * The dot grid on the background of a canvas: one dot of `radius` CSS pixels every `spacing` world units. When the
 * dots would come closer than `minGap` CSS pixels, the spacing doubles until they do not, so the grid stays calm at
 * every zoom.
 */
export interface GridStyle {
  readonly spacing: number;
  readonly radius: number;
  readonly minGap: number;
  readonly color: ColorRgba;
}

/** Start-up options of the canvas. */
export interface CanvasOptions {
  readonly renderer: RendererPreference;
  /** Tile cache on or off; default on (M-12). */
  readonly tiles?: boolean;
  readonly tileSize: number;
  readonly background: ColorRgba;
  /** The dot grid drawn on the background; none when absent. */
  readonly grid?: GridStyle;
  /**
   * How the view reaches the screen. `compositor` (the default, M-12): the worker renders an overscanned base, in
   * tiles when idle, and the host moves and scales it with a CSS transform between renders. `direct`: the worker
   * redraws the whole view, without tiles, on every animation frame in which it changed, and the element is never
   * transformed. The pixels then have one source, so nothing jumps when a render lands and nothing shows blank while
   * one is pending; each changed frame costs a full draw, so it suits scenes that draw within a frame (the app canvas).
   */
  readonly present?: "compositor" | "direct";
  /**
   * `adaptive` (the default, M-12): below the level-of-detail zooms text is drawn as bars, links straight and frame
   * shadows not at all. `full`: everything is drawn in full at every zoom, as design tools do.
   */
  readonly detail?: "adaptive" | "full";
  readonly fonts: readonly ArrayBuffer[];
  /** The family of each buffer in `fonts`, in order; text layout registers them under these names. */
  readonly fontFamilies?: readonly string[];
  readonly fontFamily: string;
  /** Wait for the GPU at the end of each frame so frame times include it (benchmarks only). */
  readonly measureFrames: boolean;
  /** The icon sets screens draw their `Icon` widgets with (THM-005); icons are placeholders without them. */
  readonly icons?: IconNames;
}

/**
 * The icon sets of the canvas (THM-005): each icon's code point by name, the font families that hold the set's glyphs
 * (one family per part of a split font, in fallback order), and the icons right-to-left text mirrors, as
 * `<set> <name>`.
 */
export interface IconNames {
  readonly material: Readonly<Record<string, number>>;
  readonly cupertino: Readonly<Record<string, number>>;
  readonly families: { readonly material: readonly string[]; readonly cupertino: readonly string[] };
  readonly mirrored: readonly string[];
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
  /** What the header shows after the name (PC-11): the route, the page kind, the entry mark; none when absent. */
  readonly meta?: string;
  readonly x: number;
  readonly y: number;
  readonly root: WidgetNode | null;
}

/**
 * A device's system areas, drawn over every screen as the device draws them (PC-11, STU-057): the status bar with its
 * time and icons, a camera cutout, and the home indicator or navigation buttons. The layout pads the page's app bar and
 * `SafeArea` by the same insets. Insets are logical pixels.
 */
export interface DeviceChromeSpec {
  readonly top: number;
  readonly right: number;
  readonly bottom: number;
  readonly left: number;
  readonly cutout: "none" | "punch" | "notch" | "island";
  readonly indicator: "none" | "gesture" | "home" | "buttons";
  readonly platform: "material" | "cupertino";
  /** The display's corner radius. */
  readonly radius: number;
  /** Landscape: the cutout sits on the left edge and the status bar is hidden on iOS. */
  readonly landscape: boolean;
  /** The time the status bar shows. */
  readonly time: string;
  /** Whether the chrome is drawn; the insets apply either way, as they do on the device. */
  readonly visible: boolean;
  /** A foldable's fold, drawn as a crease: down the middle (`vertical`) or across it (`horizontal`) (PC-33). */
  readonly fold?: "vertical" | "horizontal";
  /** Status bar and indicator colours over a light and over a dark backdrop; the cutout's colour. */
  readonly onLight: ColorRgba;
  readonly onDark: ColorRgba;
  readonly cutoutColor: ColorRgba;
}

/** How laid-out screens are drawn: colours, the device height and the layout theme. */
export interface ScreenStyle {
  readonly deviceHeight: number;
  readonly theme: ScreenTheme;
  /** The ambient text scale of the preview (A11Y-005); default 1. */
  readonly textScale?: number;
  /** The ambient text direction of the preview (I18N-010); default left to right. */
  readonly textDirection?: TextDirection;
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
  /** The colours to paint widgets with; without it, every widget is drawn as a neutral box (STU-005). */
  readonly paint?: PaintTheme;
  /** The colour of each screen's name, drawn above its device frame as a design tool names its frames; none when absent. */
  readonly title?: ColorRgba;
  /** The colour of the header's route and kind after the name; the name's when absent. */
  readonly meta?: ColorRgba;
  /** The device's system areas; a plain frame with the default corners when absent. */
  readonly chrome?: DeviceChromeSpec;
  /** Live screens (PC-15, Interact): scroll views scroll inside the device, by their offsets, with no overflow region. */
  readonly live?: boolean;
}

/** The laid-out height of one screen's content (STU-056). */
export interface ScreenHeight {
  readonly id: string;
  readonly contentHeight: number;
  /** On a live screen, each scroll view and how far it scrolls (PC-15). */
  readonly scrolls?: readonly { readonly node: string; readonly max: number }[];
  /** The widgets whose text does not fit its box (PC-32). */
  readonly overflowing?: readonly string[];
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
      /** Screens that only moved: their laid-out items are translated, not laid out again. */
      readonly moved?: readonly ScreenMove[];
      readonly style: ScreenStyle;
    }
  /** Pauses (`true`) or resumes (`false`) all drawing; the scene and view keep their state (STU-063). */
  | { readonly v: number; readonly type: "pause"; readonly paused: boolean }
  | { readonly v: number; readonly type: "dispose" };

/** A screen that moved by `dx`, `dy` and changed in nothing else. */
export interface ScreenMove {
  readonly id: string;
  readonly dx: number;
  readonly dy: number;
}

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
