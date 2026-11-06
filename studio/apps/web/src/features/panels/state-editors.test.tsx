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
  dimensionChoices,
  ShapeEditor,
  StateValueEditor,
  stateInnerOf,
  TokenNumber,
  tokenFamilyOf,
} from "./state-editors.tsx";
import { PaintThemeContext } from "./style-editors.tsx";

afterEach(cleanup);

const renderI18n = (node: ReactNode) =>
  render(
    <I18nProvider studio={createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false })}>
      {node}
    </I18nProvider>,
  );

const dimensions = { "space.sm": 8, "space.md": 12, "radius.md": 12, "radius.lg": 20 };
const theme = { scheme: colorSchemeFromSeed(0xff6750a4, false), tokens: {}, dimensions };

describe("PC-21 state, shape and token editors [STU-062] [THM-001]", () => {
  test("a WidgetState type edits the type it holds; any other type is not a state type", () => {
    expect(stateInnerOf("WidgetStateColor")).toBe("color");
    expect(stateInnerOf("WidgetStateShapeBorder?")).toBe("ShapeBorder");
    expect(stateInnerOf("WidgetStateDouble")).toBe("double");
    expect(stateInnerOf("BoxDecoration")).toBeUndefined();
  });

  test("a state chip picks the state edited; setting it keeps the others and clearing the last one clears all", async () => {
    const onChange = mock((_: unknown) => undefined);
    renderI18n(
      <StateValueEditor
        name="backgroundColor"
        inner="color"
        value={{ value: "#FF0000FF" }}
        readOnly={false}
        onChange={onChange}
        onEnd={() => undefined}
      >
        {(state, value, set) => (
          <button type="button" onClick={() => set(value === undefined ? `${state}!` : undefined)}>
            edit {state}
          </button>
        )}
      </StateValueEditor>,
    );
    // The test catalogue does not format ICU selects, so the chips are found in their order: Default, Pressed, …
    const tabs = await screen.findAllByRole("tab");
    expect(tabs).toHaveLength(8);
    const [byDefault, pressed] = tabs as [HTMLElement, HTMLElement];
    expect(byDefault.getAttribute("aria-selected")).toBe("true");
    expect(byDefault.dataset["set"]).toBe("true");
    fireEvent.click(pressed);
    expect(pressed.getAttribute("aria-selected")).toBe("true");
    fireEvent.click(screen.getByRole("button", { name: "edit pressed" }));
    expect(onChange).toHaveBeenLastCalledWith({ value: "#FF0000FF", pressed: "pressed!" });
    expect(screen.getByText(/uses Default/)).toBeTruthy();
    fireEvent.click(byDefault);
    fireEvent.click(screen.getByRole("button", { name: "edit value" }));
    expect(onChange).toHaveBeenLastCalledWith(undefined);
  });

  test("the shape editor sets a kind, a radius and an outline", async () => {
    const onChange = mock((_: unknown) => undefined);
    renderI18n(
      <ShapeEditor
        name="shape"
        value={{ kind: "roundedRectangle", borderRadius: { all: 8 } }}
        readOnly={false}
        onChange={onChange}
        onEnd={() => undefined}
      />,
    );
    const radius = (await screen.findByRole("spinbutton", { name: "Radius" })) as HTMLInputElement;
    expect(radius.value).toBe("8");
    fireEvent.change(radius, { target: { value: "16" } });
    expect(onChange).toHaveBeenLastCalledWith({ kind: "roundedRectangle", borderRadius: { all: 16 } });
    fireEvent.change(screen.getByRole("spinbutton", { name: "Outline width" }), { target: { value: "2" } });
    expect(onChange).toHaveBeenLastCalledWith({
      kind: "roundedRectangle",
      borderRadius: { all: 8 },
      side: { width: 2 },
    });
    expect(screen.getByRole("button", { name: /Shape of shape/ })).toBeTruthy();
  });

  test("numbers offer the tokens of their family first and show a bound token as a chip that unbinds to pixels", async () => {
    expect(tokenFamilyOf("style.shape.borderRadius.all")).toBe("radius");
    expect(tokenFamilyOf("spacing")).toBe("space");
    expect(tokenFamilyOf("data")).toBeUndefined();
    expect(dimensionChoices("borderRadius", dimensions).map((c) => c.path)).toEqual([
      "radius.md",
      "radius.lg",
      "space.sm",
      "space.md",
    ]);
    const onChange = mock((_: unknown) => undefined);
    renderI18n(
      <PaintThemeContext.Provider value={theme}>
        <TokenNumber
          name="spacing"
          value={{ $token: "space.md" }}
          readOnly={false}
          onChange={onChange}
          onEnd={() => undefined}
        >
          <input aria-label="raw" />
        </TokenNumber>
      </PaintThemeContext.Provider>,
    );
    await waitFor(() => expect(screen.queryByText("space.md") === null).toBe(false));
    expect(screen.queryByRole("textbox", { name: "raw" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: /Unlink space.md/ }));
    expect(onChange).toHaveBeenLastCalledWith(12);
  });

  test("an unbound number keeps its field and binds to a token from the menu", async () => {
    const onChange = mock((_: unknown) => undefined);
    renderI18n(
      <PaintThemeContext.Provider value={theme}>
        <TokenNumber name="spacing" value={4} readOnly={false} onChange={onChange} onEnd={() => undefined}>
          <input aria-label="raw" />
        </TokenNumber>
      </PaintThemeContext.Provider>,
    );
    expect(await screen.findByRole("textbox", { name: "raw" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /Use a theme token for spacing/ }));
    fireEvent.click(await screen.findByRole("menuitem", { name: "space.md · 12" }));
    expect(onChange).toHaveBeenLastCalledWith({ $token: "space.md" });
  });
});
