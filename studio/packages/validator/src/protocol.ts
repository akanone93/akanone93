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

/**
 * The pure tools the Go module offers beside validation (L-12): `import` reads an OpenAPI or GraphQL source into
 * data sources, `collections` previews the migration between two versions of a plugin's collections (DB-005).
 */
export type ToolName = "import" | "collections";

/** What the PXL language service answers (PXL-007). */
export type PxlOp = "check" | "complete" | "hover" | "signature";

/** Host to worker. `id` ties a response to its request. */
export type ValidatorRequest =
  | { readonly type: "load"; readonly id: number; readonly files: readonly ProjectFile[] }
  | { readonly type: "validate"; readonly id: number; readonly file: string; readonly data: string }
  /** A language-service request; `offset` counts code points, `scope` is the serialised use site. */
  | {
      readonly type: "pxl";
      readonly id: number;
      readonly op: PxlOp;
      readonly scope: string;
      readonly src: string;
      readonly offset: number;
    }
  /** The expression sites of a page of the loaded project, with what each can read. */
  | { readonly type: "sites"; readonly id: number; readonly file: string; readonly data: string }
  /** One of the pure tools; `args` are its text arguments (see {@link ValidatorEngine.tool}). */
  | { readonly type: "tool"; readonly id: number; readonly name: ToolName; readonly args: readonly string[] }
  | { readonly type: "cancel"; readonly id: number };

/** Worker to host. */
export type ValidatorResponse =
  | { readonly type: "ready" }
  | { readonly type: "result"; readonly id: number; readonly problems: readonly Problem[] }
  | { readonly type: "value"; readonly id: number; readonly value: unknown }
  | { readonly type: "failure"; readonly id: number; readonly message: string };

/** The validation engine behind the worker: the WebAssembly module, or a stub in tests. */
export interface ValidatorEngine {
  /** Loads the project; returns the project's own diagnostics as JSON. */
  load(files: readonly ProjectFile[]): string;
  /** Validates one page of the loaded project; returns diagnostics as JSON. */
  validatePage(file: string, data: string): string;
  /** Answers one PXL language-service request as JSON; `offset` counts code points. */
  pxl(op: PxlOp, scope: string, src: string, offset: number): string;
  /** The expression sites of a page as JSON `{sites, envs}`. */
  pxlSites(file: string, data: string): string;
  /**
   * A pure tool as JSON. `import`: `[kind, JSON of [{file, data}]]` gives `{fragment, diagnostics}`;
   * `collections`: `[previous, current, dropped]` as JSON arrays gives diagnostics.
   */
  tool(name: ToolName, args: readonly string[]): string;
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

/** Parses an engine answer that is a value; throws with the engine's own message on `{error}`. */
export function parseValue(json: string): unknown {
  const v: unknown = JSON.parse(json);
  const message = (v as { error?: unknown } | null)?.error;
  if (typeof message === "string") {
    throw new Error(message);
  }
  return v;
}
