// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Dialog, Field, Icon, Input, Select } from "@plux/ui";
import { useMemo, useState } from "react";
import { useStore } from "zustand";
import { SheetEmpty, SheetLayout, SheetNavHead, SheetSection } from "../../components/editor-sheet.tsx";
import type { EditSession, JsonValue } from "../editor/index.ts";
import {
  addKeyRecipe,
  KEYS_PATH,
  keyProblem,
  localePath,
  newKeysDocument,
  newLocaleDocument,
  readKeys,
  readMessages,
  setMessageRecipe,
} from "./l10n-model.ts";
import { localisationMessages } from "./localisation.messages.ts";

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
  const mine = useMemo(() => readMessages(messagesDoc), [messagesDoc]);
  const rows = keys.filter((k) => filtered(scope, k.id, props));
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
        <SheetSection title={m.table} icon="translate">
          {rows.length === 0 ? (
            <SheetEmpty icon="translate" title={m.empty} />
          ) : (
            <table aria-label={m.table} className="sheet-grid-table">
              <thead>
                <tr>
                  <th scope="col">{m.keyColumn}</th>
                  {shown.map((c) => (
                    <th key={c.locale} scope="col" className="font-mono" dir="ltr">
                      {c.locale}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {rows.map((k) => (
                  <tr key={k.id}>
                    <th scope="row" className="font-mono text-xs font-normal" dir="ltr">
                      {k.key}
                    </th>
                    {shown.map((c) => (
                      <td key={c.locale}>
                        {c.locale === props.locale ? (
                          <Input
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
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </SheetSection>
      </SheetLayout>
    </Dialog>
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
