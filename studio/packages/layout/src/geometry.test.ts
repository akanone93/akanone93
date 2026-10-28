// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import {
  Alignment,
  AlignmentDirectional,
  clampDouble,
  EdgeInsets,
  EdgeInsetsDirectional,
  nearEqual,
  nearZero,
  Offset,
  precisionErrorTolerance,
  Rect,
  Size,
  TextDirection,
} from "./flutter/geometry.ts";

const inf = Number.POSITIVE_INFINITY;

describe("[STU-005] geometry", () => {
  test("double comparisons", () => {
    expect(precisionErrorTolerance).toBe(1e-10);
    expect(nearEqual(1, 1 + 1e-11, precisionErrorTolerance)).toBe(true);
    expect(nearEqual(1, 1.1, precisionErrorTolerance)).toBe(false);
    expect(nearEqual(inf, inf, precisionErrorTolerance)).toBe(true);
    expect(nearEqual(null, null, 1)).toBe(true);
    expect(nearEqual(null, 0, 1)).toBe(false);
    expect(nearZero(1e-12, precisionErrorTolerance)).toBe(true);
    expect(clampDouble(5, 0, 3)).toBe(3);
    expect(clampDouble(-5, 0, 3)).toBe(0);
    expect(clampDouble(Number.NaN, 0, 3)).toBe(3);
    expect(clampDouble(2, 0, 3)).toBe(2);
  });

  test("Offset", () => {
    const o = new Offset(3, 4);
    expect(o.distance).toBe(5);
    expect(o.isFinite).toBe(true);
    expect(Offset.infinite.isFinite).toBe(false);
    expect(o.plus(new Offset(1, 1)).equals(new Offset(4, 5))).toBe(true);
    expect(o.minus(new Offset(1, 1)).equals(new Offset(2, 3))).toBe(true);
    expect(o.scale(2, 3).equals(new Offset(6, 12))).toBe(true);
    expect(o.translate(1, -1).equals(new Offset(4, 3))).toBe(true);
    expect(o.toString()).toBe("Offset(3.0, 4.0)");
  });

  test("Size", () => {
    const s = new Size(20, 10);
    expect(s.isEmpty).toBe(false);
    expect(new Size(0, 10).isEmpty).toBe(true);
    expect(s.isFinite).toBe(true);
    expect(Size.infinite.isFinite).toBe(false);
    expect(s.aspectRatio).toBe(2);
    expect(new Size(1, 0).aspectRatio).toBe(inf);
    expect(new Size(-1, 0).aspectRatio).toBe(Number.NEGATIVE_INFINITY);
    expect(Size.zero.aspectRatio).toBe(0);
    expect(s.shortestSide).toBe(10);
    expect(s.longestSide).toBe(20);
    expect(s.minus(new Size(5, 5)).equals(new Offset(15, 5))).toBe(true);
    expect(s.plus(new Offset(1, 2)).equals(new Size(21, 12))).toBe(true);
    expect(s.times(2).equals(new Size(40, 20))).toBe(true);
    expect(Size.square(3).equals(new Size(3, 3))).toBe(true);
    expect(Size.fromWidth(3).height).toBe(inf);
    expect(Size.fromHeight(3).width).toBe(inf);
    expect(s.toString()).toBe("Size(20.0, 10.0)");
  });

  test("Rect", () => {
    const r = Rect.fromLTWH(10, 20, 30, 40);
    expect(r.right).toBe(40);
    expect(r.size.equals(new Size(30, 40))).toBe(true);
    expect(r.topLeft.equals(new Offset(10, 20))).toBe(true);
    expect(r.shift(new Offset(1, 1)).equals(Rect.fromLTRB(11, 21, 41, 61))).toBe(true);
    expect(Rect.zero.width).toBe(0);
  });

  test("EdgeInsets and EdgeInsetsDirectional", () => {
    const e = EdgeInsets.fromLTRB(1, 2, 3, 4);
    expect(e.horizontal).toBe(4);
    expect(e.vertical).toBe(6);
    expect(e.topLeft.equals(new Offset(1, 2))).toBe(true);
    expect(e.resolve(null)).toBe(e);
    const r = Rect.fromLTRB(0, 0, 100, 100);
    expect(e.deflateRect(r).equals(Rect.fromLTRB(1, 2, 97, 96))).toBe(true);
    expect(e.inflateRect(r).equals(Rect.fromLTRB(-1, -2, 103, 104))).toBe(true);
    expect(e.deflateSize(new Size(10, 10)).equals(new Size(6, 4))).toBe(true);
    expect(EdgeInsets.symmetric({ horizontal: 2 }).equals(EdgeInsets.fromLTRB(2, 0, 2, 0))).toBe(true);
    expect(EdgeInsets.only({ top: 3 }).vertical).toBe(3);
    expect(EdgeInsets.zero.horizontal).toBe(0);
    const d = EdgeInsetsDirectional.fromSTEB(1, 2, 3, 4);
    expect(d.horizontal).toBe(4);
    expect(d.vertical).toBe(6);
    expect(d.resolve(TextDirection.ltr).equals(EdgeInsets.fromLTRB(1, 2, 3, 4))).toBe(true);
    expect(d.resolve(TextDirection.rtl).equals(EdgeInsets.fromLTRB(3, 2, 1, 4))).toBe(true);
    expect(() => d.resolve(null)).toThrow();
    expect(EdgeInsetsDirectional.all(2).horizontal).toBe(4);
    expect(EdgeInsetsDirectional.zero.vertical).toBe(0);
  });

  test("Alignment and AlignmentDirectional", () => {
    expect(Alignment.bottomRight.alongOffset(new Offset(100, 50)).equals(new Offset(100, 50))).toBe(true);
    expect(Alignment.center.alongOffset(new Offset(100, 50)).equals(new Offset(50, 25))).toBe(true);
    expect(Alignment.topLeft.alongSize(new Size(100, 50)).equals(Offset.zero)).toBe(true);
    expect(Alignment.center.withinRect(Rect.fromLTWH(10, 10, 20, 20)).equals(new Offset(20, 20))).toBe(true);
    expect(
      Alignment.bottomRight
        .inscribe(new Size(10, 10), Rect.fromLTWH(0, 0, 100, 50))
        .equals(Rect.fromLTWH(90, 40, 10, 10)),
    ).toBe(true);
    expect(Alignment.topCenter.resolve(null)).toBe(Alignment.topCenter);
    expect(AlignmentDirectional.centerStart.resolve(TextDirection.ltr).equals(Alignment.centerLeft)).toBe(true);
    expect(AlignmentDirectional.topStart.resolve(TextDirection.rtl).equals(Alignment.topRight)).toBe(true);
    expect(() => AlignmentDirectional.bottomEnd.resolve(null)).toThrow();
  });
});
