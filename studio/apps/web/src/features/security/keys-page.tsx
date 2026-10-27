// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import type { ReactElement } from "react";
import { Frame, Table } from "./parts.tsx";
import { securityMessages } from "./security.messages.ts";
import { useRootKeys, useScope } from "./use-security.ts";

const runbook = "https://github.com/nightCode42/plux3/blob/main/docs/runbooks/key-ceremony.md";

/** Public key ids, roles and the rotation state of the root metadata; key material is never shown. */
function KeyList({ pins }: { readonly pins: boolean }): ReactElement {
  const m = securityMessages(useLingui().i18n);
  const { app, env } = useScope();
  const result = useRootKeys(app, env);
  const all = result.data?.keys ?? [];
  const isPin = (role: string) => role.toLowerCase().includes("pin");
  const keys = all.filter((k) => isPin(k.role) === pins);
  return (
    <Frame
      id={pins ? "pins-title" : "keys-title"}
      title={pins ? m.pinsTitle : m.keysTitle}
      caption={pins ? m.pinsCaption : m.keysCaption}
      loading={result.isPending}
      failed={result.isError}
    >
      <p className="text-body">
        {m.rootVersions}: <span className="font-mono">{result.data?.roots.length ?? 0}</span>
      </p>
      {keys.length === 0 ? (
        <p className="text-body text-secondary">{pins ? m.noPins : m.noKeys}</p>
      ) : (
        <Table
          caption={pins ? m.pinsTitle : m.keysTitle}
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
      <a href={runbook} className="text-body" rel="noreferrer" target="_blank">
        {m.ceremony}
      </a>
    </Frame>
  );
}

/** A34: signing keys, read-only (O-14). */
export function KeysPage(): ReactElement {
  return <KeyList pins={false} />;
}

/** A35: certificate pins, read-only (O-14). */
export function PinsPage(): ReactElement {
  return <KeyList pins />;
}
