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
  readonly description: string;
  readonly onSubmit: (justification: string) => void;
}

/** D5 (GOV-024): submit needs a non-empty trimmed justification; the maximum is the registry's governance.justificationLength and the server stays authoritative. */
export function JustificationDialog({
  open,
  onOpenChange,
  description,
  onSubmit,
}: JustificationDialogProps): ReactElement {
  const m = dialogMessages(useLingui().i18n);
  const [text, setText] = useState("");
  const max = limit("governance.justificationLength").max;
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
      <Field label={m.justificationLabel} description={m.justificationHint(count, max)}>
        <Textarea value={text} maxLength={max} onChange={(e) => setText(e.target.value)} />
      </Field>
      <div className="flex justify-end gap-2">
        <Button variant="secondary" onClick={() => onOpenChange(false)}>
          {m.cancel}
        </Button>
        <Button disabled={count === 0} onClick={() => onSubmit(text.trim())}>
          {m.confirm}
        </Button>
      </div>
    </Dialog>
  );
}
