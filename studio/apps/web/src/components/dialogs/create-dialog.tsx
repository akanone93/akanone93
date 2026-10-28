// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { ConnectError } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { Button, Dialog, Field, Input } from "@plux/ui";
import { type FormEvent, type ReactElement, useState } from "react";
import { isKey, isName, keyFromName } from "../../lib/keys.ts";
import { dialogMessages } from "./dialogs.messages.ts";

export interface CreateDialogProps {
  readonly open: boolean;
  readonly onOpenChange: (open: boolean) => void;
  readonly title: string;
  readonly submitLabel: string;
  /** Creates the thing; a refusal is thrown and its message shown in the dialog. */
  readonly onCreate: (input: { readonly key: string; readonly name: string }) => Promise<void>;
}

/** The message of a refusal: the server's own words for a Connect error, else the generic one. */
function refusalOf(error: unknown, fallback: string): string {
  const raw = error instanceof ConnectError ? error.rawMessage : "";
  return raw === "" ? fallback : raw;
}

/** D16-style creation dialog: a name and a key validated like the server, with the server's refusal inline. */
export function CreateDialog({ open, onOpenChange, title, submitLabel, onCreate }: CreateDialogProps): ReactElement {
  const m = dialogMessages(useLingui().i18n);
  const [name, setName] = useState("");
  const [key, setKey] = useState("");
  const [keyEdited, setKeyEdited] = useState(false);
  const [attempted, setAttempted] = useState(false);
  const [working, setWorking] = useState(false);
  const [refusal, setRefusal] = useState<string | undefined>(undefined);
  const reset = () => {
    setName("");
    setKey("");
    setKeyEdited(false);
    setAttempted(false);
    setRefusal(undefined);
  };
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setAttempted(true);
    setRefusal(undefined);
    if (working || !isName(name) || !isKey(key)) {
      return;
    }
    setWorking(true);
    try {
      await onCreate({ key, name: name.trim() });
      reset();
    } catch (error) {
      setRefusal(refusalOf(error, m.createFailed));
    } finally {
      setWorking(false);
    }
  };
  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) {
          reset();
        }
        onOpenChange(next);
      }}
      size="sm"
      title={title}
      labels={{ close: m.close }}
    >
      <form noValidate onSubmit={(e) => void submit(e)} className="flex flex-col gap-3">
        <Field label={m.createName} error={attempted && !isName(name) ? m.createNameInvalid : undefined}>
          <Input
            value={name}
            autoComplete="off"
            onChange={(e) => {
              setName(e.target.value);
              if (!keyEdited) {
                setKey(keyFromName(e.target.value));
              }
            }}
          />
        </Field>
        <Field
          label={m.createKey}
          description={m.createKeyHint}
          error={attempted && !isKey(key) ? m.createKeyInvalid : undefined}
        >
          <Input
            value={key}
            autoComplete="off"
            spellCheck={false}
            onChange={(e) => {
              setKey(e.target.value);
              setKeyEdited(true);
            }}
          />
        </Field>
        {refusal !== undefined && (
          <p role="alert" className="text-body text-danger">
            {refusal}
          </p>
        )}
        <div className="flex justify-end gap-2">
          <Button type="button" variant="secondary" onClick={() => onOpenChange(false)}>
            {m.cancel}
          </Button>
          <Button type="submit" loading={working} loadingLabel={m.createWorking}>
            {submitLabel}
          </Button>
        </div>
      </form>
    </Dialog>
  );
}
