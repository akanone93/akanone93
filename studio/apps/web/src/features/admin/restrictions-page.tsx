// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { GovernanceService } from "@plux/api";
import { Button } from "@plux/ui";
import type { ReactElement } from "react";
import { adminMessages } from "./admin.messages.ts";
import { CreateForm, Frame, fmt, Table } from "./parts.tsx";
import { pageSize, splitList, useAdminQuery, useCan, useRpc, useRun } from "./use-admin.ts";

/** O22: restriction policies for the organisation, a team or a person; deny always wins (F14, GOV-006). */
export function RestrictionsPage(): ReactElement {
  const m = adminMessages(useLingui().i18n);
  const governance = useRpc(GovernanceService);
  const can = useCan("restrictions.manage");
  const { run, failed } = useRun();
  const list = useAdminQuery(["restrictions"], () => governance.listRestrictions({ page: { pageSize } }));
  return (
    <Frame
      id="admin-restrictions"
      title={m.restrictionsTitle}
      caption={m.restrictionsCaption}
      loading={list.isPending}
      failed={list.isError}
      saveFailed={failed}
    >
      <p className="text-body text-secondary">{m.lockoutNote}</p>
      <Table
        caption={m.restrictionsTitle}
        heads={[m.colSubject, m.colPatterns, m.reason, m.colCreated, ""]}
        empty={m.nothing}
        rows={(list.data?.restrictions ?? []).map((r) => ({
          key: r.id,
          cells: [
            `${r.subjectKind} ${r.subjectId}`.trim(),
            r.patterns.join(", "),
            r.reason,
            fmt(r.createdAt, m.never),
            <Button
              key="d"
              variant="danger"
              size="sm"
              disabled={!can}
              onClick={() => run(() => governance.deleteRestriction({ id: r.id }))}
            >
              {m.remove}
            </Button>,
          ],
        }))}
      />
      <CreateForm
        fields={[
          {
            name: "kind",
            label: m.subjectKind,
            options: [
              { value: "organization", label: m.kindOrg },
              { value: "team", label: m.kindTeam },
              { value: "user", label: m.kindUser },
            ],
          },
          { name: "subjectId", label: m.subjectId },
          { name: "patterns", label: m.patterns, required: true },
          { name: "reason", label: m.reason, required: true },
        ]}
        submit={m.addRestriction}
        disabled={!can}
        deniedReason={m.noPermission}
        onSubmit={(v) =>
          run(() =>
            governance.createRestriction({
              subjectKind: v("kind"),
              subjectId: v("subjectId"),
              patterns: splitList(v("patterns")),
              reason: v("reason"),
            }),
          )
        }
      />
    </Frame>
  );
}
