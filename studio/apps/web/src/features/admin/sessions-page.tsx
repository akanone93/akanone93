// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { IdentityService } from "@plux/api";
import { Button } from "@plux/ui";
import type { ReactElement } from "react";
import { adminMessages } from "./admin.messages.ts";
import { Frame, fmt, Table } from "./parts.tsx";
import { pageSize, useAdminQuery, useCan, useOrgId, useRpc, useRun } from "./use-admin.ts";

/** O26: active sessions of the organisation's people; revoking signs a session out (GOV-005). */
export function SessionsPage(): ReactElement {
  const m = adminMessages(useLingui().i18n);
  const organizationId = useOrgId();
  const identity = useRpc(IdentityService);
  const can = useCan("sessions.manage");
  const { run, failed } = useRun();
  const sessions = useAdminQuery(["sessions", organizationId], () =>
    identity.listSessions({ organizationId, page: { pageSize } }),
  );
  return (
    <Frame
      id="admin-sessions"
      title={m.sessionsTitle}
      caption={m.sessionsCaption}
      loading={sessions.isPending}
      failed={sessions.isError}
      saveFailed={failed}
    >
      <Table
        caption={m.sessionsTitle}
        heads={[m.colUser, m.colClient, m.colSeen, m.colExpires, m.colNetwork, ""]}
        empty={m.nothing}
        rows={(sessions.data?.sessions ?? []).map((s) => ({
          key: s.id,
          cells: [
            s.userId,
            s.clientKind,
            fmt(s.lastSeenAt, m.never),
            fmt(s.expiresAt, m.never),
            s.ipPrefix,
            <Button
              key="r"
              size="sm"
              variant="danger"
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
