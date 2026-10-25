// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/rendering/proxy_box.dart
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/**
 * More boxes that take their child's size or a function of it: limited,
 * aspect-ratio, intrinsic, fitted, transformed, clipped and translated boxes
 * (layout and paint transforms only; painting itself is not ported).
 *
 * Release behaviour is followed where Flutter checks with `assert`: values
 * that debug Flutter rejects (a non-positive aspect ratio, a non-positive
 * intrinsic step) are not checked here.
 */

import { applyBoxFit, BoxFit } from "./box-fit.ts";
import { BoxConstraints } from "./constraints.ts";
import { Alignment, type AlignmentGeometry, Offset, Rect, Size, type TextDirection } from "./geometry.ts";
import { Affine, type PaintTransformer } from "./matrix.ts";
import type { RenderObject } from "./object.ts";
import { RenderProxyBox } from "./proxy-box.ts";
import type { RenderBox, TextBaseline } from "./render-box.ts";

/** Limits an unbounded child's size to `maxWidth` x `maxHeight` (`LimitedBox`). */
export class RenderLimitedBox extends RenderProxyBox {
  private _maxWidth: number;
  private _maxHeight: number;

  constructor(options: { maxWidth?: number; maxHeight?: number; child?: RenderBox | null } = {}) {
    super(options.child ?? null);
    this._maxWidth = options.maxWidth ?? Number.POSITIVE_INFINITY;
    this._maxHeight = options.maxHeight ?? Number.POSITIVE_INFINITY;
  }

  get maxWidth(): number {
    return this._maxWidth;
  }

  set maxWidth(value: number) {
    if (this._maxWidth !== value) {
      this._maxWidth = value;
      this.markNeedsLayout();
    }
  }

  get maxHeight(): number {
    return this._maxHeight;
  }

  set maxHeight(value: number) {
    if (this._maxHeight !== value) {
      this._maxHeight = value;
      this.markNeedsLayout();
    }
  }

  private _limitConstraints(constraints: BoxConstraints): BoxConstraints {
    return new BoxConstraints({
      minWidth: constraints.minWidth,
      maxWidth: constraints.hasBoundedWidth ? constraints.maxWidth : constraints.constrainWidth(this._maxWidth),
      minHeight: constraints.minHeight,
      maxHeight: constraints.hasBoundedHeight ? constraints.maxHeight : constraints.constrainHeight(this._maxHeight),
    });
  }

  protected override computeDryLayout(constraints: BoxConstraints): Size {
    const child = this.child;
    if (child !== null) {
      return constraints.constrain(child.getDryLayout(this._limitConstraints(constraints)));
    }
    return this._limitConstraints(constraints).constrain(Size.zero);
  }

  protected override performLayout(): void {
    const child = this.child;
    if (child !== null) {
      child.layout(this._limitConstraints(this.constraints), { parentUsesSize: true });
      this.size = this.constraints.constrain(child.size);
    } else {
      this.size = this._limitConstraints(this.constraints).constrain(Size.zero);
    }
  }
}

/** Sizes itself to a width-to-height ratio (`AspectRatio`). */
export class RenderAspectRatio extends RenderProxyBox {
  private _aspectRatio: number;

  constructor(options: { aspectRatio: number; child?: RenderBox | null }) {
    super(options.child ?? null);
    this._aspectRatio = options.aspectRatio;
  }

  get aspectRatio(): number {
    return this._aspectRatio;
  }

  set aspectRatio(value: number) {
    if (this._aspectRatio !== value) {
      this._aspectRatio = value;
      this.markNeedsLayout();
    }
  }

  protected override computeMinIntrinsicWidth(height: number): number {
    return Number.isFinite(height) ? height * this._aspectRatio : super.computeMinIntrinsicWidth(height);
  }

  protected override computeMaxIntrinsicWidth(height: number): number {
    return Number.isFinite(height) ? height * this._aspectRatio : super.computeMaxIntrinsicWidth(height);
  }

  protected override computeMinIntrinsicHeight(width: number): number {
    return Number.isFinite(width) ? width / this._aspectRatio : super.computeMinIntrinsicHeight(width);
  }

  protected override computeMaxIntrinsicHeight(width: number): number {
    return Number.isFinite(width) ? width / this._aspectRatio : super.computeMaxIntrinsicHeight(width);
  }

  private _applyAspectRatio(constraints: BoxConstraints): Size {
    if (constraints.isTight) {
      return constraints.smallest;
    }
    let width = constraints.maxWidth;
    let height: number;
    // Pick the height from the width unless the width is infinite.
    if (Number.isFinite(width)) {
      height = width / this._aspectRatio;
    } else {
      height = constraints.maxHeight;
      width = height * this._aspectRatio;
    }
    if (width > constraints.maxWidth) {
      width = constraints.maxWidth;
      height = width / this._aspectRatio;
    }
    if (height > constraints.maxHeight) {
      height = constraints.maxHeight;
      width = height * this._aspectRatio;
    }
    if (width < constraints.minWidth) {
      width = constraints.minWidth;
      height = width / this._aspectRatio;
    }
    if (height < constraints.minHeight) {
      height = constraints.minHeight;
      width = height * this._aspectRatio;
    }
    return constraints.constrain(new Size(width, height));
  }

  protected override computeDryLayout(constraints: BoxConstraints): Size {
    return this._applyAspectRatio(constraints);
  }

  protected override computeDryBaseline(constraints: BoxConstraints, baseline: TextBaseline): number | null {
    return super.computeDryBaseline(BoxConstraints.tight(this.getDryLayout(constraints)), baseline);
  }

  protected override performLayout(): void {
    this.size = this.getDryLayout(this.constraints);
    this.child?.layout(BoxConstraints.tight(this.size));
  }
}

/** Rounds `input` up to a multiple of `step` (`RenderIntrinsicWidth._applyStep`). */
export function applyStep(input: number, step: number | null): number {
  return step === null ? input : Math.ceil(input / step) * step;
}

/** Sizes its child to the child's maximum intrinsic width (`IntrinsicWidth`), optionally in steps. */
export class RenderIntrinsicWidth extends RenderProxyBox {
  private _stepWidth: number | null;
  private _stepHeight: number | null;

  constructor(options: { stepWidth?: number | null; stepHeight?: number | null; child?: RenderBox | null } = {}) {
    super(options.child ?? null);
    this._stepWidth = options.stepWidth ?? null;
    this._stepHeight = options.stepHeight ?? null;
  }

  get stepWidth(): number | null {
    return this._stepWidth;
  }

  set stepWidth(value: number | null) {
    if (this._stepWidth !== value) {
      this._stepWidth = value;
      this.markNeedsLayout();
    }
  }

  get stepHeight(): number | null {
    return this._stepHeight;
  }

  set stepHeight(value: number | null) {
    if (this._stepHeight !== value) {
      this._stepHeight = value;
      this.markNeedsLayout();
    }
  }

  protected override computeMinIntrinsicWidth(height: number): number {
    return this.getMaxIntrinsicWidth(height);
  }

  protected override computeMaxIntrinsicWidth(height: number): number {
    const child = this.child;
    return child === null ? 0 : applyStep(child.getMaxIntrinsicWidth(height), this._stepWidth);
  }

  protected override computeMinIntrinsicHeight(width: number): number {
    const child = this.child;
    if (child === null) {
      return 0;
    }
    const finiteWidth = Number.isFinite(width) ? width : this.getMaxIntrinsicWidth(Number.POSITIVE_INFINITY);
    return applyStep(child.getMinIntrinsicHeight(finiteWidth), this._stepHeight);
  }

  protected override computeMaxIntrinsicHeight(width: number): number {
    const child = this.child;
    if (child === null) {
      return 0;
    }
    const finiteWidth = Number.isFinite(width) ? width : this.getMaxIntrinsicWidth(Number.POSITIVE_INFINITY);
    return applyStep(child.getMaxIntrinsicHeight(finiteWidth), this._stepHeight);
  }

  private _childConstraints(child: RenderBox, constraints: BoxConstraints): BoxConstraints {
    return constraints.tighten({
      width: constraints.hasTightWidth
        ? null
        : applyStep(child.getMaxIntrinsicWidth(constraints.maxHeight), this._stepWidth),
      height:
        this._stepHeight === null
          ? null
          : applyStep(child.getMaxIntrinsicHeight(constraints.maxWidth), this._stepHeight),
    });
  }

  protected override computeDryLayout(constraints: BoxConstraints): Size {
    const child = this.child;
    return child === null ? constraints.smallest : child.getDryLayout(this._childConstraints(child, constraints));
  }

  protected override computeDryBaseline(constraints: BoxConstraints, baseline: TextBaseline): number | null {
    const child = this.child;
    return child === null ? null : child.getDryBaseline(this._childConstraints(child, constraints), baseline);
  }

  protected override performLayout(): void {
    const child = this.child;
    if (child === null) {
      this.size = this.constraints.smallest;
      return;
    }
    child.layout(this._childConstraints(child, this.constraints), { parentUsesSize: true });
    this.size = child.size;
  }
}

/** Sizes its child to the child's maximum intrinsic height (`IntrinsicHeight`). */
export class RenderIntrinsicHeight extends RenderProxyBox {
  constructor(child: RenderBox | null = null) {
    super(child);
  }

  protected override computeMinIntrinsicWidth(height: number): number {
    const child = this.child;
    if (child === null) {
      return 0;
    }
    const finiteHeight = Number.isFinite(height) ? height : child.getMaxIntrinsicHeight(Number.POSITIVE_INFINITY);
    return child.getMinIntrinsicWidth(finiteHeight);
  }

  protected override computeMaxIntrinsicWidth(height: number): number {
    const child = this.child;
    if (child === null) {
      return 0;
    }
    const finiteHeight = Number.isFinite(height) ? height : child.getMaxIntrinsicHeight(Number.POSITIVE_INFINITY);
    return child.getMaxIntrinsicWidth(finiteHeight);
  }

  protected override computeMinIntrinsicHeight(width: number): number {
    return this.getMaxIntrinsicHeight(width);
  }

  private _childConstraints(child: RenderBox, constraints: BoxConstraints): BoxConstraints {
    return constraints.hasTightHeight
      ? constraints
      : constraints.tighten({ height: child.getMaxIntrinsicHeight(constraints.maxWidth) });
  }

  protected override computeDryLayout(constraints: BoxConstraints): Size {
    const child = this.child;
    return child === null ? constraints.smallest : child.getDryLayout(this._childConstraints(child, constraints));
  }

  protected override computeDryBaseline(constraints: BoxConstraints, baseline: TextBaseline): number | null {
    const child = this.child;
    return child === null ? null : child.getDryBaseline(this._childConstraints(child, constraints), baseline);
  }

  protected override performLayout(): void {
    const child = this.child;
    if (child === null) {
      this.size = this.constraints.smallest;
      return;
    }
    child.layout(this._childConstraints(child, this.constraints), { parentUsesSize: true });
    this.size = child.size;
  }
}

/** Layout is the child's; paints it with `opacity` (`Opacity`, `FadeTransition`, `AnimatedOpacity`). */
export class RenderOpacity extends RenderProxyBox {
  /** The opacity in [0, 1]; changing it affects painting only. */
  opacity: number;

  constructor(options: { opacity?: number; child?: RenderBox | null } = {}) {
    super(options.child ?? null);
    this.opacity = options.opacity ?? 1;
  }
}

/** The shape a clip box clips its child to. */
export enum ClipShape {
  rect = "rect",
  rrect = "rrect",
  oval = "oval",
  path = "path",
}

/**
 * `ClipRect`, `ClipRRect`, `ClipOval` and `ClipPath` with their default
 * clippers: layout is the child's; the clip is the box's own bounds
 * ([clipBounds]). The rounded corner radii and the path are painting.
 */
export class RenderClip extends RenderProxyBox {
  readonly shape: ClipShape;

  constructor(options: { shape: ClipShape; child?: RenderBox | null }) {
    super(options.child ?? null);
    this.shape = options.shape;
  }

  /** The clip's bounds in this box's coordinates. */
  get clipBounds(): Rect {
    return Rect.fromLTWH(0, 0, this.size.width, this.size.height);
  }
}

/** Paints the child fitted into the box (`FittedBox`): the child is laid out unconstrained, then scaled. */
export class RenderFittedBox extends RenderProxyBox implements PaintTransformer {
  private _fit: BoxFit;
  private _alignment: AlignmentGeometry;
  private _textDirection: TextDirection | null;

  constructor(
    options: {
      fit?: BoxFit;
      alignment?: AlignmentGeometry;
      textDirection?: TextDirection | null;
      child?: RenderBox | null;
    } = {},
  ) {
    super(options.child ?? null);
    this._fit = options.fit ?? BoxFit.contain;
    this._alignment = options.alignment ?? Alignment.center;
    this._textDirection = options.textDirection ?? null;
  }

  get fit(): BoxFit {
    return this._fit;
  }

  set fit(value: BoxFit) {
    if (this._fit === value) {
      return;
    }
    const affectsLayout = (fit: BoxFit): boolean => fit === BoxFit.scaleDown;
    const last = this._fit;
    this._fit = value;
    if (affectsLayout(last) || affectsLayout(value)) {
      this.markNeedsLayout();
    }
  }

  get alignment(): AlignmentGeometry {
    return this._alignment;
  }

  set alignment(value: AlignmentGeometry) {
    this._alignment = value;
  }

  get textDirection(): TextDirection | null {
    return this._textDirection;
  }

  set textDirection(value: TextDirection | null) {
    this._textDirection = value;
  }

  // Flutter notes (TODO(ianh)) that the intrinsic dimensions of this box are
  // wrong: they are the child's, not the scaled size. Kept for fidelity.

  private _sizeFor(constraints: BoxConstraints, childSize: Size): Size {
    if (this._fit === BoxFit.scaleDown) {
      return constraints.constrain(constraints.loosen().constrainSizeAndAttemptToPreserveAspectRatio(childSize));
    }
    return constraints.constrainSizeAndAttemptToPreserveAspectRatio(childSize);
  }

  protected override computeDryLayout(constraints: BoxConstraints): Size {
    const child = this.child;
    return child === null ? constraints.smallest : this._sizeFor(constraints, child.getDryLayout(new BoxConstraints()));
  }

  protected override computeDryBaseline(_constraints: BoxConstraints, baseline: TextBaseline): number | null {
    // The child's baseline with unconstrained layout, without the paint scale.
    return this.child?.getDryBaseline(new BoxConstraints(), baseline) ?? null;
  }

  protected override performLayout(): void {
    const child = this.child;
    if (child === null) {
      this.size = this.constraints.smallest;
      return;
    }
    child.layout(new BoxConstraints(), { parentUsesSize: true });
    this.size = this._sizeFor(this.constraints, child.size);
  }

  private get _resolvedAlignment(): Alignment {
    return this._alignment.resolve(this._textDirection);
  }

  /** The transform from the child's coordinates to this box's (`_updatePaintData`). */
  get childTransform(): Affine {
    const child = this.child;
    if (child === null) {
      return Affine.identity;
    }
    const childSize = child.size;
    const sizes = applyBoxFit(this._fit, childSize, this.size);
    const scaleX = sizes.destination.width / sizes.source.width;
    const scaleY = sizes.destination.height / sizes.source.height;
    const alignment = this._resolvedAlignment;
    const sourceRect = alignment.inscribe(sizes.source, Rect.fromLTWH(0, 0, childSize.width, childSize.height));
    const destinationRect = alignment.inscribe(
      sizes.destination,
      Rect.fromLTWH(0, 0, this.size.width, this.size.height),
    );
    return Affine.translation(destinationRect.left, destinationRect.top)
      .scale(scaleX, scaleY)
      .translate(-sourceRect.left, -sourceRect.top);
  }

  /** Whether the scaled child is cropped by the box (`_hasVisualOverflow`). */
  get hasVisualOverflow(): boolean {
    const child = this.child;
    if (child === null) {
      return false;
    }
    const childSize = child.size;
    const sizes = applyBoxFit(this._fit, childSize, this.size);
    const sourceRect = this._resolvedAlignment.inscribe(
      sizes.source,
      Rect.fromLTWH(0, 0, childSize.width, childSize.height),
    );
    return sourceRect.width < childSize.width || sourceRect.height < childSize.height;
  }

  paintTransformOf(_child: RenderObject): Affine {
    return this.childTransform;
  }
}

/** Applies a transform when painting and hit testing, not in layout (`Transform`, `ScaleTransition`). */
export class RenderTransform extends RenderProxyBox implements PaintTransformer {
  transform: Affine;
  origin: Offset | null;
  alignment: AlignmentGeometry | null;
  textDirection: TextDirection | null;

  constructor(options: {
    transform: Affine;
    origin?: Offset | null;
    alignment?: AlignmentGeometry | null;
    textDirection?: TextDirection | null;
    child?: RenderBox | null;
  }) {
    super(options.child ?? null);
    this.transform = options.transform;
    this.origin = options.origin ?? null;
    this.alignment = options.alignment ?? null;
    this.textDirection = options.textDirection ?? null;
  }

  /** The transform conjugated by the origin and the resolved alignment (`_effectiveTransform`). */
  get effectiveTransform(): Affine {
    const resolved = this.alignment?.resolve(this.textDirection) ?? null;
    if (this.origin === null && resolved === null) {
      return this.transform;
    }
    let result = Affine.identity;
    if (this.origin !== null) {
      result = result.translate(this.origin.dx, this.origin.dy);
    }
    const translation = resolved === null ? null : resolved.alongSize(this.size);
    if (translation !== null) {
      result = result.translate(translation.dx, translation.dy);
    }
    result = result.multiply(this.transform);
    if (translation !== null) {
      result = result.translate(-translation.dx, -translation.dy);
    }
    if (this.origin !== null) {
      result = result.translate(-this.origin.dx, -this.origin.dy);
    }
    return result;
  }

  /** The box where the transformed child appears, in this box's parent's coordinates (the hit rect). */
  get transformedBounds(): Rect {
    return this.effectiveTransform.transformRect(Rect.fromLTWH(0, 0, this.size.width, this.size.height));
  }

  paintTransformOf(_child: RenderObject): Affine {
    return this.effectiveTransform;
  }
}

/** Translates its child by a fraction of its own size when painting (`FractionalTranslation`, `SlideTransition`). */
export class RenderFractionalTranslation extends RenderProxyBox implements PaintTransformer {
  translation: Offset;
  transformHitTests: boolean;

  constructor(options: { translation: Offset; transformHitTests?: boolean; child?: RenderBox | null }) {
    super(options.child ?? null);
    this.translation = options.translation;
    this.transformHitTests = options.transformHitTests ?? true;
  }

  /** The painted translation in logical pixels. */
  get paintOffset(): Offset {
    return new Offset(this.translation.dx * this.size.width, this.translation.dy * this.size.height);
  }

  paintTransformOf(_child: RenderObject): Affine {
    const offset = this.paintOffset;
    return Affine.translation(offset.dx, offset.dy);
  }
}
