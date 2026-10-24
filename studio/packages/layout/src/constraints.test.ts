// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { BoxConstraints } from "./constraints.ts";
import { EdgeInsets, EdgeInsetsDirectional, Size, TextDirection } from "./geometry.ts";

const inf = Number.POSITIVE_INFINITY;

function bc(minWidth: number, maxWidth: number, minHeight: number, maxHeight: number) {
  return new BoxConstraints({ minWidth, maxWidth, minHeight, maxHeight });
}

function values(c: BoxConstraints): number[] {
  return [c.minWidth, c.maxWidth, c.minHeight, c.maxHeight];
}

describe("STU-005 BoxConstraints constructors", () => {
  test("defaults are unconstrained", () => {
    expect(values(new BoxConstraints())).toEqual([0, inf, 0, inf]);
  });
  test("tight, loose, tightFor, tightForFinite, expand", () => {
    expect(values(BoxConstraints.tight(new Size(10, 20)))).toEqual([10, 10, 20, 20]);
    expect(values(BoxConstraints.loose(new Size(10, 20)))).toEqual([0, 10, 0, 20]);
    expect(values(BoxConstraints.tightFor({ width: 50 }))).toEqual([50, 50, 0, inf]);
    expect(values(BoxConstraints.tightFor())).toEqual([0, inf, 0, inf]);
    expect(values(BoxConstraints.tightForFinite({ width: inf, height: 10 }))).toEqual([0, inf, 10, 10]);
    expect(values(BoxConstraints.tightForFinite())).toEqual([0, inf, 0, inf]);
    expect(values(BoxConstraints.expand())).toEqual([inf, inf, inf, inf]);
    expect(values(BoxConstraints.expand({ width: 5 }))).toEqual([5, 5, inf, inf]);
  });
  test("copyWith (Flutter box_constraints_test)", () => {
    const c = bc(3, 7, 11, 17);
    expect(c.copyWith().equals(c)).toBe(true);
    expect(values(c.copyWith({ minWidth: 13, maxWidth: 17, minHeight: 111, maxHeight: 117 }))).toEqual([
      13, 17, 111, 117,
    ]);
  });
});

describe("STU-005 BoxConstraints algebra", () => {
  test("operators (Flutter box_constraints_test)", () => {
    const c = bc(3, 7, 11, 17);
    const copy = c.multiply(2);
    expect(values(copy)).toEqual([6, 14, 22, 34]);
    expect(copy.divide(2).equals(c)).toBe(true);
    expect(values(c.truncatingDivide(2))).toEqual([1, 3, 5, 8]);
    expect(values(c.modulo(3))).toEqual([0, 1, 2, 2]);
    expect(values(bc(-1, 7, 0, 0).modulo(3))).toEqual([2, 1, 0, 0]);
  });
  test("normalize (Flutter box_constraints_test)", () => {
    const c = bc(3, 2, 11, 18);
    expect(c.isNormalized).toBe(false);
    expect(values(c.normalize())).toEqual([3, 3, 11, 18]);
    expect(values(bc(-5, 10, -1, -3).normalize())).toEqual([0, 10, 0, 0]);
    const n = bc(0, 1, 0, 1);
    expect(n.normalize()).toBe(n);
  });
  test("deflate never goes below zero and keeps infinity", () => {
    expect(values(bc(10, 100, 5, inf).deflate(EdgeInsets.all(8)))).toEqual([0, 84, 0, inf]);
    expect(values(BoxConstraints.tight(new Size(10, 10)).deflate(EdgeInsets.all(10)))).toEqual([0, 0, 0, 0]);
    const directional = EdgeInsetsDirectional.only({ start: 4, end: 6 });
    expect(values(bc(50, 50, 0, 10).deflate(directional))).toEqual([40, 40, 0, 10]);
    expect(directional.resolve(TextDirection.rtl).left).toBe(6);
  });
  test("loosen, enforce, tighten", () => {
    expect(values(bc(10, 20, 30, 40).loosen())).toEqual([0, 20, 0, 40]);
    expect(values(new BoxConstraints().enforce(BoxConstraints.tight(new Size(100, 50))))).toEqual([100, 100, 50, 50]);
    expect(values(BoxConstraints.tightFor({ width: 200 }).enforce(BoxConstraints.loose(new Size(100, 100))))).toEqual([
      100, 100, 0, 100,
    ]);
    expect(values(bc(10, 20, 30, 40).tighten({ width: 50, height: 35 }))).toEqual([20, 20, 35, 35]);
    expect(values(bc(10, 20, 30, 40).tighten())).toEqual([10, 20, 30, 40]);
  });
  test("flipped, width and height constraints", () => {
    expect(values(bc(1, 2, 3, 4).flipped)).toEqual([3, 4, 1, 2]);
    expect(values(bc(1, 2, 3, 4).widthConstraints())).toEqual([1, 2, 0, inf]);
    expect(values(bc(1, 2, 3, 4).heightConstraints())).toEqual([0, inf, 3, 4]);
  });
  test("constrain and its variants, with Infinity", () => {
    const c = bc(10, 100, 20, inf);
    expect(c.constrainWidth()).toBe(100);
    expect(c.constrainHeight()).toBe(inf);
    expect(c.constrainWidth(5)).toBe(10);
    expect(c.constrain(new Size(500, 5)).equals(new Size(100, 20))).toBe(true);
    expect(c.constrainDimensions(50, 50).equals(new Size(50, 50))).toBe(true);
    expect(c.biggest.equals(new Size(100, inf))).toBe(true);
    expect(c.smallest.equals(new Size(10, 20))).toBe(true);
  });
  test("constrainSizeAndAttemptToPreserveAspectRatio", () => {
    // Flutter box_constraints_test: an empty size is constrained directly.
    expect(bc(10, 20, 10, 20).constrainSizeAndAttemptToPreserveAspectRatio(new Size(15, 0))).toEqual(new Size(15, 10));
    const loose = BoxConstraints.loose(new Size(100, 100));
    expect(loose.constrainSizeAndAttemptToPreserveAspectRatio(new Size(200, 100))).toEqual(new Size(100, 50));
    expect(loose.constrainSizeAndAttemptToPreserveAspectRatio(new Size(100, 400))).toEqual(new Size(25, 100));
    expect(bc(50, 100, 0, 100).constrainSizeAndAttemptToPreserveAspectRatio(new Size(10, 10))).toEqual(
      new Size(50, 50),
    );
    expect(bc(0, 100, 60, 100).constrainSizeAndAttemptToPreserveAspectRatio(new Size(20, 10))).toEqual(
      new Size(100, 60),
    );
    expect(BoxConstraints.tight(new Size(7, 9)).constrainSizeAndAttemptToPreserveAspectRatio(new Size(1, 1))).toEqual(
      new Size(7, 9),
    );
  });
});

describe("STU-005 BoxConstraints predicates and equality", () => {
  test("tightness and boundedness", () => {
    const c = bc(10, 10, 0, inf);
    expect(c.hasTightWidth).toBe(true);
    expect(c.hasTightHeight).toBe(false);
    expect(c.isTight).toBe(false);
    expect(c.hasBoundedWidth).toBe(true);
    expect(c.hasBoundedHeight).toBe(false);
    expect(c.hasInfiniteWidth).toBe(false);
    const e = BoxConstraints.expand();
    expect(e.isTight).toBe(true);
    expect(e.hasInfiniteWidth).toBe(true);
    expect(e.hasInfiniteHeight).toBe(true);
  });
  test("isSatisfiedBy", () => {
    const c = bc(10, 20, 10, 20);
    expect(c.isSatisfiedBy(new Size(15, 15))).toBe(true);
    expect(c.isSatisfiedBy(new Size(25, 15))).toBe(false);
    expect(c.isSatisfiedBy(new Size(15, 5))).toBe(false);
  });
  test("isNormalized", () => {
    expect(bc(0, inf, 0, inf).isNormalized).toBe(true);
    expect(bc(-1, 1, 0, 1).isNormalized).toBe(false);
    expect(bc(0, 1, 2, 1).isNormalized).toBe(false);
    expect(bc(Number.NaN, 1, 0, 1).isNormalized).toBe(false);
  });
  test("equality and cache keys", () => {
    const a = bc(1, 2, 3, 4);
    expect(a.equals(a)).toBe(true);
    expect(a.equals(bc(1, 2, 3, 4))).toBe(true);
    expect(a.equals(bc(1, 2, 3, 5))).toBe(false);
    expect(a.cacheKey).toBe(bc(1, 2, 3, 4).cacheKey);
    expect(a.cacheKey).not.toBe(bc(1, 2, 3, inf).cacheKey);
  });
  test("toString (Flutter box_constraints_test)", () => {
    expect(BoxConstraints.expand().toString()).toContain("biggest");
    expect(new BoxConstraints().toString()).toContain("unconstrained");
    expect(BoxConstraints.tightFor({ width: 50 }).toString()).toContain("w=50");
    expect(bc(3, 2, 0, 1).toString()).toContain("NOT NORMALIZED");
  });
});
