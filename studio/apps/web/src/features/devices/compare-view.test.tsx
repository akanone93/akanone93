// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, mock, test } from "bun:test";
import { createStudioI18n } from "@plux/i18n";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { I18nProvider } from "../../i18n-provider.tsx";
import { loadSourceCatalogue } from "../../test-harness.tsx";
import { CompareView } from "./compare-view.tsx";

afterEach(cleanup);

const studio = () => createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false });

describe("PC-38 the canvas compared with the device [DEV-033] [STU-005]", () => {
  test("side by side, then one over the other with the canvas's opacity, then their difference", async () => {
    const canvas = mock(async () => new Uint8Array([137, 80, 78, 71]));
    render(
      <I18nProvider studio={studio()}>
        <CompareView device="blob:device" canvas={canvas} />
      </I18nProvider>,
    );
    const shot = await screen.findByAltText("The same screen as the canvas draws it");
    expect(screen.getByAltText("The screen as the paired device shows it").getAttribute("src")).toBe("blob:device");
    expect(screen.getByText("Canvas")).toBeTruthy();
    const modes = screen.getAllByRole("radio");
    fireEvent.click(modes[1] as HTMLElement);
    const slider = screen.getByRole("slider", { name: "Canvas opacity" });
    fireEvent.change(slider, { target: { value: "20" } });
    expect((shot.parentElement as HTMLElement).style.opacity).toBe("0.2");
    fireEvent.click(modes[2] as HTMLElement);
    expect(shot.closest("[data-mode]")?.getAttribute("data-mode")).toBe("difference");
    expect(screen.getByText("Black where the canvas and the device match; what lights up differs.")).toBeTruthy();
    expect(canvas).toHaveBeenCalledTimes(1);
  });

  test("when the canvas has not drawn the screen, it says so", async () => {
    render(
      <I18nProvider studio={studio()}>
        <CompareView device="blob:device" canvas={async () => null} />
      </I18nProvider>,
    );
    await waitFor(() =>
      expect(
        screen.getByText("The canvas has not drawn this screen: open it on the canvas, then compare again."),
      ).toBeTruthy(),
    );
    expect(screen.queryByAltText("The same screen as the canvas draws it")).toBeNull();
  });
});
