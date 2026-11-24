// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { FollowedZoomControls } from "./canvas-chrome.tsx";

afterEach(cleanup);

const labels = {
  zoomControls: "Zoom",
  zoomIn: "Zoom in",
  zoomOut: "Zoom out",
  zoomToFit: "Zoom to fit",
  zoomReset: (percent: number) => `Reset zoom from ${percent}%`,
  zoomPercent: (percent: number) => `${percent}%`,
};

describe("the zoom controls (#214)", () => {
  test("the percentage follows the surface's zoom, with no new value from the page", async () => {
    const surface = { viewport: { zoom: 0.5 } };
    const noop = () => undefined;
    render(
      <FollowedZoomControls
        surface={() => surface}
        zoom={1}
        messages={labels}
        onZoomIn={noop}
        onZoomOut={noop}
        onReset={noop}
        onFit={noop}
      />,
    );
    expect(screen.getByRole("button", { name: "Reset zoom from 50%" })).toBeTruthy();
    surface.viewport = { zoom: 0.1 };
    await waitFor(() => expect(screen.getByRole("button", { name: "Reset zoom from 10%" })).toBeTruthy());
  });

  test("before the canvas starts, the page's value stands", () => {
    const noop = () => undefined;
    render(
      <FollowedZoomControls
        surface={() => null}
        zoom={0.75}
        messages={labels}
        onZoomIn={noop}
        onZoomOut={noop}
        onReset={noop}
        onFit={noop}
      />,
    );
    expect(screen.getByRole("button", { name: "Reset zoom from 75%" })).toBeTruthy();
  });
});
