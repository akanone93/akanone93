// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import {
  buildEnvelope,
  type ClipboardSource,
  type Envelope,
  freshNodes,
} from "../../plugin-canvas/clipboard/envelope.ts";
import { uuidv7 } from "../../plugin-canvas/screens.ts";

export interface TemplateParameter {
  /** The schema name: lowerCamelCase and unique within the template (`TemplateParameter` in the template schema). */
  readonly name: string;
  readonly type: string;
  /** JSON Pointer from the template's root to the value the parameter fills. */
  readonly path: string;
  /** What the person sees for it, e.g. `Text.text` or `page.amount`; saved as the parameter's description. */
  readonly label?: string;
  /** Read back from a saved template, where the label is the description. */
  readonly description?: string;
  /** The value the template keeps when the inserter gives none. */
  readonly default?: unknown;
}

/** An entry of the page state of the page the selection comes from (`stateEntry`, STA-002). */
export interface PageStateEntry {
  readonly name: string;
  readonly type: string;
  readonly default?: unknown;
}

/** A reference to page state that cannot become a parameter, kept in the template and flagged (STU-082). */
export interface PageStateFlag {
  /** JSON Pointer from the template's root to the binding or the action path. */
  readonly path: string;
  readonly expression: string;
  /** `expression`: part of a larger expression; `undeclared`: no such page state entry; `action`: an action's state path. */
  readonly reason: "expression" | "undeclared" | "action";
}

export interface PageStateScan {
  /** Bare references (`page.amount`) to declared state: each becomes a parameter. */
  readonly parameters: readonly TemplateParameter[];
  readonly flagged: readonly PageStateFlag[];
}

const isRecord = (v: unknown): v is Record<string, unknown> => typeof v === "object" && v !== null && !Array.isArray(v);

/** The root a template saves: the single selected node, or a column holding the selection. */
export function rootOf(nodes: readonly unknown[]): unknown {
  return nodes.length === 1 ? nodes[0] : { id: uuidv7(), type: "Column", children: [...nodes] };
}

const pointerPart = (key: string | number) => String(key).replaceAll("~", "~0").replaceAll("/", "~1");

/** `Text`, `text` -> `textText`: the lowerCamelCase name the template schema requires. */
function camel(...parts: readonly string[]): string {
  const words = parts.join(" ").match(/[A-Za-z0-9]+/g) ?? ["value"];
  const joined = words.map((w, i) => (i === 0 ? w.charAt(0).toLowerCase() : w.charAt(0).toUpperCase()) + w.slice(1));
  const name = joined.join("");
  return /^[a-z]/.test(name) ? name : `value${name}`;
}

/** `base`, else `base2`, `base3`, … the first not in `taken`; the result is added to `taken`. */
function uniqueParameterName(base: string, taken: Set<string>): string {
  let name = base;
  for (let n = 2; taken.has(name); n += 1) {
    name = `${base}${n}`;
  }
  taken.add(name);
  return name;
}

function textProps(node: Record<string, unknown>, path: string, taken: Set<string>): TemplateParameter[] {
  const type = typeof node["type"] === "string" ? node["type"] : "Node";
  const props = isRecord(node["props"]) ? node["props"] : {};
  return Object.entries(props)
    .filter(([, v]) => typeof v === "string")
    .map(([k]) => ({
      name: uniqueParameterName(camel(type, k), taken),
      label: `${type}.${k}`,
      type: "string",
      path: `${path}/props/${pointerPart(k)}`,
    }));
}

/** Text props of the selection that may be exposed as parameters (D28 step 2), by pointer into the root. */
export function candidateParameters(root: unknown, limit = 20): TemplateParameter[] {
  const out: TemplateParameter[] = [];
  const taken = new Set<string>();
  const queue: [unknown, string][] = [[root, ""]];
  for (let next = queue.shift(); next !== undefined && out.length < limit; next = queue.shift()) {
    const [node, path] = next;
    if (isRecord(node)) {
      out.push(...textProps(node, path, taken));
      const children = Array.isArray(node["children"]) ? node["children"] : [];
      queue.push(...children.map((c, i): [unknown, string] => [c, `${path}/children/${i}`]));
    }
  }
  return out.slice(0, limit);
}

export interface TemplateDetails {
  readonly key: string;
  readonly name: string;
  readonly description: string;
  readonly category: string;
  readonly visibility: string;
}

const PAGE_REFERENCE = /\bpage\.[A-Za-z_]/;
const BARE_PAGE_REFERENCE = /^\s*page\.([a-z][A-Za-z0-9]*)\s*$/;

/** A page-state binding as a parameter, or why it is not one. */
function classifyBinding(
  node: Record<string, unknown>,
  expression: string,
  path: string,
  state: readonly PageStateEntry[],
  taken: Set<string>,
): TemplateParameter | PageStateFlag {
  const bare = BARE_PAGE_REFERENCE.exec(expression)?.[1];
  const entry = bare === undefined ? undefined : state.find((e) => e.name === bare);
  if (bare !== undefined && entry !== undefined && Object.keys(node).length === 1) {
    return {
      name: uniqueParameterName(bare, taken),
      label: `page.${bare}`,
      type: entry.type,
      path,
      ...(entry.default === undefined ? {} : { default: entry.default }),
    };
  }
  return { path, expression, reason: bare !== undefined && entry === undefined ? "undeclared" : "expression" };
}

/**
 * The references to page state in a template root (STU-082). A binding that is exactly `page.<name>` of a declared
 * state entry becomes a parameter of the entry's type; every other reference (inside a larger expression, to an
 * undeclared entry, or as the state path of an action) cannot be converted and is flagged.
 */
export function scanPageState(root: unknown, state: readonly PageStateEntry[]): PageStateScan {
  const parameters: TemplateParameter[] = [];
  const flagged: PageStateFlag[] = [];
  const taken = new Set<string>();
  const visitMember = (key: string, value: unknown, at: string): void => {
    if (key === "path" && typeof value === "string" && value.startsWith("page.")) {
      flagged.push({ path: at, expression: value, reason: "action" });
    } else {
      visit(value, at);
    }
  };
  const visitRecord = (node: Record<string, unknown>, path: string): void => {
    const expression = node["$expr"];
    if (typeof expression !== "string" || !PAGE_REFERENCE.test(expression)) {
      for (const [key, value] of Object.entries(node)) {
        visitMember(key, value, `${path}/${pointerPart(key)}`);
      }
      return;
    }
    const found = classifyBinding(node, expression, path, state, taken);
    if ("reason" in found) {
      flagged.push(found);
    } else {
      parameters.push(found);
    }
  };
  const visit = (node: unknown, path: string): void => {
    if (Array.isArray(node)) {
      for (const [i, item] of node.entries()) {
        visit(item, `${path}/${i}`);
      }
    } else if (isRecord(node)) {
      visitRecord(node, path);
    }
  };
  visit(root, "");
  return { parameters, flagged };
}

export interface TemplateDependencies {
  readonly tokens: readonly string[];
  readonly assets: readonly string[];
  readonly components: readonly { readonly id: string; readonly version: number }[];
}

interface DependencySets {
  readonly tokens: Set<string>;
  readonly assets: Set<string>;
  readonly components: Map<string, { id: string; version: number }>;
}

const ASSET_KEYS = new Set(["$asset", "asset", "assetId"]);

/** Records the dependency a node field names, if it names one. */
function collectField(key: string, value: unknown, sets: DependencySets): void {
  if (typeof value !== "string") {
    return;
  }
  if (key === "$token") {
    sets.tokens.add(value);
  } else if (ASSET_KEYS.has(key)) {
    sets.assets.add(value);
  }
}

function collectDependencies(node: unknown, sets: DependencySets): void {
  if (Array.isArray(node)) {
    for (const item of node) {
      collectDependencies(item, sets);
    }
  } else if (isRecord(node)) {
    const ref = node["component"];
    if (isRecord(ref) && typeof ref["id"] === "string" && typeof ref["version"] === "number") {
      sets.components.set(`${ref["id"]}@${ref["version"]}`, { id: ref["id"], version: ref["version"] });
    }
    for (const [key, value] of Object.entries(node)) {
      collectField(key, value, sets);
      collectDependencies(value, sets);
    }
  }
}

/** What the root uses, as IDs and paths and never content (STU-080, STU-082): design tokens, assets and components. */
export function templateDependencies(root: unknown): TemplateDependencies {
  const sets: DependencySets = { tokens: new Set(), assets: new Set(), components: new Map() };
  collectDependencies(root, sets);
  return {
    tokens: [...sets.tokens].sort(),
    assets: [...sets.assets].sort(),
    components: [...sets.components.entries()].sort(([a], [b]) => (a < b ? -1 : 1)).map(([, v]) => v),
  };
}

const parameterOf = (p: TemplateParameter) => ({
  name: p.name,
  type: p.type,
  path: p.path,
  ...(p.label === undefined ? {} : { description: p.label }),
  ...(p.default === undefined ? {} : { default: p.default }),
});

/**
 * The template document D28 saves (STU-080, STU-082). Dependencies are captured as IDs and token paths, never content;
 * convertible page-state bindings are replaced by the state's default and exposed as parameters; the rest is listed
 * under `x-flagged`.
 */
export function buildTemplateDoc(
  details: TemplateDetails,
  parameters: readonly TemplateParameter[],
  nodes: readonly unknown[],
  source: ClipboardSource,
  pageState: readonly PageStateEntry[] = [],
): Record<string, unknown> {
  const root = rootOf(buildEnvelope(source, nodes).nodes);
  const state = scanPageState(root, pageState);
  for (const p of state.parameters) {
    setAt(root, p.path, p.default ?? null);
  }
  return {
    schemaVersion: "1.0.0",
    kind: "template",
    id: uuidv7(),
    key: details.key,
    name: details.name,
    description: details.description,
    category: details.category,
    visibility: details.visibility,
    parameters: [...parameters, ...state.parameters].map(parameterOf),
    dependencies: templateDependencies(root),
    ...(state.flagged.length === 0 ? {} : { "x-flagged": state.flagged }),
    root,
  };
}

function setAt(root: unknown, path: string, value: unknown): void {
  const parts = path
    .split("/")
    .slice(1)
    .map((part) => part.replaceAll("~1", "/").replaceAll("~0", "~"));
  let at: unknown = root;
  for (const part of parts.slice(0, -1)) {
    at = isRecord(at) || Array.isArray(at) ? (at as Record<string, unknown>)[part] : undefined;
  }
  const last = parts.at(-1);
  if ((isRecord(at) || Array.isArray(at)) && last !== undefined) {
    (at as Record<string, unknown>)[last] = value;
  }
}

/** The text a person typed as a value of the parameter's type: numbers and booleans are not strings. */
function typed(type: string, text: string): unknown {
  const base = type.replace(/\?$/, "");
  if ((base === "int" || base === "double") && text.trim() !== "" && Number.isFinite(Number(text))) {
    return Number(text);
  }
  return base === "bool" && (text === "true" || text === "false") ? text === "true" : text;
}

/** What inserting yields: the template's root with the parameters filled and every ID new, copied not linked (STU-083). */
export function instantiateRoot(doc: Record<string, unknown>, args: Readonly<Record<string, string>>): unknown[] {
  const root = structuredClone(doc["root"]);
  const parameters = Array.isArray(doc["parameters"]) ? (doc["parameters"] as TemplateParameter[]) : [];
  for (const p of parameters) {
    const value = args[p.name];
    if (value !== undefined && (value !== "" || p.type === "string")) {
      setAt(root, p.path, typed(p.type, value));
    }
  }
  const envelope = { nodes: [root] } as unknown as Envelope;
  return [...freshNodes(envelope)];
}

/** The envelope the dependency report of a template insert runs on: foreign to every plugin, so all missing show. */
export function templateEnvelope(doc: Record<string, unknown>, nodes: readonly unknown[]): Envelope {
  return buildEnvelope({ app: `template:${String(doc["id"])}`, plugin: "template" }, nodes);
}

/** A component instance node standing in for extracted nodes (D30); `id` is the component's document ID. */
export function instanceNode(componentDocId: string, version: number): Record<string, unknown> {
  return { id: uuidv7(), component: { id: componentDocId, version }, props: {} };
}

/** The component document D30 saves from the selection. */
export function buildComponentDoc(
  details: { readonly key: string; readonly name: string; readonly description: string },
  nodes: readonly unknown[],
  source: ClipboardSource,
): Record<string, unknown> {
  return {
    schemaVersion: "1.0.0",
    kind: "component",
    id: uuidv7(),
    key: details.key,
    name: details.name,
    description: details.description,
    version: 1,
    props: [],
    slots: [],
    root: rootOf(buildEnvelope(source, nodes).nodes),
  };
}

/** D33: the plain nodes an instance becomes: the component's root with the instance's prop overrides and new IDs. */
export function detachedNodes(instance: unknown, componentRoot: unknown): unknown[] {
  const root = structuredClone(componentRoot);
  const overrides = isRecord(instance) && isRecord(instance["props"]) ? instance["props"] : {};
  if (isRecord(root)) {
    root["props"] = { ...(isRecord(root["props"]) ? root["props"] : {}), ...overrides };
  }
  return [...freshNodes({ nodes: [root] } as unknown as Envelope)];
}
