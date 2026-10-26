// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Builders for the Layer 2 components of a screen document, batch A (STU-005,
 * S4.5, WGT-020). Each composes the render objects the runtime's builder in
 * `packages/plux_components` composes, with Material 3 text roles in the
 * screen theme's fonts; the layout conformance suite checks them against
 * goldens the runtime writes. Components whose content is platform media lay
 * out as their box only.
 */

import { BoxConstraints } from "../flutter/constraints.ts";
import { Axis, CrossAxisAlignment, FlexFit, FlexParentData, MainAxisSize, RenderFlex } from "../flutter/flex.ts";
import { EdgeInsets, EdgeInsetsDirectional, TextDirection } from "../flutter/geometry.ts";
import { buildButton } from "../flutter/material/buttons.ts";
import { buildChip } from "../flutter/material/chip.ts";
import { buildInputDecorator, RenderEditableLine } from "../flutter/material/input-decorator.ts";
import { MaterialTapTargetSize, m3TextStyle, type TextRole, VisualDensity } from "../flutter/material/theme.ts";
import { RenderParagraph } from "../flutter/paragraph.ts";
import { RenderConstrainedBox } from "../flutter/proxy-box.ts";
import type { RenderBox } from "../flutter/render-box.ts";
import { RenderPadding } from "../flutter/shifted-box.ts";
import { RenderSingleChildScroll } from "../flutter/single-child-scroll.ts";
import type { TextStyle } from "../flutter/text-style.ts";
import { formatMoney, moneyOf } from "./layer2.ts";
import type { WidgetBuilder, WidgetContext, WidgetNode } from "./screen.ts";

function text(node: WidgetNode, name: string): string | undefined {
  const value = node.props?.[name];
  return typeof value === "string" ? value : undefined;
}

function role(ctx: WidgetContext, name: TextRole): TextStyle {
  const [family, ...fallback] = ctx.theme.fontFamilies;
  return { ...m3TextStyle(name, family), fontFamilyFallback: fallback };
}

function direction(ctx: WidgetContext): TextDirection {
  return ctx.textDirection ?? TextDirection.ltr;
}

/** A paragraph in `name`'s role. */
function line(ctx: WidgetContext, value: string, name: TextRole, fixedLtr = false): RenderParagraph {
  return ctx.paragraph(value, role(ctx, name), fixedLtr ? { textDirection: TextDirection.ltr } : undefined);
}

/**
 * The ambient text scale as a button or chip reads it, `scaler.scale(14) / 14`: the width of a probe the context
 * lays out, over the width of the same probe unscaled.
 */
function textScaleOf(ctx: WidgetContext): number {
  const style: TextStyle = { ...role(ctx, "bodyMedium"), fontSize: 100 };
  const scaled = ctx.paragraph("M", style).getMaxIntrinsicWidth(Number.POSITIVE_INFINITY);
  const plain = new RenderParagraph({
    text: { text: "M", style },
    textDirection: direction(ctx),
    engine: ctx.engine,
  }).getMaxIntrinsicWidth(Number.POSITIVE_INFINITY);
  return plain === 0 ? 1 : scaled / plain;
}

function sized(width: number, height: number): RenderBox {
  return new RenderConstrainedBox({ additionalConstraints: BoxConstraints.tightFor({ width, height }) });
}

function named(node: WidgetNode, box: RenderBox): RenderBox {
  box.debugId = node.id;
  return box;
}

function flexible(child: RenderBox): RenderBox {
  const data = new FlexParentData();
  data.flex = 1;
  data.fit = FlexFit.tight;
  child.parentData = data;
  return child;
}

/** `CountdownTimer`: the remaining time in `titleMedium`, drawn left to right; a design-time screen shows it at zero. */
const countdownTimer: WidgetBuilder = (node, ctx) => {
  const format = text(node, "format") ?? "hms";
  const zero = format === "dhms" ? "0d 00:00:00" : format === "compact" ? "0s" : "00:00:00";
  // A box around the paragraph keeps the rect a box's: the runtime records no baseline for the component.
  return named(
    node,
    new RenderConstrainedBox({
      additionalConstraints: new BoxConstraints(),
      child: line(ctx, zero, "titleMedium", true),
    }),
  );
};

/** `ConsentCheckbox`: a 48 px checkbox beside the consent text, and under it, set in by 48, the optional link. */
const consentCheckbox: WidgetBuilder = (node, ctx) => {
  const dir = direction(ctx);
  const row = new RenderFlex({
    direction: Axis.horizontal,
    mainAxisSize: MainAxisSize.max,
    crossAxisAlignment: CrossAxisAlignment.center,
    textDirection: dir,
    children: [sized(48, 48), flexible(line(ctx, text(node, "label") ?? "", "bodyMedium"))],
  });
  const children: RenderBox[] = [
    new RenderConstrainedBox({ additionalConstraints: new BoxConstraints({ minHeight: 48 }), child: row }),
  ];
  const link = text(node, "linkLabel");
  if (link !== undefined) {
    children.push(
      new RenderPadding({
        padding: EdgeInsetsDirectional.only({ start: 48 }),
        textDirection: dir,
        child: buildButton({
          kind: "text",
          label: line(ctx, link, "labelLarge"),
          tapTargetSize: MaterialTapTargetSize.padded,
          density: VisualDensity.standard,
          textDirection: dir,
          textScale: textScaleOf(ctx),
        }),
      }),
    );
  }
  return named(
    node,
    new RenderFlex({
      direction: Axis.vertical,
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.start,
      textDirection: dir,
      children,
    }),
  );
};

/** One `ChoiceItem` of a list prop: its label and whether it is selected. */
function choices(node: WidgetNode, name: string): { label: string; selected: boolean }[] {
  const list = node.props?.[name];
  if (!Array.isArray(list)) {
    return [];
  }
  return list.flatMap((item: unknown) => {
    const choice = item as { label?: unknown; selected?: unknown } | null;
    return typeof choice?.label === "string" ? [{ label: choice.label, selected: choice.selected === true }] : [];
  });
}

/** `FilterBar`: filter chips, 8 apart, in a horizontally scrolling row padded by 16. */
const filterBar: WidgetBuilder = (node, ctx) => {
  const dir = direction(ctx);
  const scale = textScaleOf(ctx);
  const chips = choices(node, "filters").map(
    (chip) =>
      new RenderPadding({
        padding: EdgeInsetsDirectional.only({ end: 8 }),
        textDirection: dir,
        child: buildChip({
          label: line(ctx, chip.label, "labelLarge"),
          avatar: null,
          deleteIcon: null,
          selected: chip.selected,
          tapTargetSize: MaterialTapTargetSize.padded,
          density: VisualDensity.standard,
          textScale: scale,
          textDirection: dir,
        }),
      }),
  );
  const row = new RenderFlex({
    direction: Axis.horizontal,
    mainAxisSize: MainAxisSize.max,
    crossAxisAlignment: CrossAxisAlignment.center,
    textDirection: dir,
    children: chips,
  });
  return named(
    node,
    new RenderSingleChildScroll(
      new RenderPadding({
        padding: EdgeInsets.symmetric({ horizontal: 16 }),
        textDirection: dir,
        child: row,
      }),
      Axis.horizontal,
      dir,
    ),
  );
};

/** `ImageGallery`: a `PageView` as wide as the space and 4:3 of it high; its images are not drawn. */
const imageGallery: WidgetBuilder = (node, ctx) => named(node, sized(ctx.available(), (ctx.available() * 3) / 4));

/** `CountryPicker` and `CurrencyPicker`: a search field over a list, in a box 360 high. */
const picker: WidgetBuilder = (node, ctx) => named(node, sized(ctx.available(), 360));

/** A `TextField` with a label: one line of `bodyLarge` under the default underline decoration. */
function labelledField(node: WidgetNode, ctx: WidgetContext, prefix: string | null): RenderBox {
  const label = text(node, "label");
  return named(
    node,
    buildInputDecorator({
      input: new RenderEditableLine(line(ctx, "A", "bodyLarge")),
      prefixIcon: null,
      suffixIcon: null,
      prefix: prefix === null ? null : line(ctx, prefix, "bodyLarge"),
      isEmpty: true,
      subtext: null,
      outline: false,
      filled: false,
      isDense: false,
      hasLabel: label !== undefined,
      floating: false,
      floatingLabelHeight: 16 * textScaleOf(ctx),
      density: VisualDensity.standard,
      textDirection: direction(ctx),
    }),
  );
}

/** `IbanInput`: a labelled text field. */
const ibanInput: WidgetBuilder = (node, ctx) => labelledField(node, ctx, null);

/** `AmountInput`: a labelled text field whose prefix is the currency. */
const amountInput: WidgetBuilder = (node, ctx) => labelledField(node, ctx, `${text(node, "currency") ?? ""} `);

/** The money of a `{label, amount}` line, as the runtime writes it. */
function moneyText(value: unknown): string {
  const money = moneyOf(value);
  return money === null ? "" : formatMoney(money.amount, money.currency);
}

/** A cart row: label and amount in one line, padded 4 above and below. */
function cartRow(ctx: WidgetContext, label: string, amount: string, name: TextRole): RenderBox {
  return new RenderPadding({
    padding: EdgeInsets.symmetric({ vertical: 4 }),
    child: new RenderFlex({
      direction: Axis.horizontal,
      mainAxisSize: MainAxisSize.max,
      crossAxisAlignment: CrossAxisAlignment.center,
      textDirection: direction(ctx),
      children: [flexible(line(ctx, label, name)), line(ctx, amount, name)],
    }),
  });
}

/** `CartSummary`: the lines, a divider, subtotal, tax and total, each a row across the full width. */
const cartSummary: WidgetBuilder = (node, ctx) => {
  const rows: RenderBox[] = [];
  const lines = node.props?.["lines"];
  for (const item of Array.isArray(lines) ? lines : []) {
    const entry = item as { label?: unknown; amount?: unknown } | null;
    rows.push(
      cartRow(ctx, typeof entry?.label === "string" ? entry.label : "", moneyText(entry?.amount), "bodyMedium"),
    );
  }
  rows.push(sized(0, 16));
  rows.push(cartRow(ctx, text(node, "subtotalLabel") ?? "Subtotal", moneyText(node.props?.["subtotal"]), "bodyMedium"));
  if (node.props?.["tax"] !== undefined) {
    rows.push(cartRow(ctx, text(node, "taxLabel") ?? "Tax", moneyText(node.props["tax"]), "bodyMedium"));
  }
  rows.push(cartRow(ctx, text(node, "totalLabel") ?? "Total", moneyText(node.props?.["total"]), "titleMedium"));
  return named(
    node,
    new RenderFlex({
      direction: Axis.vertical,
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      textDirection: direction(ctx),
      children: rows,
    }),
  );
};

/** The Layer 2 builders of batch A, by widget type. */
export const layer2AWidgetBuilders: ReadonlyArray<readonly [string, WidgetBuilder]> = [
  ["AmountInput", amountInput],
  ["CartSummary", cartSummary],
  ["ConsentCheckbox", consentCheckbox],
  ["CountdownTimer", countdownTimer],
  ["CountryPicker", picker],
  ["CurrencyPicker", picker],
  ["FilterBar", filterBar],
  ["IbanInput", ibanInput],
  ["ImageGallery", imageGallery],
];
