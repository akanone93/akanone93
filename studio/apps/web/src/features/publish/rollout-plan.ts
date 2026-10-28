// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The rollout plan of the publish flow as a typed form (REL-050 to REL-053). The server stores the plan as a JSON
 * object and checks only that it is one; the field names follow `RolloutService` (`Stage`, `HealthGate`) so the plan
 * can become a rollout unchanged, and the bounds below are the ones `RolloutService` enforces when it does. The
 * registry holds no limit for the number of stages or the size of a plan, so none is applied here (LIM-001).
 */

/** Shares are counted in 1/10000 of the device buckets (REL-050). */
export const buckets = 10_000;

/** The longest event name a custom gate may name, in bytes (`HealthGate.custom`). */
export const customEventBytes = 64;

export type HealthAction = "pause" | "rollback";

export interface StageForm {
  /** Share of devices in percent, as typed. */
  readonly percent: string;
  /** How long the stage lasts, in minutes, as typed. */
  readonly minutes: string;
}

export interface CustomGateForm {
  readonly event: string;
  readonly maxIncrease: string;
}

export type NumericField =
  | "minSample"
  | "windowMinutes"
  | "crashFreeDrop"
  | "errorRateIncrease"
  | "actionErrorRateIncrease"
  | "functionErrorRateIncrease";

export interface PlanForm {
  readonly stages: readonly StageForm[];
  /** `datetime-local` value; empty starts at once. */
  readonly startAt: string;
  /** `datetime-local` value; empty keeps the rollout until it completes. */
  readonly endAt: string;
  readonly action: HealthAction;
  readonly minSample: string;
  readonly windowMinutes: string;
  readonly crashFreeDrop: string;
  readonly errorRateIncrease: string;
  readonly actionErrorRateIncrease: string;
  readonly functionErrorRateIncrease: string;
  readonly custom: readonly CustomGateForm[];
}

/** What a field is wrong with; the form turns each reason into text. */
export type PlanIssue =
  | { readonly kind: "noStages" }
  | { readonly kind: "percentRange"; readonly stage: number }
  | { readonly kind: "percentFalls"; readonly stage: number }
  | { readonly kind: "durationMissing"; readonly stage: number }
  | { readonly kind: "lastStageDuration" }
  | { readonly kind: "endBeforeStart" }
  | { readonly kind: "dateInvalid"; readonly field: "startAt" | "endAt" }
  | { readonly kind: "negative"; readonly field: NumericField }
  | { readonly kind: "customEvent"; readonly index: number }
  | { readonly kind: "customThreshold"; readonly index: number };

const numericFields: readonly NumericField[] = [
  "minSample",
  "windowMinutes",
  "crashFreeDrop",
  "errorRateIncrease",
  "actionErrorRateIncrease",
  "functionErrorRateIncrease",
];

/** A single stage at 100 %: the whole channel at once, the reading of an empty plan. */
export const defaultPlan: PlanForm = {
  stages: [{ percent: "100", minutes: "0" }],
  startAt: "",
  endAt: "",
  action: "pause",
  minSample: "0",
  windowMinutes: "0",
  crashFreeDrop: "0",
  errorRateIncrease: "0",
  actionErrorRateIncrease: "0",
  functionErrorRateIncrease: "0",
  custom: [],
};

function number(text: string): number {
  return text.trim() === "" ? Number.NaN : Number(text);
}

function basisPoints(percent: string): number {
  return Math.round(number(percent) * 100);
}

function seconds(minutes: string): number {
  return Math.round(number(minutes) * 60);
}

function time(text: string): number {
  return text === "" ? Number.NaN : new Date(text).getTime();
}

/** Every reason the form cannot be sent, in field order; empty when the server will accept it. */
export function validatePlan(plan: PlanForm): PlanIssue[] {
  const issues: PlanIssue[] = [];
  if (plan.stages.length === 0) {
    issues.push({ kind: "noStages" });
  }
  let previous = 0;
  plan.stages.forEach((s, i) => {
    const bp = basisPoints(s.percent);
    const secs = seconds(s.minutes);
    const last = i === plan.stages.length - 1;
    if (!Number.isFinite(bp) || bp < 1 || bp > buckets) {
      issues.push({ kind: "percentRange", stage: i });
    } else if (bp < previous) {
      issues.push({ kind: "percentFalls", stage: i });
    }
    if (!Number.isFinite(secs) || secs < 0 || (!last && secs === 0)) {
      issues.push({ kind: "durationMissing", stage: i });
    } else if (last && secs > 0 && bp < buckets) {
      issues.push({ kind: "lastStageDuration" });
    }
    previous = Number.isFinite(bp) ? bp : previous;
  });
  for (const field of ["startAt", "endAt"] as const) {
    if (plan[field] !== "" && !Number.isFinite(time(plan[field]))) {
      issues.push({ kind: "dateInvalid", field });
    }
  }
  if (plan.startAt !== "" && plan.endAt !== "" && time(plan.endAt) <= time(plan.startAt)) {
    issues.push({ kind: "endBeforeStart" });
  }
  for (const field of numericFields) {
    const value = number(plan[field]);
    if (!Number.isFinite(value) || value < 0) {
      issues.push({ kind: "negative", field });
    }
  }
  plan.custom.forEach((c, index) => {
    if (c.event === "" || new TextEncoder().encode(c.event).length > customEventBytes) {
      issues.push({ kind: "customEvent", index });
    }
    if (!(number(c.maxIncrease) > 0)) {
      issues.push({ kind: "customThreshold", index });
    }
  });
  return issues;
}

/** The named fields of a JSON object, each of unknown type; anything else has none. */
function record<K extends string>(value: unknown): Partial<Record<K, unknown>> {
  return typeof value === "object" && value !== null && !Array.isArray(value)
    ? (value as Partial<Record<K, unknown>>)
    : {};
}

function text(value: unknown, fallback: string): string {
  return typeof value === "number" || typeof value === "string" ? String(value) : fallback;
}

/** Local `datetime-local` text of an RFC 3339 instant; empty when there is none. */
function localTime(value: unknown): string {
  if (typeof value !== "string" || value === "") {
    return "";
  }
  const at = new Date(value);
  if (Number.isNaN(at.getTime())) {
    return "";
  }
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${at.getFullYear()}-${pad(at.getMonth() + 1)}-${pad(at.getDate())}T${pad(at.getHours())}:${pad(at.getMinutes())}`;
}

/** Reads a stored plan; what it does not hold falls back to the default, so the form always opens. */
export function planFromJson(bytes: Uint8Array): PlanForm {
  if (bytes.length === 0) {
    return defaultPlan;
  }
  let root: Partial<Record<"stages" | "startAt" | "endAt" | "health", unknown>>;
  try {
    root = record(JSON.parse(new TextDecoder().decode(bytes)));
  } catch {
    return defaultPlan;
  }
  const stages = Array.isArray(root.stages)
    ? root.stages.map((s) => {
        const stage = record<"basisPoints" | "durationSeconds">(s);
        return {
          percent: String(Number(text(stage.basisPoints, "10000")) / 100),
          minutes: String(Number(text(stage.durationSeconds, "0")) / 60),
        };
      })
    : defaultPlan.stages;
  const health = record<
    | "custom"
    | "action"
    | "minSample"
    | "windowSeconds"
    | "maxCrashFreeDropBasisPoints"
    | "maxErrorRateIncreaseBasisPoints"
    | "maxActionErrorRateIncreaseBasisPoints"
    | "maxFunctionErrorRateIncreaseBasisPoints"
  >(root.health);
  const custom = Array.isArray(health.custom)
    ? health.custom.map((c) => {
        const gate = record<"event" | "maxIncreaseBasisPoints">(c);
        return { event: text(gate.event, ""), maxIncrease: text(gate.maxIncreaseBasisPoints, "") };
      })
    : [];
  return {
    stages,
    startAt: localTime(root.startAt),
    endAt: localTime(root.endAt),
    action: health.action === "HEALTH_ACTION_ROLLBACK" ? "rollback" : "pause",
    minSample: text(health.minSample, "0"),
    windowMinutes: String(Number(text(health.windowSeconds, "0")) / 60),
    crashFreeDrop: text(health.maxCrashFreeDropBasisPoints, "0"),
    errorRateIncrease: text(health.maxErrorRateIncreaseBasisPoints, "0"),
    actionErrorRateIncrease: text(health.maxActionErrorRateIncreaseBasisPoints, "0"),
    functionErrorRateIncrease: text(health.maxFunctionErrorRateIncreaseBasisPoints, "0"),
    custom,
  };
}

/** The plan as the JSON object the server stores; send it only for a form without issues. Zero thresholds are left out. */
export function planToJson(plan: PlanForm): Uint8Array {
  const health: { [key: string]: unknown; custom?: unknown; action?: string } = {};
  const put = (key: string, value: number) => {
    if (value > 0) {
      health[key] = value;
    }
  };
  put("minSample", Math.round(number(plan.minSample)));
  put("windowSeconds", seconds(plan.windowMinutes));
  put("maxCrashFreeDropBasisPoints", Math.round(number(plan.crashFreeDrop)));
  put("maxErrorRateIncreaseBasisPoints", Math.round(number(plan.errorRateIncrease)));
  put("maxActionErrorRateIncreaseBasisPoints", Math.round(number(plan.actionErrorRateIncrease)));
  put("maxFunctionErrorRateIncreaseBasisPoints", Math.round(number(plan.functionErrorRateIncrease)));
  if (plan.custom.length > 0) {
    health.custom = plan.custom.map((c) => ({
      event: c.event,
      maxIncreaseBasisPoints: Math.round(number(c.maxIncrease)),
    }));
  }
  if (plan.action === "rollback") {
    health.action = "HEALTH_ACTION_ROLLBACK";
  }
  const json = {
    stages: plan.stages.map((s) => ({ basisPoints: basisPoints(s.percent), durationSeconds: seconds(s.minutes) })),
    ...(plan.startAt !== "" && { startAt: new Date(plan.startAt).toISOString() }),
    ...(plan.endAt !== "" && { endAt: new Date(plan.endAt).toISOString() }),
    ...(Object.keys(health).length > 0 && { health }),
  };
  return new TextEncoder().encode(JSON.stringify(json));
}
