// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { AlertDialog } from "@plux/ui";
import type { ReactElement } from "react";
import { dialogMessages } from "./dialogs.messages.ts";

/** D6: the session ended; the one action opens the sign-in popup so the page and its work stay (F4). */
export function SignedOutDialog({
  open,
  onSignIn,
}: {
  readonly open: boolean;
  readonly onSignIn: () => void;
}): ReactElement {
  const m = dialogMessages(useLingui().i18n);
  return (
    <AlertDialog
      open={open}
      title={m.signedOutTitle}
      description={m.signedOutBody}
      labels={{ cancel: m.cancel, confirm: m.signIn }}
      onConfirm={onSignIn}
    />
  );
}

/** Opens the sign-in page (P1) in a popup and calls back when the person closes it. */
export function openSignInPopup(onClosed: () => void): void {
  const popup = window.open("/signin", "plux-signin", "popup,width=480,height=640");
  if (popup === null) {
    onClosed();
    return;
  }
  const timer = window.setInterval(() => {
    if (popup.closed) {
      window.clearInterval(timer);
      onClosed();
    }
  }, 500);
}
