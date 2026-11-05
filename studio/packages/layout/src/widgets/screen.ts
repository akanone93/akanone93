// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Screen documents to render trees (STU-005, STU-006, ADR-0013): each widget
 * type maps to a builder of Flutter-faithful render objects, text is laid out
 * by the paragraph engine, and a type without a builder becomes a labelled
 * placeholder box, never an error. Pure: no DOM, no CanvasKit beyond the
 * engine passed in. Builders are keyed by type so later slices add widgets by
 * extending the registry.
 */

import { BoxConstraints } from "../flutter/constraints.ts";
import { Axis, CrossAxisAlignment, MainAxisSize, RenderFlex } from "../flutter/flex.ts";
import { Alignment, EdgeInsets, Size, TextDirection } from "../flutter/geometry.ts";
import { type ButtonKind, buildButton, buildFab, type FabKind } from "../flutter/material/buttons.ts";
import { buildChip } from "../flutter/material/chip.ts";
import { buildInputDecorator, RenderEditableLine } from "../flutter/material/input-decorator.ts";
import { buildListTile } from "../flutter/material/list-tile.ts";
import { MaterialTapTargetSize, m3TextStyle, type TextRole, VisualDensity } from "../flutter/material/theme.ts";
import { PipelineOwner } from "../flutter/object.ts";
import { RenderParagraph, TextOverflow } from "../flutter/paragraph.ts";
import { RenderConstrainedBox } from "../flutter/proxy-box.ts";
import type { RenderBox } from "../flutter/render-box.ts";
import { RenderPadding, RenderPositionedBox } from "../flutter/shifted-box.ts";
import { TextScaler, type TextSpan, type TextStyle } from "../flutter/text-style.ts";
import { RenderView } from "../flutter/view.ts";
import { globalRects } from "../node-rects.ts";
import type { NodePaint, PaintTheme } from "../paint/paint.ts";
import { type PaintNode, paintTree } from "../paint/painters.ts";
import { type ResolvedText, resolveTextStyle } from "../paint/text-style.ts";
import type { ViewPadding } from "../scene/scene.ts";
import type { ParagraphEngine } from "../text/paragraph-engine.ts";
import { FontStyle, TextAlign, TextLeadingDistribution } from "../text/paragraph-engine.ts";
import { cupertinoWidgetBuilders } from "./cupertino.ts";
import { layer2Types, layer2WidgetBuilders } from "./layer2.ts";
import { materialWidgetBuilders } from "./material.ts";
import { buildPageRoute, ROUTE_TYPE, routeParts } from "./page-route.ts";
import { SCROLLING as BODY_SCROLLS, buildPageScaffold, pageBody, type ScrollExtent } from "./page-scaffold.ts";
import { cupertinoComponents, faithful, sceneWidgetBuilders } from "./scene-widgets.ts";
import { structuralWidgetBuilders } from "./structural.ts";

/** One widget of a screen document: structured-clone data only. */
export interface WidgetNode {
  readonly id: string;
  readonly type?: string | undefined;
  readonly props?: { readonly data?: unknown; readonly [name: string]: unknown } | undefined;
  readonly events?: unknown;
  readonly children?: readonly WidgetNode[] | undefined;
  readonly slots?: Readonly<Record<string, WidgetNode>> | undefined;
}

/** The look of laid-out screens: the device, the text and the fixed metrics. */
export interface ScreenTheme {
  /** The device width, in logical pixels. */
  readonly width: number;
  /** Families in fallback order: a glyph missing from the first comes from the next. */
  readonly fontFamilies: readonly string[];
  readonly fontSize: number;
  readonly pagePadding: number;
  readonly appBarHeight: number;
  /** The height a widget that needs a bounded height (a page view, a drawer) is drawn at. */
  readonly viewportHeight: number;
  readonly controlHeight: number;
  readonly spacing: number;
}

/** The default theme: a 390-pt phone, 16-pt Roboto with Noto Sans fallback. */
export const defaultScreenTheme: ScreenTheme = {
  width: 390,
  fontFamilies: ["Roboto", "Noto Sans"],
  fontSize: 16,
  pagePadding: 16,
  appBarHeight: 56,
  viewportHeight: 640,
  controlHeight: 48,
  spacing: 8,
};

/** What a builder may set on a paragraph besides its style. */
export interface ParagraphOptions {
  readonly textDirection?: TextDirection;
  readonly maxLines?: number;
  readonly textAlign?: TextAlign;
  readonly overflow?: TextOverflow;
  readonly softWrap?: boolean;
  /** The styled runs of a rich text, in place of the plain text (`RichText`, `Text.rich`). */
  readonly spans?: readonly TextSpan[];
}

/** One run of a rich text as it is drawn: its text, its resolved style and its colour (STU-006). */
export interface PlacedRun extends ResolvedText {
  readonly text: string;
}

/** How a laid-out text is drawn: its style and its paragraph settings, as it was measured (STU-006). */
export interface PlacedText {
  readonly size: number;
  readonly weight: number;
  readonly height: number;
  readonly letterSpacing: number;
  readonly italic: boolean;
  readonly family?: string;
  readonly decoration?: "underline" | "lineThrough" | "overline";
  readonly align?: "center" | "end" | "justify";
  readonly maxLines?: number;
  readonly overflow?: "clip" | "ellipsis" | "fade" | "visible";
  /** A rich text's runs, each in its own style. */
  readonly spans?: readonly PlacedRun[];
}

/** What a builder needs: the engine, the theme, and the labels it records. */
export interface WidgetContext {
  readonly engine: ParagraphEngine;
  readonly theme: ScreenTheme;
  /** Builds a child of the widget being built; `width` narrows the width available to it. */
  build(node: WidgetNode, width?: number): RenderBox;
  /** The width available to the widget being built. */
  available(): number;
  /** A paragraph of `text` in the theme's style, owned by the layout. */
  paragraph(text: string, style?: TextStyle, options?: ParagraphOptions): RenderParagraph;
  /** The ambient text scale factor (default 1). */
  readonly textScale?: number;
  /** The ambient text direction (default left to right). */
  readonly textDirection?: TextDirection;
  /** The height of the device frame the screen is drawn in (`MediaQuery` height); absent when none is known. */
  readonly deviceHeight?: number | undefined;
  /** Records how the box with `id` is drawn; a text records the style it is measured and drawn in. */
  mark(id: string, kind: PlacedKind, text?: string, style?: PlacedText): void;
  /** The colours and type scale to paint with, when the screen is painted (STU-005). */
  readonly paint?: PaintTheme | undefined;
  /**
   * `MediaQuery.padding` where the widget sits (PC-11): the device's status bar, cutouts and home indicator. The page's
   * app bar takes the top; below an app bar, the scaffold's body has none at the top, as Flutter's `Scaffold` removes it.
   */
  readonly viewPadding?: ViewPadding | undefined;
  /** The screen's root (whose `appBar` the screen lays out) and the status bar height its app bar reaches under. */
  readonly pageBar?: { readonly root: string; readonly top: number } | undefined;
  /** Builds with `padding` as the view padding, as a `MediaQuery` below a scaffold's part gives it (PC-12). */
  withViewPadding?<T>(padding: ViewPadding | undefined, build: () => T): T;
  /** The id of the page's own `Scaffold` (the painted screen's root): laid out at the device's size, parts in place. */
  readonly pageScaffold?: string | undefined;
  /**
   * On a live screen (PC-15), where the screen's scroll views report how far they scroll; their content is clipped to
   * the device instead of running on into the overflow region. Absent on a design screen.
   */
  readonly live?: ScrollExtent[] | undefined;
}

/** Builds the render object of one widget type. */
export type WidgetBuilder = (node: WidgetNode, ctx: WidgetContext) => RenderBox;

/** How a laid-out box is drawn: a box, a paragraph, or a placeholder for an unknown type. */
export type PlacedKind = "box" | "text" | "placeholder";

/** One laid-out box of a screen, relative to the screen's top-left. */
export interface Placed {
  readonly id: string;
  readonly kind: PlacedKind;
  readonly text?: string;
  readonly x: number;
  readonly y: number;
  readonly w: number;
  readonly h: number;
  /** How the box is painted, when the screen was laid out with a paint theme (`LayoutScreenOptions.paint`). */
  readonly paint?: NodePaint;
  /** The style a text was measured in, when the screen was laid out with a paint theme. */
  readonly textStyle?: PlacedText;
}

/** A laid-out screen. */
export interface ScreenLayout {
  readonly boxes: readonly Placed[];
  /** Height of the content; beyond the device height the screen overflows (STU-056). */
  readonly contentHeight: number;
  /** On a live screen, each scroll view's node and how far it scrolls (PC-15). */
  readonly scrolls?: readonly { readonly node: string; readonly max: number }[];
}

/** Slots that are drawn by the screen itself, not by the widget tree. */
const SKIPPED_SLOTS = new Set(["appBar"]);
/** Tall enough for any screen; the content is measured, not clipped to it. */
const MEASURE_HEIGHT = 100_000;

function named(id: string, box: RenderBox): RenderBox {
  box.debugId = id;
  return box;
}

function nested(node: WidgetNode): WidgetNode[] {
  return [
    ...(node.children ?? []),
    ...Object.entries(node.slots ?? {})
      .filter(([name]) => !SKIPPED_SLOTS.has(name))
      .map(([, child]) => child),
  ];
}

function textOf(node: WidgetNode): string | undefined {
  if (node.type === "Text" && typeof node.props?.data === "string") {
    return node.props.data;
  }
  for (const child of nested(node)) {
    const t = textOf(child);
    if (t !== undefined) {
      return t;
    }
  }
  return undefined;
}

function flex(direction: Axis, node: WidgetNode, ctx: WidgetContext): RenderBox {
  const kids = nested(node);
  return named(
    node.id,
    new RenderFlex({
      direction,
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.start,
      textDirection: ctx.textDirection ?? TextDirection.ltr,
      spacing: ctx.theme.spacing,
      children: kids.map((c) => ctx.build(c)),
    }),
  );
}

/** Shares a row's width among its children, so text in a row wraps instead of overflowing. */
const row: WidgetBuilder = (node, ctx) => {
  const kids = nested(node);
  const gaps = ctx.theme.spacing * Math.max(0, kids.length - 1);
  const share = Math.max(0, (ctx.available() - gaps) / Math.max(1, kids.length));
  return named(
    node.id,
    new RenderFlex({
      direction: Axis.horizontal,
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.start,
      textDirection: ctx.textDirection ?? TextDirection.ltr,
      spacing: ctx.theme.spacing,
      children: kids.map(
        (c) =>
          new RenderConstrainedBox({
            additionalConstraints: new BoxConstraints({ maxWidth: share }),
            child: ctx.build(c, share),
          }),
      ),
    }),
  );
};

/** A box of the control height with `label` centred in it, as wide as the label plus padding. */
function labelled(node: WidgetNode, ctx: WidgetContext, label: string | undefined, kind: PlacedKind): RenderBox {
  const { controlHeight, pagePadding } = ctx.theme;
  ctx.mark(node.id, kind, label);
  const inner =
    label === undefined
      ? new RenderConstrainedBox({ additionalConstraints: BoxConstraints.tightFor({ width: 120 }) })
      : new RenderPadding({ padding: EdgeInsets.symmetric({ horizontal: pagePadding }), child: ctx.paragraph(label) });
  return named(
    node.id,
    new RenderConstrainedBox({
      additionalConstraints: BoxConstraints.tightFor({ height: controlHeight }),
      child: new RenderPositionedBox({ alignment: Alignment.center, widthFactor: 1, child: inner }),
    }),
  );
}

/** A control: drawn as a box, labelled with its first text. */
const control: WidgetBuilder = (node, ctx) => labelled(node, ctx, textOf(node), "box");

/** A widget drawn through its content: its only child, or its children stacked vertically. */
const wrapper: WidgetBuilder = (node, ctx) => {
  const kids = nested(node);
  const only = kids[0];
  if (kids.length === 1 && only !== undefined && (node.children?.length ?? 0) === 0) {
    return ctx.build(only);
  }
  return kids.length === 0
    ? named(node.id, new RenderConstrainedBox({ additionalConstraints: new BoxConstraints() }))
    : flex(Axis.vertical, node, ctx);
};

/** Scroll views: a screen shows all of their content, so they are laid out without a bounded height. */
const SCROLLING: ReadonlySet<string> = new Set([
  "SingleChildScrollView",
  "ListView",
  "CustomScrollView",
  "GridView",
  "NestedScrollView",
  "ReorderableListView",
]);

/**
 * The constraints of a painted screen's root or a scaffold's body, as a route and `Scaffold` give them: the screen's
 * width, and at most `height` unless it scrolls (a screen shows a scroll view's content in full, past the frame).
 */
function bodyConstraints(node: WidgetNode | undefined, width: number, height: number): BoxConstraints {
  return node === undefined || SCROLLING.has(node.type ?? "") || node.type === "Scaffold"
    ? BoxConstraints.tightFor({ width })
    : new BoxConstraints({ minWidth: width, maxWidth: width, maxHeight: height });
}

/**
 * The page's app bar: on a painted screen the scaffold's own `AppBar`, laid out and painted as Flutter's; elsewhere a
 * stand-in of the theme's app bar height that shows the screen's title.
 */
function appBarOf(
  root: WidgetNode | null | undefined,
  ctx: WidgetContext,
  theme: ScreenTheme,
  painted: boolean,
): RenderBox {
  const bar = root?.slots?.["appBar"];
  if (painted && bar !== undefined && !Array.isArray(bar)) {
    return new RenderConstrainedBox({
      additionalConstraints: BoxConstraints.tightFor({ width: theme.width }),
      child: ctx.build(bar as WidgetNode),
    });
  }
  return named(
    "appBar",
    new RenderConstrainedBox({
      additionalConstraints: BoxConstraints.tightFor({ width: theme.width, height: theme.appBarHeight }),
    }),
  );
}

/** The root's constraints: as a route gives them on a painted screen, else the width inside the page padding. */
function rootConstraints(
  root: WidgetNode | undefined,
  theme: ScreenTheme,
  options: LayoutScreenOptions,
): BoxConstraints {
  return options.paint === undefined
    ? new BoxConstraints({ maxWidth: theme.width - 2 * theme.pagePadding })
    : bodyConstraints(root, theme.width, options.deviceHeight ?? theme.viewportHeight);
}

/** The status bar the page's app bar reaches under, for the screen's root scaffold; 0 for any other. */
function topInset(node: WidgetNode, ctx: WidgetContext): number {
  return ctx.pageBar?.root === node.id ? ctx.pageBar.top : 0;
}

/**
 * `Scaffold`: on a painted screen its body takes the device height below the app bar, as Flutter's scaffold gives it;
 * its other parts follow the body until they are placed. Elsewhere, its parts in a column.
 */
const scaffold: WidgetBuilder = (node, ctx) => {
  if (ctx.paint === undefined) {
    return wrapper(node, ctx);
  }
  if (ctx.withViewPadding !== undefined && ctx.pageScaffold === node.id) {
    return buildPageScaffold(node, ctx, named);
  }
  const body = node.slots?.["body"];
  const rest = nested(node).filter((c) => c !== body);
  // The page's app bar is as high as the toolbar plus the status bar it reaches under (PC-11).
  const bar = node.slots?.["appBar"] === undefined ? 0 : ctx.theme.appBarHeight + topInset(node, ctx);
  const height = (ctx.deviceHeight ?? ctx.theme.viewportHeight) - bar;
  return named(
    node.id,
    new RenderFlex({
      direction: Axis.vertical,
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.start,
      textDirection: ctx.textDirection ?? TextDirection.ltr,
      children: [
        new RenderConstrainedBox({
          additionalConstraints: bodyConstraints(body, ctx.available(), height),
          child: body === undefined ? null : ctx.build(body),
        }),
        ...rest.map((c) => ctx.build(c)),
      ],
    }),
  );
};

function prop(node: WidgetNode, name: string): unknown {
  return (node.props as Record<string, unknown> | undefined)?.[name];
}

/** The theme's Material 3 text style for `role`, in the screen theme's font families. */
function material(ctx: WidgetContext, role: TextRole): TextStyle {
  const [family, ...fallback] = ctx.theme.fontFamilies;
  return { ...m3TextStyle(role, family), fontFamilyFallback: fallback };
}

const tapTarget = (node: WidgetNode): MaterialTapTargetSize =>
  prop(node, "tapTargetSize") === "shrinkWrap" ? MaterialTapTargetSize.shrinkWrap : MaterialTapTargetSize.padded;

/** A Material 3 button of `kind`, sized as `ButtonStyleButton` sizes it (48 px tap target, scaled padding). */
function materialButton(kind: ButtonKind | "icon"): WidgetBuilder {
  return (node, ctx) => {
    const label = textOf(node);
    ctx.mark(node.id, "box", label);
    const content =
      kind === "icon"
        ? new RenderConstrainedBox({ additionalConstraints: BoxConstraints.tightFor({ width: 24, height: 24 }) })
        : ctx.paragraph(label ?? "", material(ctx, "labelLarge"));
    return named(
      node.id,
      buildButton({
        kind,
        label: content,
        tapTargetSize: tapTarget(node),
        density: VisualDensity.named(prop(node, "density")),
        textDirection: ctx.textDirection ?? TextDirection.ltr,
        textScale: ctx.textScale ?? 1,
      }),
    );
  };
}

const fabKinds: ReadonlySet<unknown> = new Set<FabKind>(["regular", "small", "large", "extended"]);

/** The FAB kind a node names; an unknown value is a regular FAB. */
function fabKind(value: unknown): FabKind {
  return fabKinds.has(value) ? (value as FabKind) : "regular";
}

const floatingActionButton: WidgetBuilder = (node, ctx) => {
  ctx.mark(node.id, "box", textOf(node));
  return named(
    node.id,
    buildFab({
      kind: fabKind(prop(node, "kind")),
      child: new RenderConstrainedBox({ additionalConstraints: BoxConstraints.tightFor({ width: 24, height: 24 }) }),
      tapTargetSize: tapTarget(node),
      textDirection: ctx.textDirection ?? TextDirection.ltr,
    }),
  );
};

/** A `ListTile` whose title, subtitle, leading and trailing are the widgets in the slots of that name. */
const listTile: WidgetBuilder = (node, ctx) => {
  const part = (name: string, role: TextRole): RenderBox | null => {
    const slot = node.slots?.[name];
    const text = slot === undefined ? undefined : textOf(slot);
    return text === undefined ? null : ctx.paragraph(text, material(ctx, role));
  };
  const dense = prop(node, "dense") === true;
  ctx.mark(node.id, "box", textOf(node));
  return named(
    node.id,
    buildListTile({
      leading: part("leading", "labelSmall"),
      title: part("title", "bodyLarge") ?? ctx.paragraph("", material(ctx, "bodyLarge")),
      subtitle: part("subtitle", "bodyMedium"),
      trailing: part("trailing", "labelSmall"),
      isDense: dense,
      isThreeLine: prop(node, "isThreeLine") === true,
      visualDensity: VisualDensity.named(prop(node, "density")),
      textDirection: ctx.textDirection ?? TextDirection.ltr,
    }),
  );
};

/** `Chip`, `FilterChip` and `InputChip`: the `label` slot's text inside the chip's padding and tap target. */
const chip: WidgetBuilder = (node, ctx) => {
  const labelSlot = "label";
  const slot = node.slots?.[labelSlot];
  ctx.mark(node.id, "box", slot === undefined ? undefined : textOf(slot));
  return named(
    node.id,
    buildChip({
      label: ctx.paragraph(slot === undefined ? "" : (textOf(slot) ?? ""), material(ctx, "labelLarge")),
      avatar: null,
      deleteIcon: null,
      selected: prop(node, "selected") === true,
      tapTargetSize: tapTarget(node),
      density: VisualDensity.named(prop(node, "density")),
      textScale: ctx.textScale ?? 1,
      textDirection: ctx.textDirection ?? TextDirection.ltr,
    }),
  );
};

/** A `TextField` under its `InputDecoration`: one line of `bodyLarge`, an optional helper or error text. */
const textField: WidgetBuilder = (node, ctx) => {
  const str = (name: string): string | null => {
    const value = prop(node, name);
    return typeof value === "string" ? value : null;
  };
  const subtext = str("errorText") ?? str("helperText");
  ctx.mark(node.id, "box", str("labelText") ?? undefined);
  return named(
    node.id,
    buildInputDecorator({
      input: new RenderEditableLine(ctx.paragraph("A", material(ctx, "bodyLarge"))),
      prefixIcon: null,
      suffixIcon: null,
      subtext: subtext === null ? null : ctx.paragraph(subtext, material(ctx, "bodySmall")),
      outline: prop(node, "border") === "outline",
      filled: prop(node, "filled") === true,
      isDense: prop(node, "isDense") === true,
      hasLabel: str("labelText") !== null,
      floating: (str("text") ?? "") !== "",
      floatingLabelHeight: 16,
      density: VisualDensity.named(prop(node, "density")),
      textDirection: ctx.textDirection ?? TextDirection.ltr,
    }),
  );
};

const ALIGNS: Readonly<Record<string, TextAlign>> = {
  center: TextAlign.center,
  end: TextAlign.end,
  right: TextAlign.right,
  left: TextAlign.left,
  justify: TextAlign.justify,
};
const OVERFLOWS: Readonly<Record<string, TextOverflow>> = {
  clip: TextOverflow.clip,
  ellipsis: TextOverflow.ellipsis,
  fade: TextOverflow.fade,
  visible: TextOverflow.visible,
};

/** A layout text style from a resolved one: its family first, then the theme's families as fallbacks. */
export function layoutTextStyle(t: ResolvedText, theme: ScreenTheme): TextStyle {
  const [family, ...fallback] = theme.fontFamilies;
  return {
    fontFamily: t.family ?? family,
    fontFamilyFallback: t.family === undefined ? fallback : theme.fontFamilies,
    fontSize: t.size,
    fontWeight: t.weight,
    ...(t.italic ? { fontStyle: FontStyle.italic } : {}),
    letterSpacing: t.letterSpacing,
    height: t.height,
    // Material 3's text styles spread the leading evenly above and below the line.
    leadingDistribution: TextLeadingDistribution.even,
  };
}

interface TextProps {
  readonly maxLines?: number;
  readonly align?: string;
  readonly overflow?: string;
  readonly softWrap: boolean;
}

function textPropsOf(props: Record<string, unknown>): TextProps {
  const maxLines = props["maxLines"];
  const align = props["textAlign"];
  const overflow = props["overflow"];
  return {
    ...(typeof maxLines === "number" && maxLines > 0 ? { maxLines } : {}),
    ...(typeof align === "string" ? { align } : {}),
    ...(typeof overflow === "string" ? { overflow } : {}),
    softWrap: props["softWrap"] !== false,
  };
}

/** How the canvas aligns a text: left and right are start and end in a left-to-right preview. */
const DRAW_ALIGNS: Readonly<Record<string, PlacedText["align"]>> = {
  center: "center",
  justify: "justify",
  end: "end",
  right: "end",
};
const DRAW_OVERFLOWS = new Set(["clip", "ellipsis", "fade", "visible"]);

function placedTextOf(t: ResolvedText, p: TextProps): PlacedText {
  const align = p.align === undefined ? undefined : DRAW_ALIGNS[p.align];
  return {
    size: t.size,
    weight: t.weight,
    height: t.height,
    letterSpacing: t.letterSpacing,
    italic: t.italic,
    ...(t.family === undefined ? {} : { family: t.family }),
    ...(t.decoration === undefined ? {} : { decoration: t.decoration }),
    ...(align === undefined ? {} : { align }),
    ...(p.maxLines === undefined ? {} : { maxLines: p.maxLines }),
    ...(p.overflow !== undefined && DRAW_OVERFLOWS.has(p.overflow)
      ? { overflow: p.overflow as NonNullable<PlacedText["overflow"]> }
      : {}),
  };
}

function paragraphOptionsOf(p: TextProps): ParagraphOptions {
  const align = p.align === undefined ? undefined : ALIGNS[p.align];
  const overflow = p.overflow === undefined ? undefined : OVERFLOWS[p.overflow];
  return {
    ...(p.maxLines === undefined ? {} : { maxLines: p.maxLines }),
    ...(align === undefined ? {} : { textAlign: align }),
    ...(overflow === undefined ? {} : { overflow }),
    ...(p.softWrap ? {} : { softWrap: false }),
  };
}

/** Builds `node` with its type's builder; an untyped node by its structure, an unknown type as a placeholder. */
function buildWith(builders: ReadonlyMap<string, WidgetBuilder>, node: WidgetNode, ctx: WidgetContext): RenderBox {
  const builder = node.type === undefined ? undefined : builders.get(node.type);
  if (builder !== undefined) {
    return builder(node, ctx);
  }
  // An untyped node is drawn by its structure: a control when it handles events, else its content.
  if (node.type === undefined) {
    return node.events === undefined ? wrapper(node, ctx) : control(node, ctx);
  }
  return labelled(node, ctx, node.type, "placeholder");
}

/** A paragraph of `text` in `style`, with the screen's direction and scale and the builder's options. */
function paragraphOf(
  text: string,
  style: TextStyle,
  o: ParagraphOptions | undefined,
  screen: LayoutScreenOptions,
  textScale: number,
): RenderParagraph {
  return new RenderParagraph({
    text: o?.spans === undefined ? { text, style } : { style, children: o.spans },
    textDirection: o?.textDirection ?? screen.textDirection ?? TextDirection.ltr,
    textScaler: textScale === 1 ? undefined : TextScaler.linear(textScale),
    engine: screen.engine,
    ...(o?.maxLines === undefined ? {} : { maxLines: o.maxLines }),
    ...(o?.textAlign === undefined ? {} : { textAlign: o.textAlign }),
    ...(o?.overflow === undefined ? {} : { overflow: o.overflow }),
    ...(o?.softWrap === undefined ? {} : { softWrap: o.softWrap }),
  });
}

/** How a paragraph a component lays out itself is drawn: the style it was measured in. */
function placedOfStyle(s: TextStyle, options: ParagraphOptions | undefined): PlacedText {
  const size = s.fontSize ?? 14;
  return {
    size,
    weight: s.fontWeight ?? 400,
    height: s.height ?? 1.2,
    letterSpacing: s.letterSpacing ?? 0,
    italic: s.fontStyle === FontStyle.italic,
    ...(options?.maxLines === undefined ? {} : { maxLines: options.maxLines }),
  };
}

/** What a painted screen's `Text` is laid out with and drawn in: its resolved style and its paragraph settings. */
export function paintedTextOf(
  node: WidgetNode,
  paint: PaintTheme,
  theme: ScreenTheme,
  base?: ResolvedText,
): { placed: PlacedText; style: TextStyle; options: ParagraphOptions } {
  const props = (node.props ?? {}) as Record<string, unknown>;
  const t = resolveTextStyle(props["style"], paint, base);
  const p = textPropsOf(props);
  return { placed: placedTextOf(t, p), style: layoutTextStyle(t, theme), options: paragraphOptionsOf(p) };
}

/** The runs of a `TextSpan` document value, each span's style resolved over its parent's (`TextSpan.build`). */
function runsOf(span: unknown, paint: PaintTheme, inherited: ResolvedText, runs: PlacedRun[] = []): PlacedRun[] {
  if (typeof span !== "object" || span === null) {
    return runs;
  }
  const record = span as Record<string, unknown>;
  const style = resolveTextStyle(record["style"], paint, inherited);
  if (typeof record["text"] === "string" && record["text"] !== "") {
    runs.push({ ...style, text: record["text"] });
  }
  for (const child of Array.isArray(record["children"]) ? record["children"] : []) {
    runsOf(child, paint, style, runs);
  }
  return runs;
}

/** What a painted screen's `RichText` is laid out with and drawn in: a run per styled span. */
export function paintedRichTextOf(
  node: WidgetNode,
  paint: PaintTheme,
  theme: ScreenTheme,
): { text: string; placed: PlacedText; style: TextStyle; options: ParagraphOptions } {
  const props = (node.props ?? {}) as Record<string, unknown>;
  const base = resolveTextStyle(undefined, paint);
  const runs = runsOf(props["textSpan"], paint, base);
  const p = textPropsOf(props);
  return {
    text: runs.map((r) => r.text).join(""),
    placed: { ...placedTextOf(base, p), spans: runs },
    style: layoutTextStyle(base, theme),
    options: {
      ...paragraphOptionsOf(p),
      spans: runs.map((r) => ({ text: r.text, style: layoutTextStyle(r, theme) })),
    },
  };
}

/** The builders of the widget types the canvas draws; extend with new types. */
export const defaultWidgetBuilders: ReadonlyMap<string, WidgetBuilder> = new Map<string, WidgetBuilder>([
  [ROUTE_TYPE, (node, ctx) => buildPageRoute(node, ctx)],
  [
    "Text",
    (node, ctx) => {
      const data = typeof node.props?.data === "string" ? node.props.data : "";
      if (ctx.paint === undefined) {
        ctx.mark(node.id, "text", data);
        return named(node.id, ctx.paragraph(data));
      }
      const { placed, style, options } = paintedTextOf(node, ctx.paint, ctx.theme);
      ctx.mark(node.id, "text", data, placed);
      return named(node.id, ctx.paragraph(data, style, options));
    },
  ],
  ["Column", faithful((node, ctx) => flex(Axis.vertical, node, ctx))],
  ["Row", faithful(row)],
  ["ListView", faithful((node, ctx) => flex(Axis.vertical, node, ctx))],
  ["Scaffold", scaffold],
  ["Center", faithful(wrapper)],
  ["Padding", faithful(wrapper)],
  ["SizedBox", faithful(wrapper)],
  ["Container", faithful(wrapper)],
  ["ColoredBox", faithful(wrapper)],
  ["Align", faithful(wrapper)],
  ["SafeArea", faithful(wrapper)],
  ["Expanded", faithful(wrapper)],
  ["Flexible", faithful(wrapper)],
  ["SingleChildScrollView", faithful(wrapper)],
  ["FilledButton", faithful(materialButton("filled"))],
  ["FilledTonalButton", faithful(materialButton("filledTonal"))],
  ["ElevatedButton", faithful(materialButton("elevated"))],
  ["OutlinedButton", faithful(materialButton("outlined"))],
  ["TextButton", faithful(materialButton("text"))],
  ["IconButton", faithful(materialButton("icon"))],
  ["FloatingActionButton", faithful(floatingActionButton)],
  ["ListTile", faithful(listTile)],
  ["Chip", faithful(chip)],
  ["FilterChip", faithful(chip)],
  ["InputChip", faithful(chip)],
  ["TextField", faithful(textField)],
  ["TextFormField", faithful(control)],
  ["CupertinoPageScaffold", wrapper],
  ["CupertinoTabScaffold", wrapper],
  ["CupertinoListSection", (node, ctx) => flex(Axis.vertical, node, ctx)],
  ...materialWidgetBuilders,
  ...layer2WidgetBuilders,
  ...cupertinoWidgetBuilders,
  ...sceneWidgetBuilders,
  ...structuralWidgetBuilders,
  ["TabBar", faithful(legacyBuilder(materialWidgetBuilders, "TabBar"))],
  ...cupertinoComponents.map((type): [string, WidgetBuilder] => [
    type,
    faithful(legacyBuilder([...cupertinoWidgetBuilders], type)),
  ]),
]);

/** The builder `list` registers for `type`, as the screen's approximation under a faithful builder. */
function legacyBuilder(list: ReadonlyArray<readonly [string, WidgetBuilder]>, type: string): WidgetBuilder {
  return list.find(([t]) => t === type)?.[1] ?? wrapper;
}

/** Options of [layoutScreen]. */
export interface LayoutScreenOptions {
  readonly engine: ParagraphEngine;
  readonly theme?: ScreenTheme;
  /** The app bar title; the bar is drawn when the root has an `appBar` slot. */
  readonly title?: string;
  readonly builders?: ReadonlyMap<string, WidgetBuilder>;
  /** The ambient text scale factor of the preview (default 1): A11Y-005. */
  readonly textScale?: number;
  /** The ambient text direction, right to left for `ar` and `he` (default left to right): I18N-010. */
  readonly textDirection?: TextDirection;
  /** The height of the device frame the screen is drawn in; widgets sized by a share of it use it. */
  readonly deviceHeight?: number | undefined;
  /** The colours to paint with; without it the boxes carry no paint (STU-005). */
  readonly paint?: PaintTheme | undefined;
  /** The device's system areas (status bar, cutouts, home indicator) on a painted screen; none when absent (PC-11). */
  readonly viewPadding?: ViewPadding | undefined;
  /** A live screen (PC-15): scroll views scroll inside the device, by their `x-studio.scroll` offsets. */
  readonly live?: boolean | undefined;
}

/** The laid-out boxes of a screen, with how each is drawn and, when painted, its paint. */
function placedBoxes(
  rects: ReadonlyMap<string, { x: number; y: number; w: number; h: number }>,
  marks: ReadonlyMap<string, { kind: PlacedKind; text?: string; textStyle?: PlacedText }>,
  paints: ReadonlyMap<string, NodePaint> | undefined,
  rootId: string,
): Placed[] {
  const boxes: Placed[] = [];
  for (const [id, r] of rects) {
    if (id === rootId) {
      continue;
    }
    const mark = marks.get(id) ?? { kind: "box" as const };
    const paint = paints?.get(id);
    boxes.push({ id, ...mark, x: r.x, y: r.y, w: r.w, h: r.h, ...(paint === undefined ? {} : { paint }) });
  }
  return boxes;
}

/**
 * How high a screen's content reaches: its root's height, or on a painted screen the lowest box — a page scaffold is
 * as high as the device while its scroll view's content runs on below it (PC-12, STU-056).
 */
function contentHeightOf(
  rects: ReadonlyMap<string, { y: number; h: number }>,
  rootId: string,
  paints: ReadonlyMap<string, NodePaint> | undefined,
): number {
  let height = rects.get(rootId)?.h ?? 0;
  if (paints !== undefined) {
    for (const r of rects.values()) {
      height = Math.max(height, r.y + r.h);
    }
  }
  return height;
}

/**
 * The root's box: built as it is, or on a live screen a root scroll view in a viewport as high as the device (PC-15).
 */
function rootBox(root: WidgetNode, ctx: WidgetContext, theme: ScreenTheme): RenderBox {
  const built = ctx.build(root);
  if (ctx.live === undefined || !BODY_SCROLLS.has(root.type ?? "")) {
    return built;
  }
  return new RenderConstrainedBox({
    additionalConstraints: BoxConstraints.tightFor({
      width: ctx.available(),
      height: ctx.deviceHeight ?? theme.viewportHeight,
    }),
    child: pageBody(built, root, ctx),
  });
}

/** Where a painted, live screen's scroll views report (PC-15); none on a design screen. */
function liveOf(options: LayoutScreenOptions, sink: ScrollExtent[]): ScrollExtent[] | undefined {
  return options.live === true && options.paint !== undefined ? sink : undefined;
}

/** A laid-out screen; a live one is the device's view, nothing running past the frame, with its scroll extents. */
function screenResult(boxes: Placed[], height: number, ctx: WidgetContext, options: LayoutScreenOptions): ScreenLayout {
  return ctx.live === undefined
    ? { boxes, contentHeight: height }
    : { boxes, contentHeight: options.deviceHeight ?? height, scrolls: ctx.live };
}

/** The root's id when it is a `Scaffold` on a painted screen: the page's own scaffold (PC-12). */
function pageScaffoldOf(root: WidgetNode | null | undefined, options: LayoutScreenOptions): string | undefined {
  // Under a dialog or sheet page, the page that opens it is the device's page (PC-13).
  const page = root === null || root === undefined ? undefined : (routeParts(root)?.below ?? root);
  return options.paint !== undefined && page?.type === "Scaffold" ? page.id : undefined;
}

/**
 * The view padding of a painted screen (PC-11): the whole of it for the page's app bar, the body's (none at the top
 * below an app bar), and the root the app bar belongs to with the status bar it reaches under.
 */
function pageInsets(
  root: WidgetNode | null | undefined,
  options: LayoutScreenOptions,
): {
  readonly hasBar: boolean;
  readonly full?: ViewPadding;
  readonly body?: ViewPadding;
  readonly pageBar?: { root: string; top: number };
} {
  // A painted page's own Scaffold places its app bar itself (PC-12); elsewhere the screen lays the app bar out above.
  const hasBar = root?.slots?.["appBar"] !== undefined && pageScaffoldOf(root, options) === undefined;
  const full = options.paint === undefined ? undefined : options.viewPadding;
  if (full === undefined) {
    return { hasBar };
  }
  if (!hasBar || root === null || root === undefined) {
    return { hasBar, full, body: full };
  }
  return { hasBar, full, body: { ...full, top: 0 }, pageBar: { root: root.id, top: full.top } };
}

/** Lays one screen out at the theme's device width, text by the paragraph engine. */
export function layoutScreen(root: WidgetNode | null | undefined, options: LayoutScreenOptions): ScreenLayout {
  const theme = options.theme ?? defaultScreenTheme;
  const builders = options.builders ?? defaultWidgetBuilders;
  const marks = new Map<string, { kind: PlacedKind; text?: string; textStyle?: PlacedText }>();
  const paragraphs: RenderParagraph[] = [];
  const [family, ...fallback] = theme.fontFamilies;
  const style: TextStyle = { fontSize: theme.fontSize, fontFamily: family, fontFamilyFallback: fallback };
  // A painted screen is laid out as the device lays it out, edge to edge; otherwise with a design-time page padding.
  const pagePadding = options.paint === undefined ? theme.pagePadding : 0;
  const widths: number[] = [theme.width - 2 * pagePadding];
  const owners: { id: string; count: number }[] = [];
  const textScale = options.textScale ?? 1;
  const rootId = "root";
  const live: ScrollExtent[] = [];
  const insets = pageInsets(root, options);
  const hasBar = insets.hasBar;
  // The page's app bar is laid out under the whole view padding; the body below it has none at the top.
  let padding = insets.full;
  const ctx: WidgetContext = {
    get viewPadding() {
      return padding;
    },
    pageBar: insets.pageBar,
    pageScaffold: pageScaffoldOf(root, options),
    live: liveOf(options, live),
    withViewPadding(next, build) {
      const kept = padding;
      padding = next;
      try {
        return build();
      } finally {
        padding = kept;
      }
    },
    engine: options.engine,
    theme,
    textScale,
    textDirection: options.textDirection ?? TextDirection.ltr,
    deviceHeight: options.deviceHeight,
    build(node, width) {
      if (width !== undefined) {
        widths.push(width);
      }
      // On a painted screen the texts a Layer 2 component lays out itself are its parts, drawn where they sit.
      const owns = options.paint !== undefined && layer2Types.has(node.type ?? "");
      if (owns) {
        owners.push({ id: node.id, count: 0 });
      }
      try {
        return buildWith(builders, node, ctx);
      } finally {
        if (width !== undefined) {
          widths.pop();
        }
        if (owns) {
          owners.pop();
        }
      }
    },
    available: () => widths.at(-1) ?? theme.width,
    paragraph(text, override, paragraphOptions) {
      const p = paragraphOf(text, override ?? style, paragraphOptions, options, textScale);
      paragraphs.push(p);
      const owner = owners.at(-1);
      if (owner !== undefined) {
        p.debugId = `${owner.id}#p${owner.count++}`;
        marks.set(p.debugId, { kind: "text", text, textStyle: placedOfStyle(override ?? style, paragraphOptions) });
      }
      return p;
    },
    mark(id, kind, text, textStyle) {
      marks.set(id, {
        kind,
        ...(text === undefined ? {} : { text }),
        ...(textStyle === undefined ? {} : { textStyle }),
      });
    },
    ...(options.paint === undefined ? {} : { paint: options.paint }),
  };
  if (hasBar) {
    ctx.mark("appBar", "box", options.title);
  }
  const bar = hasBar ? [appBarOf(root, ctx, theme, options.paint !== undefined)] : [];
  padding = insets.body;
  const content = named(
    rootId,
    new RenderFlex({
      direction: Axis.vertical,
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.start,
      textDirection: ctx.textDirection ?? TextDirection.ltr,
      children: [
        ...bar,
        new RenderPadding({
          padding: EdgeInsets.all(pagePadding),
          child: new RenderConstrainedBox({
            additionalConstraints: rootConstraints(root ?? undefined, theme, options),
            child: root ? rootBox(root, ctx, theme) : null,
          }),
        }),
      ],
    }),
  );
  const view = new RenderView({
    size: new Size(theme.width, MEASURE_HEIGHT),
    child: new RenderPositionedBox({
      alignment: Alignment.topLeft,
      // As wide as the screen, so that a right-to-left page starts at its right edge.
      child: new RenderConstrainedBox({
        additionalConstraints: BoxConstraints.tightFor({ width: theme.width }),
        child: content,
      }),
    }),
  });
  const owner = new PipelineOwner();
  owner.rootNode = view;
  view.prepareInitialFrame();
  try {
    owner.flushLayout();
    const rects = globalRects(view);
    const paints =
      options.paint === undefined ? undefined : paintTree(root as PaintNode | null | undefined, rects, options.paint);
    return screenResult(
      placedBoxes(rects, marks, paints, rootId),
      contentHeightOf(rects, rootId, paints),
      ctx,
      options,
    );
  } finally {
    for (const p of paragraphs) {
      p.dispose();
    }
  }
}
