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
