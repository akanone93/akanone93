// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Field, Input } from "@plux/ui";
import { useParams } from "@tanstack/react-router";
import { type ReactElement, useState } from "react";
import { uuidv7 } from "../plugin-canvas/screens.ts";
import { approvalsMessages } from "./approvals.messages.ts";
import { Frame } from "./parts.tsx";
import { MatchFields, RequireFields } from "./policy-fields.tsx";
import { documentBytes, emptyForm, formOf, type PolicyForm } from "./policy-form.ts";
import { Simulator } from "./simulator.tsx";
import { refusalOf, useMe, usePolicies, usePutPolicy, useScope } from "./use-approvals.ts";

function Editor({
  app,
  initial,
  existing,
}: {
  readonly app: string;
  readonly initial: PolicyForm;
  readonly existing: boolean;
}): ReactElement {
  const m = approvalsMessages(useLingui().i18n);
  const can = useMe().permissions.has("policies.manage");
  const put = usePutPolicy();
  const [form, setForm] = useState(initial);
  const [id] = useState(() => (initial.id === "" ? uuidv7() : initial.id));
  const [outcome, setOutcome] = useState<{ ok: boolean; text: string }>();
  const change = (patch: Partial<PolicyForm>) => setForm((f) => ({ ...f, ...patch }));
  const save = () =>
    put(app, documentBytes(form, id)).then(
      () => setOutcome({ ok: true, text: m.saved }),
      (error: unknown) => setOutcome({ ok: false, text: refusalOf(error) || m.saveFailed }),
    );
  return (
    <div className="grid gap-8 md:grid-cols-2">
      <div className="flex flex-col gap-3">
        {!can && <p className="text-body text-secondary">{m.editorNeedsPermission}</p>}
        <Field label={m.fieldKey}>
          <Input value={form.key} disabled={!can || existing} onChange={(e) => change({ key: e.target.value })} />
        </Field>
        <Field label={m.fieldName}>
          <Input value={form.name} disabled={!can} onChange={(e) => change({ name: e.target.value })} />
        </Field>
        <MatchFields form={form} disabled={!can} onChange={change} />
        <RequireFields form={form} disabled={!can} onChange={change} />
        <div>
          <Button disabled={!can} onClick={() => void save()}>
            {m.save}
          </Button>
        </div>
        {outcome !== undefined && (
          <p role={outcome.ok ? "status" : "alert"} className={outcome.ok ? "text-body" : "text-body text-danger"}>
            {outcome.text}
          </p>
        )}
      </div>
      <Simulator app={app} draft={() => documentBytes(form, id)} />
    </div>
  );
}

/** O12: one policy's editor, with the simulator panel beside it (GOV-020, GOV-021). `new` starts an empty policy. */
export function PolicyPage(): ReactElement {
  const m = approvalsMessages(useLingui().i18n);
  const { app } = useScope();
  const key = (useParams({ strict: false }) as { policyId?: string }).policyId ?? "new";
  const policies = usePolicies(app);
  const found = (policies.data?.policies ?? []).find((p) => p.key === key);
  const missing = key !== "new" && policies.isSuccess && found === undefined;
  return (
    <Frame
      id="policy-title"
      title={m.editorTitle}
      caption={m.editorCaption}
      loading={policies.isPending}
      failed={policies.isError || missing}
    >
      <Editor
        key={key}
        app={app}
        initial={found === undefined ? emptyForm : formOf(found.document)}
        existing={found !== undefined}
      />
    </Frame>
  );
}
