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

export interface MonacoViewProps {
  readonly value: string;
  /** The last published version of the document, for the diff view. */
  readonly baseline: string | undefined;
  readonly diff: boolean;
  readonly readOnly: boolean;
  readonly label: string;
  readonly markers: readonly EditorMarker[];
  readonly onChange: (text: string) => void;
}
