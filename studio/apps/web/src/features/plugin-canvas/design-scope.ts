// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The design-time scope of each page (PC-23, STA-001, SCH-024): what `params`, `page`, `plugin` and `app` hold while
 * the canvas draws a page — each parameter's mock (else its default), each state entry's default (a computed entry has
 * no value until it runs on a device). The canvas binds widgets to these values; the device binds them to real ones.
 */

import { type DocLike, jsonOf } from "./look.ts";
import type { DesignScope } from "./mock-bindings.ts";

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);

/** The defaults of a list of state entries, by name; computed entries are left out. */
export function stateDefaults(entries: unknown): Obj {
  const out: Obj = {};
  for (const e of Array.isArray(entries) ? entries : []) {
    if (isObj(e) && typeof e["name"] === "string" && e["default"] !== undefined) {
      out[e["name"]] = e["default"];
    }
  }
  return out;
}

/** The design-time values of a page's parameters, by name: the mock, else the default. */
export function paramMocks(params: unknown): Obj {
  const out: Obj = {};
  for (const p of Array.isArray(params) ? params : []) {
    const value = isObj(p) ? (p["mock"] ?? p["default"]) : undefined;
    if (isObj(p) && typeof p["name"] === "string" && value !== undefined) {
      out[p["name"]] = value;
    }
  }
  return out;
}

/** The design-time scope of every page of `documents`, by page id. */
export function designScopes(documents: readonly DocLike[]): ReadonlyMap<string, DesignScope> {
  const app = stateDefaults(jsonOf(documents, "app")[0]?.["state"]);
  const plugin = stateDefaults(jsonOf(documents, "plugin")[0]?.["state"]);
  const out = new Map<string, DesignScope>();
  for (const page of jsonOf(documents, "page")) {
    if (typeof page["id"] === "string") {
      out.set(page["id"], { params: paramMocks(page["params"]), page: stateDefaults(page["state"]), plugin, app });
    }
  }
  return out;
}
