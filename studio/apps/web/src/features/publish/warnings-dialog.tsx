// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Dialog } from "@plux/ui";
import { type ReactElement, useState } from "react";
import { publishMessages } from "./publish.messages.ts";
import type { WarningDetail } from "./use-evidence.ts";

/** D42 (SRV-051): every warning of the builds is acknowledged one by one before the reports step passes. */
export function WarningsDialog({
  warnings,
  details,
  onConfirm,
  onClose,
}: {
  readonly warnings: readonly string[];
  /** What the server says of each warning, by its `code@file` name; the bare name shows when it has nothing. */
  readonly details?: ReadonlyMap<string, readonly WarningDetail[]>;
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
          <li key={w} className="flex flex-col gap-1">
            <Button
              variant={picked.has(w) ? "primary" : "secondary"}
              aria-pressed={picked.has(w)}
              onClick={() => toggle(w)}
            >
              {w}
            </Button>
            {(details?.get(w) ?? []).map((d, i) => (
              // biome-ignore lint/suspicious/noArrayIndexKey: the same warning can be reported more than once
              <p key={i} className="text-body text-muted-foreground">
                {d.message}
                {d.fix !== "" && <span className="block">{d.fix}</span>}
              </p>
            ))}
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
