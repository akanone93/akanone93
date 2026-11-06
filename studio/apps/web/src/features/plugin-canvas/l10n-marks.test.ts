// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { defaultPreview, previewRender } from "../preview/switches.ts";
import { l10nMarks, missingKeys, multilingual, overflowKeyOf } from "./l10n-marks.ts";
import { localeChain, localizeTree, messagesFor } from "./look.ts";
import type { Node } from "./plugin-model.ts";

const doc = (parsed: { readonly kind: string; readonly [k: string]: unknown }) => ({
  kind: parsed.kind,
  path: "x.json",
  content: new Uint8Array(),
  parsed,
});
const documents = [
  doc({ kind: "app", defaultLocale: "en", supportedLocales: ["en", "de", "de-AT", "ar"] }),
  doc({
    kind: "translationKeys",
    keys: [
      { id: "k1", key: "a" },
      { id: "k2", key: "b" },
      { id: "k3", key: "c" },
    ],
  }),
  doc({ kind: "translations", locale: "en", messages: { k1: "Pay", k2: "{n, plural, one {# day} other {# days}}" } }),
  doc({ kind: "translations", locale: "de", messages: { k1: "Zahlen", k2: "" } }),
  doc({ kind: "translations", locale: "de-AT", messages: { k2: "{n, plural, one {# Tag} other {# Tage}}" } }),
];

describe("PC-32 missing and untranslated texts [I18N-006] [I18N-007]", () => {
  test("a message is looked up along the chain: the exact locale, its language, the default", () => {
    expect(localeChain("de-AT", "en")).toEqual(["de-AT", "de", "en"]);
    expect(localeChain("en", "en")).toEqual(["en"]);
    const look = (locale: string) => previewRender({ ...defaultPreview, locale }, "material");
    expect(messagesFor(documents, look("de-AT"))).toEqual({
      k1: "Zahlen",
      k2: "{n, plural, one {# Tag} other {# Tage}}",
    });
    expect(messagesFor(documents, look("de"))["k2"]).toBe("");
  });

  test("a key is missing where neither the locale nor its language has a message, empty ones included", () => {
    expect([...missingKeys(documents, "de")].sort()).toEqual(["k2", "k3"]);
    expect([...missingKeys(documents, "de-AT")].sort()).toEqual(["k3"]);
    expect([...missingKeys(documents, "en")]).toEqual(["k3"]);
    expect([...missingKeys(documents, "en-XA")]).toEqual(["k3"]);
    expect(multilingual(documents)).toBe(true);
    expect(multilingual([doc({ kind: "app", defaultLocale: "en", supportedLocales: ["en"] })])).toBe(false);
  });

  test("marks missing keys over literals, literals only in a multilingual app, and overflow over both", () => {
    const root = {
      id: "col",
      type: "Column",
      children: [
        { id: "a", type: "Text", props: { data: { $t: "k3" } } },
        { id: "b", type: "Text", props: { data: "Written in the page" } },
        { id: "c", type: "Text", props: { data: { $t: "k1" } } },
        { id: "d", type: "FilledButton", props: { label: "Go" } },
      ],
    };
    const missing = missingKeys(documents, "de");
    expect([...l10nMarks([root], missing, true, [])]).toEqual([
      ["a", "missing"],
      ["b", "literal"],
      ["d", "literal"],
    ]);
    expect([...l10nMarks([root], missing, false, ["c", "b~item1"])]).toEqual([
      ["a", "missing"],
      ["c", "overflow"],
      ["b~item1", "overflow"],
    ]);
  });

  test("the layout's overflowing widgets become one stable key", () => {
    expect(overflowKeyOf([{ overflowing: ["b", "a"] }, {}, { overflowing: ["a"] }])).toBe("a\nb");
    expect(overflowKeyOf([{}])).toBe("");
  });
});

describe("PC-32 texts drawn in the previewed locale [I18N-001] [I18N-012]", () => {
  const messages = { k2: "{n, plural, one {# day} other {# days}}", k4: "Hi {name}, {n, number}" };
  const tree = (props: Record<string, unknown>, extra: Record<string, unknown> = {}) =>
    ({ id: "t", type: "Text", props, ...extra }) as unknown as Node;
  const textOf = (n: Node | undefined) => (n as unknown as { props: { data: string } }).props.data;

  test("a plural message is drawn with its argument's value, or the sample picked for the node", () => {
    const node = tree({ data: { $t: "k2", args: { n: { $expr: "count" } } } });
    const arg = (v: unknown) => (typeof v === "object" ? 3 : v);
    expect(textOf(localizeTree(node, messages, (s) => s, { locale: "en", arg }))).toBe("3 days");
    const samples = new Map([["t", 1]]);
    expect(textOf(localizeTree(node, messages, (s) => s, { locale: "en", arg, samples }))).toBe("1 day");
    expect(textOf(localizeTree(node, messages, (s) => s))).toBe("{n, plural, one {# day} other {# days}}");
  });

  test("literal arguments are filled, unknown ones keep their name", () => {
    const node = tree({ data: { $t: "k4", args: { name: "Ada" } } });
    expect(textOf(localizeTree(node, messages, (s) => s, { locale: "de-DE", arg: (v) => v }))).toBe("Hi Ada, {n}");
  });

  test("a node's overrides for the locale replace its props: the language's, then the exact locale's", () => {
    const node = tree(
      { data: "Pay", maxLines: 1 },
      { localeOverrides: { de: { data: "Bezahlen", maxLines: 2 }, "de-AT": { data: "Zahlen" } } },
    );
    const props = (locale: string) =>
      (localizeTree(node, {}, (s) => s, { locale }) as unknown as { props: Record<string, unknown> }).props;
    expect(props("de-AT")).toEqual({ data: "Zahlen", maxLines: 2 });
    expect(props("de-DE")).toEqual({ data: "Bezahlen", maxLines: 2 });
    expect(props("en")).toEqual({ data: "Pay", maxLines: 1 });
  });
});
