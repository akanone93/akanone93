// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Builds `@plux/layout` render trees from the layout conformance scenes in
 * `test/layout-conformance/scenes/` (STU-005, ADR-0013). The Flutter side
 * builds the same scenes from widgets; each node type here maps to the render
 * object its Flutter widget creates, and the node's `id` becomes the render
 * object's `debugId`, so [goldenRects] yields what Flutter's goldens hold.
 */

import * as layout from "../index.ts";
import {
  Affine,
  Alignment,
  AlignmentDirectional,
  type AlignmentGeometry,
  Axis,
  AxisDirection,
  BoxConstraints,
  BoxFit,
  buildContainer,
  buildSizeTransition,
  ClipShape,
  CrossAxisAlignment,
  EdgeInsets,
  EdgeInsetsDirectional,
  type EdgeInsetsGeometry,
  FixedColumnWidth,
  FixedViewportOffset,
  FlexColumnWidth,
  FlexFit,
  FlexParentData,
  FractionColumnWidth,
  flipAxisDirection,
  IntrinsicColumnWidth,
  type LayoutGolden,
  lerpAlignment,
  lerpBoxConstraints,
  lerpDouble,
  lerpEdgeInsets,
  lerpProperty,
  MainAxisAlignment,
  MainAxisSize,
  MaxColumnWidth,
  MinColumnWidth,
  Offset,
  PageViewportOffset,
  type ParagraphEngine,
  PipelineOwner,
  PrebuiltChildManager,
  RenderAspectRatio,
  RenderBaseline,
  type RenderBox,
  RenderClip,
  RenderConstrainedBox,
  RenderDecoratedBox,
  RenderFittedBox,
  RenderFlex,
  RenderFractionallySizedOverflowBox,
  RenderFractionalTranslation,
  RenderIndexedStack,
  RenderIntrinsicHeight,
  RenderIntrinsicWidth,
  RenderLimitedBox,
  type RenderObject,
  RenderOpacity,
  RenderPadding,
  RenderParagraph,
  RenderPositionedBox,
  RenderShrinkWrappingViewport,
  type RenderSliver,
  RenderSliverFillRemaining,
  RenderSliverFillRemainingAndOverscroll,
  RenderSliverFillRemainingWithScrollable,
  RenderSliverFillViewport,
  RenderSliverFixedExtentList,
  RenderSliverFloatingPersistentHeader,
  RenderSliverFloatingPinnedPersistentHeader,
  RenderSliverFractionalPadding,
  RenderSliverGrid,
  RenderSliverList,
  RenderSliverPadding,
  RenderSliverPinnedPersistentHeader,
  RenderSliverScrollingPersistentHeader,
  RenderSliverToBoxAdapter,
  RenderSliverVariedExtentList,
  RenderStack,
  RenderTable,
  RenderTransform,
  RenderView,
  RenderViewport,
  RenderWrap,
  ScrollCacheExtent,
  Size,
  type SliverGridDelegate,
  SliverGridDelegateWithFixedCrossAxisCount,
  SliverGridDelegateWithMaxCrossAxisExtent,
  StackFit,
  StackParentData,
  type StrutStyle,
  TableCellParentData,
  TableCellVerticalAlignment,
  type TableColumnWidth,
  TextAlign,
  TextBaseline,
  TextDirection,
  type TextLeadingDistribution,
  TextOverflow,
  TextScaler,
  type TextStyle,
  textDirectionToAxisDirection,
  VerticalDirection,
  type ViewportOffset,
  WrapAlignment,
  WrapCrossAlignment,
} from "../index.ts";
import { layer2Types, layer2WidgetBuilders } from "../widgets/layer2.ts";
import { defaultScreenTheme, type WidgetContext, type WidgetNode } from "../widgets/screen.ts";
import {
  buildCupertino,
  buildCupertinoSliver,
  type CupertinoContext,
  cupertinoAliasTypes,
  cupertinoMultiChildTypes,
  cupertinoSliverTypes,
  cupertinoTypes,
} from "./cupertino.ts";
import { buildMaterial, materialTypes, style } from "./material-scene.ts";

/** What text nodes are laid out with: the paragraph engine and the golden's text scale. */
export interface SceneText {
  engine: ParagraphEngine;
  textScaler: TextScaler;
  /** The style `Text` nodes inherit from the Material composite they sit in. */
  base?: TextStyle;
  /** The `DefaultTextStyle` flags `Text` nodes inherit from it. */
  defaults?: TextDefaults;
  /** The frame's device pixel ratio, for hairlines that are one physical pixel. */
  devicePixelRatio?: number;
  /** The frame's size, for widgets that size themselves to the screen. */
  screen?: { width: number; height: number };
  /** The render objects of [hostType] nodes, by node id: widgets the host built itself. */
  hosts?: ReadonlyMap<string, RenderBox>;
}

/** The type of a node standing for a render object its host built (the screen builder's own widgets). */
export const hostType = "__Host";

/** The `DefaultTextStyle` fields besides the style that a composite sets. */
export interface TextDefaults {
  softWrap?: boolean;
  overflow?: string;
  maxLines?: number;
}

/** One node of a scene; `props` use Flutter's parameter names. */
export interface SceneNode {
  id: string;
  type: string;
  /** The part of a Material composite this node fills (default `child`). */
  slot?: string;
  props: Record<string, unknown>;
  children?: SceneNode[];
  /** The named children of a Plux node, as the runtime's golden writer reads them (a `SizedBox`'s `child`). */
  slots?: Record<string, SceneNode>;
}

/** The nodes below `node`: its children, then its slots. */
export function childrenOf(node: SceneNode): SceneNode[] {
  return [...(node.children ?? []), ...Object.values(node.slots ?? {})];
}

/** A conformance scene, shared verbatim with the Flutter golden writer. */
export interface Scene {
  name: string;
  /** A Layer 2 scene: its root is a Plux node, laid out in the page the runtime's golden writer builds. */
  layer2?: boolean;
  /** Ids of nodes the component does not build (a wizard's other steps); the golden writer records no rect for them. */
  unbuilt?: string[];
  /** The animation value in [0, 1] animated nodes are laid out at; absent means 1 (their `to` state). */
  t?: number;
  root: SceneNode;
}

/** Node types the box-protocol port reproduces today. */
export const boxTypes: ReadonlySet<string> = new Set([
  "SizedBox",
  "ConstrainedBox",
  "Padding",
  "Align",
  "Center",
  "DecoratedBox",
  "TestBox",
  "LimitedBox",
  "FittedBox",
  "AspectRatio",
  "FractionallySizedBox",
  "Baseline",
  "IntrinsicWidth",
  "IntrinsicHeight",
  "Table",
  "TableCell",
  "Transform",
  "ClipRect",
  "ClipRRect",
  "ClipOval",
  "ClipPath",
  "Opacity",
  "AnimatedContainer",
  "AnimatedPadding",
  "AnimatedAlign",
  "AnimatedPositioned",
  "AnimatedOpacity",
  "SizeTransition",
  "ScaleTransition",
  "SlideTransition",
  "FadeTransition",
]);

/** Node types that need `RenderFlex`. */
export const flexTypes: ReadonlySet<string> = new Set(["Row", "Column", "Flex", "Expanded", "Flexible", "Spacer"]);

/** Whether `@plux/layout` exports `RenderFlex` yet. */
export const stackTypes: ReadonlySet<string> = new Set([
  "Stack",
  "IndexedStack",
  "Positioned",
  "PositionedDirectional",
  "Wrap",
]);

/**
 * Viewports and the scroll views built on them. Their `children` are slivers
 * (`Viewport`, `ShrinkWrappingViewport`, `CustomScrollView`) or boxes
 * (`ListView`, `GridView`, `PageView`, `ReorderableListView`); the last child
 * of a `NestedScrollView` is its body, the others its header slivers. A
 * `scrollOffset` (`offset` on the viewports) is where the view is scrolled to;
 * it must lie within the scrollable range, as Flutter would clamp it by
 * animating, which a golden cannot capture.
 */
export const scrollTypes: ReadonlySet<string> = new Set([
  "Viewport",
  "ShrinkWrappingViewport",
  "CustomScrollView",
  "ListView",
  "GridView",
  "PageView",
  "NestedScrollView",
  "ReorderableListView",
]);

/** Slivers: the nodes a viewport lays out. */
export const sliverTypes: ReadonlySet<string> = new Set([
  ...cupertinoSliverTypes,
  "SliverToBoxAdapter",
  "SliverList",
  "SliverFixedExtentList",
  "SliverVariedExtentList",
  "SliverGrid",
  "SliverFillViewport",
  "SliverFillRemaining",
  "SliverPadding",
  "SliverPersistentHeader",
  "SliverAppBar",
]);

const multiChildTypes: ReadonlySet<string> = new Set([
  "Row",
  "Column",
  "Flex",
  "Stack",
  "IndexedStack",
  "Wrap",
  "Table",
]);

export function renderFlexAvailable(): boolean {
  return "RenderFlex" in layout;
}

/** The node types a scene may use to be compared; everything else awaits an engine feature. */
/** Node types laid out by `RenderParagraph`, which needs a [ParagraphEngine]. */
export const textTypes: ReadonlySet<string> = new Set(["Text", ...layer2Types]);

export function supportedTypes(): ReadonlySet<string> {
  return new Set([
    ...boxTypes,
    ...textTypes,
    ...materialTypes,
    ...stackTypes,
    ...scrollTypes,
    ...sliverTypes,
    ...cupertinoTypes,
    ...(renderFlexAvailable() ? flexTypes : []),
  ]);
}

/** Every node type the scene uses. */
export function sceneTypes(node: SceneNode, into: Set<string> = new Set()): Set<string> {
  into.add(node.type);
  for (const child of childrenOf(node)) {
    sceneTypes(child, into);
  }
  return into;
}

/** A scene number: JSON cannot hold infinity, so scenes spell it `"infinity"`. */
function num(value: unknown, what: string): number {
  if (value === "infinity") {
    return Number.POSITIVE_INFINITY;
  }
  if (typeof value !== "number") {
    throw new Error(`${what}: expected a number or "infinity", got ${JSON.stringify(value)}`);
  }
  return value;
}

/** A scene field by name (scene props are open-ended records). */
function field(record: Record<string, unknown>, name: string): unknown {
  return record[name];
}

function optionalNum(props: Record<string, unknown>, name: string): number | null {
  return name in props ? num(props[name], name) : null;
}

/**
 * Insets as a scene or a document spells them: `[left, top, right, bottom]`, or
 * an object of `all`, `horizontal`, `vertical` and single sides; `left` or
 * `right` make it physical, `start`/`end` directional.
 */
export function padding(value: unknown): EdgeInsetsGeometry {
  if (Array.isArray(value)) {
    return EdgeInsets.fromLTRB(
      num(value[0], "left"),
      num(value[1], "top"),
      num(value[2], "right"),
      num(value[3], "bottom"),
    );
  }
  const sides = value as Record<string, unknown>;
  const side = (name: string, axis: string): number =>
    num(field(sides, name) ?? field(sides, axis) ?? field(sides, "all") ?? 0, name);
  if ("left" in sides || "right" in sides) {
    return EdgeInsets.fromLTRB(
      side("left", "horizontal"),
      side("top", "vertical"),
      side("right", "horizontal"),
      side("bottom", "vertical"),
    );
  }
  return EdgeInsetsDirectional.fromSTEB(
    side("start", "horizontal"),
    side("top", "vertical"),
    side("end", "horizontal"),
    side("bottom", "vertical"),
  );
}

/** The named alignments of a document's `Alignment` value, as `[x, y]`. */
const alignmentPresets: Readonly<Record<string, readonly [number, number]>> = {
  topLeft: [-1, -1],
  topCenter: [0, -1],
  topRight: [1, -1],
  centerLeft: [-1, 0],
  center: [0, 0],
  centerRight: [1, 0],
  bottomLeft: [-1, 1],
  bottomCenter: [0, 1],
  bottomRight: [1, 1],
};

/** The named directional alignments, as `[start, y]`. */
const directionalPresets: Readonly<Record<string, readonly [number, number]>> = {
  topStart: [-1, -1],
  topEnd: [1, -1],
  centerStart: [-1, 0],
  centerEnd: [1, 0],
  bottomStart: [-1, 1],
  bottomEnd: [1, 1],
};

/** An alignment as a scene (`[x, y]`, `{start, y}`) or a document (`{x, y}`, `{start, y}`, a named preset) spells it. */
export function alignment(value: unknown): AlignmentGeometry {
  if (value === undefined) {
    return Alignment.center;
  }
  if (Array.isArray(value)) {
    return new Alignment(num(value[0], "x"), num(value[1], "y"));
  }
  const parts = value as Record<string, unknown>;
  if ("start" in parts) {
    return new AlignmentDirectional(num(field(parts, "start"), "start"), num(field(parts, "y") ?? 0, "y"));
  }
  if ("x" in parts) {
    return new Alignment(num(field(parts, "x"), "x"), num(field(parts, "y") ?? 0, "y"));
  }
  for (const name of Object.keys(parts)) {
    const flat = alignmentPresets[name];
    if (flat !== undefined) {
      return new Alignment(flat[0], flat[1]);
    }
    const directional = directionalPresets[name];
    if (directional !== undefined) {
      return new AlignmentDirectional(directional[0], directional[1]);
    }
  }
  return Alignment.center;
}

function singleChild(node: SceneNode, children: RenderBox[]): RenderBox | null {
  if (!multiChildTypes.has(node.type) && !cupertinoMultiChildTypes.has(node.type) && children.length > 1) {
    throw new Error(`node "${node.id}" of type ${node.type} takes at most one child`);
  }
  return children[0] ?? null;
}

function unsupported(node: SceneNode): Error {
  if (flexTypes.has(node.type)) {
    return new Error(
      `node "${node.id}": ${node.type} needs RenderFlex; ` +
        (renderFlexAvailable() ? "map the flex types in scene/scene.ts" : "it is not exported yet"),
    );
  }
  return new Error(`node "${node.id}": unknown scene node type "${node.type}"`);
}

/** A Text node's optional `strutStyle` prop as a [StrutStyle]. */
function buildStrut(props: Record<string, unknown> | undefined): StrutStyle | null {
  if (props === undefined) {
    return null;
  }
  const strut: StrutStyle = {};
  const family = field(props, "fontFamily");
  if (typeof family === "string") {
    strut.fontFamily = family;
  }
  for (const name of ["fontSize", "height", "leading"] as const) {
    const value = optionalNum(props, name);
    if (value !== null) {
      strut[name] = value;
    }
  }
  const force = field(props, "forceStrutHeight");
  if (force !== undefined) {
    strut.forceStrutHeight = force as boolean;
  }
  const distribution = field(props, "leadingDistribution");
  if (distribution !== undefined) {
    strut.leadingDistribution = distribution as TextLeadingDistribution;
  }
  return strut;
}

/** Text: a [RenderParagraph] with the `Text` widget's defaults and the ambient text scale. */
function buildText(node: SceneNode, textDirection: TextDirection, text: SceneText | null): RenderBox {
  if (text === null) {
    throw new Error(`node "${node.id}": Text needs a ParagraphEngine`);
  }
  const props = node.props;
  const style = (field(props, "style") ?? {}) as Record<string, unknown>;
  const textStyle: TextStyle = { ...text.base };
  const fontFamily = field(style, "fontFamily");
  if (typeof fontFamily === "string") {
    textStyle.fontFamily = fontFamily;
  }
  for (const name of ["fontSize", "fontWeight", "height", "letterSpacing"] as const) {
    const value = optionalNum(style, name);
    if (value !== null) {
      textStyle[name] = value;
    }
  }
  const distribution = field(style, "leadingDistribution");
  if (distribution !== undefined) {
    textStyle.leadingDistribution = distribution as TextLeadingDistribution;
  }
  const maxLines = optionalNum(props, "maxLines") ?? text.defaults?.maxLines ?? null;
  return new RenderParagraph({
    strutStyle: buildStrut(field(props, "strutStyle") as Record<string, unknown> | undefined),
    text: { text: String(field(props, "text") ?? field(props, "data")), style: textStyle },
    textDirection,
    engine: text.engine,
    textScaler: text.textScaler,
    textAlign: (field(props, "textAlign") ?? TextAlign.start) as TextAlign,
    maxLines,
    overflow: (field(props, "overflow") ?? text.defaults?.overflow ?? TextOverflow.clip) as TextOverflow,
    softWrap: (field(props, "softWrap") ?? text.defaults?.softWrap ?? true) as boolean,
  });
}

/** Releases the paragraphs of a scene's render tree. */
export function disposeParagraphs(node: RenderObject): void {
  if (node instanceof RenderParagraph) {
    node.dispose();
  }
  node.visitChildren(disposeParagraphs);
}

/** Row, Column and Flex: a [RenderFlex] with Flutter's widget defaults. */
function buildFlex(node: SceneNode, children: RenderBox[], textDirection: TextDirection): RenderBox {
  const props = node.props;
  return new RenderFlex({
    children,
    direction:
      node.type === "Row"
        ? Axis.horizontal
        : node.type === "Column"
          ? Axis.vertical
          : ((field(props, "direction") ?? Axis.horizontal) as Axis),
    mainAxisAlignment: (field(props, "mainAxisAlignment") ?? MainAxisAlignment.start) as MainAxisAlignment,
    mainAxisSize: (field(props, "mainAxisSize") ?? MainAxisSize.max) as MainAxisSize,
    crossAxisAlignment: (field(props, "crossAxisAlignment") ?? CrossAxisAlignment.center) as CrossAxisAlignment,
    textDirection,
    verticalDirection: (field(props, "verticalDirection") ?? VerticalDirection.down) as VerticalDirection,
    spacing: optionalNum(props, "spacing") ?? 0,
  });
}

/** Expanded, Flexible and Spacer: flex parent data on the child's render object (Spacer: a shrunk box). */
function applyFlexible(node: SceneNode, child: RenderBox | null): RenderBox {
  const props = node.props;
  // Flexible has no render object: it sets flex parent data on its child.
  const flexible =
    node.type === "Spacer"
      ? new RenderConstrainedBox({ additionalConstraints: BoxConstraints.tightFor({ width: 0, height: 0 }) })
      : child;
  if (flexible === null) {
    throw new Error(`node "${node.id}" of type ${node.type} needs a child`);
  }
  const parentData = new FlexParentData();
  parentData.flex = optionalNum(props, "flex") ?? 1;
  parentData.fit = node.type === "Flexible" ? ((field(props, "fit") ?? FlexFit.loose) as FlexFit) : FlexFit.tight;
  flexible.parentData = parentData;
  if (node.type === "Spacer") {
    flexible.debugId = node.id;
  }
  return flexible;
}

/** Stack and IndexedStack: a [RenderStack] with Flutter's widget defaults (topStart, loose). */
function buildStack(node: SceneNode, children: RenderBox[], textDirection: TextDirection): RenderBox {
  const props = node.props;
  const options = {
    children,
    alignment:
      field(props, "alignment") === undefined ? AlignmentDirectional.topStart : alignment(field(props, "alignment")),
    textDirection,
    fit: (field(props, node.type === "IndexedStack" ? "sizing" : "fit") ?? StackFit.loose) as StackFit,
  };
  return node.type === "IndexedStack"
    ? new RenderIndexedStack({ ...options, index: optionalNum(props, "index") ?? 0 })
    : new RenderStack(options);
}

/** Positioned and PositionedDirectional: stack parent data on the child's render object. */
function applyPositioned(node: SceneNode, child: RenderBox | null, textDirection: TextDirection): RenderBox {
  if (child === null) {
    throw new Error(`node "${node.id}" of type ${node.type} needs a child`);
  }
  const props = node.props;
  const parentData = new StackParentData();
  parentData.top = optionalNum(props, "top");
  parentData.bottom = optionalNum(props, "bottom");
  parentData.width = optionalNum(props, "width");
  parentData.height = optionalNum(props, "height");
  if (node.type === "PositionedDirectional") {
    parentData.setDirectional(textDirection, optionalNum(props, "start"), optionalNum(props, "end"));
  } else {
    parentData.left = optionalNum(props, "left");
    parentData.right = optionalNum(props, "right");
  }
  child.parentData = parentData;
  return child;
}

/** Wrap: a [RenderWrap] with Flutter's widget defaults. */
function buildWrap(node: SceneNode, children: RenderBox[], textDirection: TextDirection): RenderBox {
  const props = node.props;
  return new RenderWrap({
    children,
    direction: (field(props, "direction") ?? Axis.horizontal) as Axis,
    alignment: (field(props, "alignment") ?? WrapAlignment.start) as WrapAlignment,
    spacing: optionalNum(props, "spacing") ?? 0,
    runAlignment: (field(props, "runAlignment") ?? WrapAlignment.start) as WrapAlignment,
    runSpacing: optionalNum(props, "runSpacing") ?? 0,
    crossAxisAlignment: (field(props, "crossAxisAlignment") ?? WrapCrossAlignment.start) as WrapCrossAlignment,
    textDirection,
    verticalDirection: (field(props, "verticalDirection") ?? VerticalDirection.down) as VerticalDirection,
  });
}

/** A scene object field, or an empty record. */
function record(value: unknown): Record<string, unknown> {
  return (value ?? {}) as Record<string, unknown>;
}

/** An animated node's `from` and `to` props. */
function fromTo(node: SceneNode): { from: Record<string, unknown>; to: Record<string, unknown> } {
  return { from: record(field(node.props, "from")), to: record(field(node.props, "to")) };
}

/** An animated property: `from` and `to` values interpolated at `t` (see `lerpProperty`). */
function animatedProperty<T>(
  from: Record<string, unknown>,
  to: Record<string, unknown>,
  name: string,
  parse: (value: unknown) => T,
  lerp: (a: T, b: T, t: number) => T,
  t: number,
): T | null {
  return lerpProperty(name in from ? parse(from[name]) : null, name in to ? parse(to[name]) : null, t, lerp);
}

/** A Table column width spec: `{type: fixed|fraction|flex|intrinsic|max|min, value, flex, a, b}`. */
function columnWidth(spec: unknown): TableColumnWidth {
  const props = record(spec);
  switch (field(props, "type")) {
    case "fixed":
      return new FixedColumnWidth(num(field(props, "value"), "value"));
    case "fraction":
      return new FractionColumnWidth(num(field(props, "value"), "value"));
    case "flex":
      return new FlexColumnWidth(optionalNum(props, "value") ?? 1);
    case "intrinsic":
      return new IntrinsicColumnWidth(optionalNum(props, "flex"));
    case "max":
      return new MaxColumnWidth(columnWidth(field(props, "a")), columnWidth(field(props, "b")));
    case "min":
      return new MinColumnWidth(columnWidth(field(props, "a")), columnWidth(field(props, "b")));
    default:
      throw new Error(`unknown table column width ${JSON.stringify(spec)}`);
  }
}

/** Table: a [RenderTable] over the node's children, row-major, `columns` per row. */
function buildTable(node: SceneNode, children: RenderBox[], textDirection: TextDirection): RenderBox {
  const props = node.props;
  const columns = num(field(props, "columns"), "columns");
  const specs = record(field(props, "columnWidths"));
  const columnWidths = new Map<number, TableColumnWidth>(
    Object.entries(specs).map(([index, spec]) => [Number(index), columnWidth(spec)]),
  );
  return new RenderTable({
    columns,
    rows: columns === 0 ? 0 : children.length / columns,
    children,
    columnWidths,
    defaultColumnWidth:
      "defaultColumnWidth" in props ? columnWidth(field(props, "defaultColumnWidth")) : new FlexColumnWidth(),
    textDirection,
    defaultVerticalAlignment: (field(props, "defaultVerticalAlignment") ??
      TableCellVerticalAlignment.top) as TableCellVerticalAlignment,
    textBaseline: (field(props, "textBaseline") ?? null) as TextBaseline | null,
  });
}

/** TableCell: table cell parent data on the child's render object. */
function applyTableCell(node: SceneNode, child: RenderBox | null): RenderBox {
  if (child === null) {
    throw new Error(`node "${node.id}" of type ${node.type} needs a child`);
  }
  const parentData = new TableCellParentData();
  parentData.verticalAlignment = (field(node.props, "verticalAlignment") ?? null) as TableCellVerticalAlignment | null;
  child.parentData = parentData;
  return child;
}

/** AnimatedPositioned: stack parent data with each side interpolated at `t`. */
function applyAnimatedPositioned(node: SceneNode, child: RenderBox | null, t: number): RenderBox {
  if (child === null) {
    throw new Error(`node "${node.id}" of type ${node.type} needs a child`);
  }
  const { from, to } = fromTo(node);
  const side = (name: string): number | null =>
    animatedProperty(from, to, name, (value) => num(value, name), lerpDouble, t);
  const parentData = new StackParentData();
  parentData.left = side("left");
  parentData.top = side("top");
  parentData.right = side("right");
  parentData.bottom = side("bottom");
  parentData.width = side("width");
  parentData.height = side("height");
  child.parentData = parentData;
  return child;
}

/** Transform: translate, then rotate, then scale, about the alignment. */
function buildTransform(node: SceneNode, textDirection: TextDirection, child: RenderBox | null): RenderBox {
  const props = node.props;
  const translate = (field(props, "translate") ?? [0, 0]) as unknown[];
  const scale = field(props, "scale") ?? 1;
  const [sx, sy] = Array.isArray(scale)
    ? [num(scale[0], "sx"), num(scale[1], "sy")]
    : [num(scale, "scale"), num(scale, "scale")];
  const matrix = Affine.translation(num(translate[0], "dx"), num(translate[1], "dy"))
    .multiply(Affine.rotation(optionalNum(props, "rotate") ?? 0))
    .scale(sx, sy);
  return new RenderTransform({
    transform: matrix,
    alignment: "alignment" in props ? alignment(field(props, "alignment")) : null,
    textDirection,
    child,
  });
}

/** The animated nodes (implicit animations): the layout `t` of the way from `from` to `to`. */
function buildAnimated(node: SceneNode, child: RenderBox | null, textDirection: TextDirection, t: number): RenderBox {
  const { from, to } = fromTo(node);
  const prop = <T>(name: string, parse: (value: unknown) => T, lerp: (a: T, b: T, t: number) => T): T | null =>
    animatedProperty(from, to, name, parse, lerp, t);
  const number = (name: string): number | null => prop(name, (value) => num(value, name), lerpDouble);
  const insets = (name: string): EdgeInsets | null =>
    prop(name, (value) => padding(value) as EdgeInsets, lerpEdgeInsets);
  switch (node.type) {
    case "AnimatedContainer": {
      const constraintsOf = (props: Record<string, unknown>): BoxConstraints | null =>
        "width" in props || "height" in props
          ? BoxConstraints.tightFor({ width: optionalNum(props, "width"), height: optionalNum(props, "height") })
          : null;
      const box = buildContainer({
        alignment: prop("alignment", alignment, lerpAlignment),
        padding: insets("padding"),
        constraints: lerpProperty(constraintsOf(from), constraintsOf(to), t, lerpBoxConstraints),
        margin: insets("margin"),
        textDirection,
        child,
      });
      if (box === null || box === child) {
        throw new Error(`node "${node.id}": AnimatedContainer needs at least one layout property`);
      }
      return box;
    }
    case "AnimatedPadding":
      return new RenderPadding({
        padding: insets("padding") ?? EdgeInsets.zero,
        textDirection,
        child,
      });
    case "AnimatedAlign":
      return new RenderPositionedBox({
        alignment: prop("alignment", alignment, lerpAlignment) ?? Alignment.center,
        textDirection,
        widthFactor: number("widthFactor"),
        heightFactor: number("heightFactor"),
        child,
      });
    default:
      // AnimatedOpacity: FadeTransition under the hood.
      return new RenderOpacity({ opacity: number("opacity") ?? 1, child });
  }
}

/** The transition nodes: an animation running from `begin` to `end`, here at `t`. */
function buildTransition(node: SceneNode, child: RenderBox | null, textDirection: TextDirection, t: number): RenderBox {
  const props = node.props;
  const value =
    node.type === "SlideTransition"
      ? 0
      : lerpDouble(optionalNum(props, "begin") ?? 0, optionalNum(props, "end") ?? 1, t);
  switch (node.type) {
    case "SizeTransition":
      return buildSizeTransition({
        axis: (field(props, "axis") ?? Axis.vertical) as Axis,
        sizeFactor: value,
        alignment: "alignment" in props ? alignment(field(props, "alignment")) : null,
        fixedCrossAxisSizeFactor: optionalNum(props, "fixedCrossAxisSizeFactor"),
        textDirection,
        child,
      });
    case "ScaleTransition":
      return new RenderTransform({
        transform: Affine.scaling(value, value),
        alignment: alignment(field(props, "alignment")),
        textDirection,
        child,
      });
    case "SlideTransition": {
      const begin = (field(props, "begin") ?? [0, 0]) as unknown[];
      const end = (field(props, "end") ?? [0, 0]) as unknown[];
      return new RenderFractionalTranslation({
        translation: new Offset(
          lerpDouble(num(begin[0], "begin.dx"), num(end[0], "end.dx"), t),
          lerpDouble(num(begin[1], "begin.dy"), num(end[1], "end.dy"), t),
        ),
        child,
      });
    }
    default:
      // FadeTransition.
      return new RenderOpacity({ opacity: value, child });
  }
}

/** The other box widgets: limited, fitted, aspect-ratio, fractionally sized, baseline, intrinsic, clips, opacity. */
function buildOtherBox(node: SceneNode, child: RenderBox | null, textDirection: TextDirection): RenderBox | null {
  const props = node.props;
  switch (node.type) {
    case "LimitedBox":
      return new RenderLimitedBox({
        maxWidth: optionalNum(props, "maxWidth") ?? Number.POSITIVE_INFINITY,
        maxHeight: optionalNum(props, "maxHeight") ?? Number.POSITIVE_INFINITY,
        child,
      });
    case "FittedBox":
      return new RenderFittedBox({
        fit: (field(props, "fit") ?? BoxFit.contain) as BoxFit,
        alignment: alignment(field(props, "alignment")),
        textDirection,
        child,
      });
    case "AspectRatio":
      return new RenderAspectRatio({ aspectRatio: num(field(props, "aspectRatio"), "aspectRatio"), child });
    case "FractionallySizedBox":
      return new RenderFractionallySizedOverflowBox({
        alignment: alignment(field(props, "alignment")),
        textDirection,
        widthFactor: optionalNum(props, "widthFactor"),
        heightFactor: optionalNum(props, "heightFactor"),
        child,
      });
    case "Baseline":
      return new RenderBaseline({
        baseline: num(field(props, "baseline"), "baseline"),
        baselineType: (field(props, "baselineType") ?? TextBaseline.alphabetic) as TextBaseline,
        child,
      });
    case "IntrinsicWidth":
      return new RenderIntrinsicWidth({
        stepWidth: optionalNum(props, "stepWidth"),
        stepHeight: optionalNum(props, "stepHeight"),
        child,
      });
    case "IntrinsicHeight":
      return new RenderIntrinsicHeight(child);
    case "ClipRect":
      return new RenderClip({ shape: ClipShape.rect, child });
    case "ClipRRect":
      return new RenderClip({ shape: ClipShape.rrect, child });
    case "ClipOval":
      return new RenderClip({ shape: ClipShape.oval, child });
    case "ClipPath":
      return new RenderClip({ shape: ClipShape.path, child });
    case "Opacity":
      return new RenderOpacity({ opacity: optionalNum(props, "opacity") ?? 1, child });
    default:
      return null;
  }
}

/** The viewport's axis direction for a scroll view's `scrollDirection` and `reverse` (`getAxisDirectionFromAxisReverseAndDirectionality`). */
function scrollAxisDirection(props: Record<string, unknown>, textDirection: TextDirection): AxisDirection {
  const reverse = field(props, "reverse") === true;
  if ((field(props, "scrollDirection") ?? Axis.vertical) === Axis.horizontal) {
    const direction = textDirectionToAxisDirection(textDirection);
    return reverse ? flipAxisDirection(direction) : direction;
  }
  return reverse ? AxisDirection.up : AxisDirection.down;
}

/** The cross axis direction a `Viewport` picks when none is given (`Viewport.getDefaultCrossAxisDirection`). */
function defaultCrossAxisDirection(axisDirection: AxisDirection, textDirection: TextDirection): AxisDirection {
  return axisDirection === AxisDirection.up || axisDirection === AxisDirection.down
    ? textDirectionToAxisDirection(textDirection)
    : AxisDirection.down;
}

function gridDelegate(node: SceneNode): SliverGridDelegate {
  const props = node.props;
  const spacing = {
    mainAxisSpacing: optionalNum(props, "mainAxisSpacing") ?? 0,
    crossAxisSpacing: optionalNum(props, "crossAxisSpacing") ?? 0,
    childAspectRatio: optionalNum(props, "childAspectRatio") ?? 1,
    mainAxisExtent: optionalNum(props, "mainAxisExtent"),
  };
  const maxCrossAxisExtent = optionalNum(props, "maxCrossAxisExtent");
  return maxCrossAxisExtent === null
    ? new SliverGridDelegateWithFixedCrossAxisCount(num(field(props, "crossAxisCount"), "crossAxisCount"), spacing)
    : new SliverGridDelegateWithMaxCrossAxisExtent(maxCrossAxisExtent, spacing);
}

/** The box children of a sliver node, built in `textDirection`. */
function boxChildren(node: SceneNode, textDirection: TextDirection, text: SceneText | null): RenderBox[] {
  return (node.children ?? []).map((child) => buildNode(child, textDirection, text));
}

/** The one child of a sliver node that wraps a single box, or null. */
function onlyBoxChild(node: SceneNode, textDirection: TextDirection, text: SceneText | null): RenderBox | null {
  return singleChild(node, boxChildren(node, textDirection, text));
}

/** SliverPersistentHeader and SliverAppBar: the header kind follows `pinned` and `floating`. */
function buildHeader(node: SceneNode, textDirection: TextDirection, text: SceneText | null): RenderSliver {
  const props = node.props;
  let minExtent: number;
  let maxExtent: number;
  if (node.type === "SliverAppBar") {
    // SliverAppBar's delegate: collapsedHeight or toolbarHeight when collapsed, expandedHeight or toolbarHeight expanded.
    const toolbarHeight = optionalNum(props, "toolbarHeight") ?? 56;
    minExtent = optionalNum(props, "collapsedHeight") ?? toolbarHeight;
    maxExtent = Math.max(optionalNum(props, "expandedHeight") ?? toolbarHeight, minExtent);
  } else {
    minExtent = num(field(props, "minExtent"), "minExtent");
    maxExtent = num(field(props, "maxExtent"), "maxExtent");
  }
  const child =
    onlyBoxChild(node, textDirection, text) ??
    // The delegate's child when the scene gives none: SizedBox.expand().
    new RenderConstrainedBox({
      additionalConstraints: BoxConstraints.tightFor({
        width: Number.POSITIVE_INFINITY,
        height: Number.POSITIVE_INFINITY,
      }),
    });
  const options = { child, minExtent, maxExtent };
  const pinned = field(props, "pinned") === true;
  const floating = field(props, "floating") === true;
  if (pinned) {
    return floating
      ? new RenderSliverFloatingPinnedPersistentHeader(options)
      : new RenderSliverPinnedPersistentHeader(options);
  }
  return floating
    ? new RenderSliverFloatingPersistentHeader(options)
    : new RenderSliverScrollingPersistentHeader(options);
}

/** A sliver node: the render object its Flutter sliver widget creates. */
function buildSliver(node: SceneNode, textDirection: TextDirection, text: SceneText | null): RenderSliver {
  const props = node.props;
  let sliver: RenderSliver;
  switch (node.type) {
    case "SliverToBoxAdapter":
      sliver = new RenderSliverToBoxAdapter({ child: onlyBoxChild(node, textDirection, text) });
      break;
    case "SliverList":
      sliver = new RenderSliverList({ childManager: new PrebuiltChildManager(boxChildren(node, textDirection, text)) });
      break;
    case "SliverFixedExtentList":
      sliver = new RenderSliverFixedExtentList({
        childManager: new PrebuiltChildManager(boxChildren(node, textDirection, text)),
        itemExtent: num(field(props, "itemExtent"), "itemExtent"),
      });
      break;
    case "SliverVariedExtentList": {
      const extents = (field(props, "itemExtents") as unknown[]).map((extent) => num(extent, "itemExtents"));
      sliver = new RenderSliverVariedExtentList({
        childManager: new PrebuiltChildManager(boxChildren(node, textDirection, text)),
        itemExtentBuilder: (index) => extents[index] ?? null,
      });
      break;
    }
    case "SliverGrid":
      sliver = new RenderSliverGrid({
        childManager: new PrebuiltChildManager(boxChildren(node, textDirection, text)),
        gridDelegate: gridDelegate(node),
      });
      break;
    case "SliverFillViewport": {
      // SliverFillViewport builds a fractional padding around the page list; the padding is the keyed render object.
      const viewportFraction = optionalNum(props, "viewportFraction") ?? 1;
      sliver = new RenderSliverFractionalPadding({
        viewportFraction: field(props, "padEnds") === false ? 0 : Math.min(1, Math.max(0, 1 - viewportFraction)) / 2,
        child: new RenderSliverFillViewport({
          childManager: new PrebuiltChildManager(boxChildren(node, textDirection, text)),
          viewportFraction,
        }),
      });
      break;
    }
    case "SliverFillRemaining": {
      const child = onlyBoxChild(node, textDirection, text);
      if (field(props, "hasScrollBody") !== false) {
        sliver = new RenderSliverFillRemainingWithScrollable({ child });
      } else {
        sliver =
          field(props, "fillOverscroll") === true
            ? new RenderSliverFillRemainingAndOverscroll({ child })
            : new RenderSliverFillRemaining({ child });
      }
      break;
    }
    case "SliverPadding": {
      const inner = node.children?.[0];
      if ((node.children ?? []).length > 1) {
        throw new Error(`node "${node.id}" of type ${node.type} takes at most one child`);
      }
      sliver = new RenderSliverPadding({
        padding: padding(field(props, "padding")),
        textDirection,
        child: inner === undefined ? null : buildSliver(inner, textDirection, text),
      });
      break;
    }
    case "SliverPersistentHeader":
    case "SliverAppBar":
      sliver = buildHeader(node, textDirection, text);
      break;
    default:
      if (!cupertinoSliverTypes.has(node.type)) {
        throw unsupported(node);
      }
      sliver = buildCupertinoSliver(node, cupertinoContext(textDirection, text, 1));
      break;
  }
  sliver.debugId = node.id;
  return sliver;
}

/** The slivers of a viewport or `CustomScrollView` node. */
function sliverChildren(node: SceneNode, textDirection: TextDirection, text: SceneText | null): RenderSliver[] {
  return (node.children ?? []).map((child) => {
    if (!sliverTypes.has(child.type)) {
      throw new Error(`node "${child.id}": ${child.type} is not a sliver, so ${node.type} "${node.id}" cannot hold it`);
    }
    return buildSliver(child, textDirection, text);
  });
}

/** Wraps `sliver` in a `SliverPadding` when the node has `padding`, as `ListView` and `GridView` do. */
function paddedSliver(node: SceneNode, sliver: RenderSliver, textDirection: TextDirection): RenderSliver {
  const value = field(node.props, "padding");
  return value === undefined
    ? sliver
    : new RenderSliverPadding({ padding: padding(value), textDirection, child: sliver });
}

/** The cache area of a scroll view: its `cacheExtent` in pixels, else Flutter's default. */
function cacheExtentOf(props: Record<string, unknown>): ScrollCacheExtent | null {
  const cacheExtent = optionalNum(props, "cacheExtent");
  return cacheExtent === null ? null : ScrollCacheExtent.pixels(cacheExtent);
}

/** Viewport, ShrinkWrappingViewport, CustomScrollView, ListView, GridView, PageView, NestedScrollView, ReorderableListView. */
// biome-ignore lint/complexity/noExcessiveCognitiveComplexity: one case per scroll view node type; each is a few lines that mirror the widget's own build
function buildScrollable(node: SceneNode, textDirection: TextDirection, text: SceneText | null): RenderBox {
  const props = node.props;
  const explicitAxisDirection = field(props, "axisDirection") as AxisDirection | undefined;
  const axisDirection =
    node.type === "Viewport" || node.type === "ShrinkWrappingViewport"
      ? (explicitAxisDirection ?? AxisDirection.down)
      : scrollAxisDirection(props, textDirection);
  const crossAxisDirection = defaultCrossAxisDirection(axisDirection, textDirection);
  const scrollOffset = optionalNum(props, node.type.endsWith("Viewport") ? "offset" : "scrollOffset") ?? 0;
  let slivers: RenderSliver[];
  let offset: ViewportOffset = new FixedViewportOffset(scrollOffset);
  let scrollCacheExtent = cacheExtentOf(props);
  const boxes = (): PrebuiltChildManager => new PrebuiltChildManager(boxChildren(node, textDirection, text));
  switch (node.type) {
    case "Viewport":
    case "ShrinkWrappingViewport":
    case "CustomScrollView":
      slivers = sliverChildren(node, textDirection, text);
      break;
    case "ListView": {
      const itemExtent = optionalNum(props, "itemExtent");
      slivers = [
        paddedSliver(
          node,
          itemExtent === null
            ? new RenderSliverList({ childManager: boxes() })
            : new RenderSliverFixedExtentList({ childManager: boxes(), itemExtent }),
          textDirection,
        ),
      ];
      break;
    }
    case "GridView":
      slivers = [
        paddedSliver(
          node,
          new RenderSliverGrid({ childManager: boxes(), gridDelegate: gridDelegate(node) }),
          textDirection,
        ),
      ];
      break;
    case "PageView": {
      const viewportFraction = optionalNum(props, "viewportFraction") ?? 1;
      offset = new PageViewportOffset(optionalNum(props, "initialPage") ?? 0, viewportFraction);
      // PageView does not lay out beyond the viewport unless it allows implicit scrolling.
      scrollCacheExtent ??= ScrollCacheExtent.viewport(0);
      slivers = [
        new RenderSliverFractionalPadding({
          viewportFraction: field(props, "padEnds") === false ? 0 : Math.min(1, Math.max(0, 1 - viewportFraction)) / 2,
          child: new RenderSliverFillViewport({ childManager: boxes(), viewportFraction }),
        }),
      ];
      break;
    }
    case "NestedScrollView": {
      const parts = node.children ?? [];
      const body = parts[parts.length - 1];
      if (body === undefined) {
        throw new Error(`node "${node.id}": NestedScrollView needs a body as its last child`);
      }
      slivers = [
        ...sliverChildren({ ...node, children: parts.slice(0, -1) }, textDirection, text),
        new RenderSliverFillRemainingWithScrollable({ child: buildNode(body, textDirection, text) }),
      ];
      break;
    }
    case "ReorderableListView": {
      // ReorderableListView lays its items out as a SliverReorderableList (a list) inside a SliverPadding.
      const itemExtent = optionalNum(props, "itemExtent");
      slivers = [
        new RenderSliverPadding({
          padding: padding(field(props, "padding") ?? [0, 0, 0, 0]),
          textDirection,
          child:
            itemExtent === null
              ? new RenderSliverList({ childManager: boxes() })
              : new RenderSliverFixedExtentList({ childManager: boxes(), itemExtent }),
        }),
      ];
      break;
    }
    default:
      throw unsupported(node);
  }
  const options = {
    axisDirection,
    crossAxisDirection,
    offset,
    children: slivers,
    ...(scrollCacheExtent === null ? {} : { scrollCacheExtent }),
  };
  const viewport =
    node.type === "ShrinkWrappingViewport" || field(props, "shrinkWrap") === true
      ? new RenderShrinkWrappingViewport(options)
      : new RenderViewport({ ...options, anchor: optionalNum(props, "anchor") ?? 0 });
  viewport.debugId = node.id;
  return viewport;
}

/** What the Cupertino builders borrow from this file. */
function cupertinoContext(textDirection: TextDirection, text: SceneText | null, t: number): CupertinoContext {
  return {
    textDirection,
    text,
    build: (child, childText) => buildNode(child, textDirection, childText, t),
    padding: (value) => padding(value),
  };
}

/** Builds a Material composite; its slots inherit the composite's text style. */
function buildMaterialNode(
  node: SceneNode,
  textDirection: TextDirection,
  text: SceneText | null,
  t: number,
): RenderBox {
  const composite = buildMaterial(node, {
    direction: textDirection,
    text,
    build: (child, base, options) => buildNode(child, textDirection, inherited(text, base, options), t),
  });
  if (node.type !== "PreferredSize") {
    composite.debugId = node.id;
  }
  return composite;
}

const layer2Builders = new Map(layer2WidgetBuilders);

/** A scene node as the screen builder reads it; `sources` maps each widget back to its scene node. */
function widgetOf(node: SceneNode, sources: Map<WidgetNode, SceneNode>): WidgetNode {
  const slots = Object.entries(node.slots ?? {});
  const widget: WidgetNode = {
    id: node.id,
    type: node.type,
    props: node.props,
    ...(node.children === undefined ? {} : { children: node.children.map((child) => widgetOf(child, sources)) }),
    ...(slots.length === 0
      ? {}
      : { slots: Object.fromEntries(slots.map(([name, child]) => [name, widgetOf(child, sources)])) }),
  };
  sources.set(widget, node);
  return widget;
}

/** Builds a Layer 2 component with the screen builder, text laid out as the scene's engine and scale say. */
function buildLayer2(node: SceneNode, textDirection: TextDirection, text: SceneText | null): RenderBox {
  if (text === null) {
    throw new Error(`node "${node.id}": ${node.type} needs a ParagraphEngine`);
  }
  const builder = layer2Builders.get(node.type);
  if (builder === undefined) {
    throw unsupported(node);
  }
  const sources = new Map<WidgetNode, SceneNode>();
  const widget = widgetOf(node, sources);
  const width = text.screen?.width ?? defaultScreenTheme.width;
  const ctx: WidgetContext = {
    engine: text.engine,
    theme: { ...defaultScreenTheme, width, fontFamilies: ["NotoSans"] },
    textDirection,
    textScale: text.textScaler.textScaleFactor,
    build: (child) => {
      const source = sources.get(child);
      if (source === undefined) {
        throw new Error(`node "${node.id}": ${node.type} built a node that is not its own`);
      }
      return buildNode(source, textDirection, text);
    },
    available: () => width,
    paragraph: (value, paragraphStyle, options) =>
      new RenderParagraph({
        text: { text: value, style: paragraphStyle ?? {} },
        textDirection: options?.textDirection ?? textDirection,
        engine: text.engine,
        textScaler: text.textScaler,
      }),
    mark: () => undefined,
  };
  return builder(widget, ctx);
}

/** The page the runtime's golden writer lays a Layer 2 scene out in: a Scaffold body starting its children. */
function layer2Page(root: SceneNode): SceneNode {
  return {
    id: "__page",
    type: "Scaffold",
    props: {},
    children: [{ id: "__body", type: "Column", props: { crossAxisAlignment: "start" }, children: [root] }],
  };
}
/** The node types other builders own: hosts, Layer 2, scroll views, Cupertino and Material composites; null for the rest. */
function buildDelegated(
  node: SceneNode,
  textDirection: TextDirection,
  text: SceneText | null,
  t: number,
): RenderBox | null {
  if (node.type === hostType) {
    const host = text?.hosts?.get(node.id);
    if (host === undefined) {
      throw new Error(`node "${node.id}": no host built it`);
    }
    return host;
  }
  if (layer2Types.has(node.type)) {
    return buildLayer2(node, textDirection, text);
  }
  if (scrollTypes.has(node.type)) {
    return buildScrollable(node, textDirection, text);
  }
  if (cupertinoTypes.has(node.type)) {
    return buildCupertino(node, cupertinoContext(textDirection, text, t));
  }
  if (sliverTypes.has(node.type)) {
    throw new Error(`node "${node.id}": ${node.type} is a sliver; it belongs inside a viewport`);
  }
  return materialTypes.has(node.type) ? buildMaterialNode(node, textDirection, text, t) : null;
}

/**
 * Builds the render tree of `node` under `textDirection` (Flutter's ambient
 * `Directionality`). Throws on an unknown type and on a flex type, which
 * needs `RenderFlex`.
 */
export function buildNode(
  node: SceneNode,
  textDirection: TextDirection,
  text: SceneText | null = null,
  t = 1,
): RenderBox {
  const delegated = buildDelegated(node, textDirection, text, t);
  if (delegated !== null) {
    return delegated;
  }
  const children = childrenOf(node).map((child) => buildNode(child, textDirection, text, t));
  const child = singleChild(node, children);
  const props = node.props;
  let box: RenderBox;
  switch (node.type) {
    case "Row":
    case "Column":
    case "Flex":
      box = buildFlex(node, children, textDirection);
      break;
    case "Stack":
    case "IndexedStack":
      box = buildStack(node, children, textDirection);
      break;
    case "Wrap":
      box = buildWrap(node, children, textDirection);
      break;
    case "Positioned":
    case "PositionedDirectional":
      return applyPositioned(node, child, textDirection);
    case "Expanded":
    case "Flexible":
    case "Spacer":
      return applyFlexible(node, child);
    case "TableCell":
      return applyTableCell(node, child);
    case "AnimatedPositioned":
      return applyAnimatedPositioned(node, child, t);
    case "Table":
      box = buildTable(node, children, textDirection);
      break;
    case "Transform":
      box = buildTransform(node, textDirection, child);
      break;
    case "AnimatedContainer":
    case "AnimatedPadding":
    case "AnimatedAlign":
    case "AnimatedOpacity":
      box = buildAnimated(node, child, textDirection, t);
      break;
    case "SizeTransition":
    case "ScaleTransition":
    case "SlideTransition":
    case "FadeTransition":
      box = buildTransition(node, child, textDirection, t);
      break;
    case "Text":
      box = buildText(node, textDirection, text);
      break;
    case "TestBox":
      // Flutter's side is a childless SizedBox, i.e. a RenderConstrainedBox.
      box = new RenderConstrainedBox({
        additionalConstraints: BoxConstraints.tightFor({
          width: num(field(props, "width"), "width"),
          height: num(field(props, "height"), "height"),
        }),
      });
      break;
    case "SizedBox":
      box = new RenderConstrainedBox({
        additionalConstraints: BoxConstraints.tightFor({
          width: optionalNum(props, "width"),
          height: optionalNum(props, "height"),
        }),
        child,
      });
      break;
    case "ConstrainedBox":
      box = new RenderConstrainedBox({
        additionalConstraints: new BoxConstraints({
          minWidth: optionalNum(props, "minWidth") ?? 0,
          maxWidth: optionalNum(props, "maxWidth") ?? Number.POSITIVE_INFINITY,
          minHeight: optionalNum(props, "minHeight") ?? 0,
          maxHeight: optionalNum(props, "maxHeight") ?? Number.POSITIVE_INFINITY,
        }),
        child,
      });
      break;
    case "Padding":
      box = new RenderPadding({ padding: padding(field(props, "padding")), textDirection, child });
      break;
    case "Align":
    case "Center":
      box = new RenderPositionedBox({
        alignment: node.type === "Center" ? Alignment.center : alignment(field(props, "alignment")),
        textDirection,
        widthFactor: optionalNum(props, "widthFactor"),
        heightFactor: optionalNum(props, "heightFactor"),
        child,
      });
      break;
    case "DecoratedBox":
      box = new RenderDecoratedBox({ decoration: { color: field(props, "color") }, child });
      break;
    default: {
      const other = buildOtherBox(node, child, textDirection);
      if (other === null) {
        throw unsupported(node);
      }
      box = other;
    }
  }
  box.debugId = node.id;
  return box;
}

/** The text context below a composite that sets a base style, `DefaultTextStyle` flags or a scale cap. */
function inherited(
  text: SceneText | null,
  base: TextStyle | undefined,
  options: { defaults?: TextDefaults; maxScale?: number } | undefined,
): SceneText | null {
  if (text === null || (base === undefined && options === undefined)) {
    return text;
  }
  const next: SceneText = { ...text };
  if (base !== undefined) {
    next.base = base;
  }
  if (options?.defaults !== undefined) {
    next.defaults = options.defaults;
  }
  if (options?.maxScale !== undefined) {
    next.textScaler = TextScaler.linear(Math.min(text.textScaler.textScaleFactor, options.maxScale));
  }
  return next;
}

/** Lays `scene` out in the golden's frame and direction, as the Flutter writer did. */
export function layOutScene(scene: Scene, golden: LayoutGolden, engine: ParagraphEngine | null = null): RenderView {
  const direction = golden.locale === "ar" ? TextDirection.rtl : TextDirection.ltr;
  const owner = new PipelineOwner();
  const view = new RenderView({
    size: new Size(golden.frame.width, golden.frame.height),
    child: buildNode(
      scene.layer2 === true ? layer2Page(scene.root) : scene.root,
      direction,
      engine === null
        ? null
        : {
            engine,
            textScaler: TextScaler.linear(golden.textScale),
            // The golden writer's Material scope (a Layer 2 page's Scaffold too) gives every Text `bodyMedium`, as Flutter's root `Material` does.
            ...(scene.layer2 === true || [...sceneTypes(scene.root)].some((type) => materialTypes.has(type))
              ? { base: style("bodyMedium") }
              : {}),
            devicePixelRatio: golden.frame.devicePixelRatio,
            screen: { width: golden.frame.width, height: golden.frame.height },
          },
      scene.t ?? 1,
    ),
  });
  owner.rootNode = view;
  view.prepareInitialFrame();
  owner.flushLayout();
  return view;
}

/**
 * The rects of a laid-out `scene` as Flutter's golden writer records them.
 * `Expanded`, `Flexible`, `Positioned` and `PositionedDirectional` are parent-data widgets without a render object of
 * their own, so Flutter reports their id with the rect of their child.
 */
export function sceneGoldenRects(scene: Scene, view: RenderView): ReturnType<typeof layout.goldenRects> {
  const rects = layout.goldenRects(view);
  for (const id of Object.keys(rects)) {
    if (id.startsWith("__")) {
      delete rects[id];
    }
  }
  const alias = (node: SceneNode): void => {
    const child = node.children?.[0];
    if (
      ([
        "Expanded",
        "Flexible",
        "Positioned",
        "PositionedDirectional",
        "TableCell",
        "AnimatedPositioned",
        "PreferredSize",
      ].includes(node.type) ||
        cupertinoAliasTypes.has(node.type)) &&
      child !== undefined
    ) {
      const rect = rects[child.id];
      if (rect !== undefined) {
        rects[node.id] = rect;
      }
    }
    for (const c of childrenOf(node)) {
      alias(c);
    }
  };
  alias(scene.root);
  return rects;
}

/** The alphabetic baseline of every Text node in the golden's `baselines` shape. */
export function sceneGoldenBaselines(view: RenderView): Record<string, number> {
  return layout.goldenBaselines(view);
}
