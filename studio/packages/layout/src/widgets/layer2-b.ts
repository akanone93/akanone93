// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Builders for the Layer 2 components of a screen document, batch B (STU-005,
 * S4.5, WGT-020). Each composes the render objects the runtime's builder in
 * `packages/plux_components` composes, with Material 3 text roles in the
 * screen theme's fonts, so the layout agrees with the runtime's; the layout
 * conformance suite checks them against goldens the runtime writes.
 * Components whose content is platform media lay out as their box only.
 */

import { BoxConstraints } from "../flutter/constraints.ts";
import { Axis, CrossAxisAlignment, FlexFit, FlexParentData, MainAxisSize, RenderFlex } from "../flutter/flex.ts";
import { EdgeInsets, EdgeInsetsDirectional, TextDirection } from "../flutter/geometry.ts";
import { type ButtonKind, buildButton } from "../flutter/material/buttons.ts";
import { buildInputDecorator, RenderEditableLine } from "../flutter/material/input-decorator.ts";
import { MaterialTapTargetSize, m3TextStyle, type TextRole, VisualDensity } from "../flutter/material/theme.ts";
import { RenderConstrainedBox } from "../flutter/proxy-box.ts";
import { RenderAspectRatio } from "../flutter/proxy-boxes.ts";
import type { RenderBox } from "../flutter/render-box.ts";
import { RenderPadding } from "../flutter/shifted-box.ts";
import type { TextStyle } from "../flutter/text-style.ts";
import { RenderWrap, WrapCrossAlignment } from "../flutter/wrap.ts";
import { formatMoney, moneyOf } from "./layer2.ts";
import type { WidgetBuilder, WidgetContext, WidgetNode } from "./screen.ts";

function text(node: WidgetNode, name: string): string | undefined {
  const value = node.props?.[name];
  return typeof value === "string" ? value : undefined;
}

function flag(node: WidgetNode, name: string): boolean | undefined {
  const value = node.props?.[name];
  return typeof value === "boolean" ? value : undefined;
}

function count(node: WidgetNode, name: string): number | undefined {
  const value = node.props?.[name];
  return typeof value === "number" ? value : undefined;
}

function role(ctx: WidgetContext, name: TextRole, weight?: number): TextStyle {
  const [family, ...fallback] = ctx.theme.fontFamilies;
  return {
    ...m3TextStyle(name, family),
    fontFamilyFallback: fallback,
    ...(weight === undefined ? {} : { fontWeight: weight }),
  };
}

function direction(ctx: WidgetContext): TextDirection {
  return ctx.textDirection ?? TextDirection.ltr;
}

/** A paragraph in `name`'s role. */
function line(ctx: WidgetContext, value: string, name: TextRole, fixedLtr = false, weight?: number): RenderBox {
  return ctx.paragraph(value, role(ctx, name, weight), fixedLtr ? { textDirection: TextDirection.ltr } : undefined);
}

function gap(height: number): RenderBox {
  return new RenderConstrainedBox({ additionalConstraints: BoxConstraints.tightFor({ height }) });
}

function column(
  ctx: WidgetContext,
  children: RenderBox[],
  crossAxis = CrossAxisAlignment.start,
  size = MainAxisSize.min,
): RenderFlex {
  return new RenderFlex({
    direction: Axis.vertical,
    mainAxisSize: size,
    crossAxisAlignment: crossAxis,
    textDirection: direction(ctx),
    children,
  });
}

function row(ctx: WidgetContext, children: RenderBox[], size: MainAxisSize): RenderFlex {
  return new RenderFlex({
    direction: Axis.horizontal,
    mainAxisSize: size,
    crossAxisAlignment: CrossAxisAlignment.center,
    textDirection: direction(ctx),
    children,
  });
}

/** `Expanded`: the child fills the space left in its row. */
function expanded(child: RenderBox): RenderBox {
  const parentData = new FlexParentData();
  parentData.flex = 1;
  parentData.fit = FlexFit.tight;
  child.parentData = parentData;
  return child;
}

function padded(ctx: WidgetContext, padding: EdgeInsetsDirectional, child: RenderBox): RenderBox {
  return new RenderPadding({ padding, textDirection: direction(ctx), child });
}

function all(ctx: WidgetContext, value: number, child: RenderBox): RenderBox {
  return new RenderPadding({ padding: EdgeInsets.all(value), textDirection: direction(ctx), child });
}

/** The 48 px minimum touch target the runtime gives widgets whose descriptor sets `minTarget` (A11Y-004). */
function target(node: WidgetNode, box: RenderBox): RenderBox {
  return named(
    node,
    new RenderConstrainedBox({
      additionalConstraints: new BoxConstraints({ minWidth: 48, minHeight: 48 }),
      child: box,
    }),
  );
}

function named(node: WidgetNode, box: RenderBox): RenderBox {
  box.debugId = node.id;
  return box;
}

/** A Material button's box: 48 px tap target around a 40 px button. */
function button(ctx: WidgetContext, kind: ButtonKind, label: string): RenderBox {
  return buildButton({
    kind,
    label: line(ctx, label, "labelLarge"),
    tapTargetSize: MaterialTapTargetSize.padded,
    density: VisualDensity.named(undefined),
    textDirection: direction(ctx),
    textScale: ctx.textScale ?? 1,
  });
}

/** `LiveEtaCard`: a card of 16 padding; the optional icon, then title, minutes and status. */
const liveEtaCard: WidgetBuilder = (node, ctx) => {
  const title = text(node, "title");
  const minutes = count(node, "etaMinutes");
  const status = text(node, "status");
  const unit = text(node, "minutesLabel") ?? "min";
  const lines: RenderBox[] = [];
  if (title !== undefined) {
    lines.push(line(ctx, title, "labelLarge"));
  }
  if (minutes !== undefined) {
    lines.push(line(ctx, `${minutes} ${unit}`, "headlineSmall"));
  }
  if (status !== undefined) {
    lines.push(line(ctx, status, "bodyMedium"));
  }
  const cells: RenderBox[] = [];
  if (node.props?.["icon"] !== undefined && node.props["icon"] !== null) {
    cells.push(
      new RenderConstrainedBox({ additionalConstraints: BoxConstraints.tightFor({ width: 32, height: 32 }) }),
      new RenderConstrainedBox({ additionalConstraints: BoxConstraints.tightFor({ width: 16 }) }),
    );
  }
  cells.push(expanded(column(ctx, lines)));
  return named(node, all(ctx, 16, row(ctx, cells, MainAxisSize.max)));
};

/** `value` hidden but for its last `visible` digits, as the runtime's `_mask` writes it. */
export function maskValue(value: string, kind: string, visible: number): string {
  const plain = kind === "card" ? value.replaceAll(" ", "") : value;
  const chars = [...plain];
  const hideable: number[] = [];
  chars.forEach((ch, i) => {
    if (kind === "custom" || (ch >= "0" && ch <= "9")) {
      hideable.push(i);
    }
  });
  const keep = new Set(hideable.length > visible ? hideable.slice(hideable.length - visible) : hideable);
  const hide = new Set(hideable.filter((i) => !keep.has(i)));
  let out = "";
  chars.forEach((ch, i) => {
    if (kind === "card" && i > 0 && i % 4 === 0) {
      out += " ";
    }
    out += hide.has(i) ? "•" : ch;
  });
  return out;
}

/** `MaskedText`: the masked value, always left to right, and the reveal button when `revealable`. */
const maskedText: WidgetBuilder = (node, ctx) => {
  const shown = maskValue(
    text(node, "value") ?? "",
    text(node, "mask") ?? "card",
    Math.max(0, count(node, "visibleDigits") ?? 4),
  );
  const cells: RenderBox[] = [line(ctx, shown, "bodyMedium", true)];
  if (flag(node, "revealable") === true) {
    cells.push(
      buildButton({
        kind: "icon",
        label: new RenderConstrainedBox({ additionalConstraints: BoxConstraints.tightFor({ width: 24, height: 24 }) }),
        tapTargetSize: MaterialTapTargetSize.padded,
        density: VisualDensity.named(undefined),
        textDirection: direction(ctx),
        textScale: 1,
      }),
    );
  }
  return target(node, row(ctx, cells, MainAxisSize.min));
};

/** `MoneyText`: the amount in the runtime's English format, in the default text style. */
const moneyText: WidgetBuilder = (node, ctx) => {
  const money = moneyOf(node.props?.["value"]);
  const label = line(ctx, money === null ? "" : formatMoney(money.amount, money.currency), "bodyMedium");
  return named(node, new RenderConstrainedBox({ additionalConstraints: new BoxConstraints(), child: label }));
};

/** `PriceTag`: the price and the struck-through original price in one wrapping run, 8 apart. */
const priceTag: WidgetBuilder = (node, ctx) => {
  const price = moneyOf(node.props?.["price"]);
  const original = moneyOf(node.props?.["originalPrice"]);
  const children = [line(ctx, price === null ? "" : formatMoney(price.amount, price.currency), "titleLarge")];
  if (original !== null) {
    children.push(line(ctx, formatMoney(original.amount, original.currency), "bodyMedium"));
  }
  return named(
    node,
    new RenderWrap({
      spacing: 8,
      crossAxisAlignment: WrapCrossAlignment.end,
      textDirection: direction(ctx),
      children,
    }),
  );
};

/** `OfflineBanner`: while the device is online it is an empty box as wide as its parent. */
const offlineBanner: WidgetBuilder = (node) =>
  named(
    node,
    new RenderConstrainedBox({
      additionalConstraints: new BoxConstraints({
        minWidth: Number.POSITIVE_INFINITY,
        maxWidth: Number.POSITIVE_INFINITY,
        minHeight: 0,
        maxHeight: 0,
      }),
    }),
  );

/** `OnboardingPager`: 360 px of pages (the pages' own layout is theirs), the indicators and the button. */
const onboardingPager: WidgetBuilder = (node, ctx) => {
  const label = text(node, "nextLabel") ?? "Next";
  const pages = node.children ?? [];
  const current = pages[Math.min(Math.max(0, count(node, "currentPage") ?? 0), pages.length - 1)];
  const page = new RenderConstrainedBox({
    additionalConstraints: BoxConstraints.tightFor({ width: ctx.available(), height: 360 }),
    child: current === undefined ? null : ctx.build(current),
  });
  const indicators = new RenderConstrainedBox({ additionalConstraints: BoxConstraints.tightFor({ height: 16 }) });
  return target(
    node,
    column(ctx, [page, indicators, all(ctx, 16, button(ctx, "filled", label))], CrossAxisAlignment.stretch),
  );
};

/** `PaymentCard`: brand, masked number, holder and expiry on the gradient card; the back shows a stripe. */
const paymentCard: WidgetBuilder = (node, ctx) => {
  const lines: RenderBox[] = [];
  if (flag(node, "flipped") === true) {
    lines.push(
      new RenderConstrainedBox({
        additionalConstraints: new BoxConstraints({
          minWidth: Number.POSITIVE_INFINITY,
          maxWidth: Number.POSITIVE_INFINITY,
          minHeight: 40,
          maxHeight: 40,
        }),
      }),
    );
    const back = text(node, "backLabel");
    if (back !== undefined) {
      lines.push(gap(16), line(ctx, back, "bodyMedium"));
    }
  } else {
    lines.push(
      line(ctx, (text(node, "brand") ?? "other").toUpperCase(), "titleMedium", false, 700),
      gap(16),
      line(ctx, text(node, "maskedNumber") ?? "", "titleLarge", true),
      gap(16),
      row(
        ctx,
        [
          expanded(line(ctx, text(node, "holder") ?? "", "bodyMedium")),
          line(ctx, text(node, "expiry") ?? "", "bodyMedium", true),
        ],
        MainAxisSize.max,
      ),
    );
  }
  return target(
    node,
    new RenderConstrainedBox({
      additionalConstraints: new BoxConstraints({ maxWidth: 360, minHeight: 180 }),
      child: all(ctx, 20, column(ctx, lines)),
    }),
  );
};

/** `ProductCard`: a 4:3 picture, then title and price on 12 padding; the width of its parent. */
const productCard: WidgetBuilder = (node, ctx) => {
  const prices = [line(ctx, text(node, "price") ?? "", "titleMedium")];
  const original = text(node, "originalPrice");
  if (original !== undefined) {
    prices.push(line(ctx, original, "bodySmall"));
  }
  const details = column(ctx, [
    line(ctx, text(node, "title") ?? "", "titleSmall"),
    gap(4),
    new RenderWrap({
      spacing: 8,
      crossAxisAlignment: WrapCrossAlignment.center,
      textDirection: direction(ctx),
      children: prices,
    }),
  ]);
  return target(
    node,
    column(
      ctx,
      [
        new RenderAspectRatio({ aspectRatio: 4 / 3 }),
        padded(ctx, EdgeInsetsDirectional.fromSTEB(12, 12, 12, 8), details),
      ],
      CrossAxisAlignment.stretch,
    ),
  );
};

/** `ProfileHeader`: the avatar circle, the name and the subtitle, centred on 16 padding. */
const profileHeader: WidgetBuilder = (node, ctx) => {
  const radius = count(node, "radius") ?? 32;
  const lines: RenderBox[] = [
    new RenderConstrainedBox({
      additionalConstraints: BoxConstraints.tightFor({ width: radius * 2, height: radius * 2 }),
    }),
    gap(12),
    line(ctx, text(node, "name") ?? "", "headlineSmall"),
  ];
  const subtitle = text(node, "subtitle");
  if (subtitle !== undefined) {
    lines.push(line(ctx, subtitle, "bodyMedium"));
  }
  return named(node, all(ctx, 16, column(ctx, lines, CrossAxisAlignment.center)));
};

/** `PromoBanner`: title, subtitle and the call to action's pill on 16 padding. */
const promoBanner: WidgetBuilder = (node, ctx) => {
  const lines: RenderBox[] = [line(ctx, text(node, "title") ?? "", "titleLarge")];
  const subtitle = text(node, "subtitle");
  if (subtitle !== undefined) {
    lines.push(line(ctx, subtitle, "bodyMedium"));
  }
  const cta = text(node, "ctaLabel");
  if (cta !== undefined) {
    lines.push(gap(12), padded(ctx, EdgeInsetsDirectional.fromSTEB(16, 8, 16, 8), line(ctx, cta, "labelLarge")));
  }
  return target(node, all(ctx, 16, column(ctx, lines)));
};

/** A `TextField` with an underline border and an optional label, its text in `name`'s role. */
function field(ctx: WidgetContext, name: TextRole, label: string | undefined): RenderBox {
  return buildInputDecorator({
    input: new RenderEditableLine(line(ctx, "A", name)),
    prefixIcon: null,
    suffixIcon: null,
    subtext: null,
    outline: false,
    filled: false,
    isDense: false,
    hasLabel: label !== undefined,
    floating: false,
    floatingLabelHeight: 16 * (ctx.textScale ?? 1),
    density: VisualDensity.named(undefined),
    textDirection: direction(ctx),
  });
}

/** `OtpInput`: one headline-size text field labelled "One-time code", the width of its parent. */
const otpInput: WidgetBuilder = (node, ctx) => named(node, field(ctx, "headlineSmall", "One-time code"));

/** `PhoneInput`: the region button, then the number field filling the row. */
const phoneInput: WidgetBuilder = (node, ctx) =>
  target(
    node,
    new RenderFlex({
      direction: Axis.horizontal,
      mainAxisSize: MainAxisSize.max,
      crossAxisAlignment: CrossAxisAlignment.start,
      textDirection: direction(ctx),
      children: [button(ctx, "text", "+"), expanded(field(ctx, "bodyLarge", text(node, "label")))],
    }),
  );

/** `KycCameraCapture`: the framing guide (3:4 for a selfie, 1.586 for a document), the hint and the capture button. */
const kycCameraCapture: WidgetBuilder = (node, ctx) => {
  const selfie = text(node, "mode") === "selfie";
  const hint = selfie ? "Center your face in the oval." : "Fit the whole document inside the frame.";
  return named(
    node,
    column(
      ctx,
      [
        new RenderAspectRatio({ aspectRatio: selfie ? 3 / 4 : 1.586 }),
        gap(8),
        line(ctx, hint, "bodyMedium"),
        gap(8),
        new RenderWrap({
          spacing: 12,
          textDirection: direction(ctx),
          children: [button(ctx, "outlined", "Take photo")],
        }),
      ],
      CrossAxisAlignment.center,
    ),
  );
};

/** The batch B builders, by widget type. */
export const layer2BWidgetBuilders: ReadonlyArray<readonly [string, WidgetBuilder]> = [
  ["KycCameraCapture", kycCameraCapture],
  ["LiveEtaCard", liveEtaCard],
  ["MaskedText", maskedText],
  ["MoneyText", moneyText],
  ["OfflineBanner", offlineBanner],
  ["OnboardingPager", onboardingPager],
  ["OtpInput", otpInput],
  ["PaymentCard", paymentCard],
  ["PhoneInput", phoneInput],
  ["PriceTag", priceTag],
  ["ProductCard", productCard],
  ["ProfileHeader", profileHeader],
  ["PromoBanner", promoBanner],
];
