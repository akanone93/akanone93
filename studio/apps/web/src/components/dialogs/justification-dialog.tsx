// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { limit } from "@plux/schema";
import { Button, Dialog, Field, Textarea } from "@plux/ui";
import { type ReactElement, useState } from "react";
import { dialogMessages } from "./dialogs.messages.ts";

export interface JustificationDialogProps {
  readonly open: boolean;
  readonly onOpenChange: (open: boolean) => void;
  /** Minimum length in codepoints; supplied by the caller from the limit registry, never typed here (LIM-001). */
  readonly minLength: number;
  readonly description: string;
  readonly onSubmit: (justification: string) => void;
}

/** D5 (GOV-024): submit is disabled below the minimum; the maximum is the registry's governance.justificationLength. */
export function JustificationDialog({
  open,
  onOpenChange,
  minLength,
  description,
  onSubmit,
}: JustificationDialogProps): ReactElement {
  const m = dialogMessages(useLingui().i18n);
  const [text, setText] = useState("");
  const count = [...text.trim()].length;
  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      size="md"
      title={m.justificationTitle}
      description={description}
      labels={{ close: m.close }}
    >
      <Field label={m.justificationLabel} description={m.justificationHint(count, minLength)}>
        <Textarea
          value={text}
          maxLength={limit("governance.justificationLength").max}
          onChange={(e) => setText(e.target.value)}
        />
      </Field>
      <div className="flex justify-end gap-2">
        <Button variant="secondary" onClick={() => onOpenChange(false)}>
          {m.cancel}
        </Button>
        <Button disabled={count < minLength} onClick={() => onSubmit(text.trim())}>
          {m.confirm}
        </Button>
      </div>
    </Dialog>
  );
}
