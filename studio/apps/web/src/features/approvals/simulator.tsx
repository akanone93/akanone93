// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import type { SimulatedRequirement, SimulatePolicyResponse } from "@plux/api";
import { Button, Field, Input, Select } from "@plux/ui";
import { type ReactElement, useState } from "react";
import { approvalsMessages } from "./approvals.messages.ts";
import { policyResources, policyRisks } from "./approvals.schema.ts";
import { useMe, useSimulate } from "./use-approvals.ts";

function Requirement({ r }: { readonly r: SimulatedRequirement }): ReactElement {
  const m = approvalsMessages(useLingui().i18n);
  const parts = [
    m.simDescribe({ approvers: String(r.approvers) }),
    r.roles.length > 0 ? m.simRoles({ roles: r.roles.join(", ") }) : "",
    r.users.length > 0 ? m.simUsers({ users: r.users.join(", ") }) : "",
    r.stages === "sequential" ? m.simSequential : "",
    r.distinctTeams ? m.simDistinctTeams : "",
    r.distinctFromAuthor ? m.simDistinctFromAuthor : "",
  ].filter((p) => p !== "");
  return (
    <li className="text-body">
      <span className="font-mono">{r.policyKey}</span>
      {r.draft && <span className="text-caption text-secondary"> ({m.simDraft})</span>}: {parts.join(", ")}
    </li>
  );
}

function Result({ result }: { readonly result: SimulatePolicyResponse }): ReactElement {
  const m = approvalsMessages(useLingui().i18n);
  return (
    <div className="flex flex-col gap-2" role="status">
      <p className="text-body font-medium">{result.matches ? m.simMatches : m.simNoMatch}</p>
      {result.requirements.length === 0 ? (
        <p className="text-body">{m.simNone}</p>
      ) : (
        <>
          <ul className="flex flex-col gap-1">
            {result.requirements.map((r) => (
              <Requirement key={r.policyKey} r={r} />
            ))}
          </ul>
          <p className="text-body">{m.simRequired({ n: String(result.approvalsRequired) })}</p>
        </>
      )}
      {result.requiresEnterprise && <p className="text-body text-warning">{m.simEnterprise}</p>}
    </div>
  );
}

/** The simulator panel of O12: "who would need to approve this change?" for the draft, without saving it (GOV-020). */
export function Simulator({ app, draft }: { readonly app: string; readonly draft: () => Uint8Array }): ReactElement {
  const m = approvalsMessages(useLingui().i18n);
  const simulate = useSimulate();
  const can = useMe().permissions.has("policies.manage");
  const [resource, setResource] = useState<string>(policyResources[0]);
  const [environment, setEnvironment] = useState("");
  const [action, setAction] = useState("");
  const [risk, setRisk] = useState<string[]>([]);
  const [result, setResult] = useState<SimulatePolicyResponse>();
  const [failed, setFailed] = useState(false);
  const run = () => {
    setFailed(false);
    simulate(app, draft(), { resource, environment, action, risk }).then(setResult, () => {
      setResult(undefined);
      setFailed(true);
    });
  };
  const risks: Record<string, string> = {
    touchesSecurePage: m.riskTouchesSecurePage,
    touchesMoneyFlow: m.riskTouchesMoneyFlow,
    changesFunctionUsedBy: m.riskChangesFunctionUsedBy,
  };
  return (
    <section className="flex flex-col gap-3" aria-labelledby="simulator-title">
      <h2 id="simulator-title" className="text-section">
        {m.simulatorTitle}
      </h2>
      <p className="text-body text-secondary">{m.simulatorCaption}</p>
      <Select
        label={m.sampleResource}
        options={policyResources.map((value) => ({ value, label: value }))}
        value={resource}
        onValueChange={setResource}
      />
      <Field label={m.sampleEnvironment}>
        <Input value={environment} onChange={(e) => setEnvironment(e.target.value)} />
      </Field>
      <Field label={m.sampleAction}>
        <Input value={action} onChange={(e) => setAction(e.target.value)} />
      </Field>
      <fieldset className="flex flex-wrap gap-3">
        <legend className="text-label font-medium">{m.fieldRisk}</legend>
        {policyRisks.map((v) => (
          <label key={v} className="flex items-center gap-2 text-body">
            <input
              type="checkbox"
              checked={risk.includes(v)}
              onChange={(e) => setRisk(e.target.checked ? [...risk, v] : risk.filter((s) => s !== v))}
            />
            {risks[v]}
          </label>
        ))}
      </fieldset>
      <div>
        <Button disabled={!can} onClick={run}>
          {m.simulate}
        </Button>
      </div>
      {failed && (
        <p role="alert" className="text-body text-danger">
          {m.simulateFailed}
        </p>
      )}
      {result !== undefined && <Result result={result} />}
    </section>
  );
}
