// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, mock, test } from "bun:test";
import { createStudioI18n } from "@plux/i18n";
import { widget } from "@plux/schema";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { I18nProvider } from "../../i18n-provider.tsx";
import { loadSourceCatalogue } from "../../test-harness.tsx";
import { type LocalePreview, LocalePreviewContext, LocaleTools } from "./locale-tools.tsx";

afterEach(cleanup);

const messages: Record<string, string> = { k1: "{n, plural, one {# Tag} other {# Tage}}" };

function setUp(canEdit: boolean) {
  const setSample = mock((_node: string, _n: number | undefined) => undefined);
  const setOverride = mock((_locale: string, _prop: string, _value: unknown) => undefined);
  const preview: LocalePreview = {
    locale: "de",
    defaultLocale: "en",
    messageOf: (k) => messages[k],
    textOf: (k) => (k === "k1" ? "{n} Tage" : undefined),
    missing: (k) => k === "k2",
    sampleOf: () => 2,
    setSample,
  };
  render(
    <I18nProvider studio={createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false })}>
      <LocalePreviewContext.Provider value={preview}>
        <LocaleTools
          id="t"
          node={{
            id: "t",
            type: "Text",
            props: { data: { $t: "k1" }, semanticsLabel: { $t: "k2" } },
            localeOverrides: { de: { maxLines: 2 } },
          }}
          canEdit={canEdit}
          setOverride={setOverride}
        />
      </LocalePreviewContext.Provider>
    </I18nProvider>,
  );
  return { setSample, setOverride };
}

describe("PC-32 the previewed locale's tools [I18N-001] [I18N-006] [I18N-012]", () => {
  test("each text shows its message in the locale, or that the device falls back for a missing one", async () => {
    setUp(true);
    expect(await screen.findByText("{n, plural, one {# Tag} other {# Tage}}")).toBeTruthy();
    expect(screen.getByText("Translated in this locale")).toBeTruthy();
    expect(screen.getByText("k2")).toBeTruthy();
    expect(screen.getByText("No message in de: the device shows the en one")).toBeTruthy();
  });

  test("a plural text's sample is picked from data or a count", async () => {
    const { setSample } = setUp(true);
    const chips = await screen.findAllByRole("radio");
    expect(chips.map((c) => c.textContent)).toEqual(["From data", "0", "1", "2", "5", "21"]);
    expect(chips[3]?.getAttribute("aria-checked")).toBe("true");
    fireEvent.click(chips[5] as HTMLElement);
    expect(setSample).toHaveBeenLastCalledWith("t", 21);
    fireEvent.click(chips[0] as HTMLElement);
    expect(setSample).toHaveBeenLastCalledWith("t", undefined);
  });

  test("the locale's overrides are listed, removed and added, a JSON value parsed and text kept as text", async () => {
    const { setOverride } = setUp(true);
    expect(await screen.findByText("maxLines")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Remove the maxLines override" }));
    expect(setOverride).toHaveBeenLastCalledWith("de", "maxLines", undefined);
    const first = widget("Text")?.props[0]?.name ?? "";
    const value = screen.getByRole("textbox", { name: "Value in this locale" });
    const add = screen.getByRole("button", { name: "Set for de" });
    expect(add.hasAttribute("disabled")).toBe(true);
    fireEvent.change(value, { target: { value: "3" } });
    fireEvent.click(add);
    expect(setOverride).toHaveBeenLastCalledWith("de", first, 3);
    fireEvent.change(value, { target: { value: "Bezahlen" } });
    fireEvent.click(add);
    expect(setOverride).toHaveBeenLastCalledWith("de", first, "Bezahlen");
  });

  test("a read-only selection lists its overrides without editing them", async () => {
    setUp(false);
    expect(await screen.findByText("maxLines")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Remove the maxLines override" })).toBeNull();
    expect(screen.queryByRole("textbox")).toBeNull();
  });
});
