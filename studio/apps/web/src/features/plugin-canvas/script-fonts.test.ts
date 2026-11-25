// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { act, renderHook } from "@testing-library/react";
import { type ScriptFont, scriptFontOf, scriptFontSources, scriptFontsIn, useFontLoader } from "./script-fonts.ts";

const families = (sources: readonly { family: string }[]) => sources.map((s) => s.family);

describe("[STU-006] the faces the canvas loads only for texts that need them (build log rows 116, 126)", () => {
  test("Amharic needs Noto Sans Ethiopic; an arrow or a circled number Noto Sans Symbols; Latin neither", () => {
    expect(scriptFontOf("አ")).toBe("ethiopic");
    expect(scriptFontOf("→")).toBe("symbols");
    expect(scriptFontOf("①")).toBe("symbols");
    // Noto Sans and Roboto draw these.
    for (const ch of ["a", "é", "€", "•", "−", "…"]) {
      expect(scriptFontOf(ch)).toBeUndefined();
    }
  });

  test("the strings anywhere in documents and translations name the faces they need", () => {
    const page = { kind: "page", root: { type: "Text", props: { data: "EUR → USD today" } } };
    const translations = { kind: "translations", locale: "am", messages: { hello: "ሰላም" } };
    expect([...scriptFontsIn([page, translations])].sort()).toEqual(["ethiopic", "symbols"]);
    expect(scriptFontsIn({ title: "Plain Latin text", n: 3 }).size).toBe(0);
  });

  test("each face is its files, Ethiopic in both weights", () => {
    const all = new Set<ScriptFont>(["ethiopic", "symbols"]);
    expect(families(scriptFontSources(all))).toEqual(["Noto Sans Ethiopic", "Noto Sans Ethiopic", "Noto Sans Symbols"]);
    expect(scriptFontSources(new Set())).toEqual([]);
  });
});

describe("[STU-006] the loader of those faces", () => {
  test("waits for the documents, then answers with the faces they need; the same loader while that holds", async () => {
    const { result, rerender } = renderHook(({ current }) => useFontLoader(current), {
      initialProps: { current: null as ReadonlySet<ScriptFont> | null },
    });
    const first = result.current;
    const answer = first();
    await act(async () => rerender({ current: new Set<ScriptFont>(["ethiopic"]) }));
    expect(families(await answer)).toEqual(["Noto Sans Ethiopic", "Noto Sans Ethiopic"]);
    expect(result.current).toBe(first);
    // Fewer faces needed: the running canvas keeps what it has.
    await act(async () => rerender({ current: new Set<ScriptFont>() }));
    expect(result.current).toBe(first);
  });

  test("a face needed beyond what the canvas loaded makes a new loader, which answers at once", async () => {
    const { result, rerender } = renderHook(({ current }) => useFontLoader(current), {
      initialProps: { current: new Set<ScriptFont>() as ReadonlySet<ScriptFont> | null },
    });
    const first = result.current;
    expect(await first()).toEqual([]);
    await act(async () => rerender({ current: new Set<ScriptFont>(["symbols"]) }));
    expect(result.current).not.toBe(first);
    expect(families(await result.current())).toEqual(["Noto Sans Symbols"]);
  });

  test("documents that do not come in time open the canvas without the faces, and later ones bring them", async () => {
    const { result, rerender } = renderHook(({ current }) => useFontLoader(current, 5), {
      initialProps: { current: null as ReadonlySet<ScriptFont> | null },
    });
    const first = result.current;
    expect(await first()).toEqual([]);
    await act(async () => rerender({ current: new Set<ScriptFont>(["symbols"]) }));
    expect(result.current).not.toBe(first);
  });
});
