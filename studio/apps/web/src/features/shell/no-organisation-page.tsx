// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { chromeMessages } from "./shell-chrome.messages.ts";

/** U1 without an organisation. */
export function NoOrganisationPage() {
  const m = chromeMessages(useLingui().i18n);
  return (
    <section className="flex flex-col gap-3">
      <h1 className="text-screen">{m.noOrgTitle}</h1>
      <p className="text-body text-secondary">{m.noOrgBody}</p>
    </section>
  );
}
