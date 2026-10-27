// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Dialog } from "@plux/ui";
import type { ReactElement } from "react";
import { publishMessages } from "./publish.messages.ts";

/** D43: cancels the release request; plugin versions already built stay (F11). */
export function CancelDialog({
  onConfirm,
  onClose,
}: {
  readonly onConfirm: () => void;
  readonly onClose: () => void;
}): ReactElement {
  const m = publishMessages(useLingui().i18n);
  return (
    <Dialog
      open
      onOpenChange={(o) => !o && onClose()}
      size="sm"
      title={m.cancelTitle}
      description={m.cancelBody}
      labels={{ close: m.warningsClose }}
    >
      <div className="flex justify-end gap-2">
        <Button variant="secondary" onClick={onClose}>
          {m.cancelKeep}
        </Button>
        <Button onClick={onConfirm}>{m.cancelConfirm}</Button>
      </div>
    </Dialog>
  );
}
