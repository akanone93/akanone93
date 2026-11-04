// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The painters of the widgets the canvas paints (STU-005): what each draws, from its props and the theme, as Flutter
 * draws it — `Container` and `AnimatedContainer` (colour or decoration inside the margin, foreground decoration, clip),
 * `DecoratedBox`, `ColoredBox`, `Card` (Material 3: surface container low, 12-pixel corners, elevation 1, 4-pixel margin),
 * `Scaffold` (its background), `Divider` and `VerticalDivider`; and what clips or fades its children (`ClipRRect`,
 * `ClipOval`, `Opacity`, `AnimatedOpacity`, `FadeTransition`). Layout widgets draw nothing.
 */

import type { ColorRole } from "../material-color/scheme.ts";
import { checkboxMark, controlForeground, controlPainters } from "./controls.ts";
import {
  argbToRgba,
  type BoxFitName,
  type BoxPaint,
  borderRadius,
  decorationPaint,
  deflate,
  edgeInsets,
  elevationShadows,
  type IconPaint,
  type ImagePaint,
  type NodePaint,
  type PaintClip,
  type PaintTheme,
  type Radii,
  type Rect,
  type Rgba,
  resolveColor,
  roleColor,
  shapePaint,
} from "./paint.ts";

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);
const num = (v: unknown): number | undefined => (typeof v === "number" && Number.isFinite(v) ? v : undefined);

/** A widget node as the painters read it. */
export interface PaintNode {
  readonly id: string;
  readonly type?: string;
  readonly props?: Obj;
  readonly children?: readonly PaintNode[];
  readonly slots?: Readonly<Record<string, PaintNode | readonly PaintNode[]>>;
  /** The actions the widget handles; a control with none is disabled. */
  readonly events?: unknown;
}

/** What a painter returns: its boxes, and the clip and opacity it puts on its children. */
interface Painted {
  readonly boxes?: readonly BoxPaint[];
  readonly clip?: PaintClip;
  readonly opacity?: number;
}

type Painter = (props: Obj, r: Rect, theme: PaintTheme) => Painted;

const CARD_RADII: Radii = [12, 12, 12, 12];

function container(props: Obj, r: Rect, theme: PaintTheme): Painted {
  const inner = deflate(r, edgeInsets(props["margin"], theme.rtl === true));
  const color = resolveColor(props["color"], theme);
  const background =
    decorationPaint(props["decoration"], inner, theme) ?? (color === undefined ? undefined : { ...inner, fill: color });
  const front = decorationPaint(props["foregroundDecoration"], inner, theme);
  const clips = props["clipBehavior"] !== undefined && props["clipBehavior"] !== "none" && background !== undefined;
  return {
    boxes: [
      ...(background === undefined ? [] : [background]),
      ...(front === undefined ? [] : [{ ...front, foreground: true }]),
    ],
    ...(clips
      ? {
          clip: {
            ...inner,
            ...(background.oval === true
              ? { oval: true }
              : background.radii === undefined
                ? {}
                : { radii: background.radii }),
          },
        }
      : {}),
  };
}

function decoratedBox(props: Obj, r: Rect, theme: PaintTheme): Painted {
  const paint = decorationPaint(props["decoration"], r, theme);
  return {
    boxes:
      paint === undefined ? [] : [{ ...paint, ...(props["position"] === "foreground" ? { foreground: true } : {}) }],
  };
}

function coloredBox(props: Obj, r: Rect, theme: PaintTheme): Painted {
  const fill = resolveColor(props["color"], theme);
  return { boxes: fill === undefined ? [] : [{ ...r, fill }] };
}

/** Material 3's card: `surfaceContainerLow`, 12-pixel corners, elevation 1 in the scheme's shadow, a 4-pixel margin. */
function card(props: Obj, r: Rect, theme: PaintTheme): Painted {
  const inner = deflate(
    r,
    props["margin"] === undefined ? [4, 4, 4, 4] : edgeInsets(props["margin"], theme.rtl === true),
  );
  const fill = resolveColor(props["color"], theme) ?? roleColor(theme, "surfaceContainerLow");
  const shadow = resolveColor(props["shadowColor"], theme) ?? roleColor(theme, "shadow");
  const elevation = num(props["elevation"]) ?? 1.0;
  const shape = shapePaint(props["shape"], inner, theme, CARD_RADII);
  const clipped = props["clipBehavior"] !== undefined && props["clipBehavior"] !== "none";
  const box: BoxPaint = {
    ...inner,
    fill,
    ...shape,
    ...(elevation > 0 ? { shadows: elevationShadows(elevation, shadow) } : {}),
  };
  return {
    boxes: [box],
    ...(clipped
      ? {
          clip: {
            ...inner,
            ...(shape.oval === true ? { oval: true } : shape.radii === undefined ? {} : { radii: shape.radii }),
          },
        }
      : {}),
  };
}

function scaffold(props: Obj, r: Rect, theme: PaintTheme): Painted {
  return { boxes: [{ ...r, fill: resolveColor(props["backgroundColor"], theme) ?? roleColor(theme, "surface") }] };
}

/** Material 3's divider: a line of `thickness` (1) in `outlineVariant`, centred in its height, with its indents. */
function divider(vertical: boolean): Painter {
  return (props, r, theme) => {
    const thickness = num(props["thickness"]) ?? 1.0;
    const color = resolveColor(props["color"], theme) ?? roleColor(theme, "outlineVariant");
    const indent = num(props["indent"]) ?? 0;
    const endIndent = num(props["endIndent"]) ?? 0;
    const radii = borderRadius(props["radius"]);
    const line: Rect = vertical
      ? { x: r.x + (r.w - thickness) / 2, y: r.y + indent, w: thickness, h: Math.max(0, r.h - indent - endIndent) }
      : { x: r.x + indent, y: r.y + (r.h - thickness) / 2, w: Math.max(0, r.w - indent - endIndent), h: thickness };
    return { boxes: [{ ...line, fill: color, ...(radii === undefined ? {} : { radii }) }] };
  };
}

function clipRRect(props: Obj, r: Rect): Painted {
  const radii = borderRadius(props["borderRadius"]);
  return props["clipBehavior"] === "none" ? {} : { clip: { ...r, ...(radii === undefined ? {} : { radii }) } };
}

function opacity(props: Obj): Painted {
  const o = num(props["opacity"]);
  return o === undefined ? {} : { opacity: Math.min(1, Math.max(0, o)) };
}

const FITS: ReadonlySet<string> = new Set(["fill", "contain", "cover", "fitWidth", "fitHeight", "none", "scaleDown"]);
const ALIGNMENTS: Readonly<Record<string, readonly [number, number]>> = {
  topLeft: [-1, -1],
  topStart: [-1, -1],
  topCenter: [0, -1],
  topRight: [1, -1],
  topEnd: [1, -1],
  centerLeft: [-1, 0],
  centerStart: [-1, 0],
  center: [0, 0],
  centerRight: [1, 0],
  centerEnd: [1, 0],
  bottomLeft: [-1, 1],
  bottomStart: [-1, 1],
  bottomCenter: [0, 1],
  bottomRight: [1, 1],
  bottomEnd: [1, 1],
};

/** An `Alignment` by name or as `{x, y}`, centred when it is neither. */
function alignmentOf(v: unknown): readonly [number, number] {
  if (typeof v === "string") {
    return ALIGNMENTS[v] ?? [0, 0];
  }
  return isObj(v) ? [num(v["x"]) ?? 0, num(v["y"]) ?? 0] : [0, 0];
}

/** An `ImageSource` as the canvas paints it: an asset by id, or a network URL; nothing for an unbound source. */
export function imageOf(
  source: unknown,
  fit: unknown,
  alignment: unknown,
  fallback: BoxFitName,
): ImagePaint | undefined {
  if (!isObj(source)) {
    return undefined;
  }
  const asset = isObj(source["asset"]) ? source["asset"]["$asset"] : source["asset"];
  const url = typeof source["url"] === "string" ? source["url"] : undefined;
  if (typeof asset !== "string" && url === undefined) {
    return undefined;
  }
  const [alignX, alignY] = alignmentOf(alignment);
  return {
    ...(typeof asset === "string" ? { asset } : { url: url as string }),
    fit: typeof fit === "string" && FITS.has(fit) ? (fit as BoxFitName) : fallback,
    alignX,
    alignY,
  };
}

/** `Image`: its source fitted to its box; Flutter fits an image with no fit by scaling it down. */
function image(props: Obj, r: Rect): Painted {
  const paint = imageOf(props["source"], props["fit"], props["alignment"], "scaleDown");
  return paint === undefined ? {} : { boxes: [{ ...r, image: paint }] };
}

/**
 * `CircleAvatar` in Material 3: a circle in `primaryContainer` (its text in `onPrimaryContainer`), its background
 * image and then its foreground image covering it.
 */
function circleAvatar(props: Obj, r: Rect, theme: PaintTheme): Painted {
  const fill = resolveColor(props["backgroundColor"], theme) ?? roleColor(theme, "primaryContainer");
  const size = Math.min(r.w, r.h);
  const circle = { x: r.x + (r.w - size) / 2, y: r.y + (r.h - size) / 2, w: size, h: size, oval: true };
  const back = imageOf(props["backgroundImage"], "cover", "center", "cover");
  const front = imageOf(props["foregroundImage"], "cover", "center", "cover");
  return {
    boxes: [
      { ...circle, fill, ...(back === undefined ? {} : { image: back }) },
      ...(front === undefined ? [] : [{ ...circle, image: front }]),
    ],
  };
}

const painters: ReadonlyMap<string, Painter> = new Map<string, Painter>([
  ["Image", image],
  ["CircleAvatar", circleAvatar],
  ["Container", container],
  ["AnimatedContainer", container],
  ["DecoratedBox", decoratedBox],
  ["ColoredBox", coloredBox],
  ["Card", card],
  ["Scaffold", scaffold],
  ["Divider", divider(false)],
  ["VerticalDivider", divider(true)],
  ["ClipRRect", (props, r) => clipRRect(props, r)],
  ["ClipOval", (props, r) => (props["clipBehavior"] === "none" ? {} : { clip: { ...r, oval: true } })],
  ["Opacity", (props) => opacity(props)],
  ["AnimatedOpacity", (props) => opacity(props)],
  ["FadeTransition", (props) => opacity(props)],
]);

/** Widgets that place, scroll, animate or listen, and draw nothing of their own. */
const LAYOUT_ONLY: ReadonlySet<string> = new Set([
  "Align",
  "AspectRatio",
  "Baseline",
  "Center",
  "Column",
  "ConstrainedBox",
  "Expanded",
  "FittedBox",
  "Flex",
  "Flexible",
  "FractionallySizedBox",
  "IndexedStack",
  "LimitedBox",
  "Padding",
  "Positioned",
  "Row",
  "SafeArea",
  "SizedBox",
  "Spacer",
  "Stack",
  "Table",
  "Transform",
  "Wrap",
  "CustomScrollView",
  "GridView",
  "ListView",
  "NestedScrollView",
  "PageView",
  "ReorderableListView",
  "Scrollbar",
  "SingleChildScrollView",
  "SliverFillRemaining",
  "SliverGrid",
  "SliverList",
  "SliverPadding",
  "SliverPersistentHeader",
  "SliverToBoxAdapter",
  "DataScope",
  "ForEach",
  "FormScope",
  "If",
  "Match",
  "Responsive",
  "Slot",
  "AnimatedAlign",
  "AnimatedCrossFade",
  "AnimatedPadding",
  "AnimatedPositioned",
  "AnimatedRotation",
  "AnimatedScale",
  "AnimatedSize",
  "AnimatedSlide",
  "AnimatedSwitcher",
  "Hero",
  "RotationTransition",
  "ScaleTransition",
  "SlideTransition",
  "Dismissible",
  "DragTarget",
  "Draggable",
  "GestureDetector",
  "InkWell",
  "InteractiveViewer",
  "RefreshIndicator",
]);

const TEXT_TYPES: ReadonlySet<string> = new Set(["Text", "RichText", "SelectableText"]);

/**
 * The colour of a text: its style's colour, else the one its component gives its texts, else the scheme's `onSurface`
 * (Material's default body colour).
 */
function textColor(node: PaintNode, theme: PaintTheme, inherited: Rgba | undefined): number {
  const style = node.props?.["style"];
  return (isObj(style) ? resolveColor(style["color"], theme) : undefined) ?? inherited ?? roleColor(theme, "onSurface");
}

function childrenOf(node: PaintNode): PaintNode[] {
  const slots = Object.values(node.slots ?? {}).flatMap((s) => (Array.isArray(s) ? s : [s as PaintNode]));
  return [...(node.children ?? []), ...slots].filter(
    (c): c is PaintNode => isObj(c) && typeof (c as { id?: unknown }).id === "string",
  );
}

/** Which kind of widget `type` is, for the canvas: a text, a painter, a layout widget, or one not painted yet. */
function roleOf(type: string, painted: boolean): NodePaint["role"] {
  if (TEXT_TYPES.has(type)) {
    return "text";
  }
  if (painted) {
    return "paint";
  }
  return LAYOUT_ONLY.has(type) ? "layout" : "placeholder";
}

/** The children an `IndexedStack` does not paint: every one but the one at `index`. */
function hiddenChild(node: PaintNode, i: number): boolean {
  return node.type === "IndexedStack" && i !== (num(node.props?.["index"]) ?? 0);
}

/**
 * The icon colour and size a widget gives the icons below it (Flutter's `IconTheme` of the Material 3 component), and
 * the colour it gives their texts (its `DefaultTextStyle`), when it sets one.
 */
interface IconContext {
  readonly color: Rgba;
  readonly size: number;
  readonly text?: Rgba;
}

/** The text colour of the components that set one for the texts below them. */
const TEXT_THEMES: Readonly<Record<string, ColorRole>> = {
  CircleAvatar: "onPrimaryContainer",
};

const ICON_THEMES: Readonly<Record<string, readonly [ColorRole, number]>> = {
  AppBar: ["onSurfaceVariant", 24],
  BottomAppBar: ["onSurfaceVariant", 24],
  NavigationBar: ["onSurfaceVariant", 24],
  NavigationRail: ["onSurfaceVariant", 24],
  NavigationDrawer: ["onSurfaceVariant", 24],
  ListTile: ["onSurfaceVariant", 24],
  SearchBar: ["onSurfaceVariant", 24],
  TextField: ["onSurfaceVariant", 24],
};

/** The icon theme `node`'s child in `slot` sees: its component's, an app bar's leading icon in `onSurface`. */
function iconContextOf(node: PaintNode, slot: string | undefined, theme: PaintTheme, above: IconContext): IconContext {
  const control = controlForeground(node, theme);
  if (control !== undefined) {
    return {
      color: control.icon ?? above.color,
      size: control.iconSize ?? above.size,
      ...(control.text === undefined ? {} : { text: control.text }),
    };
  }
  const entry = ICON_THEMES[node.type ?? ""];
  const text = node.type === "ListTile" && slot === "subtitle" ? "onSurfaceVariant" : TEXT_THEMES[node.type ?? ""];
  const withText = text === undefined ? above : { ...above, text: roleColor(theme, text) };
  if (entry === undefined) {
    return withText;
  }
  const role = node.type === "AppBar" && slot === "leading" ? "onSurface" : entry[0];
  return { ...withText, color: roleColor(theme, role), size: entry[1] };
}

/** Flutter's default icon theme: 24 pixels, black at 87 % in a light theme and white in a dark one. */
function defaultIcons(theme: PaintTheme): IconContext {
  return { color: argbToRgba(theme.scheme.brightness === "dark" ? 0xffffffff : 0xdd000000), size: 24 };
}

/** What an `Icon` draws: its icon of a set, at its size, in its colour, else as its icon theme says. */
function iconOf(props: Obj, theme: PaintTheme, context: IconContext): IconPaint | undefined {
  const data = props["icon"];
  if (!isObj(data) || typeof data["name"] !== "string") {
    return undefined;
  }
  return {
    name: data["name"],
    set: data["set"] === "cupertino" ? "cupertino" : "material",
    size: num(props["size"]) ?? context.size,
    color: resolveColor(props["color"], theme) ?? context.color,
  };
}

function slotEntries(node: PaintNode): [string | undefined, PaintNode][] {
  const slots = Object.entries(node.slots ?? {}).flatMap(([name, s]) =>
    (Array.isArray(s) ? s : [s as PaintNode]).map((c): [string | undefined, PaintNode] => [name, c]),
  );
  return [...(node.children ?? []).map((c): [string | undefined, PaintNode] => [undefined, c]), ...slots].filter(
    ([, c]) => isObj(c) && typeof (c as { id?: unknown }).id === "string",
  );
}

/** What a widget paints in its box `r`: a control's boxes, or its painter's. */
function paintOf(node: PaintNode, r: Rect, theme: PaintTheme): Painted {
  const control = controlPainters.get(node.type ?? "");
  if (control !== undefined) {
    return { boxes: control(node, r, theme) };
  }
  return painters.get(node.type ?? "")?.(node.props ?? {}, r, theme) ?? {};
}

/** The glyph a widget draws: an `Icon`'s, or the check of a checked checkbox. */
function ownIcon(node: PaintNode, theme: PaintTheme, icons: IconContext): IconPaint | undefined {
  if (node.type === "Icon") {
    return iconOf(node.props ?? {}, theme, icons);
  }
  const mark = checkboxMark(node, theme);
  return mark === undefined ? undefined : { name: mark.name, set: "material", size: 18, color: mark.color };
}

/** Walks a screen's widgets in paint order and records how each is drawn. */
class TreePainter {
  readonly out = new Map<string, NodePaint>();
  private order = 0;

  constructor(
    private readonly rects: ReadonlyMap<string, Rect>,
    private readonly theme: PaintTheme,
  ) {}

  hide(node: PaintNode): void {
    if (this.rects.has(node.id)) {
      this.out.set(node.id, { role: "hidden", boxes: [], order: Math.max(0, this.order - 1), opacity: 0, clips: [] });
    }
    for (const child of childrenOf(node)) {
      this.hide(child);
    }
  }

  visit(node: PaintNode, opacity: number, clips: readonly PaintClip[], icons: IconContext): void {
    const r = this.rects.get(node.id);
    const painted: Painted = r === undefined ? {} : paintOf(node, r, this.theme);
    if (r !== undefined) {
      const type = node.type ?? "";
      const icon = ownIcon(node, this.theme, icons);
      // An image with no source the canvas knows is a placeholder, as a widget not painted yet is.
      const sourceless = type === "Image" && (painted.boxes?.length ?? 0) === 0;
      const painterOf = (painters.has(type) || controlPainters.has(type)) && !sourceless;
      const role = icon === undefined || type !== "Icon" ? roleOf(type, painterOf) : "icon";
      this.record(node, role, painted, opacity, clips, { icon, text: icons.text });
    }
    const childOpacity = opacity * (painted.opacity ?? 1);
    const childClips = painted.clip === undefined ? clips : [...clips, painted.clip];
    slotEntries(node).forEach(([slot, child], i) => {
      if (hiddenChild(node, i)) {
        this.hide(child);
      } else {
        this.visit(child, childOpacity, childClips, iconContextOf(node, slot, this.theme, icons));
      }
    });
  }

  private record(
    node: PaintNode,
    role: NodePaint["role"],
    painted: Painted,
    opacity: number,
    clips: readonly PaintClip[],
    own: { readonly icon: IconPaint | undefined; readonly text: Rgba | undefined },
  ): void {
    const icon = own.icon;
    const draws = role !== "layout" && (role !== "paint" || (painted.boxes?.length ?? 0) > 0);
    this.out.set(node.id, {
      role,
      boxes: painted.boxes ?? [],
      order: draws ? this.order++ : Math.max(0, this.order - 1),
      opacity,
      clips,
      ...(role === "text" ? { textColor: textColor(node, this.theme, own.text) } : {}),
      ...(icon === undefined ? {} : { icon }),
    });
  }
}

/**
 * Paints the widget tree under `root` whose laid-out rectangles are `rects` (by widget id): each widget's boxes in paint
 * order, with the opacity and clips of its ancestors. Widgets without a rectangle are walked through, not painted.
 */
export function paintTree(
  root: PaintNode | null | undefined,
  rects: ReadonlyMap<string, Rect>,
  theme: PaintTheme,
): Map<string, NodePaint> {
  const painter = new TreePainter(rects, theme);
  if (root !== null && root !== undefined) {
    painter.visit(root, 1, [], defaultIcons(theme));
  }
  return painter.out;
}
