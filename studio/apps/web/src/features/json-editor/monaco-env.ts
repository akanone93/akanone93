// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { type MonacoWorker, monacoWorkerScriptURL, WORKER_POLICY } from "../../lib/trusted-worker.ts";
import editorWorkerURL from "./monaco-editor-worker.ts?worker&url";
import jsonWorkerURL from "./monaco-json-worker.ts?worker&url";

/**
 * The Trusted Types policy requests Monaco makes for the HTML of its own rendering (lines, the diff view,
 * the screen-reader content). Studio's CSP admits one policy name, `plux-worker`; Monaco's requests under
 * these names are answered with that policy, so the CSP is unchanged. Any other name (for instance the
 * markdown renderer's) is refused, and Monaco's features that need it are switched off in the view.
 */
export const MONACO_POLICY_NAMES: ReadonlySet<string> = new Set([
  "editorViewLayer",
  "domLineBreaksComputer",
  "diffEditorWidget",
  "diffReview",
  "tokenizeToString",
  "richScreenReaderContent",
  "standaloneColorizer",
  "stickyScrollViewLayer",
  "editorGhostText",
]);

export interface PolicyFactory {
  createPolicy(name: string, rules: Record<string, unknown>): unknown;
}

/** Answers a Monaco policy request through the one allowed policy name; any other request gets no policy. */
export function monacoPolicy(name: string, rules: unknown, factory: PolicyFactory | undefined): unknown {
  return MONACO_POLICY_NAMES.has(name)
    ? factory?.createPolicy(WORKER_POLICY, rules as Record<string, unknown>)
    : undefined;
}

interface Environment {
  getWorker(moduleId: string, label: string): Worker;
  createTrustedTypesPolicy(name: string, rules: unknown): unknown;
}

/** Monaco's worker for `label`: the JSON worker for JSON, the editor worker for everything else. */
export function monacoWorker(label: string, dev: boolean): Worker {
  const which: MonacoWorker = label === "json" ? "json" : "editor";
  const url = which === "json" ? jsonWorkerURL : editorWorkerURL;
  return new Worker(monacoWorkerScriptURL(which, url, window, dev), { type: "module" });
}

/**
 * Installs Monaco's environment. Monaco asks for its Trusted Types policies while its modules evaluate, so this
 * runs before its chunk is requested (`lazy-view.tsx`); no import order inside the chunk could guarantee it.
 */
export function installMonacoEnvironment(): void {
  const env: Environment = {
    getWorker: (_moduleId, label) => monacoWorker(label, import.meta.env.DEV),
    createTrustedTypesPolicy: (name, rules) =>
      monacoPolicy(name, rules, (globalThis as { trustedTypes?: PolicyFactory }).trustedTypes),
  };
  (globalThis as { MonacoEnvironment?: Environment }).MonacoEnvironment = env;
}
