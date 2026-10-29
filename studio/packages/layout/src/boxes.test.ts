// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// S4.2: FittedBox math, table column-width resolution and the other box
// widgets, against values worked out from Flutter's sources (the Flutter
// goldens in conformance/ cover the same classes end to end).

import { describe, expect, test } from "bun:test";
import { buildContainer, buildSizeTransition, lerpAlignment, lerpBoxConstraints, lerpProperty } from "./animated.ts";
import { applyBoxFit, BoxFit } from "./flutter/box-fit.ts";
import { BoxConstraints } from "./flutter/constraints.ts";
import { Axis } from "./flutter/flex.ts";
import { Alignment, AlignmentDirectional, EdgeInsets, Offset, Rect, Size, TextDirection } from "./flutter/geometry.ts";
import { Affine } from "./flutter/matrix.ts";
import {
  applyStep,
  ClipShape,
  RenderAspectRatio,
  RenderClip,
  RenderFittedBox,
  RenderFractionalTranslation,
  RenderIntrinsicHeight,
  RenderIntrinsicWidth,
  RenderLimitedBox,
  RenderTransform,
} from "./flutter/proxy-boxes.ts";
import { type BoxParentData, type RenderBox, TextBaseline } from "./flutter/render-box.ts";
import { RenderBaseline, RenderFractionallySizedOverflowBox } from "./flutter/shifted-boxes.ts";
import {
  FixedColumnWidth,
  FlexColumnWidth,
  FractionColumnWidth,
  IntrinsicColumnWidth,
  MaxColumnWidth,
  MinColumnWidth,
  RenderTable,
  TableCellParentData,
  TableCellVerticalAlignment,
  type TableColumnWidth,
} from "./flutter/table.ts";
import { RenderTestBox } from "./test-box.ts";

const inf = Number.POSITIVE_INFINITY;

function box(width: number, height: number, extra: { min?: number; baseline?: number } = {}): RenderTestBox {
  return new RenderTestBox({
    size: new Size(width, height),
    minIntrinsicWidth: extra.min ?? width,
    ...(extra.baseline === undefined ? {} : { baseline: extra.baseline }),
  });
}

function layoutRoot(root: RenderBox, constraints: BoxConstraints): void {
  root.markNeedsLayout();
  root.layout(constraints, { parentUsesSize: true });
}

function offsetOf(child: RenderBox): Offset {
  return (child.parentData as BoxParentData).offset;
}

describe("[STU-005] applyBoxFit and FittedBox", () => {
  const input = new Size(80, 40);
  const output = new Size(100, 100);

  test("each BoxFit picks Flutter's source and destination sizes", () => {
    const sizes = (fit: BoxFit): number[] => {
      const { source, destination } = applyBoxFit(fit, input, output);
      return [source.width, source.height, destination.width, destination.height];
    };
    expect(sizes(BoxFit.fill)).toEqual([80, 40, 100, 100]);
    expect(sizes(BoxFit.contain)).toEqual([80, 40, 100, 50]);
    expect(sizes(BoxFit.cover)).toEqual([40, 40, 100, 100]);
    expect(sizes(BoxFit.fitWidth)).toEqual([80, 40, 100, 50]);
    expect(sizes(BoxFit.fitHeight)).toEqual([40, 40, 100, 100]);
    expect(sizes(BoxFit.none)).toEqual([80, 40, 80, 40]);
    expect(sizes(BoxFit.scaleDown)).toEqual([80, 40, 80, 40]);
    // fitWidth with a wide output behaves like cover, fitHeight like contain.
    const wide = new Size(200, 50);
    expect(applyBoxFit(BoxFit.fitWidth, input, wide).source.height).toBe(20);
    expect(applyBoxFit(BoxFit.fitHeight, input, wide).destination.width).toBe(100);
  });

  test("scaleDown shrinks a larger input by height, then by width, and never grows", () => {
    expect(applyBoxFit(BoxFit.scaleDown, new Size(200, 100), new Size(100, 100)).destination).toEqual(
      new Size(100, 50),
    );
    expect(applyBoxFit(BoxFit.scaleDown, new Size(100, 200), new Size(100, 100)).destination).toEqual(
      new Size(50, 100),
    );
    expect(applyBoxFit(BoxFit.scaleDown, new Size(20, 10), new Size(100, 100)).destination).toEqual(new Size(20, 10));
  });

  test("an empty input or output fits to nothing", () => {
    expect(applyBoxFit(BoxFit.contain, Size.zero, output).destination).toEqual(Size.zero);
    expect(applyBoxFit(BoxFit.cover, input, new Size(0, 10)).source).toEqual(Size.zero);
  });

  test("a contain-fitted child is scaled about the alignment and reports its transform", () => {
    const child = box(80, 40);
    const fitted = new RenderFittedBox({ fit: BoxFit.contain, alignment: Alignment.center, child });
    layoutRoot(fitted, BoxConstraints.tight(new Size(100, 100)));
    expect(child.size).toEqual(new Size(80, 40));
    const topLeft = fitted.childTransform.apply(Offset.zero);
    const bottomRight = fitted.childTransform.apply(new Offset(80, 40));
    expect([topLeft.dx, topLeft.dy, bottomRight.dx, bottomRight.dy]).toEqual([0, 25, 100, 75]);
    expect(fitted.hasVisualOverflow).toBe(false);
    expect(fitted.paintTransformOf(child)).toEqual(fitted.childTransform);
  });

  test("cover crops the child and none keeps its scale; the alignment moves the crop", () => {
    const child = box(80, 40);
    const covered = new RenderFittedBox({ fit: BoxFit.cover, alignment: new AlignmentDirectional(1, 0), child });
    covered.textDirection = TextDirection.rtl;
    layoutRoot(covered, BoxConstraints.tight(new Size(100, 100)));
    expect(covered.hasVisualOverflow).toBe(true);
    // RTL: end is the left, so the visible part is the child's left 40 px.
    expect(covered.childTransform.apply(Offset.zero).dx).toBe(0);
    expect(covered.childTransform.a).toBe(2.5);
    const none = new RenderFittedBox({ fit: BoxFit.none, alignment: Alignment.topLeft, child: box(30, 30) });
    layoutRoot(none, BoxConstraints.tight(new Size(100, 100)));
    expect(none.childTransform.a).toBe(1);
  });

  test("the fitted box sizes itself like a child with the aspect ratio preserved", () => {
    const fitted = new RenderFittedBox({ fit: BoxFit.scaleDown, child: box(200, 50) });
    layoutRoot(fitted, new BoxConstraints({ maxWidth: 100, maxHeight: 100 }));
    expect(fitted.size).toEqual(new Size(100, 25));
    expect(fitted.getDryLayout(new BoxConstraints({ maxWidth: 100, maxHeight: 100 }))).toEqual(new Size(100, 25));
    fitted.fit = BoxFit.contain;
    layoutRoot(fitted, new BoxConstraints({ maxWidth: 100, maxHeight: 100 }));
    expect(fitted.size).toEqual(new Size(100, 25));
    const empty = new RenderFittedBox();
    layoutRoot(empty, new BoxConstraints({ minWidth: 5, maxWidth: 9 }));
    expect(empty.size).toEqual(new Size(5, 0));
    expect(empty.childTransform).toBe(Affine.identity);
    expect(empty.hasVisualOverflow).toBe(false);
  });
});

describe("[STU-005] table column widths", () => {
  function table(columns: Array<TableColumnWidth | null>, cells: RenderBox[], rows = 1): RenderTable {
    const widths = new Map<number, TableColumnWidth>();
    columns.forEach((spec, index) => {
      if (spec !== null) {
        widths.set(index, spec);
      }
    });
    return new RenderTable({
      columns: columns.length,
      rows,
      children: cells,
      columnWidths: widths,
      textDirection: TextDirection.ltr,
    });
  }

  test("fixed, fraction and intrinsic columns take their ideal width; flex columns share the rest", () => {
    const cells = [box(10, 10), box(10, 10), box(70, 10, { min: 30 }), box(10, 10)];
    const t = table([new FixedColumnWidth(50), new FractionColumnWidth(0.25), new IntrinsicColumnWidth(), null], cells);
    // 360 max: fixed 50, fraction 90, intrinsic 70, flex gets 360 - 210.
    expect(t.computeColumnWidths(new BoxConstraints({ maxWidth: 360 }))).toEqual([50, 90, 70, 150]);
    layoutRoot(t, new BoxConstraints({ maxWidth: 360 }));
    expect(t.size.width).toBe(360);
    expect(cells.map((cell) => offsetOf(cell).dx)).toEqual([0, 50, 140, 210]);
  });

  test("an unbounded table uses the minimum width for flex and zero for fractions", () => {
    const t = table(
      [new FractionColumnWidth(0.5), new FlexColumnWidth(2), new FlexColumnWidth(1)],
      [box(5, 5), box(5, 5), box(5, 5)],
    );
    expect(t.computeColumnWidths(new BoxConstraints({ minWidth: 90 }))).toEqual([0, 60, 30]);
    expect(t.getMinIntrinsicWidth(inf)).toBe(0);
    expect(t.getMaxIntrinsicWidth(inf)).toBe(0);
  });

  test("without flex columns the table grows evenly to the minimum width", () => {
    const t = table([new FixedColumnWidth(10), new FixedColumnWidth(20)], [box(1, 1), box(1, 1)]);
    expect(t.computeColumnWidths(new BoxConstraints({ minWidth: 50, maxWidth: 80 }))).toEqual([20, 30]);
  });

  test("a too-wide table shrinks flex columns proportionally, then the others equally, never below their minimum", () => {
    // Intrinsic (flex 1 and 3): ideal 100 and 100, minimum 20 and 90. Fixed 60 cannot shrink.
    const cells = [box(100, 10, { min: 20 }), box(100, 10, { min: 90 }), box(60, 10)];
    const t = table([new IntrinsicColumnWidth(1), new IntrinsicColumnWidth(3), new FixedColumnWidth(60)], cells);
    const widths = t.computeColumnWidths(new BoxConstraints({ maxWidth: 200 }));
    // Deficit 160: flex shares 40 and 120 would give 60 and -20, so the second stops at 90 and the first absorbs the rest.
    expect(widths[1]).toBe(90);
    expect(widths[2]).toBe(60);
    expect(widths[0]).toBeCloseTo(50, 9);
    // Beyond the flex columns' slack the non-flex column gives way down to its minimum, which is its width here.
    const tight = t.computeColumnWidths(new BoxConstraints({ maxWidth: 100 }));
    expect(tight).toEqual([20, 90, 60]);
  });

  test("non-flex columns shrink equally down to their minimum widths", () => {
    const cells = [box(100, 10, { min: 10 }), box(100, 10, { min: 80 })];
    const t = table([new IntrinsicColumnWidth(), new IntrinsicColumnWidth()], cells);
    expect(t.computeColumnWidths(new BoxConstraints({ maxWidth: 150 }))).toEqual([70, 80]);
  });

  test("Max and Min columns combine two specs and their flex", () => {
    const cells = [box(40, 10, { min: 15 })];
    const max = new MaxColumnWidth(new FixedColumnWidth(30), new IntrinsicColumnWidth());
    const min = new MinColumnWidth(new FixedColumnWidth(30), new IntrinsicColumnWidth());
    expect(max.maxIntrinsicWidth(cells, 100)).toBe(40);
    expect(max.minIntrinsicWidth(cells, 100)).toBe(30);
    expect(min.maxIntrinsicWidth(cells, 100)).toBe(30);
    expect(min.minIntrinsicWidth(cells, 100)).toBe(15);
    expect(max.flex(cells)).toBeNull();
    expect(new MaxColumnWidth(new FlexColumnWidth(2), new IntrinsicColumnWidth(5)).flex(cells)).toBe(5);
    expect(new MinColumnWidth(new FlexColumnWidth(2), new IntrinsicColumnWidth(5)).flex(cells)).toBe(2);
    expect(new MinColumnWidth(new IntrinsicColumnWidth(), new FlexColumnWidth(2)).flex(cells)).toBe(2);
    expect(new MaxColumnWidth(new FlexColumnWidth(2), new FixedColumnWidth(1)).flex(cells)).toBe(2);
  });

  test("vertical alignments place cells in the row; right-to-left mirrors the columns", () => {
    const cells = [box(10, 10), box(10, 20), box(10, 30), box(10, 5), box(10, 8)];
    const aligns = [
      TableCellVerticalAlignment.top,
      TableCellVerticalAlignment.middle,
      TableCellVerticalAlignment.bottom,
      TableCellVerticalAlignment.fill,
      TableCellVerticalAlignment.intrinsicHeight,
    ];
    cells.forEach((cell, index) => {
      const data = new TableCellParentData();
      data.verticalAlignment = aligns[index] ?? null;
      cell.parentData = data;
    });
    const t = new RenderTable({
      columns: 5,
      rows: 1,
      children: cells,
      defaultColumnWidth: new FixedColumnWidth(10),
      textDirection: TextDirection.rtl,
    });
    layoutRoot(t, new BoxConstraints({ maxWidth: 100 }));
    expect(t.size).toEqual(new Size(50, 30));
    expect(cells.map((cell) => offsetOf(cell).dx)).toEqual([40, 30, 20, 10, 0]);
    expect(cells.map((cell) => offsetOf(cell).dy)).toEqual([0, 5, 0, 0, 0]);
    expect(cells[3]?.size.height).toBe(30);
    expect(t.rowTops).toEqual([0, 30]);
    expect(t.getMinIntrinsicHeight(50)).toBe(30);
    expect(t.getDryLayout(new BoxConstraints({ maxWidth: 100 }))).toEqual(new Size(50, 30));
  });

  test("baseline alignment lines cells up and gives the table its baseline", () => {
    const a = box(10, 20, { baseline: 15 });
    const b = box(10, 30, { baseline: 10 });
    const t = new RenderTable({
      columns: 2,
      rows: 1,
      children: [a, b],
      defaultVerticalAlignment: TableCellVerticalAlignment.baseline,
      textBaseline: TextBaseline.alphabetic,
      textDirection: TextDirection.ltr,
    });
    layoutRoot(t, new BoxConstraints({ maxWidth: 100 }));
    expect(offsetOf(a).dy).toBe(0);
    expect(offsetOf(b).dy).toBe(5);
    expect(t.size.height).toBe(35);
    expect(t.getDistanceToBaseline(TextBaseline.alphabetic)).toBe(15);
    expect(t.getDryBaseline(new BoxConstraints({ maxWidth: 100 }), TextBaseline.alphabetic)).toBe(15);
    expect(t.getDryLayout(new BoxConstraints({ maxWidth: 100 }))).toEqual(Size.zero);
    t.textBaseline = null;
    expect(() => layoutRoot(t, new BoxConstraints({ maxWidth: 100 }))).toThrow(/textBaseline/);
  });

  test("an empty table is as small as the constraints allow, and a wrong cell count throws", () => {
    const t = new RenderTable({ columns: 0, rows: 0, textDirection: TextDirection.ltr });
    layoutRoot(t, new BoxConstraints({ minWidth: 4, minHeight: 6, maxWidth: 10, maxHeight: 10 }));
    expect(t.size).toEqual(new Size(4, 6));
    expect(t.getMinIntrinsicHeight(10)).toBe(0);
    expect(t.getMaxIntrinsicWidth(10)).toBe(0);
    expect(t.getDryBaseline(new BoxConstraints(), TextBaseline.alphabetic)).toBeNull();
    expect(() => new RenderTable({ columns: 2, rows: 1, children: [], textDirection: TextDirection.ltr })).toThrow(
      /needs rows \* columns/,
    );
  });
});

describe("[STU-005] the other box widgets", () => {
  test("LimitedBox limits only unbounded axes", () => {
    const child = new RenderTestBox({ size: new Size(inf, inf) });
    const limited = new RenderLimitedBox({ maxWidth: 50, maxHeight: 30, child });
    layoutRoot(limited, new BoxConstraints({ maxHeight: 200 }));
    expect(limited.size).toEqual(new Size(50, 200));
    expect(limited.getDryLayout(new BoxConstraints({ maxHeight: 200 }))).toEqual(new Size(50, 200));
    limited.maxWidth = 70;
    limited.maxHeight = 10;
    layoutRoot(limited, new BoxConstraints());
    expect(limited.size).toEqual(new Size(70, 10));
    const empty = new RenderLimitedBox({ maxWidth: 5 });
    layoutRoot(empty, new BoxConstraints());
    expect(empty.size).toEqual(new Size(0, 0));
  });

  test("AspectRatio follows Flutter's constraint order and has width-from-height intrinsics", () => {
    const sized = (ratio: number, constraints: BoxConstraints): Size => {
      const ar = new RenderAspectRatio({ aspectRatio: ratio, child: box(1, 1) });
      layoutRoot(ar, constraints);
      return ar.size;
    };
    expect(sized(2, new BoxConstraints({ maxWidth: 200 }))).toEqual(new Size(200, 100));
    expect(sized(1.5, new BoxConstraints({ maxWidth: 300, maxHeight: 60 }))).toEqual(new Size(90, 60));
    expect(sized(0.5, new BoxConstraints({ minWidth: 120, maxWidth: 300, maxHeight: 300 }))).toEqual(
      new Size(150, 300),
    );
    expect(sized(2, new BoxConstraints({ maxHeight: 40 }))).toEqual(new Size(80, 40));
    expect(sized(2, new BoxConstraints({ minHeight: 100, maxWidth: 50, maxHeight: 200 }))).toEqual(new Size(50, 100));
    expect(sized(2, BoxConstraints.tight(new Size(30, 30)))).toEqual(new Size(30, 30));
    const ar = new RenderAspectRatio({ aspectRatio: 2, child: box(7, 9) });
    expect(ar.getMinIntrinsicWidth(10)).toBe(20);
    expect(ar.getMaxIntrinsicWidth(10)).toBe(20);
    expect(ar.getMinIntrinsicHeight(10)).toBe(5);
    expect(ar.getMaxIntrinsicHeight(10)).toBe(5);
    expect(ar.getMinIntrinsicWidth(inf)).toBe(7);
    expect(ar.getMaxIntrinsicWidth(inf)).toBe(7);
    expect(ar.getMinIntrinsicHeight(inf)).toBe(9);
    expect(ar.getMaxIntrinsicHeight(inf)).toBe(9);
    ar.aspectRatio = 1;
    expect(ar.getMinIntrinsicHeight(10)).toBe(10);
    expect(ar.getDryBaseline(new BoxConstraints({ maxWidth: 10, maxHeight: 10 }), TextBaseline.alphabetic)).toBeNull();
  });

  test("FractionallySizedBox sizes the child by a fraction of the maximum and aligns it", () => {
    const child = new RenderTestBox({ size: new Size(inf, inf), baseline: 4 });
    const fractional = new RenderFractionallySizedOverflowBox({
      widthFactor: 0.5,
      heightFactor: 1.5,
      alignment: new AlignmentDirectional(1, 1),
      textDirection: TextDirection.ltr,
      child,
    });
    layoutRoot(fractional, BoxConstraints.tight(new Size(200, 100)));
    expect(child.size).toEqual(new Size(100, 150));
    expect(fractional.size).toEqual(new Size(200, 100));
    expect(offsetOf(child)).toEqual(new Offset(100, -50));
    expect(fractional.getDryBaseline(BoxConstraints.tight(new Size(200, 100)), TextBaseline.alphabetic)).toBe(-46);
    expect(fractional.getMinIntrinsicWidth(40)).toBe(2 * child.getMinIntrinsicWidth(60));
    expect(fractional.getMaxIntrinsicWidth(40)).toBe(2 * child.getMaxIntrinsicWidth(60));
    expect(fractional.getMinIntrinsicHeight(40)).toBeCloseTo(child.getMinIntrinsicHeight(20) / 1.5, 9);
    expect(fractional.getMaxIntrinsicHeight(40)).toBeCloseTo(child.getMaxIntrinsicHeight(20) / 1.5, 9);
    fractional.widthFactor = null;
    fractional.heightFactor = null;
    layoutRoot(fractional, new BoxConstraints({ maxWidth: 80, maxHeight: 80 }));
    expect(child.size).toEqual(new Size(80, 80));
    const empty = new RenderFractionallySizedOverflowBox({ widthFactor: 0.5 });
    layoutRoot(empty, new BoxConstraints({ minWidth: 3, maxWidth: 10 }));
    expect(empty.size).toEqual(new Size(5, 0).width >= 3 ? new Size(5, 0) : Size.zero);
    expect(empty.getDryLayout(new BoxConstraints({ minWidth: 3, maxWidth: 10 }))).toEqual(empty.size);
    expect(empty.getDryBaseline(new BoxConstraints(), TextBaseline.alphabetic)).toBeNull();
    expect(empty.getMinIntrinsicWidth(1)).toBe(0);
    expect(empty.getMaxIntrinsicHeight(1)).toBe(0);
  });

  test("Baseline offsets the child so its baseline lies at the requested distance", () => {
    const child = box(30, 20, { baseline: 14 });
    const baseline = new RenderBaseline({ baseline: 50, baselineType: TextBaseline.alphabetic, child });
    layoutRoot(baseline, new BoxConstraints({ maxWidth: 100, maxHeight: 100 }));
    expect(offsetOf(child)).toEqual(new Offset(0, 36));
    expect(baseline.size).toEqual(new Size(30, 56));
    expect(baseline.getDryLayout(new BoxConstraints({ maxWidth: 100, maxHeight: 100 }))).toEqual(new Size(30, 56));
    expect(baseline.getDryBaseline(new BoxConstraints(), TextBaseline.alphabetic)).toBe(50);
    baseline.baseline = 10;
    baseline.baselineType = TextBaseline.ideographic;
    layoutRoot(baseline, new BoxConstraints({ maxWidth: 100, maxHeight: 100 }));
    expect(offsetOf(child)).toEqual(new Offset(0, -4));
    const noBaseline = new RenderBaseline({ baseline: 25, baselineType: TextBaseline.alphabetic, child: box(5, 8) });
    layoutRoot(noBaseline, new BoxConstraints());
    expect(noBaseline.size).toEqual(new Size(5, 25));
    expect(noBaseline.getDryBaseline(new BoxConstraints(), TextBaseline.alphabetic)).toBeNull();
    const empty = new RenderBaseline({ baseline: 1, baselineType: TextBaseline.alphabetic });
    layoutRoot(empty, new BoxConstraints({ minWidth: 2 }));
    expect(empty.size).toEqual(new Size(2, 0));
  });

  test("IntrinsicWidth and IntrinsicHeight size the child to its intrinsic dimension, in steps", () => {
    expect(applyStep(33, 10)).toBe(40);
    expect(applyStep(33, null)).toBe(33);
    const wide = new RenderTestBox({
      size: new Size(inf, inf),
      minIntrinsicWidth: 20,
      maxIntrinsicWidth: 70,
      minIntrinsicHeight: 12,
      maxIntrinsicHeight: 25,
    });
    const iw = new RenderIntrinsicWidth({ stepWidth: 50, stepHeight: 30, child: wide });
    layoutRoot(iw, new BoxConstraints({ maxWidth: 400, maxHeight: 400 }));
    expect(iw.size).toEqual(new Size(100, 30));
    expect(iw.getMinIntrinsicWidth(10)).toBe(100);
    expect(iw.getMaxIntrinsicWidth(10)).toBe(100);
    expect(iw.getMinIntrinsicHeight(inf)).toBe(30);
    expect(iw.getMaxIntrinsicHeight(40)).toBe(30);
    expect(iw.getDryLayout(new BoxConstraints({ maxWidth: 400, maxHeight: 400 }))).toEqual(new Size(100, 30));
    expect(
      iw.getDryBaseline(new BoxConstraints({ maxWidth: 400, maxHeight: 400 }), TextBaseline.alphabetic),
    ).toBeNull();
    iw.stepWidth = null;
    iw.stepHeight = null;
    layoutRoot(iw, new BoxConstraints({ maxWidth: 400, maxHeight: 400 }));
    expect(iw.size).toEqual(new Size(70, 400));
    expect(iw.stepWidth).toBeNull();
    expect(iw.stepHeight).toBeNull();
    const emptyW = new RenderIntrinsicWidth();
    layoutRoot(emptyW, new BoxConstraints({ minWidth: 3 }));
    expect(emptyW.size).toEqual(new Size(3, 0));
    expect(emptyW.getMaxIntrinsicHeight(5)).toBe(0);
    expect(emptyW.getMinIntrinsicHeight(5)).toBe(0);
    expect(emptyW.getMaxIntrinsicWidth(5)).toBe(0);
    expect(emptyW.getDryLayout(new BoxConstraints({ minWidth: 3 }))).toEqual(new Size(3, 0));

    const ih = new RenderIntrinsicHeight(wide);
    layoutRoot(ih, new BoxConstraints({ maxWidth: 400, maxHeight: 400 }));
    expect(ih.size).toEqual(new Size(400, 25));
    expect(ih.getMinIntrinsicWidth(inf)).toBe(20);
    expect(ih.getMaxIntrinsicWidth(inf)).toBe(70);
    expect(ih.getMinIntrinsicWidth(10)).toBe(20);
    expect(ih.getMaxIntrinsicWidth(10)).toBe(70);
    expect(ih.getMinIntrinsicHeight(10)).toBe(25);
    expect(ih.getDryLayout(new BoxConstraints({ maxWidth: 400, maxHeight: 400 }))).toEqual(new Size(400, 25));
    expect(
      ih.getDryBaseline(new BoxConstraints({ maxWidth: 400, maxHeight: 400 }), TextBaseline.alphabetic),
    ).toBeNull();
    layoutRoot(ih, BoxConstraints.tight(new Size(10, 10)));
    const emptyH = new RenderIntrinsicHeight();
    layoutRoot(emptyH, new BoxConstraints({ minHeight: 2 }));
    expect(emptyH.size).toEqual(new Size(0, 2));
    expect(emptyH.getMinIntrinsicWidth(inf)).toBe(0);
    expect(emptyH.getMaxIntrinsicWidth(inf)).toBe(0);
    expect(emptyH.getDryLayout(new BoxConstraints({ minHeight: 2 }))).toEqual(new Size(0, 2));
  });

  test("Transform composes the origin and alignment and reports the transformed bounds", () => {
    const child = box(40, 30);
    const transform = new RenderTransform({
      transform: Affine.scaling(2, 2),
      alignment: Alignment.center,
      origin: new Offset(10, 0),
      textDirection: TextDirection.ltr,
      child,
    });
    layoutRoot(transform, new BoxConstraints());
    expect(transform.size).toEqual(new Size(40, 30));
    // origin (10, 0) plus the centre (20, 15): scale about (30, 15).
    expect(transform.transformedBounds).toEqual(Rect.fromLTRB(-30, -15, 50, 45));
    expect(transform.paintTransformOf(child).apply(Offset.zero)).toEqual(new Offset(-30, -15));
    const plain = new RenderTransform({ transform: Affine.rotation(Math.PI / 2), child: box(10, 20) });
    layoutRoot(plain, new BoxConstraints());
    const bounds = plain.transformedBounds;
    expect([bounds.left, bounds.top, bounds.right, bounds.bottom].map(Math.round)).toEqual([-20, 0, 0, 10]);
  });

  test("FractionalTranslation and the clips and opacity keep the child's layout", () => {
    const child = box(40, 20);
    const slide = new RenderFractionalTranslation({ translation: new Offset(-1, 0.5), child });
    layoutRoot(slide, new BoxConstraints());
    expect(slide.size).toEqual(new Size(40, 20));
    expect(slide.paintOffset).toEqual(new Offset(-40, 10));
    expect(slide.paintTransformOf(child).apply(Offset.zero)).toEqual(new Offset(-40, 10));
    expect(slide.transformHitTests).toBe(true);
    for (const shape of [ClipShape.rect, ClipShape.rrect, ClipShape.oval, ClipShape.path]) {
      const clip = new RenderClip({ shape, child: box(7, 9) });
      layoutRoot(clip, new BoxConstraints());
      expect(clip.clipBounds).toEqual(Rect.fromLTWH(0, 0, 7, 9));
    }
  });
});

describe("[STU-068] layout at an animation value", () => {
  test("implicit animation values interpolate like Flutter's tweens", () => {
    const a = BoxConstraints.tightFor({ width: 100, height: 50 });
    const b = BoxConstraints.tightFor({ width: 200, height: 150 });
    expect(lerpBoxConstraints(a, b, 0.25).minWidth).toBe(125);
    expect(lerpBoxConstraints(a, b, 0.25).maxHeight).toBe(75);
    expect(
      lerpBoxConstraints(BoxConstraints.tightFor({ width: 1 }), BoxConstraints.tightFor({ width: 3 }), 0.5).maxHeight,
    ).toBe(inf);
    expect(lerpAlignment(new Alignment(-1, -1), new Alignment(1, 0), 0.5)).toEqual(new Alignment(0, -0.5));
    expect(lerpAlignment(new AlignmentDirectional(-1, 0), new AlignmentDirectional(1, 0), 0.75)).toEqual(
      new AlignmentDirectional(0.5, 0),
    );
    expect(() => lerpAlignment(Alignment.center, AlignmentDirectional.center, 0.5)).toThrow(/cannot interpolate/);
    const lerp = (x: number, y: number, t: number): number => x + (y - x) * t;
    expect(lerpProperty(null, 8, 0.5, lerp)).toBe(8);
    expect(lerpProperty(4, null, 0.5, lerp)).toBeNull();
    expect(lerpProperty(4, 8, 0.5, lerp)).toBe(6);
  });

  test("Container builds margin, constraints, padding and align around the child; childless ones expand", () => {
    const child = box(40, 40);
    const outer = buildContainer({
      alignment: Alignment.bottomRight,
      padding: EdgeInsets.all(10),
      constraints: BoxConstraints.tightFor({ width: 200, height: 100 }),
      margin: EdgeInsets.fromLTRB(8, 8, 16, 16),
      textDirection: TextDirection.ltr,
      child,
    });
    if (outer === null) {
      throw new Error("expected a container box");
    }
    layoutRoot(outer, new BoxConstraints({ maxWidth: 500, maxHeight: 500 }));
    expect(outer.size).toEqual(new Size(224, 124));
    expect(buildContainer({ textDirection: TextDirection.ltr, child })).toBe(child);
    const empty = buildContainer({ textDirection: TextDirection.ltr, child: null });
    if (empty === null) {
      throw new Error("expected a container box");
    }
    layoutRoot(empty, new BoxConstraints({ maxWidth: 30, maxHeight: 20 }));
    expect(empty.size).toEqual(new Size(30, 20));
    layoutRoot(empty, new BoxConstraints());
    expect(empty.size).toEqual(new Size(0, 0));
    const aligned = buildContainer({
      alignment: Alignment.center,
      constraints: BoxConstraints.tightFor({ width: 10, height: 10 }),
      textDirection: TextDirection.ltr,
      child: null,
    });
    if (aligned === null) {
      throw new Error("expected a container box");
    }
    layoutRoot(aligned, new BoxConstraints({ maxWidth: 100, maxHeight: 100 }));
    expect(aligned.size).toEqual(new Size(10, 10));
  });

  test("SizeTransition clips an Align whose factor follows the size factor, clamped at zero", () => {
    const child = box(100, 80);
    const transition = buildSizeTransition({
      axis: Axis.vertical,
      sizeFactor: 0.5,
      textDirection: TextDirection.ltr,
      child,
    });
    layoutRoot(transition, new BoxConstraints({ maxWidth: 300, maxHeight: 300 }));
    expect(transition.size).toEqual(new Size(300, 40));
    expect(transition.clipBounds).toEqual(Rect.fromLTWH(0, 0, 300, 40));
    const horizontal = buildSizeTransition({
      axis: Axis.horizontal,
      sizeFactor: -1,
      fixedCrossAxisSizeFactor: 0.5,
      alignment: new AlignmentDirectional(1, -1),
      textDirection: TextDirection.ltr,
      child: box(100, 80),
    });
    layoutRoot(horizontal, new BoxConstraints({ maxWidth: 300, maxHeight: 300 }));
    expect(horizontal.size).toEqual(new Size(0, 40));
  });
});
