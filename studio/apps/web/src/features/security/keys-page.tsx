// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import type { ReactElement } from "react";
import { Frame, Table } from "./parts.tsx";
import { securityMessages } from "./security.messages.ts";
import { useRootKeys, useScope } from "./use-security.ts";

const runbook = "https://github.com/nightCode42/plux3/blob/main/docs/runbooks/key-ceremony.md";

function Runbook({ label }: { readonly label: string }): ReactElement {
  return (
    <a href={runbook} className="text-body" rel="noreferrer" target="_blank">
      {label}
    </a>
  );
}

/** A34: public key ids, roles and the rotation state of the root metadata, read-only (O-14); key material is never shown. */
export function KeysPage(): ReactElement {
  const m = securityMessages(useLingui().i18n);
  const { app, env } = useScope();
  const result = useRootKeys(app, env);
  const keys = result.data?.keys ?? [];
  return (
    <Frame
      id="keys-title"
      title={m.keysTitle}
      caption={m.keysCaption}
      loading={result.isPending}
      failed={result.isError}
    >
      <p className="text-body">
        {m.rootVersions}: <span className="font-mono">{result.data?.roots.length ?? 0}</span>
      </p>
      {keys.length === 0 ? (
        <p className="text-body text-secondary">{m.noKeys}</p>
      ) : (
        <Table
          caption={m.keysTitle}
          heads={[m.colKeyId, m.colRole, m.colAlgorithm, m.colKind]}
          rows={keys.map((k) => ({
            key: k.keyId,
            cells: [
              <span key="i" className="font-mono">
                {k.keyId}
              </span>,
              k.role,
              k.algorithm,
              k.environmentType,
            ],
          }))}
        />
      )}
      <Runbook label={m.ceremony} />
    </Frame>
  );
}

/** A35: the certificate pins live in the signed root metadata (O-14); no RPC lists them, so none is guessed from key roles. */
export function PinsPage(): ReactElement {
  const m = securityMessages(useLingui().i18n);
  return (
    <Frame id="pins-title" title={m.pinsTitle} caption={m.pinsCaption} loading={false} failed={false}>
      <p className="text-body text-secondary">{m.pinsNote}</p>
      <Runbook label={m.ceremony} />
    </Frame>
  );
}
