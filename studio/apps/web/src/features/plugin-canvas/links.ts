// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Link creation and assisted fixes (F6, STU-040, STU-046, STU-047). An arrow always derives from a
 * `navigate` step in a widget's event handler, so every way of making a link, the Actions tab, the
 * drag shortcut and the fixes, builds the same step through the recipes below. Recipes run over a
 * page's `root` inside the edit pipeline, so each is one undo step and one JSON Patch.
 */

import { widget } from "@plux/schema";
import { findNode } from "./select/edits.ts";

interface Obj {
  [key: string]: unknown;
}
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);

/** A screen a link can target: `own` belongs to this plugin, otherwise to another plugin of the app (SCH-025). */
export interface RouteOption {
  readonly route: string;
  readonly title: string;
  readonly own: boolean;
}

/** The events the widget type declares in its descriptor, in descriptor order; empty for an unknown type. */
export function eventsOf(type: string | undefined): readonly string[] {
  const d = type === undefined ? undefined : widget(type);
  return d === undefined ? [] : d.events.map((e) => e.name);
}

/** A navigate step found below a node's event. */
export interface NavigateLink {
  readonly event: string;
  /** Position of the step in the event's handler. */
  readonly step: number;
  readonly route: string;
}

function handlerSteps(handler: unknown): unknown[] {
  return isObj(handler) && Array.isArray(handler["steps"]) ? (handler["steps"] as unknown[]) : [];
}

function routeOf(step: unknown): string | undefined {
  if (!isObj(step) || step["action"] !== "navigate" || !isObj(step["input"])) {
    return undefined;
  }
  const route = step["input"]["route"];
  return typeof route === "string" ? route : undefined;
}

/** The navigate steps directly in a node's event handlers. */
export function linksOf(node: unknown): NavigateLink[] {
  const events = isObj(node) && isObj(node["events"]) ? node["events"] : {};
  return Object.entries(events).flatMap(([event, handler]) =>
    handlerSteps(handler).flatMap((s, step) => {
      const route = routeOf(s);
      return route === undefined ? [] : [{ event, step, route }];
    }),
  );
}

/** How an event is bound: free, already navigating, or running other steps. */
export type Binding = "free" | "navigate" | "other";

export function bindingOf(node: unknown, event: string): Binding {
  const events = isObj(node) && isObj(node["events"]) ? node["events"] : {};
  const steps = handlerSteps(events[event]);
  if (steps.length === 0) {
    return "free";
  }
  return steps.some((s) => routeOf(s) !== undefined) ? "navigate" : "other";
}

/** What `linkRecipe` does with a handler that already has steps. */
export type ApplyMode = "replace" | "append";

/** A problem with a link; the fixes offered for it are redirect, remove, replace or append (F6). */
export type LinkIssue =
  | { readonly kind: "target-missing"; readonly link: NavigateLink }
  | { readonly kind: "event-bound"; readonly event: string; readonly binding: Exclude<Binding, "free"> };

/** Links whose target no screen has. */
export function targetIssues(node: unknown, routes: readonly RouteOption[]): LinkIssue[] {
  const known = new Set(routes.map((r) => r.route));
  return linksOf(node)
    .filter((l) => !known.has(l.route))
    .map((link) => ({ kind: "target-missing", link }));
}

/** The issue of binding `event` now, if any: it already navigates or already runs steps. */
export function bindIssue(node: unknown, event: string): LinkIssue | undefined {
  const binding = bindingOf(node, event);
  return binding === "free" ? undefined : { kind: "event-bound", event, binding };
}

/** The navigate step; its identifier is passed in so the recipe stays deterministic and testable. */
export function navigateStep(route: string, id: string): Record<string, unknown> {
  return { action: "navigate", id, input: { route } };
}

function eventsOfNode(node: Obj): Obj {
  if (!isObj(node["events"])) {
    node["events"] = {};
  }
  return node["events"] as Obj;
}

/**
 * Binds `event` of node `nodeId` to a navigation to `route`. A free event gets a handler; a bound one
 * is replaced or appended to, as the user chose. Unknown nodes are left alone.
 */
export function linkRecipe(
  nodeId: string,
  event: string,
  route: string,
  stepId: string,
  mode: ApplyMode = "replace",
): (root: unknown) => void {
  return (root) => {
    const at = findNode(root, nodeId);
    if (at === null) {
      return;
    }
    const events = eventsOfNode(at.node as Obj);
    const step = navigateStep(route, stepId);
    const existing = events[event];
    if (mode === "append" && isObj(existing) && Array.isArray(existing["steps"])) {
      existing["steps"].push(step);
    } else {
      events[event] = { steps: [step] };
    }
  };
}

/** Points an existing navigate step at another route ("redirect target"). */
export function redirectRecipe(nodeId: string, link: NavigateLink, route: string): (root: unknown) => void {
  return (root) => {
    const at = findNode(root, nodeId);
    const events = at !== null && isObj(at.node["events"]) ? at.node["events"] : {};
    const step = handlerSteps(events[link.event])[link.step];
    if (isObj(step) && isObj(step["input"]) && routeOf(step) === link.route) {
      step["input"]["route"] = route;
    }
  };
}

/** Removes a navigate step; an event left without steps is removed too ("remove link"). */
export function removeLinkRecipe(nodeId: string, link: NavigateLink): (root: unknown) => void {
  return (root) => {
    const at = findNode(root, nodeId);
    const events = at !== null && isObj(at.node["events"]) ? at.node["events"] : undefined;
    const handler = events?.[link.event];
    if (events === undefined || !isObj(handler) || !Array.isArray(handler["steps"])) {
      return;
    }
    const steps = handler["steps"] as unknown[];
    if (routeOf(steps[link.step]) !== link.route) {
      return;
    }
    steps.splice(link.step, 1);
    if (steps.length === 0) {
      delete events[link.event];
    }
  };
}

/** Routes of the plugin's own pages first, then other plugins' pages; `app` may include the own ones. */
export function routeOptions(
  own: readonly { readonly route: string; readonly title: string }[],
  app: readonly { readonly route: string; readonly title: string }[],
): RouteOption[] {
  const ownRoutes = new Set(own.map((s) => s.route));
  return [
    ...own.map((s) => ({ route: s.route, title: s.title, own: true })),
    ...app.filter((s) => !ownRoutes.has(s.route)).map((s) => ({ route: s.route, title: s.title, own: false })),
  ];
}

/** The widget and event step a navigate action sits in, for the lane fixes of the canvas (STU-046). */
export interface NavigateSite {
  readonly nodeId: string;
  readonly link: NavigateLink;
}

/**
 * The `index`-th navigate action of a page in document order, the order the arrows are numbered in
 * (`deriveGhosts`). Undefined when it is not a step directly in a widget's event handler, which the
 * redirect and remove recipes cannot change.
 */
export function navigateSite(root: unknown, index: number): NavigateSite | undefined {
  const found: { step: Obj; node: Obj | undefined; route: string }[] = [];
  collectNavigates(root, undefined, found);
  const at = found[index];
  return at === undefined ? undefined : siteOf(at.node, at.step, at.route);
}

function collectNavigates(
  value: unknown,
  node: Obj | undefined,
  out: { step: Obj; node: Obj | undefined; route: string }[],
): void {
  if (Array.isArray(value)) {
    for (const v of value) {
      collectNavigates(v, node, out);
    }
  } else if (isObj(value)) {
    const route = isObj(value["input"]) ? value["input"]["route"] : undefined;
    if (value["action"] === "navigate" && typeof route === "string") {
      out.push({ step: value, node, route });
    }
    const here = typeof value["id"] === "string" ? value : node;
    for (const v of Object.values(value)) {
      collectNavigates(v, here, out);
    }
  }
}

function siteOf(node: Obj | undefined, step: Obj, route: string): NavigateSite | undefined {
  if (node === undefined || typeof node["id"] !== "string" || !isObj(node["events"])) {
    return undefined;
  }
  for (const [event, handler] of Object.entries(node["events"])) {
    const at = handlerSteps(handler).indexOf(step);
    if (at >= 0) {
      return { nodeId: node["id"], link: { event, step: at, route } };
    }
  }
  return undefined;
}
