// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { BoxConstraints } from "./flutter/constraints.ts";
import { Axis, VerticalDirection } from "./flutter/flex.ts";
import { Size, TextDirection } from "./flutter/geometry.ts";
import { RenderConstrainedBox } from "./flutter/proxy-box.ts";
import { RenderBox, TextBaseline } from "./flutter/render-box.ts";
import { RenderWrap, WrapAlignment, WrapCrossAlignment } from "./flutter/wrap.ts";
import { RenderTestBox } from "./test-box.ts";

const inf = Number.POSITIVE_INFINITY;
const alphabetic = TextBaseline.alphabetic;

function layoutRoot(box: RenderBox, constraints: BoxConstraints): void {
  box.markNeedsLayout();
  box.layout(constraints, { parentUsesSize: true });
}

function box(width: number, height: number, baseline?: number): RenderTestBox {
  return new RenderTestBox(
    baseline === undefined ? { size: new Size(width, height) } : { size: new Size(width, height), baseline },
  );
}

function min80(): RenderConstrainedBox {
  return new RenderConstrainedBox({ additionalConstraints: new BoxConstraints({ minWidth: 80, minHeight: 80 }) });
}

function offsets(wrap: RenderWrap): [number, number][] {
  return wrap.getChildrenAsList().map((c) => {
    const o = RenderBox.offsetOf(c);
    return [o.dx, o.dy];
  });
}

describe("STU-005 RenderWrap intrinsics (wrap_test.dart)", () => {
  test("Compute intrinsic height test", () => {
    const wrap = new RenderWrap();
    for (const c of [min80(), min80(), min80()]) {
      wrap.add(c);
    }
    wrap.spacing = 5;
    wrap.runSpacing = 5;
    wrap.direction = Axis.horizontal;
    expect(wrap.getMaxIntrinsicHeight(245)).toBe(165);
    expect(wrap.getMaxIntrinsicHeight(250)).toBe(80);
    expect(wrap.getMaxIntrinsicHeight(80)).toBe(250);
    expect(wrap.getMaxIntrinsicHeight(79)).toBe(250);
    expect(wrap.getMinIntrinsicHeight(245)).toBe(165);
    expect(wrap.getMinIntrinsicHeight(250)).toBe(80);
    expect(wrap.getMinIntrinsicHeight(80)).toBe(250);
    expect(wrap.getMinIntrinsicHeight(79)).toBe(250);
  });

  test("Compute intrinsic width test", () => {
    const wrap = new RenderWrap();
    for (const c of [min80(), min80(), min80()]) {
      wrap.add(c);
    }
    wrap.spacing = 5;
    wrap.runSpacing = 5;
    wrap.direction = Axis.vertical;
    expect(wrap.getMaxIntrinsicWidth(245)).toBe(165);
    expect(wrap.getMaxIntrinsicWidth(250)).toBe(80);
    expect(wrap.getMaxIntrinsicWidth(80)).toBe(250);
    expect(wrap.getMaxIntrinsicWidth(79)).toBe(250);
    expect(wrap.getMinIntrinsicWidth(245)).toBe(165);
    expect(wrap.getMinIntrinsicWidth(250)).toBe(80);
    expect(wrap.getMinIntrinsicWidth(80)).toBe(250);
    expect(wrap.getMinIntrinsicWidth(79)).toBe(250);
  });

  test("Compute intrinsic height for only one run", () => {
    const wrap = new RenderWrap({ spacing: 5, runSpacing: 5, children: [min80()] });
    for (const w of [100, 79, 80]) {
      expect(wrap.getMaxIntrinsicHeight(w)).toBe(80);
      expect(wrap.getMinIntrinsicHeight(w)).toBe(80);
    }
  });

  test("Compute intrinsic width for only one run", () => {
    const wrap = new RenderWrap({ direction: Axis.vertical, spacing: 5, runSpacing: 5, children: [min80()] });
    for (const h of [100, 79, 80]) {
      expect(wrap.getMaxIntrinsicWidth(h)).toBe(80);
      expect(wrap.getMinIntrinsicWidth(h)).toBe(80);
    }
  });

  test("main-axis intrinsics: max of min widths, sum of max widths", () => {
    const children = [
      new RenderTestBox({ size: new Size(1, 1), minIntrinsicWidth: 10, maxIntrinsicWidth: 30 }),
      new RenderTestBox({ size: new Size(1, 1), minIntrinsicWidth: 20, maxIntrinsicWidth: 25 }),
    ];
    const wrap = new RenderWrap({ children });
    expect(wrap.getMinIntrinsicWidth(inf)).toBe(20);
    expect(wrap.getMaxIntrinsicWidth(inf)).toBe(55);
    const vertical = new RenderWrap({
      direction: Axis.vertical,
      children: [
        new RenderTestBox({ size: new Size(1, 1), minIntrinsicHeight: 10, maxIntrinsicHeight: 30 }),
        new RenderTestBox({ size: new Size(1, 1), minIntrinsicHeight: 20, maxIntrinsicHeight: 25 }),
      ],
    });
    expect(vertical.getMinIntrinsicHeight(inf)).toBe(20);
    expect(vertical.getMaxIntrinsicHeight(inf)).toBe(55);
  });
});

describe("STU-005 RenderWrap layout", () => {
  const constraints = new BoxConstraints({ maxWidth: 100, maxHeight: 400 });

  function threeRuns(options: ConstructorParameters<typeof RenderWrap>[0] = {}): RenderWrap {
    return new RenderWrap({
      textDirection: TextDirection.ltr,
      spacing: 10,
      runSpacing: 5,
      children: [box(40, 20), box(40, 30), box(60, 10), box(30, 10)],
      ...options,
    });
  }

  test("no children takes the smallest size", () => {
    const wrap = new RenderWrap();
    layoutRoot(wrap, new BoxConstraints({ minWidth: 7, minHeight: 9, maxWidth: 50, maxHeight: 50 }));
    expect(wrap.size).toEqual(new Size(7, 9));
    expect(wrap.hasVisualOverflow).toBe(false);
  });

  test("runs with spacing and runSpacing, start alignment", () => {
    const wrap = threeRuns();
    layoutRoot(wrap, constraints);
    // Runs: [40, 40] (90 wide, 30 tall), [60, 30] (100 wide, 10 tall).
    expect(wrap.size).toEqual(new Size(100, 45));
    expect(offsets(wrap)).toEqual([
      [0, 0],
      [50, 0],
      [0, 35],
      [70, 35],
    ]);
    expect(wrap.getDryLayout(constraints)).toEqual(wrap.size);
    expect(wrap.hasVisualOverflow).toBe(false);
  });

  const mainCases: [WrapAlignment, [number, number][]][] = [
    [
      WrapAlignment.start,
      [
        [0, 0],
        [50, 0],
        [0, 35],
        [70, 35],
      ],
    ],
    [
      WrapAlignment.end,
      [
        [10, 0],
        [60, 0],
        [0, 35],
        [70, 35],
      ],
    ],
    [
      WrapAlignment.center,
      [
        [5, 0],
        [55, 0],
        [0, 35],
        [70, 35],
      ],
    ],
    [
      WrapAlignment.spaceBetween,
      [
        [0, 0],
        [60, 0],
        [0, 35],
        [70, 35],
      ],
    ],
    [
      WrapAlignment.spaceAround,
      [
        [2.5, 0],
        [57.5, 0],
        [0, 35],
        [70, 35],
      ],
    ],
    [
      WrapAlignment.spaceEvenly,
      [
        [10 / 3, 0],
        [50 + 20 / 3, 0],
        [0, 35],
        [70, 35],
      ],
    ],
  ];
  for (const [alignment, expected] of mainCases) {
    test(`alignment ${alignment} (LTR)`, () => {
      const wrap = threeRuns({ alignment });
      layoutRoot(wrap, constraints);
      const got = offsets(wrap);
      expected.forEach(([x, y], i) => {
        expect(got[i]?.[0]).toBeCloseTo(x, 10);
        expect(got[i]?.[1]).toBeCloseTo(y, 10);
      });
    });
  }

  test("alignment under RTL mirrors the main axis", () => {
    const wrap = threeRuns({ textDirection: TextDirection.rtl });
    layoutRoot(wrap, constraints);
    expect(offsets(wrap)).toEqual([
      [60, 0],
      [10, 0],
      [40, 35],
      [0, 35],
    ]);
    wrap.alignment = WrapAlignment.end;
    layoutRoot(wrap, constraints);
    expect(offsets(wrap).slice(0, 2)).toEqual([
      [50, 0],
      [0, 0],
    ]);
    const single = new RenderWrap({
      textDirection: TextDirection.rtl,
      alignment: WrapAlignment.spaceBetween,
      children: [box(10, 10)],
    });
    layoutRoot(single, constraints);
    expect(offsets(single)).toEqual([[0, 0]]);
  });

  const runCases: [WrapAlignment, number[]][] = [
    [WrapAlignment.start, [0, 35]],
    [WrapAlignment.end, [55, 90]],
    [WrapAlignment.center, [27.5, 62.5]],
    [WrapAlignment.spaceBetween, [0, 90]],
    [WrapAlignment.spaceAround, [13.75, 76.25]],
    [WrapAlignment.spaceEvenly, [55 / 3, 35 + 110 / 3]],
  ];
  for (const [runAlignment, ys] of runCases) {
    test(`runAlignment ${runAlignment} in a tight box`, () => {
      const wrap = threeRuns({ runAlignment });
      layoutRoot(wrap, BoxConstraints.tight(new Size(100, 100)));
      const got = offsets(wrap);
      expect(got[0]?.[1]).toBeCloseTo(ys[0] ?? 0, 10);
      expect(got[2]?.[1]).toBeCloseTo(ys[1] ?? 0, 10);
    });
  }

  test("crossAxisAlignment within a run, and verticalDirection up flips it", () => {
    const wrap = threeRuns({ crossAxisAlignment: WrapCrossAlignment.end });
    layoutRoot(wrap, constraints);
    expect(offsets(wrap).slice(0, 2)).toEqual([
      [0, 10],
      [50, 0],
    ]);
    wrap.crossAxisAlignment = WrapCrossAlignment.center;
    layoutRoot(wrap, constraints);
    expect(offsets(wrap)[0]).toEqual([0, 5]);
    wrap.crossAxisAlignment = WrapCrossAlignment.start;
    wrap.verticalDirection = VerticalDirection.up;
    layoutRoot(wrap, constraints);
    // Runs reversed: the second run at the top; start aligns to the bottom of a run.
    expect(offsets(wrap)).toEqual([
      [0, 25],
      [50, 15],
      [0, 0],
      [70, 0],
    ]);
  });

  test("vertical direction, LTR and RTL", () => {
    const children = [box(20, 40), box(30, 40), box(10, 60), box(10, 30)];
    const wrap = new RenderWrap({
      direction: Axis.vertical,
      spacing: 10,
      runSpacing: 5,
      textDirection: TextDirection.ltr,
      children,
    });
    const c = new BoxConstraints({ maxWidth: 400, maxHeight: 100 });
    layoutRoot(wrap, c);
    expect(wrap.size).toEqual(new Size(45, 100));
    expect(offsets(wrap)).toEqual([
      [0, 0],
      [0, 50],
      [35, 0],
      [35, 70],
    ]);
    expect(wrap.getDryLayout(c)).toEqual(wrap.size);
    wrap.textDirection = TextDirection.rtl;
    layoutRoot(wrap, c);
    expect(offsets(wrap)).toEqual([
      [25, 0],
      [15, 50],
      [0, 0],
      [0, 70],
    ]);
    expect(wrap.direction).toBe(Axis.vertical);
    expect(wrap.textDirection).toBe(TextDirection.rtl);
  });

  test("a child wider than the wrap is clamped and starts its own run; runs overflow the cross axis", () => {
    const wrap = new RenderWrap({ textDirection: TextDirection.ltr, children: [box(150, 10), box(10, 10)] });
    layoutRoot(wrap, constraints);
    expect(wrap.size).toEqual(new Size(100, 20));
    expect(wrap.hasVisualOverflow).toBe(false);
    layoutRoot(wrap, new BoxConstraints({ maxWidth: 100, maxHeight: 15 }));
    expect(wrap.size).toEqual(new Size(100, 15));
    expect(offsets(wrap)).toEqual([
      [0, 0],
      [0, 10],
    ]);
    expect(wrap.hasVisualOverflow).toBe(true);
  });

  test("baselines: highest child baseline, dry and actual", () => {
    const wrap = new RenderWrap({
      textDirection: TextDirection.ltr,
      crossAxisAlignment: WrapCrossAlignment.end,
      children: [box(40, 20, 15), box(40, 30, 25), box(60, 10, 2)],
    });
    expect(wrap.getDryBaseline(constraints, alphabetic)).toBe(25);
    layoutRoot(wrap, constraints);
    expect(wrap.getDistanceToBaseline(alphabetic)).toBe(25);
    expect(new RenderWrap().getDryBaseline(constraints, alphabetic)).toBeNull();
  });

  test("setters keep their values", () => {
    const wrap = new RenderWrap();
    wrap.spacing = 3;
    wrap.runSpacing = 4;
    wrap.runAlignment = WrapAlignment.center;
    wrap.alignment = WrapAlignment.end;
    expect([wrap.spacing, wrap.runSpacing, wrap.runAlignment, wrap.alignment]).toEqual([
      3,
      4,
      WrapAlignment.center,
      WrapAlignment.end,
    ]);
    expect(wrap.crossAxisAlignment).toBe(WrapCrossAlignment.start);
    expect(wrap.verticalDirection).toBe(VerticalDirection.down);
  });
});
