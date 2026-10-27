// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Material composites whose size comes from their parts (STU-005, S4.6): the
 * bottom app bar, calendar date picker, data table, material banner,
 * navigation drawer, popup menu button, segmented button and vertical stepper.
 * Each is checked against Flutter's goldens in `material-bottom-app-bar`,
 * `material-calendar-date-picker`, `material-data-table`, `material-banner`,
 * `material-navigation-drawer`, `material-popup-menu-button`,
 * `material-segmented-button` and `material-stepper`.
 */

import { BoxConstraints } from "../flutter/constraints.ts";
import { Axis, CrossAxisAlignment, FlexFit, FlexParentData, MainAxisSize, RenderFlex } from "../flutter/flex.ts";
import { AlignmentDirectional, EdgeInsets, EdgeInsetsDirectional } from "../flutter/geometry.ts";
import { type Steb, m3TextStyle, scaledPadding, type TextRole } from "../flutter/material/theme.ts";
import { RenderOverflowBar } from "../flutter/overflow-bar.ts";
import { RenderConstrainedBox } from "../flutter/proxy-box.ts";
import type { RenderBox } from "../flutter/render-box.ts";
import { RenderPadding, RenderPositionedBox } from "../flutter/shifted-box.ts";
import { IntrinsicColumnWidth, RenderTable, TableCellVerticalAlignment } from "../flutter/table.ts";
import type { TextStyle } from "../flutter/text-style.ts";
import type { MaterialContext } from "./material-scene.ts";
import { padding, type SceneNode } from "./scene.ts";

type Builder = (node: SceneNode, ctx: MaterialContext) => RenderBox;

const family = "NotoSans";
const style = (role: TextRole): TextStyle => m3TextStyle(role, family);

function slotOf(node: SceneNode, name: string): SceneNode[] {
  return (node.children ?? []).filter((c) => (c.slot ?? "child") === name);
}

function num(node: SceneNode, name: string, fallback: number): number {
  const value = node.props[name];
  return typeof value === "number" ? value : fallback;
}

function records(value: unknown): Record<string, unknown>[] {
  return Array.isArray(value) ? value.filter((v): v is Record<string, unknown> => typeof v === "object" && v !== null) : [];
}

function textScale(ctx: MaterialContext): number {
  return ctx.text?.textScaler.textScaleFactor ?? 1;
}

/** A paragraph of `text` that no golden id names. */
function label(node: SceneNode, name: string, text: string, ctx: MaterialContext, base: TextStyle): RenderBox {
  const box = ctx.build({ id: `${node.id}:${name}`, type: "Text", props: { text } }, base);
  box.debugId = null;
  return box;
}

function column(children: RenderBox[], cross: CrossAxisAlignment): RenderFlex {
  return new RenderFlex({
    direction: Axis.vertical,
    mainAxisSize: MainAxisSize.min,
    crossAxisAlignment: cross,
    textDirection: null,
    children,
  });
}

function expanded(box: RenderBox): RenderBox {
  const data = new FlexParentData();
  data.flex = 1;
  data.fit = FlexFit.tight;
  box.parentData = data;
  return box;
}

/** `ButtonStyleButton.scaledPadding` of `[start, end]` at the 1x, 2x and 3x text scale. */
function scaledSides(scale: number, one: Steb, two: Steb, three: Steb): readonly [number, number] {
  const [start, , end] = scaledPadding(one, two, three, scale);
  return [start, end];
}

/** `BottomAppBar`: 80 high (M3), as wide as allowed, its child between 16 and 12 of padding, tight to the rest. */
const bottomAppBar: Builder = (node, ctx) => {
  const height = num(node, "height", 80);
  const child = slotOf(node, "child")[0];
  const inset = node.props["padding"] === undefined ? EdgeInsets.symmetric({ horizontal: 16, vertical: 12 }) : padding(node.props["padding"]);
  return new RenderConstrainedBox({
    additionalConstraints: new BoxConstraints({ minWidth: Number.POSITIVE_INFINITY, minHeight: height, maxHeight: height }),
    child:
      child === undefined ? null : new RenderPadding({ padding: inset, textDirection: ctx.direction, child: ctx.build(child) }),
  });
};

/** `CalendarDatePicker`: the sub-header and seven 48-px rows, taller once the text scale passes 1.3. */
const calendarDatePicker: Builder = (_node, ctx) => {
  const scale = Math.min(3, textScale(ctx));
  const grid = 48 * 7 + (scale > 1.3 ? 7 * ((scale - 1) * 8) : 0);
  return new RenderConstrainedBox({
    additionalConstraints: new BoxConstraints({ minWidth: Number.POSITIVE_INFINITY, minHeight: 52 + grid, maxHeight: 52 + grid }),
  });
};

/** `DataTable`: a table of intrinsic, flexing columns; the heading row is 56 high, a data row 48. */
const dataTable: Builder = (node, ctx) => {
  const columns = records(node.props["columns"]);
  const rows = Array.isArray(node.props["rows"]) ? (node.props["rows"] as unknown[]) : [];
  const margin = num(node, "horizontalMargin", 24);
  const spacing = num(node, "columnSpacing", 56);
  const headingHeight = num(node, "headingRowHeight", 56);
  const minHeight = num(node, "dataRowMinHeight", 48);
  const maxHeight = num(node, "dataRowMaxHeight", Math.max(48, minHeight));
  const cell = (text: string, index: number, box: BoxConstraints, base: TextStyle): RenderBox => {
    const start = index === 0 ? margin : spacing / 2;
    const end = index === columns.length - 1 ? margin : spacing / 2;
    return new RenderConstrainedBox({
      additionalConstraints: box,
      child: new RenderPadding({
        padding: EdgeInsetsDirectional.fromSTEB(start, 0, end, 0),
        textDirection: ctx.direction,
        child: label(node, `${index}:${text}`, text, ctx, base),
      }),
    });
  };
  const cells: RenderBox[] = columns.map((c, i) =>
    cell(String(c["label"] ?? ""), i, new BoxConstraints({ minHeight: headingHeight }), style("titleSmall")),
  );
  for (const row of rows) {
    const values = Array.isArray(row) ? (row as unknown[]) : [];
    columns.forEach((_c, i) => {
      cells.push(cell(String(values[i] ?? ""), i, new BoxConstraints({ minHeight, maxHeight }), style("bodyMedium")));
    });
  }
  return new RenderTable({
    columns: columns.length,
    rows: 1 + rows.length,
    children: cells,
    defaultColumnWidth: new IntrinsicColumnWidth(1),
    textDirection: ctx.direction,
    defaultVerticalAlignment: TableCellVerticalAlignment.middle,
  });
};

/** `MaterialBanner`: content, optional leading widget and an action bar beside it, or below it with several actions. */
const materialBanner: Builder = (node, ctx) => {
  const clamp = { maxScale: 1.5 };
  const actions = slotOf(node, "actions");
  if (actions.length === 0) {
    throw new Error(`node "${node.id}": a MaterialBanner needs an action`);
  }
  const below = actions.length > 1 || node.props["forceActionsBelow"] === true;
  const content = slotOf(node, "content")[0];
  const leading = slotOf(node, "leading")[0];
  if (content === undefined) {
    throw new Error(`node "${node.id}": a MaterialBanner needs a content child`);
  }
  const bar = (): RenderBox =>
    new RenderConstrainedBox({
      additionalConstraints: new BoxConstraints({ minHeight: num(node, "minActionBarHeight", 52) }),
      child: new RenderPadding({
        padding: EdgeInsets.symmetric({ horizontal: 8 }),
        textDirection: ctx.direction,
        child: new RenderPositionedBox({
          alignment: AlignmentDirectional.centerEnd,
          textDirection: ctx.direction,
          widthFactor: null,
          heightFactor: null,
          child: new RenderOverflowBar({
            children: actions.map((a) => ctx.build(a, undefined, below ? undefined : clamp)),
            spacing: 8,
            textDirection: ctx.direction,
          }),
        }),
      }),
    });
  const inset =
    node.props["padding"] !== undefined
      ? padding(node.props["padding"])
      : below
        ? EdgeInsetsDirectional.fromSTEB(16, 24, 16, 4)
        : EdgeInsetsDirectional.fromSTEB(16, 2, 0, 0);
  const row: RenderBox[] = [];
  if (leading !== undefined) {
    row.push(
      new RenderPadding({
        padding:
          node.props["leadingPadding"] !== undefined
            ? padding(node.props["leadingPadding"])
            : EdgeInsetsDirectional.only({ end: 16 }),
        textDirection: ctx.direction,
        child: ctx.build(leading, undefined, clamp),
      }),
    );
  }
  row.push(expanded(ctx.build(content, style("bodyMedium"), clamp)));
  if (!below) {
    row.push(bar());
  }
  const main = new RenderPadding({
    padding: inset,
    textDirection: ctx.direction,
    child: new RenderFlex({
      direction: Axis.horizontal,
      mainAxisSize: MainAxisSize.max,
      crossAxisAlignment: CrossAxisAlignment.center,
      textDirection: ctx.direction,
      children: row,
    }),
  });
  return column(below ? [main, bar()] : [main], CrossAxisAlignment.stretch);
};

/** `NavigationDrawer`: a drawer 304 wide holding its header, 56-px destinations and footer. */
const navigationDrawer: Builder = (node, ctx) => {
  const header = slotOf(node, "header")[0];
  const footer = slotOf(node, "footer")[0];
  const destinations = records(node.props["destinations"]).map(
    (_d, i) =>
      new RenderConstrainedBox({
        additionalConstraints: BoxConstraints.tightFor({ height: 56 }),
        child: label(node, `destination${i}`, "", ctx, style("labelLarge")),
      }),
  );
  const parts = [
    ...(header === undefined ? [] : [ctx.build(header)]),
    ...destinations,
    ...(footer === undefined ? [] : [ctx.build(footer)]),
  ];
  return new RenderConstrainedBox({
    additionalConstraints: new BoxConstraints({
      minWidth: num(node, "width", 304),
      maxWidth: num(node, "width", 304),
      minHeight: Number.POSITIVE_INFINITY,
    }),
    child: column(parts, CrossAxisAlignment.stretch),
  });
};

/** `PopupMenuButton`: its child, or a 48-px icon button. */
const popupMenuButton: Builder = (node, ctx) => {
  const child = slotOf(node, "child")[0];
  if (child !== undefined) {
    return new RenderPadding({ padding: EdgeInsets.zero, textDirection: ctx.direction, child: ctx.build(child) });
  }
  const side = Math.max(48, num(node, "iconSize", 24) + 16);
  return new RenderConstrainedBox({ additionalConstraints: BoxConstraints.tightFor({ width: side, height: side }) });
};

/** `SegmentedButton`: every segment as wide as the widest, 48 high with its tap target. */
const segmentedButton: Builder = (node, ctx) => {
  if (node.props["direction"] === "vertical") {
    throw new Error(`node "${node.id}": a vertical SegmentedButton has no port`);
  }
  const segments = records(node.props["segments"]);
  const selected = Array.isArray(node.props["selected"]) ? (node.props["selected"] as unknown[]) : [];
  const scale = textScale(ctx);
  const spacing = 8 + (4 - 8) * (Math.min(2, Math.max(1, scale)) - 1);
  const iconWidth = 18;
  let widest = 0;
  for (const segment of segments) {
    const text = typeof segment["label"] === "string" ? segment["label"] : null;
    const shown = selected.includes(segment["value"]) && node.props["showSelectedIcon"] !== false;
    const icon = shown || (text !== null && segment["icon"] !== undefined);
    const body = text === null ? iconWidth : label(node, "segment", text, ctx, style("labelLarge")).getMaxIntrinsicWidth(Number.POSITIVE_INFINITY);
    const [start, end] = icon
      ? scaledSides(scale, [12, 8, 16, 8], [4, 0, 4, 0], [4, 0, 4, 0])
      : scaledSides(scale, [12, 8, 12, 8], [8, 0, 8, 0], [4, 0, 4, 0]);
    widest = Math.max(widest, start + end + body + (icon && text !== null ? iconWidth + spacing : 0));
  }
  return new RenderConstrainedBox({
    additionalConstraints: BoxConstraints.tightFor({ width: widest * segments.length, height: 48 }),
  });
};

/** A vertical `Stepper`: 72-px step headers and, below the current step, its controls. */
const stepper: Builder = (node, ctx) => {
  if (node.props["type"] === "horizontal") {
    throw new Error(`node "${node.id}": a horizontal Stepper needs a bounded height`);
  }
  const steps = records(node.props["steps"]);
  const current = num(node, "currentStep", 0);
  const parts: RenderBox[] = [];
  steps.forEach((step, index) => {
    const lines: RenderBox[] = [label(node, `title${index}`, String(step["title"] ?? ""), ctx, style("bodyLarge"))];
    if (typeof step["subtitle"] === "string") {
      lines.push(
        new RenderPadding({
          padding: EdgeInsets.only({ top: 2 }),
          textDirection: ctx.direction,
          child: label(node, `subtitle${index}`, step["subtitle"], ctx, style("bodySmall")),
        }),
      );
    }
    parts.push(
      new RenderConstrainedBox({
        additionalConstraints: new BoxConstraints({ minHeight: 72 }),
        child: new RenderPositionedBox({
          alignment: AlignmentDirectional.centerStart,
          textDirection: ctx.direction,
          widthFactor: null,
          heightFactor: null,
          child: new RenderPadding({
            padding: EdgeInsetsDirectional.fromSTEB(60, 0, 24, 0),
            textDirection: ctx.direction,
            child: column(lines, CrossAxisAlignment.start),
          }),
        }),
      }),
    );
    if (index === current) {
      parts.push(
        new RenderPadding({
          padding: EdgeInsetsDirectional.fromSTEB(60, 0, 24, 24),
          textDirection: ctx.direction,
          child: new RenderConstrainedBox({ additionalConstraints: BoxConstraints.tightFor({ height: 64 }) }),
        }),
      );
    }
  });
  return new RenderConstrainedBox({
    additionalConstraints: new BoxConstraints({ minWidth: Number.POSITIVE_INFINITY }),
    child: column(parts, CrossAxisAlignment.stretch),
  });
};

/** The builders of this module, merged into the Material composites. */
export const moreBuilders: Readonly<Record<string, Builder>> = {
  BottomAppBar: bottomAppBar,
  CalendarDatePicker: calendarDatePicker,
  DataTable: dataTable,
  MaterialBanner: materialBanner,
  NavigationDrawer: navigationDrawer,
  PopupMenuButton: popupMenuButton,
  SegmentedButton: segmentedButton,
  Stepper: stepper,
};

