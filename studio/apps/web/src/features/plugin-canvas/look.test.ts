// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterAll, beforeAll, describe, expect, test } from "bun:test";
import { layoutScreen, type ScreenLayout, type WidgetNode } from "@plux/layout";
import type { CanvasKitParagraphEngine } from "../../../../../packages/layout/src/text/canvaskit-engine.ts";
import { loadCanvasKit, testParagraphEngine } from "../../../../../packages/layout/test-support/canvaskit.ts";
import { defaultPreview, previewRender } from "../preview/switches.ts";
import { dataSourcesOf, localizeTree, lookedStyle, messagesFor, resolveTheme } from "./look.ts";
import type { Node } from "./plugin-model.ts";
import { screenStyle } from "./scene.ts";

let engine: CanvasKitParagraphEngine;
beforeAll(async () => {
  engine = testParagraphEngine(await loadCanvasKit());
});
afterAll(() => engine.dispose());

const doc = (value: unknown) => ({
  kind: String((value as { kind: string }).kind),
  content: new TextEncoder().encode(JSON.stringify(value)),
});
const theme = { kind: "theme", id: "t1", key: "t", name: "T", tokens: { fontSize: { $value: 24 } } };
const documents = [
  doc({ kind: "app", id: "a", defaultLocale: "en", supportedLocales: ["en", "de"], theme: "t1" }),
  doc(theme),
  doc({ kind: "translations", id: "e", locale: "en", messages: { greet: "Hello" } }),
  doc({ kind: "translations", id: "d", locale: "de", messages: { greet: "Hallo Welt" } }),
  doc({ kind: "plugin", id: "p", dataSources: [{ id: "orders", name: "Orders", mock: [1] }] }),
];

const tree = (text: unknown): Node => ({
  id: "row",
  type: "Row",
  children: [
    { id: "a", type: "Text", props: { text } as never },
    { id: "b", type: "Text", props: { text: "B" } as never },
  ],
});

/** The screen under the switches `patch`, text translated as the canvas does. */
function laidOut(patch: Partial<typeof defaultPreview>): ScreenLayout {
  const look = previewRender({ ...defaultPreview, ...patch }, "material");
  const style = lookedStyle(screenStyle, resolveTheme(documents, look), look);
  const root = localizeTree(tree({ $t: "greet" }), messagesFor(documents, look), look.localize);
  return layoutScreen(root as unknown as WidgetNode, {
    engine,
    theme: { ...style.theme, fontFamilies: ["NotoSans", "NotoNaskhArabic"] },
    ...(style.textScale === undefined ? {} : { textScale: style.textScale }),
    ...(style.textDirection === undefined ? {} : { textDirection: style.textDirection }),
  });
}

const box = (l: ScreenLayout, id: string) => {
  const found = l.boxes.find((b) => b.id === id);
  if (found === undefined) {
    throw new Error(`no box ${id}`);
  }
  return found;
};

describe("the preview switches reach the canvas [STU-058]", () => {
  test("[A11Y-005]: text scale 2 grows a Text box", () => {
    expect(box(laidOut({ textScale: 2 }), "a").h).toBeGreaterThan(box(laidOut({}), "a").h);
  });

  test("[I18N-010]: RTL mirrors x", () => {
    const ltr = laidOut({});
    const rtl = laidOut({ locale: "ar" });
    expect(box(ltr, "a").x).toBeLessThan(box(ltr, "b").x);
    expect(box(rtl, "a").x).toBeGreaterThan(box(rtl, "b").x);
  });

  test("[I18N-010]: the locale translates, and the expanded pseudo-locale widens", () => {
    const en = box(laidOut({}), "a").w;
    expect(box(laidOut({ locale: "de" }), "a").w).toBeGreaterThan(en);
    expect(box(laidOut({ locale: "en-XA" }), "a").w).toBeGreaterThan(en);
  });

  test("the app theme document sets the layout theme, the overlay wins over it", () => {
    const look = previewRender(defaultPreview, "material");
    expect(resolveTheme(documents, look).fontSize).toBe(24);
    const overlaid = [
      ...documents,
      doc({ kind: "theme", id: "o", key: "o", name: "O", tokens: { fontSize: { $value: 30 } } }),
    ];
    expect(resolveTheme(overlaid, { ...look, brandOverlay: "o" }).fontSize).toBe(30);
    expect(resolveTheme([], look).fontSize).toBe(16);
  });

  test("Cupertino takes its bar heights", () => {
    expect(resolveTheme(documents, previewRender(defaultPreview, "cupertino")).appBarHeight).toBe(44);
  });

  test("[DAT-080]: the data sources of the plugin are listed with their mock", () => {
    expect(dataSourcesOf(documents)).toEqual([{ id: "orders", name: "Orders", mock: [1] }]);
  });
});
