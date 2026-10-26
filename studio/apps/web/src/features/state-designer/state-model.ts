// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { DocumentDraft } from "../editor/index.ts";

type Obj = Record<string, unknown>;

const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);

/** The scalar base types of a type expression (SCH-010). */
export const baseTypes = [
  "string",
  "int",
  "double",
  "bool",
  "decimal",
  "money",
  "date",
  "dateTime",
  "duration",
  "color",
  "asset",
  "route",
] as const;

/** Where a state entry lives (STA-003); the enum of `common.schema.json#/$defs/persistence`. */
export const persistences = ["memory", "session", "persisted", "secure"] as const;
export type Persistence = (typeof persistences)[number];

export interface StateEntry {
  readonly id: string;
  readonly name: string;
  readonly type: string;
  readonly persistence: Persistence | undefined;
  readonly sensitive: boolean;
  /** The default as JSON text; empty when there is none. */
  readonly defaultText: string;
  readonly computed: boolean;
}

/** The state entries of a plugin document, in document order; malformed ones are skipped. */
export function readState(plugin: unknown): StateEntry[] {
  const list = isObj(plugin) && Array.isArray(plugin["state"]) ? plugin["state"] : [];
  return list.flatMap((e) => {
    if (!isObj(e) || typeof e["id"] !== "string" || typeof e["name"] !== "string" || typeof e["type"] !== "string") {
      return [];
    }
    const persistence = persistences.find((p) => p === e["persistence"]);
    return [
      {
        id: e["id"],
        name: e["name"],
        type: e["type"],
        persistence,
        sensitive: e["sensitive"] === true,
        defaultText: "default" in e ? JSON.stringify(e["default"]) : "",
        computed: "computed" in e,
      },
    ];
  });
}

/** The names of the object types and enums a plugin or the app declares (`types`), for the type picker. */
export function declaredTypes(...docs: readonly unknown[]): string[] {
  const names = new Set<string>();
  for (const d of docs) {
    for (const t of isObj(d) && Array.isArray(d["types"]) ? d["types"] : []) {
      if (isObj(t) && typeof t["name"] === "string") {
        names.add(t["name"]);
      }
    }
  }
  return [...names].sort();
}

export type Container = "single" | "list" | "map";

export interface TypeParts {
  readonly container: Container;
  readonly base: string;
  readonly nullable: boolean;
}

/** Splits a type expression into what the pickers show; undefined for one the pickers cannot express. */
export function parseType(expr: string): TypeParts | undefined {
  const nullable = expr.endsWith("?");
  const bare = nullable ? expr.slice(0, -1) : expr;
  const list = /^list<([A-Za-z][A-Za-z0-9]*)>$/.exec(bare);
  if (list?.[1] !== undefined) {
    return { container: "list", base: list[1], nullable };
  }
  const map = /^map<string, ?([A-Za-z][A-Za-z0-9]*)>$/.exec(bare);
  if (map?.[1] !== undefined) {
    return { container: "map", base: map[1], nullable };
  }
  return /^[A-Za-z][A-Za-z0-9]*$/.test(bare) ? { container: "single", base: bare, nullable } : undefined;
}

/** The type expression of picker choices. */
export function composeType(p: TypeParts): string {
  const core = p.container === "list" ? `list<${p.base}>` : p.container === "map" ? `map<string,${p.base}>` : p.base;
  return p.nullable ? `${core}?` : core;
}

const namePattern = /^[a-z][A-Za-z0-9]*$/;

export type NameProblem = "empty" | "invalid" | "taken";

/** Why `name` cannot name a state entry (lowerCamelCase, unique in the plugin), if it cannot. */
export function nameProblem(name: string, others: readonly string[]): NameProblem | undefined {
  if (name === "") {
    return "empty";
  }
  if (!namePattern.test(name)) {
    return "invalid";
  }
  return others.includes(name) ? "taken" : undefined;
}

/** A name for a new entry that no other entry has. */
export function freshName(taken: readonly string[]): string {
  let n = 1;
  while (taken.includes(`value${n}`)) {
    n++;
  }
  return `value${n}`;
}

/** The JSON value of default text; `undefined` when the text is not JSON, `null` for no default. */
export function parseDefault(text: string): { value: unknown } | null | undefined {
  if (text.trim() === "") {
    return null;
  }
  try {
    return { value: JSON.parse(text) };
  } catch {
    return undefined;
  }
}

const entryAt = (d: DocumentDraft, id: string): Obj | undefined => {
  const list = Array.isArray(d["state"]) ? (d["state"] as unknown[]) : [];
  return list.find((e): e is Obj => isObj(e) && e["id"] === id);
};

/** The recipe that appends an entry to a plugin's `state`. */
export function addEntryRecipe(entry: { id: string; name: string; type: string }): (d: DocumentDraft) => void {
  return (d) => {
    const list = Array.isArray(d["state"]) ? (d["state"] as unknown[]) : [];
    d["state"] = [...list, { ...entry, persistence: "memory" }];
  };
}

/** The recipe that sets one field of an entry, or removes it for `undefined`. */
export function setFieldRecipe(id: string, field: string, value: unknown): (d: DocumentDraft) => void {
  return (d) => {
    const e = entryAt(d, id);
    if (e === undefined) {
      return;
    }
    if (value === undefined) {
      delete e[field];
    } else {
      e[field] = value;
    }
  };
}

/** The recipe that removes an entry. */
export function removeEntryRecipe(id: string): (d: DocumentDraft) => void {
  return (d) => {
    const list = Array.isArray(d["state"]) ? (d["state"] as unknown[]) : [];
    d["state"] = list.filter((e) => !(isObj(e) && e["id"] === id));
  };
}
