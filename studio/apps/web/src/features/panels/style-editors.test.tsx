// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, mock, test } from "bun:test";
import { createStudioI18n } from "@plux/i18n";
import { colorSchemeFromSeed } from "@plux/layout";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { I18nProvider } from "../../i18n-provider.tsx";
import { loadSourceCatalogue } from "../../test-harness.tsx";
import {
  ColorField,
  DecorationEditor,
  PaintThemeContext,
  RuntimePinContext,
  TextStyleEditor,
} from "./style-editors.tsx";
import { baseRuntime, canExtend } from "./text-style-base.ts";

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

  test("[STU-006] [THM-004] the text style editor shows a role's metrics and writes a TextStyle", async () => {
    const onChange = mock((_: unknown) => undefined);
    renderI18n(
      <TextStyleEditor
        name="style"
        value={{ $token: "typography.titleLarge" }}
        onChange={onChange}
        onEnd={() => undefined}
      />,
    );
    const size = (await screen.findAllByRole("spinbutton"))[0] as HTMLInputElement;
    expect(size.value).toBe("22");
    fireEvent.click(screen.getAllByRole("switch")[0] as HTMLElement);
    expect(onChange).toHaveBeenLastCalledWith({
      fontSize: 22,
      fontWeight: "w400",
      height: 1.27,
      letterSpacing: 0,
      fontStyle: "italic",
    });
  });

  test("[WGT-004] a change over a type-scale style keeps the style and records only the change (#84)", async () => {
    const onChange = mock((_: unknown) => undefined);
    renderI18n(
      <RuntimePinContext.Provider value="0.6.0">
        <TextStyleEditor
          name="style"
          value={{ base: { $token: "typography.titleLarge" }, color: { $token: "color.onPrimary" } }}
          onChange={onChange}
          onEnd={() => undefined}
        />
      </RuntimePinContext.Provider>,
    );
    // The style shows its base's size, and its own change can be cleared.
    const size = (await screen.findAllByRole("spinbutton"))[0] as HTMLInputElement;
    expect(size.value).toBe("22");
    fireEvent.click(screen.getAllByRole("switch")[0] as HTMLElement);
    expect(onChange).toHaveBeenLastCalledWith({
      base: { $token: "typography.titleLarge" },
      color: { $token: "color.onPrimary" },
      fontStyle: "italic",
    });
    fireEvent.click(screen.getByRole("button", { name: "Clear changes" }));
    expect(onChange).toHaveBeenLastCalledWith({ $token: "typography.titleLarge" });
  });

  test("[WGT-004] an app whose oldest runtime predates a style's base gets a custom style instead", () => {
    expect(baseRuntime()).toBe("0.6.0");
    expect(canExtend("0.6.0")).toBe(true);
    expect(canExtend("0.10.0")).toBe(true);
    expect(canExtend("0.5.0")).toBe(false);
    expect(canExtend(undefined)).toBe(false);
  });
});
