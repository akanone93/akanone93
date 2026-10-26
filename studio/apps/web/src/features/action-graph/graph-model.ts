// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The model of the action graph editor (STU-065, ACT-060): the catalogue of steps a graph can use, the ports
 * of a step, and the recipes that change a graph. Recipes run over a document inside the edit pipeline, so
 * each is one undo step and one JSON Patch (STU-121). Nothing here knows React or React Flow.
 */

import { actions, type InputDescriptor } from "@plux/schema";

export interface Obj {
  [key: string]: unknown;
}
export const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);

/** Where an event's steps live: inline in a page node's event handler, or in an action graph document. */
export type GraphLocation =
  | { readonly kind: "inline"; readonly path: string; readonly nodeId: string; readonly event: string }
  | { readonly kind: "graph"; readonly path: string };

/** One step of a graph as the document holds it (§5). */
export interface Step {
  readonly id: string;
  readonly action: string;
  readonly input?: Obj;
  readonly next?: string;
  readonly onSuccess?: string;
  readonly onError?: string;
  readonly branches?: Readonly<Record<string, string>>;
}

/** An editable value of a step: a path below `input`, its type and what it means. */
export interface StepField {
  readonly path: readonly string[];
  readonly name: string;
  readonly type: string;
  readonly required: boolean;
  readonly description: string;
}

export type CatalogueGroup = "builtin" | "custom" | "function" | "flow";

/** One line of the catalogue: what Add inserts, and how the step then looks. */
export interface CatalogueEntry {
  readonly key: string;
  readonly group: CatalogueGroup;
  readonly label: string;
  readonly description: string;
  /** The action category of a built-in; the group name for the others. */
  readonly category: string;
  /** The `action` of the step. */
  readonly action: string;
  /** Input values the entry fixes, such as the function a function step calls. */
  readonly preset: Obj;
  readonly fields: readonly StepField[];
  readonly output: string | undefined;
  readonly branches: readonly string[];
  readonly branchesFrom: string | undefined;
}

/** A host-registered custom action (ACT-060), as the native catalogue declares it. */
export interface CustomAction {
  readonly name: string;
  readonly description?: string | undefined;
  readonly inputs?: readonly { readonly name: string; readonly type: string; readonly required?: boolean }[];
  readonly output?: string | undefined;
}

/** What the project offers besides the built-in actions. */
export interface Extras {
  readonly custom: readonly CustomAction[];
  /** Functions the plugin may call (FN-012). */
  readonly functions: readonly string[];
  /** The plugin's flows: action graph documents that name no page (ACT-061). */
  readonly flows: readonly string[];
}

export const noExtras: Extras = { custom: [], functions: [], flows: [] };

const fieldOf = (i: InputDescriptor, prefix: readonly string[] = []): StepField => ({
  path: [...prefix, i.name],
  name: i.name,
  type: i.type,
  required: i.required,
  description: i.description ?? "",
});

/** The entry of a built-in action; the inputs the action's wrapper fixes (`function`, `flow`, `action`) are not editable fields. */
function builtinEntry(a: (typeof actions)[number], fixed: string | undefined): CatalogueEntry {
  return {
    key: a.name,
    group: "builtin",
    label: a.name,
    description: a.description,
    category: a.category,
    action: a.name,
    preset: {},
    fields: a.inputs.filter((i) => i.name !== fixed).map((i) => fieldOf(i)),
    output: a.output,
    branches: a.branches,
    branchesFrom: a.branchesFrom,
  };
}

const wrapper = { callNative: "action", invokeFunction: "function", callFlow: "flow" } as const;

/** The catalogue: every built-in action from the registry, then custom actions, functions and flows, each sorted by name. */
export function buildCatalogue(extras: Extras = noExtras): readonly CatalogueEntry[] {
  const builtin = actions.map((a) =>
    builtinEntry(a, a.name in wrapper ? wrapper[a.name as keyof typeof wrapper] : undefined),
  );
  const entry = (name: string) => builtin.find((e) => e.action === name);
  const wrapped = (
    group: Exclude<CatalogueGroup, "builtin">,
    via: keyof typeof wrapper,
    name: string,
    description: string,
    fields: readonly StepField[],
    output: string | undefined,
  ): CatalogueEntry[] => {
    const base = entry(via);
    return base === undefined
      ? []
      : [
          {
            ...base,
            key: `${group}:${name}`,
            group,
            label: name,
            description: description === "" ? base.description : description,
            category: group,
            preset: { [wrapper[via]]: name },
            fields: fields.length > 0 ? fields : base.fields,
            output: output ?? base.output,
          },
        ];
  };
  const sorted = <T>(xs: readonly T[], by: (x: T) => string) =>
    [...xs].sort((a, b) => by(a).localeCompare(by(b), "en"));
  return [
    ...sorted(builtin, (e) => e.label),
    ...sorted(extras.custom, (c) => c.name).flatMap((c) =>
      wrapped(
        "custom",
        "callNative",
        c.name,
        c.description ?? "",
        (c.inputs ?? []).map((i) => ({
          path: ["input", i.name],
          name: i.name,
          type: i.type,
          required: i.required === true,
          description: "",
        })),
        c.output,
      ),
    ),
    ...[...extras.functions].sort().flatMap((f) => wrapped("function", "invokeFunction", f, "", [], undefined)),
    ...[...extras.flows].sort().flatMap((f) => wrapped("flow", "callFlow", f, "", [], undefined)),
  ];
}

/** The catalogue entry a step is, so a custom, function or flow step shows its own name and inputs. */
export function entryOf(step: Step, catalogue: readonly CatalogueEntry[]): CatalogueEntry | undefined {
  const via = step.action in wrapper ? wrapper[step.action as keyof typeof wrapper] : undefined;
  const name = via === undefined ? undefined : step.input?.[via];
  const group = step.action === "callNative" ? "custom" : step.action === "invokeFunction" ? "function" : "flow";
  const own = typeof name === "string" ? catalogue.find((e) => e.key === `${group}:${name}`) : undefined;
  return own ?? catalogue.find((e) => e.group === "builtin" && e.action === step.action);
}

/** An output of a step and where it leads. */
export interface Port {
  /** The edge: `next`, `onSuccess`, `onError`, or the name of a branch. */
  readonly name: string;
  readonly kind: "next" | "onSuccess" | "onError" | "branch";
  /** The type the step produces on this port; empty for ports that carry none. */
  readonly type: string;
}

/**
 * The ports of a step (§5): a step with branches has one per branch, a step with an output continues on
 * `onSuccess`, any other on `next`; every step has `onError`. Branches that the step's list input names
 * (`branchesFrom`) come from the document.
 */
export function portsOf(entry: CatalogueEntry | undefined, step: Step): readonly Port[] {
  const named = branchNames(entry, step);
  const out = entry?.output ?? "";
  const first: Port[] =
    named.length > 0
      ? named.map((name) => ({ name, kind: "branch", type: "" }))
      : [{ name: out === "" ? "next" : "onSuccess", kind: out === "" ? "next" : "onSuccess", type: out }];
  return [...first, { name: "onError", kind: "onError", type: "" }];
}

function branchNames(entry: CatalogueEntry | undefined, step: Step): string[] {
  const named = new Set(entry?.branches ?? []);
  const list = entry?.branchesFrom === undefined ? undefined : step.input?.[entry.branchesFrom];
  for (const b of Array.isArray(list) ? list : []) {
    if (typeof b === "string") {
      named.add(b);
    }
  }
  for (const b of Object.keys(step.branches ?? {})) {
    named.add(b);
  }
  return [...named];
}

/** The step a port leads to, if any. */
export function targetOf(step: Step, port: Port): string | undefined {
  const t = port.kind === "branch" ? step.branches?.[port.name] : step[port.kind];
  return typeof t === "string" ? t : undefined;
}

/** A directed edge between two steps. */
export interface GraphEdge {
  readonly id: string;
  readonly from: string;
  readonly port: string;
  readonly to: string;
}

export function edgesOf(steps: readonly Step[], catalogue: readonly CatalogueEntry[]): readonly GraphEdge[] {
  const ids = new Set(steps.map((s) => s.id));
  return steps.flatMap((s) =>
    portsOf(entryOf(s, catalogue), s).flatMap((p) => {
      const to = targetOf(s, p);
      return to !== undefined && ids.has(to) ? [{ id: `${s.id}:${p.name}`, from: s.id, port: p.name, to }] : [];
    }),
  );
}

/** Reads the steps of a document at `loc`, ignoring what is not a step. */
export function stepsAt(doc: unknown, loc: GraphLocation): readonly Step[] {
  const list = rawSteps(doc, loc);
  return list === undefined
    ? []
    : (list.filter(
        (s): s is Obj => isObj(s) && typeof s["id"] === "string" && typeof s["action"] === "string",
      ) as unknown as Step[]);
}

export function findNodeIn(n: unknown, id: string): Obj | undefined {
  if (!isObj(n)) {
    return undefined;
  }
  if (n["id"] === id) {
    return n;
  }
  const kids: unknown[] = [];
  if (Array.isArray(n["children"])) {
    kids.push(...n["children"]);
  }
  if (isObj(n["slots"])) {
    for (const s of Object.values(n["slots"])) {
      kids.push(...(Array.isArray(s) ? s : [s]));
    }
  }
  for (const k of kids) {
    const f = findNodeIn(k, id);
    if (f !== undefined) {
      return f;
    }
  }
  return undefined;
}

function rawSteps(doc: unknown, loc: GraphLocation): unknown[] | undefined {
  if (!isObj(doc)) {
    return undefined;
  }
  if (loc.kind === "graph") {
    return Array.isArray(doc["steps"]) ? doc["steps"] : undefined;
  }
  const node = findNodeIn(doc["root"], loc.nodeId);
  const handler = node !== undefined && isObj(node["events"]) ? node["events"][loc.event] : undefined;
  return isObj(handler) && Array.isArray(handler["steps"]) ? handler["steps"] : undefined;
}

/** The steps array of `doc` at `loc` for a recipe to change, created when the event has no handler yet. */
function ensureSteps(doc: unknown, loc: GraphLocation): unknown[] | undefined {
  if (!isObj(doc)) {
    return undefined;
  }
  if (loc.kind === "graph") {
    if (!Array.isArray(doc["steps"])) {
      doc["steps"] = [];
    }
    return doc["steps"] as unknown[];
  }
  const node = findNodeIn(doc["root"], loc.nodeId);
  if (node === undefined) {
    return undefined;
  }
  if (!isObj(node["events"])) {
    node["events"] = {};
  }
  const events = node["events"] as Obj;
  const handler = events[loc.event];
  if (!isObj(handler) || !Array.isArray(handler["steps"])) {
    events[loc.event] = { steps: [] };
  }
  return (events[loc.event] as Obj)["steps"] as unknown[];
}

export type Recipe = (doc: unknown) => void;

const withSteps =
  (loc: GraphLocation, change: (steps: Obj[]) => void): Recipe =>
  (doc) => {
    const steps = ensureSteps(doc, loc);
    if (steps !== undefined) {
      change(steps as Obj[]);
    }
  };

/** An identifier no step uses: the action name and the smallest free number (`name` is lowerCamelCase, SCH-002). */
export function freshId(steps: readonly Step[], action: string): string {
  const used = new Set(steps.map((s) => s.id));
  for (let n = 1; ; n += 1) {
    if (!used.has(`${action}${n}`)) {
      return `${action}${n}`;
    }
  }
}

/** Appends a step for `entry` and, when `after` names a step with a free first port, continues from it. */
export function addStepRecipe(
  loc: GraphLocation,
  entry: CatalogueEntry,
  id: string,
  catalogue: readonly CatalogueEntry[],
  after?: string,
): Recipe {
  return withSteps(loc, (steps) => {
    const step: Obj = { id, action: entry.action };
    const required = entry.fields.filter((f) => f.required && f.path.length === 1);
    const input: Obj = { ...entry.preset };
    for (const f of required) {
      const name = f.path[0];
      if (name !== undefined && !(name in input)) {
        input[name] = defaultFor(f.type);
      }
    }
    if (Object.keys(input).length > 0) {
      step["input"] = input;
    }
    const prev = after === undefined ? undefined : steps.find((s) => s["id"] === after);
    steps.push(step);
    if (prev !== undefined) {
      const free = firstFreePort(prev, catalogue);
      if (free !== undefined) {
        connect(prev, free, id);
      }
    }
  });
}

function firstFreePort(prev: Obj, catalogue: readonly CatalogueEntry[]): Port | undefined {
  const step = prev as unknown as Step;
  const ports = portsOf(entryOf(step, catalogue), step).filter((p) => p.kind !== "onError");
  return ports.find((p) => targetOf(step, p) === undefined);
}

/** A neutral value of a type, so a new step starts valid where a required input has an obvious one. */
function defaultFor(type: string): unknown {
  if (type === "string") {
    return "";
  }
  if (type === "bool") {
    return false;
  }
  if (type === "int" || type === "double" || type === "number") {
    return 0;
  }
  return null;
}

function connect(step: Obj, port: Port, to: string | undefined): void {
  if (port.kind === "branch") {
    const branches = isObj(step["branches"]) ? (step["branches"] as Obj) : {};
    if (to === undefined) {
      delete branches[port.name];
    } else {
      branches[port.name] = to;
    }
    if (Object.keys(branches).length === 0) {
      delete step["branches"];
    } else {
      step["branches"] = branches;
    }
    return;
  }
  if (to === undefined) {
    delete step[port.kind];
  } else {
    step[port.kind] = to;
  }
}

/** Points the port of step `from` at `to`; `undefined` removes the edge. */
export function connectRecipe(loc: GraphLocation, from: string, port: Port, to: string | undefined): Recipe {
  return withSteps(loc, (steps) => {
    const step = steps.find((s) => s["id"] === from);
    if (step !== undefined) {
      connect(step, port, to);
    }
  });
}

/** Drops every edge of `step` that leads to `id`. */
function unlink(step: Obj, id: string): void {
  for (const key of ["next", "onSuccess", "onError"]) {
    if (step[key] === id) {
      delete step[key];
    }
  }
  if (isObj(step["branches"])) {
    const b = step["branches"] as Obj;
    for (const [name, to] of Object.entries(b)) {
      if (to === id) {
        delete b[name];
      }
    }
    if (Object.keys(b).length === 0) {
      delete step["branches"];
    }
  }
}

/** Removes a step and every edge that led to it. */
export function removeStepRecipe(loc: GraphLocation, id: string): Recipe {
  return withSteps(loc, (steps) => {
    const at = steps.findIndex((s) => s["id"] === id);
    if (at < 0) {
      return;
    }
    steps.splice(at, 1);
    for (const s of steps) {
      unlink(s, id);
    }
  });
}

/** Makes a step the entry: the first step of a graph is where it starts (§5). */
export function makeEntryRecipe(loc: GraphLocation, id: string): Recipe {
  return withSteps(loc, (steps) => {
    const at = steps.findIndex((s) => s["id"] === id);
    const [moved] = at > 0 ? steps.splice(at, 1) : [];
    if (moved !== undefined) {
      steps.unshift(moved);
    }
  });
}

/** Sets (or, with `undefined`, clears) the value at `path` below a step's `input`. */
export function setInputRecipe(loc: GraphLocation, id: string, path: readonly string[], value: unknown): Recipe {
  return withSteps(loc, (steps) => {
    const step = steps.find((s) => s["id"] === id);
    if (step === undefined || path.length === 0) {
      return;
    }
    if (!isObj(step["input"])) {
      step["input"] = {};
    }
    let at = step["input"] as Obj;
    for (const key of path.slice(0, -1)) {
      if (!isObj(at[key])) {
        at[key] = {};
      }
      at = at[key] as Obj;
    }
    const last = path[path.length - 1] as string;
    if (value === undefined) {
      delete at[last];
    } else {
      at[last] = value;
    }
  });
}

/** The value at `path` below a step's `input`. */
export function inputAt(step: Step, path: readonly string[]): unknown {
  let at: unknown = step.input;
  for (const key of path) {
    at = isObj(at) ? at[key] : undefined;
  }
  return at;
}

/** A PXL binding in an input is `{ "$expr": "…" }`. */
export function exprOf(value: unknown): string | undefined {
  return isObj(value) && typeof value["$expr"] === "string" ? value["$expr"] : undefined;
}

/** The text a literal input shows. */
export function literalText(value: unknown): string {
  if (value === undefined || value === null) {
    return "";
  }
  return typeof value === "string" ? value : JSON.stringify(value);
}

/** The value a literal input's text stands for: text for string types, JSON where it parses for the others. */
export function literalValue(text: string, type: string): unknown {
  if (text === "") {
    return undefined;
  }
  if (type === "string" || type === "string?") {
    return text;
  }
  try {
    return JSON.parse(text) as unknown;
  } catch {
    return text;
  }
}

/** Whether `from` may connect to `to`: not to itself and not into the entry step, which nothing leads to. */
export function canConnect(steps: readonly Step[], from: string, to: string): boolean {
  return from !== to && steps[0]?.id !== to && steps.some((s) => s.id === from) && steps.some((s) => s.id === to);
}

/**
 * Finds where an event's steps live: an inline handler, or the action graph document a `$graph` reference
 * names (by its id or key). Returns undefined when the reference names no document.
 */
export function resolveLocation(
  docs: Readonly<Record<string, unknown>>,
  pagePath: string,
  nodeId: string,
  event: string,
): GraphLocation | undefined {
  const inline: GraphLocation = { kind: "inline", path: pagePath, nodeId, event };
  const node = findNodeIn(isObj(docs[pagePath]) ? (docs[pagePath] as Obj)["root"] : undefined, nodeId);
  const handler = node !== undefined && isObj(node["events"]) ? node["events"][event] : undefined;
  if (!isObj(handler) || typeof handler["$graph"] !== "string") {
    return inline;
  }
  const ref = handler["$graph"];
  const found = Object.entries(docs).find(
    ([, d]) => isObj(d) && d["kind"] === "actionGraph" && (d["id"] === ref || d["key"] === ref),
  );
  return found === undefined ? undefined : { kind: "graph", path: found[0] };
}

/** The JSON Pointer of the steps array of `loc` in `doc`, or null when its node is not in the document. */
export function stepsPointer(
  doc: unknown,
  loc: GraphLocation,
  pointerOf: (page: unknown, id: string) => string | null,
): string | null {
  if (loc.kind === "graph") {
    return "/steps";
  }
  const node = pointerOf(doc, loc.nodeId);
  return node === null ? null : `${node}/events/${loc.event}/steps`;
}

/** Sorts findings into the steps they sit in; findings elsewhere in the handler are listed under the empty key. */
export function groupProblems<P extends { readonly path: string }>(
  problems: readonly P[],
  prefix: string | null,
  steps: readonly Step[],
): ReadonlyMap<string, readonly P[]> {
  const out = new Map<string, P[]>();
  if (prefix === null) {
    return out;
  }
  for (const p of problems) {
    if (p.path !== prefix && !p.path.startsWith(`${prefix}/`)) {
      continue;
    }
    const head = p.path.slice(prefix.length + 1).split("/")[0] ?? "";
    const index = head === "" ? Number.NaN : Number(head);
    const key = Number.isInteger(index) ? (steps[index]?.id ?? "") : "";
    out.set(key, [...(out.get(key) ?? []), p]);
  }
  return out;
}

function grantedFunctions(doc: Obj): string[] {
  const caps = doc["capabilities"];
  const grants = isObj(caps) && Array.isArray(caps["functions"]) ? caps["functions"] : [];
  return grants.flatMap((g) => {
    const name = isObj(g) ? (g["alias"] ?? g["function"]) : undefined;
    return typeof name === "string" ? [name] : [];
  });
}

/** What a project offers besides the built-in actions: its plugin flows (graphs without a page, ACT-061) and the functions its capabilities grant (FN-012). */
export function extrasOf(contents: readonly unknown[], custom: readonly CustomAction[]): Extras {
  const objects = contents.filter(isObj);
  const flows = objects.flatMap((d) =>
    d["kind"] === "actionGraph" && d["page"] === undefined && typeof d["key"] === "string" ? [d["key"]] : [],
  );
  return { custom, functions: [...new Set(objects.flatMap(grantedFunctions))], flows };
}
