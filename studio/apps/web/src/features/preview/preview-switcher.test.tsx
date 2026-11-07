// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { createStudioI18n } from "@plux/i18n";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { I18nProvider } from "../../i18n-provider.tsx";
import { loadSourceCatalogue } from "../../test-harness.tsx";
import { type PlacedScreen, screenStyle } from "../plugin-canvas/scene.ts";
import type { SurfaceFactory } from "../plugin-canvas/surface.ts";
import { CustomFrameDialog } from "./custom-frame-dialog.tsx";
import { DeviceMatrix, MATRIX_ROOM, matrixScale } from "./device-matrix.tsx";
import { frameCatalogue, matrixLayout, resolveFrame } from "./frames.ts";
import { MatrixOverlay, PreviewBarView } from "./preview-bar.tsx";
import { PreviewSwitcher } from "./preview-switcher.tsx";
import { defaultPreview, previewDevice, previewRender } from "./switches.ts";

afterEach(cleanup);

const renderI18n = (node: ReactNode) =>
  render(
    <I18nProvider studio={createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false })}>
      {node}
    </I18nProvider>,
  );

describe("[STU-058] switcher UI", () => {
  test("the toolbar offers every switch", async () => {
    renderI18n(<PreviewSwitcher state={defaultPreview} dataSources={["orders"]} onChange={() => {}} includePseudo />);
    expect(await screen.findByRole("toolbar", { name: "Preview switches" })).toBeTruthy();
    for (const label of ["Device frame", "Theme", "Locale", "Text scale", "Platform style", "Data state"]) {
      expect(screen.getAllByText(label).length).toBeGreaterThan(0);
    }
  });
});

describe("PC-23 a data state per source [DAT-080]", () => {
  test("with several sources each has its own state select, named after the source", async () => {
    renderI18n(
      <PreviewSwitcher
        part="extra"
        state={{ ...defaultPreview, mockStates: { orders: "loading" } }}
        dataSources={["orders", "user"]}
        sourceNames={{ orders: "Orders", user: "Profile" }}
        onChange={() => {}}
        includePseudo={false}
      />,
    );
    expect((await screen.findAllByText("Data: Orders")).length).toBeGreaterThan(0);
    expect(screen.getAllByText("Data: Profile").length).toBeGreaterThan(0);
    const boxes = screen.getAllByRole("combobox").map((c) => c.textContent ?? "");
    expect(boxes.filter((t) => t.includes("Loading")).length).toBe(2);
    expect(boxes.some((t) => t.includes("Success"))).toBe(true);
  });
});

describe("[STU-057] PC-11 frame orientation and chrome", () => {
  test("with the frame, the orientation and the device chrome can be set", async () => {
    renderI18n(
      <PreviewSwitcher
        part="main"
        state={defaultPreview}
        dataSources={[]}
        onChange={() => {}}
        includePseudo={false}
        view={{ orientation: "landscape", chrome: false }}
        onView={() => {}}
      />,
    );
    expect((await screen.findAllByText("Orientation")).length).toBeGreaterThan(0);
    expect(screen.getAllByText("Device chrome").length).toBeGreaterThan(0);
    const boxes = screen.getAllByRole("combobox").map((c) => c.textContent ?? "");
    expect(boxes.some((t) => t.includes("Landscape"))).toBe(true);
    expect(boxes.some((t) => t.includes("Hidden"))).toBe(true);
  });
});

describe("[STU-057] custom frame dialog D40", () => {
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

describe("[STU-057] device matrix", () => {
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

describe("[STU-057] matrix cells render the screen", () => {
  const screenOf: PlacedScreen = { id: "home", title: "Home", at: { x: 40, y: 40 }, root: undefined };
  /** A surface that records each layout and answers each image request with a PNG of its own. */
  const recording = () => {
    const made: string[] = [];
    const laid: { w: number | undefined; h: number | undefined; id: string | undefined; top: number | undefined }[] =
      [];
    const asked: { id: string; width: number; content: boolean | undefined }[] = [];
    const createSurface: SurfaceFactory = (_canvas, size) => {
      made.push(`${size.width}x${size.height} ${size.present}`);
      return {
        layoutScreens: async (screens, style) => {
          laid.push({ w: style.theme.width, h: style.deviceHeight, id: screens[0]?.id, top: style.chrome?.top });
          return [];
        },
        thumbnail: async (id, width, content) => {
          asked.push({ id, width, content });
          return new Uint8Array([137, 80, 78, 71]);
        },
        setViewport: () => {},
        pan: () => {},
        zoomAbout: () => {},
        viewport: { x: 0, y: 0, zoom: 1 },
        dispose: () => {},
      };
    };
    return { made, laid, asked, createSurface };
  };

  test("one surface lays the screen out at every frame's size, in turn, and draws each to an image", async () => {
    const r = recording();
    const { container } = renderI18n(
      <MatrixOverlay
        show
        render={previewRender(defaultPreview, "material")}
        screen={screenOf}
        style={screenStyle}
        createSurface={r.createSurface}
        onClose={() => {}}
      />,
    );
    await screen.findByRole("list", { name: "Device matrix" });
    await waitFor(() =>
      expect(container.querySelectorAll("img.device-matrix-image").length).toBe(frameCatalogue.length),
    );
    // One worker and one graphics context for the whole matrix, never one per device.
    expect(r.made).toEqual(["1x1 direct"]);
    const cells = matrixLayout(frameCatalogue, "portrait");
    expect(r.laid.map((l) => [l.w, l.h])).toEqual(cells.map((c) => [c.width, c.height]));
    expect(new Set(r.laid.map((l) => l.id))).toEqual(new Set(["home"]));
    // Each device keeps its own chrome: the iPhone's status bar is taller than the small Android phone's.
    expect(r.laid[cells.findIndex((c) => c.frame.id === "iphone")]?.top).toBe(47);
    expect(r.laid[0]?.top).toBe(24);
    // The image is the screen's content, at its drawn width: the tallest frame fills the room, the rest share its scale.
    const scale = matrixScale(cells, MATRIX_ROOM);
    expect(r.asked.every((a) => a.id === "screen:home" && a.content === true)).toBe(true);
    expect(r.asked.map((a) => a.width)).toEqual(cells.map((c) => Math.ceil(c.width * scale)));
  });

  test("a frame waits as a placeholder until its image is drawn", async () => {
    const pending: SurfaceFactory = () => ({
      layoutScreens: () => new Promise(() => {}),
      setViewport: () => {},
      pan: () => {},
      zoomAbout: () => {},
      viewport: { x: 0, y: 0, zoom: 1 },
      dispose: () => {},
    });
    const { container } = renderI18n(
      <MatrixOverlay
        show
        render={previewRender(defaultPreview, "material")}
        screen={screenOf}
        style={screenStyle}
        createSurface={pending}
        onClose={() => {}}
      />,
    );
    await screen.findByRole("list", { name: "Device matrix" });
    expect(container.querySelectorAll("[aria-busy='true']").length).toBe(frameCatalogue.length);
    expect(container.querySelectorAll("img").length).toBe(0);
  });

  test("the matrix is titled after the screen, explains itself, and Escape or the button closes it", async () => {
    const r = recording();
    let closed = 0;
    renderI18n(
      <MatrixOverlay
        show
        render={previewRender(defaultPreview, "material")}
        screen={screenOf}
        style={screenStyle}
        createSurface={r.createSurface}
        onClose={() => {
          closed += 1;
        }}
      />,
    );
    expect(await screen.findByRole("heading", { name: "Home on every device" })).toBeTruthy();
    fireEvent.keyDown(window, { key: "Escape" });
    fireEvent.click(screen.getByRole("button", { name: "Back to the canvas" }));
    expect(closed).toBe(2);
  });

  test("turned to landscape, every frame is drawn landscape", async () => {
    const r = recording();
    const landscape = previewRender(
      defaultPreview,
      "material",
      previewDevice(resolveFrame("pixel"), "landscape", true),
    );
    const { container } = renderI18n(
      <MatrixOverlay
        show
        render={landscape}
        screen={screenOf}
        style={screenStyle}
        createSurface={r.createSurface}
        onClose={() => {}}
      />,
    );
    await waitFor(() =>
      expect(container.querySelectorAll("img.device-matrix-image").length).toBe(frameCatalogue.length),
    );
    expect(r.laid[0]?.w).toBe(matrixLayout(frameCatalogue, "landscape")[0]?.width);
    expect((r.laid[0]?.w ?? 0) > (r.laid[0]?.h ?? 0)).toBe(true);
  });
});

describe("[STU-057] matrix toggle", () => {
  test("the toggle switches between the single frame and the matrix", async () => {
    const seen: boolean[] = [];
    renderI18n(<PreviewBarView switcher={<span />} matrix={false} onMatrix={(on) => seen.push(on)} />);
    const button = await screen.findByRole("button", { name: "Device matrix" });
    expect(button.getAttribute("aria-pressed")).toBe("false");
    fireEvent.click(button);
    expect(seen).toEqual([true]);
  });
});
