// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/painting/text_painter.dart
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/**
 * `TextPainter`'s layout: builds a paragraph from a [TextSpan], lays it out
 * between a minimum and maximum width with Flutter's rules — the content
 * width from [TextWidthBasis], the relayout at the max intrinsic width when
 * the paragraph is aligned in unbounded width, and the cache that skips line
 * breaking when the width does not change it. Painting, carets and
 * selection are not ported.
 */

import type { EngineParagraph, ParagraphEngine, TextHeightBehavior } from "../text/paragraph-engine.ts";
import { TextAlign } from "../text/paragraph-engine.ts";
import { clampDouble, precisionErrorTolerance, Size, TextDirection } from "./geometry.ts";
import { TextBaseline } from "./render-box.ts";
import { buildTextRuns, getParagraphStyle, type StrutStyle, TextScaler, type TextSpan } from "./text-style.ts";

/** How a paragraph's width is measured (`TextWidthBasis`). */
export enum TextWidthBasis {
  /** As wide as the constraints allow (up to the unwrapped width). */
  parent = "parent",
  /** As wide as the longest line. */
  longestLine = "longestLine",
}

/** `_TextLayout`: a laid-out paragraph and its metrics. */
class TextLayout {
  constructor(readonly paragraph: EngineParagraph) {}

  get width(): number {
    return this.paragraph.width;
  }

  get height(): number {
    return this.paragraph.height;
  }

  get minIntrinsicLineExtent(): number {
    return this.paragraph.minIntrinsicWidth;
  }

  get maxIntrinsicLineExtent(): number {
    return this.paragraph.maxIntrinsicWidth;
  }

  get longestLine(): number {
    return this.paragraph.longestLine;
  }

  getDistanceToBaseline(baseline: TextBaseline): number {
    return baseline === TextBaseline.alphabetic
      ? this.paragraph.alphabeticBaseline
      : this.paragraph.ideographicBaseline;
  }

  contentWidthFor(minWidth: number, maxWidth: number, widthBasis: TextWidthBasis): number {
    return widthBasis === TextWidthBasis.longestLine
      ? clampDouble(this.longestLine, minWidth, maxWidth)
      : clampDouble(this.maxIntrinsicLineExtent, minWidth, maxWidth);
  }
}

/** `_TextPainterLayoutCacheWithOffset`. */
class LayoutCache {
  constructor(
    readonly layout: TextLayout,
    readonly textAlignment: number,
    readonly layoutMaxWidth: number,
    public contentWidth: number,
  ) {}

  get paragraph(): EngineParagraph {
    return this.layout.paragraph;
  }

  get paintOffsetDx(): number {
    if (this.textAlignment === 0) {
      return 0;
    }
    if (!Number.isFinite(this.paragraph.width)) {
      return Number.POSITIVE_INFINITY;
    }
    return this.textAlignment * (this.contentWidth - this.paragraph.width);
  }

  /** `_resizeToFit`: whether the cached line breaks still hold for the new widths. */
  resizeToFit(minWidth: number, maxWidth: number, widthBasis: TextWidthBasis): boolean {
    if (maxWidth === this.contentWidth && minWidth === this.contentWidth) {
      this.contentWidth = this.layout.contentWidthFor(minWidth, maxWidth, widthBasis);
      return true;
    }
    if (!Number.isFinite(this.paintOffsetDx) && !Number.isFinite(this.paragraph.width) && Number.isFinite(minWidth)) {
      return false;
    }
    const maxIntrinsicWidth = this.paragraph.maxIntrinsicWidth;
    const skipLineBreaking =
      maxWidth === this.layoutMaxWidth ||
      (this.paragraph.width - maxIntrinsicWidth > -precisionErrorTolerance &&
        maxWidth - maxIntrinsicWidth > -precisionErrorTolerance);
    if (skipLineBreaking) {
      this.contentWidth = this.layout.contentWidthFor(minWidth, maxWidth, widthBasis);
      return true;
    }
    return false;
  }
}

/** The configuration of a [TextPainter]. */
export interface TextPainterOptions {
  engine: ParagraphEngine;
  text?: TextSpan | null | undefined;
  textAlign?: TextAlign | undefined;
  textDirection?: TextDirection | null | undefined;
  textScaler?: TextScaler | undefined;
  maxLines?: number | null | undefined;
  ellipsis?: string | null | undefined;
  strutStyle?: StrutStyle | null | undefined;
  textWidthBasis?: TextWidthBasis | undefined;
  textHeightBehavior?: TextHeightBehavior | null | undefined;
}

/** `_computePaintOffsetFraction`: where in the content width a line box sits. */
export function computePaintOffsetFraction(textAlign: TextAlign, textDirection: TextDirection): number {
  switch (textAlign) {
    case TextAlign.left:
      return 0;
    case TextAlign.right:
      return 1;
    case TextAlign.center:
      return 0.5;
    case TextAlign.start:
    case TextAlign.justify:
      return textDirection === TextDirection.ltr ? 0 : 1;
    case TextAlign.end:
      return textDirection === TextDirection.ltr ? 1 : 0;
  }
}

/** Lays out a [TextSpan] (`TextPainter`). Call [dispose] to release the paragraph. */
export class TextPainter {
  readonly engine: ParagraphEngine;
  text: TextSpan | null;
  textAlign: TextAlign;
  textDirection: TextDirection | null;
  textScaler: TextScaler;
  maxLines: number | null;
  ellipsis: string | null;
  strutStyle: StrutStyle | null;
  textWidthBasis: TextWidthBasis;
  textHeightBehavior: TextHeightBehavior | null;
  private _layoutCache: LayoutCache | null = null;

  constructor(options: TextPainterOptions) {
    this.engine = options.engine;
    this.text = options.text ?? null;
    this.textAlign = options.textAlign ?? TextAlign.start;
    this.textDirection = options.textDirection ?? null;
    this.textScaler = options.textScaler ?? TextScaler.noScaling;
    this.maxLines = options.maxLines ?? null;
    this.ellipsis = options.ellipsis ?? null;
    this.strutStyle = options.strutStyle ?? null;
    this.textWidthBasis = options.textWidthBasis ?? TextWidthBasis.parent;
    this.textHeightBehavior = options.textHeightBehavior ?? null;
  }

  /**
   * Drops the laid-out paragraph after a change to the text or its style
   * (Flutter's property setters call this when the value changes).
   */
  markNeedsLayout(): void {
    this._layoutCache?.paragraph.dispose();
    this._layoutCache = null;
  }

  /** Releases the paragraph (`TextPainter.dispose`). */
  dispose(): void {
    this.markNeedsLayout();
  }

  private get _cache(): LayoutCache {
    if (this._layoutCache === null) {
      throw new Error("TextPainter.layout must be called before reading its metrics");
    }
    return this._layoutCache;
  }

  get minIntrinsicWidth(): number {
    return this._cache.layout.minIntrinsicLineExtent;
  }

  get maxIntrinsicWidth(): number {
    return this._cache.layout.maxIntrinsicLineExtent;
  }

  get width(): number {
    return this._cache.contentWidth;
  }

  get height(): number {
    return this._cache.layout.height;
  }

  get size(): Size {
    return new Size(this.width, this.height);
  }

  get didExceedMaxLines(): boolean {
    return this._cache.paragraph.didExceedMaxLines;
  }

  /** The horizontal offset the paragraph is painted at within [width]. */
  get paintOffsetDx(): number {
    return this._cache.paintOffsetDx;
  }

  computeDistanceToActualBaseline(baseline: TextBaseline): number {
    return this._cache.layout.getDistanceToBaseline(baseline);
  }

  /** The metrics of every laid-out line. */
  computeLineMetrics(): ReturnType<EngineParagraph["lineMetrics"]> {
    return this._cache.paragraph.lineMetrics();
  }

  private _createParagraph(text: TextSpan, textDirection: TextDirection): EngineParagraph {
    const style = getParagraphStyle(text.style ?? {}, {
      textAlign: this.textAlign,
      textDirection,
      textScaler: this.textScaler,
      maxLines: this.maxLines ?? undefined,
      textHeightBehavior: this.textHeightBehavior ?? undefined,
      ellipsis: this.ellipsis ?? undefined,
      strutStyle: this.strutStyle ?? undefined,
    });
    return this.engine.buildParagraph(style, buildTextRuns(text, this.textScaler));
  }

  /** `TextPainter.layout`. */
  layout(options: { minWidth?: number; maxWidth?: number } = {}): void {
    const minWidth = options.minWidth ?? 0;
    const maxWidth = options.maxWidth ?? Number.POSITIVE_INFINITY;
    const cachedLayout = this._layoutCache;
    if (cachedLayout?.resizeToFit(minWidth, maxWidth, this.textWidthBasis)) {
      return;
    }
    const text = this.text;
    if (text === null) {
      throw new Error("TextPainter.text must be set to a non-null value before using the TextPainter.");
    }
    const textDirection = this.textDirection;
    if (textDirection === null) {
      throw new Error("TextPainter.textDirection must be set to a non-null value before using the TextPainter.");
    }
    const paintOffsetAlignment = computePaintOffsetFraction(this.textAlign, textDirection);
    // An aligned paragraph in unbounded width would be painted at infinity:
    // lay it out at its max intrinsic width instead.
    const adjustMaxWidth = !Number.isFinite(maxWidth) && paintOffsetAlignment !== 0;
    const adjustedMaxWidth = !adjustMaxWidth ? maxWidth : (cachedLayout?.layout.maxIntrinsicLineExtent ?? null);
    const layoutMaxWidth = adjustedMaxWidth ?? maxWidth;
    const paragraph = cachedLayout?.paragraph ?? this._createParagraph(text, textDirection);
    paragraph.layout(layoutMaxWidth);
    const layout = new TextLayout(paragraph);
    const contentWidth = layout.contentWidthFor(minWidth, maxWidth, this.textWidthBasis);
    if (adjustedMaxWidth === null && Number.isFinite(minWidth)) {
      const newInputWidth = layout.maxIntrinsicLineExtent;
      paragraph.layout(newInputWidth);
      this._layoutCache = new LayoutCache(layout, paintOffsetAlignment, newInputWidth, contentWidth);
    } else {
      this._layoutCache = new LayoutCache(layout, paintOffsetAlignment, layoutMaxWidth, contentWidth);
    }
  }
}
