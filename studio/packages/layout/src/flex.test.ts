// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { BoxConstraints } from "./flutter/constraints.ts";
import {
  Axis,
  CrossAxisAlignment,
  FlexFit,
  FlexParentData,
  MainAxisAlignment,
  MainAxisSize,
  RenderFlex,
  VerticalDirection,
} from "./flutter/flex.ts";
import { Alignment, Offset, Size, TextDirection } from "./flutter/geometry.ts";
import { PipelineOwner, type RenderObject } from "./flutter/object.ts";
import { RenderConstrainedBox } from "./flutter/proxy-box.ts";
import { RenderBox, TextBaseline } from "./flutter/render-box.ts";
import { RenderPositionedBox } from "./flutter/shifted-box.ts";
import { RenderView } from "./flutter/view.ts";
import { RenderTestBox } from "./test-box.ts";

const inf = Number.POSITIVE_INFINITY;
const alphabetic = TextBaseline.alphabetic;
const square = BoxConstraints.tightFor({ width: 100, height: 100 });

/**
 * Flutter's rendering test `layout()`: the box is the child of an 800×600
 * view; with `constraints` it sits top-left inside a constrained box.
 * Returns `pumpFrame`.
 */
function layout(box: RenderBox, constraints?: BoxConstraints): () => void {
  const child =
    constraints === undefined
      ? box
      : new RenderPositionedBox({
          alignment: Alignment.topLeft,
          child: new RenderConstrainedBox({ additionalConstraints: constraints, child: box }),
        });
  const owner = new PipelineOwner();
  const view = new RenderView({ size: new Size(800, 600), child });
  owner.rootNode = view;
  view.prepareInitialFrame();
  owner.flushLayout();
  return () => owner.flushLayout();
}

/** Lays out a parentless box directly under `constraints` (a stand-in for Flutter's RenderConstrainedOverflowBox). */
function layoutRoot(box: RenderBox, constraints: BoxConstraints): void {
  box.markNeedsLayout();
  box.layout(constraints, { parentUsesSize: true });
}

function sized(): RenderConstrainedBox {
  return new RenderConstrainedBox({ additionalConstraints: square });
}

function empty(): RenderConstrainedBox {
  return new RenderConstrainedBox({ additionalConstraints: new BoxConstraints() });
}

function parentData(box: RenderBox): FlexParentData {
  const data = box.parentData;
  if (!(data instanceof FlexParentData)) {
    throw new Error("not a flex child");
  }
  return data;
}

function setFlex(box: RenderBox, flex: number, fit: FlexFit | null = null): void {
  const data = parentData(box);
  data.flex = flex;
  data.fit = fit;
}

function getOffset(box: RenderBox): Offset {
  return parentData(box).offset;
}

/** The offset of `box` from the root (`localToGlobal(Offset.zero)`). */
function globalOffset(box: RenderBox): Offset {
  let dx = 0;
  let dy = 0;
  let node: RenderObject | null = box;
  while (node !== null && node.parent !== null) {
    const offset = RenderBox.offsetOf(node);
    dx += offset.dx;
    dy += offset.dy;
    node = node.parent;
  }
  return new Offset(dx, dy);
}

function xy(box: RenderBox): [number, number] {
  const offset = globalOffset(box);
  return [offset.dx, offset.dy];
}

/** flex_test.dart `RenderFlowBaselineTestBox`: 10×10 grids flowing into lines, like text. */
class RenderFlowBaselineTestBox extends RenderBox {
  static readonly gridSize = new Size(10, 10);
  gridCount = 0;
  baselinePlacer: (height: number) => number | null = () => null;

  lineGridCount(width: number): number {
    const gridsPerLine =
      width >= this.gridCount * RenderFlowBaselineTestBox.gridSize.width
        ? this.gridCount
        : Math.trunc(width / RenderFlowBaselineTestBox.gridSize.width);
    return Math.max(1, gridsPerLine);
  }

  lineCount(width: number): number {
    return Math.ceil(this.gridCount / this.lineGridCount(width));
  }

  protected override computeMinIntrinsicWidth(_height: number): number {
    return RenderFlowBaselineTestBox.gridSize.width;
  }

  protected override computeMaxIntrinsicWidth(_height: number): number {
    return RenderFlowBaselineTestBox.gridSize.width * this.gridCount;
  }

  protected override computeMinIntrinsicHeight(width: number): number {
    return RenderFlowBaselineTestBox.gridSize.height * this.lineCount(width);
  }

  protected override computeMaxIntrinsicHeight(width: number): number {
    return this.computeMinIntrinsicHeight(width);
  }

  protected override computeDryLayout(constraints: BoxConstraints): Size {
    return constraints.constrain(
      new Size(
        RenderFlowBaselineTestBox.gridSize.width * this.lineGridCount(constraints.maxWidth),
        RenderFlowBaselineTestBox.gridSize.height * this.lineCount(constraints.maxWidth),
      ),
    );
  }

  protected override computeDryBaseline(constraints: BoxConstraints, _baseline: TextBaseline): number | null {
    return this.baselinePlacer(this.getDryLayout(constraints).height);
  }

  protected override computeDistanceToActualBaseline(_baseline: TextBaseline): number | null {
    return this.baselinePlacer(this.size.height);
  }

  protected override performLayout(): void {
    this.size = this.computeDryLayout(this.constraints);
  }
}

function flowBox(gridCount: number, baselinePlacer: (height: number) => number | null): RenderFlowBaselineTestBox {
  const box = new RenderFlowBaselineTestBox();
  box.gridCount = gridCount;
  box.baselinePlacer = baselinePlacer;
  return box;
}

describe("[STU-005] RenderFlex: ported from Flutter flex_test.dart", () => {
  test("Overconstrained flex", () => {
    const flex = new RenderFlex({ textDirection: TextDirection.ltr, children: [empty()] });
    layout(flex, new BoxConstraints({ minWidth: 200, maxWidth: 200, minHeight: 200, maxHeight: 200 }));
    expect(flex.size).toEqual(new Size(200, 200));
  });

  test("Inconsequential overflow is ignored", () => {
    const slightlyLarger = 438.8571428571429;
    const slightlySmaller = 438.85714285714283;
    const box1 = new RenderConstrainedBox({
      additionalConstraints: BoxConstraints.tightFor({ width: slightlyLarger, height: 100 }),
    });
    const flex = new RenderFlex({ textDirection: TextDirection.ltr, mainAxisSize: MainAxisSize.min });
    flex.add(box1);
    layoutRoot(flex, new BoxConstraints({ maxWidth: slightlySmaller, maxHeight: 400 }));
    expect(flex.size).toEqual(new Size(slightlySmaller, 100));
    expect(flex.overflow).toBe(0);
  });

  for (const spacing of [0, 16]) {
    test(`Vertical Overflow (spacing ${spacing})`, () => {
      const flexible = new RenderConstrainedBox({ additionalConstraints: BoxConstraints.expand() });
      const flex = new RenderFlex({
        direction: Axis.vertical,
        spacing,
        children: [
          new RenderConstrainedBox({ additionalConstraints: BoxConstraints.tightFor({ height: 200 }) }),
          flexible,
        ],
      });
      setFlex(flexible, 1);
      layout(flex, new BoxConstraints({ maxHeight: 100, maxWidth: 100 }));
      expect(flexible.size.height).toBe(0);
      expect(flex.overflow).toBe(100 + spacing);
      expect(flex.getMinIntrinsicHeight(100)).toBe(200 + spacing);
      expect(flex.getMaxIntrinsicHeight(100)).toBe(200 + spacing);
      expect(flex.getMinIntrinsicWidth(100)).toBe(0);
      expect(flex.getMaxIntrinsicWidth(100)).toBe(0);
    });
  }

  for (const spacing of [0, 12]) {
    test(`Horizontal Overflow (spacing ${spacing})`, () => {
      const flexible = new RenderConstrainedBox({ additionalConstraints: BoxConstraints.expand() });
      const flex = new RenderFlex({
        textDirection: TextDirection.ltr,
        spacing,
        children: [
          new RenderConstrainedBox({ additionalConstraints: BoxConstraints.tightFor({ width: 200 }) }),
          flexible,
        ],
      });
      setFlex(flexible, 1);
      layout(flex, new BoxConstraints({ maxHeight: 100, maxWidth: 100 }));
      expect(flexible.size.width).toBe(0);
      expect(flex.getMinIntrinsicHeight(100)).toBe(0);
      expect(flex.getMaxIntrinsicHeight(100)).toBe(0);
      expect(flex.getMinIntrinsicWidth(100)).toBe(200 + spacing);
      expect(flex.getMaxIntrinsicWidth(100)).toBe(200 + spacing);
    });
  }

  test("Defaults", () => {
    const flex = new RenderFlex();
    expect(flex.crossAxisAlignment).toBe(CrossAxisAlignment.center);
    expect(flex.direction).toBe(Axis.horizontal);
    expect(flex.mainAxisAlignment).toBe(MainAxisAlignment.start);
    expect(flex.mainAxisSize).toBe(MainAxisSize.max);
    expect(flex.verticalDirection).toBe(VerticalDirection.down);
    expect(flex.textDirection).toBeNull();
    expect(flex.textBaseline).toBeNull();
    expect(flex.spacing).toBe(0);
  });

  test("Parent data", () => {
    const box1 = empty();
    const box2 = empty();
    const flex = new RenderFlex({ textDirection: TextDirection.ltr, children: [box1, box2] });
    const pump = layout(flex, new BoxConstraints({ maxWidth: 100, maxHeight: 100 }));
    expect(box1.size).toEqual(new Size(0, 0));
    expect(box2.size).toEqual(new Size(0, 0));
    setFlex(box2, 1);
    flex.markNeedsLayout();
    pump();
    expect(box1.size).toEqual(new Size(0, 0));
    expect(box2.size).toEqual(new Size(100, 0));
  });

  test("Stretch", () => {
    const box1 = empty();
    const box2 = empty();
    const flex = new RenderFlex({ textDirection: TextDirection.ltr });
    flex.setupParentData(box2);
    setFlex(box2, 2);
    flex.addAll([box1, box2]);
    const pump = layout(flex, new BoxConstraints({ maxWidth: 100, maxHeight: 100 }));
    expect(box1.size).toEqual(new Size(0, 0));
    expect(box2.size).toEqual(new Size(100, 0));
    flex.crossAxisAlignment = CrossAxisAlignment.stretch;
    pump();
    expect(box1.size).toEqual(new Size(0, 100));
    expect(box2.size).toEqual(new Size(100, 100));
    flex.direction = Axis.vertical;
    pump();
    expect(box1.size).toEqual(new Size(100, 0));
    expect(box2.size).toEqual(new Size(100, 100));
  });

  test("Space evenly", () => {
    const [box1, box2, box3] = [sized(), sized(), sized()];
    const flex = new RenderFlex({
      textDirection: TextDirection.ltr,
      mainAxisAlignment: MainAxisAlignment.spaceEvenly,
      children: [box1, box2, box3],
    });
    const pump = layout(flex, new BoxConstraints({ maxWidth: 500, maxHeight: 400 }));
    expect([getOffset(box1).dx, getOffset(box2).dx, getOffset(box3).dx]).toEqual([50, 200, 350]);
    flex.direction = Axis.vertical;
    pump();
    expect([getOffset(box1).dy, getOffset(box2).dy, getOffset(box3).dy]).toEqual([25, 150, 275]);
  });

  const spacingCases: [MainAxisAlignment, number[], number[]][] = [
    [MainAxisAlignment.start, [0, 114, 228], [0, 114, 228]],
    [MainAxisAlignment.end, [172, 286, 400], [72, 186, 300]],
    [MainAxisAlignment.center, [86, 200, 314], [36, 150, 264]],
    [MainAxisAlignment.spaceEvenly, [43, 200, 357], [18, 150, 282]],
    [MainAxisAlignment.spaceAround, [28.6, 200, 371.3], [12, 150, 288]],
    [MainAxisAlignment.spaceBetween, [0, 200, 400], [0, 150, 300]],
  ];
  for (const [alignment, xs, ys] of spacingCases) {
    test(`MainAxisAlignment.${alignment} with RenderFlex.spacing`, () => {
      const boxes = [sized(), sized(), sized()];
      const flex = new RenderFlex({
        textDirection: TextDirection.ltr,
        mainAxisAlignment: alignment,
        spacing: 14,
        children: boxes,
      });
      const pump = layout(flex, new BoxConstraints({ maxWidth: 500, maxHeight: 400 }));
      boxes.forEach((box, i) => {
        const x = xs[i] ?? Number.NaN;
        // Flutter checks the spaceAround thirds with closeTo(x, 0.1) and the rest exactly.
        if (Number.isInteger(x)) {
          expect(getOffset(box).dx).toBe(x);
        } else {
          expect(Math.abs(getOffset(box).dx - x)).toBeLessThan(0.1);
        }
        expect(box.size.width).toBe(100);
      });
      flex.direction = Axis.vertical;
      pump();
      boxes.forEach((box, i) => {
        expect(getOffset(box).dy).toBe(ys[i] ?? Number.NaN);
        expect(box.size.height).toBe(100);
      });
    });
  }

  test("Fit.loose", () => {
    const [box1, box2, box3] = [sized(), sized(), sized()];
    const flex = new RenderFlex({
      textDirection: TextDirection.ltr,
      mainAxisAlignment: MainAxisAlignment.spaceBetween,
      children: [box1, box2, box3],
    });
    const pump = layout(flex, new BoxConstraints({ maxWidth: 500, maxHeight: 400 }));
    expect([getOffset(box1).dx, getOffset(box2).dx, getOffset(box3).dx]).toEqual([0, 200, 400]);
    setFlex(box1, 1, FlexFit.loose);
    flex.markNeedsLayout();
    pump();
    expect([getOffset(box1).dx, getOffset(box2).dx, getOffset(box3).dx]).toEqual([0, 200, 400]);
    expect(box1.size.width).toBe(100);
    box1.additionalConstraints = BoxConstraints.tightFor({ width: 1000, height: 100 });
    pump();
    expect([getOffset(box1).dx, getOffset(box2).dx, getOffset(box3).dx]).toEqual([0, 300, 400]);
    expect([box1.size.width, box2.size.width, box3.size.width]).toEqual([300, 100, 100]);
  });

  test("Flexible with MainAxisSize.min", () => {
    const [box1, box2, box3] = [sized(), sized(), sized()];
    const flex = new RenderFlex({
      textDirection: TextDirection.ltr,
      mainAxisSize: MainAxisSize.min,
      mainAxisAlignment: MainAxisAlignment.spaceBetween,
      children: [box1, box2, box3],
    });
    const pump = layout(flex, new BoxConstraints({ maxWidth: 500, maxHeight: 400 }));
    expect([getOffset(box1).dx, getOffset(box2).dx, getOffset(box3).dx]).toEqual([0, 100, 200]);
    expect(flex.size.width).toBe(300);
    setFlex(box1, 1, FlexFit.tight);
    flex.markNeedsLayout();
    pump();
    expect([getOffset(box1).dx, getOffset(box2).dx, getOffset(box3).dx]).toEqual([0, 300, 400]);
    expect(box1.size.width).toBe(300);
    expect(flex.size.width).toBe(500);
    setFlex(box1, 1, FlexFit.loose);
    flex.markNeedsLayout();
    pump();
    expect([getOffset(box1).dx, getOffset(box2).dx, getOffset(box3).dx]).toEqual([0, 100, 200]);
    expect(box1.size.width).toBe(100);
    expect(flex.size.width).toBe(300);
  });

  test("MainAxisSize.min inside unconstrained", () => {
    const [box1, box2, box3] = [sized(), sized(), sized()];
    const flex = new RenderFlex({
      textDirection: TextDirection.ltr,
      mainAxisSize: MainAxisSize.min,
      children: [box1, box2, box3],
    });
    const parent = (maxWidth: number): BoxConstraints => new BoxConstraints({ maxWidth, maxHeight: 400 });
    layoutRoot(flex, parent(inf));
    expect(flex.size).toEqual(new Size(300, 100));
    setFlex(box2, 1, FlexFit.loose);
    layoutRoot(flex, parent(inf));
    expect(flex.size).toEqual(new Size(300, 100));
    layoutRoot(flex, parent(500));
    expect(flex.size).toEqual(new Size(300, 100));
    flex.mainAxisSize = MainAxisSize.max;
    layoutRoot(flex, parent(500));
    expect(flex.size).toEqual(new Size(500, 100));
    flex.mainAxisSize = MainAxisSize.min;
    setFlex(box2, 1, FlexFit.tight);
    layoutRoot(flex, parent(500));
    expect(flex.size).toEqual(new Size(500, 100));
    layoutRoot(flex, parent(505));
    expect(flex.size).toEqual(new Size(505, 100));
  });

  // Flutter reports a FlutterError for these two in debug builds only; release
  // builds, which this port follows, lay flexible children out as inflexible.
  for (const [mainAxisSize, fit] of [
    [MainAxisSize.min, null],
    [MainAxisSize.max, FlexFit.loose],
  ] as const) {
    test(`flex under an unbounded main axis (${mainAxisSize}, fit ${fit ?? "default"}): release layout`, () => {
      const [box1, box2, box3] = [sized(), sized(), sized()];
      const flex = new RenderFlex({ textDirection: TextDirection.ltr, mainAxisSize, children: [box1, box2, box3] });
      setFlex(box2, 1, fit);
      layoutRoot(flex, new BoxConstraints({ maxWidth: inf, maxHeight: 400 }));
      expect(flex.size).toEqual(new Size(300, 100));
      expect(box2.size).toEqual(new Size(100, 100));
      expect([getOffset(box1).dx, getOffset(box2).dx, getOffset(box3).dx]).toEqual([0, 100, 200]);
      expect(flex.getDryLayout(new BoxConstraints({ maxWidth: inf, maxHeight: 400 }))).toEqual(new Size(300, 100));
    });
  }

  test("MainAxisSize.min inside tightly constrained", () => {
    const [box1, box2, box3] = [sized(), sized(), sized()];
    const flex = new RenderFlex({
      textDirection: TextDirection.rtl,
      mainAxisSize: MainAxisSize.min,
      children: [box1, box2, box3],
    });
    layout(flex);
    expect(flex.constraints.hasTightWidth).toBe(true);
    expect([xy(box1), xy(box2), xy(box3)]).toEqual([
      [700, 250],
      [600, 250],
      [500, 250],
    ]);
    expect(box1.size).toEqual(new Size(100, 100));
  });

  test("Flex RTL", () => {
    const [box1, box2, box3] = [sized(), sized(), sized()];
    const flex = new RenderFlex({ textDirection: TextDirection.ltr, children: [box1, box2, box3] });
    const pump = layout(flex);
    const step = (change: () => void, expected: [number, number][], size = new Size(100, 100)): void => {
      change();
      pump();
      expect([xy(box1), xy(box2), xy(box3)]).toEqual(expected);
      for (const box of [box1, box2, box3]) {
        expect(box.size).toEqual(size);
      }
    };
    step(() => {}, [
      [0, 250],
      [100, 250],
      [200, 250],
    ]);
    step(
      () => (flex.mainAxisAlignment = MainAxisAlignment.end),
      [
        [500, 250],
        [600, 250],
        [700, 250],
      ],
    );
    step(
      () => (flex.textDirection = TextDirection.rtl),
      [
        [200, 250],
        [100, 250],
        [0, 250],
      ],
    );
    step(
      () => (flex.mainAxisAlignment = MainAxisAlignment.start),
      [
        [700, 250],
        [600, 250],
        [500, 250],
      ],
    );
    step(
      () => (flex.crossAxisAlignment = CrossAxisAlignment.start),
      [
        [700, 0],
        [600, 0],
        [500, 0],
      ],
    );
    step(
      () => (flex.crossAxisAlignment = CrossAxisAlignment.end),
      [
        [700, 500],
        [600, 500],
        [500, 500],
      ],
    );
    step(
      () => (flex.verticalDirection = VerticalDirection.up),
      [
        [700, 0],
        [600, 0],
        [500, 0],
      ],
    );
    step(
      () => (flex.crossAxisAlignment = CrossAxisAlignment.start),
      [
        [700, 500],
        [600, 500],
        [500, 500],
      ],
    );
    step(
      () => (flex.direction = Axis.vertical),
      [
        [700, 500],
        [700, 400],
        [700, 300],
      ],
    );
    step(
      () => (flex.crossAxisAlignment = CrossAxisAlignment.end),
      [
        [0, 500],
        [0, 400],
        [0, 300],
      ],
    );
    step(
      () => (flex.crossAxisAlignment = CrossAxisAlignment.stretch),
      [
        [0, 500],
        [0, 400],
        [0, 300],
      ],
      new Size(800, 100),
    );
    step(
      () => (flex.textDirection = TextDirection.ltr),
      [
        [0, 500],
        [0, 400],
        [0, 300],
      ],
      new Size(800, 100),
    );
    step(
      () => (flex.crossAxisAlignment = CrossAxisAlignment.start),
      [
        [0, 500],
        [0, 400],
        [0, 300],
      ],
    );
    step(
      () => (flex.crossAxisAlignment = CrossAxisAlignment.end),
      [
        [700, 500],
        [700, 400],
        [700, 300],
      ],
    );
    step(
      () => (flex.verticalDirection = VerticalDirection.down),
      [
        [700, 0],
        [700, 100],
        [700, 200],
      ],
    );
    step(
      () => (flex.mainAxisAlignment = MainAxisAlignment.end),
      [
        [700, 300],
        [700, 400],
        [700, 500],
      ],
    );
  });

  test("children with no baselines are top-aligned", () => {
    const [box1, box2] = [sized(), sized()];
    const flex = new RenderFlex({
      textDirection: TextDirection.ltr,
      children: [box1, box2],
      crossAxisAlignment: CrossAxisAlignment.baseline,
      textBaseline: alphabetic,
      verticalDirection: VerticalDirection.up,
    });
    const pump = layout(flex);
    expect([xy(box1)[1], xy(box2)[1]]).toEqual([0, 0]);
    flex.verticalDirection = VerticalDirection.down;
    pump();
    expect([xy(box1)[1], xy(box2)[1]]).toEqual([0, 0]);
  });

  test("Vertical Flex Baseline", () => {
    const baselineBox = (): RenderTestBox => new RenderTestBox({ size: new Size(100, 100), baseline: 10 });
    const filler = (): RenderTestBox => new RenderTestBox({ size: new Size(100, 100) });
    const flex = new RenderFlex({
      textDirection: TextDirection.ltr,
      direction: Axis.vertical,
      children: [filler(), baselineBox(), filler(), baselineBox(), filler()],
    });
    const pump = layout(flex);
    const flexHeight = flex.size.height;
    const check = (expected: number): void => {
      expect(flex.getDryBaseline(flex.constraints, alphabetic)).toBe(expected);
      expect(flex.getDistanceToActualBaseline(alphabetic)).toBe(expected);
    };
    check(100 + 10);
    flex.mainAxisAlignment = MainAxisAlignment.end;
    pump();
    check(flexHeight - 400 + 10);
    flex.verticalDirection = VerticalDirection.up;
    pump();
    check(300 + 10);
    flex.mainAxisAlignment = MainAxisAlignment.start;
    pump();
    check(flexHeight - 200 + 10);
  });

  test("Intrinsics: main/cross axis intrinsics in both directions with RenderFlex.spacing", () => {
    for (const direction of [Axis.horizontal, Axis.vertical]) {
      const flex = new RenderFlex({
        textDirection: TextDirection.ltr,
        direction,
        spacing: 16,
        children: [sized(), sized(), sized()],
      });
      const [width, height] = direction === Axis.horizontal ? [332, 100] : [100, 332];
      for (const extent of [inf, 300, 500]) {
        expect(flex.getMinIntrinsicWidth(extent)).toBe(width);
        expect(flex.getMaxIntrinsicWidth(extent)).toBe(width);
        expect(flex.getMinIntrinsicHeight(extent)).toBe(height);
        expect(flex.getMaxIntrinsicHeight(extent)).toBe(height);
      }
    }
  });

  // Flutter uses a RenderParagraph of "A​A​A" at font size 10; a
  // three-grid RenderFlowBaselineTestBox wraps the same way.
  for (const [flex2, flex3, cases] of [
    [
      1,
      2,
      [
        [inf, 10],
        [95, 10],
        [94, 20],
        [65, 20],
        [35, 30],
      ],
    ],
    [
      2,
      1,
      [
        [inf, 10],
        [95, 10],
        [65, 20],
        [35, 30],
      ],
    ],
  ] as const) {
    test(`Intrinsics: cross axis intrinsics, with ${flex2 < flex3 ? "ascending" : "descending"} flex flow layout`, () => {
      const box2 = flowBox(3, () => null);
      const box3 = flowBox(3, () => null);
      const flex = new RenderFlex({
        textDirection: TextDirection.ltr,
        children: [
          new RenderConstrainedBox({ additionalConstraints: BoxConstraints.tightFor({ width: 5, height: 5 }) }),
          box2,
          box3,
        ],
      });
      setFlex(box2, flex2, FlexFit.tight);
      setFlex(box3, flex3, FlexFit.tight);
      for (const [width, height] of cases) {
        expect(flex.getMinIntrinsicHeight(width)).toBe(height);
        expect(flex.getMaxIntrinsicHeight(width)).toBe(height);
      }
    });
  }

  function baselineFlex(
    box2Placer: (height: number) => number | null,
    box1Placer: (h: number) => number | null,
  ): RenderFlex {
    const box1 = flowBox(10, box1Placer);
    const box2 = flowBox(10, box2Placer);
    const flex = new RenderFlex({
      textDirection: TextDirection.ltr,
      textBaseline: alphabetic,
      crossAxisAlignment: CrossAxisAlignment.baseline,
      children: [box1, box2],
    });
    setFlex(box1, 2, FlexFit.tight);
    setFlex(box2, 1, FlexFit.loose);
    return flex;
  }

  test("Intrinsics: baseline aligned flex flow computeDryLayout", () => {
    const flex = baselineFlex(
      (height) => height,
      () => 0,
    );
    expect(flex.getDryLayout(BoxConstraints.loose(new Size(200, 100)))).toEqual(new Size(200, 30));
    expect(flex.getDryBaseline(BoxConstraints.loose(new Size(200, 100)), alphabetic)).toBe(20);
    expect(flex.getDryLayout(BoxConstraints.loose(new Size(300, 100)))).toEqual(new Size(300, 20));
    expect(flex.getDryBaseline(BoxConstraints.loose(new Size(300, 100)), alphabetic)).toBe(10);
  });

  test("Intrinsics: baseline aligned children cross intrinsic size", () => {
    const flex = baselineFlex(
      (height) => height,
      () => 0,
    );
    expect(flex.getMaxIntrinsicHeight(200)).toBe(30);
    expect(flex.getMinIntrinsicHeight(200)).toBe(30);
    expect(flex.getMaxIntrinsicHeight(300)).toBe(20);
    expect(flex.getMinIntrinsicHeight(300)).toBe(20);
  });

  test("Intrinsics: children with no baselines do not affect the baseline location", () => {
    const flex = baselineFlex(
      () => null,
      (height) => height,
    );
    expect(flex.getDryLayout(BoxConstraints.loose(new Size(200, 100)))).toEqual(new Size(200, 20));
    expect(flex.getDryBaseline(BoxConstraints.loose(new Size(200, 100)), alphabetic)).toBe(10);
    expect(flex.getDryLayout(BoxConstraints.loose(new Size(300, 100)))).toEqual(new Size(300, 10));
    expect(flex.getDryBaseline(BoxConstraints.loose(new Size(300, 100)), alphabetic)).toBe(10);
  });

  test("Can call methods that check overflow even if overflow value is not set", () => {
    const flex = new RenderFlex({ children: [] });
    expect(flex.overflow).toBe(0);
    layout(flex);
    expect(flex.size).toEqual(new Size(800, 600));
    expect(flex.overflow).toBe(0);
  });
});

describe("[STU-005] RenderFlex: ported from Flutter flex_overflow_test.dart", () => {
  test("Flex overflow indicator: overflow amount", () => {
    const column = (height: number | null): RenderFlex => {
      const flex = new RenderFlex({
        direction: Axis.vertical,
        children: [
          new RenderConstrainedBox({ additionalConstraints: BoxConstraints.tightFor({ width: 200, height: 200 }) }),
        ],
      });
      const constraints =
        height === null
          ? new BoxConstraints({ maxWidth: 800, maxHeight: 600 })
          : new BoxConstraints({ maxWidth: 800, minHeight: height, maxHeight: height });
      layoutRoot(flex, constraints);
      return flex;
    };
    expect(column(null).overflow).toBe(0);
    expect(column(100).overflow).toBe(100);
    // Flutter paints no indicator for an empty flex; the amount is still reported.
    const empty = column(0);
    expect(empty.size.height).toBe(0);
    expect(empty.overflow).toBe(200);
  });
});

describe("[STU-005] RenderFlex: alignment, direction and fit", () => {
  // Three 100×100 boxes in a 500×500 flex: free space 200.
  const ltr: Record<MainAxisAlignment, number[]> = {
    [MainAxisAlignment.start]: [0, 100, 200],
    [MainAxisAlignment.end]: [200, 300, 400],
    [MainAxisAlignment.center]: [100, 200, 300],
    [MainAxisAlignment.spaceBetween]: [0, 200, 400],
    [MainAxisAlignment.spaceAround]: [100 / 3, 200, 400 - 100 / 3],
    [MainAxisAlignment.spaceEvenly]: [50, 200, 350],
  };
  const rtl: Record<MainAxisAlignment, number[]> = {
    [MainAxisAlignment.start]: [400, 300, 200],
    [MainAxisAlignment.end]: [200, 100, 0],
    [MainAxisAlignment.center]: [300, 200, 100],
    [MainAxisAlignment.spaceBetween]: [400, 200, 0],
    [MainAxisAlignment.spaceAround]: [400 - 100 / 3, 200, 100 / 3],
    [MainAxisAlignment.spaceEvenly]: [350, 200, 50],
  };
  const layouts: [string, Axis, TextDirection, VerticalDirection, Record<MainAxisAlignment, number[]>][] = [
    ["row LTR", Axis.horizontal, TextDirection.ltr, VerticalDirection.down, ltr],
    ["row RTL", Axis.horizontal, TextDirection.rtl, VerticalDirection.down, rtl],
    ["column down", Axis.vertical, TextDirection.ltr, VerticalDirection.down, ltr],
    ["column up", Axis.vertical, TextDirection.ltr, VerticalDirection.up, rtl],
  ];
  for (const [name, direction, textDirection, verticalDirection, expected] of layouts) {
    for (const alignment of Object.values(MainAxisAlignment)) {
      test(`${name} ${alignment}`, () => {
        const boxes = [sized(), sized(), sized()];
        const flex = new RenderFlex({
          direction,
          textDirection,
          verticalDirection,
          mainAxisAlignment: alignment,
          children: boxes,
        });
        layoutRoot(flex, BoxConstraints.tight(new Size(500, 500)));
        boxes.forEach((box, i) => {
          const offset = getOffset(box);
          const main = direction === Axis.horizontal ? offset.dx : offset.dy;
          const cross = direction === Axis.horizontal ? offset.dy : offset.dx;
          expect(main).toBeCloseTo(expected[alignment][i] ?? Number.NaN, 10);
          expect(cross).toBe(200);
        });
        expect(flex.getDryLayout(BoxConstraints.tight(new Size(500, 500)))).toEqual(flex.size);
      });
    }
  }

  test("spaceBetween and spaceAround with fewer children fall back to start", () => {
    const one = sized();
    const flex = new RenderFlex({
      textDirection: TextDirection.rtl,
      mainAxisAlignment: MainAxisAlignment.spaceBetween,
      children: [one],
    });
    layoutRoot(flex, BoxConstraints.tight(new Size(500, 500)));
    expect(getOffset(one).dx).toBe(400);
    const none = new RenderFlex({ mainAxisAlignment: MainAxisAlignment.spaceAround, spacing: 10 });
    layoutRoot(none, new BoxConstraints({ maxWidth: 500, maxHeight: 500 }));
    expect(none.size).toEqual(new Size(500, 0));
    none.mainAxisSize = MainAxisSize.min;
    layoutRoot(none, new BoxConstraints({ maxWidth: 500, maxHeight: 500 }));
    expect(none.size).toEqual(new Size(0, 0));
  });

  test("tight fit fills its share; loose fit may be smaller", () => {
    const tight = new RenderTestBox({ size: new Size(50, 50) });
    const loose = new RenderTestBox({ size: new Size(50, 50) });
    const fixed = sized();
    const flex = new RenderFlex({ textDirection: TextDirection.ltr, children: [tight, fixed, loose] });
    setFlex(tight, 1);
    setFlex(loose, 3, FlexFit.loose);
    layoutRoot(flex, new BoxConstraints({ maxWidth: 500, maxHeight: 300 }));
    expect(tight.size).toEqual(new Size(100, 50));
    expect(loose.size).toEqual(new Size(50, 50));
    expect([getOffset(tight).dx, getOffset(fixed).dx, getOffset(loose).dx]).toEqual([0, 100, 200]);
    expect(flex.size).toEqual(new Size(500, 100));
    expect([getOffset(tight).dy, getOffset(fixed).dy, getOffset(loose).dy]).toEqual([25, 0, 25]);
  });

  test("stretch tightens the cross axis for flexible and inflexible children", () => {
    const fixed = new RenderTestBox({ size: new Size(40, 40) });
    const flexible = new RenderTestBox({ size: new Size(40, 40) });
    const flex = new RenderFlex({
      direction: Axis.vertical,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [fixed, flexible],
    });
    setFlex(flexible, 1, FlexFit.loose);
    layoutRoot(flex, new BoxConstraints({ maxWidth: 120, maxHeight: 300 }));
    expect(fixed.size).toEqual(new Size(120, 40));
    expect(flexible.size).toEqual(new Size(120, 40));
    expect(flex.size).toEqual(new Size(120, 300));
  });

  test("spacing with flexible children shrinks the flex space", () => {
    const a = sized();
    const b = new RenderTestBox({ size: new Size(10, 10) });
    const flex = new RenderFlex({ textDirection: TextDirection.ltr, spacing: 20, children: [a, b] });
    setFlex(b, 1);
    layoutRoot(flex, new BoxConstraints({ maxWidth: 400, maxHeight: 100 }));
    expect(b.size.width).toBe(280);
    expect(getOffset(b).dx).toBe(120);
    expect(flex.overflow).toBe(0);
  });
});

describe("[STU-005] RenderFlex: baselines, overflow, dry layout", () => {
  test("baseline alignment with RenderTestBox baselines", () => {
    const a = new RenderTestBox({ size: new Size(100, 50), baseline: 40 });
    const b = new RenderTestBox({ size: new Size(100, 100), baseline: 20 });
    const c = new RenderTestBox({ size: new Size(50, 30) });
    const flex = new RenderFlex({
      textDirection: TextDirection.ltr,
      crossAxisAlignment: CrossAxisAlignment.baseline,
      textBaseline: alphabetic,
      children: [a, b, c],
    });
    const constraints = new BoxConstraints({ maxWidth: 800, maxHeight: 600 });
    layoutRoot(flex, constraints);
    expect(flex.size).toEqual(new Size(800, 120));
    expect([getOffset(a).dy, getOffset(b).dy, getOffset(c).dy]).toEqual([0, 20, 0]);
    expect([getOffset(a).dx, getOffset(b).dx, getOffset(c).dx]).toEqual([0, 100, 200]);
    expect(flex.getDistanceToActualBaseline(alphabetic)).toBe(40);
    expect(flex.getDryBaseline(constraints, alphabetic)).toBe(40);
    expect(flex.getDryLayout(constraints)).toEqual(flex.size);
  });

  test("a baseline-aligned row without textBaseline throws, as in release Flutter", () => {
    const flex = new RenderFlex({ crossAxisAlignment: CrossAxisAlignment.baseline, children: [sized()] });
    expect(() => flex.getDryLayout(new BoxConstraints({ maxWidth: 100, maxHeight: 100 }))).toThrow(/textBaseline/);
    const column = new RenderFlex({
      direction: Axis.vertical,
      crossAxisAlignment: CrossAxisAlignment.baseline,
      children: [sized()],
    });
    expect(column.getDryLayout(new BoxConstraints({ maxWidth: 100, maxHeight: 100 }))).toEqual(new Size(100, 100));
  });

  for (const [alignment, verticalDirection] of [
    [CrossAxisAlignment.center, VerticalDirection.down],
    [CrossAxisAlignment.start, VerticalDirection.up],
    [CrossAxisAlignment.end, VerticalDirection.down],
  ] as const) {
    test(`row dry baseline matches the laid-out baseline (${alignment}, ${verticalDirection})`, () => {
      const a = new RenderTestBox({ size: new Size(50, 60), baseline: 30 });
      const b = new RenderTestBox({ size: new Size(50, 20), baseline: 15 });
      const c = new RenderTestBox({ size: new Size(50, 20) });
      const flex = new RenderFlex({
        textDirection: TextDirection.rtl,
        crossAxisAlignment: alignment,
        verticalDirection,
        children: [a, b, c],
      });
      setFlex(c, 1, FlexFit.loose);
      const constraints = new BoxConstraints({ maxWidth: 300, maxHeight: 200 });
      layoutRoot(flex, constraints);
      expect(flex.getDryBaseline(constraints, alphabetic)).toBe(flex.getDistanceToActualBaseline(alphabetic));
      expect(flex.getDryLayout(constraints)).toEqual(flex.size);
    });
  }

  test("column dry baseline with a flexible child matches layout; none without baselines", () => {
    const a = new RenderTestBox({ size: new Size(50, 60) });
    const b = new RenderTestBox({ size: new Size(50, 20), baseline: 15 });
    const flex = new RenderFlex({
      direction: Axis.vertical,
      mainAxisAlignment: MainAxisAlignment.center,
      children: [a, b],
    });
    setFlex(a, 1);
    const constraints = new BoxConstraints({ maxWidth: 300, maxHeight: 200 });
    layoutRoot(flex, constraints);
    expect(flex.getDryBaseline(constraints, alphabetic)).toBe(195);
    expect(flex.getDistanceToActualBaseline(alphabetic)).toBe(195);
    const bare = new RenderFlex({ direction: Axis.vertical, children: [sized()] });
    expect(bare.getDryBaseline(constraints, alphabetic)).toBeNull();
    const bareRow = new RenderFlex({ children: [sized()] });
    expect(bareRow.getDryBaseline(constraints, alphabetic)).toBeNull();
  });

  test("overflow amount for rows and MainAxisSize.min", () => {
    for (const mainAxisSize of [MainAxisSize.max, MainAxisSize.min]) {
      const flex = new RenderFlex({
        textDirection: TextDirection.ltr,
        mainAxisSize,
        spacing: 5,
        children: [sized(), sized(), sized()],
      });
      layoutRoot(flex, new BoxConstraints({ maxWidth: 250, maxHeight: 100 }));
      expect(flex.size.width).toBe(250);
      expect(flex.overflow).toBe(60);
    }
  });

  test("dry layout equals real layout", () => {
    const configs: [Axis, MainAxisSize, CrossAxisAlignment, BoxConstraints][] = [
      [
        Axis.horizontal,
        MainAxisSize.max,
        CrossAxisAlignment.center,
        new BoxConstraints({ maxWidth: 400, maxHeight: 300 }),
      ],
      [
        Axis.horizontal,
        MainAxisSize.min,
        CrossAxisAlignment.stretch,
        new BoxConstraints({ maxWidth: 400, maxHeight: 300 }),
      ],
      [Axis.vertical, MainAxisSize.min, CrossAxisAlignment.end, new BoxConstraints({ maxWidth: 400, maxHeight: inf })],
      [Axis.vertical, MainAxisSize.max, CrossAxisAlignment.start, BoxConstraints.tight(new Size(120, 90))],
    ];
    for (const [direction, mainAxisSize, crossAxisAlignment, constraints] of configs) {
      const flexible = new RenderTestBox({ size: new Size(30, 30) });
      const flex = new RenderFlex({
        direction,
        mainAxisSize,
        crossAxisAlignment,
        spacing: 4,
        children: [
          new RenderTestBox({ size: new Size(60, 40) }),
          flexible,
          new RenderTestBox({ size: new Size(20, 70) }),
        ],
      });
      setFlex(flexible, 2, FlexFit.loose);
      const dry = flex.getDryLayout(constraints);
      layoutRoot(flex, constraints);
      expect(dry).toEqual(flex.size);
    }
  });

  test("main-axis intrinsics use the largest flex fraction", () => {
    const a = new RenderTestBox({ size: new Size(30, 10) });
    const b = new RenderTestBox({ size: new Size(50, 20) });
    const c = new RenderTestBox({ size: new Size(10, 40) });
    const flex = new RenderFlex({ textDirection: TextDirection.ltr, children: [a, b, c] });
    setFlex(a, 1);
    setFlex(b, 2);
    // max(30/1, 50/2) * 3 + 10.
    expect(flex.getMinIntrinsicWidth(inf)).toBe(100);
    expect(flex.getMaxIntrinsicHeight(inf)).toBe(40);
    const column = new RenderFlex({
      direction: Axis.vertical,
      children: [new RenderTestBox({ size: new Size(30, 10) })],
    });
    expect(column.getMaxIntrinsicWidth(inf)).toBe(30);
  });

  test("setters relayout only on change and a foreign child has no flex parent data", () => {
    const box = sized();
    const flex = new RenderFlex({ children: [box] });
    layoutRoot(flex, new BoxConstraints({ maxWidth: 300, maxHeight: 300 }));
    flex.direction = Axis.horizontal;
    flex.mainAxisAlignment = MainAxisAlignment.start;
    flex.mainAxisSize = MainAxisSize.max;
    flex.crossAxisAlignment = CrossAxisAlignment.center;
    flex.textDirection = null;
    flex.verticalDirection = VerticalDirection.down;
    flex.textBaseline = null;
    flex.spacing = 0;
    expect(flex.needsLayout).toBe(false);
    flex.textBaseline = alphabetic;
    flex.spacing = 3;
    expect(flex.needsLayout).toBe(true);
    const stranger = sized();
    stranger.parentData = null;
    const other = new RenderFlex();
    expect(() => {
      other.setupParentData(stranger);
      parentData(stranger);
    }).not.toThrow();
    const orphan = new RenderTestBox({ size: Size.zero });
    const host = new RenderFlex({ children: [orphan] });
    orphan.parentData = null;
    expect(() => host.getMinIntrinsicWidth(10)).toThrow(/FlexParentData/);
  });
});
