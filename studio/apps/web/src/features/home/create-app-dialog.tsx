// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { AppService } from "@plux/api";
import { useQueryClient } from "@tanstack/react-query";
import { useRouter } from "@tanstack/react-router";
import type { ReactElement } from "react";
import { CreateDialog } from "../../components/dialogs/index.ts";
import { useApp } from "../../lib/use-app.tsx";
import { homeMessages } from "./home.messages.ts";

/** Creates an app in the organisation (AppService.CreateApp, needs app.manage) and opens its canvas. */
export function CreateAppDialog({
  open,
  onOpenChange,
  organizationId,
  org,
}: {
  readonly open: boolean;
  readonly onOpenChange: (open: boolean) => void;
  /** The organisation's immutable ID, which the request carries. */
  readonly organizationId: string;
  /** The `:org` URL parameter, which the canvas path continues. */
  readonly org: string;
}): ReactElement {
  const m = homeMessages(useLingui().i18n);
  const { session } = useApp();
  const queryClient = useQueryClient();
  const router = useRouter();
  return (
    <CreateDialog
      open={open}
      onOpenChange={onOpenChange}
      title={m.createApp}
      submitLabel={m.createApp}
      onCreate={async ({ key, name }) => {
        const res = await createClient(AppService, session.transport).createApp({ organizationId, key, name });
        await queryClient.invalidateQueries({ queryKey: ["apps"] });
        onOpenChange(false);
        router.history.push(`/o/${org}/apps/${res.app?.id ?? ""}`);
      }}
    />
  );
}
