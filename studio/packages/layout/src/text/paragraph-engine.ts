// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The paragraph engine the text layout port drives (STU-005, STU-006,
 * ADR-0013): the `dart:ui` paragraph surface Flutter's `TextPainter` uses —
 * build a paragraph from a paragraph style and styled runs, lay it out at a
 * width, read its metrics. The types mirror `dart:ui`'s `ParagraphStyle`,
 * `TextStyle` and `StrutStyle`; [CanvasKitParagraphEngine] implements the
 * engine on SkParagraph, the paragraph layout the device uses. Engines are
 * passed to the render objects that need them, never held globally.
 */

import type { TextDirection } from "../flutter/geometry.ts";

/** How lines of text are aligned horizontally (`dart:ui` `TextAlign`). */
export enum TextAlign {
  left = "left",
  right = "right",
  center = "center",
  justify = "justify",
  start = "start",
  end = "end",
}

/** Where a line's leading goes (`dart:ui` `TextLeadingDistribution`). */
export enum TextLeadingDistribution {
  proportional = "proportional",
  even = "even",
}

/** Whether glyphs are upright or slanted (`dart:ui` `FontStyle`). */
export enum FontStyle {
  normal = "normal",
  italic = "italic",
}

/** Where `height` applies within a paragraph (`dart:ui` `TextHeightBehavior`). */
export interface TextHeightBehavior {
  applyHeightToFirstAscent?: boolean | undefined;
  applyHeightToLastDescent?: boolean | undefined;
  leadingDistribution?: TextLeadingDistribution | undefined;
}

/** An OpenType feature setting (`dart:ui` `FontFeature`). */
export interface FontFeature {
  feature: string;
  value: number;
}

/** A run's style (`dart:ui` `TextStyle`); an absent field inherits. */
export interface EngineTextStyle {
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

/** The minimum line metrics every line uses (`dart:ui` `StrutStyle`). */
export interface EngineStrutStyle {
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

/** A paragraph's style (`dart:ui` `ParagraphStyle`); font fields are the default run style. */
export interface EngineParagraphStyle {
  textAlign?: TextAlign | undefined;
  textDirection: TextDirection;
  fontFamily?: string | undefined;
  fontSize?: number | undefined;
  fontWeight?: number | undefined;
  fontStyle?: FontStyle | undefined;
  height?: number | undefined;
  textHeightBehavior?: TextHeightBehavior | undefined;
  strutStyle?: EngineStrutStyle | undefined;
  maxLines?: number | undefined;
  ellipsis?: string | undefined;
}

/** One run of text with its fully inherited style. */
export interface EngineTextRun {
  text: string;
  style: EngineTextStyle | null;
}

/** One laid-out line (`dart:ui` `LineMetrics`). */
export interface EngineLineMetrics {
  startIndex: number;
  endIndex: number;
  hardBreak: boolean;
  ascent: number;
  descent: number;
  height: number;
  width: number;
  left: number;
  baseline: number;
  lineNumber: number;
}

/**
 * A built paragraph (`dart:ui` `Paragraph`). Metrics are valid after
 * [layout]; [dispose] releases the engine's resources.
 */
export interface EngineParagraph {
  layout(width: number): void;
  /** The width passed to the last [layout]. */
  readonly width: number;
  readonly height: number;
  readonly longestLine: number;
  readonly minIntrinsicWidth: number;
  readonly maxIntrinsicWidth: number;
  readonly alphabeticBaseline: number;
  readonly ideographicBaseline: number;
  readonly didExceedMaxLines: boolean;
  lineMetrics(): EngineLineMetrics[];
  dispose(): void;
}

/** Builds paragraphs (`dart:ui` `ParagraphBuilder`). */
export interface ParagraphEngine {
  buildParagraph(style: EngineParagraphStyle, runs: readonly EngineTextRun[]): EngineParagraph;
}
