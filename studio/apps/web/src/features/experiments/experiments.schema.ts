// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

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
  readonly segment: string;
  readonly event: string;
  readonly kind: "proportion" | "mean";
  readonly direction: "increase" | "decrease";
  readonly guardrail: string;
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
    segment: "",
    event: "",
    kind: "proportion",
    direction: "increase",
    guardrail: "",
    minimumSample: 1000,
    start: "",
    end: "",
  };
}

export type DraftProblem = "key" | "weights" | "control" | "event";

/** What keeps a draft from being created (ABT-001: one control, weights summing to 100). */
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
  if (!/^[a-z][a-zA-Z0-9_]*$/.test(d.event)) {
    found.push("event");
  }
  return found;
}

const dateTime = (value: string) => new Date(value).toISOString();

/** The experiment document (`experiment.schema.json`) of a draft; the server validates and assigns the id. */
export function buildDocument(d: Draft, id: string): Uint8Array {
  const doc = {
    schemaVersion: "1.0.0",
    kind: "experiment",
    id,
    key: d.key,
    scope: d.scope,
    variants: d.variants.map((v) => ({ name: v.name, weight: v.weight, ...(v.control && { control: true }) })),
    holdout: d.holdout,
    ...(d.segment !== "" && { segment: d.segment }),
    primaryMetric: { event: d.event, kind: d.kind, direction: d.direction },
    ...(d.guardrail !== "" && {
      guardrailMetrics: [{ event: d.guardrail, kind: "proportion", direction: "decrease" }],
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
