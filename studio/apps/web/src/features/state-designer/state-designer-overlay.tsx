// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Dialog, Field, Icon, Input, Select } from "@plux/ui";
import type { Problem } from "@plux/validator";
import { useEffect, useMemo, useState } from "react";
import { useStore } from "zustand";
import type { EditSession } from "../editor/index.ts";
import {
  BlockedDeletePanel,
  buildWhereUsed,
  checkStateDelete,
  isBlocked,
  type LoadedDoc,
  type Use,
} from "../safety/index.ts";
import { type StateMessages, stateMessages } from "./state.messages.ts";
import {
  addEntryRecipe,
  baseTypes,
  type Container,
  composeType,
  declaredTypes,
  freshName,
  nameProblem,
  type Persistence,
  parseDefault,
  parseType,
  persistences,
  readState,
  removeEntryRecipe,
  type StateEntry,
  setFieldRecipe,
} from "./state-model.ts";

/** Milliseconds after the last change before the validator checks the entry (the same as the JSON editor). */
export const STATE_DEBOUNCE_MS = 300;

export interface StateDesignerOverlayProps {
  readonly open: boolean;
  readonly onClose: () => void;
  /** The plugin's edit session and whether it may change documents (the draft lock). */
  readonly session: EditSession;
  readonly editable: boolean;
  /** The plugin's document: its path and id, where the entries live. */
  readonly pluginPath: string | undefined;
  readonly pluginId: string;
  /** Every loaded document of the app as the server holds it; the session's unsaved content is laid over it. */
  readonly documents: readonly LoadedDoc[];
  readonly validatePage: ((file: string, data: string) => Promise<readonly Problem[]>) | undefined;
  readonly newId: () => string;
}

type Verdict =
  | { readonly state: "checking" }
  | { readonly state: "unavailable" }
  | { readonly state: "done"; readonly problems: readonly Problem[] };

const persistenceLabel = (m: StateMessages, p: Persistence): string => m[p];

/** The validator's findings about entry `index` of the plugin document, `STATE_DEBOUNCE_MS` after the last change. */
function useVerdict(
  open: boolean,
  path: string | undefined,
  content: unknown,
  index: number,
  validate: StateDesignerOverlayProps["validatePage"],
): Verdict {
  const [verdict, setVerdict] = useState<Verdict>({ state: "checking" });
  useEffect(() => {
    if (!open || path === undefined || index < 0 || content === undefined) {
      return undefined;
    }
    if (validate === undefined) {
      setVerdict({ state: "unavailable" });
      return undefined;
    }
    let live = true;
    setVerdict({ state: "checking" });
    const timer = setTimeout(() => {
      validate(path, JSON.stringify(content))
        .then((found) => {
          const at = `/state/${index}`;
          const mine = found.filter((p) => p.file === path && (p.path === at || p.path.startsWith(`${at}/`)));
          return live && setVerdict({ state: "done", problems: mine });
        })
        .catch(() => live && setVerdict({ state: "unavailable" }));
    }, STATE_DEBOUNCE_MS);
    return () => {
      live = false;
      clearTimeout(timer);
    };
  }, [open, path, content, index, validate]);
  return verdict;
}

/** The state designer V5 (STU-063): the plugin's state entries, edited through the plugin's edit pipeline. */
export function StateDesignerOverlay(props: StateDesignerOverlayProps) {
  const { i18n } = useLingui();
  const m = useMemo(() => stateMessages(i18n), [i18n]);
  const { session } = props;
  const canEdit = props.editable && props.pluginPath !== undefined;
  const path = props.pluginPath;
  const doc = useStore(session.store, (st) => (path === undefined ? undefined : st.docs[path]?.content));
  const local = useStore(session.store, (st) => st.docs);
  const entries = useMemo(() => readState(doc), [doc]);
  const [chosen, setChosen] = useState<string | undefined>(undefined);
  const [blocked, setBlocked] = useState<readonly Use[] | null>(null);
  const selected = entries.find((e) => e.id === chosen) ?? entries[0];
  const index = selected === undefined ? -1 : entries.indexOf(selected);

  const index_ = useMemo(
    () =>
      buildWhereUsed(
        props.documents.map((d) => {
          const edited = local[d.path]?.content;
          return edited === undefined ? d : { ...d, json: edited };
        }),
      ),
    [props.documents, local],
  );
  const uses = (name: string): readonly Use[] => checkStateDelete(index_, props.pluginId, name).incoming;
  const types = useMemo(
    () => declaredTypes(doc, props.documents.find((d) => d.kind === "app")?.json),
    [doc, props.documents],
  );
  const verdict = useVerdict(props.open, path, doc, index, props.validatePage);

  const edit = (id: string, field: string, value: unknown) => {
    if (canEdit && path !== undefined) {
      session.edit(path, setFieldRecipe(id, field, value), { group: `state:${id}:${field}` });
    }
  };
  const add = () => {
    if (!canEdit || path === undefined) {
      return;
    }
    const id = props.newId();
    session.edit(path, addEntryRecipe({ id, name: freshName(entries.map((e) => e.name)), type: "string" }));
    setChosen(id);
  };
  const remove = (e: StateEntry) => {
    const check = checkStateDelete(index_, props.pluginId, e.name);
    if (isBlocked(check)) {
      setBlocked(check.incoming);
      return;
    }
    if (canEdit && path !== undefined) {
      session.edit(path, removeEntryRecipe(e.id));
      setChosen(undefined);
    }
  };

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
      <div className="flex min-h-0 flex-1 gap-4">
        <nav aria-label={m.entries} className="flex w-64 shrink-0 flex-col gap-2 overflow-y-auto">
          <Button onClick={add} disabled={!canEdit}>
            <Icon name="plus" />
            {m.add}
          </Button>
          {entries.length === 0 && <p className="text-body text-secondary">{m.none}</p>}
          <ul className="flex flex-col gap-1">
            {entries.map((e) => (
              <li key={e.id}>
                <Button
                  variant={e.id === selected?.id ? "secondary" : "ghost"}
                  aria-pressed={e.id === selected?.id}
                  onClick={() => setChosen(e.id)}
                >
                  <span className="font-mono text-data" dir="ltr">
                    {e.name}
                  </span>
                  <span className="font-mono text-tag text-secondary" dir="ltr">
                    {e.type}
                  </span>
                  <span className="text-caption text-secondary">{m.uses(uses(e.name).length)}</span>
                </Button>
              </li>
            ))}
          </ul>
        </nav>
        <div className="flex min-w-0 flex-1 flex-col gap-3 overflow-y-auto">
          {!canEdit && <p className="text-caption text-secondary">{m.readOnly}</p>}
          {selected !== undefined && (
            <EntryForm
              key={selected.id}
              entry={selected}
              others={entries.filter((e) => e.id !== selected.id).map((e) => e.name)}
              types={types}
              canEdit={canEdit}
              m={m}
              verdict={verdict}
              onEdit={(field, value) => edit(selected.id, field, value)}
              onRemove={() => remove(selected)}
            />
          )}
        </div>
      </div>
      <BlockedDeletePanel
        open={blocked !== null}
        check={{ entry: false, incoming: blocked ?? [], outgoing: [] }}
        onNavigate={() => {
          setBlocked(null);
          props.onClose();
        }}
        onOpenChange={(open) => {
          if (!open) {
            setBlocked(null);
          }
        }}
      />
    </Dialog>
  );
}

interface FormProps {
  readonly entry: StateEntry;
  readonly others: readonly string[];
  readonly types: readonly string[];
  readonly canEdit: boolean;
  readonly m: StateMessages;
  readonly verdict: Verdict;
  readonly onEdit: (field: string, value: unknown) => void;
  readonly onRemove: () => void;
}

function EntryForm({ entry, others, types, canEdit, m, verdict, onEdit, onRemove }: FormProps) {
  const [name, setName] = useState(entry.name);
  const [text, setText] = useState(entry.defaultText);
  const nameIssue = nameProblem(name, others);
  const parsedDefault = parseDefault(text);
  const parts = parseType(entry.type);
  const baseOptions = [...baseTypes, ...types].map((t) => ({ value: t, label: t }));
  const setType = (next: Partial<NonNullable<typeof parts>>) => {
    if (parts !== undefined) {
      onEdit("type", composeType({ ...parts, ...next }));
    }
  };
  return (
    <div className="flex flex-col gap-3">
      <Field label={m.name} error={nameIssue === undefined ? undefined : m.problem(nameIssue)}>
        <Input
          value={name}
          dir="ltr"
          readOnly={!canEdit}
          onChange={(e) => {
            setName(e.target.value);
            if (nameProblem(e.target.value, others) === undefined) {
              onEdit("name", e.target.value);
            }
          }}
        />
      </Field>
      {parts === undefined ? (
        <Field label={m.rawType}>
          <Input value={entry.type} dir="ltr" readOnly={!canEdit} onChange={(e) => onEdit("type", e.target.value)} />
        </Field>
      ) : (
        <div className="flex flex-wrap items-end gap-3">
          <Select
            label={m.container}
            options={[
              { value: "single", label: m.single },
              { value: "list", label: m.list },
              { value: "map", label: m.map },
            ]}
            value={parts.container}
            disabled={!canEdit}
            onValueChange={(v) => setType({ container: v as Container })}
          />
          <Select
            label={m.base}
            options={
              baseOptions.some((o) => o.value === parts.base)
                ? baseOptions
                : [...baseOptions, { value: parts.base, label: parts.base }]
            }
            value={parts.base}
            disabled={!canEdit}
            onValueChange={(v) => setType({ base: v })}
          />
          <label className="flex items-center gap-2 text-body">
            <input
              type="checkbox"
              checked={parts.nullable}
              disabled={!canEdit}
              onChange={(e) => setType({ nullable: e.target.checked })}
            />
            {m.nullable}
          </label>
        </div>
      )}
      <Select
        label={m.persistence}
        options={persistences.map((p) => ({ value: p, label: persistenceLabel(m, p) }))}
        value={entry.persistence ?? "memory"}
        disabled={!canEdit}
        onValueChange={(v) => onEdit("persistence", v)}
      />
      <label className="flex items-center gap-2 text-body">
        <input
          type="checkbox"
          checked={entry.sensitive}
          disabled={!canEdit}
          onChange={(e) => onEdit("sensitive", e.target.checked ? true : undefined)}
        />
        {m.sensitive}
      </label>
      {entry.computed ? (
        <p className="text-caption text-secondary">{m.computedNote}</p>
      ) : (
        <Field label={m.defaultValue} error={parsedDefault === undefined ? m.notJson : undefined}>
          <Input
            value={text}
            dir="ltr"
            readOnly={!canEdit}
            onChange={(e) => {
              setText(e.target.value);
              const next = parseDefault(e.target.value);
              if (next === null) {
                onEdit("default", undefined);
              } else if (next !== undefined) {
                onEdit("default", next.value);
              }
            }}
          />
        </Field>
      )}
      <section aria-label={m.problems} className="flex flex-col gap-1">
        <h3 className="text-label font-medium">{m.problems}</h3>
        <Findings verdict={verdict} m={m} />
      </section>
      <div>
        <Button variant="secondary" onClick={onRemove} disabled={!canEdit}>
          <Icon name="trash" />
          {m.remove}
        </Button>
      </div>
    </div>
  );
}

function Findings({ verdict, m }: { readonly verdict: Verdict; readonly m: StateMessages }) {
  if (verdict.state === "checking") {
    return <p className="text-body text-secondary">{m.checking}</p>;
  }
  if (verdict.state === "unavailable") {
    return <p className="text-body text-secondary">{m.unavailable}</p>;
  }
  const shown = verdict.problems.filter((p) => p.severity !== "info");
  return shown.length === 0 ? (
    <p className="text-body text-secondary">{m.valid}</p>
  ) : (
    <ul className="flex flex-col gap-1">
      {shown.map((p) => (
        <li key={`${p.code}${p.path}${p.message}`} className="text-body">
          {p.code === "" ? p.message : `${p.code}: ${p.message}`}
        </li>
      ))}
    </ul>
  );
}
