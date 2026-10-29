// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Dialog } from "@plux/ui";
import { type ReactElement, useState } from "react";
import { publishMessages } from "./publish.messages.ts";

/** D42 (SRV-051): every warning of the builds is acknowledged one by one before the reports step passes. */
export function WarningsDialog({
  warnings,
  onConfirm,
  onClose,
}: {
  readonly warnings: readonly string[];
  readonly onConfirm: (acknowledged: string[]) => void;
  readonly onClose: () => void;
}): ReactElement {
  const m = publishMessages(useLingui().i18n);
  const [picked, setPicked] = useState<ReadonlySet<string>>(new Set());
  const toggle = (w: string) => {
    const next = new Set(picked);
    if (!next.delete(w)) {
      next.add(w);
    }
    setPicked(next);
  };
  return (
    <Dialog
      open
      onOpenChange={(o) => !o && onClose()}
      size="md"
      title={m.warningsTitle}
      description={m.warningsBody}
      labels={{ close: m.warningsClose }}
    >
      <ul className="flex flex-col gap-2">
        {warnings.map((w) => (
          <li key={w}>
            <Button
              variant={picked.has(w) ? "primary" : "secondary"}
              aria-pressed={picked.has(w)}
              onClick={() => toggle(w)}
            >
              {w}
            </Button>
          </li>
        ))}
      </ul>
      <div className="flex justify-end gap-2">
        <Button variant="secondary" onClick={onClose}>
          {m.warningsCancel}
        </Button>
        <Button disabled={picked.size < warnings.length} onClick={() => onConfirm([...picked])}>
          {m.warningsConfirm}
        </Button>
      </div>
    </Dialog>
  );
}
