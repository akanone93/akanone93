// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useParams, useRouter } from "@tanstack/react-router";
import type { ReactElement } from "react";
import { organizationIdOf } from "../../lib/session.ts";
import { NewAppWizard } from "./wizard.tsx";

/** O3: `/o/:org/apps/new`, the wizard as a route of its own; closing it returns to the organisation's home. */
export function NewAppPage(): ReactElement {
  const org = (useParams({ strict: false }) as { org?: string }).org ?? "";
  const router = useRouter();
  return (
    <NewAppWizard
      open
      org={org}
      organizationId={organizationIdOf(org)}
      onOpenChange={(open) => {
        if (!open) {
          router.history.push(`/o/${org}`);
        }
      }}
    />
  );
}
