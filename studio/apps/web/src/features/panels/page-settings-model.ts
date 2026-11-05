// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * A page's own settings (PC-14, SCH-022 to SCH-025): what the Page pane shows and the edits it makes to the page
 * document — title, description, kind, route name (with the links of the plugin that follow it), typed parameters with
 * their defaults and design-time mocks, the result a dialog returns, route options (transition, timeline, guards) and
 * security flags. State, data sources and lifecycle handlers open their own editors.
 */

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);

export const pageKinds = ["screen", "dialog", "bottomSheet", "fullscreenDialog"] as const;
export type PageKindName = (typeof pageKinds)[number];

/** The transitions of `routeOptions.transition` (NAV-010), in the schema's order. */
export const transitions = [
  "platform",
  "fade",
  "slideLeft",
  "slideRight",
  "slideUp",
  "slideDown",
  "scale",
  "sharedAxis",
  "custom",
  "none",
] as const;
export type Transition = (typeof transitions)[number];

/** The lifecycle events of a page (SCH-022), in the order they happen. */
export const lifecycleEvents = ["onInit", "onEnter", "onResume", "onLeave", "onDispose"] as const;

/** The assurance levels a page can require (`requiresAssurance`, SEC-090). */
export const assuranceLevels = ["AL0", "AL1", "AL2", "AL3"] as const;

/** The base types of SCH-010, offered for a parameter or a result; any type expression is accepted. */
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

export interface PageParam {
  readonly name: string;
  readonly type: string;
  readonly required: boolean;
  readonly default?: unknown;
  readonly mock?: unknown;
}

export interface PageSettings {
  readonly id: string;
  readonly key: string;
  readonly route: string;
  /** A literal title, or the key of a translated one. */
  readonly title: { readonly text: string } | { readonly key: string };
  readonly description: string;
  readonly kind: PageKindName;
  readonly params: readonly PageParam[];
  readonly result: string;
  readonly state: number;
  readonly dataSources: number;
  readonly lifecycle: ReadonlySet<string>;
  readonly transition: Transition | undefined;
  readonly timeline: string;
  readonly guards: readonly string[];
  readonly secure: boolean;
  readonly assurance: string | undefined;
}

const str = (v: unknown): string => (typeof v === "string" ? v : "");
const count = (v: unknown): number => (Array.isArray(v) ? v.length : 0);

/** A page document read for the Page pane; undefined for anything that is not a page. */
export function readPage(json: unknown): PageSettings | undefined {
  if (!isObj(json) || json["kind"] !== "page" || typeof json["id"] !== "string") {
    return undefined;
  }
  const title = json["title"];
  const options = isObj(json["routeOptions"]) ? json["routeOptions"] : {};
  const security = isObj(json["security"]) ? json["security"] : {};
  const lifecycle = isObj(json["lifecycle"]) ? json["lifecycle"] : {};
  const kind = pageKinds.find((k) => k === json["pageKind"]) ?? "screen";
  const transition = transitions.find((t) => t === options["transition"]);
  return {
    id: json["id"],
    key: str(json["key"]),
    route: str(json["route"]),
    title: isObj(title) && typeof title["$t"] === "string" ? { key: title["$t"] } : { text: str(title) },
    description: str(json["description"]),
    kind,
    params: (Array.isArray(json["params"]) ? json["params"] : []).filter(isObj).map((p) => ({
      name: str(p["name"]),
      type: str(p["type"]),
      required: p["required"] === true,
      ...(p["default"] === undefined ? {} : { default: p["default"] }),
      ...(p["mock"] === undefined ? {} : { mock: p["mock"] }),
    })),
    result: str(json["result"]),
    state: count(json["state"]),
    dataSources: count(json["dataSources"]),
    lifecycle: new Set(lifecycleEvents.filter((e) => lifecycle[e] !== undefined)),
    transition,
    timeline: str(options["timeline"]),
    guards: (Array.isArray(options["guards"]) ? options["guards"] : []).flatMap((g) =>
      isObj(g) && typeof g["$graph"] === "string" ? [g["$graph"]] : [],
    ),
    secure: security["secure"] === true,
    assurance: assuranceLevels.find((l) => l === security["requiresAssurance"]),
  };
}

const ROUTE = /^[a-z][a-z0-9]*(-[a-z0-9]+)*$/;
const NAME = /^[a-z][a-zA-Z0-9]*$/;
const TYPE = /^[A-Za-z][A-Za-z0-9<>, ?]*$/;

/** Why a route name cannot be used (SCH-025): its form, or another page of the app has it; undefined when it can. */
export function routeProblem(route: string, taken: ReadonlySet<string>, own: string): "form" | "taken" | undefined {
  if (!ROUTE.test(route) || route.length > 100) {
    return "form";
  }
  return route !== own && taken.has(route) ? "taken" : undefined;
}

/** Why a parameter name cannot be used: its form (lowerCamelCase), or another parameter has it. */
export function paramNameProblem(name: string, others: readonly string[]): "form" | "taken" | undefined {
  if (!NAME.test(name) || name.length > 64) {
    return "form";
  }
  return others.includes(name) ? "taken" : undefined;
}

/** Whether `type` is a type expression of SCH-010 (`string`, `decimal?`, `list<Transaction>`). */
export const validType = (type: string): boolean => TYPE.test(type) && type.length <= 256;

/**
 * A default or mock typed in as text, as the value it stands for under `type`: a number for `int` and `double`, a
 * flag for `bool`, JSON for a list, map or object, the text itself otherwise (a `decimal` stays a string, SCH-010).
 * An empty text clears it.
 */
export function valueOfText(type: string, text: string): unknown {
  const t = text.trim();
  if (t === "") {
    return undefined;
  }
  const base = type.replace(/\?$/, "");
  if (base === "int" || base === "double") {
    return numberOf(base, t);
  }
  if (base === "bool") {
    return t === "true" ? true : t === "false" ? false : t;
  }
  return /^(list|map)</.test(base) || t.startsWith("{") || t.startsWith("[") ? jsonOf(t) : text;
}

function numberOf(base: "int" | "double", t: string): unknown {
  const n = Number(t);
  if (!Number.isFinite(n)) {
    return t;
  }
  return base === "int" ? Math.trunc(n) : n;
}

function jsonOf(t: string): unknown {
  try {
    return JSON.parse(t) as unknown;
  } catch {
    return t;
  }
}

/** A default or mock as the text its field shows. */
export function textOfValue(value: unknown): string {
  if (value === undefined || value === null) {
    return "";
  }
  return typeof value === "string" ? value : JSON.stringify(value);
}

/** The recipe that sets (or, with undefined, removes) a top-level field of a page document. */
export function setField(name: string, value: unknown): (draft: Obj) => void {
  return (draft) => {
    if (value === undefined || value === "") {
      delete draft[name];
    } else {
      draft[name] = value;
    }
  };
}

/** The recipe that sets a field of one of the page's objects (`routeOptions`, `security`); an empty object goes. */
export function setNested(object: "routeOptions" | "security", name: string, value: unknown): (draft: Obj) => void {
  return (draft) => {
    const next: Obj = isObj(draft[object]) ? { ...(draft[object] as Obj) } : {};
    if (value === undefined || value === "" || value === false) {
      delete next[name];
    } else {
      next[name] = value;
    }
    if (Object.keys(next).length === 0) {
      delete draft[object];
    } else {
      draft[object] = next;
    }
  };
}

/** The recipe that changes parameter `index` (a field set to undefined is removed). */
export function setParam(index: number, field: keyof PageParam, value: unknown): (draft: Obj) => void {
  return (draft) => {
    const params = Array.isArray(draft["params"]) ? [...(draft["params"] as Obj[])] : [];
    const param = params[index];
    if (!isObj(param)) {
      return;
    }
    const next: Obj = { ...param };
    if (value === undefined || (field === "required" && value === false)) {
      delete next[field];
    } else {
      next[field] = value;
    }
    params[index] = next;
    draft["params"] = params;
  };
}

/** The recipe that adds a string parameter named `name` at the end. */
export function addParam(name: string): (draft: Obj) => void {
  return (draft) => {
    const params = Array.isArray(draft["params"]) ? (draft["params"] as Obj[]) : [];
    draft["params"] = [...params, { name, type: "string" }];
  };
}

/** The recipe that removes parameter `index`. */
export function removeParam(index: number): (draft: Obj) => void {
  return (draft) => {
    const params = Array.isArray(draft["params"]) ? (draft["params"] as Obj[]) : [];
    const next = params.filter((_, i) => i !== index);
    if (next.length === 0) {
      delete draft["params"];
    } else {
      draft["params"] = next;
    }
  };
}

/** The first `param`, `param2`, … no parameter has. */
export function freshParamName(params: readonly PageParam[]): string {
  const taken = new Set(params.map((p) => p.name));
  for (let i = 1; ; i++) {
    const name = i === 1 ? "param" : `param${i}`;
    if (!taken.has(name)) {
      return name;
    }
  }
}

/** The recipe that adds or removes guard graph `id` of the route options. */
export function setGuard(id: string, on: boolean): (draft: Obj) => void {
  return (draft) => {
    const options: Obj = isObj(draft["routeOptions"]) ? { ...(draft["routeOptions"] as Obj) } : {};
    const guards = (Array.isArray(options["guards"]) ? (options["guards"] as Obj[]) : []).filter(
      (g) => g["$graph"] !== id,
    );
    const next = on ? [...guards, { $graph: id }] : guards;
    if (next.length === 0) {
      delete options["guards"];
    } else {
      options["guards"] = next;
    }
    if (Object.keys(options).length === 0) {
      delete draft["routeOptions"];
    } else {
      draft["routeOptions"] = options;
    }
  };
}

/** Points every step under `value` that opens `from` (navigate, openDialog, openBottomSheet) at `to`. */
export function retarget(value: unknown, from: string, to: string): number {
  let changed = 0;
  const walk = (v: unknown) => {
    if (Array.isArray(v)) {
      v.forEach(walk);
    } else if (isObj(v)) {
      const input = v["input"];
      const opens = v["action"] === "navigate" || v["action"] === "openDialog" || v["action"] === "openBottomSheet";
      if (opens && isObj(input) && input["route"] === from) {
        input["route"] = to;
        changed++;
      }
      Object.values(v).forEach(walk);
    }
  };
  walk(value);
  return changed;
}

/** The action graphs of the documents a page can name as a guard: their id and key. */
export function graphsOf(documents: readonly unknown[]): { readonly id: string; readonly key: string }[] {
  return documents.flatMap((d) =>
    isObj(d) && d["kind"] === "actionGraph" && typeof d["id"] === "string"
      ? [{ id: d["id"], key: typeof d["key"] === "string" ? d["key"] : d["id"] }]
      : [],
  );
}
