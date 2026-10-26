// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { checkDependency, listDependencies, removeDependency, setDependency } from "./dependencies.ts";
import { type BridgeClient, LanguageBridge } from "./lsp-bridge.ts";
import { starterTemplates } from "./templates.ts";
import { runStoredTests, type TestRunState, type TestsClient } from "./test-runner.ts";

const text = (b: Uint8Array) => new TextDecoder().decode(b);
const bytes = (s: string) => new TextEncoder().encode(s);

/** A server that opens a session, answers `ping` messages and ends when told. */
function fakeServer() {
  const sent: { sequence: bigint; message: string }[] = [];
  const push: (e: Awaited<ReturnType<AsyncIterator<unknown>["next"]>> | undefined) => void = () => {};
  const events: unknown[] = [];
  let wake: (() => void) | undefined;
  const client: BridgeClient = {
    open: (_req, { signal }) => ({
      async *[Symbol.asyncIterator]() {
        yield {
          event: {
            case: "opened",
            value: { sessionId: "s1", rootUri: "file:///workspace/functions/x/", sessionSeconds: 3600 },
          },
        } as const;
        while (!signal.aborted) {
          const next = events.shift();
          if (next === undefined) {
            await new Promise<void>((resolve) => {
              wake = resolve;
              signal.addEventListener("abort", () => resolve(), { once: true });
            });
            continue;
          }
          yield next as never;
          if ((next as { event: { case: string } }).event.case === "closed") {
            return;
          }
        }
      },
    }),
    send: async (req) => {
      sent.push({ sequence: req.sequence, message: text(req.message) });
      if (req.sequence === 2n && failSecond.value) {
        throw new Error("refused");
      }
    },
  };
  const failSecond = { value: false };
  void push;
  return {
    client,
    sent,
    failSecond,
    emit(event: unknown) {
      events.push(event);
      wake?.();
    },
  };
}

describe("FN-033 the language-server bridge", () => {
  test("opens a session, delivers the server's messages and numbers client messages from 1 in order", async () => {
    const server = fakeServer();
    const messages: string[] = [];
    const closed: string[] = [];
    const bridge = new LanguageBridge(server.client);
    const session = await bridge.open("src1", { onMessage: (m) => messages.push(m), onClosed: (r) => closed.push(r) });
    expect(session).toEqual({ sessionId: "s1", rootUri: "file:///workspace/functions/x/", sessionSeconds: 3600 });
    await Promise.all([bridge.send({ id: 1 }), bridge.send({ id: 2 }), bridge.send({ id: 3 })]);
    expect(server.sent.map((s) => [s.sequence, s.message])).toEqual([
      [1n, '{"id":1}'],
      [2n, '{"id":2}'],
      [3n, '{"id":3}'],
    ]);
    server.emit({ event: { case: "message", value: bytes('{"jsonrpc":"2.0","id":1}') } });
    server.emit({ event: { case: "closed", value: { reason: "exited", detail: "" } } });
    await new Promise((r) => setTimeout(r, 20));
    expect(messages).toEqual(['{"jsonrpc":"2.0","id":1}']);
    expect(closed).toEqual(["exited"]);
    await expect(bridge.send({ id: 4 })).rejects.toThrow("not open");
  });

  test("a refused message ends the session, so later messages cannot arrive out of sequence", async () => {
    const server = fakeServer();
    server.failSecond.value = true;
    const bridge = new LanguageBridge(server.client);
    await bridge.open("src1", { onMessage: () => {}, onClosed: () => {} });
    const results = await Promise.allSettled([bridge.send({ id: 1 }), bridge.send({ id: 2 }), bridge.send({ id: 3 })]);
    expect(results.map((r) => r.status)).toEqual(["fulfilled", "rejected", "rejected"]);
    expect(server.sent.map((s) => s.sequence)).toEqual([1n, 2n]);
  });

  test("closing aborts the stream, which stops the sandbox", async () => {
    const server = fakeServer();
    const closed: string[] = [];
    const bridge = new LanguageBridge(server.client);
    await bridge.open("src1", { onMessage: () => {}, onClosed: (r) => closed.push(r) });
    bridge.close();
    await new Promise((r) => setTimeout(r, 20));
    expect(closed).toEqual(["client"]);
  });
});

describe("FN-033 dependency management", () => {
  const manifest = `[package]\nname = "fee"\n\n[dependencies]\nplux-fn = "0.1"\nserde = { version = "1", features = ["derive"] }\n\n[dev-dependencies]\nfoo = "1"\n`;

  test("lists, adds, updates and removes entries of [dependencies] only", () => {
    expect(listDependencies(manifest)).toEqual([
      { name: "plux-fn", version: "0.1" },
      { name: "serde", version: "1" },
    ]);
    const added = setDependency(manifest, { name: "rust_decimal", version: "1.36" });
    expect(listDependencies(added).map((d) => d.name)).toEqual(["plux-fn", "serde", "rust_decimal"]);
    expect(added).toContain('[dev-dependencies]\nfoo = "1"');
    expect(setDependency(added, { name: "plux-fn", version: "0.2" })).toContain('plux-fn = "0.2"');
    expect(removeDependency(added, "rust_decimal")).toBe(manifest);
    expect(setDependency('[package]\nname = "x"\n', { name: "serde", version: "1" })).toBe(
      '[package]\nname = "x"\n\n[dependencies]\nserde = "1"\n',
    );
  });

  test("refuses names and requirements that are not Cargo's", () => {
    expect(checkDependency({ name: "serde", version: "^1.0.2" })).toBeUndefined();
    expect(checkDependency({ name: "se rde", version: "1" })).toBeDefined();
    expect(checkDependency({ name: "serde", version: '1"\nevil = "x' })).toBeDefined();
    expect(() => setDependency(manifest, { name: "a=b", version: "1" })).toThrow();
  });
});

describe("FN-034 the test runner", () => {
  test("reports each stored case as it arrives and the summary last", async () => {
    const client: TestsClient = {
      runTests: () => ({
        async *[Symbol.asyncIterator]() {
          yield { event: { case: "result", value: { name: "a", passed: true, message: "" } } } as const;
          yield { event: { case: "result", value: { name: "b", passed: false, message: "expected 2" } } } as const;
          yield {
            event: { case: "finished", value: { passed: 1, failed: 1, diagnostics: new Uint8Array() } },
          } as const;
        },
      }),
    };
    const states: TestRunState[] = [];
    await runStoredTests(client, "src1", (s) => states.push(s), new AbortController().signal);
    expect(states.map((s) => s.status)).toEqual(["running", "running", "running", "finished"]);
    const last = states.at(-1) as TestRunState;
    expect([last.passed, last.failed, last.results.map((r) => r.name)]).toEqual([1, 1, ["a", "b"]]);
  });

  test("a stream that breaks or ends without a summary is a failed run, not a pass", async () => {
    const broken: TestsClient = {
      runTests: () => ({
        // biome-ignore lint/correctness/useYield: the stream fails before it yields
        async *[Symbol.asyncIterator]() {
          throw new Error("builder unavailable");
        },
      }),
    };
    const states: TestRunState[] = [];
    await runStoredTests(broken, "s", (s) => states.push(s), new AbortController().signal);
    expect(states.at(-1)).toMatchObject({ status: "failed", error: "builder unavailable" });
    const silent: TestsClient = { runTests: () => ({ async *[Symbol.asyncIterator]() {} }) };
    const more: TestRunState[] = [];
    await runStoredTests(silent, "s", (s) => more.push(s), new AbortController().signal);
    expect(more.at(-1)?.status).toBe("failed");
  });
});

describe("FN-033 starter templates", () => {
  test("offers the four starters, each a function with its types and registration", () => {
    expect(starterTemplates.map((t) => t.id)).toEqual([
      "loan.schedule",
      "fee.calculator",
      "eligibility.check",
      "currency.convert",
    ]);
    for (const t of starterTemplates) {
      expect(t.source).toContain("plux_fn::function!(");
      expect(t.source).toContain("derive(Debug, Deserialize, JsonSchema)");
    }
  });
});
