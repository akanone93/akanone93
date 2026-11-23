// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, mock, test } from "bun:test";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { orgA, renderApp } from "../../test-harness.tsx";
import type { MonacoViewProps } from "../json-editor/monaco-view.types.ts";
import type { CanvasSurface } from "../plugin-canvas/surface.ts";
import type { ValidatorClient } from "../problems/validator-client.ts";

/** Monaco needs a real browser; the editor is tested against a stand-in with the same props. */
mock.module("../json-editor/monaco-view.tsx", () => ({
  default: (props: MonacoViewProps) => (
    <textarea
      aria-label={props.label}
      readOnly={props.readOnly}
      value={props.value}
      onChange={(e) => props.onChange(e.target.value)}
    />
  ),
}));

afterEach(cleanup);

const surface = (): CanvasSurface => ({
  layoutScreens: async () => [],
  setViewport: () => undefined,
  pan: () => undefined,
  zoomAbout: () => undefined,
  viewport: { x: 0, y: 0, zoom: 1 },
  dispose: () => undefined,
});
const validator = (): ValidatorClient => ({
  load: async () => [],
  validatePage: async () => [],
  dispose: () => undefined,
});
const app = {
  kind: "app",
  path: "app.json",
  json: { schemaVersion: "1.0.0", kind: "app", entryRoute: "home", nativeRoutes: ["camera"] },
};

describe("the app's own documents from the app canvas (#260, #285)", () => {
  test("the Overview opens the JSON editor on app.json, and an applied change is saved to it", async () => {
    const sent: NonNullable<Parameters<typeof renderApp>[1]>["sent"] = [];
    renderApp(`/o/${orgA}/apps/app1`, {
      surface: () => surface(),
      summaries: [{ id: "loans", key: "loans", name: "Loans" }],
      permissions: ["app.read", "app.manage", "plugin.read", "plugin.write"],
      appDocuments: [app],
      sent,
      validator,
    });
    fireEvent.click(await screen.findByRole("tab", { name: /Overview/ }, { timeout: 5000 }));
    fireEvent.click(await screen.findByRole("button", { name: "Edit the app's documents (JSON)…" }));
    const text = (await screen.findByLabelText("JSON of the document")) as HTMLTextAreaElement;
    await waitFor(() => expect(text.value).toContain('"nativeRoutes"'));
    const next = JSON.stringify({ ...app.json, nativeRoutes: ["camera", "scanner"] }, null, 2);
    fireEvent.change(text, { target: { value: `${next}\n` } });
    const apply = screen.getByRole("button", { name: "Apply" });
    await waitFor(() => expect(apply.matches(":disabled, [aria-disabled=true]")).toBe(false));
    fireEvent.click(apply);
    await waitFor(() => expect(sent.some((s) => s.path === "app.json")).toBe(true), { timeout: 5000 });
    const write = sent.find((s) => s.path === "app.json");
    expect(write?.ops.filter((o) => o.op !== "test")).toEqual([
      { op: "replace", path: "/nativeRoutes", value: ["camera", "scanner"] },
    ]);
  });
});
