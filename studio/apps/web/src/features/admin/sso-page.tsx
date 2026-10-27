// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { FederationService } from "@plux/api";
import { Button } from "@plux/ui";
import { type ReactElement, useState } from "react";
import { adminMessages } from "./admin.messages.ts";
import { CreateForm, Frame, fmt, SecretPanel, Table } from "./parts.tsx";
import { pageSize, useAdminQuery, useCan, useRpc, useRun } from "./use-admin.ts";

/** O24: the organisation's identity providers (read-only here) and its SCIM tokens; a secret is shown once (GOV-004). */
export function SsoPage(): ReactElement {
  const m = adminMessages(useLingui().i18n);
  const federation = useRpc(FederationService);
  const can = useCan("sso.manage");
  const { run, failed } = useRun();
  const [secret, setSecret] = useState<string>();
  const providers = useAdminQuery(["providers"], () => federation.listIdentityProviders({ page: { pageSize } }));
  const tokens = useAdminQuery(["scim-tokens"], () => federation.listScimTokens({ page: { pageSize } }));
  const issue = async (change: () => Promise<{ secret: string }>) => {
    let issued = "";
    const ok = await run(async () => {
      issued = (await change()).secret;
    });
    setSecret(ok ? issued : undefined);
    return ok;
  };
  return (
    <Frame
      id="admin-sso"
      title={m.ssoTitle}
      caption={m.ssoCaption}
      loading={providers.isPending || tokens.isPending}
      failed={providers.isError || tokens.isError}
      saveFailed={failed}
    >
      <h2 className="text-label font-medium">{m.providers}</h2>
      <Table
        caption={m.providers}
        heads={[m.colName, m.colKind, m.colEnabled]}
        empty={m.nothing}
        rows={(providers.data?.providers ?? []).map((p) => ({
          key: p.id,
          cells: [p.name, p.kind, p.enabled ? m.yes : m.no],
        }))}
      />
      <h2 className="text-label font-medium">{m.scimTokens}</h2>
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
        caption={m.scimTokens}
        heads={[m.colName, m.colPrefix, m.colExpires, m.colState, ""]}
        empty={m.nothing}
        rows={(tokens.data?.tokens ?? []).map((t) => ({
          key: t.id,
          cells: [
            t.name,
            <span key="p" className="font-mono">
              {t.prefix}
            </span>,
            fmt(t.expiresAt, m.never),
            t.revokedAt === undefined ? m.stateActive : m.stateRevoked,
            <span key="a" className="flex gap-2">
              <Button
                size="sm"
                variant="secondary"
                disabled={!can || t.revokedAt !== undefined}
                onClick={() => issue(() => federation.rotateScimToken({ id: t.id, overlapSeconds: 3600n }))}
              >
                {m.rotate}
              </Button>
              <Button
                size="sm"
                variant="danger"
                disabled={!can || t.revokedAt !== undefined}
                onClick={() => run(() => federation.revokeScimToken({ id: t.id }))}
              >
                {m.revoke}
              </Button>
            </span>,
          ],
        }))}
      />
      <CreateForm
        fields={[{ name: "name", label: m.tokenName, required: true }]}
        submit={m.createScim}
        disabled={!can}
        deniedReason={m.noPermission}
        onSubmit={(v) => issue(() => federation.createScimToken({ name: v("name") }))}
      />
    </Frame>
  );
}
