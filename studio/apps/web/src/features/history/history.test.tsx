// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, mock, test } from "bun:test";
import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { orgA, renderApp } from "../../test-harness.tsx";
import type { EditSession } from "../editor/edit-session.ts";
import type { MonacoViewProps } from "../json-editor/monaco-view.types.ts";
import type { CanvasSurface } from "../plugin-canvas/surface.ts";
import { missingPaths, orderedPair, restoreRecipes, sessionTag } from "./history-model.ts";

/** Monaco needs a real browser; the overlay's own behaviour is tested against a stand-in with the same props. */
const views: MonacoViewProps[] = [];
mock.module("../json-editor/monaco-view.tsx", () => ({
  default: (props: MonacoViewProps) => {
    views.push(props);
    return (
      <textarea
        readOnly
        value={props.value}
        aria-label={props.label}
        data-diff={String(props.diff)}
        data-older={props.baseline}
        data-newer={props.value}
      />
    );
  },
}));

afterEach(() => {
  cleanup();
  views.length = 0;
});

const page = (route: string) => ({
  kind: "page",
  path: "pages/home.page.json",
  json: { id: "home", key: "home", route, kind: "page", pageKind: "screen", root: { id: "home-root", children: [] } },
});
const plugin = {
  kind: "plugin",
  path: "plugin.json",
  json: { id: "pl", kind: "plugin", entryPage: "home", pages: ["home"] },
};
const documents = [plugin, page("home")];
const snapshots = [
  { id: "s2", revision: 2, reason: "write", author: "Ada", documents: [plugin, page("start")] },
  { id: "s1", revision: 1, reason: "publish", author: "Grace", documents: [plugin, page("old")] },
];
const path = `/o/${orgA}/apps/app1/plugins/pl`;

const surface = (): CanvasSurface => ({
  layoutScreens: (docs) => Promise.resolve(docs.map((d) => ({ id: d.id, contentHeight: 300 }))),
  setViewport: () => undefined,
  setPaused: () => undefined,
  pan: () => undefined,
  zoomAbout: () => undefined,
  viewport: { x: 0, y: 0, zoom: 1 },
  dispose: () => undefined,
});

const open = async () => fireEvent.click(await screen.findByRole("button", { name: "History" }, { timeout: 5000 }));

describe("STU-069 history overlay V9", () => {
  test("lists the snapshots with author and time, and compares any two", async () => {
    renderApp(path, { documents, snapshots, surface });
    await open();
    const list = await screen.findByRole("list", { name: "Snapshots" });
    expect(within(list).getAllByRole("listitem")).toHaveLength(2);
    expect(within(list).getByText("by Ada")).toBeTruthy();
    expect(within(list).getByText("by Grace")).toBeTruthy();
    expect(screen.getByText("Choose two snapshots to compare them.")).toBeTruthy();

    for (const box of within(list).getAllByRole("checkbox")) {
      fireEvent.click(box);
    }
    const view = await screen.findByLabelText("Older and newer version of the document");
    expect(view.dataset["diff"]).toBe("true");
    expect(view.dataset["older"]).toContain('"route": "old"');
    expect(view.dataset["newer"]).toContain('"route": "start"');
  });

  test("restore brings back a deleted document through RestoreSnapshot", async () => {
    let edit: EditSession | undefined;
    const restoredSnapshots: { id: string; paths: string[] }[] = [];
    const extra = { kind: "page", path: "pages/gone.page.json", json: { id: "gone", kind: "page" } };
    renderApp(path, {
      documents,
      snapshots: [{ id: "s3", revision: 3, reason: "write", author: "Ada", documents: [plugin, page("home"), extra] }],
      restoredSnapshots,
      surface,
      editor: (e) => (edit = e),
    });
    await open();
    await screen.findByRole("list", { name: "Snapshots" });
    fireEvent.click(screen.getAllByRole("button", { name: /^Restore the snapshot of/ })[0] as HTMLElement);
    fireEvent.click(await screen.findByRole("button", { name: "Restore" }));
    await waitFor(() => expect(restoredSnapshots).toEqual([{ id: "s3", paths: ["pages/gone.page.json"] }]));
    await waitFor(() => expect(edit?.store.getState().docs["pages/gone.page.json"]).toBeDefined());
  });

  test("each snapshot shows the tag of its editor session", async () => {
    renderApp(path, {
      documents,
      snapshots: [
        { ...(snapshots[0] as (typeof snapshots)[number]), editorSession: "abcdef12-3456" },
        snapshots[1] as (typeof snapshots)[number],
      ],
      surface,
    });
    await open();
    expect(await screen.findByText("session abcdef12")).toBeTruthy();
  });

  test("restore asks for confirmation and writes the snapshot through the edit pipeline as one step", async () => {
    let edit: EditSession | undefined;
    const sent: NonNullable<Parameters<typeof renderApp>[1]>["sent"] = [];
    renderApp(path, { documents, snapshots, surface, sent, editor: (e) => (edit = e) });
    await open();
    await screen.findByRole("list", { name: "Snapshots" });
    const restore = screen.getAllByRole("button", { name: /^Restore the snapshot of/ });
    fireEvent.click(restore[1] as HTMLElement);
    expect(await screen.findByText("Restore this snapshot?")).toBeTruthy();
    expect(sent).toHaveLength(0);
    fireEvent.click(screen.getByRole("button", { name: "Restore" }));
    await waitFor(() => expect(screen.getByText("Restored 1 document.")).toBeTruthy());
    expect(edit?.store.getState().docs["pages/home.page.json"]?.content).toMatchObject({ route: "old" });
    edit?.flushAll();
    await waitFor(() => expect(sent.length).toBeGreaterThan(0));
    expect(sent[0]?.ops.filter((o) => o.op !== "test")).toEqual([{ op: "replace", path: "/route", value: "old" }]);
  });

  test("an empty history says so", async () => {
    renderApp(path, { documents, surface });
    await open();
    expect(await screen.findByText("There are no snapshots yet.")).toBeTruthy();
  });
});

describe("history model", () => {
  test("missingPaths names the documents the editor lacks and sessionTag shortens an id", () => {
    const enc = new TextEncoder().encode("{}");
    expect(
      missingPaths(
        [
          { path: "a", content: enc },
          { path: "b", content: enc },
        ],
        (p) => p === "a",
      ),
    ).toEqual(["b"]);
    expect(sessionTag("abcdef12-3456")).toBe("abcdef12");
  });

  test("orderedPair puts the older revision first and needs exactly two", () => {
    const rows = [
      { id: "a", revision: 5n },
      { id: "b", revision: 2n },
      { id: "c", revision: 9n },
    ];
    expect(orderedPair(rows, ["a", "b"])).toEqual(["b", "a"]);
    expect(orderedPair(rows, ["a"])).toBeUndefined();
    expect(orderedPair(rows, ["a", "b", "c"])).toBeUndefined();
  });

  test("restoreRecipes skips documents that are not JSON objects and merges the rest minimally", () => {
    const enc = (t: string) => new TextEncoder().encode(t);
    const edits = restoreRecipes([
      { path: "a.json", content: enc('{"x":1}') },
      { path: "b.json", content: enc("[1]") },
      { path: "c.json", content: enc("nope") },
    ]);
    expect(edits.map((e) => e.path)).toEqual(["a.json"]);
    const draft: Record<string, unknown> = { x: 0, y: 2 };
    edits[0]?.recipe(draft);
    expect(draft).toEqual({ x: 1 });
  });
});
