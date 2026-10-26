// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// STU-123: opening a full-screen overlay takes at most 150 ms after its code is loaded. The overlay is
// the JSON editor, the heaviest one: a full-viewport container over a busy page, Monaco's editor created on
// a document of about 2 000 lines, and the time until the second animation frame after that (the first
// frame that shows the text). Monaco's code is loaded and warmed by one open before the measured runs.

import * as monaco from "monaco-editor";

const RUNS = 21;

(globalThis as { MonacoEnvironment?: unknown }).MonacoEnvironment = {
  getWorker: (_id: string, label: string) =>
    new Worker(label === "json" ? "/json.worker.js" : "/editor.worker.js", { type: "module" }),
};

function document2000Lines(): string {
  const screens = Array.from({ length: 160 }, (_, i) => ({
    id: `screen-${i}`,
    key: `screen_${i}`,
    root: { id: `root-${i}`, type: "Column", children: [{ id: `t-${i}`, type: "Text", props: { data: `Row ${i}` } }] },
  }));
  return JSON.stringify({ id: "bench", kind: "plugin", pages: screens }, null, 2);
}

const frame = (): Promise<number> => new Promise((resolve) => requestAnimationFrame((t) => resolve(t)));

async function openOnce(text: string): Promise<number> {
  const started = performance.now();
  const overlay = document.createElement("div");
  overlay.style.cssText = "position:fixed;inset:0;background:#fff";
  document.body.append(overlay);
  const model = monaco.editor.createModel(text, "json");
  const editor = monaco.editor.create(overlay, {
    model,
    automaticLayout: true,
    minimap: { enabled: false },
    hover: { enabled: "off" },
    quickSuggestions: false,
    ariaLabel: "bench",
  });
  editor.focus();
  await frame();
  await frame();
  const elapsed = performance.now() - started;
  editor.dispose();
  model.dispose();
  overlay.remove();
  return elapsed;
}

async function main(): Promise<void> {
  const busy = document.createElement("div");
  for (let i = 0; i < 400; i++) {
    const row = document.createElement("p");
    row.textContent = `Canvas stand-in row ${i}`;
    busy.append(row);
  }
  document.body.append(busy);
  const text = document2000Lines();
  await openOnce(text);
  await openOnce(text);
  const times: number[] = [];
  for (let i = 0; i < RUNS; i++) {
    times.push(await openOnce(text));
  }
  times.sort((a, b) => a - b);
  const at = (q: number): number => times[Math.min(times.length - 1, Math.ceil(q * times.length) - 1)] ?? 0;
  const result = {
    runs: RUNS,
    lines: text.split("\n").length,
    p50: Number(at(0.5).toFixed(1)),
    p95: Number(at(0.95).toFixed(1)),
    max: Number((times.at(-1) ?? 0).toFixed(1)),
  };
  const out = document.createElement("pre");
  out.id = "result";
  out.textContent = JSON.stringify(result);
  document.body.append(out);
}

main().catch((e: unknown) => {
  const out = document.createElement("pre");
  out.id = "result";
  out.textContent = JSON.stringify({ error: String(e) });
  document.body.append(out);
});
