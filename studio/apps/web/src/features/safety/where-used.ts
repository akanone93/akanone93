// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The "where used" index (decisions F13, STU-070–STU-072, STU-076): every reference between the
 * loaded documents of an app — screens navigating to screens in the same or another plugin,
 * nodes using components, documents using assets, and the entry points — derived on the client
 * from the documents Studio has loaded, as the compiler derives its reference graph (SCH-041).
 * Every delete checks it; the server enforces the same rules.
 */

/** One loaded document of the app; `pluginId` is "" for app-level documents. */
export interface LoadedDoc {
  readonly pluginId: string;
  readonly path: string;
  readonly kind: string;
  readonly revision: number;
  readonly json: unknown;
}

export type UseKind = "navigates" | "usesComponent" | "usesAsset" | "usesState" | "entryPage" | "entryRoute";
export type TargetKind = "page" | "component" | "asset" | "state";

/** One reference: the document and JSON pointer that holds it, and what it points at. */
export interface Use {
  readonly kind: UseKind;
  readonly pluginId: string;
  readonly path: string;
  /** The page, component or document the reference sits in, for display and navigation. */
  readonly fromId: string;
  readonly fromTitle: string;
  /** JSON pointer of the referencing value in its document. */
  readonly pointer: string;
  /** JSON pointer of the action step for a navigation; how "remove outgoing link" removes it. */
  readonly step?: string;
  readonly target: { readonly kind: TargetKind; readonly id: string };
  /** The plugin of the target page, for links between plugins. */
  readonly targetPluginId?: string;
}

/** A page as the index knows it. */
export interface PageInfo {
  readonly id: string;
  readonly key: string;
  readonly route: string;
  readonly title: string;
  readonly pluginId: string;
  readonly path: string;
}

export interface WhereUsedIndex {
  readonly uses: readonly Use[];
  readonly pages: ReadonlyMap<string, PageInfo>;
  /** Every reference to a target. */
  usesOf(kind: TargetKind, id: string): Use[];
  /** Links from pages of other plugins into this plugin. */
  incomingToPlugin(pluginId: string): Use[];
  /** Links from this plugin's pages to pages of other plugins. */
  outgoingFromPlugin(pluginId: string): Use[];
}

type Json = Record<string, unknown>;

function isObject(v: unknown): v is Json {
  return v !== null && typeof v === "object" && !Array.isArray(v);
}

function str(v: unknown): string | undefined {
  return typeof v === "string" ? v : undefined;
}

function escapeToken(token: string): string {
  return token.replaceAll("~", "~0").replaceAll("/", "~1");
}

const routeActions = new Set(["navigate", "openDialog", "openBottomSheet"]);

function pageInfo(d: LoadedDoc): PageInfo | undefined {
  if (d.kind !== "page" || !isObject(d.json)) {
    return undefined;
  }
  const id = str(d.json["id"]);
  const key = str(d.json["key"]) ?? "";
  return id === undefined
    ? undefined
    : {
        id,
        key,
        route: str(d.json["route"]) ?? key,
        title: str(d.json["title"]) ?? key,
        pluginId: d.pluginId,
        path: d.path,
      };
}

type UseBase = Pick<Use, "pluginId" | "path" | "fromId" | "fromTitle">;

/** The entry references of a plugin document (entryPage) and of the app document (entryRoute). */
function entryUses(d: LoadedDoc, json: Json, base: UseBase, byRoute: ReadonlyMap<string, PageInfo>): Use[] {
  const entryPage = str(json["entryPage"]);
  if (d.kind === "plugin" && entryPage !== undefined) {
    return [{ ...base, kind: "entryPage", pointer: "/entryPage", target: { kind: "page", id: entryPage } }];
  }
  const entry = d.kind === "app" ? byRoute.get(str(json["entryRoute"]) ?? "") : undefined;
  return entry === undefined
    ? []
    : [
        {
          ...base,
          kind: "entryRoute",
          pointer: "/entryRoute",
          target: { kind: "page", id: entry.id },
          targetPluginId: entry.pluginId,
        },
      ];
}

const stateRef = /\bstate\.([A-Za-z_][A-Za-z0-9_]*)/g;

/** The state entries a string of this object reads or writes in PXL (`state.<name>`), by the entry's name (STA-002). */
function stateUses(value: Json, pointer: string, base: UseBase): Use[] {
  const out: Use[] = [];
  for (const [k, v] of Object.entries(value)) {
    if (typeof v !== "string") {
      continue;
    }
    for (const m of v.matchAll(stateRef)) {
      const name = m[1];
      if (name !== undefined) {
        out.push({
          ...base,
          kind: "usesState",
          pointer: `${pointer}/${escapeToken(k)}`,
          target: { kind: "state", id: name },
          targetPluginId: base.pluginId,
        });
      }
    }
  }
  return out;
}

/** The references one object of a document holds: a route action, a component use, an asset. */
function objectUses(value: Json, pointer: string, base: UseBase, byRoute: ReadonlyMap<string, PageInfo>): Use[] {
  const out: Use[] = [];
  const action = str(value["action"]);
  const input = value["input"];
  const target = isObject(input) ? byRoute.get(str(input["route"]) ?? "") : undefined;
  if (action !== undefined && routeActions.has(action) && target !== undefined) {
    out.push({
      ...base,
      kind: "navigates",
      pointer: `${pointer}/input/route`,
      step: pointer,
      target: { kind: "page", id: target.id },
      targetPluginId: target.pluginId,
    });
  }
  const component = value["component"];
  const componentId = isObject(component) ? str(component["id"]) : undefined;
  if (componentId !== undefined) {
    out.push({
      ...base,
      kind: "usesComponent",
      pointer: `${pointer}/component/id`,
      target: { kind: "component", id: componentId },
    });
  }
  const asset = str(value["$asset"]);
  if (asset !== undefined) {
    out.push({ ...base, kind: "usesAsset", pointer: `${pointer}/$asset`, target: { kind: "asset", id: asset } });
  }
  out.push(...stateUses(value, pointer, base));
  return out;
}

/** Builds the index from the loaded documents of one app. */
export function buildWhereUsed(docs: readonly LoadedDoc[]): WhereUsedIndex {
  const pages = new Map<string, PageInfo>();
  const byRoute = new Map<string, PageInfo>();
  for (const info of docs.map(pageInfo)) {
    if (info !== undefined) {
      pages.set(info.id, info);
      byRoute.set(info.route, info);
    }
  }
  const uses: Use[] = [];
  for (const d of docs) {
    const json = d.json;
    if (!isObject(json)) {
      continue;
    }
    const base: UseBase = {
      pluginId: d.pluginId,
      path: d.path,
      fromId: str(json["id"]) ?? d.path,
      fromTitle: str(json["title"]) ?? str(json["name"]) ?? str(json["key"]) ?? d.path,
    };
    uses.push(...entryUses(d, json, base, byRoute));
    walk(json, "", (value, pointer) => {
      uses.push(...objectUses(value, pointer, base, byRoute));
    });
  }
  const pluginOf = (u: Use) => (u.target.kind === "page" ? pages.get(u.target.id)?.pluginId : undefined);
  return {
    uses,
    pages,
    usesOf: (kind, id) => uses.filter((u) => u.target.kind === kind && u.target.id === id),
    incomingToPlugin: (pluginId) =>
      uses.filter((u) => u.kind === "navigates" && u.pluginId !== pluginId && pluginOf(u) === pluginId),
    outgoingFromPlugin: (pluginId) =>
      uses.filter((u) => u.kind === "navigates" && u.pluginId === pluginId && pluginOf(u) !== pluginId),
  };
}

function walk(value: unknown, pointer: string, visit: (o: Json, pointer: string) => void): void {
  if (Array.isArray(value)) {
    value.forEach((v, i) => {
      walk(v, `${pointer}/${i}`, visit);
    });
    return;
  }
  if (!isObject(value)) {
    return;
  }
  visit(value, pointer);
  for (const [k, v] of Object.entries(value)) {
    walk(v, `${pointer}/${escapeToken(k)}`, visit);
  }
}

/** Why a delete is blocked; an empty list lets it go ahead. */
export interface DeleteCheck {
  /** It is an entry point: choose another entry first (STU-071). */
  readonly entry: boolean;
  /** Links into it from elsewhere (STU-070), or uses of it (STU-072). */
  readonly incoming: readonly Use[];
  /** Links leaving it, for a plugin (STU-070). */
  readonly outgoing: readonly Use[];
}

export function isBlocked(check: DeleteCheck): boolean {
  return check.entry || check.incoming.length > 0 || check.outgoing.length > 0;
}

/** A screen cannot be deleted while it is its plugin's entry or any link enters it (D24). */
export function checkScreenDelete(index: WhereUsedIndex, pageId: string): DeleteCheck {
  const uses = index.usesOf("page", pageId);
  return {
    entry: uses.some((u) => u.kind === "entryPage" || u.kind === "entryRoute"),
    incoming: uses.filter((u) => u.kind === "navigates" && u.fromId !== pageId),
    outgoing: [],
  };
}

/** A plugin cannot be deleted while the app's entry route is one of its screens or any link enters or leaves it (D19). */
export function checkPluginDelete(index: WhereUsedIndex, pluginId: string): DeleteCheck {
  return {
    entry: index.uses.some((u) => u.kind === "entryRoute" && u.targetPluginId === pluginId),
    incoming: index.incomingToPlugin(pluginId),
    outgoing: index.outgoingFromPlugin(pluginId),
  };
}

/** A component or asset cannot be deleted while anything uses it (D26). */
export function checkEntityDelete(index: WhereUsedIndex, kind: "component" | "asset", id: string): DeleteCheck {
  return { entry: false, incoming: index.usesOf(kind, id), outgoing: [] };
}

/** A state entry of a plugin cannot be removed while a PXL expression in that plugin reads it (STU-063, STA-002). */
export function checkStateDelete(index: WhereUsedIndex, pluginId: string, name: string): DeleteCheck {
  return { entry: false, incoming: index.usesOf("state", name).filter((u) => u.pluginId === pluginId), outgoing: [] };
}

/** A generated API or deep link a key or route change affects (D27, STU-076). */
export interface RenameEffect {
  readonly kind: "deepLink" | "generatedRoute" | "generatedComponent";
  readonly before: string;
  readonly after: string;
}

/** lowerCamelCase of a kebab-case route or key, as `plux codegen` names its members (HST-030). */
export function camel(kebab: string): string {
  const [first = "", ...rest] = kebab.split("-");
  return first + rest.map((p) => (p === "" ? "" : p.charAt(0).toUpperCase() + p.slice(1))).join("");
}

/**
 * What a route change breaks outside Studio: deep links into the screen and the route builder of
 * the generated Dart API. References inside the app are by ID and never break.
 */
export function routeChangeEffects(before: string, after: string): RenameEffect[] {
  if (before === after) {
    return [];
  }
  return [
    { kind: "deepLink", before: `/${before}`, after: `/${after}` },
    { kind: "generatedRoute", before: camel(before), after: camel(after) },
  ];
}

/** What a key change of an exported component breaks: its generated Dart member. */
export function componentKeyEffects(before: string, after: string, exported: boolean): RenameEffect[] {
  return exported && before !== after
    ? [{ kind: "generatedComponent", before: camel(before), after: camel(after) }]
    : [];
}
