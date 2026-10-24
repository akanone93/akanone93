// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/rendering/paragraph.dart
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/**
 * `RenderParagraph`'s layout: a paragraph of text sized by its
 * [TextPainter] within the box constraints, with `softWrap`, `overflow`,
 * `maxLines`, the intrinsic widths (min = the longest word, max = the
 * unwrapped line) and heights, dry layout and the alphabetic baseline.
 * Inline widget children, painting, selection and semantics are not ported.
 * The paragraph engine is passed in; [dispose] releases its paragraphs.
 */

import type { ParagraphEngine, TextAlign, TextHeightBehavior } from "../text/paragraph-engine.ts";
import type { BoxConstraints } from "./constraints.ts";
import type { Size, TextDirection } from "./geometry.ts";
import { RenderBox, TextBaseline } from "./render-box.ts";
import { TextPainter, type TextWidthBasis } from "./text-painter.ts";
import type { StrutStyle, TextScaler, TextSpan } from "./text-style.ts";

/** How overflowing text is shown (`TextOverflow`). */
export enum TextOverflow {
  clip = "clip",
  fade = "fade",
  ellipsis = "ellipsis",
  visible = "visible",
}

/** `_kEllipsis`: the ellipsis `TextOverflow.ellipsis` appends. */
const kEllipsis = "…";

/** The configuration of a [RenderParagraph]; defaults are Flutter's. */
export interface RenderParagraphOptions {
  text: TextSpan;
  textDirection: TextDirection;
  engine: ParagraphEngine;
  textAlign?: TextAlign | undefined;
  softWrap?: boolean | undefined;
  overflow?: TextOverflow | undefined;
  textScaler?: TextScaler | undefined;
  maxLines?: number | null | undefined;
  strutStyle?: StrutStyle | null | undefined;
  textWidthBasis?: TextWidthBasis | undefined;
  textHeightBehavior?: TextHeightBehavior | null | undefined;
}

/** A paragraph of text (`RenderParagraph`). */
export class RenderParagraph extends RenderBox {
  private readonly _textPainter: TextPainter;
  private _textIntrinsicsCache: TextPainter | null = null;
  private _softWrap: boolean;
  private _overflow: TextOverflow;
  private _needsClipping = false;
  private _hasOverflowShader = false;

  constructor(options: RenderParagraphOptions) {
    super();
    this._softWrap = options.softWrap ?? true;
    this._overflow = options.overflow ?? TextOverflow.clip;
    this._textPainter = new TextPainter({
      engine: options.engine,
      text: options.text,
      textAlign: options.textAlign,
      textDirection: options.textDirection,
      textScaler: options.textScaler,
      maxLines: options.maxLines,
      ellipsis: this._overflow === TextOverflow.ellipsis ? kEllipsis : null,
      strutStyle: options.strutStyle,
      textWidthBasis: options.textWidthBasis,
      textHeightBehavior: options.textHeightBehavior,
    });
  }

  /**
   * The painter for intrinsics and dry layout, so they never disturb the
   * laid-out paragraph (`_textIntrinsics`).
   */
  private get _textIntrinsics(): TextPainter {
    const p = this._textPainter;
    if (this._textIntrinsicsCache === null) {
      this._textIntrinsicsCache = new TextPainter({ engine: p.engine });
    }
    const t = this._textIntrinsicsCache;
    if (
      t.text !== p.text ||
      t.textAlign !== p.textAlign ||
      t.textDirection !== p.textDirection ||
      !t.textScaler.equals(p.textScaler) ||
      t.maxLines !== p.maxLines ||
      t.ellipsis !== p.ellipsis ||
      t.strutStyle !== p.strutStyle ||
      t.textWidthBasis !== p.textWidthBasis ||
      t.textHeightBehavior !== p.textHeightBehavior
    ) {
      t.text = p.text;
      t.textAlign = p.textAlign;
      t.textDirection = p.textDirection;
      t.textScaler = p.textScaler;
      t.maxLines = p.maxLines;
      t.ellipsis = p.ellipsis;
      t.strutStyle = p.strutStyle;
      t.textWidthBasis = p.textWidthBasis;
      t.textHeightBehavior = p.textHeightBehavior;
      t.markNeedsLayout();
    }
    return t;
  }

  /** Changes a painter property and relayouts when it differs. */
  private _update<K extends keyof TextPainter>(key: K, value: TextPainter[K]): void {
    if (this._textPainter[key] === value) {
      return;
    }
    (this._textPainter as Pick<TextPainter, K>)[key] = value;
    this._textPainter.markNeedsLayout();
    this.markNeedsLayout();
  }

  get text(): TextSpan {
    return this._textPainter.text as TextSpan;
  }

  set text(value: TextSpan) {
    this._update("text", value);
  }

  get textAlign(): TextAlign {
    return this._textPainter.textAlign;
  }

  set textAlign(value: TextAlign) {
    this._update("textAlign", value);
  }

  get textDirection(): TextDirection {
    return this._textPainter.textDirection as TextDirection;
  }

  set textDirection(value: TextDirection) {
    this._update("textDirection", value);
  }

  get textScaler(): TextScaler {
    return this._textPainter.textScaler;
  }

  set textScaler(value: TextScaler) {
    if (!this._textPainter.textScaler.equals(value)) {
      this._update("textScaler", value);
    }
  }

  get maxLines(): number | null {
    return this._textPainter.maxLines;
  }

  set maxLines(value: number | null) {
    this._update("maxLines", value);
  }

  get textWidthBasis(): TextWidthBasis {
    return this._textPainter.textWidthBasis;
  }

  set textWidthBasis(value: TextWidthBasis) {
    this._update("textWidthBasis", value);
  }

  get softWrap(): boolean {
    return this._softWrap;
  }

  set softWrap(value: boolean) {
    if (this._softWrap === value) {
      return;
    }
    this._softWrap = value;
    this.markNeedsLayout();
  }

  get overflow(): TextOverflow {
    return this._overflow;
  }

  set overflow(value: TextOverflow) {
    if (this._overflow === value) {
      return;
    }
    this._overflow = value;
    this._update("ellipsis", value === TextOverflow.ellipsis ? kEllipsis : null);
    this.markNeedsLayout();
  }

  /** Whether the last layout overflowed and painting clips (`_needsClipping`). */
  get needsClipping(): boolean {
    return this._needsClipping;
  }

  /** Whether `TextOverflow.fade` applies a fade (`debugHasOverflowShader`). */
  get hasOverflowShader(): boolean {
    return this._hasOverflowShader;
  }

  /** The painter after layout, for line metrics and the paint offset. */
  get textPainter(): TextPainter {
    return this._textPainter;
  }

  /** Releases the paragraphs of both painters. */
  dispose(): void {
    this._textPainter.dispose();
    this._textIntrinsicsCache?.dispose();
  }

  protected override computeMinIntrinsicWidth(_height: number): number {
    const t = this._textIntrinsics;
    t.layout();
    return t.minIntrinsicWidth;
  }

  protected override computeMaxIntrinsicWidth(_height: number): number {
    const t = this._textIntrinsics;
    t.layout();
    return t.maxIntrinsicWidth;
  }

  private _computeIntrinsicHeight(width: number): number {
    const t = this._textIntrinsics;
    t.layout({ minWidth: width, maxWidth: this._adjustMaxWidth(width) });
    return t.height;
  }

  protected override computeMinIntrinsicHeight(width: number): number {
    return this._computeIntrinsicHeight(width);
  }

  protected override computeMaxIntrinsicHeight(width: number): number {
    return this._computeIntrinsicHeight(width);
  }

  /** Without soft wrapping or ellipsis, lines break only at hard breaks. */
  private _adjustMaxWidth(maxWidth: number): number {
    return this._softWrap || this._overflow === TextOverflow.ellipsis ? maxWidth : Number.POSITIVE_INFINITY;
  }

  private _layoutTextWithConstraints(constraints: BoxConstraints): void {
    this._textPainter.layout({ minWidth: constraints.minWidth, maxWidth: this._adjustMaxWidth(constraints.maxWidth) });
  }

  protected override computeDryLayout(constraints: BoxConstraints): Size {
    const t = this._textIntrinsics;
    t.layout({ minWidth: constraints.minWidth, maxWidth: this._adjustMaxWidth(constraints.maxWidth) });
    return constraints.constrain(t.size);
  }

  // Flutter 3.47 reports the alphabetic baseline for either baseline request.
  protected override computeDistanceToActualBaseline(_baseline: TextBaseline): number | null {
    this._layoutTextWithConstraints(this.constraints as BoxConstraints);
    return this._textPainter.computeDistanceToActualBaseline(TextBaseline.alphabetic);
  }

  protected override computeDryBaseline(constraints: BoxConstraints, _baseline: TextBaseline): number | null {
    const t = this._textIntrinsics;
    t.layout({ minWidth: constraints.minWidth, maxWidth: this._adjustMaxWidth(constraints.maxWidth) });
    return t.computeDistanceToActualBaseline(TextBaseline.alphabetic);
  }

  protected override performLayout(): void {
    const constraints = this.constraints as BoxConstraints;
    this._layoutTextWithConstraints(constraints);
    const textSize = this._textPainter.size;
    this.size = constraints.constrain(textSize);
    const didOverflowHeight = this.size.height < textSize.height || this._textPainter.didExceedMaxLines;
    const didOverflowWidth = this.size.width < textSize.width;
    const hasVisualOverflow = didOverflowWidth || didOverflowHeight;
    if (!hasVisualOverflow) {
      this._needsClipping = false;
      this._hasOverflowShader = false;
      return;
    }
    switch (this._overflow) {
      case TextOverflow.visible:
        this._needsClipping = false;
        this._hasOverflowShader = false;
        break;
      case TextOverflow.clip:
      case TextOverflow.ellipsis:
        this._needsClipping = true;
        this._hasOverflowShader = false;
        break;
      case TextOverflow.fade:
        this._needsClipping = true;
        this._hasOverflowShader = true;
        break;
    }
  }
}
