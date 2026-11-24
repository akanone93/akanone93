// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Icon, Input, Tooltip } from "@plux/ui";
import { type ReactElement, type ReactNode, useState } from "react";
import { ColourChip } from "../../components/colour-field.tsx";
import { DataTable, PageEmpty } from "../../components/page.tsx";
import { download } from "../admin/download.ts";
import { type DesignMessages, designMessages } from "./design.messages.ts";
import { baseRows, contrastOf, tokenText } from "./design-model.ts";
import { AddToken, EditTokenDialog, ImportDialog, RemoveToken } from "./parts.tsx";
import { Frame, useDesign } from "./scope.tsx";

/** One mode's value of a token: a colour as a swatch and its code, anything else as code, a dash when there is none. */
function ValueCell({ type, value }: { readonly type: string; readonly value: unknown }): ReactNode {
  if (value === undefined) {
    return (
      <span aria-hidden="true" className="text-muted-foreground">
        —
      </span>
    );
  }
  const text = tokenText(type, value);
  return type === "color" ? (
    <ColourChip value={text} />
  ) : (
    <Tooltip content={text}>
      <span className="token-value">{text}</span>
    </Tooltip>
  );
}

/** The theme's tokens as a DTCG file, as the import reads it back (STU-112). */
function exportTokens(tokens: unknown, name: string): void {
  const text = `${JSON.stringify(tokens ?? {}, null, 2)}\n`;
  download(`${name}.tokens.json`, new TextEncoder().encode(text), "application/json");
}

function Actions(props: { readonly m: DesignMessages; readonly onImport: () => void }): ReactElement {
  const ctx = useDesign();
  const tokens = ctx.value?.["tokens"];
  const key = typeof ctx.value?.["key"] === "string" ? ctx.value["key"] : "theme";
  return (
    <div className="flex flex-wrap gap-2">
      <Button variant="secondary" disabled={!ctx.canEdit} onClick={props.onImport}>
        <Icon name="upload" />
        {props.m.importButton}
      </Button>
      <Button variant="secondary" disabled={ctx.rows.length === 0} onClick={() => exportTokens(tokens, key)}>
        <Icon name="download" />
        {props.m.exportButton}
      </Button>
    </div>
  );
}

type Row = ReturnType<typeof baseRows>[number];

/** One group of tokens (`color`, `spacing`, …) as a table under its name and count. */
function Group(props: {
  readonly group: string;
  readonly rows: readonly Row[];
  readonly tokens: unknown;
  readonly onEdit: (path: readonly string[]) => void;
}): ReactElement {
  const m = designMessages(useLingui().i18n);
  return (
    <section aria-label={props.group} className="flex flex-col gap-2">
      <h2 className="token-group-title">
        <span className="font-mono">{props.group}</span>
        <span className="text-muted-foreground">{props.rows.length}</span>
      </h2>
      <DataTable
        actions
        caption={props.group}
        heads={[m.colToken, m.newType, m.colLight, m.colDark, m.colContrast, m.colActions]}
        rows={props.rows.map((r) => {
          const name = r.path.join(".");
          return {
            key: name,
            cells: [
              <span key="name" className="font-mono text-body">
                {name}
              </span>,
              r.type === "" ? null : (
                <span key="type" className="token-type">
                  {r.type}
                </span>
              ),
              <ValueCell key="light" type={r.type} value={r.light} />,
              <ValueCell key="dark" type={r.type} value={r.dark} />,
              <ValueCell key="contrast" type={r.type} value={contrastOf(props.tokens, r.path)} />,
              <span key="actions" className="flex justify-end gap-1">
                <Button variant="ghost" size="sm" aria-label={m.editToken(name)} onClick={() => props.onEdit(r.path)}>
                  <Icon name="pencil-simple" />
                </Button>
                <RemoveToken path={r.path} base />
              </span>,
            ],
          };
        })}
      />
    </section>
  );
}

function Rows(): ReactElement {
  const m = designMessages(useLingui().i18n);
  const ctx = useDesign();
  const tokens = ctx.value?.["tokens"];
  const [query, setQuery] = useState("");
  const [editing, setEditing] = useState<readonly string[] | undefined>(undefined);
  const [importing, setImporting] = useState(false);
  const all = baseRows(ctx.rows);
  const q = query.trim().toLowerCase();
  const rows = q === "" ? all : all.filter((r) => r.path.join(".").toLowerCase().includes(q) || r.type === q);
  const groups = [...new Set(rows.map((r) => r.path[0] ?? ""))];
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-2">
        <Actions m={m} onImport={() => setImporting(true)} />
        {all.length > 0 && (
          <div className="ms-auto w-64">
            <Input
              type="search"
              aria-label={m.searchTokens}
              placeholder={m.searchTokens}
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />
          </div>
        )}
      </div>
      {all.length === 0 ? (
        <PageEmpty icon="palette" text={m.noTokens} />
      ) : groups.length === 0 ? (
        <PageEmpty icon="magnifying-glass" text={m.noTokenMatches} />
      ) : (
        groups.map((g) => (
          <Group key={g} group={g} rows={rows.filter((r) => r.path[0] === g)} tokens={tokens} onEdit={setEditing} />
        ))
      )}
      <section aria-label={m.addTokenHeading} className="design-card">
        <h2 className="text-label font-medium">{m.addTokenHeading}</h2>
        <AddToken prefix={[]} typed />
      </section>
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
