// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Link } from "@tanstack/react-router";
import type { ReactElement } from "react";
import { Table } from "../security/parts.tsx";
import { approvalsMessages } from "./approvals.messages.ts";
import { AppFilter, Frame } from "./parts.tsx";
import { documentOf } from "./policy-form.ts";
import { usePolicies, useScope } from "./use-approvals.ts";

/** O11: the policies in force for an app, each opening its editor (GOV-020). */
export function PoliciesPage(): ReactElement {
  const m = approvalsMessages(useLingui().i18n);
  const { org, app } = useScope();
  const policies = usePolicies(app);
  const search = app === "" ? {} : { app };
  return (
    <Frame
      id="policies-title"
      title={m.policiesTitle}
      caption={m.policiesCaption}
      loading={policies.isPending}
      failed={policies.isError}
    >
      <div className="flex items-end justify-between gap-4">
        <AppFilter />
        <Link
          to="/o/$org/approvals/policies/$policyId"
          params={{ org, policyId: "new" } as never}
          search={search as never}
          className="underline"
        >
          {m.newPolicy}
        </Link>
      </div>
      <Table
        caption={m.policiesTitle}
        heads={[m.colKey, m.colName, m.colVersion, m.colRequires]}
        rows={(policies.data?.policies ?? []).map((p) => {
          const doc = documentOf(p.document);
          return {
            key: p.key,
            cells: [
              <Link
                key="k"
                to="/o/$org/approvals/policies/$policyId"
                params={{ org, policyId: p.key } as never}
                search={search as never}
                className="font-mono underline"
              >
                {p.key}
              </Link>,
              doc.name,
              String(p.version),
              m.requires({ n: String(doc.approvers) }),
            ],
          };
        })}
      />
    </Frame>
  );
}
