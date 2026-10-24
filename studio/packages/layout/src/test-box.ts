// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * A leaf box with a preferred size and fixed intrinsics, for tests and
 * conformance scenes that need a child of known size without text.
 */

import type { BoxConstraints } from "./constraints.ts";
import type { Size } from "./geometry.ts";
import { RenderBox, type TextBaseline } from "./render-box.ts";

/** Options for [RenderTestBox]; every intrinsic defaults to the preferred size. */
export interface RenderTestBoxOptions {
  /** The size the box asks for, constrained by its parent. */
  size: Size;
  minIntrinsicWidth?: number;
  maxIntrinsicWidth?: number;
  minIntrinsicHeight?: number;
  maxIntrinsicHeight?: number;
  /** The distance from the top to the alphabetic and ideographic baseline; none when omitted. */
  baseline?: number;
  /** Size purely from the constraints, making the box a relayout boundary. */
  sizedByParent?: boolean;
}

/** A childless box that takes `size` constrained by its parent and counts its layouts. */
export class RenderTestBox extends RenderBox {
  private _preferredSize: Size;
  private readonly _options: RenderTestBoxOptions;
  /** How many times [performLayout] has run. */
  layoutCount = 0;
  /** How many times an intrinsic dimension was computed (cache misses). */
  intrinsicComputeCount = 0;
  /** How many times a dry layout was computed (cache misses). */
  dryLayoutComputeCount = 0;

  constructor(options: RenderTestBoxOptions) {
    super();
    this._options = options;
    this._preferredSize = options.size;
  }

  get preferredSize(): Size {
    return this._preferredSize;
  }

  set preferredSize(value: Size) {
    if (this._preferredSize.equals(value)) {
      return;
    }
    this._preferredSize = value;
    this.markNeedsLayout();
  }

  override get sizedByParent(): boolean {
    return this._options.sizedByParent ?? false;
  }

  protected override computeMinIntrinsicWidth(_height: number): number {
    this.intrinsicComputeCount += 1;
    return this._options.minIntrinsicWidth ?? this._preferredSize.width;
  }

  protected override computeMaxIntrinsicWidth(_height: number): number {
    this.intrinsicComputeCount += 1;
    return this._options.maxIntrinsicWidth ?? this._preferredSize.width;
  }

  protected override computeMinIntrinsicHeight(_width: number): number {
    this.intrinsicComputeCount += 1;
    return this._options.minIntrinsicHeight ?? this._preferredSize.height;
  }

  protected override computeMaxIntrinsicHeight(_width: number): number {
    this.intrinsicComputeCount += 1;
    return this._options.maxIntrinsicHeight ?? this._preferredSize.height;
  }

  protected override computeDryLayout(constraints: BoxConstraints): Size {
    this.dryLayoutComputeCount += 1;
    return constraints.constrain(this._preferredSize);
  }

  protected override computeDryBaseline(_constraints: BoxConstraints, _baseline: TextBaseline): number | null {
    return this._options.baseline ?? null;
  }

  protected override computeDistanceToActualBaseline(_baseline: TextBaseline): number | null {
    return this._options.baseline ?? null;
  }

  protected override performLayout(): void {
    this.layoutCount += 1;
    if (!this.sizedByParent) {
      this.size = this.constraints.constrain(this._preferredSize);
    }
  }
}
