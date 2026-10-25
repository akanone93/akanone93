// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/** Reads a plugin's documents into screens and navigation edges (read-only). */

export interface Node {
  readonly id: string;
  readonly type?: string;
  readonly props?: { readonly data?: unknown };
  readonly events?: unknown;
  readonly children?: readonly Node[];
  readonly slots?: Readonly<Record<string, Node>>;
}

export interface Screen {
  readonly id: string;
  readonly route: string;
  readonly title: string;
  readonly root: Node | undefined;
  /** Routes this screen navigates to. */
  readonly targets: readonly string[];
}

export interface PluginModel {
  readonly entryPage: string | undefined;
  readonly screens: readonly Screen[];
  /** Page ID to the page IDs it navigates to. */
  readonly edges: ReadonlyMap<string, readonly string[]>;
}

interface DocLike {
  readonly kind: string;
  readonly content: Uint8Array;
}

interface Doc {
  readonly id?: unknown;
  readonly key?: unknown;
  readonly kind?: unknown;
  readonly route?: unknown;
  readonly title?: unknown;
  readonly pageKind?: unknown;
  readonly pages?: unknown;
  readonly entryPage?: unknown;
  readonly root?: unknown;
}

function json(doc: DocLike): Doc | undefined {
  try {
    const v: unknown = JSON.parse(new TextDecoder().decode(doc.content));
    return v !== null && typeof v === "object" ? (v as Doc) : undefined;
  } catch {
    return undefined;
  }
}

/** Every `navigate` step's route below `value`, in document order. */
function navigations(value: unknown, out: string[]): void {
  if (Array.isArray(value)) {
    for (const v of value) {
      navigations(v, out);
    }
  } else if (value !== null && typeof value === "object") {
    const o = value as Record<string, unknown>;
    const route = (o["input"] as { route?: unknown } | undefined)?.route;
    if (o["action"] === "navigate" && typeof route === "string") {
      out.push(route);
    }
    for (const v of Object.values(o)) {
      navigations(v, out);
    }
  }
}

/** The first text below `node` (a title). */
export function firstText(node: Node | undefined): string | undefined {
  if (node === undefined) {
    return undefined;
  }
  if (node.type === "Text" && typeof node.props?.data === "string") {
    return node.props.data;
  }
  for (const child of [...(node.children ?? []), ...Object.values(node.slots ?? {})]) {
    const t = firstText(child);
    if (t !== undefined) {
      return t;
    }
  }
  return undefined;
}

export function readPlugin(documents: readonly DocLike[]): PluginModel {
  const plugin = documents.map(json).find((d) => d?.kind === "plugin");
  const pages = documents
    .filter((d) => d.kind === "page")
    .map(json)
    .filter((d): d is Doc => d !== undefined && typeof d.id === "string");
  const listed = Array.isArray(plugin?.pages) ? (plugin.pages as unknown[]) : [];
  const rank = (id: string) => {
    const i = listed.indexOf(id);
    return i < 0 ? listed.length : i;
  };
  pages.sort((a, b) => rank(String(a.id)) - rank(String(b.id)));
  const screens: Screen[] = pages
    .filter((p) => p.pageKind === undefined || p.pageKind === "screen")
    .map((p) => {
      const root = p.root as Node | undefined;
      const targets: string[] = [];
      navigations(root, targets);
      const route = typeof p.route === "string" ? p.route : String(p.key ?? p.id);
      return { id: String(p.id), route, title: firstText(root) ?? String(p.title ?? route), root, targets };
    });
  const byRoute = new Map(screens.map((s) => [s.route, s.id]));
  const edges = new Map<string, readonly string[]>();
  for (const s of screens) {
    edges.set(
      s.id,
      s.targets.flatMap((r) => byRoute.get(r) ?? []),
    );
  }
  const entry = typeof plugin?.entryPage === "string" ? plugin.entryPage : undefined;
  return { entryPage: entry, screens, edges };
}
