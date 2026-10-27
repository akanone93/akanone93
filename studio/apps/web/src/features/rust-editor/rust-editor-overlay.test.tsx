// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, mock, test } from "bun:test";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { orgA, renderApp } from "../../test-harness.tsx";
import type { CanvasSurface } from "../plugin-canvas/surface.ts";
import type { RustViewProps } from "./rust-monaco-view.tsx";

/** Monaco needs a real browser; the overlay is tested against a stand-in with the same props. */
const views: RustViewProps[] = [];
mock.module("./rust-monaco-view.tsx", () => ({
  default: (props: RustViewProps) => {
    views.push(props);
    return (
      <textarea
        aria-label={props.label}
        data-markers={props.markers.map((m) => `${m.startLine}:${m.severity}:${m.message}`).join("|")}
        data-language={String(props.language !== undefined)}
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

const documents = [
  { kind: "plugin", path: "plugin.json", json: { id: "pl", kind: "plugin", entryPage: "home", pages: ["home"] } },
  {
    kind: "page",
    path: "pages/home.page.json",
    json: { id: "home", key: "home", route: "home", kind: "page", pageKind: "screen", root: { id: "r", children: [] } },
  },
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

/** A language server that initialises, reports one error for the document it is given and answers hovers. */
function languageServer(received: string[]) {
  return (text: string): string[] => {
    const m = JSON.parse(text) as { id?: number; method?: string; params?: { textDocument?: { uri?: string } } };
    if (m.method === "initialize") {
      return [JSON.stringify({ jsonrpc: "2.0", id: m.id, result: { capabilities: {} } })];
    }
    if (m.method === "textDocument/didOpen") {
      return [
        JSON.stringify({
          jsonrpc: "2.0",
          method: "textDocument/publishDiagnostics",
          params: {
            uri: m.params?.textDocument?.uri,
            diagnostics: [
              {
                range: { start: { line: 2, character: 4 }, end: { line: 2, character: 9 } },
                severity: 1,
                message: "mismatched types",
                code: "E0308",
              },
            ],
          },
        }),
      ];
    }
    if (m.method === "textDocument/hover") {
      return [
        JSON.stringify({ jsonrpc: "2.0", id: m.id, result: { contents: { kind: "markdown", value: "fn fee()" } } }),
      ];
    }
    received.push(text);
    return [];
  };
}

const openOverlay = async () =>
  fireEvent.click(await screen.findByRole("button", { name: "Rust editor" }, { timeout: 5000 }));

describe("FN-033 Rust editor V3", () => {
  test("starts the language server on an uploaded source, shows its diagnostics and answers hovers", async () => {
    const received: string[] = [];
    const log: string[] = [];
    renderApp(path, { documents, surface, rust: { reply: languageServer(received), received: log } });
    await openOverlay();
    const area = (await screen.findByLabelText("Rust source of the function")) as HTMLTextAreaElement;
    expect(area.value).toContain("plux_fn");
    expect(area.dataset["language"]).toBe("false");
    const start = screen.getByRole("button", { name: "Start the language server" });
    expect(start.matches(":disabled, [aria-disabled=true]")).toBe(true);

    fireEvent.change(screen.getByLabelText("Uploaded source"), {
      target: { value: "0192f000-0000-7000-8000-0000000000aa" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Start the language server" }));
    await waitFor(() => expect(screen.getByText("The language server is ready.")).toBeTruthy());
    await waitFor(() => expect(area.dataset["markers"]).toBe("3:error:E0308: mismatched types"));
    expect(area.dataset["language"]).toBe("true");
    expect(JSON.parse(log[0] ?? "{}").method).toBe("initialize");
    expect(log.map((l) => (JSON.parse(l) as { method: string }).method)).toEqual([
      "initialize",
      "initialized",
      "textDocument/didOpen",
    ]);

    const hover = await views.at(-1)?.language?.hover({ line: 0, character: 0 });
    expect(hover?.contents).toEqual({ kind: "markdown", value: "fn fee()" });

    fireEvent.change(area, { target: { value: "fn main() {}\n" } });
    await waitFor(() =>
      expect(log.some((l) => l.includes("textDocument/didChange") && l.includes("fn main()"))).toBe(true),
    );

    fireEvent.click(screen.getByRole("button", { name: "Stop the language server" }));
    await waitFor(() => expect(screen.getByText("The language server was stopped.")).toBeTruthy());
    expect(area.dataset["language"]).toBe("false");
  });

  test("a starter template replaces the source", async () => {
    renderApp(path, { documents, surface, rust: { reply: () => [] } });
    await openOverlay();
    const area = (await screen.findByLabelText("Rust source of the function")) as HTMLTextAreaElement;
    const before = area.value;
    fireEvent.click(screen.getByRole("combobox", { name: "Starter template" }));
    const option = await screen.findByRole("option", { name: "Currency conversion" });
    await new Promise((resolve) => setTimeout(resolve, 300));
    fireEvent.pointerDown(option);
    fireEvent.mouseDown(option);
    fireEvent.pointerUp(option);
    fireEvent.mouseUp(option);
    fireEvent.click(option);
    const replace = screen.getByRole("button", { name: "Replace the source with the template" });
    await waitFor(() => expect(replace.matches(":disabled, [aria-disabled=true]")).toBe(false));
    await waitFor(() =>
      expect(screen.getByRole("combobox", { name: "Starter template" }).textContent).toBe("Currency conversion"),
    );
    fireEvent.click(replace);
    await waitFor(() => expect(area.value).not.toBe(before));
  });

  test("the test runner reports each case and the build output", async () => {
    renderApp(path, {
      documents,
      surface,
      rust: {
        reply: () => [],
        tests: {
          results: [
            { name: "adds", passed: true, message: "" },
            { name: "rounds", passed: false, message: "left != right" },
          ],
          diagnostics: "warning: unused",
        },
      },
    });
    await openOverlay();
    await screen.findByLabelText("Rust source of the function");
    const run = screen.getByRole("button", { name: "Run the tests" });
    expect(run.matches(":disabled, [aria-disabled=true]")).toBe(true);
    fireEvent.change(screen.getByLabelText("Uploaded source"), {
      target: { value: "0192f000-0000-7000-8000-0000000000aa" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Run the tests" }));
    await waitFor(() => expect(screen.getByText("1 test passed, 1 failed.")).toBeTruthy());
    expect(screen.getByText("adds")).toBeTruthy();
    expect(screen.getByText(/left != right/)).toBeTruthy();
    expect(screen.getByText("warning: unused")).toBeTruthy();
  });

  test("the dependency manager adds a valid crate, refuses an invalid one and removes it", async () => {
    renderApp(path, { documents, surface, rust: { reply: () => [] } });
    await openOverlay();
    await screen.findByLabelText("Rust source of the function");
    fireEvent.click(screen.getByRole("tab", { name: "Dependencies" }));
    fireEvent.change(screen.getByLabelText("Crate name"), { target: { value: "1bad" } });
    expect(await screen.findByText("Crate names start with a letter and use letters, digits, - and _.")).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Add or update the dependency" }).matches(":disabled, [aria-disabled=true]"),
    ).toBe(true);
    fireEvent.change(screen.getByLabelText("Crate name"), { target: { value: "serde" } });
    fireEvent.change(screen.getByLabelText("Version requirement"), { target: { value: "x" } });
    expect(await screen.findByText("Use a version requirement such as 1.0 or ^0.8.2.")).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Version requirement"), { target: { value: "1.0" } });
    fireEvent.click(screen.getByRole("button", { name: "Add or update the dependency" }));
    await waitFor(() => expect(screen.getAllByText(/serde = "1.0"/).length).toBeGreaterThan(0));
    fireEvent.click(screen.getByRole("button", { name: "Remove serde" }));
    await waitFor(() => expect(screen.getByText("The function has no dependencies.")).toBeTruthy());
  });

  test("a server that cannot start the sandbox is reported, not hidden", async () => {
    renderApp(path, { documents, surface });
    await openOverlay();
    await screen.findByLabelText("Rust source of the function");
    fireEvent.change(screen.getByLabelText("Uploaded source"), {
      target: { value: "0192f000-0000-7000-8000-0000000000aa" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Start the language server" }));
    await waitFor(() => expect(screen.getByText(/The language server could not start/)).toBeTruthy());
  });
});
