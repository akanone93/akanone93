// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/painting/text_style.dart, text_scaler.dart, strut_style.dart, text_span.dart
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/**
 * The text styling Flutter's `TextPainter` hands to the paragraph engine:
 * `TextScaler` (linear), `TextStyle` with `getTextStyle` and
 * `getParagraphStyle`, `StrutStyle` and `TextSpan`. Only the fields that
 * affect layout are ported; colours, decorations and shadows are not.
 */

import type {
  EngineParagraphStyle,
  EngineTextRun,
  EngineTextStyle,
  FontFeature,
  FontStyle,
  TextAlign,
  TextHeightBehavior,
  TextLeadingDistribution,
} from "../text/paragraph-engine.ts";
import type { TextDirection } from "./geometry.ts";

/** `kDefaultFontSize`: the font size when no style sets one. */
export const kDefaultFontSize = 14;

/** Scales font sizes linearly by [textScaleFactor] (`TextScaler.linear`). */
export class TextScaler {
  /** `TextScaler.noScaling`. */
  static readonly noScaling: TextScaler = new TextScaler(1);

  readonly textScaleFactor: number;

  private constructor(textScaleFactor: number) {
    this.textScaleFactor = textScaleFactor;
  }

  /** `TextScaler.linear`: scaling by 1 is [noScaling]. */
  static linear(textScaleFactor: number): TextScaler {
    return textScaleFactor === 1 ? TextScaler.noScaling : new TextScaler(textScaleFactor);
  }

  scale(fontSize: number): number {
    return fontSize * this.textScaleFactor;
  }

  equals(other: TextScaler): boolean {
    return this.textScaleFactor === other.textScaleFactor;
  }
}

/** The layout-relevant fields of Flutter's `TextStyle`; an absent field inherits. */
export interface TextStyle {
  fontFamily?: string | undefined;
  fontFamilyFallback?: readonly string[] | undefined;
  fontSize?: number | undefined;
  /** 100 to 900 (`FontWeight.value`). */
  fontWeight?: number | undefined;
  fontStyle?: FontStyle | undefined;
  letterSpacing?: number | undefined;
  wordSpacing?: number | undefined;
  height?: number | undefined;
  leadingDistribution?: TextLeadingDistribution | undefined;
  fontFeatures?: readonly FontFeature[] | undefined;
}

/** The layout fields of Flutter's `StrutStyle`. */
export interface StrutStyle {
  fontFamily?: string | undefined;
  fontFamilyFallback?: readonly string[] | undefined;
  fontSize?: number | undefined;
  height?: number | undefined;
  leading?: number | undefined;
  leadingDistribution?: TextLeadingDistribution | undefined;
  fontWeight?: number | undefined;
  fontStyle?: FontStyle | undefined;
  forceStrutHeight?: boolean | undefined;
}

/** `TextStyle.getTextStyle`: the engine style with the font size scaled by `textScaler`. */
export function getTextStyle(style: TextStyle, textScaler: TextScaler = TextScaler.noScaling): EngineTextStyle {
  const { fontSize, ...rest } = style;
  return fontSize === undefined ? { ...rest } : { ...rest, fontSize: textScaler.scale(fontSize) };
}

/** The arguments of `TextStyle.getParagraphStyle` the text painter passes. */
export interface ParagraphStyleArgs {
  textAlign?: TextAlign | undefined;
  textDirection: TextDirection;
  textScaler?: TextScaler | undefined;
  ellipsis?: string | undefined;
  maxLines?: number | undefined;
  textHeightBehavior?: TextHeightBehavior | undefined;
  strutStyle?: StrutStyle | undefined;
}

/**
 * `TextStyle.getParagraphStyle`: the paragraph's default font, with the
 * font size defaulting to [kDefaultFontSize] and scaled, and the strut's
 * font size scaled; `leadingDistribution` becomes the text height behaviour
 * when none is given.
 */
export function getParagraphStyle(style: TextStyle, args: ParagraphStyleArgs): EngineParagraphStyle {
  const textScaler = args.textScaler ?? TextScaler.noScaling;
  const textHeightBehavior =
    args.textHeightBehavior ??
    (style.leadingDistribution === undefined ? undefined : { leadingDistribution: style.leadingDistribution });
  const strut = args.strutStyle;
  return {
    textAlign: args.textAlign,
    textDirection: args.textDirection,
    fontWeight: style.fontWeight,
    fontStyle: style.fontStyle,
    fontFamily: style.fontFamily,
    fontSize: textScaler.scale(style.fontSize ?? kDefaultFontSize),
    height: style.height,
    textHeightBehavior,
    strutStyle:
      strut === undefined
        ? undefined
        : { ...strut, fontSize: strut.fontSize === undefined ? undefined : textScaler.scale(strut.fontSize) },
    maxLines: args.maxLines,
    ellipsis: args.ellipsis,
  };
}

/** A span of text with a style and child spans (`TextSpan`; placeholders are not ported). */
export interface TextSpan {
  text?: string | undefined;
  style?: TextStyle | undefined;
  children?: readonly TextSpan[] | undefined;
}

/**
 * `TextSpan.build`: the span tree as runs whose style inherits from the
 * enclosing spans, as the engine's style stack does.
 */
export function buildTextRuns(
  span: TextSpan,
  textScaler: TextScaler = TextScaler.noScaling,
  inherited: EngineTextStyle | null = null,
  runs: EngineTextRun[] = [],
): EngineTextRun[] {
  const style = span.style === undefined ? inherited : { ...inherited, ...getTextStyle(span.style, textScaler) };
  if (span.text !== undefined) {
    runs.push({ text: span.text, style });
  }
  for (const child of span.children ?? []) {
    buildTextRuns(child, textScaler, style, runs);
  }
  return runs;
}

/** `InlineSpan.toPlainText`. */
export function plainText(span: TextSpan): string {
  return (span.text ?? "") + (span.children ?? []).map(plainText).join("");
}
