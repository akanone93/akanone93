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

/** The name of the sign-in pop-up's window; the pop-up knows itself by it. */
const SIGN_IN_POPUP = "plux-signin";

/**
 * Closes this window if it is the sign-in pop-up a lapsed session opened, and says whether it did (#236). Signing in
 * there is all it was for; the page that opened it sees it close and carries on. Nothing passes between the windows:
 * they share the session cookie, and the opener reloads its own data.
 */
export function closeIfSignInPopup(): boolean {
  if (typeof window === "undefined" || window.name !== SIGN_IN_POPUP || window.opener === null) {
    return false;
  }
  window.close();
  return true;
}

/** Opens the sign-in page (P1) in a popup and calls back when the person closes it, or it closes after signing in. */
export function openSignInPopup(onClosed: () => void): void {
  const popup = window.open("/signin", SIGN_IN_POPUP, "popup,width=480,height=640");
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
