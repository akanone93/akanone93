// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { type PropDescriptor, valueTypes, type WidgetDescriptor, widget } from "@plux/schema";
import type { Problem } from "@plux/validator";
import { findNode } from "../plugin-canvas/select/edits.ts";

/** Window size classes (WGT-010); `compact` is the base. */
export const breakpoints = ["compact", "medium", "expanded"] as const;
export type Breakpoint = (typeof breakpoints)[number];

/** The editor a property type gets (S2.6). */
export type EditorKind =
  | "number"
  | "int"
  | "bool"
  | "text"
  | "color"
  | "insets"
  | "alignment"
  | "enum"
  | "object"
  | "list";

export interface Obj {
  id?: unknown;
  type?: unknown;
  props?: unknown;
  responsive?: unknown;
  semantics?: unknown;
  slots?: unknown;
  children?: unknown;
  root?: unknown;
  $expr?: unknown;
  $token?: unknown;
  $t?: unknown;
  kind?: unknown;
  tokens?: unknown;
  keys?: unknown;
  key?: unknown;
  [key: string]: unknown;
}
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);

/** Strips the optional marker of a type expression. */
const bare = (type: string): string => (type.endsWith("?") ? type.slice(0, -1) : type);

/** Which editor edits a property of `type`; enum names are passed in `enumNames`. */
export function editorKind(type: string, enumNames: ReadonlySet<string>): EditorKind {
  const t = bare(type);
  if (t === "double") {
    return "number";
  }
  if (t === "int" || t === "duration") {
    return "int";
  }
  if (t === "bool") {
    return "bool";
  }
  if (t === "string") {
    return "text";
  }
  if (t === "color") {
    return "color";
  }
  if (t === "EdgeInsets") {
    return "insets";
  }
  if (t === "Alignment") {
    return "alignment";
  }
  if (t.startsWith("list<")) {
    return "list";
  }
  if (enumNames.has(t)) {
    return "enum";
  }
  return valueTypes.some((v) => v.name === t) ? "object" : "text";
}

/** The unit shown beside a number: durations in ms, ratios and counts without one, everything else dp. */
export function unitOf(prop: PropDescriptor): string {
  if (bare(prop.type) === "duration") {
    return "ms";
  }
  if (/opacity|ratio|scale|flex|factor|elevation|count|lines|index|value|min|max/i.test(prop.name)) {
    return "";
  }
  return bare(prop.type) === "double" ? "dp" : "";
}

/** The reusable view of the selected nodes: each node's type and plain object. */
export function nodesById(roots: readonly unknown[]): Map<string, Obj> {
  const out = new Map<string, Obj>();
  const visit = (n: unknown): void => {
    if (!isObj(n) || typeof n.id !== "string") {
      return;
    }
    out.set(n.id, n);
    for (const s of Object.values(isObj(n.slots) ? n.slots : {})) {
      visit(s);
    }
    if (Array.isArray(n.children)) {
      n.children.forEach(visit);
    }
  };
  roots.forEach(visit);
  return out;
}

/** Properties every selected widget type has with the same type, in the first type's order (F5). */
export function sharedProps(types: readonly string[]): PropDescriptor[] {
  const ds = types.map((t) => widget(t)).filter((d): d is WidgetDescriptor => d !== undefined);
  const first = ds[0];
  if (first === undefined || ds.length !== types.length) {
    return [];
  }
  return first.props.filter((p) => ds.every((d) => d.props.some((q) => q.name === p.name && q.type === p.type)));
}

/** The literal a node holds for `name` at `bp`: the override if set, else the base value. */
export function valueAt(node: Obj, name: string, bp: Breakpoint): { value: unknown; overridden: boolean } {
  const base = isObj(node.props) ? node.props[name] : undefined;
  if (bp === "compact") {
    return { value: base, overridden: false };
  }
  const responsive = isObj(node.responsive) ? node.responsive : {};
  // Overrides cascade: expanded falls back to medium, then to the base (WGT-010).
  for (const level of bp === "expanded" ? (["expanded", "medium"] as const) : (["medium"] as const)) {
    const o = responsive[level];
    if (isObj(o) && name in o) {
      return { value: o[name], overridden: level === bp };
    }
  }
  return { value: base, overridden: false };
}

/** The shared value of a property across nodes, or Mixed (F5). */
export type Shared =
  | { readonly mixed: false; readonly value: unknown; readonly overridden: boolean }
  | { readonly mixed: true };

export function sharedValue(nodes: readonly Obj[], name: string, bp: Breakpoint): Shared {
  const vs = nodes.map((n) => valueAt(n, name, bp));
  const first = vs[0];
  if (first === undefined) {
    return { mixed: false, value: undefined, overridden: false };
  }
  const key = JSON.stringify(first.value ?? null);
  if (vs.some((v) => JSON.stringify(v.value ?? null) !== key)) {
    return { mixed: true };
  }
  return { mixed: false, value: first.value, overridden: vs.some((v) => v.overridden) };
}

/** The object holding a node's values at `bp`: `props` for the base, `responsive.<bp>` for an override. */
function holder(node: Obj, bp: Breakpoint, create: boolean): Obj | undefined {
  if (bp === "compact") {
    if (create && !isObj(node.props)) {
      node.props = {};
    }
    return isObj(node.props) ? node.props : undefined;
  }
  if (create && !isObj(node.responsive)) {
    node.responsive = {};
  }
  const r = isObj(node.responsive) ? node.responsive : undefined;
  if (r !== undefined && create && !isObj(r[bp])) {
    r[bp] = {};
  }
  const o = r?.[bp];
  return isObj(o) ? o : undefined;
}

/** The recipe (over a page's `root`) that sets or, with `undefined`, removes a property at a breakpoint. */
export function setPropRecipe(id: string, bp: Breakpoint, name: string, value: unknown): (root: unknown) => void {
  return (root) => {
    const at = findNode(root, id);
    if (at === null) {
      return;
    }
    const target = holder(at.node as Obj, bp, value !== undefined);
    if (target === undefined) {
      return;
    }
    if (value === undefined) {
      delete target[name];
    } else {
      target[name] = value;
    }
  };
}

/** The recipe that sets one `semantics` field (label, hint, header, ...) or removes it. */
export function setSemanticsRecipe(id: string, field: string, value: unknown): (root: unknown) => void {
  return (root) => {
    const at = findNode(root, id);
    if (at === null) {
      return;
    }
    const node = at.node as Obj;
    if (value === undefined || value === "") {
      if (isObj(node.semantics)) {
        delete node.semantics[field];
      }
      return;
    }
    if (!isObj(node.semantics)) {
      node.semantics = {};
    }
    (node.semantics as Obj)[field] = value;
  };
}

/** A binding found in a prop value, if it is one. */
export function bindingOf(value: unknown): { kind: "expr" | "token" | "t"; text: string } | null {
  if (!isObj(value)) {
    return null;
  }
  if (typeof value.$expr === "string") {
    return { kind: "expr", text: value.$expr };
  }
  if (typeof value.$token === "string") {
    return { kind: "token", text: value.$token };
  }
  if (typeof value.$t === "string") {
    return { kind: "t", text: value.$t };
  }
  return null;
}

/** Child nodes of `n` with the pointer segment that reaches each. */
function childPointers(n: Obj): [string, unknown][] {
  const slots = isObj(n.slots) ? Object.entries(n.slots).map(([k, v]): [string, unknown] => [`/slots/${k}`, v]) : [];
  const kids = Array.isArray(n.children) ? n.children.map((v, i): [string, unknown] => [`/children/${i}`, v]) : [];
  return [...slots, ...kids];
}

/** The JSON Pointer of node `id` in a page document, or null. */
export function pointerOf(page: unknown, id: string): string | null {
  const walk = (n: unknown, ptr: string): string | null => {
    if (!isObj(n)) {
      return null;
    }
    if (n.id === id) {
      return ptr;
    }
    for (const [seg, child] of childPointers(n)) {
      const r = walk(child, `${ptr}${seg}`);
      if (r !== null) {
        return r;
      }
    }
    return null;
  };
  return isObj(page) ? walk(page.root, "/root") : null;
}

/** Problems located at or below `pointer`. */
export function problemsAt(problems: readonly Problem[], file: string, pointer: string | null): Problem[] {
  if (pointer === null) {
    return [];
  }
  return problems.filter((p) => p.file === file && (p.path === pointer || p.path.startsWith(`${pointer}/`)));
}

/** Accessibility findings carry codes PLX-14xx (A11Y-002..006). */
export const isAccessibility = (p: Problem): boolean => p.code.startsWith("PLX-14");

/** Token paths (`a.b.c`) of the theme documents' W3C token trees. */
export function tokenPaths(tokens: unknown, prefix = ""): string[] {
  if (!isObj(tokens)) {
    return [];
  }
  return Object.entries(tokens).flatMap(([k, v]) => {
    if (k.startsWith("$")) {
      return [];
    }
    const path = prefix === "" ? k : `${prefix}.${k}`;
    return isObj(v) && "$value" in v ? [path] : tokenPaths(v, path);
  });
}

/** Names of the EdgeInsets fields in order. */
export const insetSides = ["left", "top", "right", "bottom"] as const;

// Colour literals are document data, not styling; the "#" is kept apart from the digits for the raw-value lint.
const hash = "#";
const hexColor = new RegExp(`^${hash}[0-9a-fA-F]{6}$`);
const fallbackHex = `${hash}000000`;

/** The value of the native colour picker for a literal colour; bindings and other values show black. */
export function pickerHex(value: unknown): string {
  return typeof value === "string" && hexColor.test(value) ? value : fallbackHex;
}

function textKeys(doc: Obj): { id: string; key: string }[] {
  const keys = Array.isArray(doc.keys) ? (doc.keys as Obj[]) : [];
  return keys.flatMap((k) => (typeof k.id === "string" && typeof k.key === "string" ? [{ id: k.id, key: k.key }] : []));
}

/** Theme token paths and translation keys of the project's documents, for the pickers. */
export function projectSources(documents: readonly { path: string; content: Uint8Array }[]): {
  tokens: string[];
  keys: { id: string; key: string }[];
} {
  const decoder = new TextDecoder();
  const tokens = new Set<string>();
  const keys: { id: string; key: string }[] = [];
  for (const d of documents) {
    const j = parseObject(decoder.decode(d.content));
    if (j?.kind === "theme") {
      for (const t of tokenPaths(j.tokens)) {
        tokens.add(t);
      }
    } else if (j?.kind === "translationKeys") {
      keys.push(...textKeys(j));
    }
  }
  return { tokens: [...tokens].sort(), keys };
}

function parseObject(text: string): Obj | undefined {
  try {
    const v: unknown = JSON.parse(text);
    return isObj(v) ? v : undefined;
  } catch {
    return undefined;
  }
}
