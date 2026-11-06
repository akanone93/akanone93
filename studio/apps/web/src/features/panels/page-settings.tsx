// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Icon, type IconName, Input, Select } from "@plux/ui";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@plux/ui/components/collapsible";
import { Switch } from "@plux/ui/components/switch";
import { type ReactElement, type ReactNode, useEffect, useId, useMemo, useRef, useState } from "react";
import { useCanEdit, useEditor, useEditSession } from "../editor/editor-provider.tsx";
import { pluginCanvasMessages } from "../plugin-canvas/plugin-canvas.messages.ts";
import { parseDocument } from "../plugin-canvas/plugin-model.ts";
import { uuidv7 } from "../plugin-canvas/screens.ts";
import { formsMessages } from "./forms.messages.ts";
import { formsOf } from "./forms-model.ts";
import { FormsSection } from "./forms-section.tsx";
import { pageSettingsMessages } from "./page-settings.messages.ts";
import {
  addParam,
  assuranceLevels,
  baseTypes,
  freshParamName,
  graphsOf,
  lifecycleEvents,
  type PageParam,
  type PageSettings,
  pageKinds,
  paramNameProblem,
  readPage,
  removeParam,
  retarget,
  routeProblem,
  setField,
  setGuard,
  setNested,
  setParam,
  textOfValue,
  transitions,
  validType,
  valueOfText,
} from "./page-settings-model.ts";
import { PropGroupHead, PropRow } from "./prop-row.tsx";

type Obj = Record<string, unknown>;
type Doc = { readonly path: string; readonly kind: string; readonly content: Uint8Array };
type Messages = ReturnType<typeof pageSettingsMessages>;
type Change = (recipe: (draft: Obj) => void, field: string) => void;

const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);

/**
 * A text field that commits when it loses the focus or on Enter, and reverts on Escape: one undo step per change, and
 * a route rename that runs once (PC-14).
 */
function CommitText(props: {
  readonly label: string;
  readonly value: string;
  readonly readOnly: boolean;
  readonly mono?: boolean;
  readonly list?: string;
  readonly error?: (text: string) => string | undefined;
  readonly onCommit: (text: string) => void;
}): ReactElement {
  const [text, setText] = useState(props.value);
  const editing = useRef(false);
  useEffect(() => {
    if (!editing.current) {
      setText(props.value);
    }
  }, [props.value]);
  const error = props.error?.(text);
  const errorId = useId();
  const commit = () => {
    editing.current = false;
    if (text !== props.value && error === undefined) {
      props.onCommit(text);
    } else if (error !== undefined) {
      setText(props.value);
    }
  };
  return (
    <div className="page-field" data-mono={props.mono === true || undefined}>
      <Input
        aria-label={props.label}
        value={text}
        readOnly={props.readOnly}
        list={props.list}
        aria-invalid={error === undefined ? undefined : true}
        aria-describedby={error === undefined ? undefined : errorId}
        onChange={(e) => {
          editing.current = true;
          setText(e.target.value);
        }}
        onBlur={commit}
        onKeyDown={(e) => {
          if (e.key === "Enter") {
            e.currentTarget.blur();
          } else if (e.key === "Escape") {
            editing.current = false;
            setText(props.value);
          }
        }}
      />
      {error !== undefined && (
        <p id={errorId} className="page-field-error" role="alert">
          {error}
        </p>
      )}
    </div>
  );
}

function Section(props: {
  readonly icon: IconName;
  readonly label: string;
  readonly set: number;
  readonly count: number;
  readonly children: ReactNode;
}): ReactElement {
  return (
    <Collapsible defaultOpen className="prop-section">
      <CollapsibleTrigger className="prop-section-head">
        <PropGroupHead icon={props.icon} label={props.label} set={props.set} count={props.count} />
      </CollapsibleTrigger>
      <CollapsibleContent className="prop-grid">{props.children}</CollapsibleContent>
    </Collapsible>
  );
}

/** The page's own fields: title, description, kind, route name, key and whether it is the entry. */
function PageSection(p: {
  readonly page: PageSettings;
  readonly m: Messages;
  readonly canEdit: boolean;
  readonly change: Change;
  readonly route: RouteRename;
  readonly entry: boolean;
  readonly onEntry: () => void;
}): ReactElement {
  const { page, m, canEdit, change } = p;
  const km = pluginCanvasMessages(useLingui().i18n);
  const reset = "";
  return (
    <Section icon="file" label={m.sectionPage} set={0} count={6}>
      <PropRow name="title" label={m.title} set resetLabel={reset}>
        {"key" in page.title ? (
          <span className="page-note font-mono">{m.titleKey(page.title.key)}</span>
        ) : (
          <CommitText
            label={m.title}
            value={page.title.text}
            readOnly={!canEdit}
            onCommit={(t) => change(setField("title", t), "title")}
          />
        )}
      </PropRow>
      <PropRow name="description" label={m.description} set={page.description !== ""} resetLabel={reset}>
        <CommitText
          label={m.description}
          value={page.description}
          readOnly={!canEdit}
          onCommit={(t) => change(setField("description", t.trim() === "" ? undefined : t), "description")}
        />
      </PropRow>
      <PropRow name="pageKind" label={m.kind} set resetLabel={reset}>
        <Select
          label={m.kind}
          value={page.kind}
          disabled={!canEdit}
          options={pageKinds.map((k) => ({ value: k, label: km.pageKind(k) }))}
          onValueChange={(v) => change(setField("pageKind", v), "kind")}
        />
      </PropRow>
      <PropRow name="route" label={m.route} set resetLabel={reset}>
        <CommitText
          label={m.route}
          value={page.route}
          readOnly={!canEdit}
          mono
          error={p.route.problem}
          onCommit={p.route.rename}
        />
      </PropRow>
      {p.route.note !== undefined && <p className="page-note">{p.route.note}</p>}
      <PropRow name="key" label={m.key} set resetLabel={reset}>
        <span className="page-note font-mono">{page.key}</span>
      </PropRow>
      <PropRow name="entry" label={m.entry} set={p.entry} resetLabel={reset}>
        <span className="prop-switch">
          <Switch
            size="sm"
            aria-label={m.entry}
            checked={p.entry}
            disabled={!canEdit || p.entry}
            onCheckedChange={(on) => on && p.onEntry()}
          />
          {p.entry && <span className="page-note">{m.entryNote}</span>}
        </span>
      </PropRow>
      {(page.kind === "dialog" || page.kind === "bottomSheet") && (
        <PropRow name="result" label={m.result} set={page.result !== ""} resetLabel={reset}>
          <CommitText
            label={m.result}
            value={page.result}
            readOnly={!canEdit}
            mono
            list="page-types"
            error={(t) => (t === "" || validType(t) ? undefined : m.typeForm)}
            onCommit={(t) => change(setField("result", t === "" ? undefined : t), "result")}
          />
        </PropRow>
      )}
    </Section>
  );
}

/** One parameter: its name, type, whether it is required, its default and its design-time mock (SCH-024). */
function ParamCard(p: {
  readonly param: PageParam;
  readonly index: number;
  readonly others: readonly string[];
  readonly m: Messages;
  readonly canEdit: boolean;
  readonly change: Change;
}): ReactElement {
  const { param, index, m, canEdit, change } = p;
  const field = (name: keyof PageParam) => `param:${index}:${name}`;
  return (
    <li className="page-param">
      <div className="page-param-head">
        <CommitText
          label={m.paramName}
          value={param.name}
          readOnly={!canEdit}
          mono
          error={(t) => {
            const problem = paramNameProblem(t, p.others);
            return problem === "form" ? m.paramNameForm : problem === "taken" ? m.paramNameTaken : undefined;
          }}
          onCommit={(t) => change(setParam(index, "name", t), field("name"))}
        />
        <Button
          variant="ghost"
          size="sm"
          aria-label={m.removeParam(param.name)}
          disabled={!canEdit}
          onClick={() => change(removeParam(index), field("name"))}
        >
          <Icon name="trash" />
        </Button>
      </div>
      <div className="page-param-grid">
        <span className="page-param-label">{m.paramType}</span>
        <CommitText
          label={m.paramType}
          value={param.type}
          readOnly={!canEdit}
          mono
          list="page-types"
          error={(t) => (validType(t) ? undefined : m.typeForm)}
          onCommit={(t) => change(setParam(index, "type", t), field("type"))}
        />
        <span className="page-param-label">{m.paramRequired}</span>
        <span className="prop-switch">
          <Switch
            size="sm"
            aria-label={m.paramRequired}
            checked={param.required}
            disabled={!canEdit}
            onCheckedChange={(on) => change(setParam(index, "required", on), field("required"))}
          />
        </span>
        <span className="page-param-label">{m.paramDefault}</span>
        <CommitText
          label={m.paramDefault}
          value={textOfValue(param.default)}
          readOnly={!canEdit}
          onCommit={(t) => change(setParam(index, "default", valueOfText(param.type, t)), field("default"))}
        />
        <span className="page-param-label">{m.paramMock}</span>
        <CommitText
          label={m.paramMock}
          value={textOfValue(param.mock)}
          readOnly={!canEdit}
          onCommit={(t) => change(setParam(index, "mock", valueOfText(param.type, t)), field("mock"))}
        />
      </div>
    </li>
  );
}

function ParamsSection(p: {
  readonly page: PageSettings;
  readonly m: Messages;
  readonly canEdit: boolean;
  readonly change: Change;
}): ReactElement {
  const { page, m, canEdit, change } = p;
  const names = page.params.map((x) => x.name);
  return (
    <Section icon="variable" label={m.sectionParams} set={page.params.length} count={page.params.length}>
      {page.params.length === 0 && <p className="page-note">{m.noParams}</p>}
      <ul className="page-params">
        {page.params.map((param, i) => (
          <ParamCard
            // biome-ignore lint/suspicious/noArrayIndexKey: parameters are edited by position; a rename keeps the row
            key={i}
            param={param}
            index={i}
            others={names.filter((_, j) => j !== i)}
            m={m}
            canEdit={canEdit}
            change={change}
          />
        ))}
      </ul>
      <Button
        variant="secondary"
        size="sm"
        disabled={!canEdit}
        onClick={() => change(addParam(freshParamName(page.params)), `param:add:${page.params.length}`)}
      >
        <Icon name="plus" />
        {m.addParam}
      </Button>
    </Section>
  );
}

function DataSection(p: {
  readonly page: PageSettings;
  readonly m: Messages;
  readonly onOpenState?: (() => void) | undefined;
  readonly onOpenData?: (() => void) | undefined;
}): ReactElement {
  const { page, m } = p;
  return (
    <Section icon="database" label={m.sectionData} set={page.state + page.dataSources} count={2}>
      <div className="page-line">
        <span>{m.stateCount(page.state)}</span>
        {p.onOpenState !== undefined && (
          <Button variant="ghost" size="sm" onClick={p.onOpenState}>
            {m.openState}
          </Button>
        )}
      </div>
      <div className="page-line">
        <span>{m.dataCount(page.dataSources)}</span>
        {p.onOpenData !== undefined && (
          <Button variant="ghost" size="sm" onClick={p.onOpenData}>
            {m.openData}
          </Button>
        )}
      </div>
    </Section>
  );
}

function LifecycleSection(p: {
  readonly page: PageSettings;
  readonly m: Messages;
  readonly onOpenGraph?: (() => void) | undefined;
}): ReactElement {
  const { page, m } = p;
  return (
    <Section icon="flow-arrow" label={m.sectionLifecycle} set={page.lifecycle.size} count={lifecycleEvents.length}>
      <ul className="page-lifecycle">
        {lifecycleEvents.map((e) => (
          <li key={e} className="page-line" data-set={page.lifecycle.has(e) || undefined}>
            <span>{m.lifecycleEvent(e)}</span>
            <span className="page-note">{page.lifecycle.has(e) ? m.handlerSet : m.handlerNone}</span>
          </li>
        ))}
      </ul>
      {p.onOpenGraph !== undefined && (
        <Button variant="ghost" size="sm" onClick={p.onOpenGraph}>
          <Icon name="flow-arrow" />
          {m.editActions}
        </Button>
      )}
    </Section>
  );
}

function RouteSection(p: {
  readonly page: PageSettings;
  readonly m: Messages;
  readonly canEdit: boolean;
  readonly change: Change;
  readonly graphs: readonly { readonly id: string; readonly key: string }[];
}): ReactElement {
  const { page, m, canEdit, change } = p;
  const keyOf = (id: string) => p.graphs.find((g) => g.id === id)?.key ?? id;
  const free = p.graphs.filter((g) => !page.guards.includes(g.id));
  return (
    <Section
      icon="navigation-arrow"
      label={m.sectionRoute}
      set={(page.transition === undefined ? 0 : 1) + page.guards.length}
      count={2}
    >
      <PropRow name="transition" label={m.transition} set={page.transition !== undefined} resetLabel="">
        <Select
          label={m.transition}
          value={page.transition ?? "platform"}
          disabled={!canEdit}
          options={transitions.map((t) => ({ value: t, label: m.transitionName(t) }))}
          onValueChange={(v) =>
            change(setNested("routeOptions", "transition", v === "platform" ? undefined : v), "transition")
          }
        />
      </PropRow>
      {page.transition === "custom" && (
        <PropRow name="timeline" label={m.timeline} set={page.timeline !== ""} resetLabel="">
          <CommitText
            label={m.timeline}
            value={page.timeline}
            readOnly={!canEdit}
            mono
            onCommit={(t) => change(setNested("routeOptions", "timeline", t.trim()), "timeline")}
          />
        </PropRow>
      )}
      <div className="page-guards">
        <span className="page-param-label">{m.guards}</span>
        {page.guards.length === 0 && <span className="page-note">{m.noGuards}</span>}
        <ul>
          {page.guards.map((g) => (
            <li key={g} className="page-line">
              <span className="font-mono">{keyOf(g)}</span>
              <Button
                variant="ghost"
                size="sm"
                aria-label={m.removeGuard(keyOf(g))}
                disabled={!canEdit}
                onClick={() => change(setGuard(g, false), `guard:${g}`)}
              >
                <Icon name="x" />
              </Button>
            </li>
          ))}
        </ul>
        {free.length > 0 && canEdit && (
          <Select
            label={m.addGuard}
            value=""
            placeholder={m.addGuard}
            options={free.map((g) => ({ value: g.id, label: g.key }))}
            onValueChange={(v) => change(setGuard(v, true), `guard:${v}`)}
          />
        )}
      </div>
    </Section>
  );
}

function SecuritySection(p: {
  readonly page: PageSettings;
  readonly m: Messages;
  readonly canEdit: boolean;
  readonly change: Change;
}): ReactElement {
  const { page, m, canEdit, change } = p;
  const NONE = "-";
  return (
    <Section
      icon="shield"
      label={m.sectionSecurity}
      set={(page.secure ? 1 : 0) + (page.assurance === undefined ? 0 : 1)}
      count={2}
    >
      <PropRow name="secure" label={m.secure} set={page.secure} resetLabel="">
        <span className="prop-switch">
          <Switch
            size="sm"
            aria-label={m.secure}
            checked={page.secure}
            disabled={!canEdit}
            onCheckedChange={(on) => change(setNested("security", "secure", on), "secure")}
          />
        </span>
      </PropRow>
      <PropRow name="requiresAssurance" label={m.assurance} set={page.assurance !== undefined} resetLabel="">
        <Select
          label={m.assurance}
          value={page.assurance ?? NONE}
          disabled={!canEdit}
          options={[{ value: NONE, label: m.assuranceNone }, ...assuranceLevels.map((l) => ({ value: l, label: l }))]}
          onValueChange={(v) =>
            change(setNested("security", "requiresAssurance", v === NONE ? undefined : v), "assurance")
          }
        />
      </PropRow>
    </Section>
  );
}

/** A route rename: why the typed name cannot be used, what follows it, and the rename itself. */
interface RouteRename {
  readonly problem: (text: string) => string | undefined;
  readonly note: string | undefined;
  readonly rename: (text: string) => void;
}

/** Parses every document, the session's copy first. */
function useLiveDocs(docs: readonly Doc[] | undefined): { path: string; kind: string; json: unknown }[] {
  const local = useEditor((st) => st.docs);
  return useMemo(
    () =>
      (docs ?? []).map((d) => ({
        path: d.path,
        kind: d.kind,
        json: local[d.path]?.content ?? parseDocument(d.content),
      })),
    [docs, local],
  );
}

/** The plugins outside this one whose pages open `route`, by name. */
function externalOpeners(
  app: readonly { readonly kind: string; readonly content: Uint8Array }[],
  own: ReadonlySet<string>,
  route: string,
): string[] {
  const out = new Set<string>();
  for (const d of app) {
    if (d.kind !== "page") {
      continue;
    }
    const json = parseDocument(d.content);
    if (
      isObj(json) &&
      typeof json["id"] === "string" &&
      !own.has(json["id"]) &&
      retarget(structuredClone(json), route, route) > 0
    ) {
      out.add(typeof json["key"] === "string" ? json["key"] : json["id"]);
    }
  }
  return [...out];
}

/**
 * The inspector's Page pane (PC-14, SCH-022 to SCH-025): the settings of the page the selection is on — title,
 * description, kind, route name (renaming it updates this plugin's links; other plugins' links block it), key, entry,
 * a dialog's result type, typed parameters with defaults and mocks, its state and data (opening their editors),
 * lifecycle handlers, transition, timeline and guards, and security flags. Each change is one undo step.
 */
export function PageSettingsTab(props: {
  readonly ids: readonly string[];
  readonly screenOf: ReadonlyMap<string, string>;
  readonly pathOf: ReadonlyMap<string, string>;
  readonly documents: readonly Doc[] | undefined;
  readonly appDocuments?: readonly { readonly kind: string; readonly content: Uint8Array }[] | undefined;
  readonly onOpenState?: (() => void) | undefined;
  readonly onOpenData?: (() => void) | undefined;
  readonly onOpenGraph?: (() => void) | undefined;
}): ReactElement {
  const { i18n } = useLingui();
  const m = pageSettingsMessages(i18n);
  const formsLabel = formsMessages(i18n).forms;
  const canEdit = useCanEdit();
  const session = useEditSession();
  const docs = useLiveDocs(props.documents);
  const first = props.ids[0];
  const screen = first === undefined ? undefined : props.screenOf.get(first);
  const path = screen === undefined ? undefined : props.pathOf.get(screen);
  const page = readPage(docs.find((d) => d.path === path)?.json);
  const burst = useRef(0);
  if (page === undefined || path === undefined) {
    return <p className="page-note p-3">{m.none}</p>;
  }
  const change: Change = (recipe, field) => {
    burst.current += 1;
    session.edit(path, (d) => recipe(d as Obj), { group: `page:${page.id}:${field}:${burst.current}` });
  };
  const plugin = docs.find((d) => d.kind === "plugin");
  const entry = isObj(plugin?.json) && plugin.json["entryPage"] === page.id;
  const ownPages = docs.filter((d) => d.kind === "page");
  const ownIds = new Set(
    ownPages.flatMap((d) => (isObj(d.json) && typeof d.json["id"] === "string" ? [d.json["id"]] : [])),
  );
  const taken = new Set(
    [
      ...ownPages.map((d) => d.json),
      ...(props.appDocuments ?? []).filter((d) => d.kind === "page").map((d) => parseDocument(d.content)),
    ].flatMap((j) => (isObj(j) && typeof j["route"] === "string" ? [j["route"]] : [])),
  );
  const links = ownPages.reduce((n, d) => n + retarget(structuredClone(d.json), page.route, page.route), 0);
  const external = externalOpeners(props.appDocuments ?? [], ownIds, page.route);
  const route: RouteRename = {
    problem: (t) => {
      if (t === page.route) {
        return undefined;
      }
      if (external.length > 0) {
        return m.routeExternal(external.join(", "));
      }
      const problem = routeProblem(t, taken, page.route);
      return problem === "form" ? m.routeForm : problem === "taken" ? m.routeTaken : undefined;
    },
    note: links > 0 ? m.routeFollows(links) : undefined,
    rename: (t) => {
      burst.current += 1;
      const group = `page:${page.id}:route:${burst.current}`;
      for (const d of ownPages) {
        if (d.path === path) {
          session.edit(
            d.path,
            (draft) => {
              (draft as Obj)["route"] = t;
              retarget(draft, page.route, t);
            },
            { group },
          );
        } else if (retarget(structuredClone(d.json), page.route, t) > 0) {
          session.edit(d.path, (draft) => void retarget(draft, page.route, t), { group });
        }
      }
    },
  };
  const graphs = graphsOf(docs.map((d) => d.json));
  const pageJson = docs.find((d) => d.path === path)?.json;
  return (
    <div className="prop-pane page-settings">
      <datalist id="page-types">
        {baseTypes.map((t) => (
          <option key={t} value={t} />
        ))}
      </datalist>
      <PageSection
        page={page}
        m={m}
        canEdit={canEdit}
        change={change}
        route={route}
        entry={entry}
        onEntry={() => {
          if (plugin !== undefined) {
            session.edit(plugin.path, (d) => setField("entryPage", page.id)(d as Obj), { group: `entry:${page.id}` });
          }
        }}
      />
      <ParamsSection page={page} m={m} canEdit={canEdit} change={change} />
      <DataSection page={page} m={m} onOpenState={props.onOpenState} onOpenData={props.onOpenData} />
      <LifecycleSection page={page} m={m} onOpenGraph={props.onOpenGraph} />
      <RouteSection page={page} m={m} canEdit={canEdit} change={change} graphs={graphs} />
      <SecuritySection page={page} m={m} canEdit={canEdit} change={change} />
      <Section icon="list-checks" label={formsLabel} set={formsOf(pageJson).length} count={formsOf(pageJson).length}>
        <FormsSection pageId={page.id} page={pageJson} canEdit={canEdit} change={change} newId={uuidv7} />
      </Section>
    </div>
  );
}
