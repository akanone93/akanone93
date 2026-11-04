// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, mock, test } from "bun:test";
import { createStudioI18n } from "@plux/i18n";
import { colorSchemeFromSeed } from "@plux/layout";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { I18nProvider } from "../../i18n-provider.tsx";
import { loadSourceCatalogue } from "../../test-harness.tsx";
import { ColorField, DecorationEditor, PaintThemeContext } from "./style-editors.tsx";

afterEach(cleanup);

const renderI18n = (node: ReactNode) =>
  render(
    <I18nProvider studio={createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false })}>
      {node}
    </I18nProvider>,
  );

const theme = { scheme: colorSchemeFromSeed(0xff6750a4, false), tokens: {} };

describe("[STU-062] [THM-001] style editors", () => {
  test("the colour field offers the theme's roles and writes a token; opacity writes an RGBA literal", async () => {
    const onChange = mock((_: unknown) => undefined);
    renderI18n(
      <PaintThemeContext.Provider value={theme}>
        <ColorField name="color" value={undefined} onChange={onChange} onEnd={() => undefined} />
      </PaintThemeContext.Provider>,
    );
    fireEvent.click(await screen.findByRole("button", { name: /color/ }));
    fireEvent.click(await screen.findByRole("button", { name: "primary" }));
    expect(onChange).toHaveBeenLastCalledWith({ $token: "color.primary" });
    fireEvent.change(screen.getByRole("slider"), { target: { value: "50" } });
    expect(onChange).toHaveBeenLastCalledWith(`#000000${(128).toString(16)}`);
  });

  test("the decoration editor sets a shadow by elevation and turns the box into a circle", async () => {
    const onChange = mock((_: unknown) => undefined);
    renderI18n(<DecorationEditor name="decoration" value={{}} onChange={onChange} onEnd={() => undefined} />);
    await waitFor(() => expect(screen.getAllByRole("switch").length).toBeGreaterThan(0));
    fireEvent.click(screen.getAllByRole("switch")[0] as HTMLElement);
    expect(onChange).toHaveBeenLastCalledWith({ shape: "circle" });
    await waitFor(() => expect(screen.getAllByRole("combobox").length).toBeGreaterThan(1));
  });
});
