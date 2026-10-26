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
import { RenderParagraph } from "../flutter/paragraph.ts";
import { RenderConstrainedBox } from "../flutter/proxy-box.ts";
import type { RenderBox } from "../flutter/render-box.ts";
import { RenderPadding, RenderPositionedBox } from "../flutter/shifted-box.ts";
import type { TextStyle } from "../flutter/text-style.ts";
import { RenderView } from "../flutter/view.ts";
import { globalRects } from "../node-rects.ts";
import type { ParagraphEngine } from "../text/paragraph-engine.ts";
import { cupertinoWidgetBuilders } from "./cupertino.ts";
import { layer2WidgetBuilders } from "./layer2.ts";
import { materialWidgetBuilders } from "./material.ts";
import { sceneWidgetBuilders } from "./scene-widgets.ts";
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
  /** Records how the box with `id` is drawn. */
  mark(id: string, kind: PlacedKind, text?: string): void;
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
}

/** A laid-out screen. */
export interface ScreenLayout {
  readonly boxes: readonly Placed[];
  /** Height of the content; beyond the device height the screen overflows (STU-056). */
  readonly contentHeight: number;
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
      textDirection: TextDirection.ltr,
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
      textDirection: TextDirection.ltr,
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
        textDirection: TextDirection.ltr,
        textScale: 1,
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
      textDirection: TextDirection.ltr,
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
      textDirection: TextDirection.ltr,
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
      textScale: 1,
      textDirection: TextDirection.ltr,
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
      textDirection: TextDirection.ltr,
    }),
  );
};

/** The builders of the widget types the canvas draws; extend with new types. */
export const defaultWidgetBuilders: ReadonlyMap<string, WidgetBuilder> = new Map<string, WidgetBuilder>([
  [
    "Text",
    (node, ctx) => {
      const data = typeof node.props?.data === "string" ? node.props.data : "";
      ctx.mark(node.id, "text", data);
      return named(node.id, ctx.paragraph(data));
    },
  ],
  ["Column", (node, ctx) => flex(Axis.vertical, node, ctx)],
  ["Row", row],
  ["ListView", (node, ctx) => flex(Axis.vertical, node, ctx)],
  ["Scaffold", wrapper],
  ["Center", wrapper],
  ["Padding", wrapper],
  ["SizedBox", wrapper],
  ["Container", wrapper],
  ["ColoredBox", wrapper],
  ["Align", wrapper],
  ["SafeArea", wrapper],
  ["Expanded", wrapper],
  ["Flexible", wrapper],
  ["SingleChildScrollView", wrapper],
  ["FilledButton", materialButton("filled")],
  ["FilledTonalButton", materialButton("filledTonal")],
  ["ElevatedButton", materialButton("elevated")],
  ["OutlinedButton", materialButton("outlined")],
  ["TextButton", materialButton("text")],
  ["IconButton", materialButton("icon")],
  ["FloatingActionButton", floatingActionButton],
  ["ListTile", listTile],
  ["Chip", chip],
  ["FilterChip", chip],
  ["InputChip", chip],
  ["TextField", textField],
  ["TextFormField", control],
  ["CupertinoPageScaffold", wrapper],
  ["CupertinoTabScaffold", wrapper],
  ["CupertinoListSection", (node, ctx) => flex(Axis.vertical, node, ctx)],
  ...materialWidgetBuilders,
  ...layer2WidgetBuilders,
  ...cupertinoWidgetBuilders,
  ...sceneWidgetBuilders,
  ...structuralWidgetBuilders,
]);

/** Options of [layoutScreen]. */
export interface LayoutScreenOptions {
  readonly engine: ParagraphEngine;
  readonly theme?: ScreenTheme;
  /** The app bar title; the bar is drawn when the root has an `appBar` slot. */
  readonly title?: string;
  readonly builders?: ReadonlyMap<string, WidgetBuilder>;
}

/** Lays one screen out at the theme's device width, text by the paragraph engine. */
export function layoutScreen(root: WidgetNode | null | undefined, options: LayoutScreenOptions): ScreenLayout {
  const theme = options.theme ?? defaultScreenTheme;
  const builders = options.builders ?? defaultWidgetBuilders;
  const marks = new Map<string, { kind: PlacedKind; text?: string }>();
  const paragraphs: RenderParagraph[] = [];
  const [family, ...fallback] = theme.fontFamilies;
  const style: TextStyle = { fontSize: theme.fontSize, fontFamily: family, fontFamilyFallback: fallback };
  const widths: number[] = [theme.width - 2 * theme.pagePadding];
  const ctx: WidgetContext = {
    engine: options.engine,
    theme,
    build(node, width) {
      const builder = node.type === undefined ? undefined : builders.get(node.type);
      if (width !== undefined) {
        widths.push(width);
      }
      try {
        if (builder !== undefined) {
          return builder(node, ctx);
        }
        // An untyped node is drawn by its structure: a control when it handles events, else its content.
        if (node.type === undefined) {
          return node.events === undefined ? wrapper(node, ctx) : control(node, ctx);
        }
        return labelled(node, ctx, node.type, "placeholder");
      } finally {
        if (width !== undefined) {
          widths.pop();
        }
      }
    },
    available: () => widths.at(-1) ?? theme.width,
    paragraph(text, override, paragraphOptions) {
      const p = new RenderParagraph({
        text: { text, style: override ?? style },
        textDirection: paragraphOptions?.textDirection ?? TextDirection.ltr,
        engine: options.engine,
      });
      paragraphs.push(p);
      return p;
    },
    mark(id, kind, text) {
      marks.set(id, text === undefined ? { kind } : { kind, text });
    },
  };
  const rootId = "root";
  const hasBar = root?.slots?.["appBar"] !== undefined;
  if (hasBar) {
    ctx.mark("appBar", "box", options.title);
  }
  const content = named(
    rootId,
    new RenderFlex({
      direction: Axis.vertical,
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.start,
      textDirection: TextDirection.ltr,
      children: [
        ...(hasBar
          ? [
              named(
                "appBar",
                new RenderConstrainedBox({
                  additionalConstraints: BoxConstraints.tightFor({ width: theme.width, height: theme.appBarHeight }),
                }),
              ),
            ]
          : []),
        new RenderPadding({
          padding: EdgeInsets.all(theme.pagePadding),
          child: new RenderConstrainedBox({
            additionalConstraints: new BoxConstraints({ maxWidth: theme.width - 2 * theme.pagePadding }),
            child: root ? ctx.build(root) : null,
          }),
        }),
      ],
    }),
  );
  const view = new RenderView({
    size: new Size(theme.width, MEASURE_HEIGHT),
    child: new RenderPositionedBox({ alignment: Alignment.topLeft, child: content }),
  });
  const owner = new PipelineOwner();
  owner.rootNode = view;
  view.prepareInitialFrame();
  try {
    owner.flushLayout();
    const rects = globalRects(view);
    const boxes: Placed[] = [];
    for (const [id, r] of rects) {
      if (id === rootId) {
        continue;
      }
      const mark = marks.get(id) ?? { kind: "box" as const };
      boxes.push({ id, ...mark, x: r.x, y: r.y, w: r.w, h: r.h });
    }
    return { boxes, contentHeight: rects.get(rootId)?.h ?? 0 };
  } finally {
    for (const p of paragraphs) {
      p.dispose();
    }
  }
}
