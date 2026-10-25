// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/** One finding of the validator (ADR-0018), reduced to what Studio shows. */
export interface Problem {
  /** Stable code, `PLX-NNNN`. */
  readonly code: string;
  readonly severity: "info" | "warning" | "error";
  /** The document's path in the project layout; empty for a project-level finding. */
  readonly file: string;
  /** JSON Pointer to the value inside the document. */
  readonly path: string;
  readonly message: string;
}

/** A project file handed to the validator; `base64` marks a binary file. */
export interface ProjectFile {
  readonly path: string;
  readonly data: string;
  readonly encoding?: "base64";
}

/** Host to worker. `id` ties a response to its request. */
export type ValidatorRequest =
  | { readonly type: "load"; readonly id: number; readonly files: readonly ProjectFile[] }
  | { readonly type: "validate"; readonly id: number; readonly file: string; readonly data: string }
  | { readonly type: "cancel"; readonly id: number };

/** Worker to host. */
export type ValidatorResponse =
  | { readonly type: "ready" }
  | { readonly type: "result"; readonly id: number; readonly problems: readonly Problem[] }
  | { readonly type: "failure"; readonly id: number; readonly message: string };

/** The validation engine behind the worker: the WebAssembly module, or a stub in tests. */
export interface ValidatorEngine {
  /** Loads the project; returns the project's own diagnostics as JSON. */
  load(files: readonly ProjectFile[]): string;
  /** Validates one page of the loaded project; returns diagnostics as JSON. */
  validatePage(file: string, data: string): string;
}

const SEVERITIES = new Set(["info", "warning", "error"]);

/** Parses the engine's JSON into Problems; throws with the engine's own message on `{error}`. */
export function parseProblems(json: string): Problem[] {
  const v: unknown = JSON.parse(json);
  if (!Array.isArray(v)) {
    const message = (v as { error?: unknown } | null)?.error;
    throw new Error(typeof message === "string" ? message : "unexpected validator output");
  }
  return v.map((d: Record<string, unknown>) => ({
    code: String(d["code"] ?? ""),
    severity: SEVERITIES.has(String(d["severity"])) ? (d["severity"] as Problem["severity"]) : "error",
    file: typeof d["file"] === "string" ? d["file"] : "",
    path: typeof d["path"] === "string" ? d["path"] : "",
    message: String(d["message"] ?? ""),
  }));
}
