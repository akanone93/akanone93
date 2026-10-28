// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Field, Input, Select } from "@plux/ui";
import type { ReactElement, ReactNode } from "react";
import { rolloutMessages } from "./rollout.messages.ts";
import type { CustomGateForm, HealthAction, NumericField, PlanForm, PlanIssue, StageForm } from "./rollout-plan.ts";

type Messages = ReturnType<typeof rolloutMessages>;

/** The field an issue belongs to, as the key `errors` is looked up by. */
function keyOf(issue: PlanIssue, lastStage: number): string {
  switch (issue.kind) {
    case "percentRange":
    case "percentFalls":
      return `percent${issue.stage}`;
    case "durationMissing":
      return `minutes${issue.stage}`;
    case "lastStageDuration":
      return `minutes${lastStage}`;
    case "dateInvalid":
      return issue.field;
    case "endBeforeStart":
      return "endAt";
    case "negative":
      return issue.field;
    case "customEvent":
      return `customEvent${issue.index}`;
    case "customThreshold":
      return `customMax${issue.index}`;
    case "noStages":
      return "stages";
  }
}

function textOf(m: Messages, issue: PlanIssue): string {
  switch (issue.kind) {
    case "noStages":
      return m.noStages;
    case "percentRange":
      return m.percentRange;
    case "percentFalls":
      return m.percentFalls;
    case "durationMissing":
      return m.durationMissing;
    case "lastStageDuration":
      return m.lastStageDuration;
    case "endBeforeStart":
      return m.endBeforeStart;
    case "dateInvalid":
      return m.dateInvalid;
    case "negative":
      return m.negative;
    case "customEvent":
      return m.customEventInvalid;
    case "customThreshold":
      return m.customThreshold;
  }
}

/** The first message per field; a field with none is valid. */
function errorsOf(m: Messages, issues: readonly PlanIssue[], stages: number): ReadonlyMap<string, string> {
  const out = new Map<string, string>();
  for (const issue of issues) {
    const key = keyOf(issue, stages - 1);
    if (!out.has(key)) {
      out.set(key, textOf(m, issue));
    }
  }
  return out;
}

function replaceAt<T>(list: readonly T[], index: number, patch: Partial<T>): T[] {
  return list.map((item, i) => (i === index ? { ...item, ...patch } : item));
}

function Group({ legend, children }: { readonly legend: string; readonly children: ReactNode }): ReactElement {
  return (
    <fieldset className="flex flex-col gap-3 rounded-sm border border-hairline p-3">
      <legend className="px-1 text-label font-medium">{legend}</legend>
      {children}
    </fieldset>
  );
}

/**
 * The rollout plan as a form: stages, schedule and the health gate (REL-050 to REL-053). `issues` are what the model
 * found wrong; each is shown on its field. The form changes nothing but the plan it is given.
 */
export function RolloutForm({
  plan,
  issues,
  editable,
  onChange,
}: {
  readonly plan: PlanForm;
  readonly issues: readonly PlanIssue[];
  readonly editable: boolean;
  readonly onChange: (next: PlanForm) => void;
}): ReactElement {
  const m = rolloutMessages(useLingui().i18n);
  const errors = errorsOf(m, issues, plan.stages.length);
  const error = (key: string) => errors.get(key);
  const set = (patch: Partial<PlanForm>) => onChange({ ...plan, ...patch });
  const number = (field: NumericField, label: string) => (
    <Field label={label} error={error(field)}>
      <Input
        type="number"
        inputMode="decimal"
        min={0}
        value={plan[field]}
        readOnly={!editable}
        onChange={(e) => set({ [field]: e.target.value })}
      />
    </Field>
  );
  return (
    <div className="flex flex-col gap-4">
      <Group legend={m.stagesLegend}>
        {error("stages") !== undefined && (
          <p role="alert" className="text-caption text-danger">
            {error("stages")}
          </p>
        )}
        {plan.stages.map((stage: StageForm, i) => (
          // The stages have no identity beyond their position; removing one renumbers the rest.
          // biome-ignore lint/suspicious/noArrayIndexKey: positional list without ids
          <div key={i} className="flex flex-wrap items-start gap-3">
            <span className="pt-6 font-mono text-data">{m.stage(String(i + 1))}</span>
            <Field label={m.stagePercent} error={error(`percent${i}`)}>
              <Input
                type="number"
                inputMode="decimal"
                min={0.01}
                max={100}
                step="any"
                value={stage.percent}
                readOnly={!editable}
                onChange={(e) => set({ stages: replaceAt(plan.stages, i, { percent: e.target.value }) })}
              />
            </Field>
            <Field label={m.stageMinutes} error={error(`minutes${i}`)}>
              <Input
                type="number"
                inputMode="decimal"
                min={0}
                step="any"
                value={stage.minutes}
                readOnly={!editable}
                onChange={(e) => set({ stages: replaceAt(plan.stages, i, { minutes: e.target.value }) })}
              />
            </Field>
            {editable && plan.stages.length > 1 && (
              <div className="pt-6">
                <Button
                  size="sm"
                  variant="ghost"
                  aria-label={m.removeStage(String(i + 1))}
                  onClick={() => set({ stages: plan.stages.filter((_, j) => j !== i) })}
                >
                  {m.removeStage(String(i + 1))}
                </Button>
              </div>
            )}
          </div>
        ))}
        {editable && (
          <div>
            <Button
              size="sm"
              variant="secondary"
              onClick={() => {
                const last = plan.stages[plan.stages.length - 1];
                set({ stages: [...plan.stages, { percent: last?.percent ?? "100", minutes: "0" }] });
              }}
            >
              {m.addStage}
            </Button>
          </div>
        )}
      </Group>
      <Group legend={m.scheduleLegend}>
        <Field label={m.startAt} error={error("startAt")}>
          <Input
            type="datetime-local"
            value={plan.startAt}
            readOnly={!editable}
            onChange={(e) => set({ startAt: e.target.value })}
          />
        </Field>
        <Field label={m.endAt} error={error("endAt")}>
          <Input
            type="datetime-local"
            value={plan.endAt}
            readOnly={!editable}
            onChange={(e) => set({ endAt: e.target.value })}
          />
        </Field>
      </Group>
      <Group legend={m.healthLegend}>
        <Select
          label={m.action}
          disabled={!editable}
          options={[
            { value: "pause", label: m.actionPause },
            { value: "rollback", label: m.actionRollback },
          ]}
          value={plan.action}
          onValueChange={(action) => set({ action: action as HealthAction })}
        />
        {number("minSample", m.minSample)}
        {number("windowMinutes", m.windowMinutes)}
        {number("crashFreeDrop", m.crashFreeDrop)}
        {number("errorRateIncrease", m.errorRateIncrease)}
        {number("actionErrorRateIncrease", m.actionErrorRateIncrease)}
        {number("functionErrorRateIncrease", m.functionErrorRateIncrease)}
        {plan.custom.length > 0 && <h3 className="text-label font-medium">{m.customLegend}</h3>}
        {plan.custom.map((gate: CustomGateForm, i) => (
          // biome-ignore lint/suspicious/noArrayIndexKey: positional list without ids
          <div key={i} className="flex flex-wrap items-start gap-3">
            <Field label={m.customEvent} error={error(`customEvent${i}`)}>
              <Input
                value={gate.event}
                readOnly={!editable}
                onChange={(e) => set({ custom: replaceAt(plan.custom, i, { event: e.target.value }) })}
              />
            </Field>
            <Field label={m.customMax} error={error(`customMax${i}`)}>
              <Input
                type="number"
                inputMode="decimal"
                min={0}
                value={gate.maxIncrease}
                readOnly={!editable}
                onChange={(e) => set({ custom: replaceAt(plan.custom, i, { maxIncrease: e.target.value }) })}
              />
            </Field>
            {editable && (
              <div className="pt-6">
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => set({ custom: plan.custom.filter((_, j) => j !== i) })}
                >
                  {m.removeCustom(String(i + 1))}
                </Button>
              </div>
            )}
          </div>
        ))}
        {editable && (
          <div>
            <Button
              size="sm"
              variant="secondary"
              onClick={() => set({ custom: [...plan.custom, { event: "", maxIncrease: "" }] })}
            >
              {m.addCustom}
            </Button>
          </div>
        )}
      </Group>
    </div>
  );
}
