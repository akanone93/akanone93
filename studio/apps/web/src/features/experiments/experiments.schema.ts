// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { limitDefault } from "@plux/schema";
import { z } from "zod";

export const wizardSteps = ["target", "variants", "allocation", "targeting", "metrics", "schedule"] as const;
export type WizardStep = (typeof wizardSteps)[number];

/** A6 search: `?status` is an ExperimentState value; bad values mean any. */
export const experimentsSearchSchema = z.object({
  env: z.string().optional().catch(undefined),
  status: z.coerce.number().int().min(1).max(4).optional().catch(undefined),
});

/** A7 search: `?target` names the varied nodes, page or prop; `?step` the wizard step. */
export const newExperimentSearchSchema = z.object({
  env: z.string().optional().catch(undefined),
  target: z.string().optional().catch(undefined),
  step: z.enum(wizardSteps).optional().catch(undefined),
});

export const scopes = ["page", "subtree", "prop", "flow", "pluginVersion"] as const;
export type Scope = (typeof scopes)[number];

export interface DraftVariant {
  readonly id: string;
  readonly name: string;
  readonly control: boolean;
  readonly weight: number;
}

export interface Draft {
  readonly key: string;
  readonly scope: Scope;
  readonly target: string;
  readonly variants: readonly DraftVariant[];
  readonly holdout: number;
  /** Segment keys the experiment targets, separated by commas or spaces; a device in any of them is eligible. */
  readonly segments: string;
  /** Attribute conditions, one per line: `<attribute> <operator> <value>`; a device must meet every one. */
  readonly attributes: string;
  /** The lead primary metric. */
  readonly event: string;
  readonly kind: "proportion" | "mean";
  readonly direction: "increase" | "decrease";
  /** Further primary metrics, one event per line, analysed with the lead metric's kind and direction. */
  readonly primaries: string;
  /** Guardrail events, one per line, each a proportion that must not rise. */
  readonly guardrails: string;
  readonly minimumSample: number;
  readonly start: string;
  readonly end: string;
}

/** The scope a target string implies: `page:` and `prop:` prefixes, otherwise a component subtree. */
export function scopeOfTarget(target: string | undefined): Scope {
  if (target?.startsWith("page:") === true) {
    return "page";
  }
  return target?.startsWith("prop:") === true ? "prop" : "subtree";
}

export function initialDraft(target: string | undefined): Draft {
  return {
    key: "",
    scope: scopeOfTarget(target),
    target: target ?? "",
    variants: [
      { id: "v0", name: "control", control: true, weight: 50 },
      { id: "v1", name: "variant", control: false, weight: 50 },
    ],
    holdout: 0,
    segments: "",
    attributes: "",
    event: "",
    kind: "proportion",
    direction: "increase",
    primaries: "",
    guardrails: "",
    minimumSample: 1000,
    start: "",
    end: "",
  };
}

export type DraftProblem = "key" | "weights" | "control" | "event" | "targeting" | "metrics" | "limits";

const eventName = /^[a-z][a-zA-Z0-9_]*$/;
const segmentKey = /^[a-z0-9]+(-[a-z0-9]+)*$/;
const attributeName = /^[a-z][a-zA-Z0-9]*(\.[a-z][a-zA-Z0-9]*)?$/;
const operators = ["eq", "neq", "in", "notIn", "lt", "lte", "gt", "gte"] as const;
const ordering = new Set(["lt", "lte", "gt", "gte"]);
const versions = new Set(["runtimeVersion", "osVersion"]);

const lines = (text: string) =>
  text
    .split("\n")
    .map((l) => l.trim())
    .filter((l) => l !== "");

/** The segment keys of a draft, in the order they are written. */
export function segmentKeys(text: string): string[] {
  return text.split(/[\s,]+/).filter((k) => k !== "");
}

export interface AttributeCondition {
  readonly attribute: string;
  readonly operator: (typeof operators)[number];
  readonly value: string | number | boolean | (string | number | boolean)[];
}

/** One operand: orderings on numbers read numbers, user attributes read booleans, versions stay text. */
function operand(attribute: string, operator: string, raw: string): string | number | boolean {
  if (ordering.has(operator) && !versions.has(attribute) && raw !== "" && !Number.isNaN(Number(raw))) {
    return Number(raw);
  }
  if (attribute.startsWith("user.") && (raw === "true" || raw === "false")) {
    return raw === "true";
  }
  return raw;
}

/** The conditions of an attribute text; a line that is not a condition makes the whole text invalid (`undefined`). */
export function parseAttributes(text: string): AttributeCondition[] | undefined {
  const out: AttributeCondition[] = [];
  for (const line of lines(text)) {
    const m = /^(\S+)\s+(\S+)\s+(.+)$/.exec(line);
    const attribute = m?.[1] ?? "";
    const operator = operators.find((o) => o === m?.[2]);
    const raw = m?.[3] ?? "";
    if (operator === undefined || !attributeName.test(attribute)) {
      return undefined;
    }
    const list = operator === "in" || operator === "notIn";
    const parts = list ? raw.split(",").map((v) => v.trim()) : [raw];
    if (parts.some((v) => v === "")) {
      return undefined;
    }
    const values = parts.map((v) => operand(attribute, operator, v));
    out.push({ attribute, operator, value: list ? values : (values[0] ?? "") });
  }
  return out;
}

/** What keeps a draft from being created (ABT-001: one control, weights summing to 100, targeting and metrics within the limits). */
export function problems(d: Draft): readonly DraftProblem[] {
  const found: DraftProblem[] = [];
  if (!/^[a-z][a-zA-Z0-9_]*$/.test(d.key)) {
    found.push("key");
  }
  if (Math.abs(d.variants.reduce((sum, v) => sum + v.weight, 0) - 100) > 1e-9) {
    found.push("weights");
  }
  if (d.variants.filter((v) => v.control).length !== 1) {
    found.push("control");
  }
  if (!eventName.test(d.event)) {
    found.push("event");
  }
  const keys = segmentKeys(d.segments);
  const attributes = parseAttributes(d.attributes);
  if (keys.some((k) => !segmentKey.test(k)) || new Set(keys).size !== keys.length || attributes === undefined) {
    found.push("targeting");
  }
  if ([...lines(d.primaries), ...lines(d.guardrails)].some((e) => !eventName.test(e))) {
    found.push("metrics");
  }
  if (
    keys.length > limitDefault("experiments.segmentsPerTargeting") ||
    (attributes?.length ?? 0) > limitDefault("experiments.attributesPerTargeting") ||
    1 + lines(d.primaries).length > limitDefault("experiments.primaryMetrics") ||
    lines(d.guardrails).length > limitDefault("experiments.guardrailMetrics")
  ) {
    found.push("limits");
  }
  return found;
}

const dateTime = (value: string) => new Date(value).toISOString();

/** The experiment document (`experiment.schema.json`) of a draft; the server validates and assigns the id. */
export function buildDocument(d: Draft, id: string): Uint8Array {
  const [segment, ...moreSegments] = segmentKeys(d.segments);
  const attributes = parseAttributes(d.attributes) ?? [];
  const primaries = lines(d.primaries);
  const guardrails = lines(d.guardrails);
  const doc = {
    schemaVersion: "1.0.0",
    kind: "experiment",
    id,
    key: d.key,
    scope: d.scope,
    variants: d.variants.map((v) => ({ name: v.name, weight: v.weight, ...(v.control && { control: true }) })),
    holdout: d.holdout,
    ...(segment !== undefined && { segment }),
    ...(moreSegments.length > 0 && { segments: moreSegments }),
    ...(attributes.length > 0 && { attributes }),
    primaryMetric: { event: d.event, kind: d.kind, direction: d.direction },
    ...(primaries.length > 0 && {
      primaryMetrics: primaries.map((event) => ({ event, kind: d.kind, direction: d.direction })),
    }),
    ...(guardrails.length > 0 && {
      guardrailMetrics: guardrails.map((event) => ({ event, kind: "proportion", direction: "decrease" })),
    }),
    minimumSampleSize: d.minimumSample,
    salt: d.key,
    ...((d.start !== "" || d.end !== "") && {
      schedule: { ...(d.start !== "" && { start: dateTime(d.start) }), ...(d.end !== "" && { end: dateTime(d.end) }) },
    }),
    ...(d.target !== "" && { description: d.target }),
  };
  return new TextEncoder().encode(JSON.stringify(doc));
}

/** The rows of one experiment's definition, read from the stored JSON; a malformed definition has none. */
export function parseDefinition(json: string): {
  scope: string;
  variants: { name: string; weight: number; control: boolean }[];
} {
  const schema = z.object({
    scope: z.string().catch(""),
    variants: z.array(z.object({ name: z.string(), weight: z.number(), control: z.boolean().optional() })).catch([]),
  });
  try {
    const parsed = schema.parse(JSON.parse(json));
    return { scope: parsed.scope, variants: parsed.variants.map((v) => ({ ...v, control: v.control === true })) };
  } catch {
    return { scope: "", variants: [] };
  }
}
