// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { PluginService } from "@plux/api";
import { Button, Icon, Input, Meter, MetricStrip, type StatusKind, StatusPill, TypeTag } from "@plux/ui";
import { useQuery } from "@tanstack/react-query";
import { useParams } from "@tanstack/react-router";
import { type FormEvent, type KeyboardEvent, useMemo, useState } from "react";
import { useApp } from "../../lib/use-app.tsx";
import { extrasOf } from "../action-graph/graph-model.ts";
import { type DesignerDocument, useDocumentValue } from "../designers/use-document.ts";
import { useCanEdit, useEditSession } from "../editor/editor-provider.tsx";
import {
  addEntryRecipe,
  nameProblem,
  parseDefault,
  readState,
  type StateEntry,
  setFieldRecipe,
} from "../state-designer/state-model.ts";
import {
  type BudgetStatus,
  budgetStatus,
  compactJson,
  functionsIn,
  nodeBudget,
  stringsIn,
  subtreeOf,
} from "./tab-model.ts";
import { tabMessages } from "./tabs.messages.ts";

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);
const objects = (v: unknown): Obj[] => (Array.isArray(v) ? v.filter(isObj) : []);

const empty = (text: string) => <p className="text-body text-secondary">{text}</p>;

export interface SummaryTabProps {
  readonly documents: readonly DesignerDocument[] | undefined;
  readonly onOpen: (() => void) | undefined;
}

/** One state entry with inline rename and initial-value fields; each commits on Enter or blur through the edit pipeline. */
function StateRow({
  entry,
  names,
  path,
}: {
  readonly entry: StateEntry;
  readonly names: readonly string[];
  readonly path: string | undefined;
}) {
  const m = tabMessages(useLingui().i18n);
  const session = useEditSession();
  const canEdit = useCanEdit() && path !== undefined;
  const [name, setName] = useState(entry.name);
  const [text, setText] = useState(entry.defaultText);
  const nameBad = nameProblem(
    name,
    names.filter((n) => n !== entry.name),
  );
  const defaultBad = parseDefault(text) === undefined;
  const commit = (field: string, value: unknown) => {
    if (path !== undefined) {
      session.edit(path, setFieldRecipe(entry.id, field, value), { group: `state:${entry.id}:${field}` });
    }
  };
  const commitName = () => {
    if (nameBad === undefined && name !== entry.name) {
      commit("name", name);
    }
  };
  const commitDefault = () => {
    const parsed = parseDefault(text);
    if (parsed !== undefined && text !== entry.defaultText) {
      commit("default", parsed === null ? undefined : parsed.value);
    }
  };
  const onEnter = (fn: () => void) => (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter") {
      fn();
    }
  };
  return (
    <li className="flex flex-col gap-1 text-body">
      <div className="flex items-center gap-2">
        <Input
          aria-label={m.stateRename(entry.name)}
          value={name}
          dir="ltr"
          readOnly={!canEdit}
          aria-invalid={nameBad !== undefined}
          onChange={(e) => setName(e.target.value)}
          onBlur={commitName}
          onKeyDown={onEnter(commitName)}
        />
        <TypeTag>{entry.type}</TypeTag>
      </div>
      {!entry.computed && (
        <Input
          aria-label={m.stateDefault(entry.name)}
          value={text}
          dir="ltr"
          readOnly={!canEdit}
          aria-invalid={defaultBad}
          onChange={(e) => setText(e.target.value)}
          onBlur={commitDefault}
          onKeyDown={onEnter(commitDefault)}
        />
      )}
      {nameBad !== undefined && <span className="text-caption text-secondary">{m.stateNameProblem}</span>}
      {defaultBad && <span className="text-caption text-secondary">{m.stateNotJson}</span>}
    </li>
  );
}

/** The State tab of the left panel (STU-061): the plugin's state with inline add, rename and initial-value forms, and the state designer. */
export function StateTab({ documents, onOpen }: SummaryTabProps) {
  const m = tabMessages(useLingui().i18n);
  const session = useEditSession();
  const canEdit = useCanEdit();
  const { path, value } = useDocumentValue(documents, "plugin.json");
  const entries = readState(value);
  const computed = entries.filter((e) => e.computed).length;
  const [fresh, setFresh] = useState("");
  const names = entries.map((e) => e.name);
  const freshBad = nameProblem(fresh, names);
  const add = (e: FormEvent) => {
    e.preventDefault();
    if (canEdit && path !== undefined && freshBad === undefined) {
      session.edit(path, addEntryRecipe({ id: crypto.randomUUID(), name: fresh, type: "string" }));
      setFresh("");
    }
  };
  return (
    <div className="flex flex-col gap-2">
      {entries.length === 0 ? (
        empty(m.stateEmpty)
      ) : (
        <>
          <p className="text-body">
            {m.stateSummary(entries.length)}, {m.stateComputed(computed)}
          </p>
          <ul aria-label={m.stateList} className="flex flex-col gap-2">
            {entries.map((e) => (
              <StateRow key={`${e.id}:${e.name}:${e.defaultText}`} entry={e} names={names} path={path} />
            ))}
          </ul>
        </>
      )}
      {canEdit && (
        <form onSubmit={add} aria-label={m.stateAddForm} className="flex items-center gap-2">
          <Input
            aria-label={m.stateAddName}
            value={fresh}
            dir="ltr"
            aria-invalid={fresh !== "" && freshBad !== undefined}
            onChange={(e) => setFresh(e.target.value)}
          />
          <Button type="submit" variant="secondary" size="sm" disabled={freshBad !== undefined}>
            {m.stateAdd}
          </Button>
        </form>
      )}
      {onOpen !== undefined && (
        <Button variant="secondary" size="sm" onClick={onOpen}>
          <Icon name="list" />
          {m.openState}
        </Button>
      )}
    </div>
  );
}

/** The Data tab of the left panel (STU-061): data sources, mocks and collections, opening the data source designer. */
export function DataTab({ documents, onOpen }: SummaryTabProps) {
  const m = tabMessages(useLingui().i18n);
  const doc = useDocumentValue(documents, "plugin.json").value;
  const sources = objects(doc?.["dataSources"]);
  const mocks = sources.filter((s) => s["mock"] !== undefined).length;
  const collections = objects(doc?.["collections"]).length;
  return (
    <div className="flex flex-col gap-2">
      {sources.length === 0 && collections === 0 ? (
        empty(m.dataEmpty)
      ) : (
        <>
          <p className="text-body">
            {m.dataSources(sources.length)}, {m.dataMocks(mocks)}, {m.dataCollections(collections)}
          </p>
          <ul aria-label={m.dataList} className="flex flex-col gap-0.5">
            {sources.map((s, i) => (
              <li key={String(s["id"] ?? i)} className="text-body">
                <code>{String(s["name"] ?? s["id"] ?? i)}</code>
              </li>
            ))}
          </ul>
        </>
      )}
      {onOpen !== undefined && (
        <Button variant="secondary" size="sm" onClick={onOpen}>
          <Icon name="list" />
          {m.openData}
        </Button>
      )}
    </div>
  );
}

export interface SelectionTabProps {
  readonly ids: readonly string[];
  /** The nodes of the edited documents by id. */
  readonly nodes: ReadonlyMap<string, Obj>;
}

/** The JSON tab of the right panel (STU-062): the compact JSON of the selection, and a way into the JSON editor. */
export function JsonTab({ ids, nodes, onOpen }: SelectionTabProps & { readonly onOpen: (() => void) | undefined }) {
  const m = tabMessages(useLingui().i18n);
  const selected = ids.flatMap((id) => nodes.get(id) ?? []);
  return (
    <div className="flex flex-col gap-2">
      <textarea
        readOnly
        aria-label={m.jsonOf}
        value={compactJson(selected)}
        className="max-h-64 min-h-24 w-full resize-y bg-transparent text-data"
      />
      {onOpen !== undefined && (
        <Button variant="secondary" size="sm" onClick={onOpen}>
          <Icon name="pencil-simple" />
          {m.openJson}
        </Button>
      )}
    </div>
  );
}

/** The Code tab (STU-062): the functions the selection calls, and the functions the plugin may call (FN-012). */
export function CodeTab({
  ids,
  nodes,
  documents,
}: SelectionTabProps & { readonly documents: readonly DesignerDocument[] | undefined }) {
  const m = tabMessages(useLingui().i18n);
  const plugin = useDocumentValue(documents, "plugin.json").value;
  const used = useMemo(() => functionsIn(ids.flatMap((id) => nodes.get(id) ?? [])), [ids, nodes]);
  const granted = useMemo(() => extrasOf(plugin === undefined ? [] : [plugin], []).functions, [plugin]);
  const list = (label: string, names: readonly string[]) => (
    <ul aria-label={label} className="flex flex-col gap-0.5">
      {names.map((f) => (
        <li key={f}>
          <TypeTag>{f}</TypeTag>
        </li>
      ))}
    </ul>
  );
  return (
    <div className="flex flex-col gap-3">
      {used.length === 0 ? empty(m.codeNone) : list(m.codeList, used)}
      {granted.length > 0 && (
        <section className="flex flex-col gap-1">
          <h3 className="text-label font-medium">{m.codePlugin}</h3>
          {list(m.codePlugin, granted)}
        </section>
      )}
    </div>
  );
}

/** The Localisation tab (STU-062): the strings of the selection, with the table (V4) one click away. */
export function LocalisationTab({
  ids,
  nodes,
  onOpen,
}: SelectionTabProps & { readonly onOpen: (() => void) | undefined }) {
  const m = tabMessages(useLingui().i18n);
  const strings = stringsIn(ids.flatMap((id) => subtreeOf(nodes.get(id))));
  return (
    <div className="flex flex-col gap-2">
      {strings.length === 0 ? (
        empty(m.l10nNone)
      ) : (
        <ul aria-label={m.l10nList} className="flex flex-col gap-1">
          {strings.map((s) => (
            <li key={`${s.node}:${s.prop}`} className="flex items-center gap-2 text-body">
              <code className="truncate">{s.text}</code>
              {s.key === undefined && <TypeTag>{m.l10nLiteral}</TypeTag>}
            </li>
          ))}
        </ul>
      )}
      {onOpen !== undefined && (
        <Button variant="secondary" size="sm" onClick={onOpen}>
          <Icon name="globe" />
          {m.l10nOpen}
        </Button>
      )}
    </div>
  );
}

const statusKind: Record<BudgetStatus, StatusKind> = { within: "success", warning: "warning", over: "danger" };

const kilobytes = (bytes: bigint) => `${(Number(bytes) / 1024).toFixed(1)} KB`;

/** The Performance tab (STU-062): node count, cost and budget status, and the plugin's summary numbers. */
export function PerformanceTab({ ids, nodes, pageNodes }: SelectionTabProps & { readonly pageNodes: number }) {
  const m = tabMessages(useLingui().i18n);
  const { session } = useApp();
  const { app, plugin } = useParams({ strict: false }) as { app?: string; plugin?: string };
  const summary = useQuery({
    queryKey: ["plugin-summaries", app],
    queryFn: async () =>
      (await createClient(PluginService, session.transport).listPluginSummaries({ appId: app ?? "" })).summaries,
    enabled: app !== undefined,
  });
  const mine = summary.data?.find((s) => s.plugin?.id === plugin);
  const count = new Set(ids.flatMap((id) => subtreeOf(nodes.get(id)).map((n) => String(n["id"])))).size;
  const budget = nodeBudget();
  const status = budgetStatus(pageNodes, budget);
  const label = { within: m.perfWithin, warning: m.perfWarning, over: m.perfOver }[status];
  const share = pageNodes === 0 ? 0 : Math.round((count / pageNodes) * 100);
  const unknown = m.perfUnknown;
  return (
    <div className="flex flex-col gap-3">
      <section aria-label={m.perfSelection} className="flex flex-col gap-2">
        <h3 className="text-label font-medium">{m.perfSelection}</h3>
        <MetricStrip
          metrics={[
            { label: m.perfNodes, value: String(count) },
            { label: m.perfCost, value: `${share}%` },
            { label: m.perfPage, value: String(pageNodes) },
          ]}
        />
        <StatusPill status={statusKind[status]} label={label} />
        <Meter
          value={pageNodes}
          max={budget.limit}
          label={m.perfBudget}
          valueText={m.perfBudgetText(pageNodes, budget.limit)}
        />
      </section>
      <section aria-label={m.perfPlugin} className="flex flex-col gap-2">
        <h3 className="text-label font-medium">{m.perfPlugin}</h3>
        <MetricStrip
          metrics={[
            { label: m.perfBundle, value: mine === undefined ? unknown : kilobytes(mine.bundleSize) },
            { label: m.perfScreens, value: mine === undefined ? unknown : String(mine.screens) },
            { label: m.perfVersions, value: mine === undefined ? unknown : String(mine.versions) },
            { label: m.perfErrors, value: mine === undefined ? unknown : String(mine.validationErrors) },
            { label: m.perfFunctions, value: mine === undefined ? unknown : String(mine.functionsUsed.length) },
          ]}
        />
      </section>
    </div>
  );
}
