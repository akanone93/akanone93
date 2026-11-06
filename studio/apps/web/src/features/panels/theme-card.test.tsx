// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, mock, test } from "bun:test";
import { createStudioI18n } from "@plux/i18n";
import { colorSchemeFromSeed } from "@plux/layout";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { I18nProvider } from "../../i18n-provider.tsx";
import { loadSourceCatalogue } from "../../test-harness.tsx";
import { PaintThemeContext } from "./style-editors.tsx";
import { ThemeCard, type ThemeRoundTrip, ThemeRoundTripContext } from "./theme-card.tsx";

afterEach(cleanup);

const H = "#";

function setUp(canEditTheme: boolean) {
  const rt: ThemeRoundTrip = {
    users: mock(() => ["a", "b", "c"]),
    select: mock((_ids: readonly string[]) => undefined),
    canEditTheme,
    setToken: mock((_t: string, _k: "color" | "dimension", _v: number) => undefined),
    addToken: mock((_n: string, _v: unknown) => undefined),
  };
  const bind = mock((_v: unknown, _t: string) => undefined);
  const node = {
    id: "x",
    type: "Container",
    props: {
      color: { $token: "color.primary" },
      padding: { all: 12 },
      decoration: { color: `${H}FF5722FF` },
      width: { $token: "size.card" },
    },
  };
  render(
    <I18nProvider studio={createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false })}>
      <PaintThemeContext.Provider
        value={{
          scheme: colorSchemeFromSeed(0xff6750a4, false),
          tokens: { "color.primary": 0x6750a4ff },
          dimensions: { "space.md": 12, "size.card": 280 },
        }}
      >
        <ThemeRoundTripContext.Provider value={rt}>
          <ThemeCard node={node} canEdit bind={bind} />
        </ThemeRoundTripContext.Provider>
      </PaintThemeContext.Provider>
    </I18nProvider>,
  );
  return { rt, bind };
}

describe("PC-35 the selection's Theme card [THM-001]", () => {
  test("a bound token shows its value and its users; selecting them and editing the value go to the canvas", async () => {
    const { rt } = setUp(true);
    expect(await screen.findByText("color.primary")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Select the widgets that use color.primary" }));
    expect(rt.select).toHaveBeenLastCalledWith(["a", "b", "c"]);
    const colour = screen.getByLabelText("Value of color.primary") as HTMLInputElement;
    expect(colour.value).toBe(`${H}6750a4`);
    fireEvent.change(colour, { target: { value: `${H}112233` } });
    expect(rt.setToken).toHaveBeenLastCalledWith("color.primary", "color", 0x112233ff);
    const px = screen.getByLabelText("Value of size.card") as HTMLInputElement;
    fireEvent.change(px, { target: { value: "300" } });
    fireEvent.blur(px);
    expect(rt.setToken).toHaveBeenLastCalledWith("size.card", "dimension", 300);
  });

  test("a raw value binds to the theme's token of that value, or becomes a new token", async () => {
    const { rt, bind } = setUp(true);
    fireEvent.click(await screen.findByRole("button", { name: "Use space.md" }));
    expect(bind).toHaveBeenLastCalledWith(expect.objectContaining({ path: ["padding", "all"] }), "space.md");
    fireEvent.click(screen.getByRole("button", { name: "Make a token" }));
    const name = screen.getByRole("textbox", { name: "Token name" }) as HTMLInputElement;
    expect(name.value).toBe("color.custom1");
    fireEvent.change(name, { target: { value: "color.accent" } });
    fireEvent.click(screen.getAllByRole("button", { name: "Make a token" }).at(-1) as HTMLElement);
    expect(rt.addToken).toHaveBeenLastCalledWith("color.accent", expect.objectContaining({ raw: 0xff5722ff }));
    expect(bind).toHaveBeenLastCalledWith(expect.objectContaining({ path: ["decoration", "color"] }), "color.accent");
  });

  test("with the theme read-only, tokens show without editors and no token is made", async () => {
    setUp(false);
    expect(await screen.findByText("The theme is being edited elsewhere: its tokens are read-only here.")).toBeTruthy();
    expect((screen.getByLabelText("Value of color.primary") as HTMLInputElement).disabled).toBe(true);
    expect(screen.queryByRole("button", { name: "Make a token" })).toBeNull();
  });
});
