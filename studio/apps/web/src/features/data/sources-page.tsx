// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Dialog } from "@plux/ui";
import { Link } from "@tanstack/react-router";
import { type ReactElement, useState } from "react";
import { draftDocument, type ImportAnswer, ImportSection, withVariables } from "../designers/data-source-overlay.tsx";
import { objects } from "../designers/db-schema-overlay.tsx";
import { designerMessages } from "../designers/designers.messages.ts";
import { findDocument, type Json, useApply, useDocumentValue } from "../designers/use-document.ts";
import { Table } from "../security/parts.tsx";
import { dataMessages } from "./data.messages.ts";
import { Frame } from "./frame.tsx";
import { CommitBar } from "./parts.tsx";
import { useDataContext } from "./scope.tsx";
import { useCommit, usePluginJson } from "./use-draft.ts";

/** D59: import an OpenAPI or GraphQL definition (DAT-002). The importer runs in the Go module; its answer is data, merged into the draft and applied only once the validator accepts it. */
function ImportDialog(props: { readonly open: boolean; readonly onClose: () => void }): ReactElement {
  const i18n = useLingui().i18n;
  const m = dataMessages(i18n);
  const d = designerMessages(i18n);
  const ctx = useDataContext();
  const { value } = usePluginJson();
  const [staged, setStaged] = useState<ImportAnswer | undefined>(undefined);
  const appPath = findDocument(ctx.appDocuments, "app.json")?.path;
  const applyApp = useApply(appPath, "app-variables", ctx.appSession);
  const { value: appValue } = useDocumentValue(ctx.appDocuments, "app.json", ctx.appSession);
  const known = new Set(objects(value?.["dataSources"]).map((s) => s["id"]));
  const next =
    value === undefined || staged === undefined
      ? undefined
      : draftDocument(
          value,
          [...objects(value["dataSources"]), ...staged.dataSources.filter((s) => !known.has(s["id"]))],
          staged.types,
        );
  const state = useCommit(next, "import");
  const blocked = (staged?.variables.length ?? 0) > 0 && (!applyApp.canEdit || appValue === undefined);
  return (
    <Dialog
      open={props.open}
      onOpenChange={(open) => {
        if (!open) {
          props.onClose();
        }
      }}
      title={m.importTitle}
      labels={{ close: m.importClose }}
    >
      <div className="flex flex-col gap-3">
        <ImportSection tool={ctx.tool} canEdit={state.canEdit} m={d} onAdd={setStaged} />
        {staged !== undefined && <p className="text-body text-secondary">{m.importStaged}</p>}
        {blocked && <p className="text-caption text-secondary">{m.importLocked}</p>}
        <CommitBar
          state={{
            ...state,
            canApply: state.canApply && !blocked,
            commit: () => {
              state.commit();
              if (staged !== undefined && staged.variables.length > 0 && appValue !== undefined) {
                applyApp.apply(withVariables(appValue, staged.variables));
              }
              setStaged(undefined);
            },
          }}
          m={m}
          d={d}
        />
      </div>
    </Dialog>
  );
}

function Rows(): ReactElement {
  const m = dataMessages(useLingui().i18n);
  const ctx = useDataContext();
  const { value } = usePluginJson();
  const sources = objects(value?.["dataSources"]);
  if (sources.length === 0) {
    return <p className="text-body text-secondary">{m.noSources}</p>;
  }
  return (
    <Table
      caption={m.listTitle}
      heads={[m.colName, m.colKind, m.colType, m.colRoute, ""]}
      rows={sources.map((s: Json) => ({
        key: String(s["id"]),
        cells: [
          String(s["name"] ?? ""),
          String(s["kind"] ?? ""),
          String(s["type"] ?? ""),
          String(s["route"] ?? m.routeDefault),
          <Link
            key="open"
            to="/o/$org/apps/$app/data/sources/$sourceId"
            params={{ org: ctx.org, app: ctx.app, sourceId: String(s["id"]) } as never}
            search={{ env: ctx.env, plugin: ctx.plugin }}
          >
            {m.open}
          </Link>,
        ],
      }))}
    />
  );
}

function ListBody(): ReactElement {
  const m = dataMessages(useLingui().i18n);
  const [importing, setImporting] = useState(false);
  return (
    <>
      <div>
        <Button variant="secondary" onClick={() => setImporting(true)}>
          {m.importButton}
        </Button>
      </div>
      <Rows />
      <ImportDialog open={importing} onClose={() => setImporting(false)} />
    </>
  );
}

/** A20: the API definitions (data sources) of the app's plugin (STU-113, DAT-002, DAT-080). */
export function DataPage(): ReactElement {
  const m = dataMessages(useLingui().i18n);
  return (
    <Frame id="data-title" title={m.listTitle} caption={m.listCaption}>
      <ListBody />
    </Frame>
  );
}
