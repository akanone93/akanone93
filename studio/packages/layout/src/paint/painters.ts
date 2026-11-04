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
import {
  argbToRgba,
  type BoxPaint,
  borderRadius,
  decorationPaint,
  deflate,
  edgeInsets,
  elevationShadows,
  type IconPaint,
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

const painters: ReadonlyMap<string, Painter> = new Map<string, Painter>([
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

/** The colour of a text: its style's colour, else the scheme's `onSurface` (Material's default body colour). */
function textColor(node: PaintNode, theme: PaintTheme): number {
  const style = node.props?.["style"];
  return (isObj(style) ? resolveColor(style["color"], theme) : undefined) ?? roleColor(theme, "onSurface");
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

/** The icon colour and size a widget gives the icons below it (Flutter's `IconTheme` of the Material 3 component). */
interface IconContext {
  readonly color: Rgba;
  readonly size: number;
}

const ICON_THEMES: Readonly<Record<string, readonly [ColorRole, number]>> = {
  IconButton: ["onSurfaceVariant", 24],
  FilledButton: ["onPrimary", 18],
  FilledTonalButton: ["onSecondaryContainer", 18],
  ElevatedButton: ["primary", 18],
  OutlinedButton: ["primary", 18],
  TextButton: ["primary", 18],
  FloatingActionButton: ["onPrimaryContainer", 24],
  AppBar: ["onSurfaceVariant", 24],
  BottomAppBar: ["onSurfaceVariant", 24],
  NavigationBar: ["onSurfaceVariant", 24],
  NavigationRail: ["onSurfaceVariant", 24],
  NavigationDrawer: ["onSurfaceVariant", 24],
  ListTile: ["onSurfaceVariant", 24],
  SearchBar: ["onSurfaceVariant", 24],
  TextField: ["onSurfaceVariant", 24],
  ActionChip: ["primary", 18],
  Chip: ["onSurfaceVariant", 18],
  FilterChip: ["onSurfaceVariant", 18],
  InputChip: ["onSurfaceVariant", 18],
  ChoiceChip: ["onSurfaceVariant", 18],
};

/** The icon theme `node`'s child in `slot` sees: its component's, an app bar's leading icon in `onSurface`. */
function iconContextOf(node: PaintNode, slot: string | undefined, theme: PaintTheme, above: IconContext): IconContext {
  const entry = ICON_THEMES[node.type ?? ""];
  if (entry === undefined) {
    return above;
  }
  const role = node.type === "AppBar" && slot === "leading" ? "onSurface" : entry[0];
  return { color: roleColor(theme, role), size: entry[1] };
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
    const type = node.type ?? "";
    const painter = painters.get(type);
    const painted: Painted = r === undefined || painter === undefined ? {} : painter(node.props ?? {}, r, this.theme);
    if (r !== undefined) {
      const icon = type === "Icon" ? iconOf(node.props ?? {}, this.theme, icons) : undefined;
      this.record(
        node,
        icon === undefined ? roleOf(type, painter !== undefined) : "icon",
        painted,
        opacity,
        clips,
        icon,
      );
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
    icon?: IconPaint,
  ): void {
    const draws = role !== "layout" && (role !== "paint" || (painted.boxes?.length ?? 0) > 0);
    this.out.set(node.id, {
      role,
      boxes: painted.boxes ?? [],
      order: draws ? this.order++ : Math.max(0, this.order - 1),
      opacity,
      clips,
      ...(role === "text" ? { textColor: textColor(node, this.theme) } : {}),
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
