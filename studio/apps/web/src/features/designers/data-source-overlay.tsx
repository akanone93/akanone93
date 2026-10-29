// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Dialog, Field, Icon, Input, Select, Textarea, TypeTag } from "@plux/ui";
import type { Problem } from "@plux/validator";
import { useEffect, useMemo, useState } from "react";
import { jsonEqual } from "../editor/json-patch.ts";
import { LockBanner } from "../editor/lock-ui.tsx";
import type { AppSession } from "../localisation/l10n-sessions.tsx";
import type { ValidatorClient } from "../problems/validator-client.ts";
import { type DesignerMessages, designerMessages } from "./designers.messages.ts";
import { ProblemList, Status } from "./parts.tsx";
import {
  type DesignerDocument,
  findDocument,
  hasErrors,
  isObject,
  type Json,
  useApply,
  useCheck,
  useDocumentValue,
} from "./use-document.ts";
import { uuid7 } from "./uuid7.ts";

/** The kinds of data source the schema names (`dataSourceKind`). */
const KINDS = ["rest", "graphql", "websocket", "sse", "function", "database", "static"] as const;

export interface DataSourceOverlayProps {
  readonly open: boolean;
  readonly onClose: () => void;
  /** The plugin's documents. */
  readonly documents: readonly DesignerDocument[] | undefined;
  readonly validatePage: ((file: string, data: string) => Promise<readonly Problem[]>) | undefined;
  /** The app's session, which holds the app lock: the importer's app-level variables go to the app document through it. */
  readonly app: AppSession | null;
  /** The app's documents: `app.json` carries the variables. */
  readonly appDocuments: readonly DesignerDocument[] | undefined;
  /** The Go module's importer (L-12). */
  readonly tool: ValidatorClient["tool"];
}

/** What the importer answered: a document fragment and its own findings. */
export interface ImportAnswer {
  readonly dataSources: Json[];
  readonly types: Json[];
  /** The app-level variables of the source, which belong to the app document and not to the plugin. */
  readonly variables: Json[];
  readonly diagnostics: readonly string[];
}

/** Reads the importer's JSON answer; anything else is not an answer. */
export function parseImport(raw: unknown): ImportAnswer | undefined {
  if (!isObject(raw) || !isObject(raw["fragment"])) {
    return undefined;
  }
  const list = (v: unknown): Json[] => (Array.isArray(v) ? v.filter(isObject) : []);
  const fragment = raw["fragment"];
  const diagnostics = Array.isArray(raw["diagnostics"])
    ? raw["diagnostics"]
        .filter(isObject)
        .map((d) => `${typeof d["code"] === "string" ? `${d["code"]}: ` : ""}${String(d["message"] ?? "")}`)
    : [];
  return {
    dataSources: list(fragment["dataSources"]),
    types: list(fragment["types"]),
    variables: list(fragment["variables"]),
    diagnostics,
  };
}

/** The draft of the plugin document: its data sources, and the types an import added, over the document. */
/** The app document with the variables it does not have yet added; a variable it has is kept as it is. */
export function withVariables(base: Json, variables: readonly Json[]): Json {
  const have = Array.isArray(base["variables"]) ? (base["variables"] as unknown[]) : [];
  const names = new Set(have.filter(isObject).map((v) => v["name"]));
  return { ...base, variables: [...have, ...variables.filter((v) => !names.has(v["name"]))] };
}

export function draftDocument(base: Json, sources: readonly Json[], types: readonly Json[]): Json {
  const next: Json = { ...base };
  if (sources.length > 0 || "dataSources" in base) {
    next["dataSources"] = sources;
  }
  if (types.length > 0) {
    const have = Array.isArray(base["types"]) ? (base["types"] as unknown[]) : [];
    const names = new Set(have.filter(isObject).map((t) => t["name"]));
    next["types"] = [...have, ...types.filter((t) => !names.has(t["name"]))];
  }
  return next;
}

function useDraft(open: boolean, value: Json | undefined) {
  const [sources, setSources] = useState<Json[]>([]);
  const [types, setTypes] = useState<Json[]>([]);
  const [variables, setVariables] = useState<Json[]>([]);
  const [selected, setSelected] = useState(0);
  const seed = Array.isArray(value?.["dataSources"]) ? (value["dataSources"] as unknown[]).filter(isObject) : [];
  // biome-ignore lint/correctness/useExhaustiveDependencies: the draft is seeded when the overlay opens
  useEffect(() => {
    if (open) {
      setSources(structuredClone(seed));
      setTypes([]);
      setVariables([]);
      setSelected(0);
    }
  }, [open]);
  return { sources, setSources, types, setTypes, variables, setVariables, selected, setSelected };
}

function SourceFields(props: { source: Json; onChange: (patch: Json) => void; canEdit: boolean; m: DesignerMessages }) {
  const { source, onChange, canEdit, m } = props;
  const [mock, setMock] = useState("");
  const [badMock, setBadMock] = useState(false);
  const id = String(source["id"] ?? "");
  // biome-ignore lint/correctness/useExhaustiveDependencies: the text is re-seeded only when another source is selected
  useEffect(() => {
    setMock(JSON.stringify(source["mock"] ?? null, null, 2));
    setBadMock(false);
  }, [id]);
  return (
    <div className="flex flex-col gap-3">
      <Field label={m.name}>
        <Input
          value={String(source["name"] ?? "")}
          disabled={!canEdit}
          onChange={(e) => onChange({ name: e.target.value })}
        />
      </Field>
      <Select
        label={m.dataKind}
        options={KINDS.map((k) => ({ value: k, label: k }))}
        value={String(source["kind"] ?? "rest")}
        disabled={!canEdit}
        onValueChange={(kind) => onChange({ kind })}
      />
      <Field label={m.dataType}>
        <Input
          value={String(source["type"] ?? "")}
          disabled={!canEdit}
          onChange={(e) => onChange({ type: e.target.value })}
        />
      </Field>
      <Field label={m.dataMock} description={m.dataMockHelp} error={badMock ? m.dataMockInvalid : undefined}>
        <Textarea
          rows={8}
          value={mock}
          disabled={!canEdit}
          onChange={(e) => {
            setMock(e.target.value);
            try {
              onChange({ mock: JSON.parse(e.target.value) as unknown });
              setBadMock(false);
            } catch {
              setBadMock(true);
            }
          }}
        />
      </Field>
    </div>
  );
}

export function ImportSection(props: {
  tool: ValidatorClient["tool"];
  canEdit: boolean;
  onAdd: (answer: ImportAnswer) => void;
  m: DesignerMessages;
}) {
  const { tool, canEdit, onAdd, m } = props;
  const [format, setFormat] = useState<"openapi" | "graphql">("openapi");
  const [text, setText] = useState("");
  const [ops, setOps] = useState("");
  const [state, setState] = useState<"idle" | "running" | "failed">("idle");
  const [answer, setAnswer] = useState<ImportAnswer | undefined>(undefined);
  const run = async () => {
    if (tool === undefined) {
      setState("failed");
      return;
    }
    setState("running");
    setAnswer(undefined);
    const files = [{ file: format === "openapi" ? "openapi" : "schema.graphql", data: text }];
    if (format === "graphql" && ops.trim() !== "") {
      files.push({ file: "operations.graphql", data: ops });
    }
    try {
      const parsed = parseImport(await tool("import", [format, JSON.stringify(files)]));
      setAnswer(parsed);
      setState(parsed === undefined ? "failed" : "idle");
    } catch {
      setState("failed");
    }
  };
  const readFile = async (file: File | undefined, set: (t: string) => void) => {
    if (file !== undefined) {
      set(await file.text());
    }
  };
  return (
    <section aria-label={m.dataImportHeading} className="flex flex-col gap-3 border-t border-hairline pt-3">
      <h3 className="text-label font-medium">{m.dataImportHeading}</h3>
      <Select
        label={m.dataImportFormat}
        options={[
          { value: "openapi", label: m.dataOpenApi },
          { value: "graphql", label: m.dataGraphQl },
        ]}
        value={format}
        disabled={!canEdit}
        onValueChange={(v) => setFormat(v === "graphql" ? "graphql" : "openapi")}
      />
      <Field label={m.dataChooseFile}>
        <Input type="file" disabled={!canEdit} onChange={(e) => void readFile(e.target.files?.[0], setText)} />
      </Field>
      <Field label={m.dataSourceText}>
        <Textarea rows={6} value={text} disabled={!canEdit} onChange={(e) => setText(e.target.value)} />
      </Field>
      {format === "graphql" && (
        <Field label={m.dataOperationsText}>
          <Textarea rows={4} value={ops} disabled={!canEdit} onChange={(e) => setOps(e.target.value)} />
        </Field>
      )}
      <div className="flex flex-wrap items-center gap-3">
        <Button
          variant="secondary"
          disabled={!canEdit || text.trim() === "" || state === "running"}
          onClick={() => void run()}
        >
          {m.dataRunImport}
        </Button>
        {state === "running" && <span className="text-body text-secondary">{m.dataImporting}</span>}
        {state === "failed" && (
          <span role="alert" className="text-body">
            {m.dataImportFailed}
          </span>
        )}
      </div>
      {answer !== undefined && (
        <div className="flex flex-col gap-2">
          <ul aria-label={m.problems} className="flex flex-col gap-1">
            {answer.diagnostics.map((d) => (
              <li key={d} className="text-body">
                {d}
              </li>
            ))}
          </ul>
          {answer.dataSources.length === 0 ? (
            <p className="text-body text-secondary">{m.dataImportNothing}</p>
          ) : (
            <>
              <p role="status" className="text-body text-secondary">
                {m.dataImportReady}
              </p>
              <ul className="flex flex-wrap gap-2">
                {answer.dataSources.map((s) => (
                  <li key={String(s["id"])}>
                    <TypeTag>{String(s["name"])}</TypeTag>
                  </li>
                ))}
              </ul>
              {answer.variables.length > 0 && <p className="text-caption text-secondary">{m.dataVariablesNote}</p>}
              <Button
                disabled={!canEdit}
                onClick={() => {
                  onAdd(answer);
                  setAnswer(undefined);
                }}
              >
                {m.dataAddImported}
              </Button>
            </>
          )}
        </div>
      )}
    </section>
  );
}

/** The data source designer V6 (STU-063): edit a plugin's data sources and mocks (DAT-080), or import them from OpenAPI or GraphQL (DAT-002); applied through the edit pipeline once the validator accepts them. */
export function DataSourceOverlay(props: DataSourceOverlayProps) {
  const { i18n } = useLingui();
  const m = useMemo(() => designerMessages(i18n), [i18n]);
  const { path, value } = useDocumentValue(props.documents, "plugin.json");
  const { canEdit, apply } = useApply(path, "data-sources");
  const appPath = findDocument(props.appDocuments, "app.json")?.path;
  const applyApp = useApply(appPath, "app-variables", props.app);
  const { value: appValue } = useDocumentValue(props.appDocuments, "app.json", props.app);
  const d = useDraft(props.open, value);
  const next = useMemo(
    () => (value === undefined ? undefined : draftDocument(value, d.sources, d.types)),
    [value, d.sources, d.types],
  );
  const check = useCheck(props.open, path, next, props.validatePage);
  const changed = next !== undefined && !jsonEqual(next, value);
  const staged = d.variables.length > 0;
  const variablesBlocked = staged && (!applyApp.canEdit || appValue === undefined);
  const canApply = canEdit && (changed || staged) && !variablesBlocked && !hasErrors(check);
  const [note, setNote] = useState("");
  // biome-ignore lint/correctness/useExhaustiveDependencies: a note belongs to the draft it was made for
  useEffect(() => setNote(""), [d.sources, d.types]);
  const current = d.sources[d.selected];
  const change = (patch: Json) => d.setSources((all) => all.map((s, i) => (i === d.selected ? { ...s, ...patch } : s)));
  const add = () => {
    d.setSources((all) => [
      ...all,
      { id: uuid7(), name: `source-${all.length + 1}`, kind: "rest", type: "string", mock: "" },
    ]);
    d.setSelected(d.sources.length);
  };
  const remove = () => {
    d.setSources((all) => all.filter((_, i) => i !== d.selected));
    d.setSelected(0);
  };
  const problems = check.state === "done" ? check.problems : [];
  return (
    <Dialog
      open={props.open}
      onOpenChange={(open) => {
        if (!open) {
          props.onClose();
        }
      }}
      size="full"
      title={m.dataTitle}
      labels={{ close: m.dataClose }}
    >
      <div className="flex min-h-0 flex-1 flex-col gap-3">
        <div className="flex flex-wrap items-center gap-3">
          <Button variant="secondary" disabled={!canEdit} onClick={add}>
            <Icon name="plus" />
            {m.dataAdd}
          </Button>
          <Button variant="secondary" disabled={!canEdit || current === undefined} onClick={remove}>
            {m.dataRemove}
          </Button>
          <Button
            disabled={!canApply}
            onClick={() => {
              if (next !== undefined) {
                apply(next);
                if (staged) {
                  if (appValue !== undefined) {
                    applyApp.apply(withVariables(appValue, d.variables));
                  }
                  d.setVariables([]);
                }
                d.setTypes((all) => (all.length === 0 ? all : []));
                setNote(m.applied);
              }
            }}
          >
            {m.apply}
          </Button>
          <Status check={check} valid={!hasErrors(check)} changed={changed} note={note} m={m} />
        </div>
        {props.app !== null && <LockBanner lock={props.app.lock} />}
        {!canEdit && <p className="text-caption text-secondary">{m.readOnly}</p>}
        {variablesBlocked && <p className="text-caption text-secondary">{m.dataVariablesLocked}</p>}
        <div className="flex min-h-0 flex-1 gap-6 overflow-y-auto">
          <nav aria-label={m.dataList} className="flex w-56 shrink-0 flex-col gap-1">
            {d.sources.length === 0 && <p className="text-body text-secondary">{m.dataNone}</p>}
            {d.sources.map((s, i) => (
              <Button
                key={String(s["id"])}
                variant={i === d.selected ? "primary" : "secondary"}
                aria-pressed={i === d.selected}
                onClick={() => d.setSelected(i)}
              >
                {String(s["name"] ?? "")}
                <TypeTag>{String(s["kind"] ?? "")}</TypeTag>
              </Button>
            ))}
          </nav>
          <div className="flex min-w-0 flex-1 flex-col gap-4">
            {current === undefined ? (
              <p className="text-body text-secondary">{m.dataSelect}</p>
            ) : (
              <SourceFields source={current} onChange={change} canEdit={canEdit} m={m} />
            )}
            <ImportSection
              tool={props.tool}
              canEdit={canEdit}
              m={m}
              onAdd={(answer) => {
                const known = new Set(d.sources.map((s) => s["id"]));
                d.setSources((all) => [...all, ...answer.dataSources.filter((s) => !known.has(s["id"]))]);
                d.setTypes((all) => [...all, ...answer.types]);
                d.setVariables((all) => withVariables({ variables: all }, answer.variables)["variables"] as Json[]);
              }}
            />
          </div>
        </div>
        <ProblemList problems={problems} m={m} />
      </div>
    </Dialog>
  );
}
