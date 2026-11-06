// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Events and actions on the canvas (PC-27, ACT-001–ACT-003): the steps a widget's events run, as badges and a tag on
 * the canvas and a list in the inspector; the quick actions that write the common steps in one go — go to a page, open
 * a dialog or a sheet, show a message, open a link, go back, set state, submit a form; and the snackbar a selected
 * widget shows, drawn at the bottom of its screen as the device shows it.
 */

import { STEP_NAME, stepName } from "./links.ts";
import { COPY_MARK } from "./list-preview.ts";
import { findNode } from "./select/edits.ts";

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);

/** What a quick action writes. */
export type QuickKind = "page" | "dialog" | "sheet" | "message" | "link" | "back" | "state" | "submit";
export const QUICK_KINDS: readonly QuickKind[] = [
  "page",
  "dialog",
  "sheet",
  "message",
  "link",
  "back",
  "state",
  "submit",
];

export interface QuickInput {
  readonly route?: string;
  readonly message?: string;
  readonly actionLabel?: string;
  readonly url?: string;
  readonly path?: string;
  readonly value?: unknown;
  readonly form?: string;
}

const ACTION: Readonly<Record<QuickKind, string>> = {
  page: "navigate",
  dialog: "openDialog",
  sheet: "openBottomSheet",
  message: "showSnackbar",
  link: "openUrl",
  back: "pop",
  state: "setState",
  submit: "submitForm",
};

/** The inputs a quick action needs before it can be written. */
export const NEEDS: Readonly<Record<QuickKind, readonly (keyof QuickInput)[]>> = {
  page: ["route"],
  dialog: ["route"],
  sheet: ["route"],
  message: ["message"],
  link: ["url"],
  back: [],
  state: ["path", "value"],
  submit: ["form"],
};

/** Whether `input` has what quick action `kind` needs. */
export const ready = (kind: QuickKind, input: QuickInput): boolean =>
  NEEDS[kind].every((k) => input[k] !== undefined && input[k] !== "");

/** The step quick action `kind` writes, with id `id`. */
export function quickStep(kind: QuickKind, input: QuickInput, id: string): Obj {
  const args: Obj = {};
  switch (kind) {
    case "page":
    case "dialog":
    case "sheet":
      args["route"] = input.route;
      break;
    case "message":
      args["message"] = input.message;
      if (input.actionLabel !== undefined && input.actionLabel !== "") {
        args["actionLabel"] = input.actionLabel;
      }
      break;
    case "link":
      args["url"] = input.url;
      break;
    case "state":
      args["path"] = input.path;
      args["value"] = input.value;
      break;
    case "submit":
      args["form"] = input.form;
      break;
    default:
      break;
  }
  return { id, action: ACTION[kind], input: args };
}

/** The steps of an event's handler: an inline graph's `steps`. */
const stepsOf = (handler: unknown): unknown[] =>
  isObj(handler) && Array.isArray(handler["steps"]) ? (handler["steps"] as unknown[]) : [];

/** A step's id: its own when it is a name, else one unique among `steps` (`common.schema.json#/$defs/name`). */
const namedStep = (step: Obj, steps: readonly unknown[]): string =>
  STEP_NAME.test(String(step["id"])) ? String(step["id"]) : stepName(steps, String(step["action"]));

/** Points the last step of `steps` at `next`, unless it already goes somewhere. */
function chain(steps: readonly unknown[], next: string): void {
  const last = steps[steps.length - 1];
  if (isObj(last) && last["next"] === undefined && typeof last["id"] === "string") {
    last["next"] = next;
  }
}

/** The recipe that gives node `id`'s `event` the step `step`: in place of its steps, or after them. */
export function addStepRecipe(
  nodeId: string,
  event: string,
  given: Obj,
  mode: "replace" | "append",
): (root: unknown) => void {
  return (root) => {
    const node = findNode(root, nodeId)?.node as Obj | undefined;
    if (node === undefined) {
      return;
    }
    const events: Obj = isObj(node["events"]) ? node["events"] : {};
    const steps = mode === "append" ? (stepsOf(events[event]) as Obj[]) : [];
    const step = { ...given, id: namedStep(given, steps) };
    if (steps.length === 0) {
      events[event] = { steps: [step] };
    } else {
      chain(steps, step.id);
      steps.push(step);
    }
    node["events"] = events;
  };
}

/** Re-points the steps that went to `gone` where `gone` went. */
function bridge(steps: readonly unknown[], gone: unknown): void {
  if (!isObj(gone)) {
    return;
  }
  for (const s of steps) {
    if (isObj(s) && s["next"] === gone["id"]) {
      if (gone["next"] === undefined) {
        delete s["next"];
      } else {
        s["next"] = gone["next"];
      }
    }
  }
}

/** The recipe that removes step `index` of node `id`'s `event`; an event left with none is removed. */
export function removeStepRecipe(id: string, event: string, index: number): (root: unknown) => void {
  return (root) => {
    const node = findNode(root, id)?.node as Obj | undefined;
    const events = isObj(node?.["events"]) ? (node["events"] as Obj) : undefined;
    const steps = stepsOf(events?.[event]);
    const [gone] = steps.splice(index, 1);
    bridge(steps, gone);
    if (events === undefined || steps.length > 0) {
      return;
    }
    delete events[event];
    if (Object.keys(events).length === 0 && node !== undefined) {
      delete node["events"];
    }
  };
}

/** One step of a widget's event handlers, as the inspector and the canvas list it. */
export interface HandlerStep {
  readonly event: string;
  readonly index: number;
  readonly action: string;
  /** The step's main input: a route, a message, a url, a path, a form. */
  readonly detail?: string;
}

const DETAIL_KEYS = ["route", "message", "url", "path", "form", "source", "name"];

/** Every step of every event of `node`, in order. */
export function handlersOf(node: unknown): HandlerStep[] {
  const events = isObj(node) && isObj(node["events"]) ? node["events"] : {};
  return Object.entries(events).flatMap(([event, handler]) =>
    stepsOf(handler).flatMap((s, index) => {
      if (!isObj(s) || typeof s["action"] !== "string") {
        return [];
      }
      const input = isObj(s["input"]) ? s["input"] : {};
      const key = DETAIL_KEYS.find((k) => typeof input[k] === "string");
      return [{ event, index, action: s["action"], ...(key === undefined ? {} : { detail: String(input[key]) }) }];
    }),
  );
}

/** The children and slot contents of a node. */
const kidsOf = (n: Obj): unknown[] => [
  ...(Array.isArray(n["children"]) ? n["children"] : []),
  ...(isObj(n["slots"]) ? Object.values(n["slots"]).flatMap((v) => (Array.isArray(v) ? v : [v])) : []),
];

/** The handler steps of every widget of `roots` that has any, by node id (PC-27). */
export function eventMarks(roots: readonly (unknown | undefined)[]): Map<string, readonly HandlerStep[]> {
  const out = new Map<string, readonly HandlerStep[]>();
  const visit = (n: unknown) => {
    if (!isObj(n)) {
      return;
    }
    const steps = handlersOf(n);
    if (steps.length > 0 && typeof n["id"] === "string") {
      out.set(n["id"], steps);
    }
    kidsOf(n).forEach(visit);
  };
  roots.forEach(visit);
  return out;
}

/** The input of a `showSnackbar` step, if `s` is one with a message. */
const snackInput = (s: unknown): Obj | undefined =>
  isObj(s) && s["action"] === "showSnackbar" && isObj(s["input"]) && typeof s["input"]["message"] === "string"
    ? s["input"]
    : undefined;

/** The first message a widget's events show (`showSnackbar`), if any. */
export function snackbarOf(node: unknown): { readonly message: string; readonly actionLabel?: string } | undefined {
  const events = isObj(node) && isObj(node["events"]) ? node["events"] : {};
  const input = Object.values(events)
    .flatMap(stepsOf)
    .map(snackInput)
    .find((i) => i !== undefined);
  if (input === undefined) {
    return undefined;
  }
  const label = input["actionLabel"];
  return { message: String(input["message"]), ...(typeof label === "string" ? { actionLabel: label } : {}) };
}

/**
 * A Material 3 snackbar for the canvas (`_SnackbarDefaultsM3`, fixed): `inverseSurface` behind the message in
 * `onInverseSurface`, the action in `inversePrimary`, 16 at the start, full width at the bottom.
 * Its ids end in `~snack`, so a click on it selects the widget that shows it.
 */
export function snackbarNode(owner: string, s: { readonly message: string; readonly actionLabel?: string }): Obj {
  const id = (part: string) => `${owner}${COPY_MARK}snack${part}`;
  // bodyMedium (14/20, 0.25) in the role's colour.
  const text = (part: string, data: string, role: string) => ({
    id: id(part),
    type: "Text",
    props: { data, style: { fontSize: 14, height: 1.43, letterSpacing: 0.25, color: { $token: `color.${role}` } } },
  });
  const message = {
    id: id("-expand"),
    type: "Expanded",
    slots: { child: text("-message", s.message, "onInverseSurface") },
  };
  const children: Obj[] = [message];
  if (s.actionLabel !== undefined) {
    children.push(text("-action", s.actionLabel, "inversePrimary"));
  }
  return {
    id: id(""),
    type: "Container",
    props: {
      padding: { start: 16, end: s.actionLabel === undefined ? 16 : 24, top: 14, bottom: 14 },
      decoration: { color: { $token: "color.inverseSurface" } },
    },
    // `SnackBar` keeps a fixed bar's content clear of the home indicator: `SafeArea(top: false)` inside it.
    slots: {
      child: {
        id: id("-safe"),
        type: "SafeArea",
        props: { top: false },
        slots: { child: { id: id("-row"), type: "Row", props: { spacing: 12 }, children } },
      },
    },
  };
}

/** `root` (a page's scaffold) with the snackbar `node` shows, in its design-time `snackBar` slot; else `root`. */
export function withSnackbar(root: unknown, owner: string | undefined): unknown {
  if (owner === undefined || !isObj(root) || root["type"] !== "Scaffold") {
    return root;
  }
  const at = findNode(root, owner);
  const snack = at === null ? undefined : snackbarOf(at.node);
  if (snack === undefined) {
    return root;
  }
  const slots = isObj(root["slots"]) ? root["slots"] : {};
  return { ...root, slots: { ...slots, snackBar: snackbarNode(owner, snack) } };
}
