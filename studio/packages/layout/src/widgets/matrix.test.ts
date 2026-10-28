// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// The full matrix (S4.6, STU-005): every widget descriptor of schema/widgets
// laid out in the phone and tablet frames, in English, in the right-to-left
// locales and in the complex scripts the canvas names, at text scales 1 and 2.
// A screen never throws and every box it places is finite and non-negative;
// the pixel comparison against Flutter is the conformance suite's job.

import { afterAll, beforeAll, describe, expect, test } from "bun:test";
import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { loadCanvasKit, testParagraphEngine } from "../../test-support/canvaskit.ts";
import { TextDirection } from "../flutter/geometry.ts";
import type { CanvasKitParagraphEngine } from "../text/canvaskit-engine.ts";
import { defaultScreenTheme, layoutScreen, type WidgetNode } from "./screen.ts";

let engine: CanvasKitParagraphEngine;
beforeAll(async () => {
  engine = testParagraphEngine(await loadCanvasKit());
});
afterAll(() => {
  engine.dispose();
});

/** The frames of the conformance suite (test/layout-conformance): a phone and a tablet. */
const frames: readonly { name: string; width: number }[] = [
  { name: "phone", width: 360 },
  { name: "tablet", width: 800 },
];

/** A label per locale; the complex scripts are those of the canvas's authoritative-device notice (STU-006). */
const locales: readonly { code: string; rtl: boolean; label: string }[] = [
  { code: "en", rtl: false, label: "Account settings and notifications" },
  { code: "ar", rtl: true, label: "إعدادات الحساب والإشعارات" },
  { code: "he", rtl: true, label: "הגדרות חשבון והתראות" },
  { code: "hi", rtl: false, label: "खाता सेटिंग्स और सूचनाएं" },
  { code: "bn", rtl: false, label: "অ্যাকাউন্ট সেটিংস ও বিজ্ঞপ্তি" },
  { code: "ta", rtl: false, label: "கணக்கு அமைப்புகள் மற்றும் அறிவிப்புகள்" },
  { code: "th", rtl: false, label: "การตั้งค่าบัญชีและการแจ้งเตือน" },
  { code: "km", rtl: false, label: "ការកំណត់គណនី និងការជូនដំណឹង" },
  { code: "my", rtl: false, label: "အကောင့်ဆက်တင်များနှင့် အသိပေးချက်များ" },
];

const scales: readonly number[] = [1, 2];

interface Descriptor {
  readonly name: string;
  readonly slots: readonly { name: string; list?: boolean }[];
}

const schemaRoot = join(import.meta.dir, "../../../../../schema/widgets");
const descriptors: Descriptor[] = ["layer1", "layer2"].flatMap((layer) =>
  readdirSync(join(schemaRoot, layer))
    .filter((file) => file.endsWith(".json"))
    .sort()
    .map((file) => {
      const json = JSON.parse(readFileSync(join(schemaRoot, layer, file), "utf8")) as {
        slots?: { name: string; list?: boolean }[];
      };
      return { name: file.slice(0, -".json".length), slots: json.slots ?? [] };
    }),
);

/** `descriptor` as a screen node whose every slot and child is the locale's label. */
function nodeOf(descriptor: Descriptor, label: string): WidgetNode {
  const text = (id: string): WidgetNode => ({ id, type: "Text", props: { data: label } });
  const slots: Record<string, WidgetNode> = {};
  for (const slot of descriptor.slots) {
    if (slot.list !== true) {
      slots[slot.name] = text(`slot-${slot.name}`);
    }
  }
  // A tab bar holds tabs; every other widget takes any children.
  const tab = (id: string): WidgetNode => ({ id, type: "Tab", props: { text: label } });
  const parts = descriptor.name === "TabBar" ? [tab("c0"), tab("c1")] : [text("c0"), text("c1")];
  return { id: "n", type: descriptor.name, props: {}, slots, children: parts };
}

/** The descriptors that fail to lay out at `width`, in `locale` and at `scale`, each with why. */
function failuresOf(width: number, locale: (typeof locales)[number], scale: number): string[] {
  const theme = { ...defaultScreenTheme, width, fontFamilies: ["NotoSans", "NotoNaskhArabic"] };
  const textDirection = locale.rtl ? TextDirection.rtl : TextDirection.ltr;
  const failures: string[] = [];
  for (const descriptor of descriptors) {
    try {
      const layout = layoutScreen(nodeOf(descriptor, locale.label), { engine, theme, textScale: scale, textDirection });
      const usable = layout.boxes.every((b) => [b.x, b.y, b.w, b.h].every(Number.isFinite) && b.w >= 0 && b.h >= 0);
      if (!usable || !Number.isFinite(layout.contentHeight) || layout.boxes.length === 0) {
        failures.push(`${descriptor.name}: unusable boxes`);
      }
    } catch (error) {
      failures.push(`${descriptor.name}: ${(error as Error).message}`);
    }
  }
  return failures;
}

describe("the full matrix (S4.6, [STU-005])", () => {
  test("covers every descriptor of schema/widgets", () => {
    expect(descriptors.length).toBeGreaterThan(200);
  });

  for (const frame of frames) {
    for (const locale of locales) {
      for (const scale of scales) {
        test(`every descriptor lays out in the ${frame.name} frame, ${locale.code}, text scale ${scale}`, () => {
          expect(failuresOf(frame.width, locale, scale)).toEqual([]);
        });
      }
    }
  }

  test("text scale 2 makes a paragraph taller and the text direction does not change its height", () => {
    const node: WidgetNode = { id: "t", type: "Text", props: { data: "Account settings and notifications" } };
    const theme = { ...defaultScreenTheme, width: 360, fontFamilies: ["NotoSans"] };
    const height = (textScale: number, textDirection: TextDirection): number =>
      layoutScreen(node, { engine, theme, textScale, textDirection }).boxes.find((b) => b.id === "t")?.h ?? 0;
    expect(height(2, TextDirection.ltr)).toBeGreaterThan(height(1, TextDirection.ltr));
    expect(height(1, TextDirection.rtl)).toBeCloseTo(height(1, TextDirection.ltr), 5);
  });

  test("a right-to-left locale starts a row at the right edge", () => {
    const node: WidgetNode = {
      id: "r",
      type: "Row",
      children: [{ id: "a", type: "Text", props: { data: "ا" } }],
    };
    const theme = { ...defaultScreenTheme, width: 360, fontFamilies: ["NotoNaskhArabic"] };
    const rtl = layoutScreen(node, { engine, theme, textDirection: TextDirection.rtl }).boxes.find((b) => b.id === "a");
    const ltr = layoutScreen(node, { engine, theme, textDirection: TextDirection.ltr }).boxes.find((b) => b.id === "a");
    expect(ltr?.x).toBe(16);
    expect((rtl?.x ?? 0) + (rtl?.w ?? 0)).toBeCloseTo(360 - 16, 1);
  });
});
