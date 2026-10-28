// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { TestRunService } from "@plux/api";
import { useQuery } from "@tanstack/react-query";
import { useApp } from "../../lib/use-app.tsx";

/** The runs reported for the app, newest first; the first page is shown. */
export function useTestRuns(app: string) {
  const { session } = useApp();
  return useQuery({
    queryKey: ["test-runs", app],
    queryFn: async () => (await createClient(TestRunService, session.transport).listRuns({ appId: app })).runs,
  });
}

/** One run with every scenario and step. */
export function useTestRun(app: string, runId: string) {
  const { session } = useApp();
  return useQuery({
    queryKey: ["test-run", app, runId],
    queryFn: async () => (await createClient(TestRunService, session.transport).getRun({ appId: app, runId })).run,
  });
}
