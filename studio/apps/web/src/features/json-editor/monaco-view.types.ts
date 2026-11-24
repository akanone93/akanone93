// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/** A finding shown in the text: 1-based lines and columns. */
export interface EditorMarker {
  readonly severity: "error" | "warning";
  readonly message: string;
  readonly startLine: number;
  readonly startColumn: number;
  readonly endLine: number;
  readonly endColumn: number;
}

/** What the JSON editor's buttons ask of the editor (build log row 285). */
export interface EditorCommands {
  /** Collapses every section. */
  foldAll(): void;
  /** Expands every section. */
  unfoldAll(): void;
  /** Collapses every section below the top level, so the document's parts read as an outline. */
  foldToParts(): void;
  /** Indents the document as JSON. */
  format(): void;
  /** Opens the find bar. */
  find(): void;
}

export interface MonacoViewProps {
  readonly value: string;
  /** The last published version of the document, for the diff view. */
  readonly baseline: string | undefined;
  readonly diff: boolean;
  readonly readOnly: boolean;
  readonly label: string;
  readonly markers: readonly EditorMarker[];
  readonly onChange: (text: string) => void;
  /** Filled with the editor's commands while it is shown; null otherwise. */
  readonly commands?: { current: EditorCommands | null };
}
