// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Field, Input, Select } from "@plux/ui";
import type { ReactElement } from "react";
import { approvalsMessages } from "./approvals.messages.ts";
import { policyResources, policyRisks } from "./approvals.schema.ts";
import type { PolicyForm } from "./policy-form.ts";

/** Checkboxes for a fixed set of values; `selected` is replaced as a whole. */
function Choices({
  legend,
  values,
  names,
  selected,
  disabled,
  onChange,
}: {
  readonly legend: string;
  readonly values: readonly string[];
  readonly names: Readonly<Record<string, string>>;
  readonly selected: readonly string[];
  readonly disabled: boolean;
  readonly onChange: (next: string[]) => void;
}): ReactElement {
  return (
    <fieldset className="flex flex-wrap gap-3">
      <legend className="text-label font-medium">{legend}</legend>
      {values.map((v) => (
        <label key={v} className="flex items-center gap-2 text-body">
          <input
            type="checkbox"
            checked={selected.includes(v)}
            disabled={disabled}
            onChange={(e) => onChange(e.target.checked ? [...selected, v] : selected.filter((s) => s !== v))}
          />
          {names[v] ?? v}
        </label>
      ))}
    </fieldset>
  );
}

/** The match half of the policy editor: which changes the policy applies to. */
export function MatchFields({
  form,
  disabled,
  onChange,
}: {
  readonly form: PolicyForm;
  readonly disabled: boolean;
  readonly onChange: (patch: Partial<PolicyForm>) => void;
}): ReactElement {
  const m = approvalsMessages(useLingui().i18n);
  const risks: Record<string, string> = {
    touchesSecurePage: m.riskTouchesSecurePage,
    touchesMoneyFlow: m.riskTouchesMoneyFlow,
    changesFunctionUsedBy: m.riskChangesFunctionUsedBy,
  };
  return (
    <>
      <Choices
        legend={m.fieldResources}
        values={policyResources}
        names={{}}
        selected={form.resources}
        disabled={disabled}
        onChange={(resources) => onChange({ resources })}
      />
      <Field label={m.fieldEnvironments}>
        <Input
          value={form.environments}
          disabled={disabled}
          onChange={(e) => onChange({ environments: e.target.value })}
        />
      </Field>
      <Field label={m.fieldActions}>
        <Input value={form.actions} disabled={disabled} onChange={(e) => onChange({ actions: e.target.value })} />
      </Field>
      <Select
        label={m.fieldProduction}
        disabled={disabled}
        options={[
          { value: "any", label: m.productionAny },
          { value: "only", label: m.productionOnly },
          { value: "never", label: m.productionNever },
        ]}
        value={form.production}
        onValueChange={(production) => onChange({ production: production as PolicyForm["production"] })}
      />
      <Choices
        legend={m.fieldRisk}
        values={policyRisks}
        names={risks}
        selected={form.risk}
        disabled={disabled}
        onChange={(risk) => onChange({ risk })}
      />
    </>
  );
}

/** The require half of the policy editor: what approval a matching change needs. */
export function RequireFields({
  form,
  disabled,
  onChange,
}: {
  readonly form: PolicyForm;
  readonly disabled: boolean;
  readonly onChange: (patch: Partial<PolicyForm>) => void;
}): ReactElement {
  const m = approvalsMessages(useLingui().i18n);
  return (
    <>
      <Field label={m.fieldApprovers}>
        <Input
          type="number"
          min={1}
          max={10}
          value={String(form.approvers)}
          disabled={disabled}
          onChange={(e) => onChange({ approvers: Number(e.target.value) })}
        />
      </Field>
      <Field label={m.fieldRoles}>
        <Input value={form.roles} disabled={disabled} onChange={(e) => onChange({ roles: e.target.value })} />
      </Field>
      <Field label={m.fieldUsers}>
        <Input value={form.users} disabled={disabled} onChange={(e) => onChange({ users: e.target.value })} />
      </Field>
      <label className="flex items-center gap-2 text-body">
        <input
          type="checkbox"
          checked={form.distinctFromAuthor}
          disabled={disabled}
          onChange={(e) => onChange({ distinctFromAuthor: e.target.checked })}
        />
        {m.fieldDistinctFromAuthor}
      </label>
      <label className="flex items-center gap-2 text-body">
        <input
          type="checkbox"
          checked={form.distinctTeams}
          disabled={disabled}
          onChange={(e) => onChange({ distinctTeams: e.target.checked })}
        />
        {m.fieldDistinctTeams}
      </label>
      <Select
        label={m.fieldStages}
        disabled={disabled}
        options={[
          { value: "none", label: m.stagesNone },
          { value: "sequential", label: m.stagesSequential },
          { value: "parallel", label: m.stagesParallel },
        ]}
        value={form.stages === "" ? "none" : form.stages}
        onValueChange={(stages) => onChange({ stages: stages === "none" ? "" : (stages as PolicyForm["stages"]) })}
      />
    </>
  );
}
