// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { ValidatorCancelled, ValidatorHost, type ValidatorWorkerLike } from "./host.ts";
import { parseProblems, type ValidatorEngine, type ValidatorRequest, type ValidatorResponse } from "./protocol.ts";
import { serveValidatorWorker, type ValidatorScope } from "./worker.ts";

const diag = JSON.stringify([
  { code: "PLX-0001", severity: "warning", file: "a.page.json", path: "/root", message: "m", cause: "c" },
]);

/** A host wired to a real server over an in-memory channel. */
async function pair(engine: ValidatorEngine) {
  const sent: ValidatorRequest[] = [];
  const scope: ValidatorScope = {
    onmessage: null,
    postMessage: (m) => queueMicrotask(() => worker.onmessage?.({ data: m })),
  };
  const worker: ValidatorWorkerLike = {
    onmessage: null,
    onerror: null,
    postMessage: (m) => {
      sent.push(m);
      scope.onmessage?.({ data: m });
    },
    terminate: () => undefined,
  };
  serveValidatorWorker(scope, engine, () => new Promise((r) => setTimeout(r, 0)));
  const host = new ValidatorHost(worker);
  await new Promise((r) => setTimeout(r, 0)); // the worker says ready
  return { host, sent };
}

const engine = (calls: string[]): ValidatorEngine => ({
  load: () => {
    calls.push("load");
    return "[]";
  },
  validatePage: (file) => {
    calls.push(file);
    return diag;
  },
});

describe("parseProblems", () => {
  test("keeps code, severity, file, path and message", () => {
    expect(parseProblems(diag)).toEqual([
      { code: "PLX-0001", severity: "warning", file: "a.page.json", path: "/root", message: "m" },
    ]);
  });
  test("throws the engine's error text", () => {
    expect(() => parseProblems('{"error":"no project is loaded"}')).toThrow("no project is loaded");
  });
});

describe("ValidatorHost", () => {
  test("loads once and validates a page", async () => {
    const calls: string[] = [];
    const { host } = await pair(engine(calls));
    expect(await host.load([{ path: "app.json", data: "{}" }])).toEqual([]);
    const problems = await host.validatePage("a.page.json", "{}");
    expect(problems[0]?.code).toBe("PLX-0001");
    expect(calls).toEqual(["load", "a.page.json"]);
  });

  test("a newer request for the same page cancels the older one before it runs", async () => {
    const calls: string[] = [];
    const { host, sent } = await pair(engine(calls));
    const first = host.validatePage("a.page.json", "1");
    first.catch(() => undefined);
    const second = host.validatePage("a.page.json", "2");
    await expect(first).rejects.toBeInstanceOf(ValidatorCancelled);
    expect((await second).length).toBe(1);
    expect(calls).toEqual(["a.page.json"]);
    expect(sent.map((m) => m.type)).toEqual(["validate", "cancel", "validate"]);
  });

  test("different pages do not cancel each other", async () => {
    const { host } = await pair(engine([]));
    const results = await Promise.all([host.validatePage("a", "{}"), host.validatePage("b", "{}")]);
    expect(results.map((r) => r.length)).toEqual([1, 1]);
  });

  test("an engine failure rejects only that request", async () => {
    const { host } = await pair({
      load: () => "[]",
      validatePage: () => {
        throw new Error("boom");
      },
    });
    await expect(host.validatePage("a", "{}")).rejects.toThrow("boom");
  });

  test("a worker error and dispose reject what is pending", async () => {
    const stub: ValidatorWorkerLike = {
      onmessage: null,
      onerror: null,
      postMessage: () => undefined,
      terminate: () => undefined,
    };
    const host = new ValidatorHost(stub);
    const a = host.load([]);
    stub.onerror?.({ message: "crash" });
    await expect(a).rejects.toThrow("crash");
    const b = host.validatePage("a", "{}");
    host.dispose();
    await expect(b).rejects.toBeInstanceOf(ValidatorCancelled);
  });

  test("ignores answers it no longer waits for and ready", () => {
    const stub: ValidatorWorkerLike = {
      onmessage: null,
      onerror: null,
      postMessage: () => undefined,
      terminate: () => undefined,
    };
    new ValidatorHost(stub);
    const msgs: ValidatorResponse[] = [{ type: "ready" }, { type: "result", id: 99, problems: [] }];
    for (const data of msgs) {
      stub.onmessage?.({ data });
    }
  });
});

describe("ValidatorHost before ready", () => {
  test("holds requests until ready and drops a superseded one unsent", () => {
    const sent: ValidatorRequest[] = [];
    const stub: ValidatorWorkerLike = {
      onmessage: null,
      onerror: null,
      postMessage: (m) => void sent.push(m),
      terminate: () => undefined,
    };
    const host = new ValidatorHost(stub);
    host.validatePage("a", "1").catch(() => undefined);
    void host.validatePage("a", "2");
    expect(sent).toEqual([]);
    stub.onmessage?.({ data: { type: "ready" } });
    expect(sent).toEqual([{ type: "validate", id: 2, file: "a", data: "2" }]);
  });
});
