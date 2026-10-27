// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { limitDefault } from "@plux/schema";
import { type PageDoc, remapIds, uuidv7 } from "../screens.ts";

/** The clipboard envelope of copy and paste (STU-067, STU-055; F7, ADR-0095): versioned, parsed as untrusted data. */

export const CLIPBOARD_MIME = "application/x-plux+json";
export const CLIPBOARD_VERSION = 1;

export type DependencyKind = "functions" | "translations" | "assets" | "dataSources";

export interface ClipboardSource {
  readonly app: string;
  readonly plugin: string;
}

export interface Envelope {
  readonly format: typeof CLIPBOARD_MIME;
  readonly version: number;
  readonly source: ClipboardSource;
  readonly nodes: readonly unknown[];
  /** The documents the nodes refer to, by ID, per kind; never their content. */
  readonly dependencies: Readonly<Record<DependencyKind, readonly string[]>>;
}

/** The node fields that name a dependency, by kind. */
const REFERENCE_KEYS: Readonly<Record<string, DependencyKind>> = {
  function: "functions",
  functionId: "functions",
  translationKey: "translations",
  asset: "assets",
  assetId: "assets",
  dataSource: "dataSources",
  dataSourceId: "dataSources",
};

/** Field names whose values are never copied in clear (rule book 11.4). */
const SENSITIVE_KEYS = new Set(["secret", "token", "password", "apiKey", "privateKey", "credential"]);

const isRecord = (v: unknown): v is Record<string, unknown> => typeof v === "object" && v !== null && !Array.isArray(v);

function walk(value: unknown, found: Record<DependencyKind, Set<string>>): unknown {
  if (Array.isArray(value)) {
    return value.map((v) => walk(v, found));
  }
  if (!isRecord(value)) {
    return value;
  }
  const out: Record<string, unknown> = {};
  for (const [k, v] of Object.entries(value)) {
    if (SENSITIVE_KEYS.has(k)) {
      continue;
    }
    const kind = REFERENCE_KEYS[k];
    if (kind !== undefined && typeof v === "string") {
      found[kind].add(v);
    }
    out[k] = walk(v, found);
  }
  return out;
}

/** Builds the envelope of the copied nodes: sensitive fields dropped, referenced dependencies listed sorted. */
export function buildEnvelope(source: ClipboardSource, nodes: readonly unknown[]): Envelope {
  const found = {
    functions: new Set<string>(),
    translations: new Set<string>(),
    assets: new Set<string>(),
    dataSources: new Set<string>(),
  };
  const copied = nodes.map((n) => walk(JSON.parse(JSON.stringify(n)), found));
  const sorted = (s: Set<string>) => [...s].sort();
  return {
    format: CLIPBOARD_MIME,
    version: CLIPBOARD_VERSION,
    source,
    nodes: copied,
    dependencies: {
      functions: sorted(found.functions),
      translations: sorted(found.translations),
      assets: sorted(found.assets),
      dataSources: sorted(found.dataSources),
    },
  };
}

export type RefusalReason = "too-large" | "unknown-version" | "malformed" | "invalid";

export type ParseResult =
  | { readonly kind: "envelope"; readonly envelope: Envelope }
  | { readonly kind: "text"; readonly text: string }
  | { readonly kind: "refused"; readonly reason: RefusalReason; readonly detail?: string };

/** Validates the nodes with the Wasm validator; returns the messages of the errors found. */
export type NodeValidator = (nodes: readonly unknown[]) => Promise<readonly string[]>;

export interface ParseOptions {
  readonly maxBytes?: number;
  readonly validate: NodeValidator;
}

const DEPENDENCY_KINDS: readonly DependencyKind[] = ["functions", "translations", "assets", "dataSources"];

function shapeOf(data: Record<string, unknown>): RefusalReason | Envelope {
  if (data["version"] !== CLIPBOARD_VERSION) {
    return "unknown-version";
  }
  const source = data["source"];
  const deps = data["dependencies"];
  if (
    !isRecord(source) ||
    typeof source["app"] !== "string" ||
    typeof source["plugin"] !== "string" ||
    !Array.isArray(data["nodes"]) ||
    data["nodes"].length === 0 ||
    !data["nodes"].every((n) => isRecord(n) && typeof n["id"] === "string" && typeof n["type"] === "string") ||
    !isRecord(deps) ||
    !DEPENDENCY_KINDS.every((k) => Array.isArray(deps[k]) && (deps[k] as unknown[]).every((s) => typeof s === "string"))
  ) {
    return "malformed";
  }
  return {
    format: CLIPBOARD_MIME,
    version: CLIPBOARD_VERSION,
    source: { app: source["app"], plugin: source["plugin"] },
    nodes: data["nodes"],
    dependencies: {
      functions: deps["functions"] as string[],
      translations: deps["translations"] as string[],
      assets: deps["assets"] as string[],
      dataSources: deps["dataSources"] as string[],
    },
  };
}

function tryJson(text: string): unknown {
  try {
    return JSON.parse(text);
  } catch {
    return undefined;
  }
}

/**
 * Parses clipboard text as untrusted data. Over the registry cap it is refused unread; anything that is not
 * an envelope is plain text only; an envelope of an unknown version, a malformed one or one the validator
 * rejects is refused. Nothing is ever interpreted as HTML.
 */
export async function parseClipboard(text: string, options: ParseOptions): Promise<ParseResult> {
  const cap = options.maxBytes ?? limitDefault("studio.clipboardBytes");
  if (new TextEncoder().encode(text).byteLength > cap) {
    return { kind: "refused", reason: "too-large" };
  }
  const data = tryJson(text);
  if (!isRecord(data) || data["format"] !== CLIPBOARD_MIME) {
    return { kind: "text", text };
  }
  const shaped = shapeOf(data);
  if (typeof shaped === "string") {
    return { kind: "refused", reason: shaped };
  }
  const problems = await options.validate(shaped.nodes);
  if (problems.length > 0) {
    return { kind: "refused", reason: "invalid", detail: problems[0] ?? "" };
  }
  return { kind: "envelope", envelope: shaped };
}

/** The nodes of an envelope with a new UUIDv7 for every node and internal references remapped. */
export function freshNodes(envelope: Envelope, newId: () => string = uuidv7): readonly unknown[] {
  const wrapped = remapIds({ nodes: envelope.nodes } as unknown as PageDoc, newId) as unknown as {
    nodes: readonly unknown[];
  };
  return wrapped.nodes;
}

/** What the target lacks, per kind, after a paste across plugins or apps. */
export type DependencyReport = Readonly<Record<DependencyKind, readonly string[]>>;

export interface TargetHas {
  readonly functions: ReadonlySet<string>;
  readonly translations: ReadonlySet<string>;
  readonly assets: ReadonlySet<string>;
  readonly dataSources: ReadonlySet<string>;
}

/** The dependencies the target lacks; empty when pasting inside the same plugin. */
export function missingDependencies(envelope: Envelope, target: ClipboardSource, has: TargetHas): DependencyReport {
  const same = envelope.source.app === target.app && envelope.source.plugin === target.plugin;
  const lacking = (kind: DependencyKind) =>
    same ? [] : envelope.dependencies[kind].filter((id) => !has[kind].has(id));
  return {
    functions: lacking("functions"),
    translations: lacking("translations"),
    assets: lacking("assets"),
    dataSources: lacking("dataSources"),
  };
}

export const hasMissing = (report: DependencyReport): boolean => DEPENDENCY_KINDS.some((k) => report[k].length > 0);
