// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Dialog, Field, Input } from "@plux/ui";
import { useEffect, useMemo, useState } from "react";
import { useStore } from "zustand";
import type { JsonValue } from "../editor/index.ts";
import { type ExtractResult, extractKey } from "./extract-key.ts";
import { KEYS_PATH, keyProblem, proposeKey, readKeys } from "./l10n-model.ts";
import type { L10nSessions } from "./l10n-sessions.tsx";
import { localisationMessages } from "./localisation.messages.ts";

export interface ExtractKeyDialogProps {
  readonly open: boolean;
  readonly onClose: () => void;
  readonly sessions: L10nSessions;
  /** The literal text, the property it is in (the proposed key starts with the node's type) and the app's schema version. */
  readonly text: string;
  readonly prefix: string;
  readonly schemaVersion: JsonValue;
  readonly newId: () => string;
  /** Replaces the property with the key reference as one edit tagged with `step`. */
  readonly replaceProp: (reference: { $t: string }, step: string) => void;
}

/** Offers the extraction of literal text to a translation key (I18N-007) and performs it on confirmation. */
export function ExtractKeyDialog(props: ExtractKeyDialogProps) {
  const { i18n } = useLingui();
  const m = useMemo(() => localisationMessages(i18n), [i18n]);
  const { sessions } = props;
  const keysDoc = useStore(sessions.keys.store, (st) => st.docs[KEYS_PATH]?.content);
  const taken = useMemo(() => new Set(readKeys(keysDoc).map((k) => k.key)), [keysDoc]);
  const proposed = useMemo(() => proposeKey(props.text, props.prefix, taken), [props.text, props.prefix, taken]);
  const [typed, setTyped] = useState<string | undefined>(undefined);
  const [failed, setFailed] = useState(false);
  const key = typed ?? proposed;
  const problem = keyProblem(key, taken);
  const sourceHeld = sessions.locale === sessions.defaultLocale && sessions.canEditMessages;
  const allowed = sessions.canEditKeys && sourceHeld;

  const { locale, defaultLocale, setLocale } = sessions;
  useEffect(() => {
    if (props.open && locale !== defaultLocale) {
      setLocale(defaultLocale);
    }
  }, [props.open, locale, defaultLocale, setLocale]);

  const confirm = async () => {
    if (problem !== undefined || !allowed) {
      return;
    }
    const result: ExtractResult = await extractKey({
      keys: sessions.keys,
      source: sessions.messages,
      sourceLocale: sessions.defaultLocale,
      schemaVersion: props.schemaVersion,
      newId: props.newId,
      key,
      text: props.text,
      replaceProp: props.replaceProp,
    });
    if (result.ok) {
      props.onClose();
    } else {
      setFailed(true);
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
      title={m.extractTitle}
      description={m.extractHint(sessions.defaultLocale)}
      labels={{ close: m.extractCancel }}
    >
      <div className="mt-3 flex flex-col gap-3">
        <Field label={m.extractText}>
          <Input value={props.text} readOnly />
        </Field>
        <Field label={m.newKey} error={problem === undefined ? undefined : m.problem(problem)}>
          <Input
            value={key}
            dir="ltr"
            onChange={(e) => {
              setTyped(e.target.value);
              setFailed(false);
            }}
          />
        </Field>
        {!allowed && <p className="text-caption text-secondary">{m.extractUnavailable}</p>}
        {failed && (
          <p role="alert" className="text-caption text-danger">
            {m.extractFailed}
          </p>
        )}
        <div className="flex justify-end gap-2">
          <Button variant="secondary" onClick={props.onClose}>
            {m.extractCancel}
          </Button>
          <Button onClick={() => void confirm()} disabled={problem !== undefined || !allowed}>
            {m.extractConfirm}
          </Button>
        </div>
      </div>
    </Dialog>
  );
}
