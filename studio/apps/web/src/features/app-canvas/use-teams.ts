// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { OrgService } from "@plux/api";
import { useQuery } from "@tanstack/react-query";
import { useParams } from "@tanstack/react-router";
import { organizationIdOf } from "../../lib/session.ts";
import { useApp } from "../../lib/use-app.tsx";

export interface TeamOption {
  readonly id: string;
  readonly name: string;
}

/** The teams of the organisation, for the owning-team picker (D17); the server lists only what the caller may see. */
export function useTeams(): readonly TeamOption[] {
  const { session } = useApp();
  const organizationId = organizationIdOf((useParams({ strict: false }) as { org?: string }).org ?? "");
  const teams = useQuery({
    queryKey: ["teams", organizationId],
    queryFn: () => createClient(OrgService, session.transport).listTeams({ organizationId, page: { pageSize: 200 } }),
  });
  return (teams.data?.teams ?? []).map((t) => ({ id: t.id, name: t.name }));
}
