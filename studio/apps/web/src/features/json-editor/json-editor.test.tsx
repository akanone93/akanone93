// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, mock, test } from "bun:test";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { orgA, renderApp } from "../../test-harness.tsx";
import type { EditSession } from "../editor/edit-session.ts";
import type { CanvasSurface } from "../plugin-canvas/surface.ts";
import type { ValidatorClient } from "../problems/validator-client.ts";
import type { MonacoViewProps } from "./monaco-view.types.ts";

/** Monaco needs a real browser; the page's own behaviour is tested against a stand-in with the same props. */
const views: MonacoViewProps[] = [];
mock.module("./monaco-view.tsx", () => ({
  default: (props: MonacoViewProps) => {
    views.push(props);
    return (
      <textarea
        aria-label={props.label}
        data-diff={String(props.diff)}
        data-markers={props.markers.map((m) => `${m.startLine}:${m.message}`).join("|")}
        readOnly={props.readOnly}
        value={props.value}
        onChange={(e) => props.onChange(e.target.value)}
      />
    );
  },
}));

afterEach(() => {
  cleanup();
  views.length = 0;
});

const page = (id: string, route: string) => ({
  kind: "page",
  path: `pages/${route}.page.json`,
  json: { id, key: route, route, kind: "page", pageKind: "screen", root: { id: `${id}-root`, children: [] } },
});
const documents = [
  { kind: "plugin", path: "plugin.json", json: { id: "pl", kind: "plugin", entryPage: "home", pages: ["home"] } },
  page("home", "home"),
];
const path = `/o/${orgA}/apps/app1/plugins/pl`;

function surface() {
  const calls: string[] = [];
  const s: CanvasSurface = {
    layoutScreens: (docs) => Promise.resolve(docs.map((d) => ({ id: d.id, contentHeight: 300 }))),
    setViewport: (x, y, z) => calls.push(`view ${x},${y},${z}`),
    setPaused: (p) => calls.push(p ? "pause" : "resume"),
    pan: () => undefined,
    zoomAbout: () => undefined,
    viewport: { x: 12, y: 34, zoom: 0.5 },
    dispose: () => undefined,
  };
  return { calls, factory: () => s };
}

const validator =
  (findings: (data: string) => { code: string; severity: "error" | "warning"; path: string; message: string }[]) =>
  (): ValidatorClient => ({
    load: async () => [],
    validatePage: async (file, data) => findings(data).map((f) => ({ ...f, file })),
    dispose: () => undefined,
  });

const area = async () => (await screen.findByLabelText("JSON of the document")) as HTMLTextAreaElement;
const apply = () => screen.getByRole("button", { name: "Apply" });
const disabled = (el: HTMLElement) => el.matches(":disabled, [aria-disabled=true]");

describe("STU-064 JSON editor V1", () => {
  test("validates as you type, blocks Apply while invalid, and applies only a valid change as a minimal patch", async () => {
    let edit: EditSession | undefined;
    const sent: NonNullable<Parameters<typeof renderApp>[1]>["sent"] = [];
    const s = surface();
    renderApp(path, {
      documents,
      surface: s.factory,
      sent,
      editor: (e) => (edit = e),
      validator: validator((data) =>
        data.includes('"route": "start"')
          ? []
          : data.includes("oops")
            ? [{ code: "PLX-1", severity: "error", path: "/route", message: "bad route" }]
            : [],
      ),
    });
    // The first find waits for the whole app to boot (router, documents, validator), which under the
    // full coverage run can exceed the 1 s default; later waits keep the default.
    fireEvent.click(await screen.findByRole("button", { name: "JSON editor" }, { timeout: 5000 }));
    const text = await area();
    expect(text.value).toContain('"route": "home"');
    expect(text.value.endsWith("}\n")).toBe(true);
    const original = text.value;

    fireEvent.change(text, { target: { value: "{ not json" } });
    await waitFor(() => expect(screen.getAllByText(/Not valid JSON/).length).toBeGreaterThan(0));
    expect(disabled(apply())).toBe(true);
    expect(text.dataset["markers"]).toContain("1:Not valid JSON");

    fireEvent.change(text, { target: { value: original.replace('"home"', '"oops"') } });
    await waitFor(() => expect(screen.getByText("PLX-1")).toBeTruthy());
    expect(disabled(apply())).toBe(true);
    expect(text.dataset["markers"]).toContain("PLX-1: bad route");

    const valid = JSON.stringify(JSON.parse(original.replace('"route": "home"', '"route": "start"')), null, 2);
    fireEvent.change(text, { target: { value: `${valid}\n` } });
    await waitFor(() => expect(disabled(apply())).toBe(false));
    expect(screen.getByText("Valid. You can apply it.")).toBeTruthy();
    fireEvent.click(apply());
    expect(screen.getByText("Applied to the document.")).toBeTruthy();
    expect(edit?.store.getState().docs["pages/home.page.json"]?.content).toMatchObject({ route: "start", key: "home" });
    edit?.flushAll();
    await waitFor(() => expect(sent.length).toBeGreaterThan(0));
    const [write] = sent;
    expect(write?.path).toBe("pages/home.page.json");
    expect(write?.ops.filter((o) => o.op !== "test")).toEqual([{ op: "replace", path: "/route", value: "start" }]);
  });

  test("a document that is not an object, and an unready validator, cannot be applied", async () => {
    renderApp(path, { documents, surface: surface().factory });
    fireEvent.click(await screen.findByRole("button", { name: "JSON editor" }, { timeout: 5000 }));
    const text = await area();
    fireEvent.change(text, { target: { value: "[1]" } });
    await waitFor(() => expect(screen.getAllByText("A document must be a JSON object.").length).toBeGreaterThan(0));
    expect(disabled(apply())).toBe(true);
  });

  test("the diff compares with the last published version, or says there is none", async () => {
    const published = [{ path: "pages/home.page.json", json: { id: "home", route: "old" } }];
    renderApp(path, { documents, surface: surface().factory, published });
    fireEvent.click(await screen.findByRole("button", { name: "JSON editor" }, { timeout: 5000 }));
    await area();
    fireEvent.click(screen.getByRole("button", { name: "Compare with the last published version" }));
    await waitFor(() => expect(views.at(-1)?.diff).toBe(true));
    expect(views.at(-1)?.baseline).toBe('{\n  "id": "home",\n  "route": "old"\n}\n');
    cleanup();

    renderApp(path, { documents, surface: surface().factory });
    fireEvent.click(await screen.findByRole("button", { name: "JSON editor" }, { timeout: 5000 }));
    await area();
    fireEvent.click(screen.getByRole("button", { name: "Compare with the last published version" }));
    expect(await screen.findByText(/has not been published yet/)).toBeTruthy();
    expect(views.at(-1)?.diff).toBe(false);
  });

  test("read-only while someone else holds the lock: the text is read-only and Apply is off", async () => {
    renderApp(path, { documents, surface: surface().factory, lockHeldElsewhere: true });
    fireEvent.click(await screen.findByRole("button", { name: "JSON editor" }, { timeout: 5000 }));
    const text = await area();
    await waitFor(() => expect(text.readOnly).toBe(true));
    expect(disabled(apply())).toBe(true);
    expect(screen.getByText(/read-only/)).toBeTruthy();
  });
});

describe("STU-063 overlay host", () => {
  test("opening pauses the canvas without unmounting it; closing restores viewport, focus and selection", async () => {
    const s = surface();
    renderApp(path, { documents, surface: s.factory });
    const canvas = await screen.findByRole("application");
    canvas.focus();
    expect(document.activeElement).toBe(canvas);
    fireEvent.click(await screen.findByRole("button", { name: "JSON editor" }, { timeout: 5000 }));
    await area();
    expect(s.calls.filter((c) => c === "pause")).toHaveLength(1);
    expect(screen.getByRole("application", { hidden: true })).toBe(canvas);
    expect(canvas.isConnected).toBe(true);
    const before = s.calls.length;
    fireEvent.keyDown(document.activeElement ?? document.body, { key: "Escape" });
    await waitFor(() => expect(screen.queryByLabelText("JSON of the document")).toBeNull());
    expect(s.calls.slice(before)).toEqual(["view 12,34,0.5", "resume"]);
    await waitFor(() => expect(document.activeElement).toBe(canvas));
    expect(screen.getByRole("application")).toBe(canvas);
  });
});
