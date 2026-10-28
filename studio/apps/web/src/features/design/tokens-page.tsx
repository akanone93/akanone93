// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Icon, TypeTag } from "@plux/ui";
import { type ReactElement, useState } from "react";
import { designMessages } from "./design.messages.ts";
import { baseRows, contrastOf } from "./design-model.ts";
import { AddToken, EditTokenDialog, ImportDialog, RemoveToken, show } from "./parts.tsx";
import { Frame, useDesign } from "./scope.tsx";

function Rows(): ReactElement {
  const m = designMessages(useLingui().i18n);
  const ctx = useDesign();
  const rows = baseRows(ctx.rows);
  const [editing, setEditing] = useState<readonly string[] | undefined>(undefined);
  const [importing, setImporting] = useState(false);
  return (
    <div className="flex flex-col gap-3">
      <div>
        <Button variant="secondary" disabled={!ctx.canEdit} onClick={() => setImporting(true)}>
          <Icon name="plus" />
          {m.importButton}
        </Button>
      </div>
      {rows.length === 0 ? (
        <p className="text-body text-secondary">{m.noTokens}</p>
      ) : (
        <table aria-label={m.tokensTable} className="w-full text-body">
          <thead>
            <tr className="text-start text-caption text-secondary">
              <th scope="col">{m.colToken}</th>
              <th scope="col">{m.colLight}</th>
              <th scope="col">{m.colDark}</th>
              <th scope="col">{m.colContrast}</th>
              <td />
            </tr>
          </thead>
          <tbody>
            {rows.map((r) => {
              const name = r.path.join(".");
              return (
                <tr key={name}>
                  <th scope="row" className="text-start font-mono font-normal">
                    {name} {r.type !== "" && <TypeTag>{r.type}</TypeTag>}
                  </th>
                  <td>{show(r.light)}</td>
                  <td>{show(r.dark)}</td>
                  <td>{show(contrastOf(ctx.value?.["tokens"], r.path))}</td>
                  <td className="flex gap-1">
                    <Button variant="secondary" aria-label={m.editToken(name)} onClick={() => setEditing(r.path)}>
                      <Icon name="pencil-simple" />
                    </Button>
                    <RemoveToken path={r.path} />
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}
      <AddToken prefix={[]} typed />
      {editing !== undefined && <EditTokenDialog path={editing} onClose={() => setEditing(undefined)} />}
      {importing && <ImportDialog onClose={() => setImporting(false)} />}
    </div>
  );
}

/** A15: the design tokens of the app (STU-112, THM-001). */
export function TokensPage(): ReactElement {
  const m = designMessages(useLingui().i18n);
  return (
    <Frame id="design-tokens" title={m.tokensTitle} caption={m.tokensCaption}>
      <Rows />
    </Frame>
  );
}
