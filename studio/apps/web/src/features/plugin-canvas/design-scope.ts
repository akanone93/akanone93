// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The design-time scope of each page (PC-23, STA-001, SCH-024): what `params`, `page`, `plugin` and `app` hold while
 * the canvas draws a page — each parameter's mock (else its default), each state entry's default (a computed entry has
 * no value until it runs on a device). The canvas binds widgets to these values; the device binds them to real ones.
 */

import { type FormMode, formStatesOf, type ValidatorTexts } from "../panels/forms-model.ts";
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

/** How the canvas previews forms (PC-26): the mode picked per page and form, and the validators' messages. */
export interface FormPreview {
  readonly modes: ReadonlyMap<string, FormMode>;
  readonly text: ValidatorTexts;
}

/** The key of a form's preview mode: its page and its name. */
export const formKey = (page: string, form: string): string => `${page}/${form}`;

/**
 * The design-time scope of every page of `documents`, by page id; with `forms`, each page's forms are in its scope
 * under their names, in the mode previewed (PC-26).
 */
export function designScopes(documents: readonly DocLike[], forms?: FormPreview): ReadonlyMap<string, DesignScope> {
  const app = stateDefaults(jsonOf(documents, "app")[0]?.["state"]);
  const plugin = stateDefaults(jsonOf(documents, "plugin")[0]?.["state"]);
  const out = new Map<string, DesignScope>();
  for (const page of jsonOf(documents, "page")) {
    if (typeof page["id"] === "string") {
      const id = page["id"];
      const own = forms === undefined ? {} : formStatesOf(page, (f) => forms.modes.get(formKey(id, f)), forms.text);
      out.set(id, {
        params: paramMocks(page["params"]),
        page: { ...stateDefaults(page["state"]), ...own },
        plugin,
        app,
      });
    }
  }
  return out;
}
