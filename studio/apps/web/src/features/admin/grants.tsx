// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { GovernanceService } from "@plux/api";
import { Button } from "@plux/ui";
import type { ReactElement } from "react";
import { adminMessages } from "./admin.messages.ts";
import { CreateForm, fmt, Table } from "./parts.tsx";
import { pageSize, useAdminQuery, useCan, useRpc, useRun } from "./use-admin.ts";

/** The grants of one person or team (GOV-001, GOV-003): list, add and remove, each audited by the server. */
export function GrantsSection({
  userId,
  teamId,
}: {
  readonly userId?: string;
  readonly teamId?: string;
}): ReactElement {
  const m = adminMessages(useLingui().i18n);
  const governance = useRpc(GovernanceService);
  const can = useCan("roles.manage");
  const { run, failed } = useRun();
  const key = userId ?? teamId ?? "";
  const grants = useAdminQuery(["grants", key], () =>
    governance.listGrants({ userId: userId ?? "", teamId: teamId ?? "", page: { pageSize } }),
  );
  const roles = useAdminQuery(["roles"], () => governance.listRoles({ page: { pageSize } }));
  const roleOptions = (roles.data?.roles ?? []).map((r) => ({ value: r.key, label: r.name }));
  return (
    <section aria-labelledby={`grants-${key}`} className="flex flex-col gap-3">
      <h2 id={`grants-${key}`} className="text-label font-medium">
        {m.grantsHeading}
      </h2>
      <Table
        caption={m.grantsHeading}
        heads={[m.colRole, m.grantScope, m.grantApp, m.colAdded, ""]}
        empty={m.nothing}
        rows={(grants.data?.grants ?? []).map((g) => ({
          key: g.id,
          cells: [
            g.role,
            g.scope,
            g.appId,
            fmt(g.createdAt, m.never),
            <Button
              key="d"
              variant="danger"
              size="sm"
              disabled={!can}
              onClick={() => run(() => governance.deleteGrant({ id: g.id }))}
            >
              {m.remove}
            </Button>,
          ],
        }))}
      />
      <CreateForm
        fields={[
          { name: "role", label: m.grantRole, options: roleOptions },
          { name: "appId", label: m.grantApp },
        ]}
        submit={m.addGrant}
        disabled={!can || roleOptions.length === 0}
        deniedReason={m.noPermission}
        onSubmit={(v) =>
          run(() =>
            governance.createGrant({
              userId: userId ?? "",
              teamId: teamId ?? "",
              role: v("role"),
              scope: v("appId") === "" ? "organization" : "app",
              appId: v("appId"),
            }),
          )
        }
      />
      {failed && (
        <p role="alert" className="text-body text-danger">
          {m.saveFailed}
        </p>
      )}
    </section>
  );
}

/** The restrictions that apply to one person or team (GOV-006); creation and removal live on the restrictions page. */
export function RestrictionsSection({ subjectId }: { readonly subjectId: string }): ReactElement {
  const m = adminMessages(useLingui().i18n);
  const governance = useRpc(GovernanceService);
  const list = useAdminQuery(["restrictions", subjectId], () =>
    governance.listRestrictions({ subjectId, page: { pageSize } }),
  );
  return (
    <section aria-labelledby={`restrictions-${subjectId}`} className="flex flex-col gap-3">
      <h2 id={`restrictions-${subjectId}`} className="text-label font-medium">
        {m.restrictionsHeading}
      </h2>
      <Table
        caption={m.restrictionsHeading}
        heads={[m.colPatterns, m.reason]}
        empty={m.nothing}
        rows={(list.data?.restrictions ?? []).map((r) => ({ key: r.id, cells: [r.patterns.join(", "), r.reason] }))}
      />
    </section>
  );
}
