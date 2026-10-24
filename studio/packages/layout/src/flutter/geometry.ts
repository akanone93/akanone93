// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/painting/alignment.dart, packages/flutter/lib/src/painting/edge_insets.dart, packages/flutter/lib/src/foundation/constants.dart, packages/flutter/lib/src/foundation/math.dart, packages/flutter/lib/src/physics/utils.dart and dart:ui geometry.dart (Offset, Size, Rect)
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/**
 * The value geometry of Flutter's layout: offsets, sizes, rectangles, edge
 * insets and alignments. Every type is immutable; operators are methods
 * (`a.plus(b)` for `a + b`) because TypeScript has no operator overloading.
 */

/** The tolerance Flutter uses when comparing doubles for layout (foundation/constants.dart). */
export const precisionErrorTolerance = 1e-10;

/**
 * Whether `a` and `b` are equal within `epsilon` (physics/utils.dart
 * `nearEqual`). Two `null`s are equal; `null` and a number are not.
 */
export function nearEqual(a: number | null, b: number | null, epsilon: number): boolean {
  if (a === null || b === null) {
    return a === b;
  }
  return (a > b - epsilon && a < b + epsilon) || a === b;
}

/** Whether `a` is within `epsilon` of zero (physics/utils.dart `nearZero`). */
export function nearZero(a: number, epsilon: number): boolean {
  return nearEqual(a, 0, epsilon);
}

/**
 * Clamps `x` to `[min, max]` the way Flutter's `clampDouble` does: a NaN
 * input yields `max` rather than propagating (foundation/math.dart).
 */
export function clampDouble(x: number, min: number, max: number): number {
  if (x < min) {
    return min;
  }
  if (x > max) {
    return max;
  }
  if (Number.isNaN(x)) {
    return max;
  }
  return x;
}

/** The direction text flows in; resolves directional insets and alignments. */
export enum TextDirection {
  rtl = "rtl",
  ltr = "ltr",
}

/** An immutable 2D offset (dart:ui `Offset`). */
export class Offset {
  static readonly zero = new Offset(0, 0);
  static readonly infinite = new Offset(Number.POSITIVE_INFINITY, Number.POSITIVE_INFINITY);

  constructor(
    readonly dx: number,
    readonly dy: number,
  ) {}

  get distance(): number {
    return Math.sqrt(this.dx * this.dx + this.dy * this.dy);
  }

  get isFinite(): boolean {
    return Number.isFinite(this.dx) && Number.isFinite(this.dy);
  }

  plus(other: Offset): Offset {
    return new Offset(this.dx + other.dx, this.dy + other.dy);
  }

  minus(other: Offset): Offset {
    return new Offset(this.dx - other.dx, this.dy - other.dy);
  }

  scale(scaleX: number, scaleY: number): Offset {
    return new Offset(this.dx * scaleX, this.dy * scaleY);
  }

  translate(translateX: number, translateY: number): Offset {
    return new Offset(this.dx + translateX, this.dy + translateY);
  }

  equals(other: Offset): boolean {
    return this.dx === other.dx && this.dy === other.dy;
  }

  toString(): string {
    return `Offset(${this.dx.toFixed(1)}, ${this.dy.toFixed(1)})`;
  }
}

/** An immutable 2D size (dart:ui `Size`). */
export class Size {
  static readonly zero = new Size(0, 0);
  static readonly infinite = new Size(Number.POSITIVE_INFINITY, Number.POSITIVE_INFINITY);

  constructor(
    readonly width: number,
    readonly height: number,
  ) {}

  static square(dimension: number): Size {
    return new Size(dimension, dimension);
  }

  static fromWidth(width: number): Size {
    return new Size(width, Number.POSITIVE_INFINITY);
  }

  static fromHeight(height: number): Size {
    return new Size(Number.POSITIVE_INFINITY, height);
  }

  /** Whether either dimension is zero or negative. */
  get isEmpty(): boolean {
    return this.width <= 0 || this.height <= 0;
  }

  get isFinite(): boolean {
    return Number.isFinite(this.width) && Number.isFinite(this.height);
  }

  get aspectRatio(): number {
    if (this.height !== 0) {
      return this.width / this.height;
    }
    if (this.width > 0) {
      return Number.POSITIVE_INFINITY;
    }
    if (this.width < 0) {
      return Number.NEGATIVE_INFINITY;
    }
    return 0;
  }

  get shortestSide(): number {
    return Math.min(Math.abs(this.width), Math.abs(this.height));
  }

  get longestSide(): number {
    return Math.max(Math.abs(this.width), Math.abs(this.height));
  }

  /** `Size - Size`, which Dart types as an [Offset]. */
  minus(other: Size): Offset {
    return new Offset(this.width - other.width, this.height - other.height);
  }

  /** `Size + Offset`. */
  plus(other: Offset): Size {
    return new Size(this.width + other.dx, this.height + other.dy);
  }

  times(operand: number): Size {
    return new Size(this.width * operand, this.height * operand);
  }

  equals(other: Size): boolean {
    return this.width === other.width && this.height === other.height;
  }

  toString(): string {
    return `Size(${this.width.toFixed(1)}, ${this.height.toFixed(1)})`;
  }
}

/** An immutable axis-aligned rectangle (dart:ui `Rect`). */
export class Rect {
  static readonly zero = new Rect(0, 0, 0, 0);

  private constructor(
    readonly left: number,
    readonly top: number,
    readonly right: number,
    readonly bottom: number,
  ) {}

  static fromLTRB(left: number, top: number, right: number, bottom: number): Rect {
    return new Rect(left, top, right, bottom);
  }

  static fromLTWH(left: number, top: number, width: number, height: number): Rect {
    return new Rect(left, top, left + width, top + height);
  }

  get width(): number {
    return this.right - this.left;
  }

  get height(): number {
    return this.bottom - this.top;
  }

  get size(): Size {
    return new Size(this.width, this.height);
  }

  get topLeft(): Offset {
    return new Offset(this.left, this.top);
  }

  shift(offset: Offset): Rect {
    return new Rect(this.left + offset.dx, this.top + offset.dy, this.right + offset.dx, this.bottom + offset.dy);
  }

  equals(other: Rect): boolean {
    return (
      this.left === other.left && this.top === other.top && this.right === other.right && this.bottom === other.bottom
    );
  }
}

/** Insets that are either absolute ([EdgeInsets]) or directional ([EdgeInsetsDirectional]). */
export interface EdgeInsetsGeometry {
  readonly horizontal: number;
  readonly vertical: number;
  /** Converts to absolute insets; `direction` is required only by directional insets. */
  resolve(direction: TextDirection | null): EdgeInsets;
}

/** Absolute insets on the four sides of a box (painting/edge_insets.dart). */
export class EdgeInsets implements EdgeInsetsGeometry {
  static readonly zero = new EdgeInsets(0, 0, 0, 0);

  private constructor(
    readonly left: number,
    readonly top: number,
    readonly right: number,
    readonly bottom: number,
  ) {}

  static fromLTRB(left: number, top: number, right: number, bottom: number): EdgeInsets {
    return new EdgeInsets(left, top, right, bottom);
  }

  static all(value: number): EdgeInsets {
    return new EdgeInsets(value, value, value, value);
  }

  static only(sides: { left?: number; top?: number; right?: number; bottom?: number }): EdgeInsets {
    return new EdgeInsets(sides.left ?? 0, sides.top ?? 0, sides.right ?? 0, sides.bottom ?? 0);
  }

  static symmetric(axes: { vertical?: number; horizontal?: number }): EdgeInsets {
    const v = axes.vertical ?? 0;
    const h = axes.horizontal ?? 0;
    return new EdgeInsets(h, v, h, v);
  }

  get horizontal(): number {
    return this.left + this.right;
  }

  get vertical(): number {
    return this.top + this.bottom;
  }

  get topLeft(): Offset {
    return new Offset(this.left, this.top);
  }

  /** Grows `rect` by these insets. */
  inflateRect(rect: Rect): Rect {
    return Rect.fromLTRB(
      rect.left - this.left,
      rect.top - this.top,
      rect.right + this.right,
      rect.bottom + this.bottom,
    );
  }

  /** Shrinks `rect` by these insets. */
  deflateRect(rect: Rect): Rect {
    return Rect.fromLTRB(
      rect.left + this.left,
      rect.top + this.top,
      rect.right - this.right,
      rect.bottom - this.bottom,
    );
  }

  /** Shrinks `size` by these insets (may go negative, as in Flutter). */
  deflateSize(size: Size): Size {
    return new Size(size.width - this.horizontal, size.height - this.vertical);
  }

  resolve(_direction: TextDirection | null): EdgeInsets {
    return this;
  }

  equals(other: EdgeInsets): boolean {
    return (
      this.left === other.left && this.top === other.top && this.right === other.right && this.bottom === other.bottom
    );
  }
}

/** Insets whose horizontal sides follow the reading direction. */
export class EdgeInsetsDirectional implements EdgeInsetsGeometry {
  static readonly zero = new EdgeInsetsDirectional(0, 0, 0, 0);

  private constructor(
    readonly start: number,
    readonly top: number,
    readonly end: number,
    readonly bottom: number,
  ) {}

  static fromSTEB(start: number, top: number, end: number, bottom: number): EdgeInsetsDirectional {
    return new EdgeInsetsDirectional(start, top, end, bottom);
  }

  static only(sides: { start?: number; top?: number; end?: number; bottom?: number }): EdgeInsetsDirectional {
    return new EdgeInsetsDirectional(sides.start ?? 0, sides.top ?? 0, sides.end ?? 0, sides.bottom ?? 0);
  }

  static all(value: number): EdgeInsetsDirectional {
    return new EdgeInsetsDirectional(value, value, value, value);
  }

  get horizontal(): number {
    return this.start + this.end;
  }

  get vertical(): number {
    return this.top + this.bottom;
  }

  /**
   * Converts to absolute insets. Flutter asserts a non-null direction; a
   * null direction here throws, since the result would be undefined.
   */
  resolve(direction: TextDirection | null): EdgeInsets {
    switch (direction) {
      case TextDirection.rtl:
        return EdgeInsets.fromLTRB(this.end, this.top, this.start, this.bottom);
      case TextDirection.ltr:
        return EdgeInsets.fromLTRB(this.start, this.top, this.end, this.bottom);
      case null:
        throw new Error("EdgeInsetsDirectional.resolve needs a TextDirection");
    }
  }
}

/** An alignment that is either absolute ([Alignment]) or directional ([AlignmentDirectional]). */
export interface AlignmentGeometry {
  /** Converts to an absolute alignment; `direction` is required only by directional alignments. */
  resolve(direction: TextDirection | null): Alignment;
}

/**
 * A point within a rectangle: (-1, -1) is the top left, (1, 1) the bottom
 * right (painting/alignment.dart).
 */
export class Alignment implements AlignmentGeometry {
  static readonly topLeft = new Alignment(-1, -1);
  static readonly topCenter = new Alignment(0, -1);
  static readonly topRight = new Alignment(1, -1);
  static readonly centerLeft = new Alignment(-1, 0);
  static readonly center = new Alignment(0, 0);
  static readonly centerRight = new Alignment(1, 0);
  static readonly bottomLeft = new Alignment(-1, 1);
  static readonly bottomCenter = new Alignment(0, 1);
  static readonly bottomRight = new Alignment(1, 1);

  constructor(
    readonly x: number,
    readonly y: number,
  ) {}

  /** The offset of this alignment within a rectangle whose size is `other`. */
  alongOffset(other: Offset): Offset {
    const centerX = other.dx / 2;
    const centerY = other.dy / 2;
    return new Offset(centerX + this.x * centerX, centerY + this.y * centerY);
  }

  /** The offset of this alignment within a rectangle of `other`'s size. */
  alongSize(other: Size): Offset {
    const centerX = other.width / 2;
    const centerY = other.height / 2;
    return new Offset(centerX + this.x * centerX, centerY + this.y * centerY);
  }

  /** The point at this alignment within `rect`. */
  withinRect(rect: Rect): Offset {
    const halfWidth = rect.width / 2;
    const halfHeight = rect.height / 2;
    return new Offset(rect.left + halfWidth + this.x * halfWidth, rect.top + halfHeight + this.y * halfHeight);
  }

  /** A rectangle of `size` placed within `rect` at this alignment. */
  inscribe(size: Size, rect: Rect): Rect {
    const halfWidthDelta = (rect.width - size.width) / 2;
    const halfHeightDelta = (rect.height - size.height) / 2;
    return Rect.fromLTWH(
      rect.left + halfWidthDelta + this.x * halfWidthDelta,
      rect.top + halfHeightDelta + this.y * halfHeightDelta,
      size.width,
      size.height,
    );
  }

  resolve(_direction: TextDirection | null): Alignment {
    return this;
  }

  equals(other: Alignment): boolean {
    return this.x === other.x && this.y === other.y;
  }
}

/** An alignment whose horizontal component follows the reading direction. */
export class AlignmentDirectional implements AlignmentGeometry {
  static readonly topStart = new AlignmentDirectional(-1, -1);
  static readonly topCenter = new AlignmentDirectional(0, -1);
  static readonly topEnd = new AlignmentDirectional(1, -1);
  static readonly centerStart = new AlignmentDirectional(-1, 0);
  static readonly center = new AlignmentDirectional(0, 0);
  static readonly centerEnd = new AlignmentDirectional(1, 0);
  static readonly bottomStart = new AlignmentDirectional(-1, 1);
  static readonly bottomCenter = new AlignmentDirectional(0, 1);
  static readonly bottomEnd = new AlignmentDirectional(1, 1);

  constructor(
    readonly start: number,
    readonly y: number,
  ) {}

  /**
   * Converts to an absolute alignment. Flutter asserts a non-null direction;
   * a null direction here throws, since the result would be undefined.
   */
  resolve(direction: TextDirection | null): Alignment {
    switch (direction) {
      case TextDirection.rtl:
        return new Alignment(-this.start, this.y);
      case TextDirection.ltr:
        return new Alignment(this.start, this.y);
      case null:
        throw new Error("AlignmentDirectional.resolve needs a TextDirection");
    }
  }
}
