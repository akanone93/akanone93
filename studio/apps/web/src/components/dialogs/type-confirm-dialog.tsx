// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Dialog, Field, Input } from "@plux/ui";
import { type ReactElement, useState } from "react";
import { dialogMessages } from "./dialogs.messages.ts";

export interface TypeConfirmDialogProps {
  readonly open: boolean;
  readonly onOpenChange: (open: boolean) => void;
  /** The exact text the person must type, e.g. the name of the environment (STU-074). */
  readonly phrase: string;
  readonly description: string;
  readonly onConfirm: () => void;
}

/** D4: the confirm button stays disabled until the typed text equals the phrase exactly. */
export function TypeConfirmDialog({
  open,
  onOpenChange,
  phrase,
  description,
  onConfirm,
}: TypeConfirmDialogProps): ReactElement {
  const { i18n } = useLingui();
  const m = dialogMessages(i18n);
  const [typed, setTyped] = useState("");
  const matches = typed === phrase;
  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) {
          setTyped("");
        }
        onOpenChange(next);
      }}
      size="sm"
      title={m.typeConfirmTitle}
      description={description}
      labels={{ close: m.close }}
    >
      <Field label={m.typeConfirmPrompt(phrase)}>
        <Input value={typed} onChange={(e) => setTyped(e.target.value)} autoComplete="off" />
      </Field>
      <div className="flex justify-end gap-2">
        <Button variant="secondary" onClick={() => onOpenChange(false)}>
          {m.cancel}
        </Button>
        <Button variant="danger" disabled={!matches} onClick={onConfirm}>
          {m.confirm}
        </Button>
      </div>
    </Dialog>
  );
}
