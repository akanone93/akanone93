// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { AlertDialog, Button, Dialog, Field, Icon, Input, Select, Textarea, Tooltip } from "@plux/ui";
import { type CSSProperties, useMemo, useState } from "react";
import { useStore } from "zustand";
import { SheetEmpty, SheetLayout, SheetNavHead, SheetSection } from "../../components/editor-sheet.tsx";
import type { EditSession, JsonValue } from "../editor/index.ts";
import {
  addKeyRecipe,
  dropKeyRecipe,
  KEYS_PATH,
  keyProblem,
  localePath,
  newKeysDocument,
  newLocaleDocument,
  readKeys,
  readMessages,
  removeKeyRecipe,
  renameKeyRecipe,
  setMessageRecipe,
} from "./l10n-model.ts";
import { localisationMessages } from "./localisation.messages.ts";
import "./localisation-area.css";

export type KeyScope = "plugin" | "selection" | "all";

export interface LocalisationOverlayProps {
  readonly open: boolean;
  readonly onClose: () => void;
  readonly locales: readonly string[];
  /** The locale whose messages can be edited, and the session of its messages. */
  readonly locale: string;
  readonly onLocale: (locale: string) => void;
  readonly keysSession: EditSession;
  readonly messagesSession: EditSession;
  readonly canEditKeys: boolean;
  readonly canEditMessages: boolean;
  /** The messages of the other locales, as the server holds them, by locale. */
  readonly otherMessages: ReadonlyMap<string, Readonly<Record<string, string>>>;
  /** Key identifiers the plugin's pages use, and those the selection uses. */
  readonly pluginKeys: ReadonlySet<string>;
  readonly selectionKeys: ReadonlySet<string>;
  /** The documents of the whole app that use each key, by key identifier: a key in use is not deleted (STU-072). */
  readonly usage: ReadonlyMap<string, readonly string[]>;
  readonly schemaVersion: JsonValue;
  readonly newId: () => string;
}

const filtered = (scope: KeyScope, id: string, p: LocalisationOverlayProps): boolean =>
  scope === "all" || (scope === "plugin" ? p.pluginKeys.has(id) : p.selectionKeys.has(id));

/** The localisation table V4 (STU-063): translation keys by locale; the chosen locale's column is edited in place. */
export function LocalisationOverlay(props: LocalisationOverlayProps) {
  const { i18n } = useLingui();
  const m = useMemo(() => localisationMessages(i18n), [i18n]);
  const [scope, setScope] = useState<KeyScope>(props.selectionKeys.size > 0 ? "selection" : "plugin");
  const keysDoc = useStore(props.keysSession.store, (st) => st.docs[KEYS_PATH]?.content);
  const messagesDoc = useStore(props.messagesSession.store, (st) => st.docs[localePath(props.locale)]?.content);
  const keys = useMemo(() => readKeys(keysDoc), [keysDoc]);
  // Every key's name, once: each row's rename checks against it (its own name is never a clash, as an unchanged name
  // is no rename). A list of the other names built per row made each render quadratic in the keys (#214).
  const names = useMemo(() => new Set(keys.map((k) => k.key)), [keys]);
  const mine = useMemo(() => readMessages(messagesDoc), [messagesDoc]);
  const rows = keys.filter((k) => filtered(scope, k.id, props));
  const [deleting, setDeleting] = useState<{ readonly id: string; readonly key: string } | null>(null);
  // One undo step across the keys and every locale's messages, as an extraction is: the key goes with its message and
  // review state in every locale, so no document keeps text for a key that is gone, and undo brings them all back. The
  // other locales are written under the draft's lock, which lets an app manager write every locale (I18N-008).
  const remove = (id: string) => {
    const step = props.newId();
    props.keysSession.edit(KEYS_PATH, removeKeyRecipe(id), { step });
    if (props.canEditMessages && mine[id] !== undefined) {
      props.messagesSession.edit(localePath(props.locale), dropKeyRecipe(id), { step });
    }
    for (const [locale, messages] of props.otherMessages) {
      if (locale !== props.locale && messages[id] !== undefined) {
        void props.keysSession.upsert(
          localePath(locale),
          () => newLocaleDocument(props.schemaVersion, props.newId(), locale, {}),
          dropKeyRecipe(id),
          { step },
        );
      }
    }
  };
  const shown = props.locales.map((l) => ({
    locale: l,
    messages: l === props.locale ? mine : props.otherMessages.get(l),
  }));

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
      icon="translate"
      flush
    >
      <SheetLayout
        nav={
          <div className="flex flex-col gap-4">
            <SheetNavHead title={m.table} count={rows.length} />
            <Field label={m.scope}>
              <Select
                label={m.scope}
                options={[
                  { value: "plugin", label: m.scopePlugin },
                  { value: "selection", label: m.scopeSelection },
                  { value: "all", label: m.scopeAll },
                ]}
                value={scope}
                onValueChange={(v) => setScope(v as KeyScope)}
              />
            </Field>
            <Field label={m.editLocale}>
              <Select
                label={m.editLocale}
                options={props.locales.map((l) => ({ value: l, label: l }))}
                value={props.locale}
                onValueChange={props.onLocale}
              />
            </Field>
            <AddKey {...props} keys={keys} keysDoc={keysDoc} />
          </div>
        }
      >
        {!props.canEditMessages && <p className="text-caption text-muted-foreground">{m.readOnly}</p>}
        <SheetSection title={m.table} icon="translate" wide>
          {rows.length === 0 ? (
            <SheetEmpty icon="translate" title={m.empty} />
          ) : (
            <div className="l10n-keys-scroll">
              <table
                aria-label={m.table}
                className="sheet-grid-table l10n-keys"
                style={{ "--locales": shown.length } as CSSProperties}
              >
                <colgroup>
                  <col className="l10n-col-key" />
                  {shown.map((c) => (
                    <col key={c.locale} />
                  ))}
                  <col className="l10n-col-use" />
                </colgroup>
                <thead>
                  <tr>
                    <th scope="col">{m.keyColumn}</th>
                    {shown.map((c) => (
                      <th key={c.locale} scope="col" className="font-mono" dir="ltr">
                        {c.locale}
                      </th>
                    ))}
                    <th scope="col">{m.usageColumn}</th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((k) => (
                    <tr key={k.id}>
                      <th scope="row" className="font-mono text-xs font-normal" dir="ltr">
                        <KeyName
                          name={k.key}
                          taken={names}
                          canRename={props.canEditKeys}
                          onRename={(key) => props.keysSession.edit(KEYS_PATH, renameKeyRecipe(k.id, key))}
                        />
                      </th>
                      {shown.map((c) => (
                        <td key={c.locale}>
                          {c.locale === props.locale ? (
                            // As tall as its text, so a long message reads whole, as the other locales' do (#221).
                            <Textarea
                              aria-label={m.messageOf(k.key, c.locale)}
                              value={mine[k.id] ?? ""}
                              placeholder={m.missing}
                              readOnly={!props.canEditMessages}
                              onChange={(e) => writeMessage(props, messagesDoc, k.id, e.target.value)}
                            />
                          ) : (
                            <span className="text-sm">
                              {c.messages?.[k.id] ?? <span className="text-muted-foreground italic">{m.missing}</span>}
                            </span>
                          )}
                        </td>
                      ))}
                      <td>
                        <KeyUse
                          files={props.usage.get(k.id) ?? []}
                          canDelete={props.canEditKeys}
                          label={m.deleteKey(k.key)}
                          usedIn={m.usedIn}
                          showAll={m.showAllUses}
                          onDelete={() => setDeleting({ id: k.id, key: k.key })}
                        />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </SheetSection>
      </SheetLayout>
      <AlertDialog
        open={deleting !== null}
        onOpenChange={(open) => {
          if (!open) {
            setDeleting(null);
          }
        }}
        title={m.deleteKeyTitle}
        description={m.deleteKeyBody}
        labels={{ cancel: m.deleteKeyCancel, confirm: m.deleteKeyConfirm }}
        onConfirm={() => {
          const target = deleting;
          setDeleting(null);
          // Checked again at the moment of deleting: a key used since the dialog opened stays.
          if (target !== null && (props.usage.get(target.id) ?? []).length === 0) {
            remove(target.id);
          }
        }}
      />
    </Dialog>
  );
}

/**
 * A key's name, and the way to rename it: a key is referred to by its identifier everywhere (I18N-007), so a rename
 * changes the name alone. Enter or leaving the field keeps a valid name, Escape puts the old one back.
 */
function KeyName(p: {
  readonly name: string;
  /** Every key's name; the key's own is ignored, as keeping it is no rename. */
  readonly taken: ReadonlySet<string>;
  readonly canRename: boolean;
  readonly onRename: (key: string) => void;
}) {
  const m = localisationMessages(useLingui().i18n);
  const [draft, setDraft] = useState<string | null>(null);
  if (draft === null) {
    return (
      <span className="inline-flex items-center gap-1">
        <span>{p.name}</span>
        {p.canRename && (
          <Tooltip content={m.renameKey(p.name)}>
            <Button variant="ghost" size="sm" aria-label={m.renameKey(p.name)} onClick={() => setDraft(p.name)}>
              <Icon name="pencil-simple" />
            </Button>
          </Tooltip>
        )}
      </span>
    );
  }
  const problem = draft === p.name ? undefined : keyProblem(draft, p.taken);
  const finish = () => {
    if (problem === undefined && draft !== p.name) {
      p.onRename(draft);
    }
    setDraft(null);
  };
  return (
    <Field label={m.renameKeyField} error={problem === undefined ? undefined : m.problem(problem)}>
      <Input
        value={draft}
        dir="ltr"
        autoFocus
        onChange={(e) => setDraft(e.target.value.trim())}
        onBlur={finish}
        onKeyDown={(e) => {
          if (e.key === "Enter") {
            e.preventDefault();
            finish();
          } else if (e.key === "Escape") {
            e.preventDefault();
            setDraft(null);
          }
        }}
      />
    </Field>
  );
}

/** How many of a key's documents its row names; the rest are a click away (#221). */
const SHOWN_USES = 2;

/** A document's path, short: the plugin and the file's name without its extensions (`account › ac-offline`). */
export function shortPath(path: string): string {
  const parts = path.split("/");
  const file = (parts[parts.length - 1] ?? path).replace(/(\.page)?\.json$/, "");
  return parts[0] === "plugins" && parts.length >= 3 ? `${parts[1]} › ${file}` : file;
}

/**
 * Where a key is used — how many documents and the first two, the rest under "Show all" — or, for a key nothing uses,
 * the button that deletes it (STU-072).
 */
function KeyUse(p: {
  readonly files: readonly string[];
  readonly canDelete: boolean;
  readonly label: string;
  readonly usedIn: (count: number) => string;
  readonly showAll: (count: number) => string;
  readonly onDelete: () => void;
}) {
  if (p.files.length > 0) {
    const list = (files: readonly string[]) => (
      <ul dir="ltr" className="l10n-uses">
        {files.map((f) => (
          <li key={f}>{shortPath(f)}</li>
        ))}
      </ul>
    );
    return (
      <div className="flex flex-col gap-0.5 text-caption text-muted-foreground">
        <span>{p.usedIn(p.files.length)}</span>
        {list(p.files.slice(0, SHOWN_USES))}
        {p.files.length > SHOWN_USES && (
          <details>
            <summary className="cursor-pointer text-foreground">{p.showAll(p.files.length)}</summary>
            {list(p.files.slice(SHOWN_USES))}
          </details>
        )}
      </div>
    );
  }
  return (
    <Tooltip content={p.label} disabledTrigger={!p.canDelete}>
      <Button variant="ghost" size="sm" aria-label={p.label} disabled={!p.canDelete} onClick={p.onDelete}>
        <Icon name="trash" />
      </Button>
    </Tooltip>
  );
}

function writeMessage(p: LocalisationOverlayProps, doc: unknown, id: string, text: string): void {
  if (!p.canEditMessages) {
    return;
  }
  const path = localePath(p.locale);
  if (doc === undefined) {
    void p.messagesSession.create(path, newLocaleDocument(p.schemaVersion, p.newId(), p.locale, { [id]: text }));
    return;
  }
  p.messagesSession.edit(path, setMessageRecipe(id, text), { group: `l10n:${p.locale}:${id}` });
}

function AddKey(props: LocalisationOverlayProps & { keys: ReturnType<typeof readKeys>; keysDoc: unknown }) {
  const { i18n } = useLingui();
  const m = useMemo(() => localisationMessages(i18n), [i18n]);
  const [name, setName] = useState("");
  const [shown, setShown] = useState(false);
  const problem = keyProblem(name, new Set(props.keys.map((k) => k.key)));
  const add = () => {
    setShown(true);
    if (problem !== undefined || !props.canEditKeys) {
      return;
    }
    const id = props.newId();
    if (props.keysDoc === undefined) {
      void props.keysSession.create(
        KEYS_PATH,
        newKeysDocument(props.schemaVersion, props.newId(), { id, key: name, description: "" }),
      );
    } else {
      props.keysSession.edit(KEYS_PATH, addKeyRecipe({ id, key: name }));
    }
    setName("");
    setShown(false);
  };
  return (
    <div className="flex flex-col gap-2 border-t pt-4">
      <Field label={m.newKey} error={shown && problem !== undefined ? m.problem(problem) : undefined}>
        <Input
          value={name}
          dir="ltr"
          readOnly={!props.canEditKeys}
          onChange={(e) => {
            setName(e.target.value);
            setShown(false);
          }}
        />
      </Field>
      <Button onClick={add} disabled={!props.canEditKeys}>
        <Icon name="plus" />
        {m.addKey}
      </Button>
      {!props.canEditKeys && <span className="text-caption text-muted-foreground">{m.keysReadOnly}</span>}
    </div>
  );
}
