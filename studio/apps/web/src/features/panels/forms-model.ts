// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Forms at design time (PC-26, STA-020, ADR-0047): a page's forms, their fields and validators as Studio edits them;
 * the state a form is previewed in — pristine, showing its errors, filled with valid values, or submitting — written
 * into the page scope under the form's name (`page.<form>.values`, `.errors`, `.dirty`, `.touched`, `.status`,
 * `.valid`), so widgets bound to it draw that state; and the bindings that tie an input widget to a field.
 */

import { findNode } from "../plugin-canvas/select/edits.ts";

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);
const num = (v: unknown): number | undefined => (typeof v === "number" && Number.isFinite(v) ? v : undefined);

export const VALIDATOR_KINDS = [
  "required",
  "length",
  "range",
  "regex",
  "email",
  "phone",
  "iban",
  "dateRange",
  "decimalPrecision",
  "custom",
  "async",
] as const;
export type ValidatorKind = (typeof VALIDATOR_KINDS)[number];

/** The options each validator kind takes (ADR-0047). */
export const VALIDATOR_OPTIONS: Readonly<Record<ValidatorKind, readonly string[]>> = {
  required: [],
  length: ["min", "max"],
  range: ["min", "max"],
  regex: ["pattern"],
  email: [],
  phone: ["region"],
  iban: [],
  dateRange: ["min", "max"],
  decimalPrecision: ["maxScale", "maxIntegerDigits"],
  custom: ["rule"],
  async: ["debounceMs"],
};

export const FIELD_TYPES = ["string", "int?", "double?", "bool", "date?", "decimal?"] as const;

/** The states a form is previewed in on the canvas. */
export const FORM_MODES = ["pristine", "errors", "valid", "submitting"] as const;
export type FormMode = (typeof FORM_MODES)[number];

export interface FormField {
  readonly name: string;
  readonly type: string;
  readonly initial?: unknown;
  readonly validators: readonly Obj[];
}

export interface Form {
  readonly id: string;
  readonly name: string;
  readonly fields: readonly FormField[];
}

function fieldOf(x: unknown): FormField[] {
  if (!isObj(x) || typeof x["name"] !== "string") {
    return [];
  }
  return [
    {
      name: x["name"],
      type: String(x["type"] ?? "string"),
      ...(x["initial"] === undefined ? {} : { initial: x["initial"] }),
      validators: Array.isArray(x["validators"]) ? x["validators"].filter(isObj) : [],
    },
  ];
}

function formOf(f: unknown): Form[] {
  if (!isObj(f) || typeof f["name"] !== "string") {
    return [];
  }
  const fields = Array.isArray(f["fields"]) ? f["fields"].flatMap(fieldOf) : [];
  return [{ id: String(f["id"] ?? f["name"]), name: f["name"], fields }];
}

/** The forms a page declares. */
export function formsOf(page: unknown): Form[] {
  return isObj(page) && Array.isArray(page["forms"]) ? page["forms"].flatMap(formOf) : [];
}

/** The built-in messages of the validators, in the Studio's language (the runtime's until P8 localises them). */
export interface ValidatorTexts {
  readonly required: string;
  readonly length: (min: number | undefined, max: number | undefined) => string;
  readonly range: (min: number | undefined, max: number | undefined) => string;
  readonly invalid: string;
  readonly email: string;
  readonly phone: string;
  readonly iban: string;
}

const EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
const E164 = /^\+[1-9]\d{6,14}$/;

/** Whether `iban` passes the ISO 13616 mod-97 check. */
export function ibanValid(iban: string): boolean {
  const s = iban.replace(/\s+/g, "").toUpperCase();
  if (!/^[A-Z]{2}\d{2}[A-Z0-9]{10,30}$/.test(s)) {
    return false;
  }
  const moved = `${s.slice(4)}${s.slice(0, 4)}`.replace(/[A-Z]/g, (c) => String(c.charCodeAt(0) - 55));
  let rest = 0;
  for (const d of moved) {
    rest = (rest * 10 + Number(d)) % 97;
  }
  return rest === 1;
}

const messageOf = (v: Obj, fallback: string): string => (typeof v["message"] === "string" ? v["message"] : fallback);
const empty = (value: unknown): boolean => value === undefined || value === null || value === "";

type Check = (v: Obj, value: unknown, text: string, t: ValidatorTexts) => string | undefined;

const outside = (n: number, v: Obj): boolean => n < (num(v["min"]) ?? -Infinity) || n > (num(v["max"]) ?? Infinity);

/** Each built-in check that runs at design time; `custom`, `async` and the date and decimal checks run on a device. */
const CHECKS: Readonly<Record<string, Check>> = {
  required: (v, value, _text, t) => (empty(value) ? messageOf(v, t.required) : undefined),
  length: (v, _value, text, t) =>
    text !== "" && outside([...text].length, v) ? messageOf(v, t.length(num(v["min"]), num(v["max"]))) : undefined,
  range: (v, value, _text, t) =>
    typeof value === "number" && outside(value, v) ? messageOf(v, t.range(num(v["min"]), num(v["max"]))) : undefined,
  regex: (v, _value, text, t) =>
    text !== "" && typeof v["pattern"] === "string" && !safeTest(v["pattern"], text)
      ? messageOf(v, t.invalid)
      : undefined,
  email: (v, _value, text, t) => (text !== "" && !EMAIL.test(text) ? messageOf(v, t.email) : undefined),
  phone: (v, _value, text, t) =>
    text !== "" && !E164.test(text.replace(/[\s()-]/g, "")) ? messageOf(v, t.phone) : undefined,
  iban: (v, _value, text, t) => (text !== "" && !ibanValid(text) ? messageOf(v, t.iban) : undefined),
};

/** The message `v` gives `value`; undefined when it passes (or only a device can say). */
function failing(v: Obj, value: unknown, t: ValidatorTexts): string | undefined {
  return CHECKS[String(v["kind"])]?.(v, value, typeof value === "string" ? value : "", t);
}

function safeTest(pattern: string, text: string): boolean {
  try {
    return new RegExp(pattern, "u").test(text);
  } catch {
    return true;
  }
}

/** The message of the first validator of `field` that `value` fails; undefined when it passes. */
export function validateField(field: FormField, value: unknown, t: ValidatorTexts): string | undefined {
  for (const v of field.validators) {
    const message = failing(v, value, t);
    if (message !== undefined) {
      return message;
    }
  }
  return undefined;
}

/** A value of `field` its validators accept, for the canvas's "valid" preview. */
export function sampleValue(field: FormField): unknown {
  const kinds = new Set(field.validators.map((v) => v["kind"]));
  const range = field.validators.find((v) => v["kind"] === "range");
  if (field.type.startsWith("int") || field.type.startsWith("double")) {
    return num(range?.["min"]) ?? 1;
  }
  if (field.type === "bool") {
    return true;
  }
  if (field.type.startsWith("date")) {
    return "2026-11-06";
  }
  if (kinds.has("email")) {
    return "ada@example.com";
  }
  if (kinds.has("phone")) {
    return "+442079460958";
  }
  if (kinds.has("iban")) {
    return "GB82WEST12345698765432";
  }
  return field.type.startsWith("decimal") ? "12.50" : "Ada Lovelace";
}

/** The state of `form` in `mode`, as bindings read it under `page.<form>`. */
export function formState(form: Form, mode: FormMode, t: ValidatorTexts): Obj {
  const values: Obj = {};
  const errors: Obj = {};
  const flags: Obj = {};
  for (const f of form.fields) {
    const value = mode === "valid" || mode === "submitting" ? sampleValue(f) : (f.initial ?? "");
    values[f.name] = value;
    flags[f.name] = mode !== "pristine";
    const error = mode === "errors" ? validateField(f, value, t) : undefined;
    if (error !== undefined) {
      errors[f.name] = error;
    }
  }
  return {
    values,
    errors,
    dirty: flags,
    touched: flags,
    status: mode === "submitting" ? "submitting" : "idle",
    valid: Object.keys(errors).length === 0,
    validating: false,
  };
}

/** The design-time state of every form of `page`, in the mode picked for it (pristine by default), by form name. */
export function formStatesOf(page: unknown, modeOf: (form: string) => FormMode | undefined, t: ValidatorTexts): Obj {
  return Object.fromEntries(formsOf(page).map((f) => [f.name, formState(f, modeOf(f.name) ?? "pristine", t)]));
}

// ——— Edits of a page's forms (each a recipe over the page document) ———

function formsArray(page: Obj): Obj[] {
  if (!Array.isArray(page["forms"])) {
    page["forms"] = [];
  }
  return page["forms"] as Obj[];
}

function fieldsOf(page: Obj, form: number): Obj[] | undefined {
  const f = formsArray(page)[form];
  return isObj(f) && Array.isArray(f["fields"]) ? (f["fields"] as Obj[]) : undefined;
}

export const addFormRecipe =
  (id: string, name: string) =>
  (page: Obj): void => {
    formsArray(page).push({ id, name, fields: [{ name: "name", type: "string", initial: "" }] });
  };

export const removeFormRecipe =
  (form: number) =>
  (page: Obj): void => {
    formsArray(page).splice(form, 1);
    if (formsArray(page).length === 0) {
      delete page["forms"];
    }
  };

export const renameFormRecipe =
  (form: number, name: string) =>
  (page: Obj): void => {
    const f = formsArray(page)[form];
    if (isObj(f)) {
      f["name"] = name;
    }
  };

export const addFieldRecipe =
  (form: number, name: string) =>
  (page: Obj): void => {
    fieldsOf(page, form)?.push({ name, type: "string", initial: "" });
  };

export const removeFieldRecipe =
  (form: number, field: number) =>
  (page: Obj): void => {
    const fields = fieldsOf(page, form);
    if (fields !== undefined && fields.length > 1) {
      fields.splice(field, 1);
    }
  };

/** Sets one key of a field (`name`, `type`, `initial`); undefined removes it. */
export const setFieldKeyRecipe =
  (form: number, field: number, key: string, value: unknown) =>
  (page: Obj): void => {
    const f = fieldsOf(page, form)?.[field];
    if (!isObj(f)) {
      return;
    }
    if (value === undefined) {
      delete f[key];
    } else {
      f[key] = value;
    }
  };

export const addValidatorRecipe =
  (form: number, field: number, kind: ValidatorKind) =>
  (page: Obj): void => {
    const f = fieldsOf(page, form)?.[field];
    if (isObj(f)) {
      const list = Array.isArray(f["validators"]) ? (f["validators"] as Obj[]) : [];
      f["validators"] = [...list, defaultValidator(kind)];
    }
  };

/** A new validator of `kind` with options that make it valid to the compiler. */
function defaultValidator(kind: ValidatorKind): Obj {
  switch (kind) {
    case "length":
      return { kind, min: 1, max: 100 };
    case "range":
      return { kind, min: 0, max: 100 };
    case "regex":
      return { kind, pattern: "^.+$" };
    case "decimalPrecision":
      return { kind, maxScale: 2 };
    case "custom":
      return { kind, rule: { $expr: "true" } };
    default:
      return { kind };
  }
}

/** Sets one option of a validator; undefined removes it. */
export const setValidatorRecipe =
  (form: number, field: number, index: number, key: string, value: unknown) =>
  (page: Obj): void => {
    const f = fieldsOf(page, form)?.[field];
    const v = isObj(f) && Array.isArray(f["validators"]) ? (f["validators"] as Obj[])[index] : undefined;
    if (!isObj(v)) {
      return;
    }
    if (value === undefined || value === "") {
      delete v[key];
    } else {
      v[key] = value;
    }
  };

export const removeValidatorRecipe =
  (form: number, field: number, index: number) =>
  (page: Obj): void => {
    const f = fieldsOf(page, form)?.[field];
    if (isObj(f) && Array.isArray(f["validators"])) {
      (f["validators"] as Obj[]).splice(index, 1);
      if ((f["validators"] as Obj[]).length === 0) {
        delete f["validators"];
      }
    }
  };

// ——— Binding an input widget to a field ———

/** The input widgets a form field binds to, and whether they show an error text in their decoration. */
export const INPUT_TYPES: Readonly<Record<string, { readonly errorText: boolean }>> = {
  TextField: { errorText: true },
  TextFormField: { errorText: true },
  Checkbox: { errorText: false },
  Switch: { errorText: false },
  Slider: { errorText: false },
  CupertinoTextField: { errorText: false },
  CupertinoSwitch: { errorText: false },
};

/** The form field a widget's `value` is bound to (`page.<form>.values.<field>`), if any. */
export function boundField(node: Obj): { readonly form: string; readonly field: string } | undefined {
  const props = isObj(node["props"]) ? node["props"] : {};
  const value = props["value"];
  const expr = isObj(value) && typeof value["$expr"] === "string" ? value["$expr"] : "";
  const m = /^\s*page\.(\w+)\.values\.(\w+)\s*$/.exec(expr);
  return m === null ? undefined : { form: m[1] as string, field: m[2] as string };
}

/**
 * The recipe that binds input widget `id` to `form`'s `field` (ADR-0047): its value reads the field, a change writes
 * it, leaving the field marks it touched, and a text field shows the field's error under it.
 */
export function bindFieldRecipe(id: string, form: string, field: string): (root: unknown) => void {
  return (root) => {
    const node = findNode(root, id)?.node as Obj | undefined;
    if (node === undefined) {
      return;
    }
    const at = `page.${form}`;
    const props: Obj = isObj(node["props"]) ? node["props"] : {};
    props["value"] = { $expr: `${at}.values.${field}` };
    if (INPUT_TYPES[String(node["type"])]?.errorText === true) {
      const decoration = isObj(props["decoration"]) ? props["decoration"] : {};
      props["decoration"] = { ...decoration, errorText: { $expr: `${at}.errors.${field}` } };
    }
    node["props"] = props;
    const events: Obj = isObj(node["events"]) ? node["events"] : {};
    events["onChanged"] = {
      steps: [{ id: "write", action: "setState", input: { path: `${at}.values.${field}`, value: { $expr: "event" } } }],
    };
    if (INPUT_TYPES[String(node["type"])]?.errorText === true) {
      events["onEditingComplete"] = {
        steps: [{ id: "touch", action: "setState", input: { path: `${at}.touched.${field}`, value: true } }],
      };
    }
    node["events"] = events;
  };
}
