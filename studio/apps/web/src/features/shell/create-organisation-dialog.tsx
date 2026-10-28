// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { OrgService } from "@plux/api";
import { useQueryClient } from "@tanstack/react-query";
import { useRouter } from "@tanstack/react-router";
import type { ReactElement } from "react";
import { CreateDialog } from "../../components/dialogs/index.ts";
import { forgetUser, loadUser } from "../../lib/guards.ts";
import { useApp } from "../../lib/use-app.tsx";
import { chromeMessages } from "./shell-chrome.messages.ts";

/**
 * Creates an organisation (OrgService.CreateOrganization, installation administrators only) and switches to it.
 * The signed-in user and the organisation list are refreshed first, because the route guard admits only
 * organisations the user is a member of.
 */
export function CreateOrganisationDialog({
  open,
  onOpenChange,
}: {
  readonly open: boolean;
  readonly onOpenChange: (open: boolean) => void;
}): ReactElement {
  const m = chromeMessages(useLingui().i18n);
  const { session } = useApp();
  const queryClient = useQueryClient();
  const router = useRouter();
  return (
    <CreateDialog
      open={open}
      onOpenChange={onOpenChange}
      title={m.createOrganisation}
      submitLabel={m.createOrganisation}
      onCreate={async ({ key, name }) => {
        const res = await createClient(OrgService, session.transport).createOrganization({ key, name });
        // The user query's function is itself a fetch of that key, so it is replaced rather than refetched.
        forgetUser(session);
        await Promise.all([loadUser(session), queryClient.invalidateQueries({ queryKey: ["organisations"] })]);
        onOpenChange(false);
        router.history.push(`/o/${res.organization?.id ?? ""}`);
      }}
    />
  );
}
