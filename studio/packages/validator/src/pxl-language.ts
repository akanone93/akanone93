// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { type LineColumn, offsetAt, positionAt } from "./positions.ts";
import type { PxlCompletionKind, PxlService } from "./pxl-service.ts";

// Monaco's PXL mode (A4.6): a language definition and the service providers as plain data and
// functions, with no import of Monaco. The full-screen editors register `pxlMonarch` and
// `pxlLanguageConfiguration` under `pxlLanguageId` and wrap the providers below in Monaco's
// own provider objects; the shapes follow Monaco's (1-based positions, UTF-16 columns).

export const pxlLanguageId = "pxl";

/** Words PXL reserves (spec Appendix E.1). */
export const pxlKeywords = ["true", "false", "null", "in"] as const;

/** A Monarch tokenizer definition for PXL; register with `monaco.languages.setMonarchTokensProvider`. */
export const pxlMonarch = {
  defaultToken: "",
  keywords: pxlKeywords,
  operators: ["?.", "??", "||", "&&", "==", "!=", "<=", ">=", "<", ">", "+", "-", "*", "/", "%", "!", "?", ":"],
  symbols: /[?.|&=!<>+\-*/%:]+/,
  tokenizer: {
    root: [
      [/[a-zA-Z_][a-zA-Z0-9_]*(?=\s*\()/, { cases: { "@keywords": "keyword", "@default": "support.function" } }],
      [/[a-zA-Z_][a-zA-Z0-9_]*/, { cases: { "@keywords": "keyword", "@default": "identifier" } }],
      [/\d+\.\d+m/, "number.float"],
      [/\d+\.\d+([eE][-+]?\d+)?/, "number.float"],
      [/\d+m/, "number.float"],
      [/\d+/, "number"],
      [/"/, { token: "string.quote", next: "@dq" }],
      [/'/, { token: "string.quote", next: "@sq" }],
      [/[()[\]{}]/, "@brackets"],
      [/@symbols/, { cases: { "@operators": "operator", "@default": "" } }],
      [/[,.]/, "delimiter"],
      [/\s+/, "white"],
    ],
    dq: [
      [/[^\\"]+/, "string"],
      [/\\./, "string.escape"],
      [/"/, { token: "string.quote", next: "@pop" }],
    ],
    sq: [
      [/[^\\']+/, "string"],
      [/\\./, "string.escape"],
      [/'/, { token: "string.quote", next: "@pop" }],
    ],
  },
} as const;

/** The editor behaviour of PXL: brackets and auto-closing pairs. */
export const pxlLanguageConfiguration = {
  brackets: [
    ["(", ")"],
    ["[", "]"],
    ["{", "}"],
  ],
  autoClosingPairs: [
    { open: "(", close: ")" },
    { open: "[", close: "]" },
    { open: "{", close: "}" },
    { open: '"', close: '"', notIn: ["string"] },
    { open: "'", close: "'", notIn: ["string"] },
  ],
  surroundingPairs: [
    { open: "(", close: ")" },
    { open: '"', close: '"' },
    { open: "'", close: "'" },
  ],
} as const;

/** Characters that start completion and signature help. */
export const pxlTriggerCharacters = { completion: ["."], signature: ["(", ","] } as const;

/** Monaco's `MarkerSeverity` values. */
export const markerSeverity = { hint: 1, info: 2, warning: 4, error: 8 } as const;

export interface MonacoRange {
  readonly startLineNumber: number;
  readonly startColumn: number;
  readonly endLineNumber: number;
  readonly endColumn: number;
}

export interface PxlMarker extends MonacoRange {
  readonly severity: number;
  readonly message: string;
  readonly code: string;
  readonly source: "pxl";
}

export interface PxlSuggestion {
  readonly label: string;
  /** Map to Monaco's `CompletionItemKind` in the wrapper. */
  readonly kind: PxlCompletionKind;
  readonly detail?: string;
  readonly insertText: string;
  readonly range: MonacoRange;
}

export interface PxlHoverResult {
  readonly range: MonacoRange;
  /** Markdown lines: the type, then the overloads of a function. */
  readonly contents: readonly string[];
}

export interface PxlSignatureResult {
  readonly signatures: readonly {
    readonly label: string;
    readonly parameters: readonly { readonly label: string }[];
  }[];
  readonly activeSignature: number;
  readonly activeParameter: number;
}

/** The provider functions of one editor: the text and the scope of the expression being edited. */
export interface PxlProviders {
  markers(text: string): Promise<readonly PxlMarker[]>;
  completions(text: string, position: LineColumn): Promise<readonly PxlSuggestion[]>;
  hover(text: string, position: LineColumn): Promise<PxlHoverResult | null>;
  signatureHelp(text: string, position: LineColumn): Promise<PxlSignatureResult | null>;
}

const rangeOf = (text: string, from: number, to: number): MonacoRange => {
  const a = positionAt(text, from);
  const b = positionAt(text, to);
  return { startLineNumber: a.lineNumber, startColumn: a.column, endLineNumber: b.lineNumber, endColumn: b.column };
};

const SEVERITY = { error: markerSeverity.error, warning: markerSeverity.warning, info: markerSeverity.info } as const;

/**
 * Providers over the language service for expressions of one scope. `scope` is read on every
 * call, so an editor whose use site changes needs no new providers; without a scope there is
 * nothing to say.
 */
export function createPxlProviders(service: PxlService, scope: () => string | undefined): PxlProviders {
  return {
    async markers(text) {
      const s = scope();
      if (s === undefined || text === "") {
        return [];
      }
      const { diagnostics } = await service.check(s, text);
      return diagnostics.map((d) => ({
        ...rangeOf(text, d.from, d.to),
        severity: SEVERITY[d.severity],
        message: d.message,
        code: d.code,
        source: "pxl" as const,
      }));
    },
    async completions(text, position) {
      const s = scope();
      if (s === undefined) {
        return [];
      }
      const list = await service.complete(s, text, offsetAt(text, position));
      const range = rangeOf(text, list.from, list.to);
      return list.items.map((i) => ({
        label: i.label,
        kind: i.kind,
        ...(i.detail === undefined ? {} : { detail: i.detail }),
        insertText: i.insert,
        range,
      }));
    },
    async hover(text, position) {
      const s = scope();
      if (s === undefined) {
        return null;
      }
      const h = await service.hover(s, text, offsetAt(text, position));
      if (h === null) {
        return null;
      }
      return {
        range: rangeOf(text, h.from, h.to),
        contents:
          h.signatures.length > 0 ? [`\`${h.type}\``, ...h.signatures.map((x) => `\`${x}\``)] : [`\`${h.type}\``],
      };
    },
    async signatureHelp(text, position) {
      const s = scope();
      if (s === undefined) {
        return null;
      }
      const h = await service.signature(s, text, offsetAt(text, position));
      if (h === null) {
        return null;
      }
      return {
        signatures: h.signatures.map((x) => ({ label: x.label, parameters: x.parameters.map((label) => ({ label })) })),
        activeSignature: h.activeSignature,
        activeParameter: h.activeParameter,
      };
    },
  };
}
