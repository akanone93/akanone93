// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Dialog } from "@plux/ui";
import { type ReactElement, useState } from "react";
import { libraryMessages } from "./library.messages.ts";

/** One instance that a revised component version changes. */
export interface ImpactRow {
  readonly key: string;
  readonly app: string;
  readonly screen: string;
}

/**
 * D31: the instances across apps that change when the revised component version is saved. When the revision
 * removes props or slots, every instance is flagged because its overrides may name them (STU-084). Confirming
 * saves; cancelling saves nothing.
 */
export function ComponentImpactDialog({
  rows,
  removed,
  save,
  onClose,
}: {
  readonly rows: readonly ImpactRow[];
  readonly removed: readonly string[];
  readonly save: () => Promise<void>;
  readonly onClose: () => void;
}): ReactElement {
  const m = libraryMessages(useLingui().i18n);
  const [failed, setFailed] = useState(false);
  const submit = async () => {
    try {
      await save();
      onClose();
    } catch {
      setFailed(true);
    }
  };
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()} title={m.impactTitle} labels={{ close: m.close }}>
      {rows.length === 0 && <p className="text-body text-secondary">{m.impactNone}</p>}
      <ul className="flex flex-col gap-1">
        {rows.map((r) => (
          <li key={r.key} className="text-body">
            {m.impactRow(r.app, r.screen)}
            {removed.length > 0 && (
              <span className="ms-2 text-caption text-danger">{m.impactConflict(removed.join(", "))}</span>
            )}
          </li>
        ))}
      </ul>
      {failed && (
        <p role="alert" className="text-body text-danger">
          {m.impactFailed}
        </p>
      )}
      <div className="mt-4 flex justify-end gap-2">
        <Button variant="secondary" onClick={onClose}>
          {m.cancel}
        </Button>
        <Button onClick={() => void submit()}>{m.impactConfirm}</Button>
      </div>
    </Dialog>
  );
}
