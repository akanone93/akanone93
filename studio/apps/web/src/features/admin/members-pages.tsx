// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { GovernanceService, IdentityService, OrgService } from "@plux/api";
import { Button } from "@plux/ui";
import { Link, useParams } from "@tanstack/react-router";
import { type ReactElement, useState } from "react";
import { TypeConfirmDialog } from "../../components/dialogs/index.ts";
import { adminMessages } from "./admin.messages.ts";
import { GrantsSection, RestrictionsSection } from "./grants.tsx";
import { CreateForm, Frame, fmt, SecretPanel, Table } from "./parts.tsx";
import { pageSize, useAdminQuery, useCan, useOrgId, useRpc, useRun } from "./use-admin.ts";

/** O16: members and invitations; an invitation link is shown once and kept nowhere (STU-116). */
export function MembersPage(): ReactElement {
  const m = adminMessages(useLingui().i18n);
  const organizationId = useOrgId();
  const org = (useParams({ strict: false }) as { org?: string }).org ?? "";
  const orgApi = useRpc(OrgService);
  const governance = useRpc(GovernanceService);
  const can = useCan("members.manage");
  const { run, failed } = useRun();
  const [invitation, setInvitation] = useState<string>();
  const [removing, setRemoving] = useState<string>();
  const members = useAdminQuery(["members", organizationId], () =>
    orgApi.listMembers({ organizationId, page: { pageSize } }),
  );
  const roles = useAdminQuery(["roles"], () => governance.listRoles({ page: { pageSize } }));
  const roleOptions = (roles.data?.roles ?? []).map((r) => ({ value: r.key, label: r.name }));
  return (
    <Frame
      id="admin-members"
      title={m.membersTitle}
      caption={m.membersCaption}
      loading={members.isPending}
      failed={members.isError}
      saveFailed={failed}
    >
      <Table
        caption={m.membersTitle}
        heads={[m.colName, m.colEmail, m.colRole, m.colAdded, ""]}
        empty={m.nothing}
        rows={(members.data?.members ?? []).map((x) => ({
          key: x.userId,
          cells: [
            <Link key="n" to="/o/$org/admin/members/$userId" params={{ org, userId: x.userId }}>
              {x.displayName === "" ? x.userId : x.displayName}
            </Link>,
            x.email,
            x.role,
            fmt(x.addedAt, m.never),
            <Button key="r" variant="danger" size="sm" disabled={!can} onClick={() => setRemoving(x.userId)}>
              {m.removeMember}
            </Button>,
          ],
        }))}
      />
      <CreateForm
        fields={[
          { name: "email", label: m.inviteEmail, required: true },
          { name: "role", label: m.grantRole, options: roleOptions },
        ]}
        submit={m.invite}
        disabled={!can || roleOptions.length === 0}
        deniedReason={m.noPermission}
        onSubmit={async (v) => {
          let link = "";
          const ok = await run(async () => {
            link = (await orgApi.addMember({ organizationId, email: v("email"), role: v("role") })).invitation;
          });
          setInvitation(ok && link !== "" ? link : undefined);
          return ok;
        }}
      />
      {invitation !== undefined && (
        <SecretPanel
          heading={m.inviteLink}
          note={m.secretNote}
          secret={invitation}
          dismiss={m.dismiss}
          onDismiss={() => setInvitation(undefined)}
        />
      )}
      <TypeConfirmDialog
        open={removing !== undefined}
        onOpenChange={(open) => !open && setRemoving(undefined)}
        phrase={members.data?.members.find((x) => x.userId === removing)?.email || (removing ?? "")}
        description={m.removeMemberDesc}
        onConfirm={() => {
          const userId = removing ?? "";
          setRemoving(undefined);
          run(() => orgApi.removeMember({ organizationId, userId }));
        }}
      />
    </Frame>
  );
}

/** O17: one member's grants, restrictions and sessions, with a way into "explain access" (F14, GOV-005). */
export function MemberPage(): ReactElement {
  const m = adminMessages(useLingui().i18n);
  const { org = "", userId = "" } = useParams({ strict: false }) as { org?: string; userId?: string };
  const organizationId = useOrgId();
  const orgApi = useRpc(OrgService);
  const identity = useRpc(IdentityService);
  const can = useCan("sessions.manage");
  const { run, failed } = useRun();
  const members = useAdminQuery(["members", organizationId], () =>
    orgApi.listMembers({ organizationId, page: { pageSize } }),
  );
  const sessions = useAdminQuery(["sessions", organizationId], () =>
    identity.listSessions({ organizationId, page: { pageSize } }),
  );
  const member = members.data?.members.find((x) => x.userId === userId);
  return (
    <Frame
      id="admin-member"
      title={member?.displayName || member?.email || m.memberTitle}
      loading={members.isPending}
      failed={members.isError}
      saveFailed={failed}
    >
      <Link to="/o/$org/admin/access" params={{ org }} search={{ user: userId }}>
        {m.explainLink}
      </Link>
      <GrantsSection userId={userId} />
      <RestrictionsSection subjectId={userId} />
      <h2 className="text-label font-medium">{m.sessionsHeading}</h2>
      <Table
        caption={m.sessionsHeading}
        heads={[m.colClient, m.colSeen, m.colNetwork, ""]}
        empty={m.nothing}
        rows={(sessions.data?.sessions ?? [])
          .filter((s) => s.userId === userId)
          .map((s) => ({
            key: s.id,
            cells: [
              s.clientKind,
              fmt(s.lastSeenAt, m.never),
              s.ipPrefix,
              <Button
                key="r"
                variant="danger"
                size="sm"
                disabled={!can}
                onClick={() => run(() => identity.revokeSession({ id: s.id }))}
              >
                {m.revoke}
              </Button>,
            ],
          }))}
      />
    </Frame>
  );
}
