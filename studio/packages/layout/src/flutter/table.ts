// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/rendering/table.dart
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/**
 * Table layout: `RenderTable` and the column width specifications
 * (`FixedColumnWidth`, `FractionColumnWidth`, `FlexColumnWidth`,
 * `IntrinsicColumnWidth`, `MaxColumnWidth`, `MinColumnWidth`).
 *
 * Borders, row decorations and painting are not ported; they do not affect
 * layout. Children are stored row-major in a flat list with `null` for empty
 * cells, as Flutter does.
 */

import { BoxConstraints } from "./constraints.ts";
import { Offset, precisionErrorTolerance, Size, TextDirection } from "./geometry.ts";
import type { PipelineOwner, RenderObject } from "./object.ts";
import { BoxParentData, RenderBox, type TextBaseline } from "./render-box.ts";

/** How a table cell is placed vertically within its row. */
export enum TableCellVerticalAlignment {
  top = "top",
  middle = "middle",
  bottom = "bottom",
  baseline = "baseline",
  fill = "fill",
  intrinsicHeight = "intrinsicHeight",
}

/** Parent data of a table cell: its optional vertical alignment and grid position. */
export class TableCellParentData extends BoxParentData {
  verticalAlignment: TableCellVerticalAlignment | null = null;
  x: number | null = null;
  y: number | null = null;
}

/** How wide a column is, as a function of its cells and the table's width. */
export abstract class TableColumnWidth {
  abstract minIntrinsicWidth(cells: readonly RenderBox[], containerWidth: number): number;
  abstract maxIntrinsicWidth(cells: readonly RenderBox[], containerWidth: number): number;
  /** The flex factor, or null for a column that is not flexible. */
  flex(_cells: readonly RenderBox[]): number | null {
    return null;
  }
}

/** The width of the widest cell; optionally also flexible. */
export class IntrinsicColumnWidth extends TableColumnWidth {
  constructor(private readonly _flex: number | null = null) {
    super();
  }

  minIntrinsicWidth(cells: readonly RenderBox[]): number {
    return cells.reduce((result, cell) => Math.max(result, cell.getMinIntrinsicWidth(Number.POSITIVE_INFINITY)), 0);
  }

  maxIntrinsicWidth(cells: readonly RenderBox[]): number {
    return cells.reduce((result, cell) => Math.max(result, cell.getMaxIntrinsicWidth(Number.POSITIVE_INFINITY)), 0);
  }

  override flex(): number | null {
    return this._flex;
  }
}

/** A fixed width in logical pixels. */
export class FixedColumnWidth extends TableColumnWidth {
  constructor(readonly value: number) {
    super();
  }

  minIntrinsicWidth(): number {
    return this.value;
  }

  maxIntrinsicWidth(): number {
    return this.value;
  }
}

/** A fraction of the table's maximum width (zero while that is unbounded). */
export class FractionColumnWidth extends TableColumnWidth {
  constructor(readonly value: number) {
    super();
  }

  minIntrinsicWidth(_cells: readonly RenderBox[], containerWidth: number): number {
    return Number.isFinite(containerWidth) ? this.value * containerWidth : 0;
  }

  maxIntrinsicWidth(_cells: readonly RenderBox[], containerWidth: number): number {
    return Number.isFinite(containerWidth) ? this.value * containerWidth : 0;
  }
}

/** A share of the free space, in proportion to `value`. */
export class FlexColumnWidth extends TableColumnWidth {
  constructor(readonly value: number = 1) {
    super();
  }

  minIntrinsicWidth(): number {
    return 0;
  }

  maxIntrinsicWidth(): number {
    return 0;
  }

  override flex(): number {
    return this.value;
  }
}

/** The larger of two column widths. */
export class MaxColumnWidth extends TableColumnWidth {
  constructor(
    readonly a: TableColumnWidth,
    readonly b: TableColumnWidth,
  ) {
    super();
  }

  minIntrinsicWidth(cells: readonly RenderBox[], containerWidth: number): number {
    return Math.max(this.a.minIntrinsicWidth(cells, containerWidth), this.b.minIntrinsicWidth(cells, containerWidth));
  }

  maxIntrinsicWidth(cells: readonly RenderBox[], containerWidth: number): number {
    return Math.max(this.a.maxIntrinsicWidth(cells, containerWidth), this.b.maxIntrinsicWidth(cells, containerWidth));
  }

  override flex(cells: readonly RenderBox[]): number | null {
    const aFlex = this.a.flex(cells);
    const bFlex = this.b.flex(cells);
    if (aFlex === null) {
      return bFlex;
    }
    if (bFlex === null) {
      return aFlex;
    }
    return Math.max(aFlex, bFlex);
  }
}

/** The smaller of two column widths. */
export class MinColumnWidth extends TableColumnWidth {
  constructor(
    readonly a: TableColumnWidth,
    readonly b: TableColumnWidth,
  ) {
    super();
  }

  minIntrinsicWidth(cells: readonly RenderBox[], containerWidth: number): number {
    return Math.min(this.a.minIntrinsicWidth(cells, containerWidth), this.b.minIntrinsicWidth(cells, containerWidth));
  }

  maxIntrinsicWidth(cells: readonly RenderBox[], containerWidth: number): number {
    return Math.min(this.a.maxIntrinsicWidth(cells, containerWidth), this.b.maxIntrinsicWidth(cells, containerWidth));
  }

  override flex(cells: readonly RenderBox[]): number | null {
    const aFlex = this.a.flex(cells);
    const bFlex = this.b.flex(cells);
    if (aFlex === null) {
      return bFlex;
    }
    if (bFlex === null) {
      return aFlex;
    }
    return Math.min(aFlex, bFlex);
  }
}

/** Options of [RenderTable]. */
export interface RenderTableOptions {
  columns: number;
  rows: number;
  /** Row-major cells, `rows * columns` long; `null` for an empty cell. */
  children?: ReadonlyArray<RenderBox | null>;
  columnWidths?: ReadonlyMap<number, TableColumnWidth>;
  defaultColumnWidth?: TableColumnWidth;
  textDirection: TextDirection;
  defaultVerticalAlignment?: TableCellVerticalAlignment;
  textBaseline?: TextBaseline | null;
}

/** A grid of cells whose columns are sized by [TableColumnWidth]s (`Table`). */
export class RenderTable extends RenderBox {
  private readonly _columns: number;
  private readonly _rows: number;
  private readonly _cells: Array<RenderBox | null> = [];
  private _columnWidths: ReadonlyMap<number, TableColumnWidth>;
  private _defaultColumnWidth: TableColumnWidth;
  private _textDirection: TextDirection;
  private _defaultVerticalAlignment: TableCellVerticalAlignment;
  private _textBaseline: TextBaseline | null;
  private _baselineDistance: number | null = null;
  private _rowTops: number[] = [];

  constructor(options: RenderTableOptions) {
    super();
    this._columns = options.columns;
    this._rows = options.rows;
    this._columnWidths = options.columnWidths ?? new Map();
    this._defaultColumnWidth = options.defaultColumnWidth ?? new FlexColumnWidth();
    this._textDirection = options.textDirection;
    this._defaultVerticalAlignment = options.defaultVerticalAlignment ?? TableCellVerticalAlignment.top;
    this._textBaseline = options.textBaseline ?? null;
    const children = options.children ?? [];
    if (children.length !== this._rows * this._columns) {
      throw new Error(`RenderTable needs rows * columns = ${this._rows * this._columns} cells, got ${children.length}`);
    }
    for (const child of children) {
      this._cells.push(child);
      if (child !== null) {
        this.adoptChild(child);
      }
    }
  }

  get columns(): number {
    return this._columns;
  }

  get rows(): number {
    return this._rows;
  }

  get defaultColumnWidth(): TableColumnWidth {
    return this._defaultColumnWidth;
  }

  set defaultColumnWidth(value: TableColumnWidth) {
    this._defaultColumnWidth = value;
    this.markNeedsLayout();
  }

  get columnWidths(): ReadonlyMap<number, TableColumnWidth> {
    return this._columnWidths;
  }

  set columnWidths(value: ReadonlyMap<number, TableColumnWidth>) {
    this._columnWidths = value;
    this.markNeedsLayout();
  }

  get textDirection(): TextDirection {
    return this._textDirection;
  }

  set textDirection(value: TextDirection) {
    if (this._textDirection !== value) {
      this._textDirection = value;
      this.markNeedsLayout();
    }
  }

  get defaultVerticalAlignment(): TableCellVerticalAlignment {
    return this._defaultVerticalAlignment;
  }

  set defaultVerticalAlignment(value: TableCellVerticalAlignment) {
    if (this._defaultVerticalAlignment !== value) {
      this._defaultVerticalAlignment = value;
      this.markNeedsLayout();
    }
  }

  get textBaseline(): TextBaseline | null {
    return this._textBaseline;
  }

  set textBaseline(value: TextBaseline | null) {
    if (this._textBaseline !== value) {
      this._textBaseline = value;
      this.markNeedsLayout();
    }
  }

  /** The top of every row plus the bottom of the last, after layout. */
  get rowTops(): readonly number[] {
    return this._rowTops;
  }

  override setupParentData(child: RenderObject): void {
    if (!(child.parentData instanceof TableCellParentData)) {
      child.parentData = new TableCellParentData();
    }
  }

  override attach(owner: PipelineOwner): void {
    super.attach(owner);
    for (const child of this._cells) {
      child?.attach(owner);
    }
  }

  override detach(): void {
    super.detach();
    for (const child of this._cells) {
      child?.detach();
    }
  }

  override visitChildren(visitor: (child: RenderObject) => void): void {
    for (const child of this._cells) {
      if (child !== null) {
        visitor(child);
      }
    }
  }

  /** The non-empty cells of column `x`, top to bottom. */
  column(x: number): RenderBox[] {
    const result: RenderBox[] = [];
    for (let y = 0; y < this._rows; y += 1) {
      const child = this._cells[x + y * this._columns];
      if (child !== null && child !== undefined) {
        result.push(child);
      }
    }
    return result;
  }

  private _widthOf(x: number): TableColumnWidth {
    return this._columnWidths.get(x) ?? this._defaultColumnWidth;
  }

  private _alignmentOf(child: RenderBox): TableCellVerticalAlignment {
    const data = child.parentData;
    return (data instanceof TableCellParentData ? data.verticalAlignment : null) ?? this._defaultVerticalAlignment;
  }

  private get _isEmpty(): boolean {
    return this._rows * this._columns === 0;
  }

  protected override computeMinIntrinsicWidth(_height: number): number {
    if (this._isEmpty) {
      return 0;
    }
    let total = 0;
    for (let x = 0; x < this._columns; x += 1) {
      total += this._widthOf(x).minIntrinsicWidth(this.column(x), Number.POSITIVE_INFINITY);
    }
    return total;
  }

  protected override computeMaxIntrinsicWidth(_height: number): number {
    if (this._isEmpty) {
      return 0;
    }
    let total = 0;
    for (let x = 0; x < this._columns; x += 1) {
      total += this._widthOf(x).maxIntrinsicWidth(this.column(x), Number.POSITIVE_INFINITY);
    }
    return total;
  }

  protected override computeMinIntrinsicHeight(width: number): number {
    if (this._isEmpty) {
      return 0;
    }
    const widths = this.computeColumnWidths(BoxConstraints.tightForFinite({ width }));
    let rowTop = 0;
    for (let y = 0; y < this._rows; y += 1) {
      let rowHeight = 0;
      for (let x = 0; x < this._columns; x += 1) {
        const child = this._cells[x + y * this._columns];
        if (child !== null && child !== undefined) {
          rowHeight = Math.max(rowHeight, child.getMaxIntrinsicHeight(widths[x] ?? 0));
        }
      }
      rowTop += rowHeight;
    }
    return rowTop;
  }

  protected override computeMaxIntrinsicHeight(width: number): number {
    return this.getMinIntrinsicHeight(width);
  }

  protected override computeDistanceToActualBaseline(_baseline: TextBaseline): number | null {
    return this._baselineDistance;
  }

  /**
   * The width of every column under `constraints` (`_computeColumnWidths`):
   * ideal widths first, then flex columns grow to the maximum width (or the
   * minimum while unbounded), then non-flex columns grow to the minimum, then
   * columns shrink to the maximum without going below their minimum widths.
   */
  computeColumnWidths(constraints: BoxConstraints): number[] {
    const state = this._idealWidths(constraints.maxWidth);
    growColumns(state, constraints);
    if (state.tableWidth > constraints.maxWidth) {
      shrinkColumns(state, state.tableWidth - constraints.maxWidth);
    }
    return state.widths;
  }

  /** Step 1: every column at its ideal (maximum intrinsic) width, with its minimum width and flex. */
  private _idealWidths(containerWidth: number): ColumnWidthState {
    const columns = this._columns;
    const state: ColumnWidthState = {
      widths: new Array<number>(columns).fill(0),
      minWidths: new Array<number>(columns).fill(0),
      flexes: new Array<number | null>(columns).fill(null),
      tableWidth: 0,
      unflexedTableWidth: 0,
      totalFlex: 0,
    };
    for (let x = 0; x < columns; x += 1) {
      const columnWidth = this._widthOf(x);
      const cells = this.column(x);
      const maxIntrinsic = columnWidth.maxIntrinsicWidth(cells, containerWidth);
      state.widths[x] = maxIntrinsic;
      state.tableWidth += maxIntrinsic;
      state.minWidths[x] = columnWidth.minIntrinsicWidth(cells, containerWidth);
      const flex = columnWidth.flex(cells);
      if (flex === null) {
        state.unflexedTableWidth += maxIntrinsic;
      } else {
        state.flexes[x] = flex;
        state.totalFlex += flex;
      }
    }
    return state;
  }

  protected override computeDryBaseline(constraints: BoxConstraints, baseline: TextBaseline): number | null {
    if (this._isEmpty) {
      return null;
    }
    const widths = this.computeColumnWidths(constraints);
    let baselineOffset: number | null = null;
    for (let col = 0; col < this._columns; col += 1) {
      const child = this._cells[col];
      if (child === null || child === undefined) {
        continue;
      }
      const childBaseline =
        this._alignmentOf(child) === TableCellVerticalAlignment.baseline
          ? child.getDryBaseline(BoxConstraints.tightFor({ width: widths[col] ?? 0 }), baseline)
          : null;
      if (childBaseline !== null && (baselineOffset === null || baselineOffset < childBaseline)) {
        baselineOffset = childBaseline;
      }
    }
    return baselineOffset;
  }

  protected override computeDryLayout(constraints: BoxConstraints): Size {
    if (this._isEmpty) {
      return constraints.constrain(Size.zero);
    }
    const widths = this.computeColumnWidths(constraints);
    const tableWidth = widths.reduce((a, b) => a + b, 0);
    let rowTop = 0;
    for (let y = 0; y < this._rows; y += 1) {
      let rowHeight = 0;
      for (let x = 0; x < this._columns; x += 1) {
        const child = this._cells[x + y * this._columns];
        if (child === null || child === undefined) {
          continue;
        }
        switch (this._alignmentOf(child)) {
          case TableCellVerticalAlignment.baseline:
            // Baseline metrics need a full layout; Flutter returns zero (release).
            return Size.zero;
          case TableCellVerticalAlignment.fill:
            break;
          default:
            rowHeight = Math.max(
              rowHeight,
              child.getDryLayout(BoxConstraints.tightFor({ width: widths[x] ?? 0 })).height,
            );
        }
      }
      rowTop += rowHeight;
    }
    return constraints.constrain(new Size(tableWidth, rowTop));
  }

  /** The left edge of every column, honouring the text direction. */
  private _columnPositions(widths: readonly number[]): number[] {
    const columns = this._columns;
    const positions: number[] = new Array<number>(columns).fill(0);
    if (this._textDirection === TextDirection.rtl) {
      for (let x = columns - 2; x >= 0; x -= 1) {
        positions[x] = (positions[x + 1] ?? 0) + (widths[x + 1] ?? 0);
      }
    } else {
      for (let x = 1; x < columns; x += 1) {
        positions[x] = (positions[x - 1] ?? 0) + (widths[x - 1] ?? 0);
      }
    }
    return positions;
  }

  /** Lays out the cells of row `y` that size the row and returns the row's measurements. */
  private _measureRow(y: number, widths: readonly number[], positions: readonly number[], rowTop: number): RowMetrics {
    const metrics: RowMetrics = {
      height: 0,
      haveBaseline: false,
      before: 0,
      after: 0,
      baselines: new Array<number>(this._columns).fill(0),
    };
    for (let x = 0; x < this._columns; x += 1) {
      const child = this._cells[x + y * this._columns];
      if (child !== null && child !== undefined) {
        this._measureCell(child, { x, y, width: widths[x] ?? 0, left: positions[x] ?? 0, rowTop }, metrics);
      }
    }
    return metrics;
  }

  private _measureCell(
    child: RenderBox,
    at: { x: number; y: number; width: number; left: number; rowTop: number },
    metrics: RowMetrics,
  ): void {
    const data = child.parentData as TableCellParentData;
    data.x = at.x;
    data.y = at.y;
    const alignment = this._alignmentOf(child);
    if (alignment === TableCellVerticalAlignment.fill) {
      return;
    }
    child.layout(BoxConstraints.tightFor({ width: at.width }), { parentUsesSize: true });
    const isBaseline = alignment === TableCellVerticalAlignment.baseline;
    const childBaseline = isBaseline ? this._cellBaseline(child) : null;
    if (childBaseline === null) {
      metrics.height = Math.max(metrics.height, child.size.height);
      if (isBaseline) {
        data.offset = new Offset(at.left, at.rowTop);
      }
      return;
    }
    metrics.before = Math.max(metrics.before, childBaseline);
    metrics.after = Math.max(metrics.after, child.size.height - childBaseline);
    metrics.baselines[at.x] = childBaseline;
    metrics.haveBaseline = true;
  }

  private _cellBaseline(child: RenderBox): number | null {
    if (this._textBaseline === null) {
      throw new Error("An explicit textBaseline is required when using baseline alignment.");
    }
    return child.getDistanceToBaseline(this._textBaseline, { onlyReal: true });
  }

  /** Places the cells of row `y` once its height is known. */
  private _placeRow(
    y: number,
    widths: readonly number[],
    positions: readonly number[],
    rowTop: number,
    m: RowMetrics,
  ): void {
    for (let x = 0; x < this._columns; x += 1) {
      const child = this._cells[x + y * this._columns];
      if (child === null || child === undefined) {
        continue;
      }
      const data = child.parentData as TableCellParentData;
      const left = positions[x] ?? 0;
      switch (this._alignmentOf(child)) {
        case TableCellVerticalAlignment.baseline:
          data.offset = new Offset(left, rowTop + m.before - (m.baselines[x] ?? 0));
          break;
        case TableCellVerticalAlignment.top:
          data.offset = new Offset(left, rowTop);
          break;
        case TableCellVerticalAlignment.middle:
          data.offset = new Offset(left, rowTop + (m.height - child.size.height) / 2);
          break;
        case TableCellVerticalAlignment.bottom:
          data.offset = new Offset(left, rowTop + m.height - child.size.height);
          break;
        case TableCellVerticalAlignment.fill:
        case TableCellVerticalAlignment.intrinsicHeight:
          child.layout(BoxConstraints.tightFor({ width: widths[x] ?? 0, height: m.height }));
          data.offset = new Offset(left, rowTop);
          break;
      }
    }
  }

  protected override performLayout(): void {
    const constraints = this.constraints;
    if (this._isEmpty) {
      this.size = constraints.constrain(Size.zero);
      return;
    }
    const widths = this.computeColumnWidths(constraints);
    const positions = this._columnPositions(widths);
    const tableWidth = widths.reduce((a, b) => a + b, 0);
    this._rowTops = [];
    this._baselineDistance = null;
    let rowTop = 0;
    for (let y = 0; y < this._rows; y += 1) {
      this._rowTops.push(rowTop);
      const metrics = this._measureRow(y, widths, positions, rowTop);
      if (metrics.haveBaseline) {
        if (y === 0) {
          this._baselineDistance = metrics.before;
        }
        metrics.height = Math.max(metrics.height, metrics.before + metrics.after);
      }
      this._placeRow(y, widths, positions, rowTop, metrics);
      rowTop += metrics.height;
    }
    this._rowTops.push(rowTop);
    this.size = constraints.constrain(new Size(tableWidth, rowTop));
  }
}

/** The running state of [RenderTable.computeColumnWidths]. */
interface ColumnWidthState {
  widths: number[];
  minWidths: number[];
  flexes: Array<number | null>;
  tableWidth: number;
  unflexedTableWidth: number;
  totalFlex: number;
}

/** The measurements of one table row while its cells are laid out. */
interface RowMetrics {
  height: number;
  haveBaseline: boolean;
  before: number;
  after: number;
  baselines: number[];
}

/** Steps 2 and 3: grow flex columns to the target width, or all columns to the minimum width. */
function growColumns(state: ColumnWidthState, constraints: BoxConstraints): void {
  if (state.totalFlex > 0) {
    const targetWidth = Number.isFinite(constraints.maxWidth) ? constraints.maxWidth : constraints.minWidth;
    if (state.tableWidth < targetWidth) {
      growFlexColumns(state, targetWidth - state.unflexedTableWidth);
    }
  } else if (state.tableWidth < constraints.minWidth) {
    const delta = (constraints.minWidth - state.tableWidth) / state.widths.length;
    state.widths = state.widths.map((width) => width + delta);
    state.tableWidth = constraints.minWidth;
  }
}

/** Gives each flex column its share of `remainingWidth` if that is more than its ideal width. */
function growFlexColumns(state: ColumnWidthState, remainingWidth: number): void {
  for (let x = 0; x < state.widths.length; x += 1) {
    const flex = state.flexes[x] ?? null;
    const width = state.widths[x] ?? 0;
    const flexedWidth = flex === null ? 0 : (remainingWidth * flex) / state.totalFlex;
    if (flex !== null && width < flexedWidth) {
      state.tableWidth += flexedWidth - width;
      state.widths[x] = flexedWidth;
    }
  }
}

/** How much is left to take out of the columns, and how many can still shrink. */
interface Shrink {
  deficit: number;
  available: number;
}

/**
 * Step 4: take the deficit out of the columns. Flexible columns shrink
 * proportionally until none is left to shrink, then the others equally,
 * never below their minimum widths.
 */
function shrinkColumns(state: ColumnWidthState, deficit: number): void {
  const shrink: Shrink = { deficit, available: state.widths.length };
  while (shrink.deficit > precisionErrorTolerance && state.totalFlex > precisionErrorTolerance) {
    shrinkFlexPass(state, shrink);
  }
  while (shrink.deficit > precisionErrorTolerance && shrink.available > 0) {
    shrinkEvenPass(state, shrink);
  }
}

/** One proportional pass over the flex columns. */
function shrinkFlexPass(state: ColumnWidthState, shrink: Shrink): void {
  let newTotalFlex = 0;
  for (let x = 0; x < state.widths.length; x += 1) {
    const flex = state.flexes[x] ?? null;
    if (flex === null) {
      continue;
    }
    const width = state.widths[x] ?? 0;
    const minWidth = state.minWidths[x] ?? 0;
    const newWidth = width - (shrink.deficit * flex) / state.totalFlex;
    if (newWidth <= minWidth) {
      shrink.deficit -= width - minWidth;
      state.widths[x] = minWidth;
      state.flexes[x] = null;
      shrink.available -= 1;
    } else {
      shrink.deficit -= width - newWidth;
      state.widths[x] = newWidth;
      newTotalFlex += flex;
    }
  }
  state.totalFlex = newTotalFlex;
}

/** One pass taking an equal share of the deficit out of every column above its minimum. */
function shrinkEvenPass(state: ColumnWidthState, shrink: Shrink): void {
  const delta = shrink.deficit / shrink.available;
  let stillAvailable = 0;
  for (let x = 0; x < state.widths.length; x += 1) {
    const width = state.widths[x] ?? 0;
    const minWidth = state.minWidths[x] ?? 0;
    const availableDelta = width - minWidth;
    if (availableDelta <= 0) {
      continue;
    }
    if (availableDelta <= delta) {
      shrink.deficit -= availableDelta;
      state.widths[x] = minWidth;
    } else {
      shrink.deficit -= delta;
      state.widths[x] = width - delta;
      stillAvailable += 1;
    }
  }
  shrink.available = stillAvailable;
}
