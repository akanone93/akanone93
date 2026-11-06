// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import type { Node } from "../plugin-canvas/plugin-model.ts";
import { nodePreview } from "./selection-model.ts";

const textOf = (key: string) => (key === "k1" ? "{n} Artikel" : undefined);

describe("PC-32 a layer row shows a translated text's message [I18N-007]", () => {
  test("a Text and a button's label show the message in the previewed locale, the key when it has none", () => {
    const text = { id: "t", type: "Text", props: { data: { $t: "k1" } } } as Node;
    expect(nodePreview(text, textOf)).toBe("{n} Artikel");
    expect(nodePreview({ id: "u", type: "Text", props: { data: { $t: "k9" } } } as Node, textOf)).toBe("k9");
    expect(nodePreview(text)).toBe("k1");
    const button = { id: "b", type: "FilledButton", slots: { child: text } } as unknown as Node;
    expect(nodePreview(button, textOf)).toBe("{n} Artikel");
  });

  test("a long text is cut to 48 characters", () => {
    const long = { id: "t", type: "Text", props: { data: "x".repeat(60) } } as Node;
    expect(nodePreview(long)).toBe(`${"x".repeat(47)}…`);
  });
});
