// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Layer 2 builders, batch C (STU-005, S4.5, WGT-020): each composes the
 * render objects the runtime's builder in `packages/plux_components`
 * composes, so the layout agrees with the runtime's; the layout conformance
 * suite checks them against goldens the runtime writes.
 */

import { BoxConstraints } from "../flutter/constraints.ts";
import { expanded } from "../flutter/cupertino.ts";
import { Axis, CrossAxisAlignment, MainAxisSize, RenderFlex } from "../flutter/flex.ts";
import { EdgeInsets, TextDirection } from "../flutter/geometry.ts";
import { buildInputDecorator, decoratorIcon, RenderEditableLine } from "../flutter/material/input-decorator.ts";
import { m3TextStyle, type TextRole, VisualDensity } from "../flutter/material/theme.ts";
import { RenderConstrainedBox } from "../flutter/proxy-box.ts";
import type { RenderBox } from "../flutter/render-box.ts";
import { RenderPadding } from "../flutter/shifted-box.ts";
import type { TextStyle } from "../flutter/text-style.ts";
import { formatMoney, moneyOf } from "./layer2.ts";
import type { WidgetBuilder, WidgetContext, WidgetNode } from "./screen.ts";

function num(node: WidgetNode, name: string): number | undefined {
  const value = node.props?.[name];
  return typeof value === "number" ? value : undefined;
}

function str(node: WidgetNode, name: string): string | undefined {
  const value = node.props?.[name];
  return typeof value === "string" ? value : undefined;
}

/** The `label` of each struct of a list prop (`ChoiceItem`, `TimelineStep`, `SettingsSection` carry it or `title`). */
function structs(node: WidgetNode, name: string): Array<Record<string, unknown>> {
  const value = node.props?.[name];
  return Array.isArray(value)
    ? value.filter((v): v is Record<string, unknown> => typeof v === "object" && v !== null)
    : [];
}

function field(item: Record<string, unknown>, name: string): string | undefined {
  const value = item[name];
  return typeof value === "string" ? value : undefined;
}

function role(ctx: WidgetContext, name: TextRole): TextStyle {
  const [family, ...fallback] = ctx.theme.fontFamilies;
  return { ...m3TextStyle(name, family), fontFamilyFallback: fallback };
}

function direction(ctx: WidgetContext): TextDirection {
  return ctx.textDirection ?? TextDirection.ltr;
}

function line(ctx: WidgetContext, value: string, name: TextRole): RenderBox {
  return ctx.paragraph(value, role(ctx, name));
}

function box(width: number, height: number, child: RenderBox | null = null): RenderBox {
  return new RenderConstrainedBox({ additionalConstraints: BoxConstraints.tightFor({ width, height }), child });
}

function row(ctx: WidgetContext, children: RenderBox[]): RenderFlex {
  return new RenderFlex({
    direction: Axis.horizontal,
    mainAxisSize: MainAxisSize.min,
    crossAxisAlignment: CrossAxisAlignment.center,
    textDirection: direction(ctx),
    children,
  });
}

function padded(ctx: WidgetContext, padding: EdgeInsets, child: RenderBox): RenderBox {
  return new RenderPadding({ padding, textDirection: direction(ctx), child });
}

function stack(ctx: WidgetContext, children: RenderBox[], crossAxis = CrossAxisAlignment.stretch): RenderFlex {
  return new RenderFlex({
    direction: Axis.vertical,
    mainAxisSize: MainAxisSize.min,
    crossAxisAlignment: crossAxis,
    textDirection: direction(ctx),
    children,
  });
}

function named(node: WidgetNode, built: RenderBox): RenderBox {
  built.debugId = node.id;
  return built;
}

/** `QuantityStepper`: a 48 px button, the quantity in at least 40 px, a 48 px button. */
const quantityStepper: WidgetBuilder = (node, ctx) => {
  const min = num(node, "min") ?? 0;
  const max = num(node, "max") ?? 99;
  const hi = max < min ? min : max;
  const value = Math.min(Math.max(num(node, "value") ?? 1, min), hi);
  const quantity = new RenderConstrainedBox({
    additionalConstraints: new BoxConstraints({ minWidth: 40 }),
    child: line(ctx, String(value), "titleMedium"),
  });
  return named(node, row(ctx, [box(48, 48), quantity, box(48, 48)]));
};

/** `RatingBar`: `max` stars of 20 px in a row. */
const ratingBar: WidgetBuilder = (node, ctx) => {
  const max = Math.min(Math.max(Math.trunc(num(node, "max") ?? 5), 1), 10);
  return named(
    node,
    row(
      ctx,
      Array.from({ length: max }, () => box(20, 20)),
    ),
  );
};

/** `RatingInput`: `max` icon buttons of 48 px in a row. */
const ratingInput: WidgetBuilder = (node, ctx) => {
  const max = Math.min(Math.max(Math.trunc(num(node, "max") ?? 5), 1), 10);
  return named(
    node,
    row(
      ctx,
      Array.from({ length: max }, () => box(48, 48)),
    ),
  );
};

/** `SearchField`: an outlined `TextField` with the search icon as its prefix and the hint shown while empty. */
const searchField: WidgetBuilder = (node, ctx) => {
  const hint = str(node, "hint") ?? "Search";
  const text = str(node, "value") ?? "";
  const field = buildInputDecorator({
    input: new RenderEditableLine(line(ctx, "A", "bodyLarge")),
    prefixIcon: decoratorIcon(box(24, 24), VisualDensity.standard),
    suffixIcon: null,
    hint: line(ctx, hint, "bodyLarge"),
    isEmpty: text === "",
    subtext: null,
    outline: true,
    filled: false,
    isDense: false,
    hasLabel: false,
    floating: false,
    floatingLabelHeight: 16,
    density: VisualDensity.standard,
    textDirection: direction(ctx),
  });
  return named(node, stack(ctx, [field]));
};

/** `SortSheet`: an optional title, then a 48 px radio row per option. */
const sortSheet: WidgetBuilder = (node, ctx) => {
  const title = str(node, "title");
  const rows = structs(node, "options").map(
    (option) =>
      new RenderConstrainedBox({
        additionalConstraints: new BoxConstraints({ minHeight: 48 }),
        child: padded(
          ctx,
          EdgeInsets.symmetric({ horizontal: 16, vertical: 8 }),
          row(ctx, [box(24, 24), box(16, 0), expanded(line(ctx, field(option, "label") ?? "", "bodyMedium"))]),
        ),
      }),
  );
  const head =
    title === undefined ? [] : [padded(ctx, EdgeInsets.fromLTRB(16, 16, 16, 8), line(ctx, title, "titleMedium"))];
  return named(node, stack(ctx, [...head, ...rows]));
};

/** `StatusTimeline`: per step a 24 px marker beside the label and detail, 16 below. */
const statusTimeline: WidgetBuilder = (node, ctx) => {
  const steps = structs(node, "steps").map((step) => {
    const detail = field(step, "detail");
    const texts = [line(ctx, field(step, "label") ?? "", "titleSmall")];
    if (detail !== undefined) {
      texts.push(line(ctx, detail, "bodySmall"));
    }
    const body = padded(ctx, EdgeInsets.only({ bottom: 16 }), stack(ctx, texts, CrossAxisAlignment.start));
    return new RenderFlex({
      direction: Axis.horizontal,
      mainAxisSize: MainAxisSize.max,
      crossAxisAlignment: CrossAxisAlignment.start,
      textDirection: direction(ctx),
      children: [box(40, 24), expanded(body)],
    });
  });
  return named(node, stack(ctx, steps));
};

/** `SettingsList`: a header (16, 16, 16, 8) in `titleSmall` per section; the rows are children. */
const settingsList: WidgetBuilder = (node, ctx) => {
  const headers = structs(node, "sections").map((section) =>
    padded(ctx, EdgeInsets.fromLTRB(16, 16, 16, 8), line(ctx, field(section, "title") ?? "", "titleSmall")),
  );
  return named(node, stack(ctx, headers));
};

/** `TransactionListItem`: title and details beside amount and status, at least 56 high, padded 16 by 8. */
const transactionListItem: WidgetBuilder = (node, ctx) => {
  const subtitle = str(node, "subtitle");
  const status = str(node, "statusLabel") ?? str(node, "status") ?? "completed";
  const amount = moneyOf(node.props?.["amount"]);
  const shown = amount === null ? "" : formatMoney(amount.amount, amount.currency);
  const money = amount !== null && !amount.amount.startsWith("-") ? `+${shown}` : shown;
  const left = [line(ctx, str(node, "title") ?? "", "bodyLarge")];
  if (subtitle !== undefined && subtitle !== "") {
    left.push(line(ctx, subtitle, "bodySmall"));
  }
  const right = stack(ctx, [line(ctx, money, "bodyLarge"), line(ctx, status, "bodySmall")], CrossAxisAlignment.end);
  const content = new RenderFlex({
    direction: Axis.horizontal,
    mainAxisSize: MainAxisSize.max,
    crossAxisAlignment: CrossAxisAlignment.center,
    textDirection: direction(ctx),
    children: [expanded(stack(ctx, left, CrossAxisAlignment.start)), right],
  });
  return named(
    node,
    new RenderConstrainedBox({
      additionalConstraints: new BoxConstraints({ minHeight: 56 }),
      child: padded(ctx, EdgeInsets.symmetric({ horizontal: 16, vertical: 8 }), content),
    }),
  );
};

/** `SkeletonLoader`: padded 16, an optional 40 px avatar and 16 gap, then `lines` blocks of 12 with 8 between. */
const skeletonLoader: WidgetBuilder = (node, ctx) => {
  const lines = Math.max(1, Math.trunc(num(node, "lines") ?? 3));
  const blocks: RenderBox[] = [];
  for (let i = 0; i < lines; i++) {
    if (i > 0) {
      blocks.push(box(0, 8));
    }
    blocks.push(new RenderConstrainedBox({ additionalConstraints: BoxConstraints.tightFor({ height: 12 }) }));
  }
  const children: RenderBox[] = node.props?.["avatar"] === true ? [box(40, 40), box(16, 0)] : [];
  children.push(expanded(stack(ctx, blocks, CrossAxisAlignment.start)));
  const content = new RenderFlex({
    direction: Axis.horizontal,
    mainAxisSize: MainAxisSize.max,
    crossAxisAlignment: CrossAxisAlignment.start,
    textDirection: direction(ctx),
    children,
  });
  return named(node, padded(ctx, EdgeInsets.all(16), content));
};

/** `SecureTextField`: an underlined `TextField` with its label and, when obscured, a 48 px reveal button. */
const secureTextField: WidgetBuilder = (node, ctx) => {
  const label = str(node, "label");
  const obscure = node.props?.["obscure"] !== false;
  const input = ctx.paragraph("A", role(ctx, "bodyLarge"));
  const field = buildInputDecorator({
    input: new RenderEditableLine(input),
    prefixIcon: null,
    suffixIcon: obscure ? decoratorIcon(box(48, 48), VisualDensity.standard) : null,
    subtext: null,
    outline: false,
    filled: false,
    isDense: false,
    hasLabel: label !== undefined,
    floating: (str(node, "value") ?? "") !== "",
    floatingLabelHeight: input.textScaler.scale(16),
    density: VisualDensity.standard,
    textDirection: direction(ctx),
  });
  return named(node, stack(ctx, [field]));
};

/** `SecurePinPad`: a row of `length` dots (26 each), 16 gap, then four rows of 80 by 60 keys. */
const securePinPad: WidgetBuilder = (node, ctx) => {
  const length = Math.trunc(num(node, "length") ?? 4);
  const dots = row(
    ctx,
    Array.from({ length: Math.max(0, length) }, () => box(26, 26)),
  );
  const keys = Array.from({ length: 4 }, () => row(ctx, [box(80, 60), box(80, 60), box(80, 60)]));
  return named(node, stack(ctx, [dots, box(0, 16), ...keys], CrossAxisAlignment.center));
};

/** `Sparkline`: 32 high and as wide as the space allows. */
const sparkline: WidgetBuilder = (node, ctx) =>
  named(
    node,
    new RenderConstrainedBox({
      additionalConstraints: new BoxConstraints({
        minWidth: ctx.available(),
        maxWidth: ctx.available(),
        minHeight: 32,
        maxHeight: 32,
      }),
    }),
  );

/** The Layer 2 builders of batch C, by widget type. */
export const layer2CWidgetBuilders: ReadonlyArray<readonly [string, WidgetBuilder]> = [
  ["QuantityStepper", quantityStepper],
  ["RatingBar", ratingBar],
  ["RatingInput", ratingInput],
  ["SearchField", searchField],
  ["SecurePinPad", securePinPad],
  ["SecureTextField", secureTextField],
  ["SettingsList", settingsList],
  ["SkeletonLoader", skeletonLoader],
  ["SortSheet", sortSheet],
  ["Sparkline", sparkline],
  ["StatusTimeline", statusTimeline],
  ["TransactionListItem", transactionListItem],
];
