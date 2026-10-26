// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Layer 2 builders, batch D (STU-005, S4.5, WGT-020): the components of the optional widget packages (charts,
 * Markdown, media, maps, scanners), the wizard and the dual-calendar date picker. Each composes the render objects
 * the runtime's builder composes, so the layout agrees with the runtime's; the layout conformance suite checks them
 * against goldens the runtime writes. Components whose content is platform media lay out as their box only.
 */

import { BoxConstraints } from "../flutter/constraints.ts";
import { Axis, CrossAxisAlignment, FlexFit, FlexParentData, MainAxisSize, RenderFlex } from "../flutter/flex.ts";
import {
  Alignment,
  AlignmentDirectional,
  EdgeInsets,
  EdgeInsetsDirectional,
  TextDirection,
} from "../flutter/geometry.ts";
import { type ButtonKind, buildButton } from "../flutter/material/buttons.ts";
import { MaterialTapTargetSize, m3TextStyle, type TextRole, VisualDensity } from "../flutter/material/theme.ts";
import { RenderConstrainedBox } from "../flutter/proxy-box.ts";
import { RenderAspectRatio, RenderLimitedBox } from "../flutter/proxy-boxes.ts";
import type { RenderBox } from "../flutter/render-box.ts";
import { RenderPadding, RenderPositionedBox } from "../flutter/shifted-box.ts";
import type { TextStyle } from "../flutter/text-style.ts";
import { RenderWrap } from "../flutter/wrap.ts";
import {
  type CalendarSystem,
  calendarSystem,
  calendarSystemOf,
  calendarWeekday,
  eraSuffix,
  gregorianId,
  jdnOfIsoDate,
  monthName,
  secondaryTitle,
} from "./calendar.ts";
import type { WidgetBuilder, WidgetContext, WidgetNode } from "./screen.ts";

function text(node: WidgetNode, name: string): string | undefined {
  const value = node.props?.[name];
  return typeof value === "string" ? value : undefined;
}

function count(node: WidgetNode, name: string): number | undefined {
  const value = node.props?.[name];
  return typeof value === "number" ? value : undefined;
}

/** The struct items of a list prop. */
function structs(node: WidgetNode, name: string): Array<Record<string, unknown>> {
  const value = node.props?.[name];
  return Array.isArray(value)
    ? value.filter((v): v is Record<string, unknown> => typeof v === "object" && v !== null)
    : [];
}

function field(item: Record<string, unknown>, name: string): string {
  const value = item[name];
  return typeof value === "string" ? value : "";
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

function sized(width: number, height: number): RenderBox {
  return new RenderConstrainedBox({ additionalConstraints: BoxConstraints.tightFor({ width, height }) });
}

function gap(height: number): RenderBox {
  return new RenderConstrainedBox({ additionalConstraints: BoxConstraints.tightFor({ height }) });
}

function named(node: WidgetNode, box: RenderBox): RenderBox {
  box.debugId = node.id;
  return box;
}

function expanded(child: RenderBox): RenderBox {
  const parentData = new FlexParentData();
  parentData.flex = 1;
  parentData.fit = FlexFit.tight;
  child.parentData = parentData;
  return child;
}

function flexible(child: RenderBox): RenderBox {
  const parentData = new FlexParentData();
  parentData.flex = 1;
  parentData.fit = FlexFit.loose;
  child.parentData = parentData;
  return child;
}

function column(
  ctx: WidgetContext,
  children: RenderBox[],
  crossAxis = CrossAxisAlignment.stretch,
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

function row(ctx: WidgetContext, children: RenderBox[], size = MainAxisSize.max): RenderFlex {
  return new RenderFlex({
    direction: Axis.horizontal,
    mainAxisSize: size,
    crossAxisAlignment: CrossAxisAlignment.center,
    textDirection: direction(ctx),
    children,
  });
}

function padded(ctx: WidgetContext, padding: EdgeInsets | EdgeInsetsDirectional, child: RenderBox): RenderBox {
  return new RenderPadding({ padding, textDirection: direction(ctx), child });
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

/** One entry of a chart's key: a 10 px dot, 6 px, then the label in `bodySmall`. */
function keyEntry(ctx: WidgetContext, label: string): RenderBox {
  return row(
    ctx,
    [
      sized(10, 10),
      new RenderConstrainedBox({ additionalConstraints: BoxConstraints.tightFor({ width: 6 }) }),
      flexible(line(ctx, label, "bodySmall")),
    ],
    MainAxisSize.min,
  );
}

/** A chart's key: 8 below the drawing, its entries wrapping 16 apart and 4 between runs. */
function key(ctx: WidgetContext, labels: string[]): RenderBox {
  return padded(
    ctx,
    EdgeInsets.only({ top: 8 }),
    new RenderWrap({
      spacing: 16,
      runSpacing: 4,
      textDirection: direction(ctx),
      children: labels.map((label) => keyEntry(ctx, label)),
    }),
  );
}

/** A chart's title in `titleMedium`, 8 above the drawing; none without one. */
function chartTitle(node: WidgetNode, ctx: WidgetContext, name: string): RenderBox[] {
  const title = text(node, name);
  return title === undefined ? [] : [padded(ctx, EdgeInsets.only({ bottom: 8 }), line(ctx, title, "titleMedium"))];
}

/**
 * `LineChart`: the title, the drawing 220 high and, for several series, the key. The data table of `showTable` is
 * not laid out.
 */
const lineChart: WidgetBuilder = (node, ctx) => {
  const series = structs(node, "series");
  return named(
    node,
    column(ctx, [
      ...chartTitle(node, ctx, "title"),
      gap(220),
      ...(series.length > 1
        ? [
            key(
              ctx,
              series.map((s) => field(s, "name")),
            ),
          ]
        : []),
    ]),
  );
};

/** `PieChart`: the title, the pie 180 across and centred, then the key of the slices. `showTable` is not laid out. */
const pieChart: WidgetBuilder = (node, ctx) =>
  named(
    node,
    column(ctx, [
      ...chartTitle(node, ctx, "title"),
      new RenderPositionedBox({ alignment: Alignment.center, textDirection: direction(ctx), child: sized(180, 180) }),
      key(
        ctx,
        structs(node, "slices").map((s) => field(s, "label")),
      ),
    ]),
  );

const headingRoles: Readonly<Record<number, TextRole>> = {
  1: "headlineMedium",
  2: "headlineSmall",
  3: "titleLarge",
  4: "titleMedium",
  5: "titleSmall",
  6: "labelLarge",
};

/**
 * The blocks of `source` that Studio lays out: ATX headings and paragraphs (consecutive lines joined by a soft break),
 * separated by blank lines. Lists, quotes, code, rules and inline markup are not parsed; they lay out as the text
 * they are written in.
 */
function markdownBlocks(source: string): Array<{ role: TextRole; text: string }> {
  const blocks: Array<{ role: TextRole; text: string }> = [];
  let paragraph: string[] = [];
  const flush = (): void => {
    if (paragraph.length > 0) {
      blocks.push({ role: "bodyMedium", text: paragraph.join("\n") });
      paragraph = [];
    }
  };
  for (const raw of source.split(/\r\n|\r|\n/)) {
    const trimmed = raw.trim();
    const heading = /^(#{1,6})[ \t]+(.*?)(?:[ \t]+#+)?$/.exec(trimmed);
    if (trimmed === "") {
      flush();
    } else if (heading?.[1] !== undefined && heading[2] !== undefined) {
      flush();
      blocks.push({ role: headingRoles[heading[1].length] ?? "bodyMedium", text: heading[2] });
    } else {
      paragraph.push(trimmed);
    }
  }
  flush();
  return blocks;
}

/** The blocks of a Markdown source, 12 apart. */
function markdownColumn(ctx: WidgetContext, source: string): RenderFlex {
  return column(
    ctx,
    markdownBlocks(source).map((block, index) => {
      const paragraph = line(ctx, block.text, block.role);
      return index === 0 ? paragraph : padded(ctx, EdgeInsets.only({ top: 12 }), paragraph);
    }),
  );
}

/** `MarkdownView`: headings and paragraphs, 12 apart, across the width of the space. */
const markdownView: WidgetBuilder = (node, ctx) => named(node, markdownColumn(ctx, text(node, "source") ?? ""));

/**
 * A design-time stand-in for the 0.6 of the screen's height a `TermsViewer` takes when its parent gives it no height;
 * the screen theme carries the width of the device only.
 */
const unboundedTermsHeight = 480;

/**
 * `TermsViewer`: as high as its parent allows, with the scrolling text filling it, then (the end is not reached at
 * design time) the hint and the accept button, each 8 below.
 */
const termsViewer: WidgetBuilder = (node, ctx) =>
  named(
    node,
    new RenderLimitedBox({
      maxHeight: unboundedTermsHeight,
      child: column(
        ctx,
        [
          expanded(sized(0, 0)),
          padded(
            ctx,
            EdgeInsets.only({ top: 8 }),
            line(ctx, text(node, "scrollHint") ?? "Scroll to the end to accept", "bodySmall"),
          ),
          padded(ctx, EdgeInsets.only({ top: 8 }), button(ctx, "filled", text(node, "acceptLabel") ?? "Accept")),
        ],
        CrossAxisAlignment.stretch,
        MainAxisSize.max,
      ),
    }),
  );

/** The play, slider and position controls of a player, a row 48 high. */
function controls(ctx: WidgetContext): RenderBox {
  return row(ctx, [
    sized(48, 48),
    flexible(new RenderConstrainedBox({ additionalConstraints: new BoxConstraints() })),
    line(ctx, "0:00", "labelSmall", true),
  ]);
}

/** `VideoPlayer`: the surface at `aspectRatio` (16:9 by default, 0.2 to 5) over the controls. */
const videoPlayer: WidgetBuilder = (node, ctx) =>
  named(
    node,
    column(ctx, [
      new RenderAspectRatio({ aspectRatio: Math.min(5, Math.max(0.2, count(node, "aspectRatio") ?? 16 / 9)) }),
      controls(ctx),
    ]),
  );

/** `SignaturePad`: the pad (`height`, at least 80, 160 by default) over the clear button at the end. */
const signaturePad: WidgetBuilder = (node, ctx) =>
  named(
    node,
    column(ctx, [
      gap(Math.max(80, count(node, "height") ?? 160)),
      new RenderPositionedBox({
        alignment: AlignmentDirectional.centerEnd,
        textDirection: direction(ctx),
        child: button(ctx, "text", text(node, "clearLabel") ?? "Clear"),
      }),
    ]),
  );

/**
 * `LottieView` and `RiveView`: nothing until the animation has been fetched and decoded, and Studio fetches nothing,
 * so a design-time screen lays out the runtime's box before the load: zero.
 */
const animation: WidgetBuilder = (node) => named(node, sized(0, 0));

/**
 * `MapView` and `OrderTrackerMap`: the map as wide as the space and, with no height from the parent, three quarters of
 * that high. Its markers, route and the ETA badge sit on the map and take no space. The canvas draws no map, so the
 * box is marked as a labelled placeholder.
 */
const map: WidgetBuilder = (node, ctx) => {
  ctx.mark(node.id, "placeholder", node.type);
  return named(node, sized(ctx.available(), ctx.available() * 0.75));
};

/**
 * `QrCodeView`: a square of `size` (200 by default, at least 16). No runtime golden is possible for it (the scanner
 * package cannot be built into the golden writer), so its box is checked by a unit test, not by a conformance scene.
 */
const qrCodeView: WidgetBuilder = (node) => {
  const size = Math.max(16, count(node, "size") ?? 200);
  return named(node, sized(size, size));
};

/**
 * `QrScanner` and `BarcodeScanner`: the camera view fills its parent, so a design-time screen, which has no camera,
 * lays out a square as wide as the space. Checked as a box only, by a unit test (see `QrCodeView`).
 */
const scanner: WidgetBuilder = (node, ctx) => {
  ctx.mark(node.id, "placeholder", node.type);
  return named(node, sized(ctx.available(), ctx.available()));
};

/** `WizardForm`: the progress, the shown step and the buttons: Back (after the first step), then Next or Submit. */
const wizardForm: WidgetBuilder = (node, ctx) => {
  const steps = node.children ?? [];
  if (steps.length === 0) {
    return named(node, sized(0, 0));
  }
  const step = Math.min(Math.max(0, Math.trunc(count(node, "currentStep") ?? 0)), steps.length - 1);
  const titles = node.props?.["titles"];
  const title = Array.isArray(titles) && typeof titles[step] === "string" ? (titles[step] as string) : undefined;
  const progress = new RenderConstrainedBox({
    additionalConstraints: new BoxConstraints({
      minWidth: Number.POSITIVE_INFINITY,
      maxWidth: Number.POSITIVE_INFINITY,
      minHeight: 4,
      maxHeight: Number.POSITIVE_INFINITY,
    }),
  });
  const last = step === steps.length - 1;
  const buttons: RenderBox[] = [];
  if (step > 0) {
    buttons.push(button(ctx, "outlined", text(node, "backLabel") ?? "Back"));
  }
  buttons.push(
    expanded(sized(0, 0)),
    last
      ? button(ctx, "filled", text(node, "submitLabel") ?? "Submit")
      : button(ctx, "filled", text(node, "nextLabel") ?? "Next"),
  );
  const shown = steps[step];
  return named(
    node,
    column(ctx, [
      padded(
        ctx,
        EdgeInsetsDirectional.fromSTEB(16, 8, 16, 8),
        column(
          ctx,
          [
            progress,
            gap(8),
            line(ctx, `${step + 1} / ${steps.length}${title === undefined ? "" : `  ${title}`}`, "titleMedium"),
          ],
          CrossAxisAlignment.start,
        ),
      ),
      ...(shown === undefined ? [] : [ctx.build(shown)]),
      padded(ctx, EdgeInsets.all(16), row(ctx, buttons)),
    ]),
  );
};

/** The default bounds of the picker when `firstDate` or `lastDate` is absent: 1900-01-01 and 2100-12-31. */
const defaultFirst = jdnOfIsoDate("1900-01-01") ?? 0;

/** A day's cell: at least 48 square, the day number in `bodyMedium` and, with a secondary calendar, its number below. */
function dayCell(
  ctx: WidgetContext,
  primary: CalendarSystem,
  secondary: CalendarSystem | undefined,
  jdn: number,
): RenderBox {
  const lines = [line(ctx, `${primary.fromJdn(jdn).day}`, "bodyMedium")];
  if (secondary !== undefined) {
    lines.push(line(ctx, `${secondary.fromJdn(jdn).day}`, "labelSmall"));
  }
  return new RenderConstrainedBox({
    additionalConstraints: new BoxConstraints({ minWidth: 48, minHeight: 48 }),
    child: new RenderPositionedBox({
      alignment: Alignment.center,
      textDirection: direction(ctx),
      child: column(ctx, lines, CrossAxisAlignment.center),
    }),
  });
}

/**
 * `DualCalendarDatePicker`: the month of `value` in `calendar` as a grid of 48 px cells (Sunday first, the runtime's
 * English locale data), each with the day number of `secondaryCalendar` below, under a header of two 48 px buttons
 * around the month's title and the secondary calendar's span. A picker without a `value` opens on `firstDate`'s
 * month: a design-time screen has no clock to open it on today's.
 */
const dualCalendarDatePicker: WidgetBuilder = (node, ctx) => {
  const primary = calendarSystemOf(node.props?.["calendar"]) ?? calendarSystem(gregorianId);
  if (primary === undefined) {
    return named(node, sized(0, 0));
  }
  const secondary = calendarSystemOf(node.props?.["secondaryCalendar"]);
  const first = jdnOfIsoDate(node.props?.["firstDate"]) ?? defaultFirst;
  const opening = jdnOfIsoDate(node.props?.["value"]) ?? first;
  const shown = primary.fromJdn(opening);
  const start = primary.toJdn({ year: shown.year, month: shown.month, day: 1 });
  const days = primary.daysInMonth(shown.year, shown.month);
  const lead = (calendarWeekday(start) + 7) % 7;
  const rows = Math.trunc((lead + days + 6) / 7);
  const titleLines = [
    line(
      ctx,
      `${monthName(primary, shown.month)} ${primary.eraOf(shown.year).year}${eraSuffix(primary, shown.year)}`,
      "titleMedium",
    ),
  ];
  if (secondary !== undefined) {
    titleLines.push(line(ctx, secondaryTitle(secondary, start, start + days - 1), "bodySmall"));
  }
  const cells = (r: number): RenderBox[] =>
    Array.from({ length: 7 }, (_, i) => {
      const index = r * 7 + i - lead;
      return expanded(index < 0 || index >= days ? sized(0, 0) : dayCell(ctx, primary, secondary, start + index));
    });
  return named(
    node,
    column(ctx, [
      row(ctx, [sized(48, 48), expanded(column(ctx, titleLines, CrossAxisAlignment.center)), sized(48, 48)]),
      row(
        ctx,
        Array.from({ length: 7 }, () => expanded(line(ctx, "S", "labelMedium"))),
      ),
      ...Array.from({ length: rows }, (_, r) => row(ctx, cells(r))),
    ]),
  );
};

/** The batch D builders, by widget type. */
export const layer2DWidgetBuilders: ReadonlyArray<readonly [string, WidgetBuilder]> = [
  ["BarcodeScanner", scanner],
  ["DualCalendarDatePicker", dualCalendarDatePicker],
  ["LineChart", lineChart],
  ["LottieView", animation],
  ["MapView", map],
  ["MarkdownView", markdownView],
  ["OrderTrackerMap", map],
  ["PieChart", pieChart],
  ["QrCodeView", qrCodeView],
  ["QrScanner", scanner],
  ["RiveView", animation],
  ["SignaturePad", signaturePad],
  ["TermsViewer", termsViewer],
  ["VideoPlayer", videoPlayer],
  ["WizardForm", wizardForm],
];
