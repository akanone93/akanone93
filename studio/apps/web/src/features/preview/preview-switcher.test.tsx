// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { createStudioI18n } from "@plux/i18n";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { I18nProvider } from "../../i18n-provider.tsx";
import { loadSourceCatalogue } from "../../test-harness.tsx";
import { CustomFrameDialog } from "./custom-frame-dialog.tsx";
import { DeviceMatrix } from "./device-matrix.tsx";
import { resolveFrame } from "./frames.ts";
import { PreviewSwitcher } from "./preview-switcher.tsx";
import { defaultPreview, previewRender } from "./switches.ts";

afterEach(cleanup);

const renderI18n = (node: ReactNode) =>
  render(
    <I18nProvider studio={createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false })}>
      {node}
    </I18nProvider>,
  );

describe("STU-058 switcher UI", () => {
  test("the toolbar offers every switch", async () => {
    renderI18n(<PreviewSwitcher state={defaultPreview} dataSources={["orders"]} onChange={() => {}} includePseudo />);
    expect(await screen.findByRole("toolbar", { name: "Preview switches" })).toBeTruthy();
    for (const label of ["Device frame", "Theme", "Locale", "Text scale", "Platform style", "Data state"]) {
      expect(screen.getAllByText(label).length).toBeGreaterThan(0);
    }
  });
});

describe("STU-057 custom frame dialog D40", () => {
  test("a valid size is handed over, an invalid one shows the error", async () => {
    const used: unknown[] = [];
    renderI18n(<CustomFrameDialog open onClose={() => {}} onUse={(f) => used.push(f)} />);
    fireEvent.change(await screen.findByLabelText("Width (logical pixels)"), { target: { value: "0" } });
    fireEvent.click(screen.getByText("Use frame"));
    expect(screen.getByText("Enter whole positive pixels for the size and a positive density.")).toBeTruthy();
    expect(used).toEqual([]);
    fireEvent.change(screen.getByLabelText("Width (logical pixels)"), { target: { value: "500" } });
    fireEvent.click(screen.getByText("Use frame"));
    expect(used).toMatchObject([{ width: 500, height: 800, density: 2 }]);
  });
});

describe("STU-057 device matrix", () => {
  test("one cell per frame, sized and mirrored by the switches", async () => {
    const render1 = previewRender({ ...defaultPreview, locale: "ar", textScale: 1.5 }, "material");
    renderI18n(
      <DeviceMatrix
        frames={[resolveFrame("android-small"), resolveFrame("ipad")]}
        orientation="portrait"
        render={render1}
        renderScreen={(c) => <span>{c.sizeClass}</span>}
      />,
    );
    const list = await screen.findByRole("list", { name: "Device matrix" });
    expect(list.getAttribute("dir")).toBe("rtl");
    const items = screen.getAllByRole("listitem");
    expect(items.length).toBe(2);
    expect(items[0]?.getAttribute("data-size-class")).toBe("compact");
    expect((items[1] as HTMLElement).style.fontSize).toBe("1.5em");
  });
});
