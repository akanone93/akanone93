// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { EditorMarker } from "../json-editor/monaco-view.types.ts";
import type { Diagnostic, MarkupContent, Position, Range } from "./lsp-client.ts";

/** Monaco numbers lines and columns from 1, LSP from 0; both count UTF-16 code units. */
export const toLspPosition = (line: number, column: number): Position => ({ line: line - 1, character: column - 1 });

export interface MonacoRange {
  startLineNumber: number;
  startColumn: number;
  endLineNumber: number;
  endColumn: number;
}

export const toMonacoRange = (r: Range): MonacoRange => ({
  startLineNumber: r.start.line + 1,
  startColumn: r.start.character + 1,
  endLineNumber: r.end.line + 1,
  endColumn: r.end.character + 1,
});

/** The text of a hover or documentation field; markup kinds are kept as Markdown (rendered by Monaco, never as HTML). */
export function markupText(content: MarkupContent | undefined): string {
  if (content === undefined) {
    return "";
  }
  if (typeof content === "string") {
    return content;
  }
  return content.language === undefined ? content.value : `\`\`\`${content.language}\n${content.value}\n\`\`\``;
}

/** Hover contents as a list of Markdown strings, empty parts dropped. */
export function hoverTexts(contents: MarkupContent | MarkupContent[]): string[] {
  return (Array.isArray(contents) ? contents : [contents]).map(markupText).filter((t) => t !== "");
}

const isRecord = (v: unknown): v is Record<string, unknown> => typeof v === "object" && v !== null && !Array.isArray(v);

const isPosition = (v: unknown): boolean =>
  isRecord(v) &&
  Number.isSafeInteger(v["line"]) &&
  Number.isSafeInteger(v["character"]) &&
  (v["line"] as number) >= 0 &&
  (v["character"] as number) >= 0;

function isDiagnostic(v: unknown): v is Diagnostic {
  if (!isRecord(v) || typeof v["message"] !== "string") {
    return false;
  }
  const { range, severity, code } = v;
  return (
    isRecord(range) &&
    isPosition(range["start"]) &&
    isPosition(range["end"]) &&
    (severity === undefined || severity === 1 || severity === 2 || severity === 3 || severity === 4) &&
    (code === undefined || typeof code === "string" || typeof code === "number")
  );
}

/**
 * The diagnostics of a `textDocument/publishDiagnostics` notification for `uri`, or undefined when it is about another
 * document or is not one. The language server works on the user's own code, so its output is untrusted: a malformed
 * entry is dropped rather than handed to the editor.
 */
export function publishedDiagnostics(params: unknown, uri: string): Diagnostic[] | undefined {
  if (!isRecord(params) || params["uri"] !== uri || !Array.isArray(params["diagnostics"])) {
    return undefined;
  }
  return params["diagnostics"].filter(isDiagnostic);
}

/** Diagnostics as editor markers; hints and information are shown as warnings only when the server marks them so. */
export function markersOfDiagnostics(diagnostics: readonly Diagnostic[]): EditorMarker[] {
  return diagnostics.flatMap((d) => {
    if (d.severity === 3 || d.severity === 4) {
      return [];
    }
    const r = toMonacoRange(d.range);
    return [
      {
        severity: d.severity === 2 ? ("warning" as const) : ("error" as const),
        message: d.code === undefined ? d.message : `${d.code}: ${d.message}`,
        startLine: r.startLineNumber,
        startColumn: r.startColumn,
        endLine: r.endLineNumber,
        endColumn: Math.max(r.endColumn, r.startColumn + (r.endLineNumber === r.startLineNumber ? 1 : 0)),
      },
    ];
  });
}
