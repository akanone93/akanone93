// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { orgA, renderApp } from "../../test-harness.tsx";
import type { CanvasSurface } from "../plugin-canvas/surface.ts";

afterEach(cleanup);

const text = (id: string, data: string) => ({ id, type: "Text", props: { data } });
const documents = [
  { kind: "plugin", path: "plugin.json", json: { id: "pl", kind: "plugin", entryPage: "home", pages: ["home"] } },
  {
    kind: "page",
    path: "pages/home.page.json",
    json: {
      id: "home",
      key: "home",
      route: "home",
      kind: "page",
      pageKind: "screen",
      root: {
        id: "home-root",
        slots: { body: { id: "home-body", children: [text("t1", "Hello"), text("t2", "World")] } },
      },
    },
  },
];
const path = `/o/${orgA}/apps/app1/plugins/pl`;

/** A surface whose hit test answers with the nodes the test names. */
function surface(hits: { current: string[] }): () => CanvasSurface {
  return () => ({
    layoutScreens: (docs) => Promise.resolve(docs.map((d) => ({ id: d.id, contentHeight: 300 }))),
    setViewport: () => undefined,
    pan: () => undefined,
    zoomAbout: () => undefined,
    viewport: { x: 0, y: 0, zoom: 1 },
    hitTest: () => hits.current,
    dispose: () => undefined,
  });
}

function pick(canvas: HTMLElement, shift: boolean) {
  fireEvent.pointerDown(canvas, { clientX: 1, clientY: 1, ctrlKey: true, shiftKey: shift });
  fireEvent.pointerUp(canvas, { clientX: 1, clientY: 1, ctrlKey: true, shiftKey: shift });
}

describe("S2.6 right dock on the plugin canvas", () => {
  test("[STU-121] the selected node's property is generated from its descriptor and edits through the pipeline", async () => {
    const hits = { current: ["t1"] };
    renderApp(path, { documents, surface: surface(hits) });
    const canvas = await screen.findByLabelText("Plugin canvas");
    expect(await screen.findByText("Select a widget on the canvas to edit it.")).toBeTruthy();
    pick(canvas, false);
    const data = (await screen.findByRole("textbox", { name: "data" })) as HTMLInputElement;
    expect(data.value).toBe("Hello");
    fireEvent.change(data, { target: { value: "Hi" } });
    await waitFor(() => expect((screen.getByRole("textbox", { name: "data" }) as HTMLInputElement).value).toBe("Hi"));
  });

  test("a multi-selection with different values shows Mixed", async () => {
    const hits = { current: ["t1"] };
    renderApp(path, { documents, surface: surface(hits) });
    const canvas = await screen.findByLabelText("Plugin canvas");
    pick(canvas, false);
    await screen.findByRole("textbox", { name: "data" });
    hits.current = ["t2"];
    pick(canvas, true);
    await waitFor(() =>
      expect((screen.getByRole("textbox", { name: "data" }) as HTMLInputElement).placeholder).toBe("Mixed"),
    );
  });

  test("the Docs tab documents the widget", async () => {
    const hits = { current: ["t1"] };
    renderApp(path, { documents, surface: surface(hits) });
    pick(await screen.findByLabelText("Plugin canvas"), false);
    fireEvent.click(await screen.findByRole("tab", { name: "Docs" }));
    expect(await screen.findByRole("heading", { name: "Text" })).toBeTruthy();
  });
});
