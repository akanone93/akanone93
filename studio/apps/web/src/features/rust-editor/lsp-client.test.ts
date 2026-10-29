// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import type { BridgeHandlers } from "./lsp-bridge.ts";
import { LspClient, RpcError } from "./lsp-client.ts";
import { hoverTexts, markersOfDiagnostics, markupText, toLspPosition, toMonacoRange } from "./lsp-convert.ts";
import { RustSession, type SessionBridge, type SessionState } from "./rust-session.ts";

function transport() {
  const sent: Record<string, unknown>[] = [];
  return { sent, send: async (m: unknown) => void sent.push(m as Record<string, unknown>) };
}

describe("[FN-033] LSP client", () => {
  test("correlates answers with requests and rejects server errors", async () => {
    const t = transport();
    const c = new LspClient(t);
    const ok = c.request<string>("a", {});
    const bad = c.request("b", {});
    c.receive(JSON.stringify({ jsonrpc: "2.0", id: 2, error: { code: -32803, message: "nope" } }));
    c.receive(JSON.stringify({ jsonrpc: "2.0", id: 1 }));
    expect(await ok).toBeNull();
    await expect(bad).rejects.toMatchObject({ code: -32803, message: "nope" });
    expect(t.sent.map((m) => m["method"])).toEqual(["a", "b"]);
  });

  test("answers the server's requests and routes its notifications", () => {
    const t = transport();
    const c = new LspClient(t);
    const seen: unknown[] = [];
    c.onNotification("n", (p) => seen.push(p));
    c.receive(
      JSON.stringify({ jsonrpc: "2.0", id: 7, method: "workspace/configuration", params: { items: [{}, {}] } }),
    );
    c.receive(JSON.stringify({ jsonrpc: "2.0", id: "x", method: "client/registerCapability", params: {} }));
    c.receive(JSON.stringify({ jsonrpc: "2.0", id: 9, method: "unknown/thing" }));
    c.receive(JSON.stringify({ jsonrpc: "2.0", method: "n", params: { a: 1 } }));
    c.receive(JSON.stringify({ jsonrpc: "2.0", method: "ignored" }));
    c.receive("not json");
    c.receive("5");
    c.receive(JSON.stringify({ jsonrpc: "2.0", id: "str", result: 1 }));
    expect(seen).toEqual([{ a: 1 }]);
    expect(t.sent[0]).toMatchObject({ id: 7, result: [null, null] });
    expect(t.sent[1]).toMatchObject({ id: "x", result: null });
    expect(t.sent[2]).toMatchObject({ id: 9, error: { code: -32601 } });
  });

  test("malformed server messages are dropped or fail their request, never throw", async () => {
    const t = transport();
    const c = new LspClient(t);
    const seen: unknown[] = [];
    c.onNotification("n", (p) => seen.push(p));
    const first = c.request("a", {});
    const second = c.request("b", {});
    const third = c.request("c", {});
    for (const raw of [
      JSON.stringify([{ jsonrpc: "2.0", id: 1, result: 1 }]),
      JSON.stringify({ jsonrpc: "2.0", method: 5, params: {} }),
      JSON.stringify({ jsonrpc: "2.0", id: { x: 1 }, method: "workspace/configuration", params: { items: [{}] } }),
      JSON.stringify({ jsonrpc: "2.0", id: null, result: 1 }),
      JSON.stringify({ jsonrpc: "2.0", id: 8, method: "workspace/configuration", params: { items: "x" } }),
      JSON.stringify({ jsonrpc: "2.0", id: 1, error: null }),
      JSON.stringify({ jsonrpc: "2.0", id: 2, error: { code: "x", message: 7 } }),
      JSON.stringify({ jsonrpc: "2.0", id: 3, error: "broken" }),
    ]) {
      expect(() => c.receive(raw)).not.toThrow();
    }
    await expect(first).rejects.toMatchObject({ code: -32603, message: "the language server failed" });
    await expect(second).rejects.toMatchObject({ code: -32603, message: "the language server failed" });
    await expect(third).rejects.toBeInstanceOf(RpcError);
    expect(seen).toEqual([]);
    expect(t.sent.slice(3)).toEqual([{ jsonrpc: "2.0", id: 8, result: [] }]);
  });

  test("a request times out, a failed send rejects, and closing fails what is open", async () => {
    const slow = new LspClient(transport(), 5);
    await expect(slow.request("slow", {})).rejects.toBeInstanceOf(RpcError);
    const broken = new LspClient({ send: () => Promise.reject(new Error("down")) });
    await expect(broken.request("x", {})).rejects.toThrow("down");
    const c = new LspClient(transport());
    const open = c.request("y", {});
    c.close();
    await expect(open).rejects.toBeInstanceOf(RpcError);
    await expect(c.request("z", {})).rejects.toBeInstanceOf(RpcError);
    await c.notify("after", {});
  });
});

describe("[FN-033] LSP conversion", () => {
  test("positions, ranges, markup and markers", () => {
    expect(toLspPosition(3, 5)).toEqual({ line: 2, character: 4 });
    expect(toMonacoRange({ start: { line: 0, character: 0 }, end: { line: 1, character: 2 } })).toEqual({
      startLineNumber: 1,
      startColumn: 1,
      endLineNumber: 2,
      endColumn: 3,
    });
    expect(markupText(undefined)).toBe("");
    expect(markupText("plain")).toBe("plain");
    expect(markupText({ language: "rust", value: "fn a()" })).toBe("```rust\nfn a()\n```");
    expect(markupText({ kind: "markdown", value: "**b**" })).toBe("**b**");
    expect(hoverTexts(["a", { value: "" }, { value: "c" }])).toEqual(["a", "c"]);
    const r = { start: { line: 1, character: 3 }, end: { line: 1, character: 3 } };
    expect(
      markersOfDiagnostics([
        { range: r, severity: 2, message: "w" },
        { range: r, message: "e", code: "E1" },
        { range: r, severity: 4, message: "hint" },
        { range: r, severity: 3, message: "info" },
      ]),
    ).toEqual([
      { severity: "warning", message: "w", startLine: 2, startColumn: 4, endLine: 2, endColumn: 5 },
      { severity: "error", message: "E1: e", startLine: 2, startColumn: 4, endLine: 2, endColumn: 5 },
    ]);
  });
});

/** A bridge whose server is a function from the client's message to its replies. */
function fakeBridge(reply: (m: Record<string, unknown>) => unknown[], opened = true) {
  let handlers: BridgeHandlers | undefined;
  const sent: Record<string, unknown>[] = [];
  const bridge: SessionBridge = {
    open: async (_id, h) => {
      handlers = h;
      if (!opened) {
        throw new Error("the language server did not start");
      }
      return { sessionId: "s", rootUri: "file:///w/f", sessionSeconds: 30 };
    },
    send: async (m) => {
      sent.push(m as Record<string, unknown>);
      queueMicrotask(() => {
        for (const r of reply(m as Record<string, unknown>)) {
          handlers?.onMessage(JSON.stringify(r));
        }
      });
    },
    close: () => undefined,
  };
  return { bridge, sent, closed: (reason: string, detail = "") => handlers?.onClosed(reason, detail) };
}

describe("[FN-033] Rust session", () => {
  const answers = (m: Record<string, unknown>): unknown[] => {
    const id = m["id"];
    switch (m["method"]) {
      case "initialize":
        return [{ jsonrpc: "2.0", id, result: { capabilities: {} } }];
      case "textDocument/completion":
        return [{ jsonrpc: "2.0", id, result: { items: [{ label: "len" }] } }];
      case "textDocument/hover":
        return [{ jsonrpc: "2.0", id, result: null }];
      case "textDocument/signatureHelp":
        return [{ jsonrpc: "2.0", id, result: { signatures: [{ label: "f(a)" }] } }];
      case "textDocument/formatting":
        return [{ jsonrpc: "2.0", id, result: null }];
      case "textDocument/didOpen":
        return [
          {
            jsonrpc: "2.0",
            method: "textDocument/publishDiagnostics",
            params: {
              uri: "file:///w/f/src/lib.rs",
              diagnostics: [
                { range: { start: { line: 0, character: 0 }, end: { line: 0, character: 1 } }, message: "m" },
              ],
            },
          },
          {
            jsonrpc: "2.0",
            method: "textDocument/publishDiagnostics",
            params: { uri: "file:///other.rs", diagnostics: [] },
          },
        ];
      default:
        return [];
    }
  };

  test("initialises, opens the document, relays diagnostics and answers requests", async () => {
    const f = fakeBridge(answers);
    const states: SessionState[] = [];
    const diagnostics: number[] = [];
    const s = new RustSession(f.bridge, {
      onState: (st) => states.push(st),
      onDiagnostics: (d) => diagnostics.push(d.length),
    });
    expect(await s.completion({ line: 0, character: 0 })).toEqual([]);
    await s.start("src", "fn a() {}");
    expect(states.map((x) => x.phase)).toEqual(["starting", "ready"]);
    expect(f.sent.map((m) => m["method"])).toEqual(["initialize", "initialized", "textDocument/didOpen"]);
    expect(f.sent[0]).toMatchObject({ params: { rootUri: "file:///w/f/" } });
    expect(f.sent[2]).toMatchObject({
      params: { textDocument: { uri: "file:///w/f/src/lib.rs", text: "fn a() {}", version: 1 } },
    });
    await Promise.resolve();
    expect(diagnostics).toEqual([1]);
    expect((await s.completion({ line: 0, character: 0 })).map((i) => i.label)).toEqual(["len"]);
    expect(await s.hover({ line: 0, character: 0 })).toBeNull();
    expect((await s.signatureHelp({ line: 0, character: 0 }))?.signatures).toHaveLength(1);
    expect(await s.format()).toEqual([]);
    s.change("fn b() {}");
    expect(f.sent.at(-1)).toMatchObject({
      method: "textDocument/didChange",
      params: { textDocument: { version: 2 }, contentChanges: [{ text: "fn b() {}" }] },
    });
    f.closed("time", "");
    expect(states.at(-1)).toEqual({ phase: "closed", reason: "time", detail: "" });
    expect(await s.hover({ line: 0, character: 0 })).toBeNull();
    s.change("ignored once over");
    s.close();
    expect(states.filter((x) => x.phase === "closed")).toHaveLength(1);
  });

  test("malformed diagnostics are dropped before they reach the editor", async () => {
    const range = { start: { line: 0, character: 0 }, end: { line: 0, character: 1 } };
    const uri = "file:///w/f/src/lib.rs";
    const bad = (m: Record<string, unknown>): unknown[] =>
      m["method"] === "textDocument/didOpen"
        ? [
            { jsonrpc: "2.0", method: "textDocument/publishDiagnostics", params: { uri, diagnostics: null } },
            { jsonrpc: "2.0", method: "textDocument/publishDiagnostics", params: null },
            {
              jsonrpc: "2.0",
              method: "textDocument/publishDiagnostics",
              params: {
                uri,
                diagnostics: [
                  { range, message: "kept", severity: 2, code: "E1" },
                  { range, message: 1 },
                  { message: "no range" },
                  { range: { start: { line: -1, character: 0 }, end: range.end }, message: "negative" },
                  { range: { start: { line: 0.5, character: 0 }, end: range.end }, message: "fraction" },
                  { range, message: "severity", severity: 9 },
                  { range, message: "code", code: {} },
                  null,
                ],
              },
            },
          ]
        : answers(m);
    const f = fakeBridge(bad);
    const got: (readonly unknown[])[] = [];
    const s = new RustSession(f.bridge, { onState: () => undefined, onDiagnostics: (d) => got.push(d) });
    await s.start("src", "");
    await Promise.resolve();
    expect(got).toEqual([[{ range, message: "kept", severity: 2, code: "E1" }]]);
    s.close();
  });

  test("a sandbox that does not start fails the session; stopping reports the client", async () => {
    const f = fakeBridge(answers, false);
    const states: SessionState[] = [];
    const s = new RustSession(f.bridge, { onState: (st) => states.push(st), onDiagnostics: () => undefined });
    await s.start("src", "");
    expect(states.at(-1)).toEqual({ phase: "failed", detail: "the language server did not start" });
    const g = fakeBridge(answers);
    const t: SessionState[] = [];
    const s2 = new RustSession(g.bridge, { onState: (st) => t.push(st), onDiagnostics: () => undefined });
    await s2.start("src", "");
    s2.close();
    expect(t.at(-1)).toEqual({ phase: "closed", reason: "client", detail: "" });
  });
});
