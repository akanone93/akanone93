// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button } from "@plux/ui";
import { useState } from "react";
import { useCurrentUser } from "../../lib/use-current-user.ts";
import { CreateOrganisationDialog } from "./create-organisation-dialog.tsx";
import { chromeMessages } from "./shell-chrome.messages.ts";

/** U1 without an organisation; an installation administrator can create the first one. */
export function NoOrganisationPage() {
  const m = chromeMessages(useLingui().i18n);
  const user = useCurrentUser();
  const admin = user.data?.installationAdmin === true;
  const [creating, setCreating] = useState(false);
  return (
    <section className="flex flex-col gap-3">
      <h1 className="text-screen">{m.noOrgTitle}</h1>
      <p className="text-body text-muted-foreground">{admin ? m.noOrgAdminBody : m.noOrgBody}</p>
      {admin && (
        <div>
          <Button onClick={() => setCreating(true)}>{m.createOrganisation}</Button>
          <CreateOrganisationDialog open={creating} onOpenChange={setCreating} />
        </div>
      )}
    </section>
  );
}
