// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { GovernanceService, OrgService } from "@plux/api";
import { Button } from "@plux/ui";
import type { ReactElement } from "react";
import { adminMessages } from "./admin.messages.ts";
import { CreateForm, Frame, fmt, Table } from "./parts.tsx";
import { pageSize, splitList, useAdminQuery, useCan, useOrgId, useRpc, useRun } from "./use-admin.ts";

/** What a restriction can apply to, as one choice: `organization:`, `team:<id>` or `user:<id>`. */
const subjectValue = (kind: string, id: string): string => `${kind}:${id}`;

function splitSubject(value: string): { kind: string; id: string } {
  const at = value.indexOf(":");
  return at < 0 ? { kind: value, id: "" } : { kind: value.slice(0, at), id: value.slice(at + 1) };
}

/** O22: restriction policies for the organisation, a team or a person; deny always wins (F14, GOV-006). */
export function RestrictionsPage(): ReactElement {
  const m = adminMessages(useLingui().i18n);
  const governance = useRpc(GovernanceService);
  const orgApi = useRpc(OrgService);
  const organizationId = useOrgId();
  const can = useCan("restrictions.manage");
  const { run, failed, error: saveError } = useRun();
  const list = useAdminQuery(["restrictions"], () => governance.listRestrictions({ page: { pageSize } }));
  // A restriction applies to the organisation, a team or a person: chosen by name, never typed as an identifier.
  const teams = useAdminQuery(["teams", organizationId], () =>
    orgApi.listTeams({ organizationId, page: { pageSize } }),
  );
  const members = useAdminQuery(["members", organizationId], () =>
    orgApi.listMembers({ organizationId, page: { pageSize } }),
  );
  const people = (members.data?.members ?? []).filter((p) => p.teamId === "");
  const subjects = [
    { value: subjectValue("organization", ""), label: m.kindOrg },
    ...(teams.data?.teams ?? []).map((t) => ({
      value: subjectValue("team", t.id),
      label: m.subjectTeam(t.name || t.key),
    })),
    ...people.map((p) => ({ value: subjectValue("user", p.userId), label: m.subjectPerson(p.displayName || p.email) })),
  ];
  const nameOf = (kind: string, id: string): string =>
    subjects.find((o) => o.value === subjectValue(kind, id))?.label ?? `${kind} ${id}`.trim();
  return (
    <Frame
      id="admin-restrictions"
      title={m.restrictionsTitle}
      caption={m.restrictionsCaption}
      loading={list.isPending}
      failed={list.isError}
      error={list.error}
      saveFailed={failed}
      saveError={saveError}
    >
      <p className="text-body text-muted-foreground">{m.lockoutNote}</p>
      <Table
        caption={m.restrictionsTitle}
        heads={[m.colSubject, m.colPatterns, m.reason, m.colCreated, ""]}
        empty={m.nothing}
        rows={(list.data?.restrictions ?? []).map((r) => ({
          key: r.id,
          cells: [
            nameOf(r.subjectKind, r.subjectId),
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
          { name: "subject", label: m.subjectKind, options: subjects },
          { name: "patterns", label: m.patterns, required: true },
          { name: "reason", label: m.reason, required: true },
        ]}
        submit={m.addRestriction}
        disabled={!can}
        deniedReason={m.noPermission}
        onSubmit={(v) =>
          run(() =>
            governance.createRestriction({
              subjectKind: splitSubject(v("subject")).kind,
              subjectId: splitSubject(v("subject")).id,
              patterns: splitList(v("patterns")),
              reason: v("reason"),
            }),
          )
        }
      />
    </Frame>
  );
}
