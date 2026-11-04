// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, mock, test } from "bun:test";
import { createStudioI18n } from "@plux/i18n";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { I18nProvider } from "../../i18n-provider.tsx";
import { loadSourceCatalogue } from "../../test-harness.tsx";
import { IconField, iconValue, matchIcons } from "./icon-field.tsx";

afterEach(cleanup);

const renderI18n = (node: ReactNode) =>
  render(
    <I18nProvider studio={createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false })}>
      {node}
    </I18nProvider>,
  );

describe("[THM-005] the icon picker", () => {
  test("a search matches every word, names starting with it first, then shorter names", () => {
    const names = { arrow_back: 1, arrow_back_ios: 2, keyboard_arrow_left: 3, home: 4 };
    expect(matchIcons(names, "arrow back")).toEqual(["arrow_back", "arrow_back_ios"]);
    expect(matchIcons(names, "arrow")).toEqual(["arrow_back", "arrow_back_ios", "keyboard_arrow_left"]);
    expect(matchIcons(names, "")).toEqual(["home", "arrow_back", "arrow_back_ios", "keyboard_arrow_left"]);
    expect(iconValue({ name: "heart", set: "cupertino" })).toEqual({ name: "heart", set: "cupertino" });
    expect(iconValue({ name: "home" })).toEqual({ name: "home", set: "material" });
    expect(iconValue("home")).toBeUndefined();
  });

  test("searching and picking an icon writes its IconData; the set switch picks Cupertino", async () => {
    const onChange = mock((_: unknown) => undefined);
    renderI18n(<IconField name="icon" value={{ name: "home" }} onChange={onChange} onEnd={() => undefined} />);
    fireEvent.click(await screen.findByRole("button", { name: /icon: home/ }));
    fireEvent.change(await screen.findByRole("textbox"), { target: { value: "rocket launch" } });
    fireEvent.click(await screen.findByRole("button", { name: "rocket_launch" }));
    expect(onChange).toHaveBeenLastCalledWith({ name: "rocket_launch", set: "material" });
    // The second set of the switch: Cupertino (the test catalogue does not render select messages).
    const sets = document.querySelectorAll<HTMLButtonElement>(".icon-set");
    expect(sets[0]?.getAttribute("aria-pressed")).toBe("true");
    fireEvent.click(sets[1] as HTMLButtonElement);
    fireEvent.change(screen.getByRole("textbox"), { target: { value: "heart" } });
    fireEvent.click(await screen.findByRole("button", { name: "heart" }));
    expect(onChange).toHaveBeenLastCalledWith({ name: "heart", set: "cupertino" });
  });
});
