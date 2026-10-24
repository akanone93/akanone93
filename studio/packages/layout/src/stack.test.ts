// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { BoxConstraints } from "./flutter/constraints.ts";
import { MainAxisAlignment, RenderFlex } from "./flutter/flex.ts";
import { Alignment, AlignmentDirectional, Offset, Size, TextDirection } from "./flutter/geometry.ts";
import { RenderConstrainedBox, RenderDecoratedBox } from "./flutter/proxy-box.ts";
import { RenderBox, TextBaseline } from "./flutter/render-box.ts";
import { RenderIndexedStack, RenderStack, StackFit, StackParentData } from "./flutter/stack.ts";
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

function pd(child: RenderBox): StackParentData {
  return child.parentData as StackParentData;
}

function at(child: RenderBox): Offset {
  return RenderBox.offsetOf(child);
}

const tight200 = BoxConstraints.tight(new Size(200, 100));

describe("STU-005 StackParentData (stack_test.dart)", () => {
  test("StackParentData basic test", () => {
    const parentData = new StackParentData();
    const stackSize = new Size(800, 600);
    expect(parentData.isPositioned).toBe(false);
    parentData.width = -100;
    expect(parentData.isPositioned).toBe(true);
    expect(parentData.positionedChildConstraints(stackSize)).toEqual(BoxConstraints.tightFor({ width: 0 }));
    parentData.width = 100;
    expect(parentData.positionedChildConstraints(stackSize)).toEqual(BoxConstraints.tightFor({ width: 100 }));
    parentData.left = 0;
    parentData.right = 0;
    expect(parentData.positionedChildConstraints(stackSize)).toEqual(BoxConstraints.tightFor({ width: 800 }));
    parentData.height = -100;
    expect(parentData.positionedChildConstraints(stackSize)).toEqual(
      BoxConstraints.tightFor({ width: 800, height: 0 }),
    );
    parentData.height = 100;
    expect(parentData.positionedChildConstraints(stackSize)).toEqual(
      BoxConstraints.tightFor({ width: 800, height: 100 }),
    );
    parentData.top = 0;
    parentData.bottom = 0;
    expect(parentData.positionedChildConstraints(stackSize)).toEqual(
      BoxConstraints.tightFor({ width: 800, height: 600 }),
    );
    parentData.bottom = 1000;
    expect(parentData.positionedChildConstraints(stackSize)).toEqual(
      BoxConstraints.tightFor({ width: 800, height: 0 }),
    );
  });

  test("setDirectional resolves start and end by text direction", () => {
    const parentData = new StackParentData();
    parentData.setDirectional(TextDirection.ltr, 10, 20);
    expect([parentData.left, parentData.right]).toEqual([10, 20]);
    parentData.setDirectional(TextDirection.rtl, 10, 20);
    expect([parentData.left, parentData.right]).toEqual([20, 10]);
  });
});

describe("STU-005 RenderStack (stack_test.dart)", () => {
  test("Stack can layout with top, right, bottom, left 0.0", () => {
    const size = new RenderConstrainedBox({ additionalConstraints: BoxConstraints.tight(new Size(100, 100)) });
    const red = new RenderDecoratedBox({ decoration: { color: "#f00" }, child: size });
    const green = new RenderDecoratedBox({ decoration: { color: "#f00" } });
    const stack = new RenderStack({ textDirection: TextDirection.ltr, children: [red, green] });
    Object.assign(pd(green), { top: 0, right: 0, bottom: 0, left: 0 });
    layoutRoot(stack, new BoxConstraints());
    expect(stack.size).toEqual(new Size(100, 100));
    expect(red.size).toEqual(new Size(100, 100));
    expect(green.size).toEqual(new Size(100, 100));
  });

  test("Stack can layout with no children", () => {
    const stack = new RenderStack({ textDirection: TextDirection.ltr, children: [] });
    layoutRoot(stack, BoxConstraints.tight(new Size(100, 100)));
    expect(stack.size).toEqual(new Size(100, 100));
  });

  test("no children under unbounded constraints takes the smallest size", () => {
    const stack = new RenderStack({ textDirection: TextDirection.ltr });
    layoutRoot(stack, new BoxConstraints({ minWidth: 10, minHeight: 20 }));
    expect(stack.size).toEqual(new Size(10, 20));
  });

  test("Stack in Flex can layout with no children", () => {
    const stack = new RenderStack({ textDirection: TextDirection.ltr, children: [] });
    const flex = new RenderFlex({
      textDirection: TextDirection.ltr,
      mainAxisAlignment: MainAxisAlignment.spaceEvenly,
      children: [stack],
    });
    expect(() => layoutRoot(flex, BoxConstraints.tight(new Size(100, 100)))).not.toThrow();
  });

  test("only positioned children: the stack takes the biggest size (release: infinite when unbounded)", () => {
    const child = box(10, 10);
    const stack = new RenderStack({ textDirection: TextDirection.ltr, children: [child] });
    pd(child).left = 5;
    layoutRoot(stack, new BoxConstraints({ maxWidth: 300, maxHeight: 200 }));
    expect(stack.size).toEqual(new Size(300, 200));
    layoutRoot(stack, new BoxConstraints());
    expect(stack.size).toEqual(new Size(inf, inf));
  });

  test("StackFit loose, expand and passthrough size the non-positioned children", () => {
    const child = box(50, 40);
    const stack = new RenderStack({ textDirection: TextDirection.ltr, children: [child] });
    const constraints = new BoxConstraints({ minWidth: 100, maxWidth: 200, minHeight: 60, maxHeight: 100 });
    layoutRoot(stack, constraints);
    expect(child.size).toEqual(new Size(50, 40));
    expect(stack.size).toEqual(new Size(100, 60));
    stack.fit = StackFit.expand;
    layoutRoot(stack, constraints);
    expect(child.size).toEqual(new Size(200, 100));
    expect(stack.size).toEqual(new Size(200, 100));
    stack.fit = StackFit.passthrough;
    layoutRoot(stack, constraints);
    expect(child.size).toEqual(new Size(100, 60));
    expect(stack.fit).toBe(StackFit.passthrough);
  });

  test("directional alignment under LTR and RTL", () => {
    const child = box(50, 40);
    const stack = new RenderStack({
      alignment: AlignmentDirectional.bottomEnd,
      textDirection: TextDirection.ltr,
      fit: StackFit.passthrough,
      children: [child],
    });
    layoutRoot(stack, tight200);
    expect(at(child)).toEqual(new Offset(0, 0));
    stack.fit = StackFit.loose;
    layoutRoot(stack, tight200);
    expect(at(child)).toEqual(new Offset(150, 60));
    stack.textDirection = TextDirection.rtl;
    layoutRoot(stack, tight200);
    expect(at(child)).toEqual(new Offset(0, 60));
    expect(stack.textDirection).toBe(TextDirection.rtl);
    stack.alignment = Alignment.center;
    layoutRoot(stack, tight200);
    expect(at(child)).toEqual(new Offset(75, 30));
    expect(stack.alignment).toBe(Alignment.center);
  });

  test("a directional alignment without a text direction throws", () => {
    const stack = new RenderStack({ children: [box(10, 10)] });
    expect(() => layoutRoot(stack, tight200)).toThrow();
  });

  test("positioned children: edges, extents, alignment fallback and overflow", () => {
    const base = box(200, 100);
    const leftTop = box(10, 10);
    const rightBottom = box(10, 10);
    const sized = box(1, 1);
    const onlyTop = box(20, 20);
    const stack = new RenderStack({
      alignment: Alignment.center,
      textDirection: TextDirection.ltr,
      children: [base, leftTop, rightBottom, sized, onlyTop],
    });
    Object.assign(pd(leftTop), { left: 5, top: 6 });
    Object.assign(pd(rightBottom), { right: 5, bottom: 6 });
    Object.assign(pd(sized), { width: 30, height: 40 });
    Object.assign(pd(onlyTop), { top: 90 });
    layoutRoot(stack, new BoxConstraints({ maxWidth: 400, maxHeight: 400 }));
    expect(stack.size).toEqual(new Size(200, 100));
    expect(at(leftTop)).toEqual(new Offset(5, 6));
    expect(at(rightBottom)).toEqual(new Offset(185, 84));
    expect(sized.size).toEqual(new Size(30, 40));
    expect(at(sized)).toEqual(new Offset(85, 30));
    expect(at(onlyTop)).toEqual(new Offset(90, 90));
    expect(stack.hasVisualOverflow).toBe(true);
    pd(onlyTop).top = 0;
    layoutRoot(stack, new BoxConstraints({ maxWidth: 400, maxHeight: 400 }));
    expect(stack.hasVisualOverflow).toBe(false);
    pd(onlyTop).top = -1;
    layoutRoot(stack, new BoxConstraints({ maxWidth: 400, maxHeight: 400 }));
    expect(stack.hasVisualOverflow).toBe(true);
    pd(onlyTop).top = null;
    pd(onlyTop).left = 190;
    layoutRoot(stack, new BoxConstraints({ maxWidth: 400, maxHeight: 400 }));
    expect(at(onlyTop)).toEqual(new Offset(190, 40));
    expect(stack.hasVisualOverflow).toBe(true);
  });

  test("intrinsics take the largest non-positioned child", () => {
    const a = new RenderTestBox({
      size: new Size(10, 10),
      minIntrinsicWidth: 10,
      maxIntrinsicWidth: 30,
      minIntrinsicHeight: 5,
      maxIntrinsicHeight: 15,
    });
    const b = new RenderTestBox({
      size: new Size(10, 10),
      minIntrinsicWidth: 20,
      maxIntrinsicWidth: 25,
      minIntrinsicHeight: 8,
      maxIntrinsicHeight: 12,
    });
    const positioned = new RenderTestBox({
      size: new Size(10, 10),
      minIntrinsicWidth: 999,
      maxIntrinsicWidth: 999,
      minIntrinsicHeight: 999,
      maxIntrinsicHeight: 999,
    });
    const stack = new RenderStack({ textDirection: TextDirection.ltr, children: [a, b, positioned] });
    pd(positioned).left = 0;
    expect(stack.getMinIntrinsicWidth(inf)).toBe(20);
    expect(stack.getMaxIntrinsicWidth(inf)).toBe(30);
    expect(stack.getMinIntrinsicHeight(inf)).toBe(8);
    expect(stack.getMaxIntrinsicHeight(inf)).toBe(15);
  });

  test("dry layout matches layout", () => {
    const stack = new RenderStack({ textDirection: TextDirection.ltr, children: [box(50, 40), box(30, 70)] });
    const constraints = new BoxConstraints({ maxWidth: 300, maxHeight: 300 });
    expect(stack.getDryLayout(constraints)).toEqual(new Size(50, 70));
    expect(stack.getDryLayout(new BoxConstraints())).toEqual(new Size(50, 70));
    layoutRoot(stack, constraints);
    expect(stack.size).toEqual(new Size(50, 70));
  });

  test("baselines: highest of the children, dry and actual", () => {
    const a = box(50, 40, 30);
    const b = box(20, 20, 15);
    const c = box(10, 10);
    const top = box(10, 10, 4);
    const bottom = box(10, 10, 4);
    const stack = new RenderStack({
      alignment: Alignment.bottomCenter,
      textDirection: TextDirection.ltr,
      children: [a, b, c, top, bottom],
    });
    Object.assign(pd(top), { top: 30 });
    Object.assign(pd(bottom), { bottom: 5 });
    const constraints = new BoxConstraints({ maxWidth: 100, maxHeight: 100 });
    // a: 0+30; b: 20+15; top: 30+4; bottom: 40-5-10+4 = 29.
    expect(stack.getDryBaseline(constraints, alphabetic)).toBe(29);
    layoutRoot(stack, constraints);
    expect(stack.getDistanceToBaseline(alphabetic)).toBe(29);
    const none = new RenderStack({ textDirection: TextDirection.ltr, children: [c] });
    expect(none.getDryBaseline(constraints, alphabetic)).toBeNull();
  });

  test("positioned extents beat the alignment fallback for a missing edge", () => {
    const child = box(1, 1);
    const stack = new RenderStack({ textDirection: TextDirection.rtl, children: [box(100, 100), child] });
    Object.assign(pd(child), { right: 10, width: 20, height: 30 });
    layoutRoot(stack, new BoxConstraints({ maxWidth: 400, maxHeight: 400 }));
    expect(child.size).toEqual(new Size(20, 30));
    expect(at(child)).toEqual(new Offset(70, 0));
  });

  test("a child without StackParentData is rejected", () => {
    const child = box(1, 1);
    expect(() => RenderStack.getIntrinsicDimension(child, () => 0)).toThrow();
  });
});

describe("STU-005 RenderIndexedStack", () => {
  test("every child sizes the stack; index selects the displayed child", () => {
    const a = box(50, 40, 10);
    const b = box(30, 70, 20);
    const stack = new RenderIndexedStack({
      alignment: AlignmentDirectional.topEnd,
      textDirection: TextDirection.rtl,
      children: [a, b],
    });
    expect(stack.index).toBe(0);
    layoutRoot(stack, new BoxConstraints({ maxWidth: 300, maxHeight: 300 }));
    expect(stack.size).toEqual(new Size(50, 70));
    expect(at(a)).toEqual(new Offset(0, 0));
    expect(at(b)).toEqual(new Offset(0, 0));
    expect(stack.displayedChild).toBe(a);
    expect(stack.getDistanceToBaseline(alphabetic)).toBe(10);
    stack.index = 1;
    expect(stack.displayedChild).toBe(b);
    expect(stack.getDryBaseline(new BoxConstraints({ maxWidth: 300, maxHeight: 300 }), alphabetic)).toBe(20);
    layoutRoot(stack, new BoxConstraints({ maxWidth: 300, maxHeight: 300 }));
    expect(stack.getDistanceToBaseline(alphabetic)).toBe(20);
    stack.index = 5;
    expect(stack.displayedChild).toBeNull();
    stack.index = null;
    expect(stack.index).toBeNull();
    expect(stack.displayedChild).toBeNull();
    expect(stack.getDryBaseline(new BoxConstraints({ maxWidth: 300 }), alphabetic)).toBeNull();
    layoutRoot(stack, new BoxConstraints({ maxWidth: 300, maxHeight: 300 }));
    expect(stack.getDistanceToBaseline(alphabetic, { onlyReal: true })).toBeNull();
    expect(new RenderIndexedStack({ index: null }).index).toBeNull();
  });
});
