// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import "../build/wasm_exec.js";
import { describe, expect, test } from "bun:test";
import { readdirSync } from "node:fs";
import { join } from "node:path";
import { ValidatorHost, type ValidatorWorkerLike } from "./host.ts";
import { codePointsBefore, offsetAt, positionAt, utf16Index } from "./positions.ts";
import type { ProjectFile, ValidatorEngine } from "./protocol.ts";
import { createPxlProviders, pxlMonarch } from "./pxl-language.ts";
import { type PxlScope, PxlService, parseSites, scopeAt, serializeScope } from "./pxl-service.ts";
import { serveValidatorWorker, type ValidatorScope } from "./worker.ts";

declare class Go {
  importObject: WebAssembly.Imports;
  run(instance: WebAssembly.Instance): Promise<void>;
}

/** The real Go module, instantiated as the worker entry does. */
async function wasmEngine(): Promise<ValidatorEngine> {
  const go = new Go();
  const bytes = await Bun.file(new URL("../build/validator.wasm", import.meta.url)).arrayBuffer();
  const { instance } = await WebAssembly.instantiate(bytes, go.importObject);
  void go.run(instance);
  const g = globalThis as unknown as {
    pluxValidatePage: (files: readonly ProjectFile[], file: string, data: string) => string;
    pluxPxl: (op: string, scope: string, src: string, offset: number) => string;
    pluxPxlSites: (file: string, data: string) => string;
    pluxImport: (kind: string, files: readonly { file: string; data: string }[]) => string;
    pluxCollectionsPreview: (previous: string, current: string, dropped: string) => string;
  };
  return {
    load: (files) => g.pluxValidatePage(files, "", ""),
    validatePage: (file, data) => g.pluxValidatePage([], file, data),
    pxl: (op, scope, src, offset) => g.pluxPxl(op, scope, src, offset),
    pxlSites: (file, data) => g.pluxPxlSites(file, data),
    tool: (name, args) =>
      name === "import"
        ? g.pluxImport(args[0] ?? "", JSON.parse(args[1] ?? "[]"))
        : g.pluxCollectionsPreview(args[0] ?? "[]", args[1] ?? "[]", args[2] ?? ""),
  };
}

/** A host wired through the real worker protocol to the real module. */
async function connect(engine: ValidatorEngine): Promise<ValidatorHost> {
  const scope: ValidatorScope = {
    onmessage: null,
    postMessage: (m) => queueMicrotask(() => worker.onmessage?.({ data: m })),
  };
  const worker: ValidatorWorkerLike = {
    onmessage: null,
    onerror: null,
    postMessage: (m) => scope.onmessage?.({ data: m }),
    terminate: () => undefined,
  };
  serveValidatorWorker(scope, engine, () => new Promise((r) => setTimeout(r, 0)));
  const host = new ValidatorHost(worker);
  await new Promise((r) => setTimeout(r, 0));
  return host;
}

const host = await connect(await wasmEngine());
const service = new PxlService(host);

const loanScope = serializeScope({
  types: { Loan: { fields: { amount: "decimal", months: "int", status: "string?" } } },
  roots: { page: "Loan", items: "list<Loan>", name: "string?" },
});

interface VectorFile {
  readonly env: Pick<PxlScope, "types" | "roots">;
  readonly limits?: PxlScope["limits"];
  readonly cases: readonly {
    readonly name: string;
    readonly expr: string;
    readonly type?: string;
    readonly diagnostics?: readonly { readonly code: string; readonly range: readonly [number, number] }[];
  }[];
}

const vectorDir = join(import.meta.dir, "../../../../schema/testdata/pxl");
const vectorFiles = readdirSync(vectorDir)
  .filter((f) => f.endsWith(".json"))
  .sort();

// Verifies: PXL-007.
describe("the PXL conformance vectors through the worker", () => {
  test("the vectors are there", () => {
    expect(vectorFiles.length).toBeGreaterThan(10);
  });
  for (const file of vectorFiles) {
    test(file, async () => {
      const vectors = (await Bun.file(join(vectorDir, file)).json()) as VectorFile;
      const scope = serializeScope({
        ...vectors.env,
        ...(vectors.limits === undefined ? {} : { limits: vectors.limits }),
      });
      for (const c of vectors.cases) {
        const got = await service.check(scope, c.expr);
        const errors = got.diagnostics
          .filter((d) => d.severity === "error")
          .map((d) => ({ code: d.code, range: [d.from, d.to] }));
        if (c.diagnostics !== undefined && c.diagnostics.length > 0) {
          const want = c.diagnostics.map((d) => ({
            code: d.code,
            range: [utf16Index(c.expr, d.range[0]), utf16Index(c.expr, d.range[1])],
          }));
          expect({ name: c.name, errors }).toEqual({ name: c.name, errors: want });
        } else {
          expect({ name: c.name, errors, type: got.type }).toEqual({ name: c.name, errors: [], type: c.type ?? "" });
        }
      }
    });
  }
});

describe("the PXL language service [PXL-008]", () => {
  test("diagnostics carry UTF-16 ranges, also after astral characters", async () => {
    const src = '"😀" + page.nope';
    const got = await service.check(loanScope, src);
    expect(got.type).toBe("");
    const d = got.diagnostics[0];
    expect(d?.code).toMatch(/^PLX-/);
    expect(src.slice(d?.from, d?.to)).toBe("page.nope");
  });

  test("an expression that compiles has its result type", async () => {
    expect((await service.check(loanScope, "page.months * 2")).type).toBe("int");
  });

  test("completion lists the fields after a dot", async () => {
    const src = "page.mo";
    const list = await service.complete(loanScope, src, src.length);
    expect(list.items.map((i) => i.label)).toEqual(["months"]);
    expect(src.slice(list.from, list.to)).toBe("mo");
    expect(list.items[0]?.detail).toBe("int");
  });

  test("completion offers macro variables and functions", async () => {
    const src = "items.map(row, ro";
    const list = await service.complete(loanScope, src, src.length);
    expect(list.items[0]).toMatchObject({ label: "row", kind: "variable", detail: "Loan" });
    const fns = await service.complete(loanScope, "le", 2);
    expect(fns.items.some((i) => i.label === "len" && i.kind === "function")).toBe(true);
  });

  test("hover shows the type, a function name its overloads", async () => {
    const src = 'len(name ?? "") + page.months';
    const field = await service.hover(loanScope, src, src.indexOf("months") + 2);
    expect(field).toMatchObject({ type: "int" });
    expect(src.slice(field?.from, field?.to)).toBe("page.months");
    const fn = await service.hover(loanScope, src, 1);
    expect(fn?.signatures).toContain("len(s string) int");
    expect(await service.hover(loanScope, "page.", 2)).toBeNull();
  });

  test("signature help names the call and the argument being typed", async () => {
    const src = 'replace("a,b", ';
    const help = await service.signature(loanScope, src, src.length);
    expect(help?.signatures[0]?.label).toStartWith("replace(");
    expect(help?.activeParameter).toBe(1);
    expect(await service.signature(loanScope, "page.months", 3)).toBeNull();
  });

  test("a bad scope rejects the request with the module's message", async () => {
    await expect(service.check("{", "1")).rejects.toThrow("scope");
  });
});

describe("the providers for Monaco [PXL-008]", () => {
  const providers = createPxlProviders(service, () => loanScope);
  test("markers use 1-based line and column", async () => {
    const [marker] = await providers.markers("1 +\npage.nope");
    expect(marker).toMatchObject({
      startLineNumber: 2,
      startColumn: 1,
      endLineNumber: 2,
      endColumn: 10,
      severity: 8,
      source: "pxl",
    });
  });
  test("completions replace the typed word", async () => {
    const items = await providers.completions("page.mo", { lineNumber: 1, column: 8 });
    expect(items[0]).toMatchObject({ label: "months", kind: "field", insertText: "months" });
    expect(items[0]?.range).toEqual({ startLineNumber: 1, startColumn: 6, endLineNumber: 1, endColumn: 8 });
  });
  test("hover and signature help", async () => {
    const hover = await providers.hover("page.months", { lineNumber: 1, column: 3 });
    expect(hover?.contents).toEqual(["`Loan`"]);
    const help = await providers.signatureHelp("len(", { lineNumber: 1, column: 5 });
    expect(help?.signatures[0]?.parameters).toEqual([{ label: "s string" }]);
  });
  test("without a scope there is nothing to say", async () => {
    const none = createPxlProviders(service, () => undefined);
    expect(await none.markers("x")).toEqual([]);
    expect(await none.completions("x", { lineNumber: 1, column: 2 })).toEqual([]);
    expect(await none.hover("x", { lineNumber: 1, column: 1 })).toBeNull();
    expect(await none.signatureHelp("len(", { lineNumber: 1, column: 5 })).toBeNull();
  });
  test("the tokenizer definition lists the keywords", () => {
    expect(pxlMonarch.keywords).toContain("null");
    expect(pxlMonarch.tokenizer.root.length).toBeGreaterThan(5);
  });
});

describe("positions", () => {
  test("code points and UTF-16 units convert both ways", () => {
    const s = "a😀b";
    expect(codePointsBefore(s, 3)).toBe(2);
    expect(codePointsBefore(s, 1)).toBe(1);
    expect(codePointsBefore(s, 99)).toBe(3);
    expect(utf16Index(s, 2)).toBe(3);
    expect(utf16Index(s, 99)).toBe(4);
    expect(utf16Index("a\ud83d", 2)).toBe(2);
  });
  test("lines and columns convert both ways", () => {
    const s = "ab\ncd";
    expect(positionAt(s, 4)).toEqual({ lineNumber: 2, column: 2 });
    expect(offsetAt(s, { lineNumber: 2, column: 2 })).toBe(4);
    expect(offsetAt(s, { lineNumber: 9, column: 1 })).toBe(5);
    expect(offsetAt(s, { lineNumber: 1, column: 99 })).toBe(2);
  });
});

describe("scopes of a page's expression sites", () => {
  const sites = parseSites({
    sites: [
      { pointer: "/a", env: 0 },
      { pointer: "/b", env: 0, nonNull: ["page.status"] },
      { pointer: "/c", env: 7 },
    ],
    envs: [{ roots: { page: "int" } }],
  });
  test("a site reads its environment and the paths its guard checked", () => {
    expect(JSON.parse(scopeAt(sites, "/a") ?? "{}")).toEqual({ roots: { page: "int" } });
    expect(JSON.parse(scopeAt(sites, "/b") ?? "{}")).toEqual({ roots: { page: "int" }, nonNull: ["page.status"] });
  });
  test("an unknown site or environment has no scope", () => {
    expect(scopeAt(sites, "/c")).toBeUndefined();
    expect(scopeAt(sites, "/nope")).toBeUndefined();
    expect(parseSites(null)).toEqual({ sites: [], envs: [] });
  });
});
