// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { IdentityService } from "@plux/api";
import { Button } from "@plux/ui";
import { type ReactElement, useState } from "react";
import { adminMessages } from "./admin.messages.ts";
import { CreateForm, Frame, fmt, SecretPanel, Table } from "./parts.tsx";
import { pageSize, splitList, useAdminQuery, useCan, useOrgId, useRpc, useRun } from "./use-admin.ts";

const secondsPerDay = 86_400n;

/** O25 (D72): API and CI tokens and workload identities; a new token's secret is shown once and kept nowhere (STU-116). */
export function TokensPage(): ReactElement {
  const m = adminMessages(useLingui().i18n);
  const organizationId = useOrgId();
  const identity = useRpc(IdentityService);
  const can = useCan("tokens.manage");
  const { run, failed } = useRun();
  const [secret, setSecret] = useState<string>();
  const tokens = useAdminQuery(["tokens", organizationId], () =>
    identity.listAccessTokens({ organizationId, page: { pageSize } }),
  );
  const workloads = useAdminQuery(["workloads", organizationId], () =>
    identity.listWorkloadIdentities({ organizationId, page: { pageSize } }),
  );
  return (
    <Frame
      id="admin-tokens"
      title={m.tokensTitle}
      caption={m.tokensCaption}
      loading={tokens.isPending || workloads.isPending}
      failed={tokens.isError || workloads.isError}
      saveFailed={failed}
    >
      {secret !== undefined && (
        <SecretPanel
          heading={m.secretHeading}
          note={m.secretNote}
          secret={secret}
          dismiss={m.dismiss}
          onDismiss={() => setSecret(undefined)}
        />
      )}
      <Table
        caption={m.tokensTitle}
        heads={[m.colName, m.colPrefix, m.colExpires, m.colLastUsed, m.colState, ""]}
        empty={m.nothing}
        rows={(tokens.data?.tokens ?? []).map((t) => ({
          key: t.id,
          cells: [
            t.name,
            <span key="p" className="font-mono">
              {t.prefix}
            </span>,
            fmt(t.expiresAt, m.never),
            fmt(t.lastUsedAt, m.never),
            t.revokedAt === undefined ? m.stateActive : m.stateRevoked,
            <Button
              key="r"
              size="sm"
              variant="danger"
              disabled={!can || t.revokedAt !== undefined}
              onClick={() => run(() => identity.revokeAccessToken({ id: t.id }))}
            >
              {m.revoke}
            </Button>,
          ],
        }))}
      />
      <CreateForm
        fields={[
          { name: "name", label: m.tokenName, required: true },
          { name: "scopes", label: m.tokenScopes, required: true },
          { name: "days", label: m.tokenDays, type: "number", initial: "30", required: true },
        ]}
        submit={m.createToken}
        disabled={!can}
        deniedReason={m.noPermission}
        onSubmit={async (v) => {
          let issued = "";
          const ok = await run(async () => {
            issued = (
              await identity.createAccessToken({
                name: v("name"),
                organizationId,
                scopes: splitList(v("scopes")),
                ttlSeconds: BigInt(Number(v("days")) || 1) * secondsPerDay,
              })
            ).secret;
          });
          setSecret(ok ? issued : undefined);
          return ok;
        }}
      />
      <h2 className="text-label font-medium">{m.workloads}</h2>
      <Table
        caption={m.workloads}
        heads={[m.issuer, m.audience, m.subjectPattern, m.tokenScopes, ""]}
        empty={m.nothing}
        rows={(workloads.data?.identities ?? []).map((w) => ({
          key: w.id,
          cells: [
            w.issuer,
            w.audience,
            w.subjectPattern,
            w.scopes.join(", "),
            <Button
              key="d"
              size="sm"
              variant="danger"
              disabled={!can}
              onClick={() => run(() => identity.deleteWorkloadIdentity({ id: w.id }))}
            >
              {m.delete}
            </Button>,
          ],
        }))}
      />
      <CreateForm
        fields={[
          { name: "issuer", label: m.issuer, required: true },
          { name: "audience", label: m.audience, required: true },
          { name: "subject", label: m.subjectPattern, required: true },
          { name: "scopes", label: m.tokenScopes, required: true },
        ]}
        submit={m.createWorkload}
        disabled={!can}
        deniedReason={m.noPermission}
        onSubmit={(v) =>
          run(() =>
            identity.createWorkloadIdentity({
              organizationId,
              issuer: v("issuer"),
              audience: v("audience"),
              subjectPattern: v("subject"),
              scopes: splitList(v("scopes")),
            }),
          )
        }
      />
    </Frame>
  );
}
