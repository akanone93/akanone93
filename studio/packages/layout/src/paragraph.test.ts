// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterAll, beforeAll, describe, expect, test } from "bun:test";
import type { CanvasKit } from "canvaskit-wasm";
import { loadCanvasKit, testParagraphEngine } from "../test-support/canvaskit.ts";
import {
  BoxConstraints,
  buildTextRuns,
  type CanvasKitParagraphEngine,
  computePaintOffsetFraction,
  FontStyle,
  getParagraphStyle,
  kDefaultFontSize,
  plainText,
  RenderParagraph,
  type RenderParagraphOptions,
  Size,
  TextAlign,
  TextBaseline,
  TextDirection,
  TextLeadingDistribution,
  TextOverflow,
  TextPainter,
  TextScaler,
  type TextSpan,
  TextWidthBasis,
} from "./index.ts";

let canvasKit: CanvasKit;
let engine: CanvasKitParagraphEngine;
const created: RenderParagraph[] = [];

beforeAll(async () => {
  canvasKit = await loadCanvasKit();
  engine = testParagraphEngine(canvasKit);
});

afterAll(() => {
  for (const p of created) {
    p.dispose();
  }
  engine.dispose();
});

const sans = { fontFamily: "NotoSans", fontSize: 14 };
const sentence = "The quick brown fox jumps over the lazy dog";
const arabic = "مرحبا بالعالم هذا نص عربي طويل للتجربة";

function paragraph(text: string | TextSpan, options: Partial<RenderParagraphOptions> = {}): RenderParagraph {
  const p = new RenderParagraph({
    text: typeof text === "string" ? { text, style: sans } : text,
    textDirection: TextDirection.ltr,
    engine,
    ...options,
  });
  created.push(p);
  return p;
}

function laidOut(p: RenderParagraph, constraints: BoxConstraints): RenderParagraph {
  p.layout(constraints, { parentUsesSize: true });
  return p;
}

const loose = (width: number): BoxConstraints => new BoxConstraints({ maxWidth: width, maxHeight: 1000 });

describe("[STU-005] RenderParagraph wraps, truncates and sizes text as Flutter", () => {
  test("wraps at the max width, breaking only between words", () => {
    const p = laidOut(paragraph(sentence), loose(120));
    const lines = p.textPainter.computeLineMetrics();
    expect(lines.length).toBeGreaterThan(2);
    for (const line of lines) {
      expect(line.width).toBeLessThanOrEqual(120);
      expect(sentence[line.endIndex - 1] === " " || line.endIndex === sentence.length).toBe(true);
    }
    expect(p.size.width).toBe(120);
    expect(p.size.height).toBeCloseTo(
      lines.reduce((sum, l) => sum + l.height, 0),
      3,
    );
    expect(p.needsClipping).toBe(false);
  });

  test("maxLines with ellipsis stops at the line limit and clips", () => {
    const p = laidOut(paragraph(sentence, { maxLines: 2, overflow: TextOverflow.ellipsis }), loose(100));
    const lines = p.textPainter.computeLineMetrics();
    expect(lines.length).toBe(2);
    expect(p.textPainter.didExceedMaxLines).toBe(true);
    expect(p.needsClipping).toBe(true);
    expect(p.hasOverflowShader).toBe(false);
    const all = laidOut(paragraph(sentence), loose(100));
    expect(p.size.height).toBeLessThan(all.size.height);
  });

  test("softWrap false keeps one line; ellipsis still truncates at the width", () => {
    const noWrap = laidOut(paragraph(sentence, { softWrap: false }), loose(100));
    expect(noWrap.textPainter.computeLineMetrics().length).toBe(1);
    expect(noWrap.size.width).toBe(100);
    expect(noWrap.needsClipping).toBe(true);
    const ellipsis = laidOut(
      paragraph(sentence, { softWrap: false, maxLines: 1, overflow: TextOverflow.ellipsis }),
      loose(100),
    );
    expect(ellipsis.textPainter.computeLineMetrics()[0]?.width).toBeLessThanOrEqual(100);
    const fade = laidOut(paragraph(sentence, { softWrap: false, overflow: TextOverflow.fade }), loose(100));
    expect(fade.hasOverflowShader).toBe(true);
    const visible = laidOut(paragraph(sentence, { softWrap: false, overflow: TextOverflow.visible }), loose(100));
    expect(visible.needsClipping).toBe(false);
  });

  test("height multiplies the line height; even leading keeps the line height", () => {
    const base = laidOut(paragraph("Hg"), loose(300)).size.height;
    const tall = laidOut(paragraph({ text: "Hg", style: { ...sans, height: 2 } }), loose(300));
    expect(tall.size.height).toBeCloseTo(28, 3);
    expect(base).not.toBeCloseTo(28, 1);
    const even = laidOut(
      paragraph({ text: "Hg", style: { ...sans, height: 2, leadingDistribution: TextLeadingDistribution.even } }),
      loose(300),
    );
    expect(even.size.height).toBeCloseTo(28, 3);
    const trimmed = laidOut(
      paragraph(
        { text: "Hg", style: { ...sans, height: 2 } },
        { textHeightBehavior: { applyHeightToFirstAscent: false, applyHeightToLastDescent: false } },
      ),
      loose(300),
    );
    expect(trimmed.size.height).toBeLessThan(tall.size.height);
    for (const [first, last] of [
      [false, true],
      [true, false],
    ] as const) {
      const partial = laidOut(
        paragraph(
          { text: "Hg", style: { ...sans, height: 2 } },
          { textHeightBehavior: { applyHeightToFirstAscent: first, applyHeightToLastDescent: last } },
        ),
        loose(300),
      );
      expect(partial.size.height).toBeLessThan(tall.size.height);
      expect(partial.size.height).toBeGreaterThan(trimmed.size.height);
    }
  });

  test("a forced strut sets every line's height", () => {
    const strut = { fontFamily: "NotoSans", fontSize: 10, height: 3, leading: 0, forceStrutHeight: true };
    const p = laidOut(paragraph(sentence, { strutStyle: strut }), loose(100));
    const lines = p.textPainter.computeLineMetrics();
    for (const line of lines) {
      expect(line.height).toBeCloseTo(30, 3);
    }
    const even = laidOut(
      paragraph("Hg", {
        strutStyle: {
          fontSize: 10,
          height: 3,
          leadingDistribution: TextLeadingDistribution.even,
          forceStrutHeight: true,
        },
      }),
      loose(300),
    );
    expect(even.size.height).toBeCloseTo(30, 3);
  });

  test("text scaler 2.0 doubles font sizes, strut included", () => {
    const one = laidOut(paragraph("Hello"), loose(1000));
    const two = laidOut(paragraph("Hello", { textScaler: TextScaler.linear(2) }), loose(1000));
    expect(two.size.width).toBeCloseTo(one.size.width * 2, 1);
    expect(two.size.height).toBeCloseTo(one.size.height * 2, 0);
    const strut = laidOut(
      paragraph("Hello", {
        textScaler: TextScaler.linear(2),
        strutStyle: { fontSize: 20, height: 1, forceStrutHeight: true },
      }),
      loose(1000),
    );
    expect(strut.size.height).toBeCloseTo(40, 3);
    expect(TextScaler.linear(1)).toBe(TextScaler.noScaling);
  });

  test("textAlign × direction: the paint offset follows Flutter's fraction", () => {
    const cases: Array<[TextAlign, TextDirection, number]> = [
      [TextAlign.left, TextDirection.rtl, 0],
      [TextAlign.right, TextDirection.ltr, 1],
      [TextAlign.center, TextDirection.ltr, 0.5],
      [TextAlign.start, TextDirection.ltr, 0],
      [TextAlign.start, TextDirection.rtl, 1],
      [TextAlign.justify, TextDirection.rtl, 1],
      [TextAlign.end, TextDirection.ltr, 1],
      [TextAlign.end, TextDirection.rtl, 0],
    ];
    for (const [align, direction, fraction] of cases) {
      expect(computePaintOffsetFraction(align, direction)).toBe(fraction);
      const p = laidOut(paragraph("Hello", { textAlign: align, textDirection: direction }), loose(300));
      expect(p.size.width).toBeCloseTo(p.getMaxIntrinsicWidth(Number.POSITIVE_INFINITY), 3);
      const tight = laidOut(
        paragraph("Hello", { textAlign: align, textDirection: direction }),
        BoxConstraints.tightFor({ width: 300 }),
      );
      expect(tight.size.width).toBe(300);
    }
    // Aligned text in unbounded width is laid out at its unwrapped width.
    const unbounded = laidOut(
      paragraph(sentence, { textAlign: TextAlign.center }),
      new BoxConstraints({ maxWidth: Number.POSITIVE_INFINITY }),
    );
    expect(unbounded.size.width).toBeCloseTo(unbounded.getMaxIntrinsicWidth(Number.POSITIVE_INFINITY), 3);
    expect(unbounded.textPainter.paintOffsetDx).toBeCloseTo(0, 3);
    const justified = laidOut(paragraph(sentence, { textAlign: TextAlign.justify }), loose(150));
    const lines = justified.textPainter.computeLineMetrics();
    expect(lines[0]?.width).toBeGreaterThan(149);
  });

  test("intrinsics: min is the longest word, max the unwrapped line, heights follow the width", () => {
    const p = paragraph(sentence);
    const word = Math.max(...sentence.split(" ").map((w) => laidOut(paragraph(w), loose(1000)).size.width));
    expect(p.getMinIntrinsicWidth(Number.POSITIVE_INFINITY)).toBeCloseTo(word, 1);
    const line = laidOut(paragraph(sentence), loose(10000)).size.width;
    expect(p.getMaxIntrinsicWidth(Number.POSITIVE_INFINITY)).toBeCloseTo(line, 3);
    const oneLine = p.getMinIntrinsicHeight(10000);
    expect(p.getMaxIntrinsicHeight(100)).toBeGreaterThan(oneLine * 2);
    const noWrap = paragraph(sentence, { softWrap: false });
    expect(noWrap.getMinIntrinsicHeight(100)).toBeCloseTo(oneLine, 3);
  });

  test("dry layout equals layout without disturbing the laid-out paragraph", () => {
    const p = laidOut(paragraph(sentence), loose(120));
    const size = p.size;
    const dry = p.getDryLayout(loose(120));
    expect(dry.equals(size)).toBe(true);
    expect(p.getDryLayout(BoxConstraints.tight(new Size(50, 50))).equals(new Size(50, 50))).toBe(true);
    expect(p.getDryBaseline(loose(120), TextBaseline.alphabetic)).toBeCloseTo(
      p.getDistanceToActualBaseline(TextBaseline.alphabetic) ?? 0,
      3,
    );
    expect(p.textPainter.computeLineMetrics().length).toBeGreaterThan(2);
  });

  test("baselines: alphabetic from the first line; ideographic below it", () => {
    const p = laidOut(paragraph("Hg"), loose(300));
    const alphabetic = p.getDistanceToActualBaseline(TextBaseline.alphabetic) ?? 0;
    expect(alphabetic).toBeGreaterThan(10);
    expect(alphabetic).toBeLessThan(p.size.height);
    // Flutter 3.47's RenderParagraph answers the alphabetic baseline for both.
    expect(p.getDistanceToActualBaseline(TextBaseline.ideographic)).toBeCloseTo(alphabetic, 6);
    expect(p.textPainter.computeDistanceToActualBaseline(TextBaseline.ideographic)).toBeGreaterThan(alphabetic);
  });

  test("textWidthBasis longestLine shrinks the box to the longest line", () => {
    const parent = laidOut(paragraph(sentence), loose(150));
    const longest = laidOut(paragraph(sentence, { textWidthBasis: TextWidthBasis.longestLine }), loose(150));
    const widest = Math.max(...longest.textPainter.computeLineMetrics().map((l) => l.width));
    expect(parent.size.width).toBe(150);
    expect(longest.size.width).toBeLessThan(150);
    expect(longest.size.width).toBeCloseTo(widest, 3);
  });

  test("relayout reuses line breaks when the width does not change them", () => {
    const p = laidOut(paragraph("Hi"), loose(300));
    const first = p.size;
    laidOut(p, loose(400));
    expect(p.size.equals(first)).toBe(true);
    laidOut(p, BoxConstraints.tight(p.size));
    expect(p.size.equals(first)).toBe(true);
    laidOut(p, loose(5));
    expect(p.size.width).toBe(5);
  });

  test("setters relayout only on change", () => {
    const p = laidOut(paragraph(sentence), loose(150));
    const height = p.size.height;
    const same = p.text;
    p.text = same;
    p.textAlign = TextAlign.start;
    p.textDirection = TextDirection.ltr;
    p.textScaler = TextScaler.linear(1);
    p.maxLines = null;
    p.textWidthBasis = TextWidthBasis.parent;
    p.softWrap = true;
    p.overflow = TextOverflow.clip;
    expect(p.needsLayout).toBe(false);
    p.maxLines = 1;
    p.overflow = TextOverflow.ellipsis;
    p.softWrap = false;
    p.textScaler = TextScaler.linear(1.5);
    p.textWidthBasis = TextWidthBasis.longestLine;
    p.textAlign = TextAlign.end;
    p.textDirection = TextDirection.rtl;
    p.text = { text: "Short", style: sans };
    expect(p.needsLayout).toBe(true);
    laidOut(p, loose(150));
    expect(p.size.height).toBeLessThan(height);
    expect([p.maxLines, p.overflow, p.softWrap, p.textScaler.textScaleFactor]).toEqual([
      1,
      TextOverflow.ellipsis,
      false,
      1.5,
    ]);
    expect([p.textWidthBasis, p.textAlign, p.textDirection, plainText(p.text)]).toEqual([
      TextWidthBasis.longestLine,
      TextAlign.end,
      TextDirection.rtl,
      "Short",
    ]);
  });
});

describe("[STU-006] SkParagraph through CanvasKit, as on the device", () => {
  test("an Arabic RTL paragraph shapes, wraps and aligns to the right", () => {
    const p = laidOut(
      paragraph(
        { text: arabic, style: { fontFamily: "NotoNaskhArabic", fontSize: 16 } },
        { textDirection: TextDirection.rtl },
      ),
      loose(150),
    );
    const lines = p.textPainter.computeLineMetrics();
    expect(lines.length).toBeGreaterThan(1);
    for (const line of lines) {
      expect(line.left + line.width).toBeCloseTo(150, 0);
    }
    expect(engine.liveObjectCount).toBeGreaterThan(2);
  });

  test("span trees inherit styles; weight, italic, spacing and features apply", () => {
    const span: TextSpan = {
      style: { ...sans, fontStyle: FontStyle.italic },
      children: [
        { text: "Bold ", style: { fontWeight: 700, letterSpacing: 1 } },
        { text: "fi", style: { wordSpacing: 2, fontFeatures: [{ feature: "liga", value: 0 }] } },
        { text: " big", style: { fontSize: 28 } },
      ],
    };
    const runs = buildTextRuns(span, TextScaler.linear(2));
    expect(runs.map((r) => r.style?.fontSize)).toEqual([28, 28, 56]);
    expect(runs[0]?.style?.fontStyle).toBe(FontStyle.italic);
    expect(plainText(span)).toBe("Bold fi big");
    const p = laidOut(paragraph(span), loose(1000));
    expect(p.size.height).toBeGreaterThan(30);
    expect(getParagraphStyle({}, { textDirection: TextDirection.ltr }).fontSize).toBe(kDefaultFontSize);
    expect(
      getParagraphStyle({ leadingDistribution: TextLeadingDistribution.even }, { textDirection: TextDirection.ltr })
        .textHeightBehavior,
    ).toEqual({ leadingDistribution: TextLeadingDistribution.even });
    const thin = laidOut(paragraph({ text: "Light", style: { ...sans, fontWeight: 100 } }), loose(1000));
    expect(thin.size.width).toBeGreaterThan(0);
  });

  test("a text painter needs text, a direction and a layout before metrics", () => {
    const painter = new TextPainter({ engine });
    expect(() => painter.width).toThrow(/layout must be called/);
    expect(() => painter.layout()).toThrow(/text must be set/);
    painter.text = { text: "x", style: sans };
    expect(() => painter.layout()).toThrow(/textDirection must be set/);
    painter.textDirection = TextDirection.ltr;
    painter.layout();
    expect(painter.width).toBeGreaterThan(0);
    painter.dispose();
  });

  test("no CanvasKit object is leaked", () => {
    const local = testParagraphEngine(canvasKit);
    expect(local.liveObjectCount).toBe(2);
    const p = new RenderParagraph({
      text: { text: sentence, style: sans },
      textDirection: TextDirection.ltr,
      engine: local,
    });
    laidOut(p, loose(100));
    p.getMinIntrinsicWidth(Number.POSITIVE_INFINITY);
    p.text = { text: "changed", style: sans };
    laidOut(p, loose(100));
    expect(local.liveObjectCount).toBe(4);
    p.dispose();
    p.dispose();
    expect(local.liveObjectCount).toBe(2);
    const built = local.buildParagraph({ textDirection: TextDirection.ltr }, [{ text: "x", style: null }]);
    built.dispose();
    expect(() => built.layout(10)).toThrow(/after dispose/);
    local.dispose();
    local.dispose();
    expect(local.liveObjectCount).toBe(0);
    expect(() => local.registerFont(new Uint8Array(), "x")).toThrow(/after dispose/);
  });
});
