// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Loads the Go validator (WebAssembly) in a worker and times ValidatePage.

import { summarize } from "./scenes";

declare function importScripts(...urls: string[]): void;
declare class Go {
  importObject: WebAssembly.Imports;
  run(instance: WebAssembly.Instance): Promise<void>;
}

interface ProjectFile {
  path: string;
  data: string;
  encoding?: string;
}

const scope = self as unknown as {
  onmessage: ((event: MessageEvent<{ calls: number }>) => void) | null;
  postMessage(message: unknown): void;
  pluxValidatePage?: (files: ProjectFile[], file: string, data: string) => string;
};

function diagnosticsOf(json: string): unknown[] {
  const parsed: unknown = JSON.parse(json);
  if (!Array.isArray(parsed)) {
    throw new Error(`the validator returned ${json.slice(0, 300)}`);
  }
  return parsed;
}

const PAGE = "plugins/welcome/pages/welcome.page.json";

scope.onmessage = async (event) => {
  try {
    const t0 = performance.now();
    importScripts("/wasm_exec.js");
    const go = new Go();
    const { instance } = await WebAssembly.instantiateStreaming(fetch("/validator.wasm"), go.importObject);
    void go.run(instance);
    while (!scope.pluxValidatePage) {
      await new Promise((resolve) => setTimeout(resolve, 5));
    }
    const instantiateMs = performance.now() - t0;
    const validate = scope.pluxValidatePage;
    const files = (await (await fetch("/starter.json")).json()) as ProjectFile[];
    const page = files.find((f) => f.path === PAGE);
    if (!page) {
      throw new Error(`${PAGE} is not in the project`);
    }
    const t1 = performance.now();
    const loadDiagnostics = diagnosticsOf(validate(files, "", ""));
    const loadMs = performance.now() - t1;
    const times: number[] = [];
    let diagnostics = 0;
    for (let i = 0; i < event.data.calls + 1; i++) {
      const t = performance.now();
      const out = validate([], PAGE, page.data);
      times.push(performance.now() - t);
      diagnostics = diagnosticsOf(out).length;
    }
    scope.postMessage({
      type: "result",
      files: files.length,
      instantiateMs,
      loadProjectMs: loadMs,
      projectDiagnostics: loadDiagnostics.length,
      pageDiagnostics: diagnostics,
      coldFirstCallMs: times[0],
      warm: summarize(times.slice(1)),
    });
  } catch (error) {
    scope.postMessage({ type: "error", message: String(error) });
  }
};
