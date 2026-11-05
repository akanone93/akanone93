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
  /** The page's kind (SCH-022); a page without one is a screen. */
  readonly kind: "screen" | "dialog" | "bottomSheet" | "fullscreenDialog";
  readonly title: string;
  /** The translation key of a translated title (`{"$t": key}`), resolved for the previewed locale by the canvas. */
  readonly titleKey?: string;
  readonly root: Node | undefined;
  /** Routes this screen navigates to. */
  readonly targets: readonly string[];
}

export interface PluginModel {
  /** The plugin's name, or its key when it has none. */
  readonly name: string | undefined;
  readonly entryPage: string | undefined;
  readonly screens: readonly Screen[];
  /** Page ID to the page IDs it navigates to. */
  readonly edges: ReadonlyMap<string, readonly string[]>;
}

interface DocLike {
  readonly kind: string;
  readonly content: Uint8Array;
  /** The content already parsed, e.g. the editor's local state with unsaved edits (STU-121); wins over `content`. */
  readonly parsed?: unknown;
}

/** `content` parsed as JSON, or undefined when it is not JSON. */
export function parseDocument(content: Uint8Array): unknown {
  try {
    return JSON.parse(new TextDecoder().decode(content)) as unknown;
  } catch {
    return undefined;
  }
}

interface Doc {
  readonly id?: unknown;
  readonly key?: unknown;
  readonly name?: unknown;
  readonly kind?: unknown;
  readonly route?: unknown;
  readonly title?: unknown;
  readonly pageKind?: unknown;
  readonly pages?: unknown;
  readonly entryPage?: unknown;
  readonly root?: unknown;
}

function json(doc: DocLike): Doc | undefined {
  const v = doc.parsed !== undefined ? doc.parsed : parseDocument(doc.content);
  return v !== null && typeof v === "object" ? (v as Doc) : undefined;
}

/** The page state each page declares, by page ID: name, type and default of every entry (STA-002, STU-082). */
export function pageStates(
  documents: readonly DocLike[],
): ReadonlyMap<string, readonly { name: string; type: string; default?: unknown }[]> {
  const out = new Map<string, { name: string; type: string; default?: unknown }[]>();
  for (const doc of documents) {
    const page = json(doc) as (Doc & { readonly state?: unknown }) | undefined;
    if (page?.kind !== "page" || typeof page.id !== "string" || !Array.isArray(page.state)) {
      continue;
    }
    const entries: { name: string; type: string; default?: unknown }[] = [];
    for (const e of page.state as unknown[]) {
      const entry = e as { name?: unknown; type?: unknown; default?: unknown } | null;
      if (typeof entry?.name === "string" && typeof entry.type === "string") {
        entries.push({
          name: entry.name,
          type: entry.type,
          ...(entry.default === undefined ? {} : { default: entry.default }),
        });
      }
    }
    out.set(page.id, entries);
  }
  return out;
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

/** The key of a translated title (`{"$t": key}`), or undefined for a plain or missing one. */
export function titleKeyOf(title: unknown): string | undefined {
  if (typeof title !== "object" || title === null) {
    return undefined;
  }
  const key = (title as Record<string, unknown>)["$t"];
  return typeof key === "string" && key !== "" ? key : undefined;
}

/**
 * A screen's name: its title when the title is text; otherwise the first text it shows (its app bar's title); otherwise
 * its key or route. A translated title is a reference (`{"$t": …}`) that only the keys document resolves: it used to
 * print as "[object Object]".
 */
export function screenTitle(page: Readonly<Record<string, unknown>>, root: Node | undefined, route: string): string {
  if (typeof page["title"] === "string" && page["title"].trim() !== "") {
    return page["title"];
  }
  const shown = firstText(root);
  if (shown !== undefined && shown.trim() !== "") {
    return shown;
  }
  return typeof page["key"] === "string" && page["key"] !== "" ? page["key"] : route;
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
      const key = titleKeyOf(p.title);
      return {
        id: String(p.id),
        route,
        kind: "screen" as const,
        title: screenTitle({ ...p }, root, route),
        ...(key === undefined ? {} : { titleKey: key }),
        root,
        targets,
      };
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
  const named = (v: unknown) => (typeof v === "string" && v.trim() !== "" ? v : undefined);
  return { name: named(plugin?.name) ?? named(plugin?.key), entryPage: entry, screens, edges };
}
