// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { create } from "@bufbuild/protobuf";
import { ReleaseSchema } from "@plux/api";
import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { orgA, renderApp } from "../../test-harness.tsx";
import type { CanvasSurface } from "../plugin-canvas/surface.ts";
import { newestRelease } from "./app-releases.tsx";

afterEach(cleanup);

const surface = (): CanvasSurface => ({
  layoutScreens: async () => [],
  setViewport: () => undefined,
  pan: () => undefined,
  zoomAbout: () => undefined,
  viewport: { x: 0, y: 0, zoom: 1 },
  dispose: () => undefined,
});

describe("where the app stands, on the app canvas (#261)", () => {
  test("an environment's newest release is the one of the highest sequence", () => {
    const r = (sequence: number) => create(ReleaseSchema, { sequence: BigInt(sequence) });
    expect(newestRelease([r(3), r(9), r(4)])?.sequence).toBe(9n);
    expect(newestRelease([])).toBeUndefined();
  });

  test("the Overview lists each environment's newest release and offers Publish to those who may", async () => {
    renderApp(`/o/${orgA}/apps/app1`, {
      surface: () => surface(),
      summaries: [{ id: "loans", key: "loans", name: "Loans" }],
      permissions: ["app.read", "plugin.read", "release.publish"],
      releases: { releases: [{ sequence: 3 }, { sequence: 7 }] },
    });
    fireEvent.click(await screen.findByRole("tab", { name: /Overview/ }));
    const releases = await screen.findByRole("region", { name: "Releases" });
    await waitFor(() => expect(within(releases).getAllByText(/^Release 7 · /)).toHaveLength(2));
    expect(within(releases).getByText("production")).toBeTruthy();
    const publish = within(releases).getByRole("link", { name: "Publish…" });
    expect(publish.getAttribute("href")).toBe(`/o/${orgA}/apps/app1/publish`);
  });

  test("the Overview says who is editing which plugin (#267)", async () => {
    renderApp(`/o/${orgA}/apps/app1`, {
      surface: () => surface(),
      summaries: [
        { id: "loans", key: "loans", name: "Loans", lockHolder: "Ana" },
        { id: "cards", key: "cards", name: "Cards" },
      ],
      permissions: ["app.read", "plugin.read"],
    });
    fireEvent.click(await screen.findByRole("tab", { name: /Overview/ }));
    const editing = await screen.findByRole("region", { name: "Being edited" });
    await waitFor(() => expect(editing.textContent).toContain("LoansAna"));
    expect(editing.textContent).not.toContain("Cards");
  });

  test("without the permission to publish, there is no Publish", async () => {
    renderApp(`/o/${orgA}/apps/app1`, {
      surface: () => surface(),
      summaries: [{ id: "loans", key: "loans", name: "Loans" }],
      permissions: ["app.read", "plugin.read"],
    });
    fireEvent.click(await screen.findByRole("tab", { name: /Overview/ }));
    const releases = await screen.findByRole("region", { name: "Releases" });
    await waitFor(() => expect(within(releases).getAllByText("No release yet")).toHaveLength(2));
    expect(within(releases).queryByRole("link", { name: "Publish…" })).toBeNull();
  });
});
