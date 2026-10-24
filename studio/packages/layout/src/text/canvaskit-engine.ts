// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * [ParagraphEngine] on CanvasKit's SkParagraph (STU-006, ADR-0013). Studio
 * loads Flutter's own CanvasKit build at the pinned engine revision, so a
 * paragraph is shaped and broken by the same Skia, HarfBuzz and ICU as on the
 * device. The style mapping follows Flutter's CanvasKit renderer: a run
 * inherits every field it leaves unset from the paragraph's default style,
 * tabs are replaced and Skia's rounding hack is off, as in Flutter.
 *
 * CanvasKit objects live on the WebAssembly heap; every one this engine
 * creates is released with `.delete()` — builders at once, paragraphs on
 * [EngineParagraph.dispose], the font collection on [dispose].
 * [liveObjectCount] counts the objects still alive.
 */

import type {
  CanvasKit,
  FontCollection,
  Paragraph,
  ParagraphStyle,
  FontWeight as SkFontWeight,
  StrutStyle,
  TextStyle,
  TypefaceFontProvider,
} from "canvaskit-wasm";
import { TextDirection } from "../flutter/geometry.ts";
import {
  type EngineLineMetrics,
  type EngineParagraph,
  type EngineParagraphStyle,
  type EngineStrutStyle,
  type EngineTextRun,
  type EngineTextStyle,
  FontStyle,
  type ParagraphEngine,
  TextAlign,
  type TextHeightBehavior,
  TextLeadingDistribution,
} from "./paragraph-engine.ts";

interface Deletable {
  delete(): void;
}

const weightNames = [
  "Thin",
  "ExtraLight",
  "Light",
  "Normal",
  "Medium",
  "SemiBold",
  "Bold",
  "ExtraBold",
  "Black",
] as const;

/** A paragraph engine on one CanvasKit instance and the fonts registered with it. */
export class CanvasKitParagraphEngine implements ParagraphEngine {
  private readonly _ck: CanvasKit;
  private readonly _provider: TypefaceFontProvider;
  private readonly _collection: FontCollection;
  private _live = 0;
  private _disposed = false;

  constructor(canvasKit: CanvasKit) {
    this._ck = canvasKit;
    this._provider = this._track(canvasKit.TypefaceFontProvider.Make());
    this._collection = this._track(canvasKit.FontCollection.Make());
    this._collection.setDefaultFontManager(this._provider);
    this._collection.enableFontFallback();
  }

  /** CanvasKit objects this engine created and has not deleted yet. */
  get liveObjectCount(): number {
    return this._live;
  }

  /** Registers a font file (TrueType or OpenType bytes) under `family`, as Flutter's `FontLoader` does. */
  registerFont(bytes: Uint8Array, family: string): void {
    this._assertAlive();
    this._provider.registerFont(bytes, family);
  }

  buildParagraph(style: EngineParagraphStyle, runs: readonly EngineTextRun[]): EngineParagraph {
    this._assertAlive();
    const ck = this._ck;
    const base = paragraphTextStyle(style);
    const builder = this._track(
      ck.ParagraphBuilder.MakeFromFontCollection(this._paragraphStyle(style, base), this._collection),
    );
    try {
      for (const run of runs) {
        builder.pushStyle(new ck.TextStyle(this._textStyle({ ...base, ...defined(run.style ?? {}) })));
        builder.addText(run.text);
        builder.pop();
      }
      return new CanvasKitParagraph(this._track(builder.build()), (paragraph) => this._release(paragraph));
    } finally {
      this._release(builder);
    }
  }

  /** Deletes the font collection; paragraphs built earlier must be disposed first. */
  dispose(): void {
    if (this._disposed) {
      return;
    }
    this._disposed = true;
    this._release(this._collection);
    this._release(this._provider);
  }

  private _assertAlive(): void {
    if (this._disposed) {
      throw new Error("CanvasKitParagraphEngine used after dispose");
    }
  }

  private _track<T extends Deletable>(object: T): T {
    this._live += 1;
    return object;
  }

  private _release(object: Deletable): void {
    object.delete();
    this._live -= 1;
  }

  private _paragraphStyle(style: EngineParagraphStyle, base: EngineTextStyle): ParagraphStyle {
    const ck = this._ck;
    const props: ParagraphStyle = {
      textAlign: ck.TextAlign[textAlignName(style.textAlign ?? TextAlign.start)],
      textDirection: style.textDirection === TextDirection.rtl ? ck.TextDirection.RTL : ck.TextDirection.LTR,
      textHeightBehavior: this._heightBehavior(style.textHeightBehavior),
      textStyle: this._textStyle(base),
      replaceTabCharacters: true,
      applyRoundingHack: false,
    };
    if (style.height !== undefined) {
      props.heightMultiplier = style.height;
    }
    if (style.maxLines !== undefined) {
      props.maxLines = style.maxLines;
    }
    if (style.ellipsis !== undefined) {
      props.ellipsis = style.ellipsis;
    }
    if (style.strutStyle !== undefined) {
      props.strutStyle = this._strutStyle(style.strutStyle);
    }
    return new ck.ParagraphStyle(props);
  }

  private _heightBehavior(behavior: TextHeightBehavior | undefined): NonNullable<ParagraphStyle["textHeightBehavior"]> {
    const values = this._ck.TextHeightBehavior;
    const first = behavior?.applyHeightToFirstAscent ?? true;
    const last = behavior?.applyHeightToLastDescent ?? true;
    if (first && last) {
      return values.All;
    }
    if (last) {
      return values.DisableFirstAscent;
    }
    return first ? values.DisableLastDescent : values.DisableAll;
  }

  private _strutStyle(strut: EngineStrutStyle): StrutStyle {
    const props: StrutStyle = {
      strutEnabled: true,
      fontFamilies: families(strut.fontFamily, strut.fontFamilyFallback),
      fontStyle: this._fontStyle(strut.fontWeight, strut.fontStyle),
      halfLeading: strut.leadingDistribution === TextLeadingDistribution.even,
      forceStrutHeight: strut.forceStrutHeight ?? false,
    };
    if (strut.fontSize !== undefined) {
      props.fontSize = strut.fontSize;
    }
    if (strut.height !== undefined) {
      props.heightMultiplier = strut.height;
    }
    if (strut.leading !== undefined) {
      props.leading = strut.leading;
    }
    return props;
  }

  private _fontStyle(weight: number | undefined, style: FontStyle | undefined): NonNullable<TextStyle["fontStyle"]> {
    const ck = this._ck;
    const index = Math.min(Math.max(Math.round((weight ?? 400) / 100), 1), 9) - 1;
    const name = weightNames[index] ?? "Normal";
    return {
      weight: ck.FontWeight[name] as SkFontWeight,
      slant: style === FontStyle.italic ? ck.FontSlant.Italic : ck.FontSlant.Upright,
    };
  }

  private _textStyle(style: EngineTextStyle): TextStyle {
    const props: TextStyle = {
      fontFamilies: families(style.fontFamily, style.fontFamilyFallback),
      fontSize: style.fontSize ?? 14,
      fontStyle: this._fontStyle(style.fontWeight, style.fontStyle),
      letterSpacing: style.letterSpacing ?? 0,
      wordSpacing: style.wordSpacing ?? 0,
      halfLeading: style.leadingDistribution === TextLeadingDistribution.even,
      fontFeatures: (style.fontFeatures ?? []).map((f) => ({ name: f.feature, value: f.value })),
    };
    if (style.height !== undefined) {
      props.heightMultiplier = style.height;
    }
    return props;
  }
}

/** A CanvasKit paragraph; [dispose] deletes it. */
class CanvasKitParagraph implements EngineParagraph {
  private _paragraph: Paragraph | null;
  private readonly _onDispose: (paragraph: Paragraph) => void;

  constructor(paragraph: Paragraph, onDispose: (paragraph: Paragraph) => void) {
    this._paragraph = paragraph;
    this._onDispose = onDispose;
  }

  private get _p(): Paragraph {
    if (this._paragraph === null) {
      throw new Error("paragraph used after dispose");
    }
    return this._paragraph;
  }

  layout(width: number): void {
    this._p.layout(width);
  }

  get width(): number {
    return this._p.getMaxWidth();
  }

  get height(): number {
    return this._p.getHeight();
  }

  get longestLine(): number {
    return this._p.getLongestLine();
  }

  get minIntrinsicWidth(): number {
    return this._p.getMinIntrinsicWidth();
  }

  get maxIntrinsicWidth(): number {
    return this._p.getMaxIntrinsicWidth();
  }

  get alphabeticBaseline(): number {
    return this._p.getAlphabeticBaseline();
  }

  get ideographicBaseline(): number {
    return this._p.getIdeographicBaseline();
  }

  get didExceedMaxLines(): boolean {
    return this._p.didExceedMaxLines();
  }

  lineMetrics(): EngineLineMetrics[] {
    return this._p.getLineMetrics().map((line) => ({
      startIndex: line.startIndex,
      endIndex: line.endIndex,
      hardBreak: line.isHardBreak,
      ascent: line.ascent,
      descent: line.descent,
      height: line.height,
      width: line.width,
      left: line.left,
      baseline: line.baseline,
      lineNumber: line.lineNumber,
    }));
  }

  dispose(): void {
    const paragraph = this._paragraph;
    if (paragraph !== null) {
      this._paragraph = null;
      this._onDispose(paragraph);
    }
  }
}

/** The paragraph's default run style, as the engine derives it from a `ParagraphStyle`. */
function paragraphTextStyle(style: EngineParagraphStyle): EngineTextStyle {
  return defined({
    fontFamily: style.fontFamily,
    fontSize: style.fontSize,
    fontWeight: style.fontWeight,
    fontStyle: style.fontStyle,
    height: style.height,
    leadingDistribution: style.textHeightBehavior?.leadingDistribution,
  });
}

/** `style` without its undefined fields, so spreading it overrides only what it sets. */
function defined(style: EngineTextStyle): EngineTextStyle {
  return Object.fromEntries(Object.entries(style).filter(([, value]) => value !== undefined)) as EngineTextStyle;
}

function families(family: string | undefined, fallback: readonly string[] | undefined): string[] {
  return [...(family === undefined ? [] : [family]), ...(fallback ?? [])];
}

function textAlignName(align: TextAlign): "Left" | "Right" | "Center" | "Justify" | "Start" | "End" {
  switch (align) {
    case TextAlign.left:
      return "Left";
    case TextAlign.right:
      return "Right";
    case TextAlign.center:
      return "Center";
    case TextAlign.justify:
      return "Justify";
    case TextAlign.start:
      return "Start";
    case TextAlign.end:
      return "End";
  }
}
