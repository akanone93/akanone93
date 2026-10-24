// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { chromeMessages } from "./shell-chrome.messages.ts";

/** P7: shown after sign-out. */
export function SignedOutPage() {
  const m = chromeMessages(useLingui().i18n);
  return (
    <section className="flex flex-col gap-3">
      <h1 className="text-screen">{m.signedOut}</h1>
      <a href="/signin" className="text-body underline">
        {m.signInAgain}
      </a>
    </section>
  );
}
