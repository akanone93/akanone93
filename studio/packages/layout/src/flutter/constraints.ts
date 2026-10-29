// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/rendering/box.dart (BoxConstraints) and packages/flutter/lib/src/rendering/object.dart (Constraints)
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

import { clampDouble, type EdgeInsetsGeometry, Size } from "./geometry.ts";

const infinity = Number.POSITIVE_INFINITY;

/** Dart's `%` on doubles: the Euclidean modulo, never negative. */
function dartModulo(a: number, b: number): number {
  const r = a % b;
  return r < 0 ? r + Math.abs(b) : r;
}

/**
 * The abstract layout constraints a parent hands its child
 * (object.dart `Constraints`). Box layout uses [BoxConstraints].
 */
export interface Constraints {
  /** Whether exactly one size satisfies these constraints. */
  readonly isTight: boolean;
  /** Whether the constraints are in canonical form. */
  readonly isNormalized: boolean;
  /** Value equality (Dart `==`). */
  equals(other: Constraints): boolean;
}

/**
 * Immutable layout constraints for a box: a child's width must lie in
 * `[minWidth, maxWidth]` and its height in `[minHeight, maxHeight]`.
 */
export class BoxConstraints implements Constraints {
  readonly minWidth: number;
  readonly maxWidth: number;
  readonly minHeight: number;
  readonly maxHeight: number;

  constructor(init: { minWidth?: number; maxWidth?: number; minHeight?: number; maxHeight?: number } = {}) {
    this.minWidth = init.minWidth ?? 0;
    this.maxWidth = init.maxWidth ?? infinity;
    this.minHeight = init.minHeight ?? 0;
    this.maxHeight = init.maxHeight ?? infinity;
  }

  /** Constraints satisfied only by `size`. */
  static tight(size: Size): BoxConstraints {
    return new BoxConstraints({
      minWidth: size.width,
      maxWidth: size.width,
      minHeight: size.height,
      maxHeight: size.height,
    });
  }

  /** Tight in each given dimension, unconstrained in the others. */
  static tightFor(dims: { width?: number | null; height?: number | null } = {}): BoxConstraints {
    const width = dims.width ?? null;
    const height = dims.height ?? null;
    return new BoxConstraints({
      minWidth: width ?? 0,
      maxWidth: width ?? infinity,
      minHeight: height ?? 0,
      maxHeight: height ?? infinity,
    });
  }

  /** Tight in each finite given dimension, unconstrained in infinite ones. */
  static tightForFinite(dims: { width?: number; height?: number } = {}): BoxConstraints {
    const width = dims.width ?? infinity;
    const height = dims.height ?? infinity;
    return new BoxConstraints({
      minWidth: width !== infinity ? width : 0,
      maxWidth: width !== infinity ? width : infinity,
      minHeight: height !== infinity ? height : 0,
      maxHeight: height !== infinity ? height : infinity,
    });
  }

  /** Any size up to `size`. */
  static loose(size: Size): BoxConstraints {
    return new BoxConstraints({ maxWidth: size.width, maxHeight: size.height });
  }

  /** Tight in each given dimension, infinite (expanding) in the others. */
  static expand(dims: { width?: number | null; height?: number | null } = {}): BoxConstraints {
    const width = dims.width ?? null;
    const height = dims.height ?? null;
    return new BoxConstraints({
      minWidth: width ?? infinity,
      maxWidth: width ?? infinity,
      minHeight: height ?? infinity,
      maxHeight: height ?? infinity,
    });
  }

  copyWith(
    init: { minWidth?: number; maxWidth?: number; minHeight?: number; maxHeight?: number } = {},
  ): BoxConstraints {
    return new BoxConstraints({
      minWidth: init.minWidth ?? this.minWidth,
      maxWidth: init.maxWidth ?? this.maxWidth,
      minHeight: init.minHeight ?? this.minHeight,
      maxHeight: init.maxHeight ?? this.maxHeight,
    });
  }

  /** These constraints shrunk by `edges`, never below zero. */
  deflate(edges: EdgeInsetsGeometry): BoxConstraints {
    const horizontal = edges.horizontal;
    const vertical = edges.vertical;
    const deflatedMinWidth = Math.max(0, this.minWidth - horizontal);
    const deflatedMinHeight = Math.max(0, this.minHeight - vertical);
    return new BoxConstraints({
      minWidth: deflatedMinWidth,
      maxWidth: Math.max(deflatedMinWidth, this.maxWidth - horizontal),
      minHeight: deflatedMinHeight,
      maxHeight: Math.max(deflatedMinHeight, this.maxHeight - vertical),
    });
  }

  /** The same maxima with the minima removed. */
  loosen(): BoxConstraints {
    return new BoxConstraints({ maxWidth: this.maxWidth, maxHeight: this.maxHeight });
  }

  /** These constraints clamped to lie within `constraints`. */
  enforce(constraints: BoxConstraints): BoxConstraints {
    return new BoxConstraints({
      minWidth: clampDouble(this.minWidth, constraints.minWidth, constraints.maxWidth),
      maxWidth: clampDouble(this.maxWidth, constraints.minWidth, constraints.maxWidth),
      minHeight: clampDouble(this.minHeight, constraints.minHeight, constraints.maxHeight),
      maxHeight: clampDouble(this.maxHeight, constraints.minHeight, constraints.maxHeight),
    });
  }

  /** Tight in each given dimension, as close to it as these constraints allow. */
  tighten(dims: { width?: number | null; height?: number | null } = {}): BoxConstraints {
    const width = dims.width ?? null;
    const height = dims.height ?? null;
    return new BoxConstraints({
      minWidth: width === null ? this.minWidth : clampDouble(width, this.minWidth, this.maxWidth),
      maxWidth: width === null ? this.maxWidth : clampDouble(width, this.minWidth, this.maxWidth),
      minHeight: height === null ? this.minHeight : clampDouble(height, this.minHeight, this.maxHeight),
      maxHeight: height === null ? this.maxHeight : clampDouble(height, this.minHeight, this.maxHeight),
    });
  }

  /** Width and height swapped. */
  get flipped(): BoxConstraints {
    return new BoxConstraints({
      minWidth: this.minHeight,
      maxWidth: this.maxHeight,
      minHeight: this.minWidth,
      maxHeight: this.maxWidth,
    });
  }

  /** Only the width constraints. */
  widthConstraints(): BoxConstraints {
    return new BoxConstraints({ minWidth: this.minWidth, maxWidth: this.maxWidth });
  }

  /** Only the height constraints. */
  heightConstraints(): BoxConstraints {
    return new BoxConstraints({ minHeight: this.minHeight, maxHeight: this.maxHeight });
  }

  /** The width closest to `width` that satisfies these constraints. */
  constrainWidth(width: number = infinity): number {
    return clampDouble(width, this.minWidth, this.maxWidth);
  }

  /** The height closest to `height` that satisfies these constraints. */
  constrainHeight(height: number = infinity): number {
    return clampDouble(height, this.minHeight, this.maxHeight);
  }

  /** The size closest to `size` that satisfies these constraints. */
  constrain(size: Size): Size {
    return new Size(this.constrainWidth(size.width), this.constrainHeight(size.height));
  }

  /** The size closest to `width` x `height` that satisfies these constraints. */
  constrainDimensions(width: number, height: number): Size {
    return new Size(this.constrainWidth(width), this.constrainHeight(height));
  }

  /**
   * A size that satisfies these constraints and, where possible, keeps the
   * aspect ratio of `size` (used for images).
   */
  constrainSizeAndAttemptToPreserveAspectRatio(size: Size): Size {
    if (this.isTight) {
      return this.smallest;
    }
    if (size.isEmpty) {
      return this.constrain(size);
    }
    let width = size.width;
    let height = size.height;
    const aspectRatio = width / height;
    if (width > this.maxWidth) {
      width = this.maxWidth;
      height = width / aspectRatio;
    }
    if (height > this.maxHeight) {
      height = this.maxHeight;
      width = height * aspectRatio;
    }
    if (width < this.minWidth) {
      width = this.minWidth;
      height = width / aspectRatio;
    }
    if (height < this.minHeight) {
      height = this.minHeight;
      width = height * aspectRatio;
    }
    return new Size(this.constrainWidth(width), this.constrainHeight(height));
  }

  /** The biggest size that satisfies the constraints. */
  get biggest(): Size {
    return new Size(this.constrainWidth(), this.constrainHeight());
  }

  /** The smallest size that satisfies the constraints. */
  get smallest(): Size {
    return new Size(this.constrainWidth(0), this.constrainHeight(0));
  }

  get hasTightWidth(): boolean {
    return this.minWidth >= this.maxWidth;
  }

  get hasTightHeight(): boolean {
    return this.minHeight >= this.maxHeight;
  }

  get isTight(): boolean {
    return this.hasTightWidth && this.hasTightHeight;
  }

  get hasBoundedWidth(): boolean {
    return this.maxWidth < infinity;
  }

  get hasBoundedHeight(): boolean {
    return this.maxHeight < infinity;
  }

  get hasInfiniteWidth(): boolean {
    return this.minWidth >= infinity;
  }

  get hasInfiniteHeight(): boolean {
    return this.minHeight >= infinity;
  }

  /** Whether `size` satisfies these constraints. */
  isSatisfiedBy(size: Size): boolean {
    return (
      this.minWidth <= size.width &&
      size.width <= this.maxWidth &&
      this.minHeight <= size.height &&
      size.height <= this.maxHeight
    );
  }

  /** `constraints * factor`. */
  multiply(factor: number): BoxConstraints {
    return new BoxConstraints({
      minWidth: this.minWidth * factor,
      maxWidth: this.maxWidth * factor,
      minHeight: this.minHeight * factor,
      maxHeight: this.maxHeight * factor,
    });
  }

  /** `constraints / factor`. */
  divide(factor: number): BoxConstraints {
    return new BoxConstraints({
      minWidth: this.minWidth / factor,
      maxWidth: this.maxWidth / factor,
      minHeight: this.minHeight / factor,
      maxHeight: this.maxHeight / factor,
    });
  }

  /** `constraints ~/ factor` (truncating division). */
  truncatingDivide(factor: number): BoxConstraints {
    return new BoxConstraints({
      minWidth: Math.trunc(this.minWidth / factor),
      maxWidth: Math.trunc(this.maxWidth / factor),
      minHeight: Math.trunc(this.minHeight / factor),
      maxHeight: Math.trunc(this.maxHeight / factor),
    });
  }

  /** `constraints % value` (Dart's Euclidean modulo). */
  modulo(value: number): BoxConstraints {
    return new BoxConstraints({
      minWidth: dartModulo(this.minWidth, value),
      maxWidth: dartModulo(this.maxWidth, value),
      minHeight: dartModulo(this.minHeight, value),
      maxHeight: dartModulo(this.maxHeight, value),
    });
  }

  get isNormalized(): boolean {
    return (
      this.minWidth >= 0 && this.minWidth <= this.maxWidth && this.minHeight >= 0 && this.minHeight <= this.maxHeight
    );
  }

  /** Canonical constraints: non-negative minima, maxima no smaller than minima. */
  normalize(): BoxConstraints {
    if (this.isNormalized) {
      return this;
    }
    const minWidth = this.minWidth >= 0 ? this.minWidth : 0;
    const minHeight = this.minHeight >= 0 ? this.minHeight : 0;
    return new BoxConstraints({
      minWidth,
      maxWidth: minWidth > this.maxWidth ? minWidth : this.maxWidth,
      minHeight,
      maxHeight: minHeight > this.maxHeight ? minHeight : this.maxHeight,
    });
  }

  equals(other: Constraints): boolean {
    if (this === other) {
      return true;
    }
    return (
      other instanceof BoxConstraints &&
      other.minWidth === this.minWidth &&
      other.maxWidth === this.maxWidth &&
      other.minHeight === this.minHeight &&
      other.maxHeight === this.maxHeight
    );
  }

  /** A string usable as a map key; equal constraints give equal keys. */
  get cacheKey(): string {
    return `${this.minWidth},${this.maxWidth},${this.minHeight},${this.maxHeight}`;
  }

  toString(): string {
    const annotation = this.isNormalized ? "" : "; NOT NORMALIZED";
    if (this.minWidth === infinity && this.minHeight === infinity) {
      return `BoxConstraints(biggest${annotation})`;
    }
    if (this.minWidth === 0 && this.maxWidth === infinity && this.minHeight === 0 && this.maxHeight === infinity) {
      return `BoxConstraints(unconstrained${annotation})`;
    }
    const describe = (min: number, max: number, dim: string): string =>
      min === max ? `${dim}=${min.toFixed(1)}` : `${min.toFixed(1)}<=${dim}<=${max.toFixed(1)}`;
    const width = describe(this.minWidth, this.maxWidth, "w");
    const height = describe(this.minHeight, this.maxHeight, "h");
    return `BoxConstraints(${width}, ${height}${annotation})`;
  }
}
