// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { codePointsBefore, utf16Index } from "./positions.ts";
import type { PxlOp } from "./protocol.ts";

/** What answers language-service requests: the validator host, or a stub in tests. */
export interface PxlBackend {
  pxl(op: PxlOp, scope: string, src: string, offset: number): Promise<unknown>;
}

/** What an expression at one use site can read: the declared types and the roots (PXL-002). */
export interface PxlScope {
  readonly types?: Readonly<
    Record<string, { readonly enum?: readonly string[]; readonly fields?: Readonly<Record<string, string>> }>
  >;
  readonly roots: Readonly<Record<string, string>>;
  /** Paths a guard around the expression has checked are not null. */
  readonly nonNull?: readonly string[];
  /** Overrides of the registry limits for constant folding; the conformance vectors set them. */
  readonly limits?: {
    readonly budget?: number;
    readonly stringLength?: number;
    readonly collectionSize?: number;
    readonly decimalDigits?: number;
  };
}

/** The scope as the service takes it: JSON, so the module can keep one checked environment per scope. */
export function serializeScope(scope: PxlScope): string {
  return JSON.stringify(scope);
}

export interface PxlDiagnostic {
  readonly code: string;
  readonly severity: "error" | "warning" | "info";
  readonly message: string;
  /** UTF-16 range in the checked text. */
  readonly from: number;
  readonly to: number;
}

export interface PxlAnalysis {
  /** The result type; empty when the expression has errors. */
  readonly type: string;
  readonly diagnostics: readonly PxlDiagnostic[];
}

export type PxlCompletionKind = "variable" | "root" | "field" | "function" | "method" | "keyword";

export interface PxlCompletion {
  readonly label: string;
  readonly kind: PxlCompletionKind;
  readonly detail?: string;
  /** Replaces the typed word. */
  readonly insert: string;
}

export interface PxlCompletions {
  /** UTF-16 range of the word the items replace. */
  readonly from: number;
  readonly to: number;
  readonly items: readonly PxlCompletion[];
}

export interface PxlHover {
  readonly from: number;
  readonly to: number;
  readonly type: string;
  /** The overloads, when the name of a function is hovered. */
  readonly signatures: readonly string[];
}

export interface PxlSignature {
  readonly label: string;
  readonly parameters: readonly string[];
}

export interface PxlSignatureHelp {
  readonly signatures: readonly PxlSignature[];
  readonly activeSignature: number;
  readonly activeParameter: number;
}

const SEVERITIES = new Set(["error", "warning", "info"]);

const isRecord = (v: unknown): v is Record<string, unknown> => typeof v === "object" && v !== null;
const num = (v: unknown): number => (typeof v === "number" ? v : 0);
const str = (v: unknown): string => (typeof v === "string" ? v : "");
const list = (v: unknown): unknown[] => (Array.isArray(v) ? v : []);

/**
 * The PXL language service for editors. Every method takes UTF-16 offsets and returns UTF-16
 * ranges; the module counts code points, and the conversion happens here, once. Both the
 * CodeMirror inline fields and Monaco's PXL mode use this class (A4.6, PXL-008).
 */
export class PxlService {
  readonly #backend: PxlBackend;

  constructor(backend: PxlBackend) {
    this.#backend = backend;
  }

  async check(scope: string, src: string): Promise<PxlAnalysis> {
    const v = await this.#backend.pxl("check", scope, src, 0);
    const r = isRecord(v) ? v : {};
    return {
      type: str(r["type"]),
      diagnostics: list(r["diagnostics"]).map((d) => {
        const x = isRecord(d) ? d : {};
        return {
          code: str(x["code"]),
          severity: SEVERITIES.has(str(x["severity"])) ? (str(x["severity"]) as PxlDiagnostic["severity"]) : "error",
          message: str(x["message"]),
          from: utf16Index(src, num(x["start"])),
          to: utf16Index(src, num(x["end"])),
        };
      }),
    };
  }

  async complete(scope: string, src: string, offset: number): Promise<PxlCompletions> {
    const v = await this.#backend.pxl("complete", scope, src, codePointsBefore(src, offset));
    const r = isRecord(v) ? v : {};
    return {
      from: utf16Index(src, num(r["from"])),
      to: utf16Index(src, num(r["to"])),
      items: list(r["items"]).map((i) => {
        const x = isRecord(i) ? i : {};
        const detail = str(x["detail"]);
        return {
          label: str(x["label"]),
          kind: str(x["kind"]) as PxlCompletionKind,
          ...(detail === "" ? {} : { detail }),
          insert: str(x["insert"]),
        };
      }),
    };
  }

  async hover(scope: string, src: string, offset: number): Promise<PxlHover | null> {
    const v = await this.#backend.pxl("hover", scope, src, codePointsBefore(src, offset));
    if (!isRecord(v)) {
      return null;
    }
    return {
      from: utf16Index(src, num(v["start"])),
      to: utf16Index(src, num(v["end"])),
      type: str(v["type"]),
      signatures: list(v["signatures"]).map(str),
    };
  }

  async signature(scope: string, src: string, offset: number): Promise<PxlSignatureHelp | null> {
    const v = await this.#backend.pxl("signature", scope, src, codePointsBefore(src, offset));
    if (!isRecord(v)) {
      return null;
    }
    return {
      signatures: list(v["signatures"]).map((s) => {
        const x = isRecord(s) ? s : {};
        return { label: str(x["label"]), parameters: list(x["parameters"]).map(str) };
      }),
      activeSignature: num(v["activeSignature"]),
      activeParameter: num(v["activeParameter"]),
    };
  }
}

/** The expression sites of a page: where an expression can be written and what it can read. */
export interface PxlSites {
  readonly sites: readonly { readonly pointer: string; readonly env: number; readonly nonNull?: readonly string[] }[];
  readonly envs: readonly Omit<PxlScope, "nonNull">[];
}

/** The scope of the binding at a JSON Pointer of the page, serialised for the service; undefined when no expression site is there. */
export function scopeAt(sites: PxlSites, pointer: string): string | undefined {
  const site = sites.sites.find((s) => s.pointer === pointer);
  const env = site === undefined ? undefined : sites.envs[site.env];
  if (site === undefined || env === undefined) {
    return undefined;
  }
  return serializeScope({
    ...env,
    ...(site.nonNull === undefined || site.nonNull.length === 0 ? {} : { nonNull: site.nonNull }),
  });
}

/** Reads the answer of `pxlSites`; anything else is no sites. */
export function parseSites(v: unknown): PxlSites {
  const r = isRecord(v) ? v : {};
  return { sites: list(r["sites"]) as PxlSites["sites"], envs: list(r["envs"]) as PxlSites["envs"] };
}
