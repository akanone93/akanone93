// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Builders for the Layer 2 components of a screen document (STU-005, S4.5,
 * WGT-020). Each composes the render objects the runtime's builder in
 * `packages/plux_components` composes, with Material 3 text roles in the
 * screen theme's fonts, so the layout agrees with the runtime's; the layout
 * conformance suite checks them against goldens the runtime writes.
 * Components whose content is platform media lay out as their box only.
 */

import { BoxConstraints } from "../flutter/constraints.ts";
import { Axis, CrossAxisAlignment, MainAxisSize, RenderFlex } from "../flutter/flex.ts";
import { EdgeInsets, TextDirection } from "../flutter/geometry.ts";
import { m3TextStyle, type TextRole } from "../flutter/material/theme.ts";
import { RenderConstrainedBox } from "../flutter/proxy-box.ts";
import type { RenderBox } from "../flutter/render-box.ts";
import { RenderPadding } from "../flutter/shifted-box.ts";
import type { TextStyle } from "../flutter/text-style.ts";
import { layer2AWidgetBuilders } from "./layer2-a.ts";
import { layer2BWidgetBuilders } from "./layer2-b.ts";
import { layer2CWidgetBuilders } from "./layer2-c.ts";
import { layer2DWidgetBuilders } from "./layer2-d.ts";
import type { WidgetBuilder, WidgetContext, WidgetNode } from "./screen.ts";

function text(node: WidgetNode, name: string): string | undefined {
  const value = node.props?.[name];
  return typeof value === "string" ? value : undefined;
}

function flag(node: WidgetNode, name: string): boolean | undefined {
  const value = node.props?.[name];
  return typeof value === "boolean" ? value : undefined;
}

function role(ctx: WidgetContext, name: TextRole): TextStyle {
  const [family, ...fallback] = ctx.theme.fontFamilies;
  return { ...m3TextStyle(name, family), fontFamilyFallback: fallback };
}

function direction(ctx: WidgetContext): TextDirection {
  return ctx.textDirection ?? TextDirection.ltr;
}

/** A paragraph in `name`'s role. */
function line(ctx: WidgetContext, value: string, name: TextRole, fixedLtr = false): RenderBox {
  return ctx.paragraph(value, role(ctx, name), fixedLtr ? { textDirection: TextDirection.ltr } : undefined);
}

function gap(height: number): RenderBox {
  return new RenderConstrainedBox({ additionalConstraints: BoxConstraints.tightFor({ height }) });
}

function column(ctx: WidgetContext, children: RenderBox[], crossAxis = CrossAxisAlignment.start): RenderFlex {
  return new RenderFlex({
    direction: Axis.vertical,
    mainAxisSize: MainAxisSize.min,
    crossAxisAlignment: crossAxis,
    textDirection: direction(ctx),
    children,
  });
}

const currencySymbols: Readonly<Record<string, string>> = { EUR: "€", USD: "$", GBP: "£", JPY: "¥" };
const zeroDigits = new Set(["JPY", "KRW", "VND", "CLP", "ISK", "UGX", "XAF", "XOF"]);

/** The money value of a document prop: `money("1234.56", "EUR")` as the compiler keeps it. */
export function moneyOf(value: unknown): { amount: string; currency: string } | null {
  const expr = (value as { $expr?: unknown } | null)?.$expr;
  const match = typeof expr === "string" ? /^money\("(-?[0-9.]+)", "([A-Z]{3})"\)$/.exec(expr) : null;
  return match?.[1] !== undefined && match[2] !== undefined ? { amount: match[1], currency: match[2] } : null;
}

/** The English format of the runtime's built-in locale data: `€1,234.56`. */
export function formatMoney(amount: string, currency: string): string {
  const digits = zeroDigits.has(currency) ? 0 : 2;
  const negative = amount.startsWith("-");
  const [whole = "0", fraction = ""] = amount.replace("-", "").split(".");
  const grouped = whole.replace(/\B(?=(\d{3})+(?!\d))/g, ",");
  const symbol = currencySymbols[currency] ?? `${currency} `;
  return `${negative ? "-" : ""}${symbol}${grouped}${digits > 0 ? `.${fraction.padEnd(digits, "0").slice(0, digits)}` : ""}`;
}

/** A card's face: a box at least 180 high and at most 360 wide, padded 20, its lines in a column. */
function card(ctx: WidgetContext, lines: RenderBox[]): RenderBox {
  return new RenderConstrainedBox({
    additionalConstraints: new BoxConstraints({ maxWidth: 360, minHeight: 180 }),
    child: new RenderPadding({
      padding: EdgeInsets.all(20),
      textDirection: direction(ctx),
      child: column(ctx, lines),
    }),
  });
}

function named(node: WidgetNode, box: RenderBox): RenderBox {
  box.debugId = node.id;
  return box;
}

/** `AccountCard`: name, masked number and balance on the gradient card; the back shows `backLabel`. */
const accountCard: WidgetBuilder = (node, ctx) => {
  const name = text(node, "name") ?? "";
  if (flag(node, "flipped") === true) {
    return named(node, card(ctx, [line(ctx, text(node, "backLabel") ?? name, "titleMedium")]));
  }
  const balance = moneyOf(node.props?.["balance"]);
  return named(
    node,
    card(ctx, [
      line(ctx, name, "titleMedium"),
      gap(16),
      line(ctx, text(node, "maskedNumber") ?? "", "titleLarge", true),
      gap(16),
      line(ctx, balance === null ? "" : formatMoney(balance.amount, balance.currency), "headlineSmall"),
    ]),
  );
};

/** The Layer 2 builders, by widget type. */
export const layer2WidgetBuilders: ReadonlyArray<readonly [string, WidgetBuilder]> = [
  ["AccountCard", accountCard],
  ...layer2AWidgetBuilders,
  ...layer2BWidgetBuilders,
  ...layer2CWidgetBuilders,
  ...layer2DWidgetBuilders,
];

/** The Layer 2 types the layout conformance suite compares. */
export const layer2Types: ReadonlySet<string> = new Set(layer2WidgetBuilders.map(([type]) => type));
