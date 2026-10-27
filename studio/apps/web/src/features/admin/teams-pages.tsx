// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { OrgService } from "@plux/api";
import { Button } from "@plux/ui";
import { Link, useParams } from "@tanstack/react-router";
import type { ReactElement } from "react";
import { adminMessages } from "./admin.messages.ts";
import { GrantsSection, RestrictionsSection } from "./grants.tsx";
import { CreateForm, Frame, fmt, Table } from "./parts.tsx";
import { pageSize, useAdminQuery, useCan, useOrgId, useRpc, useRun } from "./use-admin.ts";

/** O18: teams of the organisation (GOV-001). */
export function TeamsPage(): ReactElement {
  const m = adminMessages(useLingui().i18n);
  const organizationId = useOrgId();
  const org = (useParams({ strict: false }) as { org?: string }).org ?? "";
  const api = useRpc(OrgService);
  const can = useCan("teams.manage");
  const { run, failed } = useRun();
  const teams = useAdminQuery(["teams", organizationId], () => api.listTeams({ organizationId, page: { pageSize } }));
  return (
    <Frame
      id="admin-teams"
      title={m.teamsTitle}
      caption={m.teamsCaption}
      loading={teams.isPending}
      failed={teams.isError}
      saveFailed={failed}
    >
      <Table
        caption={m.teamsTitle}
        heads={[m.colName, m.colKey, m.colCreated, ""]}
        empty={m.nothing}
        rows={(teams.data?.teams ?? []).map((t) => ({
          key: t.id,
          cells: [
            <Link key="n" to="/o/$org/admin/teams/$teamId" params={{ org, teamId: t.id }}>
              {t.name}
            </Link>,
            t.key,
            fmt(t.createdAt, m.never),
            <Button
              key="d"
              variant="danger"
              size="sm"
              disabled={!can}
              onClick={() => run(() => api.deleteTeam({ id: t.id }))}
            >
              {m.delete}
            </Button>,
          ],
        }))}
      />
      <CreateForm
        fields={[
          { name: "key", label: m.colKey, required: true },
          { name: "name", label: m.colName, required: true },
        ]}
        submit={m.createTeam}
        disabled={!can}
        deniedReason={m.noPermission}
        onSubmit={(v) => run(() => api.createTeam({ organizationId, key: v("key"), name: v("name") }))}
      />
    </Frame>
  );
}

/** O19: one team's members, grants and restrictions (F14). */
export function TeamPage(): ReactElement {
  const m = adminMessages(useLingui().i18n);
  const teamId = (useParams({ strict: false }) as { teamId?: string }).teamId ?? "";
  const organizationId = useOrgId();
  const api = useRpc(OrgService);
  const teams = useAdminQuery(["teams", organizationId], () => api.listTeams({ organizationId, page: { pageSize } }));
  const members = useAdminQuery(["members", organizationId, teamId], () =>
    api.listMembers({ organizationId, teamId, page: { pageSize } }),
  );
  const team = teams.data?.teams.find((t) => t.id === teamId);
  return (
    <Frame
      id="admin-team"
      title={team?.name ?? m.teamTitle}
      loading={teams.isPending || members.isPending}
      failed={teams.isError || members.isError}
    >
      <h2 className="text-label font-medium">{m.teamMembers}</h2>
      <Table
        caption={m.teamMembers}
        heads={[m.colName, m.colEmail]}
        empty={m.nothing}
        rows={(members.data?.members ?? []).map((x) => ({
          key: x.userId,
          cells: [x.displayName || x.userId, x.email],
        }))}
      />
      <GrantsSection teamId={teamId} />
      <RestrictionsSection subjectId={teamId} />
    </Frame>
  );
}
