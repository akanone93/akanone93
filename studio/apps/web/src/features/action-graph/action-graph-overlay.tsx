// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Dialog, Icon, Input, Select, TypeTag } from "@plux/ui";
import { type Problem, scopeAt } from "@plux/validator";
import { Suspense, useEffect, useMemo, useRef, useState } from "react";
import { useCanEdit, useEditor, useEditSession } from "../editor/editor-provider.tsx";
import { JSON_DEBOUNCE_MS } from "../json-editor/json-editor-overlay.tsx";
import { pointerOf } from "../panels/properties-model.ts";
import { PXL_SCOPE_TTL_MS } from "../panels/pxl-source.ts";
import { eventsOf } from "../plugin-canvas/links.ts";
import { parseDocument } from "../plugin-canvas/plugin-model.ts";
import type { PxlLanguage } from "../problems/use-validation.ts";
import { type ActionGraphMessages, actionGraphMessages } from "./action-graph.messages.ts";
import { GraphContext, type GraphContextValue } from "./graph-context.ts";
import {
  addStepRecipe,
  buildCatalogue,
  type CatalogueEntry,
  type CatalogueGroup,
  type Extras,
  findNodeIn,
  freshId,
  groupProblems,
  isObj,
  type Recipe,
  resolveLocation,
  type Step,
  type StepField,
  setInputRecipe,
  stepsAt,
  stepsPointer,
} from "./graph-model.ts";
import { GraphCanvas } from "./lazy-canvas.tsx";

export interface GraphDocument {
  readonly path: string;
  readonly content: Uint8Array;
}

export interface ActionGraphOverlayProps {
  readonly open: boolean;
  readonly onClose: () => void;
  /** The plugin's documents as the server holds them; unsaved edits come from the editor store. */
  readonly documents: readonly GraphDocument[];
  /** The page document and node whose events are edited; none when nothing is selected. */
  readonly pagePath: string | undefined;
  readonly nodeId: string | undefined;
  readonly extras: Extras;
  readonly validatePage: ((file: string, data: string) => Promise<readonly Problem[]>) | undefined;
  readonly language: PxlLanguage | undefined;
}

const GROUPS: readonly CatalogueGroup[] = ["builtin", "custom", "function", "flow"];

function groupLabel(g: CatalogueGroup, m: ActionGraphMessages): string {
  return g === "builtin"
    ? m.groupBuiltin
    : g === "custom"
      ? m.groupCustom
      : g === "function"
        ? m.groupFunction
        : m.groupFlow;
}

function CataloguePanel(props: {
  catalogue: readonly CatalogueEntry[];
  canEdit: boolean;
  m: ActionGraphMessages;
  onAdd: (e: CatalogueEntry) => void;
}) {
  const { m } = props;
  const [query, setQuery] = useState("");
  const q = query.trim().toLowerCase();
  const shown = props.catalogue.filter((e) => q === "" || `${e.label} ${e.description}`.toLowerCase().includes(q));
  return (
    <section aria-label={m.catalogue} className="flex h-full w-72 shrink-0 flex-col gap-2 overflow-y-auto">
      <h3 className="text-label font-medium">{m.catalogue}</h3>
      <Input value={query} aria-label={m.search} onChange={(e) => setQuery(e.target.value)} />
      {shown.length === 0 && <p className="text-body text-secondary">{m.nothingFound}</p>}
      {GROUPS.map((g) => {
        const entries = shown.filter((e) => e.group === g);
        return entries.length === 0 ? null : (
          <section key={g} aria-label={groupLabel(g, m)} className="flex flex-col gap-1">
            <h4 className="text-caption text-secondary">{groupLabel(g, m)}</h4>
            <ul className="flex flex-col gap-1">
              {entries.map((e) => (
                <li key={e.key}>
                  <Button
                    variant="ghost"
                    size="sm"
                    disabled={!props.canEdit}
                    aria-label={m.addNamed(e.label)}
                    title={e.description}
                    onClick={() => props.onAdd(e)}
                  >
                    <Icon name="plus" />
                    <span className="truncate font-mono text-data">{e.label}</span>
                  </Button>
                </li>
              ))}
            </ul>
          </section>
        );
      })}
    </section>
  );
}

/** The documents as the editor shows them: the session's unsaved content over the server's. */
function useDocs(documents: readonly GraphDocument[]): Readonly<Record<string, unknown>> {
  const local = useEditor((st) => st.docs);
  return useMemo(() => {
    const out: Record<string, unknown> = {};
    for (const d of documents) {
      out[d.path] = local[d.path]?.content ?? parseDocument(d.content);
    }
    return out;
  }, [documents, local]);
}

type Verdict =
  | { readonly state: "checking" }
  | { readonly state: "unavailable" }
  | { readonly state: "done"; readonly problems: readonly Problem[] };

/** The validator's findings for the edited document, `JSON_DEBOUNCE_MS` after the last change (path validation, STU-065). */
function useVerdict(
  open: boolean,
  path: string | undefined,
  doc: unknown,
  validate: ActionGraphOverlayProps["validatePage"],
): Verdict {
  const [verdict, setVerdict] = useState<Verdict>({ state: "checking" });
  useEffect(() => {
    if (!open || path === undefined || doc === undefined) {
      return undefined;
    }
    if (validate === undefined) {
      setVerdict({ state: "unavailable" });
      return undefined;
    }
    let live = true;
    setVerdict({ state: "checking" });
    const timer = setTimeout(() => {
      validate(path, JSON.stringify(doc))
        .then((all) => live && setVerdict({ state: "done", problems: all.filter((p) => p.file === path) }))
        .catch(() => live && setVerdict({ state: "unavailable" }));
    }, JSON_DEBOUNCE_MS);
    return () => {
      live = false;
      clearTimeout(timer);
    };
  }, [open, path, doc, validate]);
  return verdict;
}

/** The widget, event and graph the editor shows: from the selection, the event chosen and the documents. */
function useTarget(props: ActionGraphOverlayProps, docs: Readonly<Record<string, unknown>>) {
  const [chosen, setEvent] = useState<string | undefined>(undefined);
  useEffect(() => {
    if (!props.open) {
      setEvent(undefined);
    }
  }, [props.open]);
  const page = props.pagePath === undefined ? undefined : docs[props.pagePath];
  const node = props.nodeId === undefined || !isObj(page) ? undefined : findNodeIn(page["root"], props.nodeId);
  const events = eventsOf(typeof node?.["type"] === "string" ? node["type"] : undefined);
  const event = chosen !== undefined && events.includes(chosen) ? chosen : events[0];
  const location =
    props.pagePath === undefined || props.nodeId === undefined || event === undefined
      ? undefined
      : resolveLocation(docs, props.pagePath, props.nodeId, event);
  const doc = location === undefined ? undefined : docs[location.path];
  const steps: readonly Step[] = useMemo(() => (location === undefined ? [] : stepsAt(doc, location)), [doc, location]);
  const prefix = location === undefined ? null : stepsPointer(doc, location, pointerOf);
  return { node, events, event, setEvent, location, doc, steps, prefix };
}

/** The scope the PXL language service needs for one input of a step: asked of the validator and kept for a moment. */
function usePxlSource(
  language: PxlLanguage | undefined,
  t: Pick<ReturnType<typeof useTarget>, "location" | "doc" | "steps" | "prefix">,
): GraphContextValue["pxlSource"] {
  const scopes = useRef(new Map<string, { at: number; scope: Promise<string | undefined> }>());
  const { location, doc, steps, prefix } = t;
  return (step: Step, field: StepField) => {
    if (language === undefined || location === undefined || prefix === null) {
      return undefined;
    }
    const index = steps.findIndex((s) => s.id === step.id);
    const key = `${location.path}|${step.id}|${field.path.join(".")}`;
    const ask = async (): Promise<string | undefined> => {
      const copy = structuredClone(doc);
      setInputRecipe(location, step.id, field.path, { $expr: "null" })(copy);
      const sites = await language.sites(location.path, JSON.stringify(copy));
      return scopeAt(sites, `${prefix}/${index}/input/${field.path.join("/")}`);
    };
    return {
      service: language.service,
      scope: () => {
        const cached = scopes.current.get(key);
        if (cached !== undefined && Date.now() - cached.at <= PXL_SCOPE_TTL_MS) {
          return cached.scope;
        }
        const scope = ask().catch(() => {
          scopes.current.delete(key);
          return undefined;
        });
        scopes.current.set(key, { at: Date.now(), scope });
        return scope;
      },
    };
  };
}

function statusOf(verdict: Verdict, m: ActionGraphMessages): string {
  if (verdict.state !== "done") {
    return verdict.state === "checking" ? m.checking : m.unavailable;
  }
  return verdict.problems.length === 0 ? m.noProblems : "";
}

function noticeOf(t: ReturnType<typeof useTarget>, props: ActionGraphOverlayProps, m: ActionGraphMessages) {
  if (props.nodeId === undefined || t.node === undefined) {
    return m.noSelection;
  }
  if (t.events.length === 0) {
    return m.noEvents;
  }
  return t.location === undefined ? m.unresolved : undefined;
}

function ProblemsSection({ problems, m }: { problems: readonly Problem[]; m: ActionGraphMessages }) {
  return problems.length === 0 ? null : (
    <section aria-label={m.problems} className="max-h-32 overflow-y-auto">
      <h3 className="text-label font-medium">{m.problems}</h3>
      <ul className="flex flex-col gap-1">
        {problems.map((p) => (
          <li key={`${p.code}${p.path}${p.message}`} className="text-body">
            {p.code !== "" && <TypeTag>{p.code}</TypeTag>} {p.message}
          </li>
        ))}
      </ul>
    </section>
  );
}

/** The action graph editor of S5.3 (STU-065): React Flow over the canvas, editing an event's graph through the edit pipeline. */
export function ActionGraphOverlay(props: ActionGraphOverlayProps) {
  const { i18n } = useLingui();
  const m = useMemo(() => actionGraphMessages(i18n), [i18n]);
  const canEdit = useCanEdit();
  const session = useEditSession();
  const docs = useDocs(props.documents);
  const catalogue = useMemo(() => buildCatalogue(props.extras), [props.extras]);
  const [layoutToken, setLayoutToken] = useState(0);
  const target = useTarget(props, docs);
  const { location, steps, prefix } = target;
  const verdict = useVerdict(props.open, location?.path, target.doc, props.validatePage);
  const problems = verdict.state === "done" ? verdict.problems : [];
  const byStep = useMemo(() => groupProblems(problems, prefix, steps), [problems, prefix, steps]);
  const pxlSource = usePxlSource(props.language, target);
  const applied = useRef(0);

  const apply = (recipe: Recipe, group?: string) => {
    if (location !== undefined && canEdit) {
      applied.current += 1;
      session.edit(location.path, (draft) => recipe(draft), {
        group: group ?? `graph:${location.path}:${applied.current}`,
      });
    }
  };
  const add = (entry: CatalogueEntry) => {
    if (location !== undefined) {
      apply(addStepRecipe(location, entry, freshId(steps, entry.action), catalogue, steps[steps.length - 1]?.id));
    }
  };
  const context: GraphContextValue | undefined =
    location === undefined ? undefined : { steps, catalogue, canEdit, m, apply, problems: byStep, pxlSource, location };
  const notice = noticeOf(target, props, m);

  return (
    <Dialog
      open={props.open}
      onOpenChange={(open) => {
        if (!open) {
          props.onClose();
        }
      }}
      size="full"
      title={m.title}
      labels={{ close: m.close }}
    >
      <div className="flex min-h-0 flex-1 flex-col gap-3">
        <div className="flex flex-wrap items-center gap-3">
          {target.events.length > 0 && (
            <Select
              label={m.event}
              options={target.events.map((e) => ({ value: e, label: e }))}
              value={target.event ?? ""}
              onValueChange={target.setEvent}
            />
          )}
          <Button variant="secondary" disabled={steps.length === 0} onClick={() => setLayoutToken((n) => n + 1)}>
            <Icon name="eye" />
            {m.autoLayout}
          </Button>
          <span role="status" aria-live="polite" className="text-body text-secondary">
            {statusOf(verdict, m)}
          </span>
        </div>
        {!canEdit && <p className="text-caption text-secondary">{m.readOnly}</p>}
        {notice !== undefined && <p className="text-body text-secondary">{notice}</p>}
        {context !== undefined && (
          <div className="flex min-h-0 flex-1 gap-3">
            <CataloguePanel catalogue={catalogue} canEdit={canEdit} m={m} onAdd={add} />
            <div className="min-h-0 min-w-0 flex-1">
              {props.open && steps.length === 0 && <p className="text-body text-secondary">{m.empty}</p>}
              {props.open && steps.length > 0 && (
                <GraphContext.Provider value={context}>
                  <Suspense fallback={<p className="text-body text-secondary">{m.checking}</p>}>
                    <GraphCanvas layoutToken={layoutToken} />
                  </Suspense>
                </GraphContext.Provider>
              )}
            </div>
          </div>
        )}
        <ProblemsSection problems={[...byStep.values()].flat()} m={m} />
      </div>
    </Dialog>
  );
}
