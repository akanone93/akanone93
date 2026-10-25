// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { useBlocker } from "@tanstack/react-router";
import type { ReactElement } from "react";
import { AlertDialog } from "@plux/ui";
import { dialogMessages } from "./dialogs.messages.ts";

/** D7: blocks in-app navigation while `dirty` and asks; a browser unload uses the native prompt. */
export function UnsavedChangesDialog({ dirty }: { readonly dirty: boolean }): ReactElement {
  const m = dialogMessages(useLingui().i18n);
  const blocker = useBlocker({ shouldBlockFn: () => dirty, enableBeforeUnload: () => dirty, withResolver: true });
  return (
    <AlertDialog
      open={blocker.status === "blocked"}
      title={m.unsavedTitle}
      description={m.unsavedBody}
      labels={{ cancel: m.unsavedStay, confirm: m.unsavedLeave }}
      onConfirm={() => blocker.proceed?.()}
      onOpenChange={(open) => {
        if (!open && blocker.status === "blocked") blocker.reset?.();
      }}
    />
  );
}
