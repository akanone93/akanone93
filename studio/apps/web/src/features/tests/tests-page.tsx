// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { timestampDate } from "@bufbuild/protobuf/wkt";
import { useLingui } from "@lingui/react";
import type { TestRun } from "@plux/api";
import { Button, Field, Input, StatusPill } from "@plux/ui";
import { Link, useNavigate, useParams, useSearch } from "@tanstack/react-router";
import { type ReactElement, useState } from "react";
import { useApp } from "../../lib/use-app.tsx";
import { EditorProvider, useCanEdit, useEditSession } from "../editor/editor-provider.tsx";
import { usePluginDocuments } from "../plugin-canvas/use-plugin-documents.ts";
import { Frame } from "../releases/frame.tsx";
import { Table } from "../security/parts.tsx";
import { usePermissions, useScope } from "../security/use-security.ts";
import { ScenarioForm } from "./scenario-form.tsx";
import { type TestsMessages, testsMessages } from "./tests.messages.ts";
import {
  appendRecipe,
  buildDocument,
  type Draft,
  emptyDraft,
  fileOf,
  filterRows,
  pathOf,
  problems,
  removeRecipe,
  scenarioOf,
} from "./tests.model.ts";
import { useScenarioFiles } from "./use-scenarios.ts";
import { useTestRuns } from "./use-tests.ts";

/** A45: the app's Plux Test scenarios, authored as documents, and the runs reported for the app (TST-001, TST-002). */
export function TestsPage(): ReactElement {
  const { app } = useScope();
  const docs = usePluginDocuments(app, "");
  return (
    <EditorProvider appId={app} pluginId="" documents={docs.data} canEdit={usePermissions().has("plugin.edit")}>
      <TestsView documents={docs.data} loading={docs.isPending} failed={docs.isError} />
    </EditorProvider>
  );
}

const pillOf = (failed: number) => (failed > 0 ? "danger" : "success");

function RunsTable({
  m,
  runs,
  org,
  app,
  env,
}: {
  m: TestsMessages;
  runs: readonly TestRun[];
  org: string;
  app: string;
  env: string;
}) {
  if (runs.length === 0) {
    return <p className="text-body text-secondary">{m.noRuns}</p>;
  }
  return (
    <Table
      caption={m.runsHeading}
      heads={[m.colRun, m.colSource, m.colResult, m.colWhen]}
      rows={runs.map((r) => ({
        key: r.id,
        cells: [
          <Link
            key="l"
            to="/o/$org/apps/$app/tests/$runId"
            params={{ org, app, runId: r.id } as never}
            search={{ env }}
          >
            {r.label === "" ? r.reportKey : r.label}
          </Link>,
          r.source,
          <StatusPill key="s" status={pillOf(r.failed)} label={`${r.passed}/${r.passed + r.failed + r.skipped}`} />,
          r.createdAt === undefined ? m.never : timestampDate(r.createdAt).toLocaleString(),
        ],
      }))}
    />
  );
}

function useScenarioActions(app: string, m: TestsMessages, known: ReadonlySet<string>) {
  const edit = useEditSession();
  const { session } = useApp();
  const [note, setNote] = useState<{ ok: boolean; text: string } | null>(null);
  const refresh = () => session.queryClient.invalidateQueries({ queryKey: ["plugin-documents", app, ""] });
  const save = async (draft: Draft) => {
    const path = pathOf(draft.file);
    const ok = known.has(path)
      ? edit.edit(path, appendRecipe(scenarioOf(draft))) !== undefined
      : (await edit.create(path, buildDocument(draft))).ok;
    setNote({ ok, text: ok ? m.saved : m.saveFailed });
    await refresh();
    return ok;
  };
  const remove = async (path: string, name: string, only: boolean) => {
    const done = only ? (await edit.remove(path)).ok : edit.edit(path, removeRecipe(name)) !== undefined;
    setNote({ ok: done, text: done ? m.removed : m.saveFailed });
    await refresh();
  };
  return { save, remove, note };
}

function TestsView(props: {
  readonly documents: ReturnType<typeof usePluginDocuments>["data"];
  readonly loading: boolean;
  readonly failed: boolean;
}): ReactElement {
  const m = testsMessages(useLingui().i18n);
  const { app, env } = useScope();
  const org = (useParams({ strict: false }) as { org?: string }).org ?? "";
  const q = (useSearch({ strict: false }) as { q?: string }).q;
  const navigate = useNavigate();
  const files = useScenarioFiles(props.documents);
  const runs = useTestRuns(app);
  const canEdit = useCanEdit();
  const mayEdit = usePermissions().has("plugin.edit") && canEdit;
  const actions = useScenarioActions(app, m, new Set(files.keys()));
  const [draft, setDraft] = useState<Draft>(emptyDraft);
  const rows = filterRows([...files.values()].flat(), q);
  return (
    <Frame id="tests-title" title={m.listTitle} caption={m.listCaption} loading={props.loading} failed={props.failed}>
      <Field label={m.searchLabel}>
        <Input
          value={q ?? ""}
          onChange={(e) =>
            void navigate({
              to: ".",
              replace: true,
              search: (s: object) => ({ ...s, q: e.target.value === "" ? undefined : e.target.value }),
            } as never)
          }
        />
      </Field>
      <h2 className="text-label font-medium">{m.scenariosHeading}</h2>
      {rows.length === 0 ? (
        <p className="text-body text-secondary">{m.noScenarios}</p>
      ) : (
        <Table
          caption={m.scenariosHeading}
          heads={[m.colName, m.colTarget, m.colFile, m.colSteps, m.colExpectations, m.colActions]}
          rows={rows.map((r) => ({
            key: `${r.path}:${r.name}`,
            cells: [
              r.name,
              r.target,
              fileOf(r.path),
              r.steps,
              r.expectations,
              mayEdit && (
                <Button
                  key="x"
                  variant="ghost"
                  aria-label={`${m.remove} ${r.name}`}
                  onClick={() => void actions.remove(r.path, r.name, files.get(r.path)?.length === 1)}
                >
                  {m.remove}
                </Button>
              ),
            ],
          }))}
        />
      )}
      {actions.note !== null && (
        <p role={actions.note.ok ? "status" : "alert"} className="text-body">
          {actions.note.text}
        </p>
      )}
      {mayEdit ? (
        <ScenarioForm
          m={m}
          draft={draft}
          problems={problems(draft)}
          busy={false}
          onChange={setDraft}
          onSave={() => void actions.save(draft).then((ok) => ok && setDraft(emptyDraft))}
        />
      ) : (
        <p className="text-body text-secondary">{m.readOnly}</p>
      )}
      <h2 className="text-label font-medium">{m.runsHeading}</h2>
      {runs.isPending ? null : <RunsTable m={m} runs={runs.data ?? []} org={org} app={app} env={env} />}
    </Frame>
  );
}
